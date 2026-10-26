package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jide-dev/jide/internal/dap"
	jide "github.com/jide-dev/jide/proto/jide"
)

// DAPDispatcher owns the DAP client for one daemon connection and dispatches
// the typed dap/* protocol methods. Adapter events are sent to onEvent as
// server-to-client protocol notifications.
type DAPDispatcher struct {
	mu      sync.Mutex
	client  *dap.Client
	caps    dap.Capabilities
	pending *dap.PendingRequest
	onEvent func(jide.DAPEventParams)
	closed  bool
}

// NewDAPDispatcher creates an empty DAP protocol dispatcher.
func NewDAPDispatcher(onEvent func(jide.DAPEventParams)) *DAPDispatcher {
	return &DAPDispatcher{onEvent: onEvent}
}

// Handle executes one DAP protocol method and returns its typed result.
func (d *DAPDispatcher) Handle(ctx context.Context, method jide.Method, rawParams json.RawMessage) (any, error) {
	if ctx == nil {
		return nil, errors.New("jided: DAP request context is nil")
	}
	switch method {
	case jide.MethodDAPStart:
		var params jide.DAPStartParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		return d.start(ctx, params)
	case jide.MethodDAPLaunch:
		var params jide.DAPArgumentsParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		if err := d.beginDebuggee(ctx, "launch", params.Arguments); err != nil {
			return nil, err
		}
		return jide.DAPEmptyResult{}, nil
	case jide.MethodDAPAttach:
		var params jide.DAPArgumentsParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		if err := d.beginDebuggee(ctx, "attach", params.Arguments); err != nil {
			return nil, err
		}
		return jide.DAPEmptyResult{}, nil
	case jide.MethodDAPConfigurationDone:
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		if err := client.ConfigurationDone(ctx); err != nil {
			return nil, err
		}
		d.waitPending(client)
		return jide.DAPEmptyResult{}, nil
	case jide.MethodDAPSetBreakpoints:
		var params jide.DAPSetBreakpointsParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		breakpoints := make([]dap.SourceBreakpoint, len(params.Breakpoints))
		for i, value := range params.Breakpoints {
			breakpoints[i] = dap.SourceBreakpoint{Line: value.Line, Column: value.Column, Condition: value.Condition,
				HitCondition: value.HitCondition, LogMessage: value.LogMessage}
		}
		verified, err := client.SetBreakpoints(ctx, dap.Source{Name: params.Source.Name, Path: params.Source.Path, SourceReference: params.Source.SourceReference}, breakpoints)
		return protocolDAPResult[jide.DAPBreakpointsResult](jide.DAPBreakpointsResult{Breakpoints: protocolDAPBreakpoints(verified)}, err)
	case jide.MethodDAPSetFunctionBreakpoints:
		var params jide.DAPFunctionBreakpointsParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		breakpoints := make([]dap.FunctionBreakpoint, len(params.Breakpoints))
		for i, value := range params.Breakpoints {
			breakpoints[i] = dap.FunctionBreakpoint{Name: value.Name, Condition: value.Condition, HitCondition: value.HitCondition}
		}
		verified, err := client.SetFunctionBreakpoints(ctx, breakpoints)
		return protocolDAPResult[jide.DAPBreakpointsResult](jide.DAPBreakpointsResult{Breakpoints: protocolDAPBreakpoints(verified)}, err)
	case jide.MethodDAPSetExceptionBreakpoints:
		var params jide.DAPExceptionBreakpointsParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		caps := d.capabilities()
		breakpoints, err := client.SetExceptionBreakpointsNamedResult(ctx, params.Filters, caps)
		return protocolDAPResult[jide.DAPBreakpointsResult](jide.DAPBreakpointsResult{Breakpoints: protocolDAPBreakpoints(breakpoints)}, err)
	case jide.MethodDAPThreads:
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		threads, err := client.Threads(ctx)
		return protocolDAPResult[jide.DAPThreadsResult](protocolDAPThreadsResult(threads), err)
	case jide.MethodDAPStackTrace:
		var params jide.DAPStackTraceParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		result, err := client.StackTrace(ctx, params.ThreadID, params.StartFrame, params.Levels)
		return protocolDAPResult[jide.DAPStackTraceResult](protocolDAPStackResult(result), err)
	case jide.MethodDAPContinue, jide.MethodDAPNext, jide.MethodDAPStepIn, jide.MethodDAPStepOut, jide.MethodDAPPause, jide.MethodDAPExceptionInfo:
		var params jide.DAPThreadParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		switch method {
		case jide.MethodDAPContinue:
			result, err := client.Continue(ctx, params.ThreadID)
			return jide.DAPContinueResult{AllThreadsContinued: result.AllThreadsContinued}, err
		case jide.MethodDAPNext:
			return jide.DAPEmptyResult{}, client.Next(ctx, params.ThreadID)
		case jide.MethodDAPStepIn:
			if params.TargetID > 0 {
				return jide.DAPEmptyResult{}, client.StepInAtTarget(ctx, params.ThreadID, params.TargetID)
			}
			return jide.DAPEmptyResult{}, client.StepIn(ctx, params.ThreadID)
		case jide.MethodDAPStepOut:
			return jide.DAPEmptyResult{}, client.StepOut(ctx, params.ThreadID)
		case jide.MethodDAPPause:
			return jide.DAPEmptyResult{}, client.Pause(ctx, params.ThreadID)
		default:
			result, err := client.ExceptionInfo(ctx, params.ThreadID)
			return protocolDAPResult[jide.DAPExceptionInfoResult](result, err)
		}
	case jide.MethodDAPStepInTargets:
		var params jide.DAPFrameParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		targets, err := client.StepInTargets(ctx, params.FrameID)
		result := jide.DAPStepInTargetsResult{Targets: make([]jide.DAPStepInTarget, len(targets))}
		for i, target := range targets {
			result.Targets[i] = jide.DAPStepInTarget{ID: target.ID, Label: target.Label}
		}
		return result, err
	case jide.MethodDAPScopes:
		var params jide.DAPFrameParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		scopes, err := client.Scopes(ctx, params.FrameID)
		return protocolDAPResult[jide.DAPScopesResult](map[string]any{"scopes": scopes}, err)
	case jide.MethodDAPVariables:
		var params jide.DAPVariablesParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		variables, err := client.Variables(ctx, params.VariablesReference, params.Start, params.Count)
		return protocolDAPResult[jide.DAPVariablesResult](map[string]any{"variables": variables}, err)
	case jide.MethodDAPEvaluate:
		var params jide.DAPEvaluateParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		caps := d.capabilities()
		result, err := client.EvaluateIfSupported(ctx, params.Expression, params.FrameID, params.Context, caps)
		return protocolDAPResult[jide.DAPEvaluateResult](result, err)
	case jide.MethodDAPSetVariable:
		var params jide.DAPSetVariableParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		caps := d.capabilities()
		result, err := client.SetVariable(ctx, params.VariablesReference, params.Name, params.Value, caps)
		return protocolDAPResult[jide.DAPSetVariableResult](result, err)
	case jide.MethodDAPTerminate:
		client, err := d.currentClient()
		if err != nil {
			return nil, err
		}
		return jide.DAPEmptyResult{}, client.Terminate(ctx)
	case jide.MethodDAPDisconnect:
		var params jide.DAPDisconnectParams
		if err := decodeDAPParams(rawParams, &params); err != nil {
			return nil, err
		}
		return jide.DAPEmptyResult{}, d.disconnect(ctx, params.TerminateDebuggee)
	default:
		return nil, fmt.Errorf("jided: unsupported DAP method %q", method)
	}
}

