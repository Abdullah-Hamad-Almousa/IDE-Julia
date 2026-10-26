package testrun

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
)

const eventPrefix = "JIDE_TEST_EVENT_V1"

// EventKind identifies a test-run lifecycle, output, test-set, or test result.
type EventKind string

const (
	EventStarted       EventKind = "started"
	EventOutput        EventKind = "output"
	EventTestSetStart  EventKind = "testset-start"
	EventTestSetFinish EventKind = "testset-finish"
	EventTestResult    EventKind = "test"
	EventFinished      EventKind = "finished"
)

// Status describes an individual Julia Test result.
type Status string

const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusError   Status = "error"
	StatusSkipped Status = "skipped"
	StatusBroken  Status = "broken"
)

// Options selects a Julia project, test file, and test working directory.
type Options struct {
	RunID            string
	ProjectPath      string
	WorkingDirectory string
	TestFile         string
	Arguments        []string
}

// Event is a streamed Test Explorer update. SourcePath and Line identify a
// failing assertion for editor navigation.
type Event struct {
	RunID      string
	Kind       EventKind
	Name       string
	Status     Status
	Message    string
	SourcePath string
	Line       int
	Passed     int
	Failed     int
	Errors     int
	Skipped    int
	Broken     int
	ExitCode   int
	Stream     string
	Data       string
}

// EventHandler receives updates while the Julia test process is running.
// Calls are serialized; handlers should return promptly to avoid backpressure.
type EventHandler func(Event)

// Result summarizes a completed test run.
type Result struct {
	RunID    string
	ExitCode int
	Passed   int
	Failed   int
	Errors   int
	Skipped  int
	Broken   int
	Started  time.Time
	Finished time.Time
}

// ErrTestsFailed is returned after a completed run with failed/error tests.
var ErrTestsFailed = errors.New("testrun: one or more Julia tests failed")

// Runner runs Julia test files and translates Julia Test results into events.
type Runner struct {
	runtime jlproc.Runtime
}

// NewRunner creates a test runner for runtime.
func NewRunner(runtime jlproc.Runtime) *Runner {
	return &Runner{runtime: runtime}
}

