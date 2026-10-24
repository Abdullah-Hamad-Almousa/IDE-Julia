package jlproc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// ConsoleState is the lifecycle state of a Julia REPL session.
type ConsoleState string

const (
	ConsoleRunning ConsoleState = "running"
	ConsoleClosing ConsoleState = "closing"
	ConsoleExited  ConsoleState = "exited"
	ConsoleClosed  ConsoleState = "closed"
)

// ConsoleOptions selects the project environment and terminal dimensions for
// a new REPL session.
type ConsoleOptions struct {
	// ProjectPath is either the environment directory or its Project.toml.
	// An empty path uses Julia's default environment.
	ProjectPath string
	// WorkingDirectory is the initial current directory. When empty, the
	// project directory is used, or the current process directory if no
	// project was selected.
	WorkingDirectory string
	// Width and Height are the initial terminal dimensions. Zero selects 80x24.
	Width  int
	Height int
}

// ConsoleSession describes a running or archived console session.
type ConsoleSession struct {
	ID               string
	ProjectPath      string
	WorkingDirectory string
	ProcessID        int
	State            ConsoleState
	CreatedAt        time.Time
	ReopenedFrom     string
	History          []string
}

// ConsoleOutput is a chunk of terminal output or a process-exit notification.
type ConsoleOutput struct {
	SessionID string
	Data      []byte
	Exited    bool
	ExitCode  int
	Error     string
}

// ConsoleOutputHandler receives output from a Julia session. Calls for one
// session are serialized; handlers should return promptly to avoid backpressure.
type ConsoleOutputHandler func(ConsoleOutput)

// consoleProcess is the platform-specific PTY process implementation.
type consoleProcess interface {
	io.ReadWriteCloser
	Wait() (int, error)
	PID() int
	Resize(width, height int) error
}

type consoleStarter func(context.Context, Runtime, string, []string, string, int, int) (consoleProcess, error)

type consoleRecord struct {
	info      ConsoleSession
	options   ConsoleOptions
	process   consoleProcess
	writeMu   sync.Mutex
	inputMu   sync.Mutex
	pending   string
	closeOnce sync.Once
	doneOnce  sync.Once
	done      chan struct{}
	closeErr  error
}

// ConsoleManager manages multiple Julia REPL sessions. Closed and exited
// sessions remain in memory so callers can inspect history and reopen them.
type ConsoleManager struct {
	runtime  Runtime
	onOutput ConsoleOutputHandler
	start    consoleStarter

	mu       sync.RWMutex
	sessions map[string]*consoleRecord
	closed   bool
}

// NewConsoleManager returns a manager for runtime. Output events are delivered
// to onOutput; pass nil when the caller does not need streaming notifications.
func NewConsoleManager(runtime Runtime, onOutput ConsoleOutputHandler) *ConsoleManager {
	return &ConsoleManager{
		runtime:  runtime,
		onOutput: onOutput,
		start:    startConsoleProcess,
		sessions: make(map[string]*consoleRecord),
	}
}

