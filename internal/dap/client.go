package dap

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// TimeoutPolicy bounds the DAP operations that can stall while the adapter
// starts Julia or evaluates code. Zero fields use their documented defaults.
type TimeoutPolicy struct {
	Initialize             time.Duration
	Launch                 time.Duration
	Attach                 time.Duration
	Evaluate               time.Duration
	BreakpointVerification time.Duration
	Request                time.Duration
}

// DefaultTimeoutPolicy follows the initial PyCharm-measured policy from WP8.
var DefaultTimeoutPolicy = TimeoutPolicy{
	Initialize:             10 * time.Second,
	Launch:                 10 * time.Second,
	Attach:                 20 * time.Second,
	Evaluate:               30 * time.Second,
	BreakpointVerification: 10 * time.Second,
	Request:                10 * time.Second,
}

// Options configures one DAP adapter connection.
type Options struct {
	Command  AdapterCommand
	Timeouts TimeoutPolicy
	Stderr   io.Writer
}

// Event is a DAP event delivered by the adapter.
type Event struct {
	Name string
	Body json.RawMessage
}

type dapRequest struct {
	Seq       int    `json:"seq"`
	Type      string `json:"type"`
	Command   string `json:"command"`
	Arguments any    `json:"arguments,omitempty"`
}

type dapResponse struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"`
	RequestSeq int             `json:"request_seq"`
	Success    bool            `json:"success"`
	Command    string          `json:"command"`
	Message    string          `json:"message,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
}

type dapEvent struct {
	Seq   int             `json:"seq"`
	Type  string          `json:"type"`
	Event string          `json:"event"`
	Body  json.RawMessage `json:"body,omitempty"`
}

type dapClientRequest struct {
	Seq       int             `json:"seq"`
	Type      string          `json:"type"`
	Command   string          `json:"command"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type dapEnvelope struct {
	Type       string          `json:"type"`
	RequestSeq int             `json:"request_seq"`
	Success    bool            `json:"success"`
	Command    string          `json:"command"`
	Message    string          `json:"message,omitempty"`
	Event      string          `json:"event"`
	Body       json.RawMessage `json:"body,omitempty"`
}

// AdapterError reports a negative DAP response.
type AdapterError struct {
	Command string
	Message string
}

func (e *AdapterError) Error() string {
	return fmt.Sprintf("dap: adapter rejected %s: %s", e.Command, e.Message)
}

// Client owns a single adapter process and multiplexes DAP requests and
// events. It must be closed when the debug session ends.
type Client struct {
	command      *exec.Cmd
	stdin        io.WriteCloser
	stdoutPipe   io.ReadCloser
	stdout       *bufio.Reader
	listener     net.Listener
	clientConn   net.Conn
	policy       TimeoutPolicy
	capabilities Capabilities

	writeMu sync.Mutex
	mu      sync.Mutex
	seq     int
	pending map[int]chan dapResponse
	closed  bool
	done    chan struct{}
	waitErr error

	eventInput        chan Event
	eventOutput       chan Event
	initializedEvents chan struct{}
	closeOnce         sync.Once
}

// PendingRequest is a DAP request whose response arrives asynchronously. It is
// used for launch/attach, where the adapter waits for configurationDone before
// replying while the client applies breakpoints.
type PendingRequest struct {
	client   *Client
	name     string
	sequence int
	response <-chan dapResponse
	ctx      context.Context
	cancel   context.CancelFunc
}

// Wait waits for a pending adapter response or the request timeout.
func (p *PendingRequest) Wait() (json.RawMessage, error) {
	defer p.cancel()
	select {
	case response := <-p.response:
		if response.Type == "" {
			if response.Message != "" {
				return nil, errors.New(response.Message)
			}
			return nil, errors.New("dap: adapter connection closed")
		}
		if !response.Success {
			return nil, &AdapterError{Command: p.name, Message: response.Message}
		}
		return response.Body, nil
	case <-p.ctx.Done():
		p.client.mu.Lock()
		_, pending := p.client.pending[p.sequence]
		delete(p.client.pending, p.sequence)
		p.client.mu.Unlock()
		if !pending {
			select {
			case response := <-p.response:
				if response.Type == "" {
					if response.Message != "" {
						return nil, errors.New(response.Message)
					}
					return nil, errors.New("dap: adapter connection closed")
				}
				if !response.Success {
					return nil, &AdapterError{Command: p.name, Message: response.Message}
				}
				return response.Body, nil
			default:
			}
		}
		return nil, fmt.Errorf("dap: %s request: %w", p.name, p.ctx.Err())
	}
}

// Cancel abandons the local wait and removes the request from response
// routing. DebugAdapter.jl does not currently advertise request cancellation,
// so the operation may still finish inside the adapter.
func (p *PendingRequest) Cancel() {
	p.cancel()
	p.client.mu.Lock()
	delete(p.client.pending, p.sequence)
	p.client.mu.Unlock()
}

