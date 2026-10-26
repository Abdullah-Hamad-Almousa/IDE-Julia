package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
	"github.com/jide-dev/jide/internal/runconf"
	"github.com/jide-dev/jide/internal/testrun"
	jide "github.com/jide-dev/jide/proto/jide"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// StartRunConfiguration starts a registered Julia run configuration and
// streams output as run:output events.
func (a *App) StartRunConfiguration(configuration jide.RunConfiguration) (jide.RunExecuteResult, error) {
	a.mu.Lock()
	session, err := a.currentSession()
	desktopContext := a.ctx
	a.mu.Unlock()
	if err != nil {
		return jide.RunExecuteResult{}, err
	}
	if desktopContext == nil {
		return jide.RunExecuteResult{}, errors.New("jide: desktop runtime is not ready")
	}

	selected := runconf.Configuration{
		ID: configuration.ID, Name: configuration.Name, Type: configuration.Type,
		ProjectPath: configuration.ProjectPath, WorkingDirectory: configuration.WorkingDirectory,
		Program: configuration.Program, Module: configuration.Module, Arguments: configuration.Arguments,
		Environment: configuration.Environment, Properties: configuration.Properties,
	}
	if selected.ProjectPath == "" {
		selected.ProjectPath = session.workspace.ProjectPath()
	}
	selected.ProjectPath, err = appProjectPath(session, selected.ProjectPath)
	if err != nil {
		return jide.RunExecuteResult{}, err
	}
	if selected.WorkingDirectory == "" {
		selected.WorkingDirectory = session.workspace.Root()
	} else {
		selected.WorkingDirectory, err = appWorkspaceDirectory(session, selected.WorkingDirectory)
		if err != nil {
			return jide.RunExecuteResult{}, err
		}
	}
	if selected.Program != "" {
		selected.Program, err = session.filePath(selected.Program)
		if err != nil {
			return jide.RunExecuteResult{}, err
		}
	}

	discoveryContext, cancelDiscovery := context.WithTimeout(context.Background(), 30*time.Second)
	runtime, err := (&jlproc.Discoverer{}).Discover(discoveryContext)
	cancelDiscovery()
	if err != nil {
		return jide.RunExecuteResult{}, fmt.Errorf("jide: discover Julia for run configuration: %w", err)
	}
	runContext, cancelRun := context.WithCancel(context.Background())
	runner := runconf.NewRunner(*runtime, nil)
	process, err := runner.Start(runContext, selected, func(event runconf.Event) {
		if event.Stream == runconf.StreamExit {
			wailsruntime.EventsEmit(desktopContext, "run:exited", jide.RunExitedParams{RunID: event.RunID, ExitCode: event.ExitCode, Error: event.Error})
			return
		}
		wailsruntime.EventsEmit(desktopContext, "run:output", jide.RunOutputParams{RunID: event.RunID, Stream: string(event.Stream), Data: event.Data})
	})
	if err != nil {
		cancelRun()
		return jide.RunExecuteResult{}, err
	}
	a.mu.Lock()
	if a.active != session || a.ctx != desktopContext {
		a.mu.Unlock()
		cancelRun()
		terminateContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = process.Terminate(terminateContext)
		return jide.RunExecuteResult{}, errors.New("jide: workspace changed while starting the run")
	}
	if a.runProcesses == nil {
		a.runProcesses = make(map[string]*runconf.Process)
		a.runCancel = make(map[string]context.CancelFunc)
	}
	a.runProcesses[process.RunID] = process
	a.runCancel[process.RunID] = cancelRun
	a.mu.Unlock()
	go func() {
		_, _ = process.Wait(context.Background())
		a.mu.Lock()
		delete(a.runProcesses, process.RunID)
		delete(a.runCancel, process.RunID)
		a.mu.Unlock()
		cancelRun()
	}()
	return jide.RunExecuteResult{RunID: process.RunID, PID: process.PID}, nil
}

// StopRun terminates an active Julia process by run identifier.
func (a *App) StopRun(runID string) error {
	a.mu.Lock()
	process := a.runProcesses[runID]
	cancel := a.runCancel[runID]
	a.mu.Unlock()
	if process == nil {
		return fmt.Errorf("jide: run %q was not found", runID)
	}
	if cancel != nil {
		cancel()
	}
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	return process.Terminate(ctx)
}

// WriteRunInput sends text to the stdin pipe of an interactive Julia run.
func (a *App) WriteRunInput(runID, input string) error {
	a.mu.Lock()
	process := a.runProcesses[runID]
	a.mu.Unlock()
	if process == nil {
		return fmt.Errorf("jide: run %q was not found", runID)
	}
	return process.WriteInput([]byte(input))
}

