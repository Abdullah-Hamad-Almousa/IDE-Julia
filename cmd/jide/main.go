// Command jide is the jide desktop shell.
//
// The shell is a Wails v2 application: a Go window/lifecycle host and a Svelte
// frontend rendered in the platform webview. Its Go binding currently delegates
// to the headless workspace, VFS, and syntax packages in-process; it will switch
// to the jide:// JSON-RPC daemon when cmd/jided has a request dispatcher.
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
	launch, err := run(os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jide:", err)
		os.Exit(1)
	}
	if launch {
		if err := launchDesktop(); err != nil {
			fmt.Fprintln(os.Stderr, "jide:", err)
			os.Exit(1)
		}
	}
}

// run parses the command line. With no arguments it requests the desktop
// window; the version flags print the version and exit.
func run(args []string, out io.Writer) (bool, error) {
	if len(args) == 0 {
		return true, nil
	}
	for _, arg := range args {
		switch arg {
		case "-version", "--version", "version":
			return false, printVersion(out)
		default:
			return false, fmt.Errorf("unknown argument %q; use --version or start without arguments", arg)
		}
	}
	return false, nil
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
		"jide %s (desktop, %s, %s/%s)",
		version,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	)
}
