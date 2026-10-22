package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Errors returned when a workspace is opened or scanned.
var (
	// ErrEmptyPath is returned when the path of a workspace is empty.
	ErrEmptyPath = errors.New("workspace: the path is empty")
	// ErrNotFound is returned when the path of a workspace does not exist.
	ErrNotFound = errors.New("workspace: the path does not exist")
	// ErrNotADirectory is returned when the path of a workspace is not a
	// directory.
	ErrNotADirectory = errors.New("workspace: the path is not a directory")
)

// ProjectFileName is the name of the Julia project file.
const ProjectFileName = "Project.toml"

// ManifestFileName is the name of the Julia manifest file.
const ManifestFileName = "Manifest.toml"

// Workspace is an opened project folder.
//
// The workspace resolves the Julia environment of the folder: the
// Project.toml and the Manifest.toml of the root, if they exist. A file that
// exists but cannot be parsed does not prevent the workspace from opening;
// the parse error is reported by Project and Manifest.
type Workspace struct {
	root         string
	projectPath  string
	manifestPath string
	project      *Project
	projectErr   error
	manifest     *Manifest
	manifestErr  error
}

// Open opens the folder at root as a workspace.
//
// The root is resolved to an absolute, cleaned path. The function fails with
// ErrEmptyPath, ErrNotFound or ErrNotADirectory when the folder cannot be
// opened.
func Open(root string) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("workspace: %w", ErrEmptyPath)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: resolve %q: %w", root, err)
	}
	absolute = filepath.Clean(absolute)

	info, err := os.Stat(absolute)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("workspace: %w: %s", ErrNotFound, absolute)
		}
		return nil, fmt.Errorf("workspace: stat %q: %w", absolute, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace: %w: %s", ErrNotADirectory, absolute)
	}

	workspace := &Workspace{root: absolute}
	workspace.detectEnvironment()
	return workspace, nil
}

// Root returns the absolute path of the workspace root.
func (w *Workspace) Root() string { return w.root }

// RealRoot returns the absolute path of the workspace root with the symbolic
// links resolved.
//
// The path is the root itself when it cannot be resolved.
func (w *Workspace) RealRoot() string {
	resolved, err := filepath.EvalSymlinks(w.root)
	if err != nil {
		return w.root
	}
	return resolved
}

// ProjectPath returns the absolute path of the Project.toml of the
// workspace.
//
// The path is empty when the workspace has no Julia project.
func (w *Workspace) ProjectPath() string { return w.projectPath }

// ManifestPath returns the absolute path of the Manifest.toml of the
// workspace.
//
// The path is empty when the workspace has no Julia manifest.
func (w *Workspace) ManifestPath() string { return w.manifestPath }

// HasJuliaEnv reports whether the workspace has a Julia project.
func (w *Workspace) HasJuliaEnv() bool { return w.projectPath != "" }

// Project returns the parsed Project.toml of the workspace.
//
// The project is nil and the error is nil when the workspace has no
// Project.toml. A Project.toml that cannot be parsed yields a nil project and
// a non-nil error.
func (w *Workspace) Project() (*Project, error) { return w.project, w.projectErr }

// Manifest returns the parsed Manifest.toml of the workspace.
//
// The manifest is nil and the error is nil when the workspace has no
// Manifest.toml. A Manifest.toml that cannot be parsed yields a nil manifest
// and a non-nil error.
func (w *Workspace) Manifest() (*Manifest, error) { return w.manifest, w.manifestErr }

// Tree scans the source tree of the workspace.
func (w *Workspace) Tree(options ScanOptions) (*Tree, error) {
	return Scan(w.root, options)
}

// detectEnvironment locates and parses the Julia environment of the root.
func (w *Workspace) detectEnvironment() {
	projectPath := filepath.Join(w.root, ProjectFileName)
	if isRegularFile(projectPath) {
		w.projectPath = projectPath
		project, err := ParseProject(projectPath)
		if err != nil {
			w.projectErr = err
		} else {
			w.project = project
		}
	}

	manifestPath := filepath.Join(w.root, ManifestFileName)
	if isRegularFile(manifestPath) {
		w.manifestPath = manifestPath
		manifest, err := ParseManifest(manifestPath)
		if err != nil {
			w.manifestErr = err
		} else {
			w.manifest = manifest
		}
	}

	if w.project != nil && w.manifest != nil {
		w.project.resolve(w.manifest)
	}
}

// FindProjectFile returns the path of the Project.toml that owns the folder
// at start.
//
// The search walks the parents of the folder until it finds a Project.toml.
// The second result reports whether such a file exists.
func FindProjectFile(start string) (string, bool) {
	absolute, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	directory := filepath.Clean(absolute)
	for {
		candidate := filepath.Join(directory, ProjectFileName)
		if isRegularFile(candidate) {
			return candidate, true
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false
		}
		directory = parent
	}
}

// isRegularFile reports whether the path exists and is a regular file.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
