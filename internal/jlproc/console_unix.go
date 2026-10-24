//go:build !windows

package jlproc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type unixConsoleProcess struct {
	command  *exec.Cmd
	terminal *os.File
}

func startConsoleProcess(ctx context.Context, runtime Runtime, _ string, args []string, directory string, width, height int) (consoleProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command := exec.Command(runtime.Executable, args...)
	command.Dir = directory
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)})
	if err != nil {
		return nil, err
	}
	return &unixConsoleProcess{command: command, terminal: terminal}, nil
}

func (p *unixConsoleProcess) Read(buffer []byte) (int, error) { return p.terminal.Read(buffer) }

func (p *unixConsoleProcess) Write(buffer []byte) (int, error) { return p.terminal.Write(buffer) }

func (p *unixConsoleProcess) Close() error {
	var firstErr error
	if p.command.Process != nil {
		if err := p.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			firstErr = err
		}
	}
	if err := p.terminal.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (p *unixConsoleProcess) Wait() (int, error) {
	err := p.command.Wait()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return -1, fmt.Errorf("wait for Julia console: %w", err)
}

func (p *unixConsoleProcess) PID() int {
	if p.command.Process == nil {
		return 0
	}
	return p.command.Process.Pid
}

func (p *unixConsoleProcess) Resize(width, height int) error {
	return pty.Setsize(p.terminal, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)})
}
