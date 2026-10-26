package runconf

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
)

// Stream identifies a subprocess output stream or process lifecycle event.
type Stream string

const (
	StreamStdout Stream = "stdout"
	StreamStderr Stream = "stderr"
	StreamExit   Stream = "exit"
)

// Event is a streamed run-process output or exit notification.
type Event struct {
	RunID    string
	Stream   Stream
	Data     string
	ExitCode int
	Error    string
}

// EventHandler receives process events. Calls are serialized per process;
// handlers should return promptly to avoid blocking output draining.
type EventHandler func(Event)

// Result describes a completed Julia process.
type Result struct {
	RunID    string
	PID      int
	ExitCode int
	Started  time.Time
	Finished time.Time
	Error    string
}

type processRecord struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	writeMu sync.Mutex
	eventMu sync.Mutex
	handler EventHandler
	done    chan struct{}
	result  Result
	waitErr error
}

// Process is a handle to a started run configuration.
type Process struct {
	owner  *Runner
	RunID  string
	PID    int
	record *processRecord
}

// Runner starts and supervises Julia run configurations.
type Runner struct {
	runtime jlproc.Runtime
	types   *Registry
	mu      sync.RWMutex
	runs    map[string]*processRecord
}

// NewRunner creates a process runner. A nil registry selects the built-ins.
func NewRunner(runtime jlproc.Runtime, registry *Registry) *Runner {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Runner{runtime: runtime, types: registry, runs: make(map[string]*processRecord)}
}

// Start resolves and starts config, then streams stdout, stderr, and exit state.
func (r *Runner) Start(ctx context.Context, config Configuration, handler EventHandler) (*Process, error) {
	if r == nil {
		return nil, errors.New("runconf: runner is nil")
	}
	if ctx == nil {
		return nil, errors.New("runconf: process context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	invocation, err := r.types.Build(r.runtime, config)
	if err != nil {
		return nil, err
	}
	runID, err := newRunID()
	if err != nil {
		return nil, fmt.Errorf("runconf: create run id: %w", err)
	}
	r.mu.RLock()
	_, exists := r.runs[runID]
	r.mu.RUnlock()
	if exists {
		return nil, fmt.Errorf("runconf: run id %q is already in use", runID)
	}

	command := exec.CommandContext(ctx, invocation.Executable, invocation.Arguments...)
	command.Dir = invocation.WorkingDirectory
	if len(invocation.Environment) > 0 {
		command.Env = mergeEnvironment(os.Environ(), invocation.Environment)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("runconf: create stdin pipe: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("runconf: create stdout pipe: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("runconf: create stderr pipe: %w", err)
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("runconf: start Julia: %w", err)
	}
	record := &processRecord{
		command: command, stdin: stdin, handler: handler, done: make(chan struct{}),
		result: Result{RunID: runID, PID: command.Process.Pid, Started: time.Now().UTC()},
	}
	r.mu.Lock()
	if _, exists := r.runs[runID]; exists {
		r.mu.Unlock()
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("runconf: run id %q is already in use", runID)
	}
	r.runs[runID] = record
	r.mu.Unlock()
	var outputReaders sync.WaitGroup
	outputReaders.Add(2)
	go func() {
		defer outputReaders.Done()
		streamProcessOutput(stdout, func(data string) { record.emit(Event{RunID: runID, Stream: StreamStdout, Data: data}) })
	}()
	go func() {
		defer outputReaders.Done()
		streamProcessOutput(stderr, func(data string) { record.emit(Event{RunID: runID, Stream: StreamStderr, Data: data}) })
	}()
	go func() {
		waitErr := command.Wait()
		outputReaders.Wait()
		_ = stdin.Close()
		result := record.result
		result.Finished = time.Now().UTC()
		if command.ProcessState != nil {
			result.ExitCode = command.ProcessState.ExitCode()
		}
		if waitErr != nil {
			result.Error = waitErr.Error()
		}
		record.eventMu.Lock()
		record.result = result
		record.waitErr = waitErr
		record.eventMu.Unlock()
		close(record.done)
		r.mu.Lock()
		if r.runs[runID] == record {
			delete(r.runs, runID)
		}
		r.mu.Unlock()
		record.emit(Event{RunID: runID, Stream: StreamExit, ExitCode: result.ExitCode, Error: result.Error})
	}()
	return &Process{owner: r, RunID: runID, PID: command.Process.Pid, record: record}, nil
}

// Process returns an active process handle by run id.
func (r *Runner) Process(runID string) (*Process, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	record, exists := r.runs[runID]
	r.mu.RUnlock()
	if !exists {
		return nil, false
	}
	return &Process{owner: r, RunID: runID, PID: record.result.PID, record: record}, true
}

// WriteInput writes bytes to a process's stdin, useful for REPL configurations.
func (p *Process) WriteInput(data []byte) error {
	if p == nil || p.record == nil {
		return errors.New("runconf: process handle is nil")
	}
	select {
	case <-p.record.done:
		return errors.New("runconf: process has exited")
	default:
	}
	p.record.writeMu.Lock()
	defer p.record.writeMu.Unlock()
	if _, err := p.record.stdin.Write(data); err != nil {
		return fmt.Errorf("runconf: write process input: %w", err)
	}
	return nil
}

// Wait waits for process exit or ctx cancellation and returns its final result.
func (p *Process) Wait(ctx context.Context) (Result, error) {
	if p == nil || p.record == nil {
		return Result{}, errors.New("runconf: process handle is nil")
	}
	if ctx == nil {
		return Result{}, errors.New("runconf: wait context is nil")
	}
	select {
	case <-p.record.done:
		p.record.eventMu.Lock()
		result := p.record.result
		p.record.eventMu.Unlock()
		return result, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

// Terminate kills the process and waits for its output streams to drain.
func (p *Process) Terminate(ctx context.Context) error {
	if p == nil || p.record == nil {
		return errors.New("runconf: process handle is nil")
	}
	select {
	case <-p.record.done:
		return nil
	default:
	}
	if err := p.record.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("runconf: terminate process: %w", err)
	}
	_, err := p.Wait(ctx)
	return err
}

// Terminate stops a run by id.
func (r *Runner) Terminate(ctx context.Context, runID string) error {
	process, exists := r.Process(runID)
	if !exists {
		return fmt.Errorf("runconf: run %q was not found", runID)
	}
	return process.Terminate(ctx)
}

// Close terminates all known runs and waits for their streams to finish.
func (r *Runner) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	processes := make([]*Process, 0, len(r.runs))
	for id, record := range r.runs {
		processes = append(processes, &Process{owner: r, RunID: id, PID: record.result.PID, record: record})
	}
	r.mu.RUnlock()
	var failures []error
	for _, process := range processes {
		if err := process.Terminate(ctx); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (record *processRecord) emit(event Event) {
	if record.handler == nil {
		return
	}
	record.eventMu.Lock()
	defer record.eventMu.Unlock()
	record.handler(event)
}

func streamProcessOutput(reader io.Reader, send func(string)) {
	buffered := bufio.NewReader(reader)
	for {
		line, err := buffered.ReadString('\n')
		if line != "" {
			send(line)
		}
		if err != nil {
			return
		}
	}
}

func mergeEnvironment(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		key, value, found := strings.Cut(entry, "=")
		if found {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func newRunID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
