package jide

// This file holds the parameter and result objects of the protocol methods
// that are stable enough to be part of the v1 surface. Every object is
// JSON-marshalable and carries explicit json tags, so the wire format is
// defined by the Go type itself.

// WorkspaceOpenParams are the parameters of MethodWorkspaceOpen.
type WorkspaceOpenParams struct {
	// Path is the absolute path of the folder to open.
	Path string `json:"path"`
	// CreateIfMissing requests that the engine creates the folder when it
	// does not exist yet.
	CreateIfMissing bool `json:"createIfMissing,omitempty"`
}

// WorkspaceOpenResult is the result of MethodWorkspaceOpen.
type WorkspaceOpenResult struct {
	// WorkspaceID identifies the opened workspace.
	WorkspaceID string `json:"workspaceId"`
	// Root is the absolute path of the workspace root folder.
	Root string `json:"root"`
	// JuliaProject is the absolute path of the detected Project.toml, if any.
	JuliaProject string `json:"juliaProject,omitempty"`
	// JuliaVersion is the version of the Julia runtime selected for the
	// workspace, if one could be resolved.
	JuliaVersion string `json:"juliaVersion,omitempty"`
}

// WorkspaceCloseParams are the parameters of MethodWorkspaceClose.
type WorkspaceCloseParams struct {
	// WorkspaceID identifies the workspace to close.
	WorkspaceID string `json:"workspaceId"`
}

// WorkspaceStateParams are the parameters of MethodWorkspaceState.
type WorkspaceStateParams struct {
	// WorkspaceID identifies the workspace whose state is requested.
	WorkspaceID string `json:"workspaceId"`
}

// WorkspaceStateResult is the result of MethodWorkspaceState.
//
// The project and the manifest are absent when the workspace has no Julia
// environment or when the environment could not be parsed. The engine reports
// a parse failure through the error member of the JSON-RPC response.
type WorkspaceStateResult struct {
	// WorkspaceID identifies the workspace.
	WorkspaceID string `json:"workspaceId"`
	// Root is the absolute path of the workspace root folder.
	Root string `json:"root"`
	// Project describes the detected Project.toml, if any.
	Project *JuliaProjectInfo `json:"project,omitempty"`
	// Manifest describes the detected Manifest.toml, if any.
	Manifest *JuliaManifestInfo `json:"manifest,omitempty"`
}

// JuliaProjectInfo describes the Julia project of a workspace.
type JuliaProjectInfo struct {
	// Path is the absolute path of the Project.toml file.
	Path string `json:"path"`
	// Name is the package name of the project.
	Name string `json:"name"`
	// UUID is the package UUID of the project.
	UUID string `json:"uuid,omitempty"`
	// Version is the package version of the project.
	Version string `json:"version,omitempty"`
	// JuliaCompat is the Julia version compatibility of the project, for
	// example "1.10".
	JuliaCompat string `json:"juliaCompat,omitempty"`
	// Compat holds the whole [compat] section of the project.
	Compat map[string]string `json:"compat,omitempty"`
	// Dependencies holds the direct dependencies of the project.
	Dependencies []JuliaDependencyInfo `json:"dependencies"`
	// WeakDependencies holds the weak dependencies of the project.
	WeakDependencies []JuliaDependencyInfo `json:"weakDependencies"`
	// Extras holds the extra packages of the project.
	Extras []JuliaDependencyInfo `json:"extras"`
}

// JuliaDependencyInfo describes one dependency of a Julia project.
type JuliaDependencyInfo struct {
	// Name is the package name.
	Name string `json:"name"`
	// UUID is the package UUID.
	UUID string `json:"uuid,omitempty"`
	// Version is the resolved version, empty when it is unknown.
	Version string `json:"version,omitempty"`
	// Weak reports whether the dependency is a weak dependency.
	Weak bool `json:"weak,omitempty"`
	// Dependencies holds the names of the direct dependencies of the package.
	Dependencies []string `json:"dependencies,omitempty"`
}