// StartTestRun launches a Julia test file and streams Test Explorer updates as
// testrun:event events. Failed test events carry source locations for navigation.
func (a *App) StartTestRun(params jide.TestRunExecuteParams) (jide.TestRunExecuteResult, error) {
	a.mu.Lock()
	session, err := a.currentSession()
	desktopContext := a.ctx
	a.mu.Unlock()
	if err != nil {
		return jide.TestRunExecuteResult{}, err
	}
	if desktopContext == nil {
		return jide.TestRunExecuteResult{}, errors.New("jide: desktop runtime is not ready")
	}
	testFile, err := session.filePath(params.TestFile)
	if err != nil {
		return jide.TestRunExecuteResult{}, err
	}
	if !strings.EqualFold(filepath.Ext(testFile), ".jl") {
		return jide.TestRunExecuteResult{}, errors.New("jide: test file must be a Julia .jl file")
	}
	projectPath := params.ProjectPath
	if projectPath == "" {
		projectPath = session.workspace.ProjectPath()
	}
	projectPath, err = appProjectPath(session, projectPath)
	if err != nil {
		return jide.TestRunExecuteResult{}, err
	}
	workingDirectory := params.WorkingDirectory
	if workingDirectory == "" {
		workingDirectory = session.workspace.Root()
	} else {
		workingDirectory, err = appWorkspaceDirectory(session, workingDirectory)
		if err != nil {
			return jide.TestRunExecuteResult{}, err
		}
	}
	discoveryContext, cancelDiscovery := context.WithTimeout(context.Background(), 30*time.Second)
	runtime, err := (&jlproc.Discoverer{}).Discover(discoveryContext)
	cancelDiscovery()
	if err != nil {
		return jide.TestRunExecuteResult{}, fmt.Errorf("jide: discover Julia for test run: %w", err)
	}
	runID, err := newDesktopRunID()
	if err != nil {
		return jide.TestRunExecuteResult{}, fmt.Errorf("jide: create test run id: %w", err)
	}
	runContext, cancelRun := context.WithCancel(context.Background())
	a.mu.Lock()
	if a.active != session || a.ctx != desktopContext {
		a.mu.Unlock()
		cancelRun()
		return jide.TestRunExecuteResult{}, errors.New("jide: workspace changed while starting the test run")
	}
	if a.testRunCancel == nil {
		a.testRunCancel = make(map[string]context.CancelFunc)
	}
	a.testRunCancel[runID] = cancelRun
	a.mu.Unlock()
	go func() {
		_, runErr := testrun.NewRunner(*runtime).Run(runContext, testrun.Options{
			RunID: runID, ProjectPath: projectPath, WorkingDirectory: workingDirectory,
			TestFile: testFile, Arguments: params.Arguments,
		}, func(event testrun.Event) {
			wailsruntime.EventsEmit(desktopContext, "testrun:event", appTestRunEvent(event))
		})
		if runErr != nil && !errors.Is(runErr, testrun.ErrTestsFailed) {
			wailsruntime.EventsEmit(desktopContext, "testrun:event", jide.TestRunEventParams{
				RunID: runID, Kind: string(testrun.EventFinished), ExitCode: 1, Message: runErr.Error(),
			})
		}
		a.mu.Lock()
		delete(a.testRunCancel, runID)
		a.mu.Unlock()
		cancelRun()
	}()
	return jide.TestRunExecuteResult{RunID: runID}, nil
}

// StopTestRun cancels a test process by run identifier.
func (a *App) StopTestRun(runID string) error {
	a.mu.Lock()
	cancel := a.testRunCancel[runID]
	a.mu.Unlock()
	if cancel == nil {
		return fmt.Errorf("jide: test run %q was not found", runID)
	}
	cancel()
	return nil
}

type appRunState struct {
	processes   map[string]*runconf.Process
	runCancels  map[string]context.CancelFunc
	testCancels map[string]context.CancelFunc
}

func (a *App) takeRunStateLocked() appRunState {
	state := appRunState{processes: a.runProcesses, runCancels: a.runCancel, testCancels: a.testRunCancel}
	a.runProcesses = make(map[string]*runconf.Process)
	a.runCancel = make(map[string]context.CancelFunc)
	a.testRunCancel = make(map[string]context.CancelFunc)
	return state
}

func stopRunState(state appRunState) {
	for _, cancel := range state.runCancels {
		cancel()
	}
	for _, cancel := range state.testCancels {
		cancel()
	}
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	for _, process := range state.processes {
		_ = process.Terminate(ctx)
	}
}

func appProjectPath(session *workspaceSession, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	project := path
	if !filepath.IsAbs(project) {
		project = filepath.Join(session.workspace.Root(), project)
	}
	if filepath.Base(project) != "Project.toml" {
		project = filepath.Join(project, "Project.toml")
	}
	project, err := filepath.Abs(project)
	if err != nil {
		return "", fmt.Errorf("jide: resolve Julia project: %w", err)
	}
	realProject, err := filepath.EvalSymlinks(project)
	if err != nil {
		return "", fmt.Errorf("jide: resolve Julia project link: %w", err)
	}
	if !pathWithin(session.workspace.RealRoot(), realProject) {
		return "", ErrFileOutsideWorkspace
	}
	info, err := os.Stat(project)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("jide: Julia project is not a regular Project.toml")
	}
	return filepath.Dir(project), nil
}

func appWorkspaceDirectory(session *workspaceSession, path string) (string, error) {
	directory := path
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(session.workspace.Root(), directory)
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("jide: resolve working directory: %w", err)
	}
	realDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", fmt.Errorf("jide: resolve working directory link: %w", err)
	}
	if !pathWithin(session.workspace.RealRoot(), realDirectory) {
		return "", ErrFileOutsideWorkspace
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return "", errors.New("jide: working directory is not a directory")
	}
	return directory, nil
}

func appTestRunEvent(event testrun.Event) jide.TestRunEventParams {
	return jide.TestRunEventParams{
		RunID: event.RunID, Kind: string(event.Kind), Name: event.Name, Status: string(event.Status),
		Message: event.Message, SourcePath: event.SourcePath, Line: event.Line,
		Passed: event.Passed, Failed: event.Failed, Errors: event.Errors, Skipped: event.Skipped,
		Broken: event.Broken, ExitCode: event.ExitCode, Stream: event.Stream, Data: event.Data,
	}
}

func newDesktopRunID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
