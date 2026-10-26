package testrun

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
)

func TestRunStreamsPassFailSkippedAndFailureLocation(t *testing.T) {
	runtime, err := (&jlproc.Discoverer{}).Discover(context.Background())
	if err != nil {
		if _, lookErr := exec.LookPath("julia"); lookErr != nil {
			t.Skipf("Julia is not installed; real @testset integration is unavailable: %v", err)
		}
		t.Fatalf("discover Julia: %v", err)
	}
	project := filepath.Join("testdata", "fixture")
	testFile := filepath.Join(project, "test", "runtests.jl")
	absoluteTestFile, err := filepath.Abs(testFile)
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	started := time.Now()
	result, runErr := NewRunner(*runtime).Run(context.Background(), Options{
		ProjectPath: project, WorkingDirectory: project, TestFile: testFile,
	}, func(event Event) {
		events = append(events, event)
	})
	if !errors.Is(runErr, ErrTestsFailed) {
		t.Fatalf("Run() error = %v, want ErrTestsFailed", runErr)
	}
	if result.RunID == "" || result.ExitCode != 1 || result.Failed != 1 || result.Passed != 2 || result.Skipped != 1 {
		t.Fatalf("test summary = %+v", result)
	}
	if result.Finished.Before(started) {
		t.Fatalf("finish time %v is before start %v", result.Finished, started)
	}
	var foundFailure, foundSetStart, foundSetFinish, foundFinished bool
	var statuses []Status
	for _, event := range events {
		if event.RunID != result.RunID {
			t.Errorf("event run id %q, want %q", event.RunID, result.RunID)
		}
		switch event.Kind {
		case EventTestSetStart:
			foundSetStart = true
		case EventTestSetFinish:
			foundSetFinish = true
		case EventTestResult:
			statuses = append(statuses, event.Status)
			if event.Status == StatusFailed {
				foundFailure = true
				if event.SourcePath != absoluteTestFile || event.Line != 5 {
					t.Errorf("failure location = %s:%d, want %s:5", event.SourcePath, event.Line, absoluteTestFile)
				}
				if event.Message == "" {
					t.Error("failure message is empty")
				}
			}
		case EventFinished:
			foundFinished = event.ExitCode == 1 && event.Failed == 1 && event.Passed == 2 && event.Skipped == 1
		}
	}
	if !foundFailure || !foundSetStart || !foundSetFinish || !foundFinished {
		t.Fatalf("test stream missing expected events: failure=%t set-start=%t set-finish=%t finished=%t events=%+v", foundFailure, foundSetStart, foundSetFinish, foundFinished, events)
	}
	for _, status := range []Status{StatusPassed, StatusFailed, StatusSkipped} {
		if !slices.Contains(statuses, status) {
			t.Errorf("test results did not include %q: %v", status, statuses)
		}
	}
}

func TestRunRejectsInvalidTestFileAndProject(t *testing.T) {
	runner := NewRunner(jlproc.Runtime{Executable: "julia"})
	if _, err := runner.Run(context.Background(), Options{TestFile: t.TempDir()}, nil); err == nil {
		t.Fatal("Run() accepted a directory as a test file")
	}
	testFile := filepath.Join(t.TempDir(), "runtests.jl")
	if err := os.WriteFile(testFile, []byte("using Test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(context.Background(), Options{TestFile: testFile, ProjectPath: filepath.Join(t.TempDir(), "missing")}, nil); err == nil {
		t.Fatal("Run() accepted a missing Julia project")
	}
	if _, err := runner.Run(context.Background(), Options{TestFile: "missing.jl"}, nil); err == nil {
		t.Fatal("Run() accepted a missing test file")
	}
}