// Start launches the adapter, begins reading DAP messages, and performs the
// initialize handshake. A failed handshake cleans up the child process.
func Start(ctx context.Context, options Options) (*Client, Capabilities, error) {
	if ctx == nil {
		return nil, Capabilities{}, errors.New("dap: context is nil")
	}
	initializeContext, cancelInitialize := context.WithTimeout(ctx, timeoutValue(options.Timeouts.Initialize, DefaultTimeoutPolicy.Initialize))
	defer cancelInitialize()
	if err := options.Command.Validate(); err != nil {
		return nil, Capabilities{}, err
	}
	var listener net.Listener
	if options.Command.TCPConnect {
		var err error
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, Capabilities{}, fmt.Errorf("dap: listen for Julia adapter: %w", err)
		}
		if options.Command.Env == nil {
			options.Command.Env = os.Environ()
		}
		address, ok := listener.Addr().(*net.TCPAddr)
		if !ok {
			_ = listener.Close()
			return nil, Capabilities{}, errors.New("dap: loopback listener has an unexpected address type")
		}
		options.Command.Env = slices.DeleteFunc(slices.Clone(options.Command.Env), func(entry string) bool {
			name, _, found := strings.Cut(entry, "=")
			return found && strings.EqualFold(name, "JIDE_DAP_PORT")
		})
		options.Command.Env = append(options.Command.Env, "JIDE_DAP_PORT="+strconv.Itoa(address.Port))
	}
	if listener != nil {
		defer listener.Close()
	}
	if !filepath.IsAbs(options.Command.Executable) {
		resolved, err := exec.LookPath(options.Command.Executable)
		if err != nil {
			return nil, Capabilities{}, fmt.Errorf("dap: locate adapter executable %q: %w", options.Command.Executable, err)
		}
		options.Command.Executable = resolved
	}
	if options.Command.Env != nil && !hasEnvironmentKey(options.Command.Env, "PATH") {
		options.Command.Env = append(slices.Clone(options.Command.Env), "PATH="+os.Getenv("PATH"))
	}
	if options.Command.Env != nil && !hasEnvironmentKey(options.Command.Env, "SYSTEMROOT") && runtime.GOOS == "windows" {
		options.Command.Env = append(options.Command.Env, "SYSTEMROOT="+os.Getenv("SYSTEMROOT"))
	}
	if err := initializeContext.Err(); err != nil {
		return nil, Capabilities{}, err
	}
	command := exec.Command(options.Command.Executable, options.Command.Args...)
	command.Dir = options.Command.Directory
	command.Env = options.Command.Env
	if options.Command.TCPConnect {
		command.Env = options.Command.Env
		if !hasEnvironmentKey(command.Env, "JIDE_DAP_PORT") {
			return nil, Capabilities{}, errors.New("dap: Julia adapter connection port was not configured")
		}
	}
	if options.Stderr != nil {
		command.Stderr = options.Stderr
	} else {
		command.Stderr = io.Discard
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, Capabilities{}, fmt.Errorf("dap: create adapter stdin: %w", err)
	}
	var stdout io.ReadCloser
	if options.Command.TCPConnect {
		command.Stdout = io.Discard
	} else {
		stdout, err = command.StdoutPipe()
		if err != nil {
			_ = stdin.Close()
			if listener != nil {
				_ = listener.Close()
			}
			return nil, Capabilities{}, fmt.Errorf("dap: create adapter stdout: %w", err)
		}
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		if listener != nil {
			_ = listener.Close()
		}
		return nil, Capabilities{}, fmt.Errorf("dap: start adapter: %w", err)
	}
	var clientConnection net.Conn
	if listener != nil {
		connection, acceptErr := acceptDAPConnection(initializeContext, listener)
		if acceptErr != nil {
			_ = stdin.Close()
			_ = command.Process.Kill()
			_ = command.Wait()
			return nil, Capabilities{}, fmt.Errorf("dap: accept Julia adapter connection: %w", acceptErr)
		}
		clientConnection = connection
		_ = stdin.Close()
		stdin = connection
		stdout = connection
	}
	client := &Client{
		command: command, stdin: stdin, stdoutPipe: stdout, stdout: bufio.NewReader(stdout),
		listener: listener, clientConn: clientConnection,
		policy: normalizeTimeouts(options.Timeouts), pending: make(map[int]chan dapResponse),
		done: make(chan struct{}), eventInput: make(chan Event, 32), eventOutput: make(chan Event), initializedEvents: make(chan struct{}, 8),
	}
	go client.readLoop()
	go client.eventLoop()
	initialize, err := client.request(initializeContext, "initialize", map[string]any{
		"clientID": "jide", "clientName": "jide", "adapterID": "julia", "locale": "en-US",
		"linesStartAt1": true, "columnsStartAt1": true, "pathFormat": "path",
		"supportsVariableType": true, "supportsVariablePaging": true,
		"supportsRunInTerminalRequest": false,
	}, client.policy.Initialize)
	if err != nil {
		_ = client.Close()
		return nil, Capabilities{}, fmt.Errorf("dap: initialize adapter: %w", err)
	}
	var capabilities Capabilities
	if err := json.Unmarshal(initialize, &capabilities); err != nil {
		_ = client.Close()
		return nil, Capabilities{}, fmt.Errorf("dap: decode initialize capabilities: %w", err)
	}
	client.capabilities = capabilities
	return client, capabilities, nil
}

// Events returns the ordered stream of adapter events.
func (c *Client) Events() <-chan Event { return c.eventOutput }

// Launch starts a debuggee and completes the initial DAP configuration with
// empty breakpoint sets before waiting for the adapter response.
func (c *Client) Launch(ctx context.Context, arguments map[string]any) (json.RawMessage, error) {
	return c.LaunchAndConfigure(ctx, arguments, nil, nil, nil)
}