// JuliaManifestInfo describes the resolved Julia manifest of a workspace.
type JuliaManifestInfo struct {
	// Path is the absolute path of the Manifest.toml file.
	Path string `json:"path"`
	// Format is the manifest format version, for example "2.0".
	Format string `json:"format,omitempty"`
	// JuliaVersion is the Julia version the manifest was resolved with.
	JuliaVersion string `json:"juliaVersion,omitempty"`
	// ProjectHash identifies the project the manifest belongs to.
	ProjectHash string `json:"projectHash,omitempty"`
	// Packages holds the resolved packages, sorted by name.
	Packages []JuliaManifestPackageInfo `json:"packages"`
}

// JuliaManifestPackageInfo describes one resolved package of a manifest.
type JuliaManifestPackageInfo struct {
	// Name is the package name.
	Name string `json:"name"`
	// UUID is the package UUID.
	UUID string `json:"uuid,omitempty"`
	// Version is the resolved version.
	Version string `json:"version,omitempty"`
	// TreeHash is the source tree hash of the package. It is empty for
	// manifest format 2.0, which does not record it.
	TreeHash string `json:"treeHash,omitempty"`
	// Dependencies holds the names of the direct dependencies of the package.
	Dependencies []string `json:"dependencies,omitempty"`
}

// WorkspaceTreeParams are the parameters of MethodWorkspaceTree.
type WorkspaceTreeParams struct {
	// WorkspaceID identifies the workspace to list.
	WorkspaceID string `json:"workspaceId"`
	// MaxDepth limits how many directory levels below the root are listed.
	// Zero means the engine default, which lists the whole tree.
	MaxDepth int `json:"maxDepth,omitempty"`
}

// WorkspaceTreeResult is the result of MethodWorkspaceTree.
type WorkspaceTreeResult struct {
	// Root is the absolute path of the workspace root folder.
	Root string `json:"root"`
	// Entries holds the entries of the tree in depth-first order.
	Entries []WorkspaceFileEntry `json:"entries"`
	// ExternalSymlinks holds the symbolic links that point outside the root.
	// The engine reports them instead of following them.
	ExternalSymlinks []ExternalSymlink `json:"externalSymlinks"`
}

// WorkspaceFileEntry is one entry of a workspace file tree.
type WorkspaceFileEntry struct {
	// Path is the path of the entry relative to the workspace root, in slash
	// form.
	Path string `json:"path"`
	// AbsolutePath is the absolute path of the entry.
	AbsolutePath string `json:"absolutePath"`
	// IsDirectory reports whether the entry is a directory.
	IsDirectory bool `json:"isDirectory"`
	// Symlink reports whether the entry itself is a symbolic link.
	Symlink bool `json:"symlink,omitempty"`
	// OutsideRoot reports whether the entry points outside the workspace root.
	OutsideRoot bool `json:"outsideRoot,omitempty"`
	// LinkTarget is the raw target of a symbolic link.
	LinkTarget string `json:"linkTarget,omitempty"`
	// Alias reports that the entry points at a real path that the scan
	// already visited. The engine lists the entry but does not scan it again.
	Alias bool `json:"alias,omitempty"`
	// Size is the size of the file in bytes.
	Size int64 `json:"size,omitempty"`
}

// ExternalSymlink records a symbolic link that points outside the workspace
// root.
type ExternalSymlink struct {
	// Path is the path of the link relative to the workspace root, in slash
	// form.
	Path string `json:"path"`
	// Target is the raw target of the link.
	Target string `json:"target"`
	// ResolvedPath is the absolute path the link points to.
	ResolvedPath string `json:"resolvedPath"`
}

// FileChangeType classifies a file-system change.
type FileChangeType string