// Open starts a Julia REPL in the requested environment.
func (m *ConsoleManager) Open(ctx context.Context, options ConsoleOptions) (ConsoleSession, error) {
	if ctx == nil {
		return ConsoleSession{}, errors.New("jlproc: console context is nil")
	}
	if err := ctx.Err(); err != nil {
		return ConsoleSession{}, err
	}
	if strings.TrimSpace(m.runtime.Executable) == "" {
		return ConsoleSession{}, errors.New("jlproc: Julia executable is empty")
	}
	resolved, err := resolveConsoleOptions(options)
	if err != nil {
		return ConsoleSession{}, err
	}
	width, height := resolved.Width, resolved.Height
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}
	if width < 1 || height < 1 || width > 32767 || height > 32767 {
		return ConsoleSession{}, errors.New("jlproc: console dimensions must be between 1 and 32767")
	}
	args := juliaConsoleArgs(resolved.ProjectPath)
	process, err := m.start(ctx, m.runtime, resolved.ProjectPath, args, resolved.WorkingDirectory, width, height)
	if err != nil {
		return ConsoleSession{}, fmt.Errorf("jlproc: start Julia console: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = process.Close()
		return ConsoleSession{}, err
	}
	id, err := newConsoleID()
	if err != nil {
		_ = process.Close()
		return ConsoleSession{}, fmt.Errorf("jlproc: create console session id: %w", err)
	}
	now := time.Now().UTC()
	record := &consoleRecord{
		info: ConsoleSession{
			ID: id, ProjectPath: resolved.ProjectPath, WorkingDirectory: resolved.WorkingDirectory,
			ProcessID: process.PID(), State: ConsoleRunning, CreatedAt: now, History: []string{},
		},
		options: resolved,
		process: process,
		done:    make(chan struct{}),
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = process.Close()
		return ConsoleSession{}, errors.New("jlproc: console manager is closed")
	}
	m.sessions[id] = record
	m.mu.Unlock()
	go m.pump(record)
	return m.Session(id)
}

// Reopen starts a new REPL using the environment and directory of an existing
// session. The original session and its history are retained; the new session
// starts with a copy of that history and does not replay commands.
func (m *ConsoleManager) Reopen(ctx context.Context, sessionID string) (ConsoleSession, error) {
	if ctx == nil {
		return ConsoleSession{}, errors.New("jlproc: console context is nil")
	}
	m.mu.RLock()
	previous := m.sessions[sessionID]
	if previous == nil {
		m.mu.RUnlock()
		return ConsoleSession{}, fmt.Errorf("jlproc: console session %q was not found", sessionID)
	}
	options := previous.options
	history := slices.Clone(previous.info.History)
	m.mu.RUnlock()
	opened, err := m.Open(ctx, options)
	if err != nil {
		return ConsoleSession{}, err
	}
	m.mu.Lock()
	record := m.sessions[opened.ID]
	record.info.ReopenedFrom = sessionID
	record.info.History = history
	m.mu.Unlock()
	return m.Session(opened.ID)
}

