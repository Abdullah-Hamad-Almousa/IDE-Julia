package workspace

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jide-dev/jide/internal/vfs"
	jide "github.com/jide-dev/jide/proto/jide"
)

// TestProjectProtocolInfo checks the protocol representation of a project.
func TestProjectProtocolInfo(t *testing.T) {
	project, err := ParseProject(filepath.Join("testdata", "project", ProjectFileName))
	if err != nil {
		t.Fatalf("ParseProject() error = %v", err)
	}
	info := project.ProtocolInfo()
	if got, want := info.Name, "Demo"; got != want {
		t.Errorf("the name of the project = %q, want %q", got, want)
	}
	if got, want := info.JuliaCompat, "1.10"; got != want {
		t.Errorf("the julia compat of the project = %q, want %q", got, want)
	}
	if got, want := len(info.Dependencies), 2; got != want {
		t.Fatalf("the project has %d dependencies, want %d", got, want)
	}
	if got, want := info.Dependencies[0].Name, "Dates"; got != want {
		t.Errorf("the first dependency is %q, want %q", got, want)
	}
	if got, want := len(info.WeakDependencies), 1; got != want {
		t.Fatalf("the project has %d weak dependencies, want %d", got, want)
	}
	if !info.WeakDependencies[0].Weak {
		t.Error("a weak dependency must be marked as weak")
	}
	if got, want := len(info.Extras), 1; got != want {
		t.Fatalf("the project has %d extras, want %d", got, want)
	}
}

// TestManifestProtocolInfo checks the protocol representation with a
// manifest.
func TestManifestProtocolInfo(t *testing.T) {
	manifest, err := ParseManifest(filepath.Join("testdata", "project", ManifestFileName))
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	info := manifest.ProtocolInfo()
	if got, want := info.Format, "2.0"; got != want {
		t.Errorf("the format of the manifest = %q, want %q", got, want)
	}
	if got, want := len(info.Packages), 4; got != want {
		t.Fatalf("the manifest holds %d packages, want %d", got, want)
	}
	if got, want := info.Packages[1].Name, "JSON"; got != want {
		t.Errorf("the second package is %q, want %q", got, want)
	}
	if got, want := len(info.Packages[1].Dependencies), 2; got != want {
		t.Errorf("JSON has %d dependencies, want %d", got, want)
	}
}

// TestTreeProtocolResult checks the protocol representation with a tree.
func TestTreeProtocolResult(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.jl", "x = 1\n")

	tree, err := Scan(root, ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	result := tree.ProtocolResult()
	if got, want := result.Root, tree.Root; got != want {
		t.Errorf("the root of the tree = %q, want %q", got, want)
	}
	if got, want := len(result.Entries), 1; got != want {
		t.Fatalf("the tree holds %d entries, want %d", got, want)
	}
	if got, want := result.Entries[0].Path, "a.jl"; got != want {
		t.Errorf("the path of the entry = %q, want %q", got, want)
	}
	if got, want := len(result.ExternalSymlinks), 0; got != want {
		t.Errorf("the tree holds %d external links, want %d", got, want)
	}
}

// TestStateResult checks the protocol representation with the state of a
// workspace.
func TestStateResult(t *testing.T) {
	workspace, err := Open(filepath.Join("testdata", "project"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	result := workspace.StateResult("w1")
	if result.Project == nil {
		t.Fatal("the state must hold the project")
	}
	if result.Manifest == nil {
		t.Fatal("the state must hold the manifest")
	}
	if got, want := result.WorkspaceID, "w1"; got != want {
		t.Errorf("the workspace id of the state = %q, want %q", got, want)
	}
	if got, want := result.Root, workspace.Root(); got != want {
		t.Errorf("the root of the state = %q, want %q", got, want)
	}
	if got, want := result.Project.Name, "Demo"; got != want {
		t.Errorf("the name of the project = %q, want %q", got, want)
	}
}

// TestChangesParams checks the conversion of the file-system events into the
// parameters of the protocol notification.
func TestChangesParams(t *testing.T) {
	events := []vfs.WatchEvent{
		{Path: "/tmp/a.jl", Op: vfs.OpCreate},
		{Path: "/tmp/b.jl", Op: vfs.OpWrite},
		{Path: "/tmp/c.jl", Op: vfs.OpRemove},
		{Path: "/tmp/d.jl", Op: vfs.OpRename},
	}
	params := ChangesParams("w1", events)
	if got, want := params.WorkspaceID, "w1"; got != want {
		t.Errorf("the workspace id = %q, want %q", got, want)
	}
	if got, want := len(params.Changes), 4; got != want {
		t.Fatalf("the notification holds %d changes, want %d", got, want)
	}
	want := []jide.FileChangeType{
		jide.FileChangeCreated,
		jide.FileChangeChanged,
		jide.FileChangeDeleted,
		jide.FileChangeDeleted,
	}
	for i, change := range params.Changes {
		if change.Type != want[i] {
			t.Errorf("the type of the change %d = %q, want %q", i, change.Type, want[i])
		}
	}
}

// TestProtocolPayloadsAreMarshalable checks that the protocol representation
// of a workspace is valid JSON.
func TestProtocolPayloadsAreMarshalable(t *testing.T) {
	workspace, err := Open(filepath.Join("testdata", "project"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	tree, err := workspace.Tree(ScanOptions{})
	if err != nil {
		t.Fatalf("Tree() error = %v", err)
	}
	payloads := []any{
		workspace.StateResult("w1"),
		tree.ProtocolResult(),
		ChangesParams("w1", nil),
	}
	for _, payload := range payloads {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		if !json.Valid(raw) {
			t.Errorf("the payload is not valid JSON:\n%s", raw)
		}
	}
}