// BeginLaunch sends launch without blocking for the response. The caller must
// wait for initialized, apply breakpoints, send configurationDone, then Wait.
func (c *Client) BeginLaunch(ctx context.Context, arguments map[string]any) (*PendingRequest, error) {
	return c.beginRequest(ctx, "launch", arguments, c.policy.Launch)
}

// BeginAttach sends attach without blocking for the response. The caller must
// complete the same initialized/configuration handshake as for launch.
func (c *Client) BeginAttach(ctx context.Context, arguments map[string]any) (*PendingRequest, error) {
	return c.beginRequest(ctx, "attach", arguments, c.policy.Attach)
}

// LaunchJuliaFile builds DebugAdapter.jl launch arguments and completes the
// standard initialized/configurationDone handshake.
func (c *Client) LaunchJuliaFile(ctx context.Context, program, project, workingDirectory string, args []string, stopOnEntry bool) error {
	pending, err := c.BeginLaunchJuliaFile(ctx, program, project, workingDirectory, args, stopOnEntry, nil)
	if err != nil {
		return err
	}
	_, err = pending.Wait()
	return err
}

// BeginLaunchJuliaFile starts a Julia program and completes the initial DAP
// configuration handshake without waiting for the debuggee to finish. The
// returned request can be waited on in the background while the caller
// continues to service debugger commands and events.
func (c *Client) BeginLaunchJuliaFile(ctx context.Context, program, project, workingDirectory string, args []string, stopOnEntry bool, breakpoints []SourceBreakpoint) (*PendingRequest, error) {
	if strings.TrimSpace(program) == "" {
		return nil, errors.New("dap: Julia program path is empty")
	}
	if strings.TrimSpace(workingDirectory) == "" {
		workingDirectory = filepath.Dir(program)
	}
	absolute, err := filepath.Abs(program)
	if err != nil {
		return nil, fmt.Errorf("dap: resolve Julia program: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("dap: Julia program: %w", err)
	}
	if !info.Mode().IsRegular() || strings.ToLower(filepath.Ext(absolute)) != ".jl" {
		return nil, fmt.Errorf("dap: Julia program %q must be a .jl file", absolute)
	}
	arguments := map[string]any{"program": absolute, "stopOnEntry": stopOnEntry}
	if project != "" {
		absoluteProject, err := filepath.Abs(project)
		if err != nil {
			return nil, fmt.Errorf("dap: resolve Julia project: %w", err)
		}
		if filepath.Base(absoluteProject) == "Project.toml" {
			info, statErr := os.Stat(absoluteProject)
			if statErr != nil || !info.Mode().IsRegular() {
				return nil, fmt.Errorf("dap: Julia project %q is not a regular Project.toml", absoluteProject)
			}
			absoluteProject = filepath.Dir(absoluteProject)
		}
		if info, statErr := os.Stat(filepath.Join(absoluteProject, "Project.toml")); statErr != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("dap: Julia project directory %q has no Project.toml", absoluteProject)
		}
		arguments["project"] = absoluteProject
	}
	if workingDirectory != "" {
		absoluteDirectory, err := filepath.Abs(workingDirectory)
		if err != nil {
			return nil, fmt.Errorf("dap: resolve Julia working directory: %w", err)
		}
		info, statErr := os.Stat(absoluteDirectory)
		if statErr != nil || !info.IsDir() {
			return nil, fmt.Errorf("dap: Julia working directory %q is not a directory", absoluteDirectory)
		}
		arguments["cwd"] = absoluteDirectory
	}
	if args != nil {
		arguments["args"] = args
	}
	pending, err := c.BeginLaunch(ctx, arguments)
	if err != nil {
		return nil, err
	}
	if err := c.ConfigureAndStart(ctx, map[Source][]SourceBreakpoint{{Path: absolute}: breakpoints}, nil, nil, c.policy.Launch); err != nil {
		pending.Cancel()
		_ = c.Close()
		return nil, err
	}
	return pending, nil
}

// AttachJuliaCode attaches DebugAdapter.jl and requests it to debug code in
// Main. An empty filename receives the adapter's documented synthetic name.
func (c *Client) AttachJuliaCode(ctx context.Context, code, filename string, stopOnEntry bool) error {
	if strings.TrimSpace(code) == "" {
		return errors.New("dap: Julia code to debug is empty")
	}
	arguments := map[string]any{"code": code, "stopOnEntry": stopOnEntry}
	sourcePath := ""
	if filename != "" {
		if !filepath.IsAbs(filename) {
			return fmt.Errorf("dap: Julia source filename %q must be absolute", filename)
		}
		absolute, err := filepath.Abs(filename)
		if err != nil {
			return fmt.Errorf("dap: resolve Julia source filename: %w", err)
		}
		arguments["program"] = absolute
		sourcePath = absolute
	}
	source := Source{}
	source.Path = sourcePath
	_, err := c.AttachAndConfigure(ctx, arguments, map[Source][]SourceBreakpoint{source: nil}, nil, nil)
	return err
}

