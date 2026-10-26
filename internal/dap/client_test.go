package dap

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
)

const mockAdapterEnv = "JIDE_DAP_MOCK_ADAPTER"
const mockAdapterTCPEnv = "JIDE_DAP_MOCK_TCP"

func TestMain(m *testing.M) {
	if os.Getenv(mockAdapterTCPEnv) == "1" {
		connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", os.Getenv("JIDE_DAP_PORT")), 5*time.Second)
		if err != nil {
			os.Exit(12)
		}
		result := runMockAdapter(connection, connection)
		_ = connection.Close()
		os.Exit(result)
	}
	if os.Getenv(mockAdapterEnv) == "1" {
		os.Exit(runMockAdapter(os.Stdin, os.Stdout))
	}
	os.Exit(m.Run())
}

func runMockAdapter(input io.Reader, output io.Writer) int {
	reader := bufio.NewReader(input)
	breakpoints := make(map[string][]Breakpoint)
	lastSequence := 0
	for {
		data, err := readMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0
			}
			return 2
		}
		var request dapRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return 3
		}
		if request.Seq <= lastSequence {
			return 10
		}
		lastSequence = request.Seq
		if request.Command == "launch" || request.Command == "attach" {
			if err := writeMessage(output, dapEvent{Seq: 501, Type: "event", Event: "initialized"}); err != nil {
				return 4
			}
			pendingRequests := map[int]string{request.Seq: request.Command}
			for len(pendingRequests) > 0 {
				configured, configuredErr := readMessage(reader)
				if configuredErr != nil {
					return 5
				}
				var configuration dapRequest
				if err := json.Unmarshal(configured, &configuration); err != nil {
					return 6
				}
				if configuration.Seq <= lastSequence {
					return 11
				}
				lastSequence = configuration.Seq
				if configuration.Command == "configurationDone" {
					for sequence, command := range pendingRequests {
						body := mockResponse(command, nil, breakpoints)
						if err := writeMessage(output, map[string]any{
							"seq": configuration.Seq + 100, "type": "response", "request_seq": sequence,
							"success": true, "command": command, "body": body,
						}); err != nil {
							return 7
						}
					}
					body := mockResponse(configuration.Command, nil, breakpoints)
					if err := writeMessage(output, map[string]any{
						"seq": configuration.Seq + 101, "type": "response", "request_seq": configuration.Seq,
						"success": true, "command": configuration.Command, "body": body,
					}); err != nil {
						return 8
					}
					break
				}
				arguments, _ := json.Marshal(configuration.Arguments)
				body := mockResponse(configuration.Command, arguments, breakpoints)
				if err := writeMessage(output, map[string]any{
					"seq": configuration.Seq + 100, "type": "response", "request_seq": configuration.Seq,
					"success": true, "command": configuration.Command, "body": body,
				}); err != nil {
					return 9
				}
			}
			continue
		}
		arguments, _ := json.Marshal(request.Arguments)
		body := mockResponse(request.Command, arguments, breakpoints)
		response := map[string]any{
			"seq": request.Seq + 100, "type": "response", "request_seq": request.Seq,
			"success": true, "command": request.Command, "body": body,
		}
		if err := writeMessage(output, response); err != nil {
			return 4
		}
	}
}

