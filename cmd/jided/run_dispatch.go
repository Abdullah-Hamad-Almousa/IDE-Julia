package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jide-dev/jide/internal/jlproc"
	"github.com/jide-dev/jide/internal/runconf"
	"github.com/jide-dev/jide/internal/testrun"
	jide "github.com/jide-dev/jide/proto/jide"
)

// RunDispatcher owns run-configuration and test-run processes for a daemon
// connection. Events are sent as server-to-client protocol notifications.
type RunDispatcher struct {
	runner   *runconf.Runner
	registry *runconf.Registry
	tests    *testrun.Runner
	onEvent  func(jide.Method, any)
	mu       sync.Mutex
	testRuns map[string]context.CancelFunc
	closed   bool
	workers  sync.WaitGroup
}

// NewRunDispatcher creates run services using runtime and an event callback.
func NewRunDispatcher(runtime jlproc.Runtime, onEvent func(jide.Method, any)) *RunDispatcher {
	registry := runconf.NewRegistry()
	return &RunDispatcher{
		runner: runconf.NewRunner(runtime, registry), registry: registry, tests: testrun.NewRunner(runtime),
		onEvent: onEvent, testRuns: make(map[string]context.CancelFunc),
	}
}

// RegisterRunConfigurationType adds a custom run configuration type before
// the dispatcher is closed.
func (d *RunDispatcher) RegisterRunConfigurationType(configType runconf.ConfigType) error {
	if d == nil {
		return errors.New("jided: run dispatcher is nil")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("jided: run dispatcher is closed")
	}
	if d.registry == nil {
		return errors.New("jided: run dispatcher has no configuration registry")
	}
	return d.registry.Register(configType)
}

// Handle executes one run or test-run method.
func (d *RunDispatcher) Handle(ctx context.Context, method jide.Method, rawParams json.RawMessage) (any, error) {
	if d == nil {
		return nil, errors.New("jided: run dispatcher is nil")
	}
	if ctx == nil {
		return nil, errors.New("jided: run request context is nil")
	}
	switch method {
	case jide.MethodRunExecute:
		var params jide.RunExecuteParams
		if err := decodeRunParams(rawParams, &params); err != nil {
			return nil, err
		}
		configuration := runconf.Configuration{
			ID: params.Configuration.ID, Name: params.Configuration.Name, Type: params.Configuration.Type,
			ProjectPath: params.Configuration.ProjectPath, WorkingDirectory: params.Configuration.WorkingDirectory,
			Program: params.Configuration.Program, Module: params.Configuration.Module, Arguments: params.Configuration.Arguments,
			Environment: params.Configuration.Environment, Properties: params.Configuration.Properties,
		}
		process, err := d.runner.Start(context.Background(), configuration, func(event runconf.Event) {
			if event.Stream == runconf.StreamExit {
				d.publish(jide.MethodRunExited, jide.RunExitedParams{RunID: event.RunID, ExitCode: event.ExitCode, Error: event.Error})
				return
			}
			d.publish(jide.MethodRunOutput, jide.RunOutputParams{RunID: event.RunID, Stream: string(event.Stream), Data: event.Data})
		})
		if err != nil {
			return nil, err
		}
		return jide.RunExecuteResult{RunID: process.RunID, PID: process.PID}, nil
	case jide.MethodRunTerminate:
		var params jide.RunTerminateParams
		if err := decodeRunParams(rawParams, &params); err != nil {
			return nil, err
		}
		if strings.TrimSpace(params.RunID) == "" {
			return nil, errors.New("jided: decode run parameters: runId is required")
		}
		if err := d.runner.Terminate(ctx, params.RunID); err != nil {
			return nil, err
		}
		return jide.RunTerminateResult{Terminated: true}, nil
	case jide.MethodRunInput:
		var params jide.RunInputParams
		if err := decodeRunParams(rawParams, &params); err != nil {
			return nil, err
		}
		if strings.TrimSpace(params.RunID) == "" {
			return nil, errors.New("jided: decode run parameters: runId is required")
		}
		process, exists := d.runner.Process(params.RunID)
		if !exists {
			return nil, fmt.Errorf("jided: run %q was not found", params.RunID)
		}
		if err := process.WriteInput([]byte(params.Input)); err != nil {
			return nil, err
		}
		return jide.RunInputResult{Written: true}, nil
	case jide.MethodTestRunExecute:
		var params jide.TestRunExecuteParams
		if err := decodeRunParams(rawParams, &params); err != nil {
			return nil, err
		}
		runID, err := newProtocolRunID()
		if err != nil {
			return nil, fmt.Errorf("jided: create test run id: %w", err)
		}
		runCtx, cancel := context.WithCancel(context.Background())
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			cancel()
			return nil, errors.New("jided: run dispatcher is closed")
		}
		d.testRuns[runID] = cancel
		d.workers.Add(1)
		d.mu.Unlock()
		go func() {
			defer d.workers.Done()
			defer cancel()
			_, runErr := d.tests.Run(runCtx, testrun.Options{
				RunID: runID, ProjectPath: params.ProjectPath, WorkingDirectory: params.WorkingDirectory,
				TestFile: params.TestFile, Arguments: params.Arguments,
			}, func(event testrun.Event) {
				d.publish(jide.MethodTestRunEvent, protocolTestRunEvent(event))
			})
			if runErr != nil && !errors.Is(runErr, testrun.ErrTestsFailed) {
				d.publish(jide.MethodTestRunEvent, jide.TestRunEventParams{RunID: runID, Kind: string(testrun.EventFinished), ExitCode: 1, Message: runErr.Error()})
			}
			d.mu.Lock()
			delete(d.testRuns, runID)
			d.mu.Unlock()
		}()
		return jide.TestRunExecuteResult{RunID: runID}, nil
	default:
		return nil, fmt.Errorf("jided: unsupported run method %q", method)
	}
}

// Close terminates running scripts, cancels test runs, and waits for workers.
func (d *RunDispatcher) Close(ctx context.Context) error {
	if d == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("jided: run dispatcher close context is nil")
	}
	d.mu.Lock()
	d.closed = true
	cancellations := make([]context.CancelFunc, 0, len(d.testRuns))
	for _, cancel := range d.testRuns {
		cancellations = append(cancellations, cancel)
	}
	d.mu.Unlock()
	for _, cancel := range cancellations {
		cancel()
	}
	err := d.runner.Close(ctx)
	done := make(chan struct{})
	go func() {
		d.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		return err
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
}

func (d *RunDispatcher) publish(method jide.Method, params any) {
	d.mu.Lock()
	active := !d.closed
	handler := d.onEvent
	d.mu.Unlock()
	if active && handler != nil {
		handler(method, params)
	}
}

func decodeRunParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("jided: decode run parameters: %w", err)
	}
	return nil
}

func protocolTestRunEvent(event testrun.Event) jide.TestRunEventParams {
	return jide.TestRunEventParams{
		RunID: event.RunID, Kind: string(event.Kind), Name: event.Name, Status: string(event.Status), Message: event.Message,
		SourcePath: event.SourcePath, Line: event.Line, Passed: event.Passed, Failed: event.Failed,
		Errors: event.Errors, Skipped: event.Skipped, Broken: event.Broken, ExitCode: event.ExitCode,
		Stream: event.Stream, Data: event.Data,
	}
}

func newProtocolRunID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