const (
	// FileChangeCreated reports that a file appeared.
	FileChangeCreated FileChangeType = "created"
	// FileChangeChanged reports that the content of a file changed.
	FileChangeChanged FileChangeType = "changed"
	// FileChangeDeleted reports that a file disappeared.
	FileChangeDeleted FileChangeType = "deleted"
)

// WorkspaceFileChange is a single file-system change of a workspace.
type WorkspaceFileChange struct {
	// Path is the absolute path of the changed file.
	Path string `json:"path"`
	// Type classifies the change.
	Type FileChangeType `json:"type"`
}

// WorkspaceDidChangeFilesParams are the parameters of
// MethodWorkspaceDidChangeFiles.
type WorkspaceDidChangeFilesParams struct {
	// WorkspaceID identifies the workspace the changes belong to.
	WorkspaceID string `json:"workspaceId"`
	// Changes holds the observed changes.
	Changes []WorkspaceFileChange `json:"changes"`
}

// DocumentOpenParams are the parameters of MethodDocumentOpen.
type DocumentOpenParams struct {
	// WorkspaceID identifies the workspace that owns the document.
	WorkspaceID string `json:"workspaceId"`
	// URI is the document URI.
	URI string `json:"uri"`
	// Text is the initial document content. It is optional: when empty the
	// engine reads the document from disk.
	Text string `json:"text,omitempty"`
}

// DocumentOpenResult is the result of MethodDocumentOpen.
type DocumentOpenResult struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Version is the initial document version, starting at 1.
	Version int `json:"version"`
	// LanguageID is the language identifier of the document, for example
	// "julia".
	LanguageID string `json:"languageId"`
}

// DocumentCloseParams are the parameters of MethodDocumentClose.
type DocumentCloseParams struct {
	// URI is the document URI.
	URI string `json:"uri"`
}

// DocumentChangeParams are the parameters of MethodDocumentChange.
type DocumentChangeParams struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Version is the new document version, incremented by the client for
	// every change.
	Version int `json:"version"`
	// Text is the full new document content.
	Text string `json:"text"`
}

// DocumentSaveParams are the parameters of MethodDocumentSave.
type DocumentSaveParams struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Text is the full document content at save time.
	Text string `json:"text"`
}

// DocumentReadParams are the parameters of MethodDocumentRead.
type DocumentReadParams struct {
	// URI is the document URI.
	URI string `json:"uri"`
}

// DocumentReadResult is the result of MethodDocumentRead.
type DocumentReadResult struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Version is the version of the returned snapshot.
	Version int `json:"version"`
	// Text is the full document content.
	Text string `json:"text"`
	// Dirty reports whether the content differs from the file on disk.
	Dirty bool `json:"dirty,omitempty"`
}

// DocumentSaveResult is the result of MethodDocumentSave.
type DocumentSaveResult struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Version is the version of the snapshot that was written to disk.
	Version int `json:"version"`
}

// TextSearchParams are the parameters of MethodIndexSearch.
type TextSearchParams struct {
	// WorkspaceID identifies the workspace to search in.
	WorkspaceID string `json:"workspaceId"`
	// Query is the search text.
	Query string `json:"query"`
	// Limit caps the number of returned matches. Zero means the engine
	// default.
	Limit int `json:"limit,omitempty"`
}

// TextSearchMatch is a single hit of a text search.
type TextSearchMatch struct {
	// URI is the URI of the file that contains the hit.
	URI string `json:"uri"`
	// Line is the one-based line of the hit.
	Line int `json:"line"`
	// Column is the one-based column of the hit.
	Column int `json:"column"`
	// Preview is the trimmed source line of the hit.
	Preview string `json:"preview"`
}

// TextSearchResult is the result of MethodIndexSearch.
type TextSearchResult struct {
	// Matches holds the hits in the order produced by the index.
	Matches []TextSearchMatch `json:"matches"`
	// Truncated reports that the engine dropped further matches because
	// the limit was reached.
	Truncated bool `json:"truncated,omitempty"`
}
