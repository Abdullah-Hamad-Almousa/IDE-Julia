package jlproc

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidStatus reports that the output of "juliaup status" cannot be
// read.
var ErrInvalidStatus = errors.New("jlproc: the output of juliaup status cannot be read")

// Channel is a juliaup release channel.
//
// juliaup installs a Julia release per channel and launches the release of
// the current channel.
type Channel string

const (
	// ChannelUnknown is the channel of a runtime that juliaup does not
	// manage.
	ChannelUnknown Channel = ""
	// ChannelStable is the channel of the current stable release.
	ChannelStable Channel = "stable"
	// ChannelRelease is the channel of the current release of the
	// development branch.
	ChannelRelease Channel = "release"
	// ChannelLTS is the channel of the current long term support release.
	ChannelLTS Channel = "lts"
	// ChannelNightly is the channel of the nightly builds.
	ChannelNightly Channel = "nightly"
)

// String returns the name of the channel.
func (c Channel) String() string {
	return string(c)
}

// JuliaupChannel is one installed channel of juliaup.
type JuliaupChannel struct {
	// Name is the name of the channel.
	Name Channel
	// Version is the version that is installed for the channel.
	Version Version
	// URL is the download location of the release.
	URL string
}

// JuliaupStatus is the parsed output of "juliaup status".
type JuliaupStatus struct {
	// Current is the channel that juliaup launches by default.
	Current Channel
	// Channels holds the installed channels.
	Channels []JuliaupChannel
}

// ParseJuliaupStatus parses the output of "juliaup status".
//
// The output lists the installed channels and names the current one:
//
//	Installed Channels:
//	  stable    1.10.3    https://example.invalid/julia-1.10.3.tar.gz
//	Current Channel: stable
//
// The parser ignores the lines it does not understand, so a juliaup release
// that adds a section does not break the discovery.
func ParseJuliaupStatus(output string) (JuliaupStatus, error) {
	status := JuliaupStatus{}
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if rest, found := strings.CutPrefix(trimmed, "Current Channel:"); found {
			status.Current = Channel(strings.TrimSpace(rest))
			continue
		}
		if strings.HasPrefix(trimmed, "Installed Channels") {
			continue
		}
		if strings.Contains(trimmed, "No julia versions installed") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		version, err := ParseVersion(fields[1])
		if err != nil {
			continue
		}
		entry := JuliaupChannel{Name: Channel(fields[0]), Version: version}
		if len(fields) > 2 {
			entry.URL = fields[2]
		}
		status.Channels = append(status.Channels, entry)
	}
	if status.Current == ChannelUnknown && len(status.Channels) == 0 && strings.TrimSpace(output) != "" {
		return JuliaupStatus{}, fmt.Errorf("jlproc: %w", ErrInvalidStatus)
	}
	return status, nil
}

// ChannelOf returns the channel that provides the given version.
//
// It returns ChannelUnknown when the status does not mention the version.
func (s JuliaupStatus) ChannelOf(version Version) Channel {
	for _, channel := range s.Channels {
		if channel.Version.Compare(version) == 0 {
			return channel.Name
		}
	}
	return ChannelUnknown
}