// Close releases the active adapter and any pending launch/attach request.
func (d *DAPDispatcher) Close() error {
	d.mu.Lock()
	d.closed = true
	client := d.client
	pending := d.pending
	d.client = nil
	d.pending = nil
	d.caps = dap.Capabilities{}
	d.mu.Unlock()
	if pending != nil {
		pending.Cancel()
	}
	if client != nil {
		return client.Close()
	}
	return nil
}

func (d *DAPDispatcher) start(ctx context.Context, params jide.DAPStartParams) (jide.DAPStartResult, error) {
	var client *dap.Client
	var caps dap.Capabilities
	var err error
	if params.ProjectPath != "" {
		client, caps, err = (&dap.Discoverer{}).StartJuliaDebugSession(ctx, params.ProjectPath, params.WorkingDirectory, dap.DefaultTimeoutPolicy, nil)
	} else {
		if strings.TrimSpace(params.Executable) == "" {
			return jide.DAPStartResult{}, errors.New("jided: decode DAP parameters: dap/start requires projectPath or executable")
		}
		command, findErr := dap.FindAdapter(params.Executable)
		if findErr != nil {
			return jide.DAPStartResult{}, findErr
		}
		command.Args = append([]string(nil), params.Arguments...)
		command.Directory = params.Directory
		client, caps, err = dap.Start(ctx, dap.Options{Command: command, Timeouts: dap.DefaultTimeoutPolicy})
	}
	if err != nil {
		return jide.DAPStartResult{}, err
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		_ = client.Close()
		return jide.DAPStartResult{}, errors.New("jided: DAP dispatcher is closed")
	}
	previous := d.client
	previousPending := d.pending
	d.client = client
	d.caps = caps
	d.pending = nil
	d.mu.Unlock()
	if previousPending != nil {
		previousPending.Cancel()
	}
	if previous != nil {
		_ = previous.Close()
	}
	go d.forwardEvents(client)
	go d.watchClient(client)
	return protocolDAPResult[jide.DAPStartResult](jide.DAPStartResult{Capabilities: protocolDAPCapabilities(caps)}, nil)
}

