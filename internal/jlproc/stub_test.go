package jlproc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The tests of the discovery drive it with a fake juliaup and a fake julia
// instead of a real installation. The fake is a copy of the test binary
// itself: when it runs with the stub marker in its environment it answers
// the probes of the discovery instead of running the tests.

const (
	// stubEnv marks the test binary as a fake juliaup or julia.
	stubEnv = "JIDE_JLPROC_STUB"
	// stubVersionEnv holds the version that the fake julia reports.
	stubVersionEnv = "JIDE_JLPROC_STUB_JULIA_VERSION"
	// stubStatusEnv holds the output that the fake juliaup reports.
	stubStatusEnv = "JIDE_JLPROC_STUB_JULIAUP_STATUS"
	// consoleStubEnv makes this test binary act as a PTY child process.
	consoleStubEnv = "JIDE_JLPROC_CONSOLE_STUB"
)

// defaultStubVersion is the version that the fake julia reports when the
// test does not configure another one.
const defaultStubVersion = "1.10.3"

// defaultStubStatus is the output that the fake juliaup reports when the
// test does not configure another one.
const defaultStubStatus = `Installed Channels:
  stable    1.10.3    https://example.invalid/julia-1.10.3.tar.gz
  lts       1.10.3    https://example.invalid/julia-1.10.3.tar.gz
Current Channel: stable
`

// TestMain turns the test binary into the fake juliaup and the fake julia
// when the environment asks for it.
func TestMain(m *testing.M) {
	if os.Getenv(stubEnv) != "" {
		os.Exit(runStub())
	}
	if os.Getenv(consoleStubEnv) != "" {
		os.Exit(runConsoleStub())
	}
	os.Exit(m.Run())
}

func runConsoleStub() int {
	fmt.Fprintln(os.Stdout, "console-ready")
	reader := bufio.NewReader(os.Stdin)
	var line strings.Builder
	lastWasCR := false
	for {
		character, err := reader.ReadByte()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			return 0
		}
		if character == '\n' && lastWasCR {
			lastWasCR = false
			continue
		}
		if character == '\r' || character == '\n' {
			lastWasCR = character == '\r'
			text := line.String()
			line.Reset()
			fmt.Fprintln(os.Stdout, "received:"+text)
			if text == "exit" {
				return 0
			}
			continue
		}
		lastWasCR = false
		line.WriteByte(character)
	}
}

// runStub answers the probes of the discovery.
func runStub() int {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	switch name {
	case "juliaup":
		if len(os.Args) > 1 && os.Args[1] == "status" {
			status := os.Getenv(stubStatusEnv)
			if status == "" {
				status = defaultStubStatus
			}
			fmt.Fprint(os.Stdout, status)
			return 0
		}
		fmt.Fprintln(os.Stdout, "juliaup 1.12.0")
		return 0
	case "julia":
		version := os.Getenv(stubVersionEnv)
		if version == "" {
			version = defaultStubVersion
		}
		fmt.Fprintf(os.Stdout, "julia version %s\n", version)
		return 0
	default:
		if os.Getenv(consoleStubEnv) != "" {
			return runConsoleStub()
		}
		fmt.Fprintf(os.Stderr, "jlproc stub: unknown program %q\n", name)
		return 2
	}
}

// installStub copies the test binary into a temporary directory under the
// given names and puts the directory at the front of the search path.
//
// The copy answers the probes of the discovery instead of running the tests,
// so the discovery sees a Julia installation without one being installed.
func installStub(t *testing.T, names ...string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	directory := t.TempDir()
	for _, name := range names {
		target := filepath.Join(directory, name)
		if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(target), ".exe") {
			target += ".exe"
		}
		if err := copyExecutable(executable, target); err != nil {
			t.Fatalf("copy the test binary to %q: %v", target, err)
		}
	}
	t.Setenv(stubEnv, "1")
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// copyExecutable copies a file and keeps the executable bit.
func copyExecutable(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer output.Close()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Close()
}
