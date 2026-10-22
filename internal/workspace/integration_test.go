package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// fixture is the project tree of the integration test.
type fixture struct {
	// base holds the project folder and the folder outside of it.
	base string
	// root is the project folder.
	root string
	// outside is the folder outside of the project folder.
	outside string
}

// newFixture builds the project tree of the integration test.
//
// The tree holds a Project.toml, a Manifest.toml, a source folder, a test
// folder, a package folder that is linked from the dependencies folder, a
// link that points outside of the project folder and a link that points at
// the project folder itself.
func newFixture(t *testing.T) fixture {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")

	for _, folder := range []string{
		filepath.Join(root, "src"),
		filepath.Join(root, "test"),
		filepath.Join(root, "pkg", "src"),
		filepath.Join(root, "deps"),
		filepath.Join(root, "external"),
		outside,
	} {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatalf("create %q: %v", folder, err)
		}
	}

	writeFile(t, filepath.Join(root, "src"), "Demo.jl", "module Demo\nend\n")
	writeFile(t, filepath.Join(root, "test"), "runtests.jl", "using Test\n")
	writeFile(t, filepath.Join(root, "pkg", "src"), "Pkg.jl", "module Pkg\nend\n")
	writeFile(t, outside, "Outside.jl", "module Outside\nend\n")
	writeFile(t, root, ProjectFileName, `name = "Demo"
uuid = "6f6c8b12-1f1a-4f0a-9c0a-1f1a4f0a9c0a"
version = "0.1.0"

[deps]
Dates = "ade2ca70-3891-5945-98fb-dc099432e06a"

[compat]
julia = "1.10"
`)
	writeFile(t, root, ManifestFileName, `julia_version = "1.10.3"
manifest_format = "2.0"
project_hash = "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c"

[[deps.Dates]]
uuid = "ade2ca70-3891-5945-98fb-dc099432e06a"
version = "1.10.0"
`)

	return fixture{base: base, root: root, outside: outside}
}

// TestIntegrationOpensAFixtureProject checks the detection of the Julia
// environment of a project folder.
func TestIntegrationOpensAFixtureProject(t *testing.T) {
	fixture := newFixture(t)

	workspace, err := Open(fixture.root)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if !workspace.HasJuliaEnv() {
		t.Fatal("the workspace must have a Julia environment")
	}

	project, err := workspace.Project()
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if got, want := project.Name, "Demo"; got != want {
		t.Errorf("the name of the project = %q, want %q", got, want)
	}
	if got, want := project.JuliaCompat(), "1.10"; got != want {
		t.Errorf("the julia compat of the project = %q, want %q", got, want)
	}
	if got, want := len(project.Deps), 1; got != want {
		t.Fatalf("the project has %d dependencies, want %d", got, want)
	}
	if got, want := project.Deps[0].Name, "Dates"; got != want {
		t.Errorf("the dependency is %q, want %q", got, want)
	}

	manifest, err := workspace.Manifest()
	if err != nil {
		t.Fatalf("Manifest() error = %v", err)
	}
	if got, want := manifest.Format, "2.0"; got != want {
		t.Errorf("the format of the manifest = %q, want %q", got, want)
	}
	if got, want := manifest.JuliaVersion, "1.10.3"; got != want {
		t.Errorf("the julia version of the manifest = %q, want %q", got, want)
	}
	dates, ok := manifest.Package("Dates")
	if !ok {
		t.Fatal("the manifest must hold the package Dates")
	}
	if got, want := dates.Version, "1.10.0"; got != want {
		t.Errorf("the version of Dates = %q, want %q", got, want)
	}
	// The version of the manifest is resolved into the project.
	if got, want := project.Deps[0].Version, "1.10.0"; got != want {
		t.Errorf("the resolved version of Dates = %q, want %q", got, want)
	}
}

