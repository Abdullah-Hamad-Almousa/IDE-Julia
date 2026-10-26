// Command jided is the jide engine daemon.
//
// The daemon owns the workspace model, the virtual file system, the indexes,
// the syntax services, the Julia runtime supervisor and the extension
// registry. It exposes all of it over the jide:// JSON-RPC 2.0 protocol
// defined in github.com/jide-dev/jide/proto/jide.
//
// The protocol is the seam between the daemon and the UI shell: every engine
// behavior must be reachable through it so that the engine stays testable
// without a GUI.
//
// The daemon provides DAP, run-configuration, and test-run methods over
// line-oriented JSON-RPC on stdin/stdout. The broader engine transport and
// service dispatch remain incremental work; --version is kept for diagnostics.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/jide-dev/jide/proto/jide"
)

// version is the daemon version. Release builds override it with
// -ldflags "-X main.version=<version>".
var version = "0.0.0-dev"

func main() {
	if err := runWithIO(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "jided:", err)
		os.Exit(1)
	}
}

// run executes the command line with an empty input stream, which is useful for
// version parsing tests. --stdio is handled by runWithIO in the daemon entry.
func run(args []string, out io.Writer) error {
	return runWithIO(args, strings.NewReader(""), out)
}

func runWithIO(args []string, in io.Reader, out io.Writer) error {
	if len(args) == 1 && args[0] == "--stdio" {
		return serveStdio(in, out)
	}
	for _, arg := range args {
		switch arg {
		case "-version", "--version", "version":
			return printVersion(out)
		default:
			return fmt.Errorf("unknown argument %q; use --version or --stdio", arg)
		}
	}
	return printVersion(out)
}

// printVersion writes the version banner to out.
func printVersion(out io.Writer) error {
	if _, err := fmt.Fprintln(out, versionLine()); err != nil {
		return fmt.Errorf("jided: print version: %w", err)
	}
	return nil
}

// versionLine renders the one line version banner.
func versionLine() string {
	return fmt.Sprintf(
		"jided %s (protocol %s, %s, %s/%s)",
		version,
		jide.Version,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	)
}
