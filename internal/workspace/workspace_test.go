package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestOpenRejectsAnEmptyPath checks the error of an empty path.
func TestOpenRejectsAnEmptyPath(t *testing.T) {
	if _, err := Open("  "); !errors.Is(err, ErrEmptyPath) {
		t.Errorf("Open() error = %v, want ErrEmptyPath", err)
	}
}

// TestOpenRejectsAMissingFolder checks the error of a folder that does not
// exist.
func TestOpenRejectsAMissingFolder(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := Open(missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("Open() error = %v, want ErrNotFound", err)
	}
}

// TestOpenRejectsAFile checks the error of a path that is not a folder.
func TestOpenRejectsAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.jl")
	if err := os.WriteFile(path, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	if _, err := Open(path); !errors.Is(err, ErrNotADirectory) {
		t.Errorf("Open() error = %v, want ErrNotADirectory", err)
	}
}

// TestOpenResolvesAnAbsolutePath checks that the root of a workspace is an
// absolute, cleaned path.
func TestOpenResolvesAnAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	workspace, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if !filepath.IsAbs(workspace.Root()) {
		t.Errorf("root = %q, want an absolute path", workspace.Root())
	}
	if got, want := workspace.Root(), filepath.Clean(dir); got != want {
		t.Errorf("root = %q, want %q", got, want)
	}
	if got, want := workspace.RealRoot(), filepath.Clean(dir); got != want {
		t.Errorf("real root = %q, want %q", got, want)
	}
}

// TestOpenDetectsTheJuliaEnvironment checks the detection of the Project.toml
// and of the Manifest.toml of a project.
func TestOpenDetectsTheJuliaEnvironment(t *testing.T) {
	workspace, err := Open(filepath.Join("testdata", "project"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if !workspace.HasJuliaEnv() {
		t.Fatal("the workspace must have a Julia environment")
	}
	if got, want := filepath.Base(workspace.ProjectPath()), ProjectFileName; got != want {
		t.Errorf("project path = %q, want the file %q", workspace.ProjectPath(), want)
	}
	if got, want := filepath.Base(workspace.ManifestPath()), ManifestFileName; got != want {
		t.Errorf("manifest path = %q, want the file %q", workspace.ManifestPath(), want)
	}

	project, err := workspace.Project()
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if got, want := project.Name, "Demo"; got != want {
		t.Errorf("project name = %q, want %q", got, want)
	}
	if got, want := project.UUID, "6f6c8b12-1f1a-4f0a-9c0a-1f1a4f0a9c0a"; got != want {
		t.Errorf("project uuid = %q, want %q", got, want)
	}
	if got, want := project.JuliaCompat(), "1.10"; got != want {
		t.Errorf("julia compat = %q, want %q", got, want)
	}
	if got, want := project.Compat["JSON"], "0.21"; got != want {
		t.Errorf("compat[JSON] = %q, want %q", got, want)
	}
	if got, want := len(project.Deps), 2; got != want {
		t.Fatalf("the project has %d dependencies, want %d", got, want)
	}
	if got, want := project.Deps[0].Name, "Dates"; got != want {
		t.Errorf("the first dependency is %q, want %q", got, want)
	}
	if got, want := project.Deps[0].Version, "1.10.0"; got != want {
		t.Errorf("the version of Dates = %q, want %q", got, want)
	}
	if got, want := project.Deps[1].Name, "JSON"; got != want {
		t.Errorf("the second dependency is %q, want %q", got, want)
	}
	if got, want := project.Deps[1].UUID, "682c06a0-de6a-54ab-a142-c8b1cf79cde6"; got != want {
		t.Errorf("the uuid of JSON = %q, want %q", got, want)
	}
	if got, want := len(project.Deps[1].Deps), 2; got != want {
		t.Errorf("JSON has %d dependencies, want %d", got, want)
	}
	if got, want := len(project.WeakDeps), 1; got != want {
		t.Fatalf("the project has %d weak dependencies, want %d", got, want)
	}
	if !project.WeakDeps[0].Weak {
		t.Error("a weak dependency must be marked as weak")
	}
	if got, want := project.WeakDeps[0].Version, "1.9.0"; got != want {
		t.Errorf("the version of StaticArrays = %q, want %q", got, want)
	}
	if got, want := len(project.Extras), 1; got != want {
		t.Fatalf("the project has %d extras, want %d", got, want)
	}
	if got, want := project.Targets["test"], []string{"Test"}; !slices.Equal(got, want) {
		t.Errorf("the test target = %v, want %v", got, want)
	}

	manifest, err := workspace.Manifest()
	if err != nil {
		t.Fatalf("Manifest() error = %v", err)
	}
	if got, want := manifest.Format, "2.0"; got != want {
		t.Errorf("manifest format = %q, want %q", got, want)
	}
	if got, want := manifest.JuliaVersion, "1.10.3"; got != want {
		t.Errorf("manifest julia version = %q, want %q", got, want)
	}
	if got, want := manifest.ProjectHash, "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c"; got != want {
		t.Errorf("manifest project hash = %q, want %q", got, want)
	}
	if got, want := len(manifest.Packages), 4; got != want {
		t.Fatalf("the manifest holds %d packages, want %d", got, want)
	}
	if got, want := manifest.Packages[0].Name, "Dates"; got != want {
		t.Errorf("the first package is %q, want %q", got, want)
	}
	if got, want := manifest.Packages[0].Version, "1.10.0"; got != want {
		t.Errorf("the version of Dates = %q, want %q", got, want)
	}
}

// TestOpenWithoutAJuliaEnvironment checks a folder that holds no Julia
// project.
func TestOpenWithoutAJuliaEnvironment(t *testing.T) {
	workspace, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if workspace.HasJuliaEnv() {
		t.Error("the workspace must not have a Julia environment")
	}
	project, err := workspace.Project()
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if project != nil {
		t.Errorf("the project = %v, want nil", project)
	}
	manifest, err := workspace.Manifest()
	if err != nil {
		t.Fatalf("Manifest() error = %v", err)
	}
	if manifest != nil {
		t.Errorf("the manifest = %v, want nil", manifest)
	}
}

// TestOpenReportsABrokenProject checks that a Project.toml that cannot be
// parsed does not prevent the workspace from opening.
func TestOpenReportsABrokenProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ProjectFileName), []byte("name = \"Broken\n"), 0o644); err != nil {
		t.Fatalf("write the project: %v", err)
	}
	workspace, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	project, err := workspace.Project()
	if err == nil {
		t.Fatal("Project() must report the parse error")
	}
	if project != nil {
		t.Errorf("the project = %v, want nil", project)
	}
}