// LaunchAndConfigure sends launch, waits for the adapter's initialized event,
// installs all requested breakpoints, then sends configurationDone.
func (c *Client) LaunchAndConfigure(ctx context.Context, arguments map[string]any, sources map[Source][]SourceBreakpoint, functions []FunctionBreakpoint, exceptionFilters []string) (json.RawMessage, error) {
	pending, err := c.BeginLaunch(ctx, arguments)
	if err != nil {
		return nil, err
	}
	if err := c.ConfigureAndStart(ctx, sources, functions, exceptionFilters, c.policy.Launch); err != nil {
		pending.Cancel()
		_ = c.Close()
		return nil, err
	}
	return pending.Wait()
}

// AttachAndConfigure sends attach, waits for initialized, applies
// breakpoints, and acknowledges DebugAdapter.jl's configuration phase.
func (c *Client) AttachAndConfigure(ctx context.Context, arguments map[string]any, sources map[Source][]SourceBreakpoint, functions []FunctionBreakpoint, exceptionFilters []string) (json.RawMessage, error) {
	pending, err := c.BeginAttach(ctx, arguments)
	if err != nil {
		return nil, err
	}
	if err := c.ConfigureAndStart(ctx, sources, functions, exceptionFilters, c.policy.Attach); err != nil {
		pending.Cancel()
		_ = c.Close()
		return nil, err
	}
	return pending.Wait()
}

// StartJuliaDebugSession launches DebugAdapter.jl over a loopback connection and returns its
// negotiated capabilities. The package must be installed in projectPath.
func (d *Discoverer) StartJuliaDebugSession(ctx context.Context, projectPath, workingDirectory string, timeouts TimeoutPolicy, stderr io.Writer) (*Client, Capabilities, error) {
	command, err := d.Discover(ctx, projectPath, workingDirectory)
	if err != nil {
		return nil, Capabilities{}, err
	}
	return Start(ctx, Options{Command: command, Timeouts: timeouts, Stderr: stderr})
}

// Attach attaches to a debuggee and completes the initial DAP configuration
// with empty breakpoint sets before waiting for the adapter response.
func (c *Client) Attach(ctx context.Context, arguments map[string]any) (json.RawMessage, error) {
	return c.AttachAndConfigure(ctx, arguments, nil, nil, nil)
}

// WaitInitialized waits for DebugAdapter.jl's initialized event after launch
// or attach. Configure breakpoints first, then call ConfigurationDone.
func (c *Client) WaitInitialized(ctx context.Context, timeout time.Duration) error {
	return c.waitEvent(ctx, "initialized", timeout)
}

// ConfigureAndStart applies breakpoints after launch/attach emits initialized,
// then sends configurationDone so DebugAdapter.jl can begin running code.
func (c *Client) ConfigureAndStart(ctx context.Context, sources map[Source][]SourceBreakpoint, functions []FunctionBreakpoint, exceptionFilters []string, timeout time.Duration) error {
	if err := c.WaitInitialized(ctx, timeout); err != nil {
		return err
	}
	orderedSources := make([]Source, 0, len(sources))
	for source := range sources {
		orderedSources = append(orderedSources, source)
	}
	slices.SortFunc(orderedSources, func(a, b Source) int {
		if value := strings.Compare(a.Path, b.Path); value != 0 {
			return value
		}
		return strings.Compare(a.Name, b.Name)
	})
	for _, source := range orderedSources {
		if _, err := c.SetBreakpoints(ctx, source, sources[source]); err != nil {
			return err
		}
	}
	if functions != nil {
		if len(functions) > 0 && !c.capabilities.SupportsFunctionBreakpoints {
			return errors.New("dap: adapter does not support function breakpoints")
		}
		if c.capabilities.SupportsFunctionBreakpoints {
			if _, err := c.SetFunctionBreakpoints(ctx, functions); err != nil {
				return err
			}
		}
	}
	if exceptionFilters != nil {
		if err := c.SetExceptionBreakpointsNamed(ctx, exceptionFilters, c.capabilities); err != nil {
			return err
		}
	}
	return c.ConfigurationDone(ctx)
}

// ConfigurationDone tells the adapter that breakpoints and launch settings
// have been supplied.
func (c *Client) ConfigurationDone(ctx context.Context) error {
	_, err := c.request(ctx, "configurationDone", struct{}{}, c.policy.Request)
	return err
}

// SetExceptionBreakpoints configures DebugAdapter.jl's "error" and "throw"
// filters. Empty filters disables exception breakpoints.
func (c *Client) SetExceptionBreakpoints(ctx context.Context, filters []string) error {
	_, err := c.SetExceptionBreakpointsResult(ctx, filters)
	return err
}

// SetExceptionBreakpointsResult selects exception filters and returns the
// adapter's updated breakpoint records, when it provides them.
func (c *Client) SetExceptionBreakpointsResult(ctx context.Context, filters []string) ([]Breakpoint, error) {
	if filters == nil {
		filters = []string{}
	}
	var result struct {
		Breakpoints []Breakpoint `json:"breakpoints"`
	}
	_, err := c.requestInto(ctx, "setExceptionBreakpoints", map[string]any{"filters": filters}, &result, c.policy.BreakpointVerification)
	return result.Breakpoints, err
}

// SetExceptionBreakpointsNamed configures known adapter exception filters and
// rejects names the adapter did not advertise.
func (c *Client) SetExceptionBreakpointsNamed(ctx context.Context, filters []string, capabilities Capabilities) error {
	_, err := c.SetExceptionBreakpointsNamedResult(ctx, filters, capabilities)
	return err
}