func (d *DAPDispatcher) beginDebuggee(ctx context.Context, kind string, arguments map[string]any) error {
	client, err := d.currentClient()
	if err != nil {
		return err
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	var pending *dap.PendingRequest
	if kind == "launch" {
		pending, err = client.BeginLaunch(ctx, arguments)
	} else {
		pending, err = client.BeginAttach(ctx, arguments)
	}
	if err != nil {
		return err
	}
	d.mu.Lock()
	if d.client != client || d.closed {
		d.mu.Unlock()
		pending.Cancel()
		return errors.New("jided: DAP session changed while starting the debuggee")
	}
	previous := d.pending
	d.pending = pending
	d.mu.Unlock()
	if previous != nil {
		previous.Cancel()
	}
	return nil
}

func (d *DAPDispatcher) waitPending(client *dap.Client) {
	d.mu.Lock()
	pending := d.pending
	d.pending = nil
	d.mu.Unlock()
	if pending == nil {
		return
	}
	go func() {
		if _, err := pending.Wait(); err != nil {
			d.publishEvent(client, jide.DAPEventParams{Event: "jide/error", Body: mustJSON(map[string]string{"message": err.Error()})})
		}
	}()
}

func (d *DAPDispatcher) disconnect(ctx context.Context, terminate bool) error {
	d.mu.Lock()
	client := d.client
	pending := d.pending
	d.client = nil
	d.pending = nil
	d.caps = dap.Capabilities{}
	d.mu.Unlock()
	if pending != nil {
		pending.Cancel()
	}
	if client == nil {
		return nil
	}
	err := client.Disconnect(ctx, terminate)
	closeErr := client.Close()
	return errors.Join(err, closeErr)
}

func (d *DAPDispatcher) currentClient() (*dap.Client, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, errors.New("jided: DAP dispatcher is closed")
	}
	if d.client == nil {
		return nil, errors.New("jided: no DAP session is active")
	}
	return d.client, nil
}

func (d *DAPDispatcher) capabilities() dap.Capabilities {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.caps
}

func (d *DAPDispatcher) forwardEvents(client *dap.Client) {
	for event := range client.Events() {
		d.publishEvent(client, jide.DAPEventParams{Event: event.Name, Body: event.Body})
	}
}

func (d *DAPDispatcher) watchClient(client *dap.Client) {
	err := client.Wait(context.Background())
	d.mu.Lock()
	active := d.client == client
	handler := d.onEvent
	if active {
		d.client = nil
		d.pending = nil
		d.caps = dap.Capabilities{}
	}
	d.mu.Unlock()
	if active && err != nil && handler != nil {
		handler(jide.DAPEventParams{Event: "jide/error", Body: mustJSON(map[string]string{"message": err.Error()})})
	}
}

func (d *DAPDispatcher) publishEvent(client *dap.Client, event jide.DAPEventParams) {
	d.mu.Lock()
	active := d.client == client && !d.closed
	handler := d.onEvent
	d.mu.Unlock()
	if active && handler != nil {
		handler(event)
	}
}

func decodeDAPParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("jided: decode DAP parameters: %w", err)
	}
	return nil
}

func protocolDAPResult[T any](value any, err error) (T, error) {
	var result T
	if err != nil {
		return result, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(raw, &result)
	return result, err
}

func protocolDAPCapabilities(value dap.Capabilities) jide.DAPCapabilities {
	result, _ := protocolDAPResult[jide.DAPCapabilities](value, nil)
	return result
}

func protocolDAPBreakpoints(values []dap.Breakpoint) []jide.DAPBreakpoint {
	result, _ := protocolDAPResult[[]jide.DAPBreakpoint](values, nil)
	return result
}

func protocolDAPThreadsResult(values []dap.Thread) jide.DAPThreadsResult {
	result, _ := protocolDAPResult[jide.DAPThreadsResult](map[string]any{"threads": values}, nil)
	return result
}

func protocolDAPStackResult(value dap.StackTraceResult) jide.DAPStackTraceResult {
	result, _ := protocolDAPResult[jide.DAPStackTraceResult](value, nil)
	return result
}

func mustJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}