// TestIntegrationScansAFixtureProject checks the source tree of a project
// folder.
func TestIntegrationScansAFixtureProject(t *testing.T) {
	fixture := newFixture(t)

	workspace, err := Open(fixture.root)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	tree, err := workspace.Tree(ScanOptions{})
	if err != nil {
		t.Fatalf("Tree() error = %v", err)
	}

	paths := entryPaths(tree)
	for _, want := range []string{"Project.toml", "Manifest.toml", "src", "src/Demo.jl", "test", "test/runtests.jl", "pkg"} {
		if !slices.Contains(paths, want) {
			t.Errorf("the tree holds %v, want the entry %q", paths, want)
		}
	}
	// The content of the project must not be listed twice.
	occurrences := 0
	for _, entry := range tree.Entries {
		if entry.RealPath == filepath.Join(fixture.root, "src", "Demo.jl") {
			occurrences++
		}
	}
	if occurrences != 1 {
		t.Errorf("the content of the source folder is listed %d times, want once", occurrences)
	}
}

// TestIntegrationScansTheSymlinkedDependencies checks the handling of the
// symbolic links of a project folder.
//
// The test needs a platform that allows the creation of a symbolic link.
func TestIntegrationScansTheSymlinkedDependencies(t *testing.T) {
	fixture := newFixture(t)

	// The link that points inside the project folder.
	makeLink(t, filepath.Join("..", "pkg"), filepath.Join(fixture.root, "deps", "Linked"))
	// The link that points outside of the project folder.
	makeLink(t, filepath.Join("..", "..", "outside"), filepath.Join(fixture.root, "external", "Out"))
	// The link that points at the project folder itself.
	makeLink(t, ".", filepath.Join(fixture.root, "loop"))

	workspace, err := Open(fixture.root)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	tree, err := workspace.Tree(ScanOptions{})
	if err != nil {
		t.Fatalf("Tree() error = %v", err)
	}

	// The link inside the project folder is resolved and its target is
	// visited exactly once.
	link := findEntry(t, tree, "deps/Linked")
	if !link.Symlink {
		t.Error("the entry of the link must be marked as a symbolic link")
	}
	if link.OutsideRoot {
		t.Error("the link points inside the project folder")
	}
	if got, want := link.RealPath, filepath.Join(fixture.root, "pkg"); got != want {
		t.Errorf("the real path of the link = %q, want %q", got, want)
	}
	occurrences := 0
	for _, entry := range tree.Entries {
		if entry.RealPath == filepath.Join(fixture.root, "pkg", "src", "Pkg.jl") {
			occurrences++
		}
	}
	if occurrences != 1 {
		t.Errorf("the content of the linked package is listed %d times, want once", occurrences)
	}

	// The link outside of the project folder is reported, not followed.
	if got, want := len(tree.External), 1; got != want {
		t.Fatalf("the tree holds %d external links, want %d: %v", got, want, tree.External)
	}
	external := tree.External[0]
	if got, want := external.Path, "external/Out"; got != want {
		t.Errorf("the path of the external link = %q, want %q", got, want)
	}
	if got, want := external.ResolvedPath, fixture.outside; got != want {
		t.Errorf("the resolved path of the external link = %q, want %q", got, want)
	}
	for _, entry := range tree.Entries {
		if entry.RealPath == filepath.Join(fixture.outside, "Outside.jl") {
			t.Error("the content of an external link must not be listed")
		}
	}

	// The link that points at the project folder itself does not make the
	// scan loop.
	loop := findEntry(t, tree, "loop")
	if !loop.Alias {
		t.Error("the link that points at the project folder must be marked as an alias")
	}
	occurrences = 0
	for _, entry := range tree.Entries {
		if entry.RealPath == filepath.Join(fixture.root, "src", "Demo.jl") {
			occurrences++
		}
	}
	if occurrences != 1 {
		t.Errorf("the content of the project folder is listed %d times, want once", occurrences)
	}
}