// SetExceptionBreakpointsNamedResult validates advertised filters, applies
// them, and returns the adapter's updated breakpoint records.
func (c *Client) SetExceptionBreakpointsNamedResult(ctx context.Context, filters []string, capabilities Capabilities) ([]Breakpoint, error) {
	for _, filter := range filters {
		known := false
		for _, advertised := range capabilities.ExceptionBreakpointFilters {
			if advertised.Filter == filter {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("dap: unsupported Julia exception breakpoint filter %q", filter)
		}
	}
	return c.SetExceptionBreakpointsResult(ctx, filters)
}

// SetJuliaExceptionBreakpoints applies all exception filters advertised by
// DebugAdapter.jl, or a caller-supplied subset.
func (c *Client) SetJuliaExceptionBreakpoints(ctx context.Context, filters []string, capabilities Capabilities) error {
	if filters == nil {
		filters = make([]string, 0, len(capabilities.ExceptionBreakpointFilters))
		for _, filter := range capabilities.ExceptionBreakpointFilters {
			filters = append(filters, filter.Filter)
		}
	}
	return c.SetExceptionBreakpointsNamed(ctx, filters, capabilities)
}

// SetBreakpoints replaces line breakpoints in a source file.
func (c *Client) SetBreakpoints(ctx context.Context, source Source, breakpoints []SourceBreakpoint) ([]Breakpoint, error) {
	var result struct {
		Breakpoints []Breakpoint `json:"breakpoints"`
	}
	if breakpoints == nil {
		breakpoints = []SourceBreakpoint{}
	}
	if source.Path == "" && source.SourceReference == 0 {
		return nil, errors.New("dap: source breakpoint requires a path or source reference")
	}
	for _, breakpoint := range breakpoints {
		if breakpoint.Line < 1 || breakpoint.Column < 0 {
			return nil, errors.New("dap: source breakpoint line must be positive and column cannot be negative")
		}
	}
	_, err := c.requestInto(ctx, "setBreakpoints", SetBreakpointsArguments{Source: source, Breakpoints: breakpoints, SourceModified: false}, &result, c.policy.BreakpointVerification)
	return result.Breakpoints, err
}

// SetFunctionBreakpoints replaces function breakpoints.
func (c *Client) SetFunctionBreakpoints(ctx context.Context, breakpoints []FunctionBreakpoint) ([]Breakpoint, error) {
	var result struct {
		Breakpoints []Breakpoint `json:"breakpoints"`
	}
	if breakpoints == nil {
		breakpoints = []FunctionBreakpoint{}
	}
	if len(breakpoints) > 0 && !c.capabilities.SupportsFunctionBreakpoints {
		return nil, errors.New("dap: adapter does not support function breakpoints")
	}
	for _, breakpoint := range breakpoints {
		if strings.TrimSpace(breakpoint.Name) == "" {
			return nil, errors.New("dap: function breakpoint name is empty")
		}
	}
	_, err := c.requestInto(ctx, "setFunctionBreakpoints", map[string]any{"breakpoints": breakpoints}, &result, c.policy.BreakpointVerification)
	return result.Breakpoints, err
}

// SetFunctionBreakpointsIfSupported returns a clear error if the adapter did
// not advertise function-breakpoint support.
func (c *Client) SetFunctionBreakpointsIfSupported(ctx context.Context, breakpoints []FunctionBreakpoint, capabilities Capabilities) ([]Breakpoint, error) {
	if !capabilities.SupportsFunctionBreakpoints {
		return nil, errors.New("dap: adapter does not support function breakpoints")
	}
	return c.SetFunctionBreakpoints(ctx, breakpoints)
}

// SetFunctionBreakpointsEmpty clears all existing function breakpoints.
func (c *Client) SetFunctionBreakpointsEmpty(ctx context.Context) error {
	_, err := c.SetFunctionBreakpoints(ctx, []FunctionBreakpoint{})
	return err
}

// Continue resumes execution of a thread.
func (c *Client) Continue(ctx context.Context, threadID int) (ContinueResult, error) {
	if threadID < 1 {
		return ContinueResult{}, errors.New("dap: thread id must be positive")
	}
	var result ContinueResult
	_, err := c.requestInto(ctx, "continue", map[string]any{"threadId": threadID}, &result, c.policy.Request)
	return result, err
}

// Next steps over the current source line.
func (c *Client) Next(ctx context.Context, threadID int) error {
	if threadID < 1 {
		return errors.New("dap: thread id must be positive")
	}
	return c.simpleRequest(ctx, "next", map[string]any{"threadId": threadID})
}

// StepInTarget is a Julia function call site available as a step-into target.
type StepInTarget struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

// StepInTargets asks the adapter for step-in targets associated with a stack
// frame.
func (c *Client) StepInTargets(ctx context.Context, frameID int) ([]StepInTarget, error) {
	if frameID < 1 {
		return nil, errors.New("dap: frame id must be positive")
	}
	if !c.capabilities.SupportsStepInTargetsRequest {
		return nil, errors.New("dap: adapter does not support step-in targets")
	}
	var result struct {
		Targets []StepInTarget `json:"targets"`
	}
	_, err := c.requestInto(ctx, "stepInTargets", map[string]any{"frameId": frameID}, &result, c.policy.Request)
	return result.Targets, err
}

// StepIn enters the current function call.
func (c *Client) StepIn(ctx context.Context, threadID int) error {
	if threadID < 1 {
		return errors.New("dap: thread id must be positive")
	}
	return c.simpleRequest(ctx, "stepIn", map[string]any{"threadId": threadID})
}

// StepInAtTarget steps into a selected call site on a thread.
func (c *Client) StepInAtTarget(ctx context.Context, threadID, targetID int) error {
	if threadID < 1 || targetID < 0 {
		return errors.New("dap: thread id must be positive and step-in target id cannot be negative")
	}
	return c.simpleRequest(ctx, "stepIn", map[string]any{"threadId": threadID, "targetId": targetID})
}

// StepOut exits the current function.
func (c *Client) StepOut(ctx context.Context, threadID int) error {
	if threadID < 1 {
		return errors.New("dap: thread id must be positive")
	}
	return c.simpleRequest(ctx, "stepOut", map[string]any{"threadId": threadID})
}

// Pause interrupts a running thread.
func (c *Client) Pause(ctx context.Context, threadID int) error {
	if threadID < 1 {
		return errors.New("dap: thread id must be positive")
	}
	return c.simpleRequest(ctx, "pause", map[string]any{"threadId": threadID})
}

// StackTrace returns stack frames and pagination metadata for a thread.
func (c *Client) StackTrace(ctx context.Context, threadID, startFrame, levels int) (StackTraceResult, error) {
	if threadID < 1 || startFrame < 0 || levels < 0 {
		return StackTraceResult{}, errors.New("dap: thread id must be positive and stack pagination values cannot be negative")
	}
	var result StackTraceResult
	_, err := c.requestInto(ctx, "stackTrace", map[string]any{"threadId": threadID, "startFrame": startFrame, "levels": levels}, &result, c.policy.Request)
	return result, err
}

// Threads lists the threads currently known to the adapter.
func (c *Client) Threads(ctx context.Context) ([]Thread, error) {
	var result struct {
		Threads []Thread `json:"threads"`
	}
	_, err := c.requestInto(ctx, "threads", struct{}{}, &result, c.policy.Request)
	return result.Threads, err
}

// Scopes returns the variable scopes for a stack frame.
func (c *Client) Scopes(ctx context.Context, frameID int) ([]Scope, error) {
	if frameID < 1 {
		return nil, errors.New("dap: frame id must be positive")
	}
	var result struct {
		Scopes []Scope `json:"scopes"`
	}
	_, err := c.requestInto(ctx, "scopes", map[string]any{"frameId": frameID}, &result, c.policy.Request)
	return result.Scopes, err
}

// ScopesIfSupported requests stack-frame scopes after validating frame ids.
func (c *Client) ScopesIfSupported(ctx context.Context, frameID int) ([]Scope, error) {
	if frameID < 1 {
		return nil, errors.New("dap: frame id must be positive")
	}
	return c.Scopes(ctx, frameID)
}

// Variables returns children for a variable reference.
func (c *Client) Variables(ctx context.Context, reference, start, count int) ([]Variable, error) {
	if reference < 0 || start < 0 || count < 0 {
		return nil, errors.New("dap: variable reference and pagination values cannot be negative")
	}
	var result struct {
		Variables []Variable `json:"variables"`
	}
	_, err := c.requestInto(ctx, "variables", map[string]any{"variablesReference": reference, "start": start, "count": count}, &result, c.policy.Request)
	return result.Variables, err
}

// Evaluate evaluates an expression in a frame or the REPL context.
func (c *Client) Evaluate(ctx context.Context, expression string, frameID int, contextName string) (EvaluateResult, error) {
	if strings.TrimSpace(expression) == "" || frameID < 0 {
		return EvaluateResult{}, errors.New("dap: expression must not be empty and frame id cannot be negative")
	}
	var result EvaluateResult
	arguments := map[string]any{"expression": expression}
	if contextName != "" {
		arguments["context"] = contextName
	}
	if frameID > 0 {
		arguments["frameId"] = frameID
	}
	_, err := c.requestInto(ctx, "evaluate", arguments, &result, c.policy.Evaluate)
	return result, err
}

// EvaluateIfSupported checks the advertised hover/watch evaluation capability
// before issuing evaluate in the selected context.
func (c *Client) EvaluateIfSupported(ctx context.Context, expression string, frameID int, contextName string, capabilities Capabilities) (EvaluateResult, error) {
	if !capabilities.SupportsEvaluateForHovers && (contextName == "hover" || contextName == "watch") {
		return EvaluateResult{}, errors.New("dap: adapter does not support hover/watch evaluation")
	}
	return c.Evaluate(ctx, expression, frameID, contextName)
}

// Disconnect asks the adapter to shut down the debug session.
func (c *Client) Disconnect(ctx context.Context, terminate bool) error {
	_, err := c.request(ctx, "disconnect", map[string]any{"terminateDebuggee": terminate}, c.policy.Request)
	if err == nil {
		_ = c.stdin.Close()
		if c.clientConn != nil {
			_ = c.clientConn.Close()
		}
	}
	return err
}

// Terminate asks the adapter to stop the debuggee without disconnecting.
func (c *Client) Terminate(ctx context.Context) error {
	if !c.capabilities.SupportsTerminateRequest {
		return errors.New("dap: adapter does not support terminate")
	}
	return c.simpleRequest(ctx, "terminate", struct{}{})
}

// ExceptionInfo returns structured information about the last exception.
func (c *Client) ExceptionInfo(ctx context.Context, threadID int) (ExceptionInfoResult, error) {
	if threadID < 1 {
		return ExceptionInfoResult{}, errors.New("dap: thread id must be positive")
	}
	if !c.capabilities.SupportsExceptionInfoRequest {
		return ExceptionInfoResult{}, errors.New("dap: adapter does not support exception information")
	}
	var result ExceptionInfoResult
	_, err := c.requestInto(ctx, "exceptionInfo", map[string]any{"threadId": threadID}, &result, c.policy.Request)
	return result, err
}

// SetVariable changes a named variable in a scope or expandable value.
func (c *Client) SetVariable(ctx context.Context, variablesReference int, name, value string, capabilities Capabilities) (SetVariableResult, error) {
	if !capabilities.SupportsSetVariable {
		return SetVariableResult{}, errors.New("dap: adapter does not support setVariable")
	}
	if variablesReference < 0 || strings.TrimSpace(name) == "" {
		return SetVariableResult{}, errors.New("dap: variables reference must be non-negative and name must not be empty")
	}
	var result SetVariableResult
	_, err := c.requestInto(ctx, "setVariable", map[string]any{
		"variablesReference": variablesReference,
		"name":               name,
		"value":              value,
	}, &result, c.policy.Request)
	return result, err
}

// Close kills the adapter process and fails any outstanding requests.
func (c *Client) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		for sequence, response := range c.pending {
			delete(c.pending, sequence)
			select {
			case response <- dapResponse{Message: "dap: client is closed"}:
			default:
			}
		}
		c.mu.Unlock()
		c.writeMu.Lock()
		_ = c.stdin.Close()
		c.writeMu.Unlock()
		select {
		case <-c.done:
			closeErr = nil
		case <-time.After(250 * time.Millisecond):
			if c.command.Process != nil {
				if err := c.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					closeErr = err
				}
			}
			_ = c.stdoutPipe.Close()
			select {
			case <-c.done:
				closeErr = nil
			case <-time.After(2 * time.Second):
				if c.command.Process != nil {
					_ = c.command.Process.Kill()
				}
				if closeErr == nil {
					closeErr = errors.New("dap: timed out waiting for adapter process to exit")
				}
			}
		}
	})
	return closeErr
}

