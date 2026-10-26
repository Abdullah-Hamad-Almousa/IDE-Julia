package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
	"github.com/jide-dev/jide/internal/runconf"
	jide "github.com/jide-dev/jide/proto/jide"
)

func TestRunDispatcherValidatesTypedRunRequests(t *testing.T) {
	dispatcher := NewRunDispatcher(jlproc.Runtime{}, nil)
	t.Cleanup(func() { _ = dispatcher.Close(context.Background()) })
	if err := dispatcher.RegisterRunConfigurationType(runconf.ConfigType{
		ID: "user-custom", Validate: func(runconf.Configuration) error { return nil },
		Build: func(_ jlproc.Runtime, _ runconf.Configuration) (runconf.Invocation, error) {
			return runconf.Invocation{Executable: "custom"}, nil
		},
	}); err != nil {
		t.Fatalf("register custom run type: %v", err)
	}
	if _, err := dispatcher.Handle(context.Background(), jide.MethodRunExecute, json.RawMessage(`{"configuration":{"name":"bad","type":"julia-script"}}`)); err == nil {
		t.Fatal("run/execute accepted a script without a source file")
	}
	if _, err := dispatcher.Handle(context.Background(), jide.MethodRunTerminate, json.RawMessage(`{"runId":"missing"}`)); err == nil {
		t.Fatal("run/terminate accepted an unknown run identifier")
	}
	if _, err := dispatcher.Handle(context.Background(), jide.MethodRunInput, json.RawMessage(`{"runId":"missing","input":"x"}`)); err == nil {
		t.Fatal("run/input accepted an unknown run identifier")
	}
	if _, err := dispatcher.Handle(context.Background(), jide.MethodRunExecute, json.RawMessage(`{"configuration":42}`)); err == nil || !strings.Contains(err.Error(), "decode run parameters") {
		t.Fatalf("malformed run parameters error = %v", err)
	}
}

func TestRunDispatcherStreamsTestStartFailureAsAnEvent(t *testing.T) {
	events := make(chan jide.TestRunEventParams, 4)
	dispatcher := NewRunDispatcher(jlproc.Runtime{}, func(method jide.Method, params any) {
		if method == jide.MethodTestRunEvent {
			if event, ok := params.(jide.TestRunEventParams); ok {
				events <- event
			}
		}
	})
	t.Cleanup(func() { _ = dispatcher.Close(context.Background()) })
	result, err := dispatcher.Handle(context.Background(), jide.MethodTestRunExecute, json.RawMessage(`{"testFile":"missing.jl"}`))
	if err != nil {
		t.Fatal(err)
	}
	started, ok := result.(jide.TestRunExecuteResult)
	if !ok || started.RunID == "" {
		t.Fatalf("testrun/execute result = %#v", result)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case event := <-events:
		if event.RunID != started.RunID || event.Kind != "finished" || event.ExitCode != 1 || event.Message == "" {
			t.Fatalf("test-run start failure event = %+v", event)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for the test-run failure event")
	}
}
