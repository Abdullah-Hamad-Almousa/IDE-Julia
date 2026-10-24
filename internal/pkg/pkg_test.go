package pkg

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jide-dev/jide/internal/jlproc"
	"github.com/jide-dev/jide/internal/workspace"
)

func TestListEnvironmentsDeterministicAndFiltered(t *testing.T) {
	root := t.TempDir()
	writeProject(t, filepath.Join(root, "z", "Project.toml"), "Zed", "z-uuid")
	writeProject(t, filepath.Join(root, "a", "Project.toml"), "Alpha", "a-uuid")
	writeProject(t, filepath.Join(root, "node_modules", "hidden", "Project.toml"), "Hidden", "h-uuid")
	writeFile(t, filepath.Join(root, "a", "Manifest.toml"), "manifest_format = \"2.0\"\n[[deps.Dates]]\nuuid = \"dates-uuid\"\nversion = \"1.0.0\"\n")

	got, err := ListEnvironments(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("found %d environments, want 2: %#v", len(got), got)
	}
	if got[0].Project.Name != "Alpha" || got[1].Project.Name != "Zed" {
		t.Fatalf("environments are not sorted by path: %q, %q", got[0].Project.Name, got[1].Project.Name)
	}
	if got[0].Manifest == nil || len(got[0].Manifest.Packages) != 1 || got[0].Manifest.Packages[0].Name != "Dates" {
		t.Fatalf("manifest was not parsed: %#v", got[0].Manifest)
	}
}

func TestListEnvironmentsAcceptsProjectFileAndHonorsCancellation(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "Project.toml")
	writeProject(t, projectPath, "Demo", "demo-uuid")
	got, err := ListEnvironments(context.Background(), projectPath)
	if err != nil || len(got) != 1 || got[0].Root != filepath.Dir(projectPath) {
		t.Fatalf("ListEnvironments(project file) = %#v, %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ListEnvironments(ctx, filepath.Dir(projectPath)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ListEnvironments error = %v, want context.Canceled", err)
	}
}

func TestOperationsUseProjectAndArgumentsWithoutShellInterpolation(t *testing.T) {
	root := t.TempDir()
	writeProject(t, filepath.Join(root, "Project.toml"), "Demo", "demo-uuid")
	var commands []Command
	manager := &Manager{
		Runtime: jlproc.Runtime{Executable: "julia"},
		Runner: func(_ context.Context, command Command) ([]byte, error) {
			commands = append(commands, command)
			return []byte("Pkg completed\n"), nil
		},
	}
	packageName := `Name"; error("injected") #`
	for _, operation := range []func(context.Context, string, string) (string, error){manager.Add, manager.Remove, manager.Upgrade} {
		output, err := operation(context.Background(), filepath.Join(root, "Project.toml"), packageName)
		if err != nil || output != "Pkg completed\n" {
			t.Fatalf("operation result = %q, %v", output, err)
		}
	}
	if _, err := manager.Upgrade(context.Background(), root, ""); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 4 {
		t.Fatalf("captured %d commands, want 4", len(commands))
	}
	wantScripts := []string{addScript, removeScript, updateOneScript, updateAllScript}
	for i, command := range commands {
		if command.Dir != root || command.Args[3] != "--project="+root || command.Args[5] != wantScripts[i] {
			t.Fatalf("command %d = %#v", i, command)
		}
		if i < 3 && command.Args[len(command.Args)-1] != packageName {
			t.Fatalf("package name was not passed as an argument: %#v", command.Args)
		}
	}
}

func TestOperationsValidateInputsAndReportSubprocessOutput(t *testing.T) {
	manager := &Manager{Runtime: jlproc.Runtime{Executable: "julia"}}
	if _, err := manager.Add(context.Background(), t.TempDir(), "  "); err == nil {
		t.Fatal("Add accepted an empty package name")
	}
	if _, err := manager.Remove(context.Background(), "", "Dates"); err == nil {
		t.Fatal("Remove accepted an empty project path")
	}
	root := t.TempDir()
	writeProject(t, filepath.Join(root, "Project.toml"), "Demo", "demo-uuid")
	manager.Runner = func(context.Context, Command) ([]byte, error) {
		return []byte("resolver failed"), errors.New("exit status 1")
	}
	_, err := manager.Add(context.Background(), root, "Dates")
	if err == nil || !strings.Contains(err.Error(), "resolver failed") {
		t.Fatalf("Add error = %v, want captured subprocess output", err)
	}
}