// Wait waits for the adapter process to exit. Close is the cancellation path;
// Wait does not kill a still-running adapter.
func (c *Client) Wait(ctx context.Context) error {
	if ctx == nil {
		return errors.New("dap: wait context is nil")
	}
	select {
	case <-c.done:
		return c.waitErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) simpleRequest(ctx context.Context, name string, arguments any) error {
	_, err := c.request(ctx, name, arguments, c.policy.Request)
	return err
}

func (c *Client) requestInto(ctx context.Context, name string, arguments, target any, timeout time.Duration) (json.RawMessage, error) {
	body, err := c.request(ctx, name, arguments, timeout)
	if err != nil {
		return nil, err
	}
	if target != nil && len(body) > 0 && string(body) != "null" {
		if err := json.Unmarshal(body, target); err != nil {
			return nil, fmt.Errorf("dap: decode %s response: %w", name, err)
		}
	}
	return body, nil
}

func (c *Client) request(ctx context.Context, name string, arguments any, timeout time.Duration) (json.RawMessage, error) {
	pending, err := c.beginRequest(ctx, name, arguments, timeout)
	if err != nil {
		return nil, err
	}
	return pending.Wait()
}

func (c *Client) beginRequest(ctx context.Context, name string, arguments any, timeout time.Duration) (*PendingRequest, error) {
	if ctx == nil {
		return nil, errors.New("dap: request context is nil")
	}
	if timeout <= 0 {
		timeout = c.policy.Request
	}
	if err := validateDAPArguments(arguments); err != nil {
		return nil, fmt.Errorf("dap: invalid %s arguments: %w", name, err)
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	if err := requestContext.Err(); err != nil {
		cancel()
		return nil, err
	}
	c.writeMu.Lock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		c.writeMu.Unlock()
		cancel()
		return nil, errors.New("dap: client is closed")
	}
	c.seq++
	sequence := c.seq
	responseChannel := make(chan dapResponse, 1)
	c.pending[sequence] = responseChannel
	c.mu.Unlock()
	err := writeMessage(c.stdin, dapRequest{Seq: sequence, Type: "request", Command: name, Arguments: arguments})
	if err != nil {
		c.mu.Lock()
		delete(c.pending, sequence)
		c.mu.Unlock()
		c.writeMu.Unlock()
		cancel()
		return nil, fmt.Errorf("dap: write %s request: %w", name, err)
	}
	c.writeMu.Unlock()
	return &PendingRequest{client: c, name: name, sequence: sequence, response: responseChannel, ctx: requestContext, cancel: cancel}, nil
}

func (c *Client) waitEvent(ctx context.Context, name string, timeout time.Duration) error {
	if name != "initialized" {
		return fmt.Errorf("dap: waiting for event %q is not supported by this helper", name)
	}
	if ctx == nil {
		return errors.New("dap: event context is nil")
	}
	waitContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		select {
		case <-c.initializedEvents:
			return nil
		case _, ok := <-c.eventInput:
			if !ok {
				return errors.New("dap: adapter connection closed before initialized event")
			}
		case <-waitContext.Done():
			return waitContext.Err()
		}
	}
}

