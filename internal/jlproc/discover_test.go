package jlproc

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestDiscoverResolvesTheRuntimeOfJuliaup(t *testing.T) {
	installStub(t, "juliaup", "julia")

	runtime, err := NewDiscoverer().Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := runtime.Version.String(), "1.10.3"; got != want {
		t.Errorf("Version = %q, want %q", got, want)
	}
	if got, want := runtime.Channel, ChannelStable; got != want {
		t.Errorf("Channel = %q, want %q", got, want)
	}
	if got, want := runtime.Source, SourceJuliaup; got != want {
		t.Errorf("Source = %q, want %q", got, want)
	}
	if runtime.Executable == "" {
		t.Error("Executable is empty")
	}
}

func TestDiscoverResolvesARuntimeWithoutJuliaup(t *testing.T) {
	installStub(t, "julia")

	runtime, err := (&Discoverer{LookPath: hideJuliaup}).Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := runtime.Source, SourcePath; got != want {
		t.Errorf("Source = %q, want %q", got, want)
	}
	if got, want := runtime.Channel, ChannelUnknown; got != want {
		t.Errorf("Channel = %q, want %q", got, want)
	}
	if got, want := runtime.Version.String(), "1.10.3"; got != want {
		t.Errorf("Version = %q, want %q", got, want)
	}
}

func TestDiscoverRejectsARuntimeOlderThanTheMinimum(t *testing.T) {
	installStub(t, "juliaup", "julia")
	t.Setenv(stubVersionEnv, "1.9.4")

	_, err := NewDiscoverer().Discover(context.Background())
	if err == nil {
		t.Fatal("Discover() error = nil, want a version error")
	}
	if !errors.Is(err, ErrVersionTooOld) {
		t.Fatalf("Discover() error = %v, want it to wrap ErrVersionTooOld", err)
	}
	var versionErr *VersionError
	if !errors.As(err, &versionErr) {
		t.Fatalf("Discover() error = %v, want a *VersionError", err)
	}
	if got, want := versionErr.Found.String(), "1.9.4"; got != want {
		t.Errorf("VersionError.Found = %q, want %q", got, want)
	}
	if got, want := versionErr.Minimum, MinVersion; got != want {
		t.Errorf("VersionError.Minimum = %v, want %v", got, want)
	}
}

func TestDiscoverAcceptsTheMinimumVersion(t *testing.T) {
	installStub(t, "julia")
	t.Setenv(stubVersionEnv, "1.10.0")

	runtime, err := NewDiscoverer().Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := runtime.Version.String(), "1.10.0"; got != want {
		t.Errorf("Version = %q, want %q", got, want)
	}
}

func TestDiscoverReportsAMissingRuntime(t *testing.T) {
	// The lookup is injected instead of using the search path of the
	// machine: a developer that works on jide has a Julia installation, and
	// the test must report the same result on every machine.
	discoverer := &Discoverer{
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
	}

	_, err := discoverer.Discover(context.Background())
	if err == nil {
		t.Fatal("Discover() error = nil, want a not found error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Discover() error = %v, want it to wrap ErrNotFound", err)
	}
	var notFoundErr *NotFoundError
	if !errors.As(err, &notFoundErr) {
		t.Fatalf("Discover() error = %v, want a *NotFoundError", err)
	}
	if len(notFoundErr.Searched) == 0 {
		t.Error("NotFoundError.Searched is empty")
	}
}

func TestDiscoverFailsWhenTheProbeFails(t *testing.T) {
	discoverer := &Discoverer{
		LookPath: func(name string) (string, error) {
			if name == "julia" {
				return "/usr/bin/julia", nil
			}
			return "", exec.ErrNotFound
		},
		Run: func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("the probe failed")
		},
	}

	_, err := discoverer.Discover(context.Background())
	if err == nil {
		t.Fatal("Discover() error = nil, want a probe error")
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrVersionTooOld) {
		t.Fatalf("Discover() error = %v, want a probe error", err)
	}
}

func TestDiscoverUsesTheInjectedRun(t *testing.T) {
	installStub(t, "julia")
	var calls []string
	discoverer := &Discoverer{
		LookPath: hideJuliaup,
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			calls = append(calls, name+" "+joinArgs(args))
			return []byte("julia version 1.11.2\n"), nil
		},
	}

	runtime, err := discoverer.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := runtime.Version.String(), "1.11.2"; got != want {
		t.Errorf("Version = %q, want %q", got, want)
	}
	if len(calls) != 1 {
		t.Fatalf("the discoverer ran %d commands, want 1: %v", len(calls), calls)
	}
}

// joinArgs renders the arguments of a command for the assertions.
func joinArgs(args []string) string {
	result := ""
	for index, arg := range args {
		if index > 0 {
			result += " "
		}
		result += arg
	}
	return result
}

// hideJuliaup behaves like exec.LookPath but never finds juliaup.
//
// A machine that has the juliaup alias of the Microsoft Store installed must
// not run it during the tests.
func hideJuliaup(name string) (string, error) {
	if name == "juliaup" {
		return "", exec.ErrNotFound
	}
	return exec.LookPath(name)
}
