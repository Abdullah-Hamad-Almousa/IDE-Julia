//go:build windows

package jlproc

import (
	"context"
	"fmt"

	"github.com/UserExistsError/conpty"
	"golang.org/x/sys/windows"
)

type windowsConsoleProcess struct {
	terminal      *conpty.ConPty
	processHandle windows.Handle
}

func startConsoleProcess(_ context.Context, runtime Runtime, _ string, args []string, directory string, width, height int) (consoleProcess, error) {
	commandLine := windows.ComposeCommandLine(append([]string{runtime.Executable}, args...))
	terminal, err := conpty.Start(commandLine,
		conpty.ConPtyWorkDir(directory),
		conpty.ConPtyDimensions(width, height),
	)
	if err != nil {
		return nil, err
	}
	processHandle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(terminal.Pid()))
	if err != nil {
		_ = terminal.Close()
		return nil, fmt.Errorf("open Julia console process handle: %w", err)
	}
	return &windowsConsoleProcess{terminal: terminal, processHandle: processHandle}, nil
}

func (p *windowsConsoleProcess) Read(buffer []byte) (int, error) { return p.terminal.Read(buffer) }

func (p *windowsConsoleProcess) Write(buffer []byte) (int, error) { return p.terminal.Write(buffer) }

func (p *windowsConsoleProcess) Close() error { return p.terminal.Close() }

func (p *windowsConsoleProcess) Wait() (int, error) {
	defer func() { _ = windows.CloseHandle(p.processHandle) }()
	_, err := windows.WaitForSingleObject(p.processHandle, windows.INFINITE)
	if err != nil {
		return -1, fmt.Errorf("wait for Julia console: %w", err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.processHandle, &code); err != nil {
		return -1, fmt.Errorf("read Julia console exit code: %w", err)
	}
	return int(code), nil
}

func (p *windowsConsoleProcess) PID() int { return p.terminal.Pid() }

func (p *windowsConsoleProcess) Resize(width, height int) error {
	return p.terminal.Resize(width, height)
}
