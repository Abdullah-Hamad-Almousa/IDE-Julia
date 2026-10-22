// Command jide is the jide desktop shell.
//
// The shell is a Wails v2 application: a Go core that owns the window and the
// lifecycle, and a Svelte frontend rendered in the platform webview. It talks
// to the engine daemon over the jide:// JSON-RPC 2.0 protocol, so the shell
// never owns IDE state and can be replaced without touching the engine.
//
// In WP0 the shell is a stub. It only reports its version; the Wails
// application, the Monaco editor surface and the Svelte frontend arrive with
// the work packages that own them.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
)

// version is the shell version. Release builds override it with
// -ldflags "-X main.version=<version>".
var version = "0.0.0-dev"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "jide:", err)
		os.Exit(1)
	}
}

// run executes the command line of the stub. Only --version is supported.
func run(args []string, out io.Writer) error {
	for _, arg := range args {
		switch arg {
		case "-version", "--version", "version":
			return printVersion(out)
		default:
			return fmt.Errorf("unknown argument %q; the WP0 stub only supports --version", arg)
		}
	}
	return printVersion(out)
}

// printVersion writes the version banner to out.
func printVersion(out io.Writer) error {
	if _, err := fmt.Fprintln(out, versionLine()); err != nil {
		return fmt.Errorf("jide: print version: %w", err)
	}
	return nil
}

// versionLine renders the one line version banner.
func versionLine() string {
	return fmt.Sprintf(
		"jide %s (desktop shell stub, %s, %s/%s)",
		version,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	)
}
