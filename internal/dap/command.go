package dap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
)

// AdapterCommand describes an adapter process. It is suitable for an explicit
// adapter executable as well as Julia's DebugAdapter.jl entry point.
type AdapterCommand struct {
	Executable string
	Args       []string
	Directory  string
	Env        []string
	TCPConnect bool
}

// JuliaDebugAdapterCommand starts DebugAdapter.jl in the requested project
// environment. DebugAdapter.jl must already be available in that environment.
func JuliaDebugAdapterCommand(runtime jlproc.Runtime, projectPath, workingDirectory string) AdapterCommand {
	args := []string{"--startup-file=no", "--history-file=no", "--color=no"}
	if projectPath != "" {
		args = append(args, "--project="+projectPath)
	}
	args = append(args, "-e", `using Sockets, DebugAdapter; conn = connect(ip"127.0.0.1", parse(Int, ENV["JIDE_DAP_PORT"])); Base.run(DebugAdapter.DebugSession(conn))`)
	if workingDirectory == "" {
		workingDirectory = projectPath
	}
	return AdapterCommand{Executable: runtime.Executable, Args: args, Directory: workingDirectory, TCPConnect: true}
}

// Discoverer locates Julia and verifies DebugAdapter.jl in the selected
// environment before returning a launch command.
type Discoverer struct {
	Julia *jlproc.Discoverer
	Run   func(context.Context, string, ...string) ([]byte, error)
}

// Discover finds Julia and checks that DebugAdapter.jl can be loaded from
// projectPath. An empty project uses Julia's default environment.
func (d *Discoverer) Discover(ctx context.Context, projectPath, workingDirectory string) (AdapterCommand, error) {
	if ctx == nil {
		return AdapterCommand{}, errors.New("dap: discovery context is nil")
	}
	discoverer := d.Julia
	if discoverer == nil {
		discoverer = jlproc.NewDiscoverer()
	}
	runtime, err := discoverer.Discover(ctx)
	if err != nil {
		return AdapterCommand{}, fmt.Errorf("dap: discover Julia: %w", err)
	}
	if projectPath != "" {
		absolute, err := filepath.Abs(projectPath)
		if err != nil {
			return AdapterCommand{}, fmt.Errorf("dap: resolve project path: %w", err)
		}
		if filepath.Base(absolute) == "Project.toml" {
			info, statErr := os.Stat(absolute)
			if statErr != nil || !info.Mode().IsRegular() {
				return AdapterCommand{}, fmt.Errorf("dap: project path %q is not a regular Project.toml", absolute)
			}
			absolute = filepath.Dir(absolute)
		}
		if info, statErr := os.Stat(filepath.Join(absolute, "Project.toml")); statErr != nil || !info.Mode().IsRegular() {
			return AdapterCommand{}, fmt.Errorf("dap: project directory %q has no regular Project.toml", absolute)
		}
		projectPath = absolute
	}
	if workingDirectory != "" {
		absolute, err := filepath.Abs(workingDirectory)
		if err != nil {
			return AdapterCommand{}, fmt.Errorf("dap: resolve working directory: %w", err)
		}
		info, statErr := os.Stat(absolute)
		if statErr != nil || !info.IsDir() {
			return AdapterCommand{}, fmt.Errorf("dap: working directory %q is not a directory", absolute)
		}
		workingDirectory = absolute
	}
	command := JuliaDebugAdapterCommand(*runtime, projectPath, workingDirectory)
	args := append([]string(nil), command.Args...)
	args[len(args)-1] = `Base.find_package("DebugAdapter") === nothing && exit(1)`
	run := d.Run
	if run == nil {
		run = runJuliaProbe
	}
	probeContext, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err := run(probeContext, runtime.Executable, args...); err != nil {
		return AdapterCommand{}, fmt.Errorf("dap: DebugAdapter.jl is unavailable in the selected Julia environment: %w", err)
	}
	return command, nil
}

func runJuliaProbe(ctx context.Context, executable string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return output, ctx.Err()
		}
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

// FindAdapter resolves an explicit DAP adapter executable from PATH.
func FindAdapter(name string) (AdapterCommand, error) {
	if strings.TrimSpace(name) == "" {
		return AdapterCommand{}, errors.New("dap: adapter executable is empty")
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return AdapterCommand{}, fmt.Errorf("dap: locate adapter %q: %w", name, err)
	}
	return AdapterCommand{Executable: path}, nil
}

// Validate checks the fields needed to start an adapter.
func (c AdapterCommand) Validate() error {
	if strings.TrimSpace(c.Executable) == "" {
		return errors.New("dap: adapter executable is empty")
	}
	if c.Directory != "" {
		info, err := os.Stat(c.Directory)
		if err != nil {
			return fmt.Errorf("dap: adapter working directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("dap: adapter working directory %q is not a directory", c.Directory)
		}
	}
	return nil
}