func validateDAPArguments(value any) error {
	return validateJSONValue(reflect.ValueOf(value), "arguments")
}

func validateJSONValue(value reflect.Value, path string) error {
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		return validateJSONValue(value.Elem(), path)
	}
	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("%s has non-string object keys", path)
		}
		iter := value.MapRange()
		for iter.Next() {
			if err := validateJSONValue(iter.Value(), path+"."+iter.Key().String()); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if err := validateJSONValue(value.Index(index), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(value.Float()) || math.IsInf(value.Float(), 0) {
			return fmt.Errorf("%s contains a non-finite number", path)
		}
	case reflect.String:
		for _, character := range value.String() {
			if character == 0 || (unicode.IsControl(character) && character != '\n' && character != '\r' && character != '\t') {
				return fmt.Errorf("%s contains a control character", path)
			}
		}
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			field := value.Type().Field(index)
			if field.PkgPath != "" || field.Tag.Get("json") == "-" {
				continue
			}
			if err := validateJSONValue(value.Field(index), path+"."+field.Name); err != nil {
				return err
			}
		}
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
	default:
		return fmt.Errorf("%s has unsupported JSON value type %s", path, value.Type())
	}
	return nil
}

func (c *Client) readLoop() {
	defer func() {
		c.waitErr = c.command.Wait()
		if c.waitErr != nil {
			c.failPending(fmt.Errorf("dap: adapter process exited: %w", c.waitErr))
		}
		close(c.done)
	}()
	defer close(c.eventInput)
	for {
		data, err := readMessage(c.stdout)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				c.failPending(fmt.Errorf("dap: read adapter message: %w", err))
			} else {
				c.failPending(errors.New("dap: adapter process exited"))
			}
			return
		}
		var envelope dapEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			c.failPending(fmt.Errorf("dap: decode adapter message: %w", err))
			return
		}
		switch envelope.Type {
		case "response":
			var response dapResponse
			if err := json.Unmarshal(data, &response); err != nil {
				continue
			}
			c.mu.Lock()
			pending := c.pending[response.RequestSeq]
			delete(c.pending, response.RequestSeq)
			c.mu.Unlock()
			if pending != nil {
				pending <- response
			}
		case "event":
			if envelope.Event == "initialized" {
				select {
				case c.initializedEvents <- struct{}{}:
				default:
				}
			}
			c.eventInput <- Event{Name: envelope.Event, Body: envelope.Body}
		case "request":
			var request dapClientRequest
			if err := json.Unmarshal(data, &request); err == nil {
				c.replyUnsupported(request)
			}
		}
	}
}

