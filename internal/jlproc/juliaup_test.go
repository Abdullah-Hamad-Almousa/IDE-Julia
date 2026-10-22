package jlproc

import (
	"errors"
	"testing"
)

func TestParseJuliaupStatus(t *testing.T) {
	output := `Installed Channels:
  stable    1.10.3    https://example.invalid/julia-1.10.3.tar.gz
  lts       1.10.3    https://example.invalid/julia-1.10.3.tar.gz
  release   1.11.0    https://example.invalid/julia-1.11.0.tar.gz
Current Channel: stable
`
	status, err := ParseJuliaupStatus(output)
	if err != nil {
		t.Fatalf("ParseJuliaupStatus() error = %v", err)
	}
	if got, want := status.Current, ChannelStable; got != want {
		t.Errorf("Current = %q, want %q", got, want)
	}
	if got, want := len(status.Channels), 3; got != want {
		t.Fatalf("the status has %d channels, want %d", got, want)
	}
	first := status.Channels[0]
	if got, want := first.Name, ChannelStable; got != want {
		t.Errorf("Channels[0].Name = %q, want %q", got, want)
	}
	if got, want := first.Version.String(), "1.10.3"; got != want {
		t.Errorf("Channels[0].Version = %q, want %q", got, want)
	}
	if got, want := first.URL, "https://example.invalid/julia-1.10.3.tar.gz"; got != want {
		t.Errorf("Channels[0].URL = %q, want %q", got, want)
	}
}

func TestParseJuliaupStatusOfAnEmptyInstallation(t *testing.T) {
	output := `Installed Channels:
No julia versions installed.
Current Channel: stable
`
	status, err := ParseJuliaupStatus(output)
	if err != nil {
		t.Fatalf("ParseJuliaupStatus() error = %v", err)
	}
	if got, want := status.Current, ChannelStable; got != want {
		t.Errorf("Current = %q, want %q", got, want)
	}
	if len(status.Channels) != 0 {
		t.Errorf("the status has %d channels, want 0", len(status.Channels))
	}
}

func TestParseJuliaupStatusRejectsGarbage(t *testing.T) {
	if _, err := ParseJuliaupStatus("the command failed"); err == nil {
		t.Fatal("ParseJuliaupStatus() error = nil, want an error")
	} else if !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("ParseJuliaupStatus() error = %v, want it to wrap ErrInvalidStatus", err)
	}
}

func TestJuliaupStatusChannelOf(t *testing.T) {
	status := JuliaupStatus{
		Current: ChannelStable,
		Channels: []JuliaupChannel{
			{Name: ChannelStable, Version: Version{Major: 1, Minor: 10, Patch: 3}},
			{Name: ChannelRelease, Version: Version{Major: 1, Minor: 11}},
		},
	}
	if got, want := status.ChannelOf(Version{Major: 1, Minor: 11}), ChannelRelease; got != want {
		t.Errorf("ChannelOf() = %q, want %q", got, want)
	}
	if got, want := status.ChannelOf(Version{Major: 2, Minor: 0}), ChannelUnknown; got != want {
		t.Errorf("ChannelOf() = %q, want %q", got, want)
	}
}