// Write sends terminal input to a session and records completed input lines
// in its in-memory command history.
func (m *ConsoleManager) Write(ctx context.Context, sessionID, input string) error {
	if ctx == nil {
		return errors.New("jlproc: console context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	record, err := m.runningSession(sessionID)
	if err != nil {
		return err
	}
	record.writeMu.Lock()
	defer record.writeMu.Unlock()
	m.mu.RLock()
	state := record.info.State
	m.mu.RUnlock()
	if state != ConsoleRunning {
		return fmt.Errorf("jlproc: console session %q is %s", sessionID, state)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	bytes := []byte(input)
	for len(bytes) > 0 {
		written, writeErr := record.process.Write(bytes)
		if writeErr != nil {
			return fmt.Errorf("jlproc: write to console session %q: %w", sessionID, writeErr)
		}
		if written == 0 {
			return fmt.Errorf("jlproc: write to console session %q: %w", sessionID, io.ErrShortWrite)
		}
		bytes = bytes[written:]
	}
	m.appendConsoleHistory(record, input)
	return nil
}

// Resize changes the terminal dimensions of a running session.
func (m *ConsoleManager) Resize(sessionID string, width, height int) error {
	if width < 1 || height < 1 || width > 32767 || height > 32767 {
		return errors.New("jlproc: console dimensions must be between 1 and 32767")
	}
	record, err := m.runningSession(sessionID)
	if err != nil {
		return err
	}
	record.writeMu.Lock()
	defer record.writeMu.Unlock()
	m.mu.RLock()
	state := record.info.State
	m.mu.RUnlock()
	if state != ConsoleRunning {
		return fmt.Errorf("jlproc: console session %q is %s", sessionID, state)
	}
	if err := record.process.Resize(width, height); err != nil {
		return fmt.Errorf("jlproc: resize console session %q: %w", sessionID, err)
	}
	return nil
}

// Close stops one session. Its metadata and command history remain available.
func (m *ConsoleManager) Close(ctx context.Context, sessionID string) error {
	if ctx == nil {
		return errors.New("jlproc: console context is nil")
	}
	m.mu.RLock()
	record := m.sessions[sessionID]
	m.mu.RUnlock()
	if record == nil {
		return fmt.Errorf("jlproc: console session %q was not found", sessionID)
	}
	m.mu.RLock()
	state := record.info.State
	m.mu.RUnlock()
	if state == ConsoleClosed || state == ConsoleExited {
		return nil
	}
	m.mu.Lock()
	if record.info.State == ConsoleRunning {
		record.info.State = ConsoleClosing
	}
	m.mu.Unlock()
	record.writeMu.Lock()
	record.closeOnce.Do(func() { record.closeErr = record.process.Close() })
	record.writeMu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-recordDone(record):
	}
	m.mu.Lock()
	if record.info.State != ConsoleExited {
		record.info.State = ConsoleClosed
	}
	m.mu.Unlock()
	return record.closeErr
}

// Session returns a copy of one active or archived session.
func (m *ConsoleManager) Session(sessionID string) (ConsoleSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record := m.sessions[sessionID]
	if record == nil {
		return ConsoleSession{}, fmt.Errorf("jlproc: console session %q was not found", sessionID)
	}
	return cloneConsoleSession(record.info), nil
}

// Sessions returns all active and archived sessions sorted by creation time.
func (m *ConsoleManager) Sessions() []ConsoleSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sessions := make([]ConsoleSession, 0, len(m.sessions))
	for _, record := range m.sessions {
		sessions = append(sessions, cloneConsoleSession(record.info))
	}
	slices.SortFunc(sessions, func(a, b ConsoleSession) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return sessions
}

// History returns the commands recorded for one session.
func (m *ConsoleManager) History(sessionID string) ([]string, error) {
	session, err := m.Session(sessionID)
	if err != nil {
		return nil, err
	}
	return slices.Clone(session.History), nil
}

// Shutdown closes every running session and prevents new sessions from being
// opened. It returns the first process cleanup error, if any.
func (m *ConsoleManager) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("jlproc: console context is nil")
	}
	m.mu.Lock()
	m.closed = true
	ids := make([]string, 0, len(m.sessions))
	for id, record := range m.sessions {
		if record.info.State == ConsoleRunning || record.info.State == ConsoleClosing {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	slices.Sort(ids)
	var firstErr error
	for _, id := range ids {
		if err := m.Close(ctx, id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return errors.Join(firstErr, ctx.Err())
}

func (m *ConsoleManager) pump(record *consoleRecord) {
	buffer := make([]byte, 4096)
	for {
		count, err := record.process.Read(buffer)
		if count > 0 && m.onOutput != nil {
			m.onOutput(ConsoleOutput{SessionID: record.info.ID, Data: slices.Clone(buffer[:count])})
		}
		if err != nil {
			m.mu.RLock()
			closing := record.info.State == ConsoleClosing || record.info.State == ConsoleClosed
			m.mu.RUnlock()
			if !errors.Is(err, io.EOF) && !closing && m.onOutput != nil {
				m.onOutput(ConsoleOutput{SessionID: record.info.ID, Error: err.Error()})
			}
			break
		}
	}
	exitCode, waitErr := record.process.Wait()
	record.closeOnce.Do(func() { record.closeErr = record.process.Close() })
	m.mu.Lock()
	if record.info.State == ConsoleClosing {
		record.info.State = ConsoleClosed
	} else if record.info.State != ConsoleClosed {
		record.info.State = ConsoleExited
	}
	m.mu.Unlock()
	if m.onOutput != nil {
		event := ConsoleOutput{SessionID: record.info.ID, Exited: true, ExitCode: exitCode}
		if waitErr != nil {
			event.Error = waitErr.Error()
		}
		m.onOutput(event)
	}
	closeRecordDone(record)
}

func (m *ConsoleManager) runningSession(sessionID string) (*consoleRecord, error) {
	m.mu.RLock()
	record := m.sessions[sessionID]
	m.mu.RUnlock()
	if record == nil {
		return nil, fmt.Errorf("jlproc: console session %q was not found", sessionID)
	}
	m.mu.RLock()
	state := record.info.State
	m.mu.RUnlock()
	if state != ConsoleRunning {
		return nil, fmt.Errorf("jlproc: console session %q is %s", sessionID, state)
	}
	return record, nil
}

func resolveConsoleOptions(options ConsoleOptions) (ConsoleOptions, error) {
	if strings.TrimSpace(options.ProjectPath) != "" {
		projectPath, err := filepath.Abs(options.ProjectPath)
		if err != nil {
			return ConsoleOptions{}, fmt.Errorf("jlproc: resolve project path: %w", err)
		}
		if filepath.Base(projectPath) == "Project.toml" {
			info, err := os.Stat(projectPath)
			if err != nil {
				return ConsoleOptions{}, fmt.Errorf("jlproc: stat project file %q: %w", projectPath, err)
			}
			if !info.Mode().IsRegular() {
				return ConsoleOptions{}, fmt.Errorf("jlproc: project file %q is not a regular file", projectPath)
			}
			projectPath = filepath.Dir(projectPath)
		} else {
			info, err := os.Stat(filepath.Join(projectPath, "Project.toml"))
			if err != nil {
				return ConsoleOptions{}, fmt.Errorf("jlproc: locate Project.toml in %q: %w", projectPath, err)
			}
			if !info.Mode().IsRegular() {
				return ConsoleOptions{}, fmt.Errorf("jlproc: project file in %q is not a regular file", projectPath)
			}
		}
		options.ProjectPath = projectPath
	}
	if strings.TrimSpace(options.WorkingDirectory) == "" {
		options.WorkingDirectory = options.ProjectPath
	}
	if options.WorkingDirectory == "" {
		current, err := os.Getwd()
		if err != nil {
			return ConsoleOptions{}, fmt.Errorf("jlproc: get working directory: %w", err)
		}
		options.WorkingDirectory = current
	}
	workingDirectory, err := filepath.Abs(options.WorkingDirectory)
	if err != nil {
		return ConsoleOptions{}, fmt.Errorf("jlproc: resolve working directory: %w", err)
	}
	info, err := os.Stat(workingDirectory)
	if err != nil {
		return ConsoleOptions{}, fmt.Errorf("jlproc: stat working directory %q: %w", workingDirectory, err)
	}
	if !info.IsDir() {
		return ConsoleOptions{}, fmt.Errorf("jlproc: working directory %q is not a directory", workingDirectory)
	}
	options.WorkingDirectory = workingDirectory
	return options, nil
}

func juliaConsoleArgs(projectPath string) []string {
	args := []string{"--startup-file=no", "--history-file=yes", "--color=no", "--interactive"}
	if projectPath != "" {
		args = append(args, "--project="+projectPath)
	}
	return args
}

func newConsoleID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func cloneConsoleSession(session ConsoleSession) ConsoleSession {
	session.History = slices.Clone(session.History)
	return session
}

func (m *ConsoleManager) appendConsoleHistory(record *consoleRecord, input string) {
	record.inputMu.Lock()
	defer record.inputMu.Unlock()
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	record.pending += input
	lines := strings.Split(record.pending, "\n")
	record.pending = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		m.mu.Lock()
		record.info.History = append(record.info.History, line)
		m.mu.Unlock()
	}
}

func (record *consoleRecord) markClosed() {
	record.doneOnce.Do(func() { close(record.done) })
}

func closeRecordDone(record *consoleRecord) {
	record.markClosed()
}

func recordDone(record *consoleRecord) <-chan struct{} {
	return record.done
}
