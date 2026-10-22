package lsp

import (
	"context"
	"testing"
	"time"

	jide "github.com/jide-dev/jide/proto/jide"
)

// TestClientEndToEnd drives the client with the fake language server.
//
// The test exercises the whole flow of a session: the initialize handshake,
// the synchronization of a document, the diagnostics that the server
// publishes and the four requests of the semantic analysis.
func TestClientEndToEnd(t *testing.T) {
	directory := t.TempDir()
	command, logPath := fakeServerCommand(t, directory)

	published := make(chan string, 8)
	client := New(Options{
		Command: command,
		RootURI: "file:///workspace",
		OnDiagnostics: func(uri string, _ []jide.LSPDiagnostic) {
			select {
			case published <- uri:
			default:
			}
		},
	})
	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Stop(context.Background()) })

	if got, want := client.State(), StateRunning; got != want {
		t.Errorf("State() = %q, want %q", got, want)
	}

	uri := "file:///workspace/main.jl"
	if err := client.DidOpen(ctx, uri, "julia", "f(x) = x + 1\n", 1); err != nil {
		t.Fatalf("DidOpen() error = %v", err)
	}

	select {
	case got := <-published:
		if got != uri {
			t.Errorf("the diagnostics were published for %q, want %q", got, uri)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the language server did not publish diagnostics")
	}

	diagnostics := client.Diagnostics(uri)
	if len(diagnostics) != 1 {
		t.Fatalf("the client holds %d diagnostics, want 1", len(diagnostics))
	}
	if got, want := diagnostics[0].Message, "undefined variable"; got != want {
		t.Errorf("the message of the diagnostic = %q, want %q", got, want)
	}
	if got, want := diagnostics[0].Severity, jide.LSPDiagnosticError; got != want {
		t.Errorf("the severity of the diagnostic = %v, want %v", got, want)
	}

	if err := client.DidChange(ctx, uri, 2, []jide.LSPTextChange{{Text: "f(x) = x + 2\n"}}); err != nil {
		t.Fatalf("DidChange() error = %v", err)
	}

	hover, err := client.Hover(ctx, uri, 0, 0)
	if err != nil {
		t.Fatalf("Hover() error = %v", err)
	}
	if hover == nil {
		t.Fatal("Hover() = nil, want the hover of the server")
	}
	if got, want := hover.Contents.Value, "the hover text"; got != want {
		t.Errorf("the hover text = %q, want %q", got, want)
	}
	if got, want := hover.Contents.Kind, jide.LSPMarkupMarkdown; got != want {
		t.Errorf("the kind of the hover = %q, want %q", got, want)
	}

	completion, err := client.Completion(ctx, uri, 0, 0)
	if err != nil {
		t.Fatalf("Completion() error = %v", err)
	}
	if len(completion.Items) != 1 {
		t.Fatalf("the completion holds %d items, want 1", len(completion.Items))
	}
	if got, want := completion.Items[0].Label, "alpha"; got != want {
		t.Errorf("the label of the completion = %q, want %q", got, want)
	}
	if got, want := completion.Items[0].Kind, jide.LSPCompletionFunction; got != want {
		t.Errorf("the kind of the completion = %v, want %v", got, want)
	}

	references, err := client.References(ctx, uri, 0, 0, true)
	if err != nil {
		t.Fatalf("References() error = %v", err)
	}
	if len(references) != 1 {
		t.Fatalf("the references hold %d locations, want 1", len(references))
	}
	if got, want := references[0].URI, "file:///workspace/main.jl"; got != want {
		t.Errorf("the URI of the reference = %q, want %q", got, want)
	}

	definition, err := client.Definition(ctx, uri, 0, 0)
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	if len(definition) != 1 {
		t.Fatalf("the definition holds %d locations, want 1", len(definition))
	}
	if got, want := definition[0].URI, "file:///workspace/other.jl"; got != want {
		t.Errorf("the URI of the definition = %q, want %q", got, want)
	}

	if err := client.DidSave(ctx, uri, "f(x) = x + 2\n"); err != nil {
		t.Fatalf("DidSave() error = %v", err)
	}
	if err := client.DidClose(ctx, uri); err != nil {
		t.Fatalf("DidClose() error = %v", err)
	}

	trace := waitForTrace(t, logPath, "message ", 9)
	for _, method := range []string{
		"message initialize",
		"message textDocument/didOpen",
		"message textDocument/didChange",
		"message textDocument/hover",
		"message textDocument/completion",
		"message textDocument/references",
		"message textDocument/definition",
		"message textDocument/didSave",
		"message textDocument/didClose",
	} {
		if !containsLine(trace, method) {
			t.Errorf("the trace of the fake server does not hold %q: %v", method, trace)
		}
	}
}

// TestClientRefusesASecondStart checks that a client does not spawn a
// second server.
func TestClientRefusesASecondStart(t *testing.T) {
	directory := t.TempDir()
	command, _ := fakeServerCommand(t, directory)
	client := New(Options{Command: command})
	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	if err := client.Start(ctx); err == nil {
		t.Fatal("Start() error = nil, want an error")
	}
}

// TestClientReportsTheStateOfTheServer checks the state that the shell
// shows in its status bar.
func TestClientReportsTheStateOfTheServer(t *testing.T) {
	directory := t.TempDir()
	command, _ := fakeServerCommand(t, directory)
	states := make(chan State, 8)
	client := New(Options{
		Command:       command,
		OnStateChange: func(state State) { states <- state },
	})
	if got, want := client.State(), StateStopped; got != want {
		t.Errorf("State() = %q, want %q", got, want)
	}
	status := client.Status()
	if got, want := status.State, jide.LSPStateStopped; got != want {
		t.Errorf("the state of the status = %q, want %q", got, want)
	}
	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	if got, want := client.State(), StateRunning; got != want {
		t.Errorf("State() = %q, want %q", got, want)
	}
	first := <-states
	if got, want := first, StateStarting; got != want {
		t.Errorf("the first transition = %q, want %q", got, want)
	}
	second := <-states
	if got, want := second, StateRunning; got != want {
		t.Errorf("the second transition = %q, want %q", got, want)
	}
}

// TestClientRefusesAnUnknownDocument checks that a change of a document
// that was never opened is reported.
func TestClientRefusesAnUnknownDocument(t *testing.T) {
	directory := t.TempDir()
	command, _ := fakeServerCommand(t, directory)
	client := New(Options{Command: command})
	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	err := client.DidChange(ctx, "file:///workspace/unknown.jl", 2, []jide.LSPTextChange{{Text: "x"}})
	if err == nil {
		t.Fatal("DidChange() error = nil, want an error")
	}
}

// containsLine reports whether the trace holds the line.
func containsLine(trace []string, line string) bool {
	for _, entry := range trace {
		if entry == line {
			return true
		}
	}
	return false
}
