package jlproc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Errors of the discovery.
//
// The errors are typed so that a caller can tell a missing runtime from a
// runtime that is too old without matching on the message.
var (
	// ErrNotFound reports that no Julia runtime was found.
	ErrNotFound = errors.New("jlproc: no Julia runtime was found")
	// ErrVersionTooOld reports that the Julia runtime is older than the
	// minimum release that jide supports.
	ErrVersionTooOld = errors.New("jlproc: the Julia runtime is older than the supported minimum")
)

// NotFoundError reports that no Julia runtime could be found.
type NotFoundError struct {
	// Searched lists the locations that were inspected.
	Searched []string
}

// Error implements the error interface.
func (e *NotFoundError) Error() string {
	if len(e.Searched) == 0 {
		return ErrNotFound.Error()
	}
	return fmt.Sprintf("%s (searched %s)", ErrNotFound, strings.Join(e.Searched, ", "))
}

// Unwrap returns the sentinel error of a missing runtime.
func (e *NotFoundError) Unwrap() error {
	return ErrNotFound
}

// VersionError reports that the Julia runtime is older than the minimum
// release that jide supports.
type VersionError struct {
	// Found is the version of the runtime that was found.
	Found Version
	// Minimum is the oldest version that jide supports.
	Minimum Version
}

// Error implements the error interface.
func (e *VersionError) Error() string {
	return fmt.Sprintf("%s: found %s, need at least %s", ErrVersionTooOld, e.Found, e.Minimum)
}

// Unwrap returns the sentinel error of a runtime that is too old.
func (e *VersionError) Unwrap() error {
	return ErrVersionTooOld
}

// Source names the origin of a discovered runtime.
type Source string

const (
	// SourceJuliaup marks a runtime that juliaup manages.
	SourceJuliaup Source = "juliaup"
	// SourcePath marks a runtime that was found on the search path of the
	// operating system and that juliaup does not manage.
	SourcePath Source = "path"
)

// String returns the name of the source.
func (s Source) String() string {
	return string(s)
}

// Runtime is a Julia installation that jide can run.
type Runtime struct {
	// Executable is the absolute path of the julia executable.
	Executable string
	// Version is the version of the runtime.
	Version Version
	// Channel is the juliaup channel that provides the runtime. It is
	// ChannelUnknown when juliaup does not manage the runtime.
	Channel Channel
	// Source describes how the runtime was found.
	Source Source
}

// Discoverer resolves the Julia runtime of the machine.
//
// Every dependency of the discovery is injectable, so that a test can drive
// it with a fake juliaup and a fake julia on the search path instead of a
// real installation.
type Discoverer struct {
	// LookPath resolves the path of an executable. It defaults to
	// exec.LookPath.
	LookPath func(name string) (string, error)
	// Run executes a command and returns its standard output. It defaults to
	// a real subprocess.
	Run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewDiscoverer returns a discoverer that uses the search path and the
// process table of the machine.
func NewDiscoverer() *Discoverer {
	return &Discoverer{}
}

// lookPath returns the configured lookup function.
func (d *Discoverer) lookPath(name string) (string, error) {
	if d.LookPath != nil {
		return d.LookPath(name)
	}
	return exec.LookPath(name)
}

// run executes a command and returns its standard output.
func (d *Discoverer) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if d.Run != nil {
		return d.Run(ctx, name, args...)
	}
	return runCommand(ctx, name, args...)
}

// Discover resolves the Julia runtime of the machine.
//
// The discovery reads the channel of juliaup, when juliaup is installed,
// and probes the version of the runtime. It fails with a NotFoundError when
// no runtime is installed and with a VersionError when the runtime is older
// than MinVersion.
func (d *Discoverer) Discover(ctx context.Context) (*Runtime, error) {
	juliaupPath, juliaupFound := d.lookPathOptional("juliaup")
	status := JuliaupStatus{}
	if juliaupFound {
		if output, err := d.run(ctx, juliaupPath, "status"); err == nil {
			if parsed, err := ParseJuliaupStatus(string(output)); err == nil {
				status = parsed
			}
		}
	}

	executable, source, err := d.findJulia(juliaupPath, juliaupFound)
	if err != nil {
		return nil, err
	}
	output, err := d.run(ctx, executable, "--version")
	if err != nil {
		return nil, fmt.Errorf("jlproc: probe %q: %w", executable, err)
	}
	version, err := ParseJuliaVersion(string(output))
	if err != nil {
		return nil, fmt.Errorf("jlproc: probe %q: %w", executable, err)
	}
	if !version.AtLeast(MinVersion) {
		return nil, &VersionError{Found: version, Minimum: MinVersion}
	}

	resolved := &Runtime{
		Executable: executable,
		Version:    version,
		Channel:    ChannelUnknown,
		Source:     source,
	}
	if source == SourceJuliaup {
		resolved.Channel = status.Current
		if resolved.Channel == ChannelUnknown {
			resolved.Channel = status.ChannelOf(version)
		}
	}
	return resolved, nil
}

// lookPathOptional resolves an executable and reports whether it exists.
func (d *Discoverer) lookPathOptional(name string) (string, bool) {
	path, err := d.lookPath(name)
	if err != nil {
		return "", false
	}
	return path, true
}

// findJulia locates the julia executable.
//
// juliaup installs the shims of the channels next to the juliaup executable,
// so the shim of the current channel is preferred over a Julia installation
// that lies somewhere else on the search path.
func (d *Discoverer) findJulia(juliaupPath string, juliaupFound bool) (string, Source, error) {
	searched := make([]string, 0, 4)
	if juliaupFound {
		directory := filepath.Dir(juliaupPath)
		for _, name := range juliaExecutableNames() {
			candidate := filepath.Join(directory, name)
			searched = append(searched, candidate)
			if isExecutableFile(candidate) {
				return candidate, SourceJuliaup, nil
			}
		}
	}
	searched = append(searched, "julia")
	if path, err := d.lookPath("julia"); err == nil {
		return path, SourcePath, nil
	}
	return "", "", &NotFoundError{Searched: searched}
}

// juliaExecutableNames returns the file names of the julia executable for
// the current platform.
func juliaExecutableNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"julia.exe", "julia.bat", "julia.cmd"}
	}
	return []string{"julia"}
}

// isExecutableFile reports whether the path is a regular file.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// runCommand executes a command and returns its standard output.
func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

// ParseJuliaVersion extracts the version from the output of
// "julia --version".
//
// The output of the command has the form "julia version 1.10.3". The parser
// also accepts a bare version, so that a shim that prints the version alone
// is understood.
func ParseJuliaVersion(output string) (Version, error) {
	fields := strings.Fields(strings.TrimSpace(output))
	for index, field := range fields {
		if field == "version" && index+1 < len(fields) {
			return ParseVersion(fields[index+1])
		}
	}
	for index := len(fields) - 1; index >= 0; index-- {
		if version, err := ParseVersion(fields[index]); err == nil {
			return version, nil
		}
	}
	return Version{}, fmt.Errorf("jlproc: %w: %q", ErrInvalidVersion, strings.TrimSpace(output))
}
