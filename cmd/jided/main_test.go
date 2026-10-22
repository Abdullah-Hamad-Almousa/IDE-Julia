package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsTheVersionBanner(t *testing.T) {
	var out bytes.Buffer
	if err := run(nil, &out); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "jided ") {
		t.Errorf("run() output = %q, want it to start with %q", got, "jided ")
	}
	if !strings.Contains(got, version) {
		t.Errorf("run() output = %q, want it to contain the version %q", got, version)
	}
}

func TestRunSupportsTheVersionFlag(t *testing.T) {
	for _, flag := range []string{"-version", "--version", "version"} {
		var out bytes.Buffer
		if err := run([]string{flag}, &out); err != nil {
			t.Fatalf("run(%q) error = %v", flag, err)
		}
		if !strings.Contains(out.String(), "jided ") {
			t.Errorf("run(%q) output = %q, want the version banner", flag, out.String())
		}
	}
}

func TestRunRejectsUnknownArguments(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--serve"}, &out); err == nil {
		t.Fatal("run() error = nil, want an error for an unknown argument")
	}
	if out.Len() != 0 {
		t.Errorf("run() output = %q, want no output", out.String())
	}
}

func TestVersionLineMentionsTheProtocolVersion(t *testing.T) {
	line := versionLine()
	if !strings.Contains(line, "protocol 2.0") {
		t.Errorf("versionLine() = %q, want it to mention the protocol version", line)
	}
}