// TestParseProjectRejectsInvalidTOML checks the error of a Project.toml that
// is not valid TOML.
func TestParseProjectRejectsInvalidTOML(t *testing.T) {
	if _, err := ParseProject(filepath.Join("testdata", "broken", ProjectFileName)); err == nil {
		t.Fatal("ParseProject() must fail on an invalid document")
	}
}

// TestParseManifestOfTheFirstFormat checks the manifest format 1.0, which
// records the tree hash of the packages.
func TestParseManifestOfTheFirstFormat(t *testing.T) {
	manifest, err := ParseManifest(filepath.Join("testdata", "manifest-v1", ManifestFileName))
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	if got, want := manifest.Format, "1.0"; got != want {
		t.Errorf("manifest format = %q, want %q", got, want)
	}
	if got, want := len(manifest.Packages), 2; got != want {
		t.Fatalf("the manifest holds %d packages, want %d", got, want)
	}
	base64, ok := manifest.Package("Base64")
	if !ok {
		t.Fatal("the manifest must hold the package Base64")
	}
	if got, want := base64.Version, "1.0.2"; got != want {
		t.Errorf("the version of Base64 = %q, want %q", got, want)
	}
	if got, want := base64.TreeHash, "9a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b"; got != want {
		t.Errorf("the tree hash of Base64 = %q, want %q", got, want)
	}
	dates, ok := manifest.Package("Dates")
	if !ok {
		t.Fatal("the manifest must hold the package Dates")
	}
	if got, want := dates.Deps, []string{"Printf"}; !slices.Equal(got, want) {
		t.Errorf("the dependencies of Dates = %v, want %v", got, want)
	}
}

// TestFindProjectFile checks the search of the project file that owns a
// folder.
func TestFindProjectFile(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, ProjectFileName)
	if err := os.WriteFile(projectPath, []byte("name = \"Demo\"\n"), 0o644); err != nil {
		t.Fatalf("write the project: %v", err)
	}
	nested := filepath.Join(root, "src", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create %q: %v", nested, err)
	}

	found, ok := FindProjectFile(nested)
	if !ok {
		t.Fatal("FindProjectFile() did not find the project file")
	}
	if got, want := found, projectPath; got != want {
		t.Errorf("FindProjectFile() = %q, want %q", got, want)
	}

	if _, ok := FindProjectFile(t.TempDir()); ok {
		t.Error("FindProjectFile() must not find a project file above the temporary folder")
	}
}