func mockResponse(command string, rawArguments json.RawMessage, sourceBreakpoints map[string][]Breakpoint) any {
	var arguments map[string]any
	_ = json.Unmarshal(rawArguments, &arguments)
	switch command {
	case "initialize":
		return map[string]any{
			"supportsConfigurationDoneRequest": true,
			"supportsFunctionBreakpoints":      true,
			"supportsConditionalBreakpoints":   true,
			"supportsSetVariable":              true,
			"supportsRestartFrame":             true,
			"supportsExceptionInfoRequest":     true,
			"supportsTerminateRequest":         true,
			"supportsStepInTargetsRequest":     true,
			"supportsVariablePaging":           true,
			"supportsVariableType":             true,
			"exceptionBreakpointFilters": []any{
				map[string]any{"filter": "error", "label": "Uncaught Exceptions", "default": true},
				map[string]any{"filter": "throw", "label": "All Exceptions"},
			},
		}
	case "setBreakpoints", "setFunctionBreakpoints":
		entries, _ := arguments["breakpoints"].([]any)
		if command == "setFunctionBreakpoints" {
			if len(entries) == 0 {
				return map[string]any{"breakpoints": []any{}}
			}
			return map[string]any{"breakpoints": []any{map[string]any{"id": 8, "verified": true}}}
		}
		var source Source
		if raw, ok := arguments["source"]; ok {
			data, _ := json.Marshal(raw)
			_ = json.Unmarshal(data, &source)
		}
		if len(entries) == 0 {
			delete(sourceBreakpoints, source.Path)
			return map[string]any{"breakpoints": []any{}}
		}
		var requests []SourceBreakpoint
		data, _ := json.Marshal(entries)
		_ = json.Unmarshal(data, &requests)
		verified := make([]Breakpoint, len(requests))
		for index, breakpoint := range requests {
			verified[index] = Breakpoint{ID: index + 1, Verified: true, Line: breakpoint.Line}
		}
		sourceBreakpoints[source.Path] = verified
		return map[string]any{"breakpoints": verified}
	case "continue":
		return map[string]any{"allThreadsContinued": true}
	case "setExceptionBreakpoints":
		return map[string]any{"breakpoints": []any{}}
	case "exceptionInfo":
		return map[string]any{
			"exceptionId": "BoundsError", "description": "bounds error", "breakMode": "unhandled",
			"details": map[string]any{"typeName": "BoundsError", "stackTrace": "at main.jl:4"},
		}
	case "setVariable":
		return map[string]any{"value": "43", "type": "Int64", "variablesReference": 0}
	case "stackTrace":
		return map[string]any{"stackFrames": []any{map[string]any{
			"id": 2, "name": "main", "line": 4, "column": 1,
			"source": map[string]any{"name": "main.jl", "path": "C:/project/main.jl"},
		}}, "totalFrames": 1}
	case "stepInTargets":
		return map[string]any{"targets": []any{map[string]any{"id": 3, "label": "foo()"}}}
	case "threads":
		return map[string]any{"threads": []any{map[string]any{"id": 1, "name": "julia-main"}}}
	case "cancel":
		return map[string]any{}
	case "configurationDone":
		return map[string]any{}
	case "terminate", "disconnect":
		return map[string]any{}
	case "scopes":
		return map[string]any{"scopes": []any{map[string]any{"name": "Locals", "variablesReference": 8, "expensive": false}}}
	case "variables":
		return map[string]any{"variables": []any{map[string]any{"name": "x", "value": "42", "type": "Int64", "variablesReference": 0}}}
	case "evaluate":
		return map[string]any{"result": "42", "type": "Int64", "variablesReference": 0}
	default:
		return map[string]any{}
	}
}

