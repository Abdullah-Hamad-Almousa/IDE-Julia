package pkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jide-dev/jide/internal/jlproc"
)

// Command describes one Julia subprocess invocation.
type Command struct {
	Executable string
	Args       []string
	Dir        string
}

// Runner executes one Julia command and returns its combined output.
//
// A runner can be supplied to Manager for deterministic tests and embedding.
type Runner func(ctx context.Context, command Command) ([]byte, error)

// Manager runs Pkg operations for a selected Julia runtime.
type Manager struct {
	Runtime jlproc.Runtime
	Runner  Runner
}

// NewManager returns a package manager for runtime.
func NewManager(runtime jlproc.Runtime) *Manager {
	return &Manager{Runtime: runtime}
}

// run executes command through the configured runner or the operating system.
func (m *Manager) run(ctx context.Context, command Command) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("pkg: context is nil")
	}
	if m.Runner != nil {
		return m.Runner(ctx, command)
	}
	process := exec.CommandContext(ctx, command.Executable, command.Args...)
	process.Dir = command.Dir
	var stdout bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stdout
	if err := process.Run(); err != nil {
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
}

func (m *Manager) juliaCommand(dir, script string, args ...string) (Command, error) {
	if strings.TrimSpace(m.Runtime.Executable) == "" {
		return Command{}, errors.New("pkg: Julia executable is empty")
	}
	commandArgs := []string{"--startup-file=no", "--history-file=no", "--color=no"}
	if dir != "" {
		commandArgs = append(commandArgs, "--project="+dir)
	}
	commandArgs = append(commandArgs, "-e", script)
	if len(args) > 0 {
		commandArgs = append(commandArgs, "--")
		commandArgs = append(commandArgs, args...)
	}
	return Command{Executable: m.Runtime.Executable, Args: commandArgs, Dir: dir}, nil
}

func (m *Manager) runPkg(ctx context.Context, projectPath, script string, args ...string) ([]byte, error) {
	directory, err := projectDirectory(projectPath)
	if err != nil {
		return nil, err
	}
	command, err := m.juliaCommand(directory, script, append([]string{directory}, args...)...)
	if err != nil {
		return nil, err
	}
	output, err := m.run(ctx, command)
	if err != nil {
		return output, fmt.Errorf("pkg: run Pkg in %q: %w: %s", directory, err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func projectDirectory(projectPath string) (string, error) {
	if strings.TrimSpace(projectPath) == "" {
		return "", errors.New("pkg: project path is empty")
	}
	absolute, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("pkg: resolve project path %q: %w", projectPath, err)
	}
	if filepath.Base(absolute) == "Project.toml" {
		projectPath = absolute
		absolute = filepath.Dir(absolute)
	} else {
		projectPath = filepath.Join(absolute, "Project.toml")
	}
	info, err := os.Stat(projectPath)
	if err != nil {
		return "", fmt.Errorf("pkg: locate project file %q: %w", projectPath, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("pkg: project file %q is not a regular file", projectPath)
	}
	return absolute, nil
}