// Run executes the test file and streams test-set, assertion, and output
// events. A test failure is a completed run and returns its Result together
// with ErrTestsFailed; process/startup errors return a zero Result.
func (r *Runner) Run(ctx context.Context, options Options, handler EventHandler) (Result, error) {
	if r == nil {
		return Result{}, errors.New("testrun: runner is nil")
	}
	if ctx == nil {
		return Result{}, errors.New("testrun: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(r.runtime.Executable) == "" {
		return Result{}, errors.New("testrun: Julia executable is empty")
	}
	testFile, err := filepath.Abs(options.TestFile)
	if err != nil {
		return Result{}, fmt.Errorf("testrun: resolve test file: %w", err)
	}
	info, err := os.Stat(testFile)
	if err != nil {
		return Result{}, fmt.Errorf("testrun: stat test file: %w", err)
	}
	if !info.Mode().IsRegular() || strings.ToLower(filepath.Ext(testFile)) != ".jl" {
		return Result{}, errors.New("testrun: test file must be a regular .jl file")
	}
	projectPath, err := normalizeProject(options.ProjectPath)
	if err != nil {
		return Result{}, err
	}
	workingDirectory := options.WorkingDirectory
	if workingDirectory == "" {
		workingDirectory = filepath.Dir(testFile)
	}
	workingDirectory, err = filepath.Abs(workingDirectory)
	if err != nil {
		return Result{}, fmt.Errorf("testrun: resolve working directory: %w", err)
	}
	info, err = os.Stat(workingDirectory)
	if err != nil || !info.IsDir() {
		if err != nil {
			return Result{}, fmt.Errorf("testrun: stat working directory: %w", err)
		}
		return Result{}, errors.New("testrun: working directory is not a directory")
	}
	runID := strings.TrimSpace(options.RunID)
	if runID == "" {
		runID, err = newID()
		if err != nil {
			return Result{}, fmt.Errorf("testrun: create run id: %w", err)
		}
	}
	tempDir, err := os.MkdirTemp("", "jide-testrun-")
	if err != nil {
		return Result{}, fmt.Errorf("testrun: create harness directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	harnessPath := filepath.Join(tempDir, "testrun.jl")
	if err := os.WriteFile(harnessPath, []byte(juliaHarness), 0o600); err != nil {
		return Result{}, fmt.Errorf("testrun: write Julia harness: %w", err)
	}
	arguments := []string{"--startup-file=no", "--history-file=no", "--color=no"}
	if projectPath != "" {
		arguments = append(arguments, "--project="+projectPath)
	}
	arguments = append(arguments, harnessPath, testFile)
	arguments = append(arguments, options.Arguments...)
	command := exec.CommandContext(ctx, r.runtime.Executable, arguments...)
	command.Dir = workingDirectory
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("testrun: create stdout pipe: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return Result{}, fmt.Errorf("testrun: create stderr pipe: %w", err)
	}
	if err := command.Start(); err != nil {
		return Result{}, fmt.Errorf("testrun: start Julia: %w", err)
	}
	result := Result{RunID: runID, Started: time.Now().UTC()}
	var eventMu sync.Mutex
	emit := func(event Event) {
		if handler == nil {
			return
		}
		event.RunID = runID
		eventMu.Lock()
		defer eventMu.Unlock()
		handler(event)
	}
	emit(Event{Kind: EventStarted, Name: filepath.Base(testFile)})
	var countersMu sync.Mutex
	var streamErrors []error
	var streams sync.WaitGroup
	streams.Add(2)
	go func() {
		defer streams.Done()
		if streamErr := readStdoutEvents(stdout, testFile, workingDirectory, emit, &result, &countersMu); streamErr != nil {
			countersMu.Lock()
			streamErrors = append(streamErrors, streamErr)
			countersMu.Unlock()
		}
	}()
	go func() {
		defer streams.Done()
		if streamErr := readOutput(stderr, "stderr", emit); streamErr != nil {
			countersMu.Lock()
			streamErrors = append(streamErrors, streamErr)
			countersMu.Unlock()
		}
	}()
	waitErr := command.Wait()
	streams.Wait()
	result.Finished = time.Now().UTC()
	if command.ProcessState != nil {
		result.ExitCode = command.ProcessState.ExitCode()
	}
	countersMu.Lock()
	combinedStreamErr := errors.Join(streamErrors...)
	countersMu.Unlock()
	if waitErr != nil && ctx.Err() != nil {
		return result, ctx.Err()
	}
	if combinedStreamErr != nil {
		return result, fmt.Errorf("testrun: read Julia output: %w", combinedStreamErr)
	}
	if waitErr != nil && result.ExitCode != 0 && result.Failed+result.Errors == 0 {
		return result, fmt.Errorf("testrun: Julia exited with code %d: %w", result.ExitCode, waitErr)
	}
	if result.Failed+result.Errors > 0 && result.ExitCode == 0 {
		result.ExitCode = 1
	}
	emit(Event{Kind: EventFinished, ExitCode: result.ExitCode, Passed: result.Passed, Failed: result.Failed,
		Errors: result.Errors, Skipped: result.Skipped, Broken: result.Broken})
	if result.Failed+result.Errors > 0 {
		return result, ErrTestsFailed
	}
	if waitErr != nil {
		return result, fmt.Errorf("testrun: Julia exited with code %d: %w", result.ExitCode, waitErr)
	}
	return result, nil
}

func normalizeProject(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("testrun: resolve Julia project: %w", err)
	}
	if filepath.Base(absolute) != "Project.toml" {
		absolute = filepath.Join(absolute, "Project.toml")
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("testrun: stat Julia project: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("testrun: project path is not a regular Project.toml")
	}
	return filepath.Dir(absolute), nil
}

func readStdoutEvents(reader io.Reader, testFile, workingDirectory string, emit func(Event), result *Result, countersMu *sync.Mutex) error {
	buffered := bufio.NewReader(reader)
	for {
		line, err := buffered.ReadString('\n')
		if line != "" {
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if strings.HasPrefix(line, eventPrefix+"\t") {
				event, parseErr := parseHarnessEvent(line, testFile, workingDirectory)
				if parseErr != nil {
					return parseErr
				}
				countersMu.Lock()
				countResult(result, event)
				countersMu.Unlock()
				emit(event)
			} else {
				emit(Event{Kind: EventOutput, Stream: "stdout", Data: line + "\n"})
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func readOutput(reader io.Reader, stream string, emit func(Event)) error {
	buffered := bufio.NewReader(reader)
	for {
		line, err := buffered.ReadString('\n')
		if line != "" {
			emit(Event{Kind: EventOutput, Stream: stream, Data: line})
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func parseHarnessEvent(line, testFile, workingDirectory string) (Event, error) {
	fields := strings.Split(line, "\t")
	if len(fields) < 2 || fields[0] != eventPrefix {
		return Event{}, errors.New("invalid Julia test event prefix")
	}
	values := make([]string, 0, len(fields)-2)
	for _, field := range fields[2:] {
		decoded, err := base64.StdEncoding.DecodeString(field)
		if err != nil {
			return Event{}, fmt.Errorf("decode Julia test event: %w", err)
		}
		values = append(values, string(decoded))
	}
	get := func(index int) string {
		if index < 0 || index >= len(values) {
			return ""
		}
		return values[index]
	}
	integer := func(index int) int {
		value, _ := strconv.Atoi(get(index))
		return value
	}
	event := Event{Kind: EventKind(fields[1])}
	switch event.Kind {
	case EventTestSetStart:
		event.Name = get(0)
	case EventTestSetFinish:
		event.Name = get(0)
		event.Passed, event.Failed, event.Errors, event.Skipped, event.Broken = integer(1), integer(2), integer(3), integer(4), integer(5)
	case EventTestResult:
		event.Status = Status(get(0))
		event.Name = get(1)
		event.SourcePath = get(2)
		event.Line = integer(3)
		event.Message = get(4)
		if event.SourcePath == "" || event.SourcePath == "none" {
			event.SourcePath = testFile
		} else if !filepath.IsAbs(event.SourcePath) {
			event.SourcePath = filepath.Join(workingDirectory, event.SourcePath)
		}
		if absolute, err := filepath.Abs(event.SourcePath); err == nil {
			event.SourcePath = absolute
		}
	case EventOutput:
		event.Stream = get(0)
		event.Data = get(1)
	default:
		return Event{}, fmt.Errorf("unknown Julia test event %q", fields[1])
	}
	return event, nil
}

func countResult(result *Result, event Event) {
	if event.Kind != EventTestResult {
		return
	}
	switch event.Status {
	case StatusPassed:
		result.Passed++
	case StatusFailed:
		result.Failed++
	case StatusError:
		result.Errors++
	case StatusSkipped:
		result.Skipped++
	case StatusBroken:
		result.Broken++
	}
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

const juliaHarness = `using Base64, Test

const JIDE_TEST_PREFIX = "JIDE_TEST_EVENT_V1"
const JIDE_TEST_FILE = popfirst!(ARGS)

function jide_encode(value)
    base64encode(codeunits(string(value)))
end

function jide_emit(kind, values...)
    print(stdout, JIDE_TEST_PREFIX, '\t', kind)
    for value in values
        print(stdout, '\t', jide_encode(value))
    end
    println(stdout)
    flush(stdout)
end

function jide_status(result)
    result isa Test.Pass && return "passed"
    result isa Test.Fail && return "failed"
    result isa Test.Error && return "error"
    if result isa Test.Broken
        return result.test_type == :skipped ? "skipped" : "broken"
    end
    return "error"
end

function jide_source(result)
    if hasproperty(result, :source)
        source = getproperty(result, :source)
        if source isa LineNumberNode
            return string(source.file), source.line
        end
    end
    return "", 0
end

function jide_result(result)
    source, line = jide_source(result)
    name = hasproperty(result, :orig_expr) ? sprint(show, getproperty(result, :orig_expr)) : string(typeof(result))
    message = sprint(show, result)
    jide_emit("test", jide_status(result), name, source, line, message)
end

function jide_counts(results)
    counts = Dict("passed" => 0, "failed" => 0, "error" => 0, "skipped" => 0, "broken" => 0)
    for result in results
        if result isa Test.AbstractTestSet
            child = jide_counts(result.results)
            for key in keys(counts)
                counts[key] += child[key]
            end
        else
            status = jide_status(result)
            haskey(counts, status) && (counts[status] += 1)
        end
    end
    counts
end

function jide_emit_default_testset(testset)
    name = string(testset.description)
    jide_emit("testset-start", name)
    for result in testset.results
        if result isa Test.AbstractTestSet
            jide_emit_default_testset(result)
        else
            jide_result(result)
        end
    end
    counts = jide_counts(testset.results)
    jide_emit("testset-finish", name, counts["passed"], counts["failed"], counts["error"], counts["skipped"], counts["broken"])
end

struct JideTestSet <: Test.AbstractTestSet
    description::String
    results::Vector{Any}
end

function JideTestSet(description; kwargs...)
    name = string(description)
    jide_emit("testset-start", name)
    JideTestSet(name, Any[])
end

function Test.record(testset::JideTestSet, result::Test.Result)
    push!(testset.results, result)
    jide_result(result)
    result
end

function Test.record(testset::JideTestSet, child)
    push!(testset.results, child)
    if child isa Test.AbstractTestSet
        jide_emit_default_testset(child)
    else
        jide_result(child)
    end
    child
end

function Test.finish(testset::JideTestSet)
    counts = jide_counts(testset.results)
    jide_emit("testset-finish", testset.description, counts["passed"], counts["failed"], counts["error"], counts["skipped"], counts["broken"])
    testset
end

Test.@testset JideTestSet "Julia test run" begin
    include(JIDE_TEST_FILE)
end
`