func (c *Client) replyUnsupported(request dapClientRequest) {
	c.writeMu.Lock()
	sequence := c.nextSequence()
	_ = writeMessage(c.stdin, dapResponse{Seq: sequence, Type: "response", RequestSeq: request.Seq,
		Success: false, Command: request.Command, Message: "jide does not support adapter-initiated requests"})
	c.writeMu.Unlock()
}

func (c *Client) nextSequence() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	return c.seq
}

func (c *Client) failPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for sequence, response := range c.pending {
		delete(c.pending, sequence)
		select {
		case response <- dapResponse{Message: err.Error()}:
		default:
		}
	}
}

func (c *Client) eventLoop() {
	defer close(c.eventOutput)
	queue := make([]Event, 0)
	input := c.eventInput
	for input != nil || len(queue) > 0 {
		var output chan Event
		var first Event
		if len(queue) > 0 {
			output = c.eventOutput
			first = queue[0]
		}
		select {
		case event, ok := <-input:
			if !ok {
				input = nil
			} else {
				queue = append(queue, event)
			}
		case output <- first:
			queue[0] = Event{}
			queue = queue[1:]
		case <-c.done:
			return
		}
	}
}

func normalizeTimeouts(policy TimeoutPolicy) TimeoutPolicy {
	defaults := DefaultTimeoutPolicy
	if policy.Initialize <= 0 {
		policy.Initialize = defaults.Initialize
	}
	if policy.Launch <= 0 {
		policy.Launch = defaults.Launch
	}
	if policy.Attach <= 0 {
		policy.Attach = defaults.Attach
	}
	if policy.Evaluate <= 0 {
		policy.Evaluate = defaults.Evaluate
	}
	if policy.BreakpointVerification <= 0 {
		policy.BreakpointVerification = defaults.BreakpointVerification
	}
	if policy.Request <= 0 {
		policy.Request = defaults.Request
	}
	return policy
}

func timeoutValue(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func hasEnvironmentKey(environment []string, key string) bool {
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(name, key) {
			return true
		}
	}
	return false
}

func acceptDAPConnection(ctx context.Context, listener net.Listener) (net.Conn, error) {
	accepted := make(chan struct {
		connection net.Conn
		err        error
	}, 1)
	go func() {
		connection, err := listener.Accept()
		accepted <- struct {
			connection net.Conn
			err        error
		}{connection, err}
	}()
	select {
	case result := <-accepted:
		return result.connection, result.err
	case <-ctx.Done():
		_ = listener.Close()
		result := <-accepted
		if result.connection != nil {
			_ = result.connection.Close()
		}
		return nil, ctx.Err()
	}
}
