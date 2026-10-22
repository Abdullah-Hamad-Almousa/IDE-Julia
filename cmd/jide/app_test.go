package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jide-dev/jide/internal/workspace"
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

func containsEnvironment(environments []EnvironmentView, id string) bool {
	for _, environment := range environments {
		if environment.ID == id {
			return true
		}
	}
	return false
}
