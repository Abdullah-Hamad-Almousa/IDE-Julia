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
