package workspace

import (
	"github.com/jide-dev/jide/internal/vfs"
	jide "github.com/jide-dev/jide/proto/jide"
)

// The functions of this file adapt the project model to the payload objects
// of the jide protocol. They are the seam between the engine and the user
// interface: everything a shell needs to render a workspace is reachable
// through them.

// ProtocolInfo returns the protocol representation of the project.
func (p *Project) ProtocolInfo() jide.JuliaProjectInfo {
	return jide.JuliaProjectInfo{
		Path:             p.Path,
		Name:             p.Name,
		UUID:             p.UUID,
		Version:          p.Version,
		JuliaCompat:      p.JuliaCompat(),
		Compat:           p.Compat,
		Dependencies:     dependencyInfo(p.Deps),
		WeakDependencies: dependencyInfo(p.WeakDeps),
		Extras:           dependencyInfo(p.Extras),
	}
}

// ProtocolInfo returns the protocol representation of the manifest.
func (m *Manifest) ProtocolInfo() jide.JuliaManifestInfo {
	packages := make([]jide.JuliaManifestPackageInfo, 0, len(m.Packages))
	for _, resolved := range m.Packages {
		packages = append(packages, jide.JuliaManifestPackageInfo{
			Name:         resolved.Name,
			UUID:         resolved.UUID,
			Version:      resolved.Version,
			TreeHash:     resolved.TreeHash,
			Dependencies: resolved.Deps,
		})
	}
	return jide.JuliaManifestInfo{
		Path:         m.Path,
		Format:       m.Format,
		JuliaVersion: m.JuliaVersion,
		ProjectHash:  m.ProjectHash,
		Packages:     packages,
	}
}

// ProtocolResult returns the protocol representation of the tree.
func (t *Tree) ProtocolResult() jide.WorkspaceTreeResult {
	entries := make([]jide.WorkspaceFileEntry, 0, len(t.Entries))
	for _, entry := range t.Entries {
		entries = append(entries, jide.WorkspaceFileEntry{
			Path:         entry.Path,
			AbsolutePath: entry.AbsolutePath,
			IsDirectory:  entry.IsDir,
			Symlink:      entry.Symlink,
			OutsideRoot:  entry.OutsideRoot,
			LinkTarget:   entry.LinkTarget,
			Alias:        entry.Alias,
			Size:         entry.Size,
		})
	}
	external := make([]jide.ExternalSymlink, 0, len(t.External))
	for _, link := range t.External {
		external = append(external, jide.ExternalSymlink{
			Path:         link.Path,
			Target:       link.Target,
			ResolvedPath: link.ResolvedPath,
		})
	}
	return jide.WorkspaceTreeResult{
		Root:             t.Root,
		Entries:          entries,
		ExternalSymlinks: external,
	}
}

// StateResult returns the protocol representation of the state of the
// workspace.
//
// The identifier of the workspace is assigned by the engine, so the caller
// passes it. The project and the manifest are absent when the workspace has
// none.
func (w *Workspace) StateResult(workspaceID string) jide.WorkspaceStateResult {
	result := jide.WorkspaceStateResult{
		WorkspaceID: workspaceID,
		Root:        w.Root(),
	}
	if w.project != nil {
		info := w.project.ProtocolInfo()
		result.Project = &info
	}
	if w.manifest != nil {
		info := w.manifest.ProtocolInfo()
		result.Manifest = &info
	}
	return result
}

// ChangesParams converts the file-system events of a watcher into the
// parameters of the workspace file change notification.
//
// A rename is reported as the removal of the old path, because the native
// notifications of the operating systems report the new path as a creation.
func ChangesParams(workspaceID string, events []vfs.WatchEvent) jide.WorkspaceDidChangeFilesParams {
	params := jide.WorkspaceDidChangeFilesParams{
		WorkspaceID: workspaceID,
		Changes:     make([]jide.WorkspaceFileChange, 0, len(events)),
	}
	for _, event := range events {
		change := jide.WorkspaceFileChange{Path: event.Path}
		switch {
		case event.Op.Has(vfs.OpCreate):
			change.Type = jide.FileChangeCreated
		case event.Op.Has(vfs.OpRemove), event.Op.Has(vfs.OpRename):
			change.Type = jide.FileChangeDeleted
		default:
			change.Type = jide.FileChangeChanged
		}
		params.Changes = append(params.Changes, change)
	}
	return params
}

// dependencyInfo returns the protocol representation of a list of
// dependencies.
func dependencyInfo(dependencies []Dependency) []jide.JuliaDependencyInfo {
	result := make([]jide.JuliaDependencyInfo, 0, len(dependencies))
	for _, dependency := range dependencies {
		result = append(result, jide.JuliaDependencyInfo{
			Name:         dependency.Name,
			UUID:         dependency.UUID,
			Version:      dependency.Version,
			Weak:         dependency.Weak,
			Dependencies: dependency.Deps,
		})
	}
	return result
}
