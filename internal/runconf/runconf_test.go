package runconf

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
)

const helperModeEnv = "JIDE_RUNCONF_HELPER_MODE"

func TestConfigurationJSONRoundTripAndBuiltins(t *testing.T) {
	config := Configuration{
		ID: "run-1", Name: "Example", Type: TypeJuliaScript, Program: "main.jl",
		Arguments: []string{"hello"}, Properties: map[string]json.RawMessage{"custom": json.RawMessage(`{"enabled":true}`)},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Configuration
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != config.ID || decoded.Type != config.Type || !slices.Equal(decoded.Arguments, config.Arguments) || string(decoded.Properties["custom"]) != string(config.Properties["custom"]) {
		t.Fatalf("configuration round trip = %+v, want %+v", decoded, config)
	}
	if got, want := NewRegistry().Types(), []string{TypeJuliaModule, TypeJuliaREPL, TypeJuliaScript}; !slices.Equal(got, want) {
		t.Fatalf("built-in types = %v, want %v", got, want)
	}
}

func TestJuliaScriptRunIntegration(t *testing.T) {
	runtime, err := jlproc.NewDiscoverer().Discover(context.Background())
	if err != nil {
		t.Skipf("Julia is not available for the optional process integration test: %v", err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Project.toml"), []byte("name = \"RunFixture\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	program := filepath.Join(root, "main.jl")
	if err := os.WriteFile(program, []byte("println(\"runconf script integration\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	runner := NewRunner(*runtime, nil)
	process, err := runner.Start(context.Background(), Configuration{
		Type: TypeJuliaScript, Name: "integration", Program: program, ProjectPath: root,
	}, func(event Event) {
		if event.Stream == StreamStdout {
			output.WriteString(event.Data)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := process.Wait(context.Background())
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("Julia script result = %+v, %v", result, err)
	}
	if !strings.Contains(output.String(), "runconf script integration") {
		t.Fatalf("Julia script output = %q", output.String())
	}
}

func TestRegistryBuildsScriptModuleAndREPLInvocations(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "Project.toml")
	if err := os.WriteFile(project, []byte("name = \"Fixture\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "main.jl")
	if err := os.WriteFile(script, []byte("println(ARGS)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := jlproc.Runtime{Executable: "julia"}
	registry := NewRegistry()

	invocation, err := registry.Build(runtime, Configuration{Type: TypeJuliaScript, Program: script, ProjectPath: project, Arguments: []string{"x"}})
	if err != nil {
		t.Fatal(err)
	}
	if invocation.Executable != "julia" || invocation.WorkingDirectory != root || !containsString(invocation.Arguments, "--project="+root) || !containsString(invocation.Arguments, script) || invocation.Arguments[len(invocation.Arguments)-1] != "x" {
		t.Fatalf("script invocation = %+v", invocation)
	}

	moduleInvocation, err := registry.Build(runtime, Configuration{Type: TypeJuliaModule, Module: "Fixture.Tools", ProjectPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(moduleInvocation.Arguments, "using Fixture.Tools") || moduleInvocation.WorkingDirectory != root {
		t.Fatalf("module invocation = %+v", moduleInvocation)
	}

	replInvocation, err := registry.Build(runtime, Configuration{Type: TypeJuliaREPL, ProjectPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if !replInvocation.Interactive || !containsString(replInvocation.Arguments, "--interactive") {
		t.Fatalf("REPL invocation = %+v", replInvocation)
	}
}

func TestRegistryValidatesAndAcceptsUserDefinedTypes(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(ConfigType{ID: "custom", Validate: func(Configuration) error { return nil }, Build: func(_ jlproc.Runtime, _ Configuration) (Invocation, error) {
		return Invocation{Executable: "custom-runner", Arguments: []string{"--custom"}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(ConfigType{ID: "custom"}); err == nil {
		t.Fatal("Register() accepted an incomplete configuration type")
	}
	invocation, err := registry.Build(jlproc.Runtime{}, Configuration{Type: "custom"})
	if err != nil || invocation.Executable != "custom-runner" {
		t.Fatalf("custom invocation = %+v, %v", invocation, err)
	}
	if _, err := registry.Build(jlproc.Runtime{}, Configuration{Type: "missing"}); err == nil {
		t.Fatal("Build() accepted an unknown configuration type")
	}
	if _, err := registry.Build(jlproc.Runtime{}, Configuration{Type: TypeJuliaModule, Module: "bad-name"}); err == nil {
		t.Fatal("Build() accepted an invalid Julia module name")
	}
}

func TestRunnerStreamsOutputAndSupervisesProcess(t *testing.T) {
	var eventsMu sync.Mutex
	var events []Event
	registry := NewRegistry()
	if err := registry.Register(ConfigType{
		ID:       "test-helper",
		Validate: func(Configuration) error { return nil },
		Build: func(_ jlproc.Runtime, config Configuration) (Invocation, error) {
			return Invocation{Executable: os.Args[0], Arguments: []string{"-test.run=TestRunConfHelper"},
				Environment: map[string]string{helperModeEnv: config.Name}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(jlproc.Runtime{}, registry)
	defer func() { _ = runner.Close(context.Background()) }()
	process, err := runner.Start(context.Background(), Configuration{ID: "unique", Type: "test-helper", Name: "output"}, func(event Event) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := process.Wait(context.Background())
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("Wait() = %+v, %v", result, err)
	}
	eventsMu.Lock()
	defer eventsMu.Unlock()
	var stdout, stderr string
	for _, event := range events {
		switch event.Stream {
		case StreamStdout:
			stdout += event.Data
		case StreamStderr:
			stderr += event.Data
		}
	}
	if !strings.Contains(stdout, "runconf stdout") || !strings.Contains(stderr, "runconf stderr") {
		t.Fatalf("streamed output stdout=%q stderr=%q; events=%+v", stdout, stderr, events)
	}
	second, err := runner.Start(context.Background(), Configuration{ID: "same-config", Type: "test-helper", Name: "output"}, nil)
	if err != nil {
		t.Fatalf("start a second process for the same configuration: %v", err)
	}
	if second.RunID == process.RunID {
		t.Fatalf("run ids were reused for separate process starts: %q", second.RunID)
	}
	if _, err := second.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerTerminateAndWaitCancellation(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(ConfigType{
		ID:       "test-sleeper",
		Validate: func(Configuration) error { return nil },
		Build: func(_ jlproc.Runtime, _ Configuration) (Invocation, error) {
			return Invocation{Executable: os.Args[0], Arguments: []string{"-test.run=TestRunConfHelper"},
				Environment: map[string]string{helperModeEnv: "sleep"}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(jlproc.Runtime{}, registry)
	process, err := runner.Start(context.Background(), Configuration{Type: "test-sleeper"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := process.Wait(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait() error = %v, want deadline exceeded", err)
	}
	terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer terminateCancel()
	if err := process.Terminate(terminateCtx); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	if _, err := process.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerWritesInputToInteractiveProcess(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(ConfigType{
		ID:       "test-input",
		Validate: func(Configuration) error { return nil },
		Build: func(_ jlproc.Runtime, _ Configuration) (Invocation, error) {
			return Invocation{Executable: os.Args[0], Arguments: []string{"-test.run=TestRunConfHelper"},
				Environment: map[string]string{helperModeEnv: "input"}, Interactive: true}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(jlproc.Runtime{}, registry)
	process, err := runner.Start(context.Background(), Configuration{Type: "test-input"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.WriteInput([]byte("read from stdin\n")); err != nil {
		t.Fatal(err)
	}
	result, err := process.Wait(context.Background())
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("Wait() = %+v, %v", result, err)
	}
}

func TestRunConfHelper(t *testing.T) {
	switch os.Getenv(helperModeEnv) {
	case "output":
		_, _ = os.Stdout.WriteString("runconf stdout\n")
		_, _ = os.Stderr.WriteString("runconf stderr\n")
	case "sleep":
		time.Sleep(10 * time.Second)
	case "input":
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			t.Fatalf("read stdin: %v", err)
		}
		_, _ = os.Stdout.WriteString("received: " + line)
	default:
		t.Skip("helper process used by the process-runner tests")
	}
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}
