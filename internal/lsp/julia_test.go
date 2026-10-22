package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
)

// TestJuliaLanguageServerEndToEndWithRealJulia drives the client with a real
// LanguageServer.jl installation.
//
// The test needs Julia 1.10+ and installs LanguageServer.jl into the per-user
// environment without modifying the fixture project's manifest. It skips
// itself when no supported runtime is available.
func TestJuliaLanguageServerEndToEndWithRealJulia(t *testing.T) {
	_, err := jlproc.NewDiscoverer().Discover(context.Background())
	if err != nil {
		t.Skipf("skipping the end-to-end test of the language server: %v", err)
	}
	directory := t.TempDir()
	project := filepath.Join(directory, "Project.toml")
	if err := os.WriteFile(project, []byte("[deps]\n"), 0o600); err != nil {
		t.Fatalf("write the project: %v", err)
	}
	source := filepath.Join(directory, "main.jl")
	if err := os.WriteFile(source, []byte("f(x) = x + 1\n"), 0o600); err != nil {
		t.Fatalf("write the source: %v", err)
	}

	client := New(Options{
		RootURI:           FileURI(directory),
		ProjectPath:       project,
		InitializeTimeout: 5 * time.Minute,
		RequestTimeout:    5 * time.Minute,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Stop(context.Background()) })

	uri := FileURI(source)
	if err := client.DidOpen(ctx, uri, "julia", "f(x) = x + 1\n", 1); err != nil {
		t.Fatalf("DidOpen() error = %v", err)
	}

	hover, err := client.Hover(ctx, uri, 0, 0)
	if err != nil {
		t.Fatalf("Hover() error = %v", err)
	}
	if hover == nil {
		t.Log("the language server has no hover for the position")
	}

	completion, err := client.Completion(ctx, uri, 0, 0)
	if err != nil {
		t.Fatalf("Completion() error = %v", err)
	}
	t.Logf("the completion holds %d items", len(completion.Items))

	if _, err := client.Definition(ctx, uri, 0, 2); err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	if _, err := client.References(ctx, uri, 0, 2, true); err != nil {
		t.Fatalf("References() error = %v", err)
	}
}
