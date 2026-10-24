package jlproc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestConsoleSessionsLifecycleHistoryAndReopen(t *testing.T) {
	installConsoleStub(t)
	projectA := writeConsoleProject(t, "alpha")
	projectB := writeConsoleProject(t, "beta")
	output := make(chan ConsoleOutput, 64)
	manager := NewConsoleManager(Runtime{Executable: consoleTestExecutable(t)}, func(event ConsoleOutput) {
		output <- event
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := manager.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	})

	first, err := manager.Open(context.Background(), ConsoleOptions{ProjectPath: projectA})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Open(context.Background(), ConsoleOptions{ProjectPath: projectB})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.ProcessID == 0 || second.ProcessID == 0 {
		t.Fatalf("sessions were not distinct/running: first=%+v second=%+v", first, second)
	}
	if first.ProjectPath != projectA || second.ProjectPath != projectB {
		t.Fatalf("project selection lost: first=%q second=%q", first.ProjectPath, second.ProjectPath)
	}
	waitConsoleOutput(t, output, first.ID, "console-ready")
	waitConsoleOutput(t, output, second.ID, "console-ready")

	newline := "\n"
	if runtime.GOOS == "windows" {
		newline = "\r"
	}
	if err := manager.Write(context.Background(), first.ID, "one"+newline+"two"+newline+"partial"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Write(context.Background(), first.ID, " command"+newline); err != nil {
		t.Fatal(err)
	}
	waitConsoleOutput(t, output, first.ID, "received:partial command")
	wantHistory := []string{"one", "two", "partial command"}
	history, err := manager.History(first.ID)
	if err != nil || !slices.Equal(history, wantHistory) {
		t.Fatalf("History() = %#v, %v; want %#v", history, err, wantHistory)
	}

	if err := manager.Resize(first.ID, 100, 36); err != nil {
		t.Fatalf("Resize() error = %v", err)
	}
	if err := manager.Close(context.Background(), first.ID); err != nil {
		t.Fatalf("Close(first) error = %v", err)
	}

	reopened, err := manager.Reopen(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ID == first.ID || reopened.ReopenedFrom != first.ID || !slices.Equal(reopened.History, wantHistory) {
		t.Fatalf("reopened session = %+v; want copied history and source ID", reopened)
	}
	if err := manager.Close(context.Background(), reopened.ID); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	closed, err := manager.Session(reopened.ID)
	if err != nil || closed.State != ConsoleClosed {
		t.Fatalf("closed session = %+v, %v", closed, err)
	}
	if err := manager.Write(context.Background(), reopened.ID, "not-running\n"); err == nil {
		t.Fatal("Write() succeeded for a closed console")
	}

	sessions := manager.Sessions()
	if len(sessions) != 3 {
		t.Fatalf("Sessions() returned %d entries, want 3: %+v", len(sessions), sessions)
	}
	if err := manager.Close(context.Background(), second.ID); err != nil {
		t.Fatalf("Close(second) error = %v", err)
	}
}

func TestConsoleValidatesEnvironmentDimensionsAndContext(t *testing.T) {
	manager := NewConsoleManager(Runtime{Executable: "julia"}, nil)
	if _, err := manager.Open(context.Background(), ConsoleOptions{ProjectPath: t.TempDir()}); err == nil {
		t.Fatal("Open() accepted a directory without Project.toml")
	}
	project := writeConsoleProject(t, "validation")
	if _, err := manager.Open(context.Background(), ConsoleOptions{ProjectPath: project, Width: -1}); err == nil {
		t.Fatal("Open() accepted a negative terminal width")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.Open(ctx, ConsoleOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Open(canceled) error = %v, want context.Canceled", err)
	}
	if _, err := manager.Reopen(context.Background(), "missing"); err == nil {
		t.Fatal("Reopen() accepted an unknown session")
	}
}

func TestConsoleJuliaREPLIntegration(t *testing.T) {
	julia, err := execLookPath("julia")
	if err != nil {
		t.Skip("Julia is not installed; real REPL PTY integration is unavailable")
	}
	output := make(chan ConsoleOutput, 64)
	manager := NewConsoleManager(Runtime{Executable: julia}, func(event ConsoleOutput) { output <- event })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := manager.Open(ctx, ConsoleOptions{WorkingDirectory: t.TempDir()})
	if err != nil {
		t.Fatalf("Open(Julia): %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = manager.Shutdown(cleanupCtx)
	}()
	if err := manager.Write(ctx, session.ID, "println(\"JIDE_REPL_ACCEPTANCE\")\n"); err != nil {
		t.Fatal(err)
	}
	waitConsoleOutput(t, output, session.ID, "JIDE_REPL_ACCEPTANCE")
	if err := manager.Close(ctx, session.ID); err != nil {
		t.Fatalf("Close(Julia): %v", err)
	}
}

func TestJuliaConsoleArgsSelectProjectAndInteractiveHistory(t *testing.T) {
	args := juliaConsoleArgs(filepath.Join("C:", "work", "Project"))
	want := []string{"--startup-file=no", "--history-file=yes", "--color=no", "--interactive", "--project=" + filepath.Join("C:", "work", "Project")}
	if !slices.Equal(args, want) {
		t.Fatalf("juliaConsoleArgs() = %#v, want %#v", args, want)
	}
}

func installConsoleStub(t *testing.T) {
	t.Helper()
	t.Setenv(consoleStubEnv, "1")
	t.Setenv(stubEnv, "")
}

func consoleTestExecutable(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return executable
}

func writeConsoleProject(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Project.toml"), []byte("name = \""+name+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func waitConsoleOutput(t *testing.T, output <-chan ConsoleOutput, sessionID, contains string) {
	t.Helper()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-output:
			if event.SessionID != sessionID {
				continue
			}
			if strings.Contains(string(event.Data), contains) {
				return
			}
			if event.Error != "" {
				t.Fatalf("console %q reported error: %s", sessionID, event.Error)
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for console %q output containing %q", sessionID, contains)
		}
	}
}

var execLookPath = exec.LookPath
