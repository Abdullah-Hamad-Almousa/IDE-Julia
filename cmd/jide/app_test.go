package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jide-dev/jide/internal/lsp"
	"github.com/jide-dev/jide/internal/workspace"
	jide "github.com/jide-dev/jide/proto/jide"
)

func TestWorkspaceOpenEditSaveRoundTrip(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "Project.toml")
	if err := os.WriteFile(projectPath, []byte("name = \"Fixture\"\n[compat]\njulia = \"1.10\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(root, "src", "main.jl")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o700); err != nil {
		t.Fatal(err)
	}
	original := "function greet(name)\n    \"hello $name\"\nend\n"
	if err := os.WriteFile(sourcePath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	workspaceView, err := app.OpenWorkspace(root)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if workspaceView.Root != root || workspaceView.Name != filepath.Base(root) {
		t.Errorf("workspace view = %+v", workspaceView)
	}
	if workspaceView.ProjectPath != projectPath || workspaceView.JuliaCompat != "1.10" {
		t.Errorf("project environment = %+v", workspaceView)
	}
	if workspaceView.ActiveEnvironment != "project" || !containsEnvironment(workspaceView.Environments, "project") {
		t.Errorf("project environment was not selected: %+v", workspaceView)
	}
	foundSource := false
	for _, entry := range workspaceView.Entries {
		if entry.Path == "src/main.jl" && !entry.IsDirectory {
			foundSource = true
		}
	}
	if !foundSource {
		t.Fatalf("workspace tree does not include src/main.jl: %+v", workspaceView.Entries)
	}

	document, err := app.OpenFile(sourcePath)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if document.Text != original || document.Dirty || len(document.Highlights.Tokens) == 0 {
		t.Errorf("opened document = %+v", document)
	}

	updated := "function greet(name)\n    println(\"hello $name\")\nend\n"
	changed, err := app.UpdateFile(sourcePath, updated)
	if err != nil {
		t.Fatalf("UpdateFile() error = %v", err)
	}
	if !changed.Dirty || changed.Text != updated {
		t.Errorf("updated document = %+v", changed)
	}
	disk, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(disk) != original {
		t.Fatalf("UpdateFile wrote to disk before SaveFile: %q", disk)
	}

	saved, err := app.SaveFile(sourcePath, updated)
	if err != nil {
		t.Fatalf("SaveFile() error = %v", err)
	}
	if saved.Dirty || saved.Text != updated || saved.Version <= changed.Version {
		t.Errorf("saved document = %+v; preceding version %d", saved, changed.Version)
	}
	disk, err = os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(disk) != updated {
		t.Errorf("saved content = %q, want %q", disk, updated)
	}
	if err := app.CloseFile(sourcePath, false); err != nil {
		t.Errorf("CloseFile() after save error = %v", err)
	}
}

func TestDirtyDocumentRequiresExplicitDiscard(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.jl")
	if err := os.WriteFile(path, []byte("x = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	if _, err := app.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}
	if _, err := app.OpenFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpdateFile(path, "x = 2\n"); err != nil {
		t.Fatal(err)
	}
	if err := app.CloseFile(path, false); !errors.Is(err, ErrDirtyDocument) {
		t.Errorf("CloseFile(discard=false) error = %v, want ErrDirtyDocument", err)
	}
	if err := app.CloseFile(path, true); err != nil {
		t.Errorf("CloseFile(discard=true) error = %v", err)
	}
}

func TestDebugOperationsRequireAnOpenWorkspaceAndSession(t *testing.T) {
	app := NewApp()
	if _, err := app.StartDebugSession(); !errors.Is(err, ErrWorkspaceNotOpen) {
		t.Errorf("StartDebugSession() error = %v, want ErrWorkspaceNotOpen", err)
	}
	if err := app.LaunchDebuggee("main.jl", "", nil, false, nil); !errors.Is(err, ErrWorkspaceNotOpen) {
		t.Errorf("LaunchDebuggee() error = %v, want ErrWorkspaceNotOpen", err)
	}
	if _, err := app.DebugThreads(); err == nil || !strings.Contains(err.Error(), "debugger is not running") {
		t.Errorf("DebugThreads() error = %v, want no-session error", err)
	}
	if err := app.EndDebugSession(false); err != nil {
		t.Errorf("EndDebugSession() without a session error = %v", err)
	}
}

func TestRunOperationsRequireAnOpenWorkspace(t *testing.T) {
	app := NewApp()
	if _, err := app.StartRunConfiguration(jide.RunConfiguration{Name: "Script", Type: "julia-script"}); !errors.Is(err, ErrWorkspaceNotOpen) {
		t.Errorf("StartRunConfiguration() error = %v, want ErrWorkspaceNotOpen", err)
	}
	if _, err := app.StartTestRun(jide.TestRunExecuteParams{TestFile: "test/runtests.jl"}); !errors.Is(err, ErrWorkspaceNotOpen) {
		t.Errorf("StartTestRun() error = %v, want ErrWorkspaceNotOpen", err)
	}
	if err := app.StopRun("missing"); err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Errorf("StopRun() error = %v, want unknown-run error", err)
	}
	if err := app.StopTestRun("missing"); err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Errorf("StopTestRun() error = %v, want unknown-test-run error", err)
	}
}

func TestFileOperationsStayInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.jl")
	if err := os.WriteFile(outside, []byte("secret = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	if _, err := app.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}
	if _, err := app.OpenFile(outside); !errors.Is(err, ErrFileOutsideWorkspace) {
		t.Errorf("OpenFile(outside) error = %v, want ErrFileOutsideWorkspace", err)
	}
	if _, err := app.OpenFile(filepath.Join(root, "missing.jl")); err == nil || !strings.Contains(err.Error(), "resolve file link") {
		t.Errorf("OpenFile(missing) error = %v, want a missing-file error", err)
	}
}

func TestEnvironmentPickerRejectsUnknownEnvironment(t *testing.T) {
	app := NewApp()
	if _, err := app.OpenWorkspace(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SelectEnvironment("not-an-environment"); err == nil {
		t.Fatal("SelectEnvironment accepted an unknown environment")
	}
	selected, err := app.SelectEnvironment("default")
	if err != nil {
		t.Fatalf("SelectEnvironment(default) error = %v", err)
	}
	if selected.ActiveEnvironment != "default" {
		t.Errorf("active environment = %q, want default", selected.ActiveEnvironment)
	}
}

func TestMalformedProjectIsReportedWithoutSelectingIt(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "Project.toml")
	if err := os.WriteFile(projectPath, []byte("name = [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	view, err := NewApp().OpenWorkspace(root)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if view.ProjectPath != projectPath || view.EnvironmentWarning == "" {
		t.Errorf("malformed project status = %+v", view)
	}
	if view.ActiveEnvironment != "default" || containsEnvironment(view.Environments, "project") {
		t.Errorf("malformed project was selected as an environment: %+v", view)
	}
}

func TestGetWorkspaceReturnsNilBeforeOpen(t *testing.T) {
	view, err := NewApp().GetWorkspace()
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if view != nil {
		t.Errorf("GetWorkspace() = %+v, want nil", view)
	}
}

func TestSourceOnlyWorkspaceStartsLanguageServer(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "hello.jl")
	if err := os.WriteFile(source, []byte("println(\"hello\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opened, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := opened.Tree(workspace.ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if opened.ProjectPath() != "" {
		t.Fatalf("fixture unexpectedly has Project.toml at %q", opened.ProjectPath())
	}
	if !shouldStartLanguageServer(opened.ProjectPath(), tree) {
		t.Fatal("source-only Julia workspace was not selected for language-server startup")
	}
}

func TestLSPStatusReportsStartupAndError(t *testing.T) {
	app := NewApp()
	if _, err := app.OpenWorkspace(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	app.mu.Lock()
	app.lsp = lsp.New(lsp.Options{})
	app.lspContext, app.ctxCancel = context.WithCancel(context.Background())
	app.lspStarting = true
	app.mu.Unlock()
	if got := app.GetLSPStatusView()["state"]; got != string(lsp.StateStarting) {
		t.Errorf("startup state = %v, want starting", got)
	}
	app.mu.Lock()
	app.lspStarting = false
	app.mu.Unlock()
	status := app.GetLSPStatusView()
	if status["state"] != string(lsp.StateStopped) {
		t.Errorf("failed status = %v", status)
	}
}

func TestGetLSPDiagnosticsReturnsCopy(t *testing.T) {
	app := NewApp()
	app.diagnostics["file:///sample.jl"] = []jide.LSPDiagnostic{{Message: "error"}}
	got := app.GetLSPDiagnostics("file:///sample.jl")
	got[0].Message = "modified"
	if app.GetLSPDiagnostics("file:///sample.jl")[0].Message != "error" {
		t.Fatal("GetLSPDiagnostics returned the stored backing slice")
	}
}

func TestOpenNonJuliaTextDoesNotUseJuliaParser(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.md")
	content := "# Julia notes\nThis is Markdown, not Julia.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	if _, err := app.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}
	document, err := app.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if document.Text != content || len(document.Highlights.Diagnostics) != 0 || len(document.Highlights.Tokens) != 0 {
		t.Errorf("Markdown document = %+v", document)
	}
}

func TestOpenJuliaFileIndexesWindowsSafeEscapedURI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Julia workspace #1% λ")
	path := filepath.Join(root, "source file.jl")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("uri_regression = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	if _, err := app.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}
	if _, err := app.OpenFile(path); err != nil {
		t.Fatalf("OpenFile() rejected the document URI: %v", err)
	}
	result, err := app.SearchText("uri_regression", 10)
	if err != nil || len(result.Matches) != 1 {
		t.Fatalf("SearchText() after open = %+v, %v; want the opened file indexed", result, err)
	}
}

func TestWorkspaceIndexSearchAndDefinitionFallbackWhileLSPUnavailable(t *testing.T) {
	root := t.TempDir()
	definitionPath := filepath.Join(root, "defs.jl")
	usePath := filepath.Join(root, "use.jl")
	if err := os.WriteFile(definitionPath, []byte("function greet(name)\n name\nend\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(usePath, []byte("greet(\"Julia\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	if _, err := app.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}
	search, err := app.SearchText("Julia", 10)
	if err != nil || len(search.Matches) != 1 || search.Matches[0].URI != pathToURI(usePath) {
		t.Fatalf("SearchText() = %+v, %v", search, err)
	}
	symbols, err := app.SearchWorkspaceSymbols("greet", 10)
	if err != nil || len(symbols.Symbols) != 1 {
		t.Fatalf("SearchWorkspaceSymbols() = %+v, %v", symbols, err)
	}
	definition, err := app.LSPDefinition(pathToURI(usePath), 0, 1)
	if err != nil || len(definition.Locations) != 1 || definition.Locations[0].URI != pathToURI(definitionPath) {
		t.Fatalf("LSPDefinition() while unavailable = %+v, %v", definition, err)
	}
	if _, err := app.OpenFile(usePath); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpdateFile(definitionPath, "function greet(name)\n name + 1\nend\n"); err != nil {
		t.Fatal(err)
	}
	search, err = app.SearchText("name + 1", 10)
	if err != nil || len(search.Matches) != 1 {
		t.Fatalf("SearchText() did not see unsaved document content: %+v, %v", search, err)
	}
	definition, err = app.LSPDefinition(pathToURI(usePath), 0, 1)
	if err != nil || len(definition.Locations) != 1 || definition.Locations[0].Range.Start.Line != 0 {
		t.Fatalf("LSPDefinition() did not see an unsaved declaration change: %+v, %v", definition, err)
	}
	if _, err := app.LSPRename(pathToURI(usePath), 0, 1, "welcome"); err == nil {
		t.Fatal("LSPRename() succeeded without a language server")
	}
	paramsResult, err := app.LSPDefinitionRequest(jide.LSPDefinitionParams{URI: pathToURI(usePath), Line: 0, Character: 1})
	if err != nil || len(paramsResult.Locations) != 1 {
		t.Fatalf("LSPDefinitionRequest() = %+v, %v", paramsResult, err)
	}
}

func TestIndexAndRenameOperationsReturnTypedJSONPayloads(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.jl")
	if err := os.WriteFile(path, []byte("function greet()\nend\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	if _, err := app.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}
	text, err := app.IndexSearch(jide.TextSearchParams{WorkspaceID: root, Query: "greet"})
	if err != nil || len(text.Matches) != 1 {
		t.Fatalf("IndexSearch() = %+v, %v", text, err)
	}
	workspaceSymbols, err := app.IndexWorkspaceSymbols(jide.WorkspaceSymbolsParams{WorkspaceID: root, Query: "greet"})
	if err != nil || len(workspaceSymbols.Symbols) != 1 {
		t.Fatalf("IndexWorkspaceSymbols() = %+v, %v", workspaceSymbols, err)
	}
	documentSymbols, err := app.IndexDocumentSymbols(jide.DocumentSymbolsParams{URI: pathToURI(path), Text: "function greet()\nend\n"})
	if err != nil || len(documentSymbols.Symbols) != 1 {
		t.Fatalf("IndexDocumentSymbols() = %+v, %v", documentSymbols, err)
	}
	for _, payload := range []any{text, workspaceSymbols, documentSymbols} {
		if _, err := json.Marshal(payload); err != nil {
			t.Errorf("JSON marshal %T: %v", payload, err)
		}
	}
	if _, err := app.IndexSearch(jide.TextSearchParams{WorkspaceID: "another-workspace", Query: "greet"}); err == nil {
		t.Fatal("IndexSearch() accepted a mismatched workspace id")
	}
}

func containsEnvironment(environments []EnvironmentView, id string) bool {
	for _, environment := range environments {
		if environment.ID == id {
			return true
		}
	}
	return false
}