func TestDiffManifestsStableAddedRemovedAndUpdated(t *testing.T) {
	before := &workspace.Manifest{Packages: []workspace.ManifestPackage{
		{Name: "Old", UUID: "old", Version: "1.0"},
		{Name: "Changed", UUID: "changed", Version: "1.0", Deps: []string{"A"}},
		{Name: "Same", UUID: "same", Version: "2.0"},
	}}
	after := &workspace.Manifest{Packages: []workspace.ManifestPackage{
		{Name: "Added", UUID: "added", Version: "1.0"},
		{Name: "Changed", UUID: "changed", Version: "2.0", Deps: []string{"B"}},
		{Name: "Same", UUID: "same", Version: "2.0"},
	}}
	diff := DiffManifests(before, after)
	if len(diff.Added) != 1 || diff.Added[0].Name != "Added" || len(diff.Removed) != 1 || diff.Removed[0].Name != "Old" {
		t.Fatalf("added/removed diff = %#v", diff)
	}
	if len(diff.Updated) != 1 || diff.Updated[0].Before.Version != "1.0" || diff.Updated[0].After.Version != "2.0" {
		t.Fatalf("updated diff = %#v", diff.Updated)
	}
	if empty := DiffManifests(nil, nil); len(empty.Added)+len(empty.Removed)+len(empty.Updated) != 0 {
		t.Fatalf("nil manifests diff = %#v", empty)
	}
}

func TestDiffManifestFilesParsesBothInputs(t *testing.T) {
	root := t.TempDir()
	beforePath := filepath.Join(root, "before.toml")
	afterPath := filepath.Join(root, "after.toml")
	writeFile(t, beforePath, "manifest_format = \"2.0\"\n[[deps.Kite]]\nuuid = \"kite-uuid\"\nversion = \"1.0.0\"\n")
	writeFile(t, afterPath, "manifest_format = \"2.0\"\n[[deps.Kite]]\nuuid = \"kite-uuid\"\nversion = \"2.0.0\"\n")
	diff, err := DiffManifestFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Updated) != 1 || diff.Updated[0].Before.Version != "1.0.0" || diff.Updated[0].After.Version != "2.0.0" {
		t.Fatalf("DiffManifestFiles() = %#v", diff)
	}
}

func TestBuildDependencyGraphSortedAndStable(t *testing.T) {
	project := &workspace.Project{Deps: []workspace.Dependency{{Name: "Zed"}, {Name: "Alpha"}}}
	manifest := &workspace.Manifest{Packages: []workspace.ManifestPackage{
		{Name: "Zed", UUID: "z", Version: "2.0", Deps: []string{"Beta", "Alpha"}},
		{Name: "Alpha", UUID: "a", Version: "1.0"},
	}}
	graph := BuildDependencyGraph(project, manifest)
	if !reflect.DeepEqual(graph.DirectDependencies, []string{"Alpha", "Zed"}) {
		t.Fatalf("direct dependencies = %#v", graph.DirectDependencies)
	}
	if len(graph.Nodes) != 2 || graph.Nodes[0].Name != "Alpha" || graph.Nodes[1].Name != "Zed" {
		t.Fatalf("nodes are not sorted: %#v", graph.Nodes)
	}
	if !reflect.DeepEqual(graph.Nodes[1].Dependencies, []string{"Alpha", "Beta"}) {
		t.Fatalf("node dependencies = %#v", graph.Nodes[1].Dependencies)
	}
}

func TestJuliaPkgAddRemoveIntegration(t *testing.T) {
	julia, err := exec.LookPath("julia")
	if err != nil {
		t.Skip("Julia is not installed; Pkg subprocess integration is not available")
	}
	root := t.TempDir()
	writeProject(t, filepath.Join(root, "Project.toml"), "PkgFixture", "12345678-1234-1234-1234-123456789abc")
	manager := NewManager(jlproc.Runtime{Executable: julia})
	ctx := context.Background()
	if _, err := manager.Add(ctx, root, "Dates"); err != nil {
		t.Fatalf("Pkg.add(Dates): %v", err)
	}
	project, err := workspace.ParseProject(filepath.Join(root, "Project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(project.Deps) != 1 || project.Deps[0].Name != "Dates" {
		t.Fatalf("Project.toml after add = %#v", project.Deps)
	}
	if _, err := manager.Upgrade(ctx, root, "Dates"); err != nil {
		t.Fatalf("Pkg.update(Dates): %v", err)
	}
	if _, err := manager.Remove(ctx, root, "Dates"); err != nil {
		t.Fatalf("Pkg.rm(Dates): %v", err)
	}
	project, err = workspace.ParseProject(filepath.Join(root, "Project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(project.Deps) != 0 {
		t.Fatalf("Project.toml after remove = %#v, want no dependencies", project.Deps)
	}
}

func writeProject(t *testing.T, path, name, uuid string) {
	t.Helper()
	writeFile(t, path, "name = \""+name+"\"\nuuid = \""+uuid+"\"\n")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