func TestClientLifecycleAndTypedRequests(t *testing.T) {
	client, capabilities := startMock(t)
	if !capabilities.SupportsFunctionBreakpoints || !capabilities.SupportsConfigurationDoneRequest {
		t.Fatalf("initialize capabilities = %+v", capabilities)
	}
	if !capabilities.SupportsSetVariable || !capabilities.SupportsRestartFrame || !capabilities.SupportsExceptionInfoRequest || len(capabilities.ExceptionBreakpointFilters) != 2 {
		t.Fatalf("Julia adapter capabilities were not decoded: %+v", capabilities)
	}
	breakpoints, err := client.SetBreakpoints(context.Background(), Source{Path: "C:/project/main.jl"}, []SourceBreakpoint{{Line: 4, Condition: "x == 42"}})
	if err != nil || len(breakpoints) != 1 || !breakpoints[0].Verified || breakpoints[0].Line != 4 {
		t.Fatalf("SetBreakpoints() = %+v, %v", breakpoints, err)
	}
	functions, err := client.SetFunctionBreakpoints(context.Background(), []FunctionBreakpoint{{Name: "main"}})
	if err != nil || len(functions) != 1 || !functions[0].Verified {
		t.Fatalf("SetFunctionBreakpoints() = %+v, %v", functions, err)
	}
	launchRequest, err := client.BeginLaunch(context.Background(), map[string]any{"program": "C:/project/main.jl"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.WaitInitialized(context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SetBreakpoints(context.Background(), Source{Path: "C:/project/main.jl"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := client.SetFunctionBreakpointsEmpty(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.SetJuliaExceptionBreakpoints(context.Background(), nil, capabilities); err != nil {
		t.Fatal(err)
	}
	if err := client.ConfigurationDone(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-client.Events():
		if event.Name != "initialized" {
			t.Fatalf("first post-launch event = %+v, want initialized", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initialized event")
	}
	if _, err := launchRequest.Wait(); err != nil {
		t.Fatal(err)
	}
	continued, err := client.Continue(context.Background(), 1)
	if err != nil || !continued.AllThreadsContinued {
		t.Fatalf("Continue() = %+v, %v", continued, err)
	}
	for _, step := range []func(context.Context, int) error{client.Next, client.StepIn, client.StepOut, client.Pause} {
		if err := step(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.StepInAtTarget(context.Background(), 1, 3); err != nil {
		t.Fatal(err)
	}
	stack, err := client.StackTrace(context.Background(), 1, 0, 20)
	if err != nil || len(stack.StackFrames) != 1 || stack.StackFrames[0].Name != "main" {
		t.Fatalf("StackTrace() = %+v, %v", stack, err)
	}
	threads, err := client.Threads(context.Background())
	if err != nil || len(threads) != 1 || threads[0].Name != "julia-main" {
		t.Fatalf("Threads() = %+v, %v", threads, err)
	}
	targets, err := client.StepInTargets(context.Background(), 2)
	if err != nil || len(targets) != 1 || targets[0].Label != "foo()" {
		t.Fatalf("StepInTargets() = %+v, %v", targets, err)
	}
	scopes, err := client.Scopes(context.Background(), 2)
	if err != nil || len(scopes) != 1 || scopes[0].Name != "Locals" {
		t.Fatalf("Scopes() = %+v, %v", scopes, err)
	}
	variables, err := client.Variables(context.Background(), 8, 0, 20)
	if err != nil || len(variables) != 1 || variables[0].Value != "42" {
		t.Fatalf("Variables() = %+v, %v", variables, err)
	}
	evaluated, err := client.Evaluate(context.Background(), "x", 2, "watch")
	if err != nil || evaluated.Result != "42" || evaluated.Type != "Int64" {
		t.Fatalf("Evaluate() = %+v, %v", evaluated, err)
	}
	setVariable, err := client.SetVariable(context.Background(), 8, "x", "43", capabilities)
	if err != nil || setVariable.Value != "43" || setVariable.Type != "Int64" {
		t.Fatalf("SetVariable() = %+v, %v", setVariable, err)
	}
	exception, err := client.ExceptionInfo(context.Background(), 1)
	if err != nil || exception.ExceptionID != "BoundsError" || exception.Details == nil || exception.Details.TypeName != "BoundsError" {
		t.Fatalf("ExceptionInfo() = %+v, %v", exception, err)
	}
	if err := client.Terminate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.Disconnect(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Continue(context.Background(), 1); err == nil {
		t.Fatal("request succeeded after Close")
	}
}

func TestClientTimeoutAdapterErrorAndCancellation(t *testing.T) {
	client, _ := startMock(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.request(ctx, "continue", map[string]any{"threadId": 1}, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("request canceled error = %v", err)
	}
	if got := normalizeTimeouts(TimeoutPolicy{}); got.Initialize != 10*time.Second || got.Launch != 10*time.Second || got.Attach != 20*time.Second || got.Evaluate != 30*time.Second || got.BreakpointVerification != 10*time.Second {
		t.Fatalf("default timeout policy = %+v", got)
	}
	if _, err := (&Client{policy: normalizeTimeouts(TimeoutPolicy{}), pending: map[int]chan dapResponse{}, closed: true}).request(context.Background(), "continue", nil, time.Second); err == nil {
		t.Fatal("closed client accepted request")
	}
	if _, err := client.request(context.Background(), "invalid", map[int]string{1: "no"}, time.Second); err == nil {
		t.Fatal("request with non-string JSON object keys succeeded")
	}
	if _, err := client.request(context.Background(), "invalid", map[string]any{"value": math.NaN()}, time.Second); err == nil {
		t.Fatal("request with NaN argument succeeded")
	}
}

func TestConcurrentRequestsPreserveWireSequence(t *testing.T) {
	client, _ := startMock(t)
	const requests = 48
	errorsFound := make(chan error, requests)
	var workers sync.WaitGroup
	workers.Add(requests)
	for range requests {
		go func() {
			defer workers.Done()
			errorsFound <- client.Next(context.Background(), 1)
		}()
	}
	workers.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestDAPFramingHandlesHeadersAndValidation(t *testing.T) {
	var output strings.Builder
	if err := writeMessage(&output, map[string]any{"seq": 1, "type": "request"}); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(strings.NewReader("Content-Type: application/vscode-jsonrpc; charset=utf-8\r\n" + output.String()))
	data, err := readMessage(reader)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil || got["type"] != "request" {
		t.Fatalf("framed message = %s, %v", data, err)
	}
	for _, malformed := range []string{
		"Content-Length: -1\r\n\r\n",
		"Content-Length: 2\r\nContent-Length: 2\r\n\r\n{}",
		"Other: 1\r\n\r\n{}",
		"Content-Length: 2\r\n\r\nno",
		"X: " + strings.Repeat("a", 5000) + "\r\n\r\n",
	} {
		if _, err := readMessage(bufio.NewReader(strings.NewReader(malformed))); err == nil {
			t.Errorf("readMessage(%q) succeeded", malformed)
		}
	}
	short := &limitedWriter{maxWrite: 3}
	if err := writeMessage(short, map[string]any{"type": "event", "event": "stopped"}); err != nil {
		t.Fatalf("writeMessage with short writes: %v", err)
	}
	if _, err := readMessage(bufio.NewReader(strings.NewReader(string(short.data)))); err != nil {
		t.Fatalf("read short-written message: %v", err)
	}
	client := &Client{policy: normalizeTimeouts(TimeoutPolicy{}), pending: make(map[int]chan dapResponse)}
	if err := validateDAPArguments(map[string]any{"value": math.NaN()}); err == nil {
		t.Fatal("DAP JSON validation accepted NaN")
	}
	if err := validateDAPArguments(map[string]any{"value": make(chan int)}); err == nil {
		t.Fatal("DAP JSON validation accepted a channel")
	}
	if _, err := client.request(context.Background(), "nope", map[string]any{"value": make(chan int)}, time.Second); err == nil {
		t.Fatal("request sent unsupported argument value")
	}
}

func TestTCPAdapterBridge(t *testing.T) {
	command := mockAdapterCommand(t, true)
	client, capabilities, err := Start(context.Background(), Options{
		Command: command,
		Timeouts: TimeoutPolicy{
			Initialize: 3 * time.Second,
			Request:    3 * time.Second,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !capabilities.SupportsFunctionBreakpoints {
		t.Fatalf("TCP adapter capabilities = %+v", capabilities)
	}
	if err := client.Next(context.Background(), 1); err != nil {
		t.Fatalf("request over TCP bridge: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}

type limitedWriter struct {
	data     []byte
	maxWrite int
}

func (w *limitedWriter) Write(data []byte) (int, error) {
	count := min(len(data), w.maxWrite)
	w.data = append(w.data, data[:count]...)
	return count, nil
}

func TestJuliaAdapterCommandAndDiscovery(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "Project.toml"), []byte("name = \"Demo\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := JuliaDebugAdapterCommand(mockJuliaRuntime(), project, "")
	if command.Executable != "julia" || command.Directory != project || !command.TCPConnect || !contains(command.Args, "--project="+project) || !contains(command.Args, `using Sockets, DebugAdapter; conn = connect(ip"127.0.0.1", parse(Int, ENV["JIDE_DAP_PORT"])); Base.run(DebugAdapter.DebugSession(conn))`) {
		t.Fatalf("JuliaDebugAdapterCommand() = %+v", command)
	}
	discoverer := &Discoverer{
		Julia: &jlproc.Discoverer{
			LookPath: func(name string) (string, error) {
				if name == "juliaup" {
					return "", os.ErrNotExist
				}
				return "julia", nil
			},
			Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
				if reflect.DeepEqual(args, []string{"--version"}) {
					return []byte("julia version 1.13.1"), nil
				}
				return nil, nil
			},
		},
		Run: func(_ context.Context, executable string, args ...string) ([]byte, error) {
			if executable != "julia" || !contains(args, `Base.find_package("DebugAdapter") === nothing && exit(1)`) {
				t.Fatalf("DebugAdapter probe = %q %q", executable, args)
			}
			return nil, nil
		},
	}
	discovered, err := discoverer.Discover(context.Background(), project, project)
	if err != nil || discovered.Executable != "julia" {
		t.Fatalf("Discover() = %+v, %v", discovered, err)
	}
	missing := *discoverer
	missing.Run = func(context.Context, string, ...string) ([]byte, error) { return nil, os.ErrNotExist }
	if _, err := missing.Discover(context.Background(), project, project); err == nil {
		t.Fatal("Discover() accepted unavailable DebugAdapter.jl")
	}

	program := filepath.Join(project, "main.jl")
	if err := os.WriteFile(program, []byte("x = 42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter, _ := startMock(t)
	if _, err := adapter.LaunchAndConfigure(context.Background(), map[string]any{"program": program, "project": project, "cwd": project}, nil, nil, nil); err != nil {
		t.Fatalf("LaunchAndConfigureJuliaFile() error = %v", err)
	}
	asyncAdapter, _ := startMock(t)
	pending, err := asyncAdapter.BeginLaunchJuliaFile(context.Background(), program, project, project, []string{"arg"}, true, []SourceBreakpoint{{Line: 1}})
	if err != nil {
		t.Fatalf("BeginLaunchJuliaFile() error = %v", err)
	}
	if _, err := pending.Wait(); err != nil {
		t.Fatalf("BeginLaunchJuliaFile() response error = %v", err)
	}
	codeAdapter, _ := startMock(t)
	if _, err := codeAdapter.AttachAndConfigure(context.Background(), map[string]any{"code": "x = 42", "program": filepath.Join(project, "inline.jl")}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestJuliaDebugAdapterIntegration(t *testing.T) {
	if os.Getenv("JIDE_DAP_REAL_ADAPTER") != "1" {
		t.Skip("optional real-adapter test; set JIDE_DAP_REAL_ADAPTER=1 after instantiating testdata/debugadapter")
	}
	project, err := filepath.Abs(filepath.Join("testdata", "debugadapter"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	client, capabilities, err := (&Discoverer{}).StartJuliaDebugSession(ctx, project, project, DefaultTimeoutPolicy, os.Stderr)
	if err != nil {
		t.Fatalf("start pinned DebugAdapter.jl environment: %v", err)
	}
	defer client.Close()

	program := filepath.Join(t.TempDir(), "debuggee.jl")
	if err := os.WriteFile(program, []byte("value = 40\nvalue += 2\nprintln(value)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pending, err := client.BeginLaunchJuliaFile(ctx, program, project, project, nil, true, []SourceBreakpoint{{Line: 2}})
	if err != nil {
		t.Fatalf("launch Julia program: %v", err)
	}
	defer pending.Cancel()

	stopped, err := waitForDAPEvent(ctx, client, "stopped")
	if err != nil {
		t.Fatal(err)
	}
	var stopBody struct {
		ThreadID int `json:"threadId"`
	}
	if err := json.Unmarshal(stopped.Body, &stopBody); err != nil || stopBody.ThreadID < 1 {
		t.Fatalf("stopped event body = %s, decode error = %v", stopped.Body, err)
	}
	threads, err := client.Threads(ctx)
	if err != nil || len(threads) == 0 {
		t.Fatalf("threads at stop = %+v, %v", threads, err)
	}
	stack, err := client.StackTrace(ctx, stopBody.ThreadID, 0, 20)
	if err != nil || len(stack.StackFrames) == 0 {
		t.Fatalf("stack at stop = %+v, %v", stack, err)
	}
	frameID := stack.StackFrames[0].ID
	if _, err := client.Scopes(ctx, frameID); err != nil {
		t.Fatalf("scopes at stop: %v", err)
	}
	if result, err := client.Evaluate(ctx, "1 + 1", frameID, "repl"); err != nil || strings.TrimSpace(result.Result) == "" {
		t.Fatalf("evaluate at stop = %+v, %v", result, err)
	}
	if capabilities.SupportsFunctionBreakpoints {
		if _, err := client.SetFunctionBreakpoints(ctx, []FunctionBreakpoint{{Name: "println"}}); err != nil {
			t.Fatalf("function breakpoint: %v", err)
		}
	}
	if len(capabilities.ExceptionBreakpointFilters) > 0 {
		if err := client.SetJuliaExceptionBreakpoints(ctx, nil, capabilities); err != nil {
			t.Fatalf("exception breakpoints: %v", err)
		}
	}
	if err := client.Next(ctx, stopBody.ThreadID); err != nil {
		t.Fatalf("step over: %v", err)
	}
	stepEvent, err := waitForDAPEvent(ctx, client, "stopped", "terminated", "exited")
	if err != nil {
		t.Fatalf("wait after stepping: %v", err)
	}
	if stepEvent.Name == "stopped" && capabilities.SupportsSetVariable {
		var steppedBody struct {
			ThreadID int `json:"threadId"`
		}
		if err := json.Unmarshal(stepEvent.Body, &steppedBody); err != nil {
			t.Fatal(err)
		}
		steppedStack, err := client.StackTrace(ctx, steppedBody.ThreadID, 0, 20)
		if err != nil || len(steppedStack.StackFrames) == 0 {
			t.Fatalf("stack after stepping = %+v, %v", steppedStack, err)
		}
		steppedScopes, err := client.Scopes(ctx, steppedStack.StackFrames[0].ID)
		if err != nil {
			t.Fatalf("scopes after stepping: %v", err)
		}
		assigned := false
		for _, scope := range steppedScopes {
			variables, varsErr := client.Variables(ctx, scope.VariablesReference, 0, 100)
			if varsErr != nil {
				t.Fatalf("variables after stepping: %v", varsErr)
			}
			for _, variable := range variables {
				if variable.Name == "value" {
					if _, err := client.SetVariable(ctx, scope.VariablesReference, "value", "41", capabilities); err != nil {
						t.Fatalf("assign variable: %v", err)
					}
					assigned = true
					break
				}
			}
			if assigned {
				break
			}
		}
		if !assigned {
			t.Fatal("adapter advertises setVariable but no value binding was available at the breakpoint")
		}
	}
	if _, err := pending.Wait(); err != nil {
		t.Fatalf("launch response: %v", err)
	}
	if err := client.Disconnect(ctx, true); err != nil {
		// DebugAdapter.jl may close its process as soon as it publishes the
		// terminated event, before the client can send a final disconnect.
		if waitErr := client.Wait(ctx); waitErr != nil {
			t.Fatalf("disconnect: %v (adapter exit: %v)", err, waitErr)
		}
	}
}

func waitForDAPEvent(ctx context.Context, client *Client, names ...string) (Event, error) {
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[name] = struct{}{}
	}
	for {
		select {
		case event, ok := <-client.Events():
			if !ok {
				return Event{}, errors.New("dap: adapter event stream closed")
			}
			if _, match := wanted[event.Name]; match {
				return event, nil
			}
		case <-ctx.Done():
			return Event{}, ctx.Err()
		}
	}
}

func startMock(t *testing.T) (*Client, Capabilities) {
	t.Helper()
	adapter := mockAdapterCommand(t, false)
	client, capabilities, err := Start(context.Background(), Options{
		Command:  adapter,
		Timeouts: TimeoutPolicy{Initialize: 3 * time.Second, Request: 3 * time.Second, Launch: 3 * time.Second, Attach: 3 * time.Second, Evaluate: 3 * time.Second, BreakpointVerification: 3 * time.Second},
		Stderr:   os.Stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, capabilities
}

func mockAdapterCommand(t *testing.T, tcp bool) AdapterCommand {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	childArguments := slices.DeleteFunc(os.Args[1:], func(argument string) bool { return strings.HasPrefix(argument, "-test.run=") })
	childArguments = append(childArguments, "-test.run=TestDAPMockAdapterProcess")
	environment := append(os.Environ(), mockAdapterEnv+"=1")
	if tcp {
		environment = append(environment, mockAdapterTCPEnv+"=1")
	}
	return AdapterCommand{
		Executable: executable,
		Args:       childArguments,
		Env:        environment,
		TCPConnect: tcp,
	}
}

func mockJuliaRuntime() jlproc.Runtime { return jlproc.Runtime{Executable: "julia"} }

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func TestDAPMockAdapterProcess(t *testing.T) {
	if os.Getenv(mockAdapterEnv) != "1" {
		t.Skip("helper test is only run as a mock adapter child")
	}
}
