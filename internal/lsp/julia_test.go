package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
	jide "github.com/jide-dev/jide/proto/jide"
)

// TestJuliaLanguageServerEndToEndWithRealJulia drives the client with a real
// LanguageServer.jl installation.
//
// The test needs Julia 1.10+ and installs LanguageServer.jl into the per-user
// tooling environment without modifying the source-only fixture. It skips
// itself when no supported runtime is available.
func TestJuliaLanguageServerEndToEndWithRealJulia(t *testing.T) {
	_, err := jlproc.NewDiscoverer().Discover(context.Background())
	if err != nil {
		t.Skipf("skipping the end-to-end test of the language server: %v", err)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "main.jl")
	mainText := "function localGreeting(name)\n    println(name)\nend\nlocalGreeting(\"Julia\")\npri\n"
	if err := os.WriteFile(source, []byte(mainText), 0o600); err != nil {
		t.Fatalf("write the source: %v", err)
	}
	otherSource := filepath.Join(directory, "other.jl")
	if err := os.WriteFile(otherSource, []byte("localGreeting(\"other\")\n"), 0o600); err != nil {
		t.Fatalf("write the other source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "Project.toml")); !os.IsNotExist(err) {
		t.Fatalf("fixture must remain source-only; Project.toml stat error = %v", err)
	}
	client := New(Options{
		RootURI:           FileURI(directory),
		WorkspaceRoot:     directory,
		InitializeTimeout: 15 * time.Minute,
		RequestTimeout:    5 * time.Minute,
		AnalysisTimeout:   5 * time.Minute,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	t.Cleanup(func() { _ = client.Stop(context.Background()) })

	uri := FileURI(source)
	started := make(chan error, 1)
	go func() { started <- client.Start(ctx) }()
	startupDeadline := time.Now().Add(10 * time.Second)
	for client.State() != StateStarting && time.Now().Before(startupDeadline) {
		select {
		case err := <-started:
			t.Fatalf("Start() finished before entering starting state: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if client.State() != StateStarting {
		t.Fatalf("language server did not enter starting state; got %q", client.State())
	}
	if err := client.DidOpen(ctx, uri, "julia", mainText, 1); err != nil {
		t.Fatalf("queue DidOpen() during startup: %v", err)
	}
	completionReady := make(chan struct {
		result jide.LSPCompletionResult
		err    error
	}, 1)
	go func() {
		result, err := client.Completion(ctx, uri, 4, 3)
		completionReady <- struct {
			result jide.LSPCompletionResult
			err    error
		}{result: result, err: err}
	}()
	if err := <-started; err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	answer := <-completionReady
	if answer.err != nil {
		t.Fatalf("immediate Completion() during startup error = %v", answer.err)
	}
	foundPrint := false
	for _, item := range answer.result.Items {
		if item.Label == "print" || item.Label == "println" {
			foundPrint = true
		}
	}
	if !foundPrint {
		t.Fatalf("immediate completion did not include print/println: %+v", answer.result.Items)
	}

	if err := client.DidOpen(ctx, FileURI(otherSource), "julia", "localGreeting(\"other\")\n", 1); err != nil {
		t.Fatalf("DidOpen(other) error = %v", err)
	}

	hover, err := client.Hover(ctx, uri, 1, 5)
	if err != nil {
		t.Fatalf("Hover() error = %v", err)
	}
	if hover == nil || hover.Contents.Value == "" {
		t.Fatal("Hover() returned no documentation for Base.println")
	}

	definitions, err := client.Definition(ctx, uri, 3, 2)
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	if len(definitions) == 0 {
		t.Fatal("Definition() returned no locations for localGreeting call")
	}
	references, err := client.References(ctx, uri, 3, 2, true)
	if err != nil {
		t.Fatalf("References() error = %v", err)
	}
	if len(references) < 2 {
		t.Fatalf("References() returned %d locations, want the declaration and cross-file use", len(references))
	}
	rename, err := client.Rename(ctx, uri, 3, 2, "localGreetingRenamed")
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if len(rename.Changes) == 0 && len(rename.DocumentChanges) == 0 {
		t.Fatal("Rename() returned an empty workspace edit")
	}
}
