package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsTheVersionBanner(t *testing.T) {
	var out bytes.Buffer
	launch, err := run([]string{"--version"}, &out)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if launch {
		t.Fatal("version flag requested the desktop window")
	}
	got := out.String()
	if !strings.HasPrefix(got, "jide ") {
		t.Errorf("run() output = %q, want it to start with %q", got, "jide ")
	}
	if !strings.Contains(got, version) {
		t.Errorf("run() output = %q, want it to contain the version %q", got, version)
	}
}

func TestRunSupportsTheVersionFlag(t *testing.T) {
	for _, flag := range []string{"-version", "--version", "version"} {
		var out bytes.Buffer
		launch, err := run([]string{flag}, &out)
		if err != nil {
			t.Fatalf("run(%q) error = %v", flag, err)
		}
		if launch {
			t.Errorf("run(%q) requested the desktop window", flag)
		}
		if !strings.Contains(out.String(), "jide ") {
			t.Errorf("run(%q) output = %q, want the version banner", flag, out.String())
		}
	}
}

func TestRunWithoutArgumentsLaunchesTheDesktop(t *testing.T) {
	var out bytes.Buffer
	launch, err := run(nil, &out)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !launch {
		t.Fatal("run() did not request the desktop window")
	}
	if out.Len() != 0 {
		t.Errorf("run() output = %q, want no output", out.String())
	}
}

func TestRunRejectsUnknownArguments(t *testing.T) {
	var out bytes.Buffer
	if _, err := run([]string{"--window"}, &out); err == nil {
		t.Fatal("run() error = nil, want an error for an unknown argument")
	}
	if out.Len() != 0 {
		t.Errorf("run() output = %q, want no output", out.String())
	}
}
