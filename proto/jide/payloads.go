package jide

import "encoding/json"

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
	// Query is the case-sensitive literal search text. An empty query returns no
	// matches.
	Query string `json:"query"`
	// Limit caps the number of returned matches. Zero means no explicit limit.
	Limit int `json:"limit,omitempty"`
}

// TextSearchMatch is a single hit of a text search. Line and column are
// one-based; column counts Unicode code points.
type TextSearchMatch struct {
	// URI is the URI of the file that contains the hit.
	URI string `json:"uri"`
	// Line is the one-based line of the hit.
	Line int `json:"line"`
	// Column is the one-based Unicode-code-point column of the hit.
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

// WorkspaceSymbolsParams are the parameters of MethodIndexWorkspaceSymbols.
type WorkspaceSymbolsParams struct {
	// WorkspaceID identifies the workspace to search in.
	WorkspaceID string `json:"workspaceId"`
	// Query is matched against declaration names, case-insensitively. Empty
	// matches all symbols up to the configured result limit.
	Query string `json:"query"`
	// Limit caps returned declarations. Zero means no explicit limit.
	Limit int `json:"limit,omitempty"`
}

// WorkspaceSymbol is one declaration in the workspace symbol index. Symbol
// locations use LSP UTF-16 positions.
type WorkspaceSymbol struct {
	// Name is the declaration name.
	Name string `json:"name"`
	// Kind classifies the Julia declaration.
	Kind DocumentSymbolKind `json:"kind"`
	// Location is the declaration's name range.
	Location LSPLocation `json:"location"`
}

// WorkspaceSymbolsResult is the result of MethodIndexWorkspaceSymbols.
type WorkspaceSymbolsResult struct {
	// Symbols holds matching declarations.
	Symbols []WorkspaceSymbol `json:"symbols"`
	// Truncated reports that additional matches were omitted by the limit.
	Truncated bool `json:"truncated,omitempty"`
}

// SyntaxPosition is a zero-based source position. Column counts UTF-8 bytes
// to match Tree-sitter source coordinates.
type SyntaxPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// SyntaxRange is a half-open source range with byte offsets and positions.
type SyntaxRange struct {
	StartByte int            `json:"startByte"`
	EndByte   int            `json:"endByte"`
	Start     SyntaxPosition `json:"start"`
	End       SyntaxPosition `json:"end"`
}

// SyntaxTokenKind is the editor-neutral category of a Julia syntax token.
type SyntaxTokenKind string

const (
	SyntaxTokenIdentifier      SyntaxTokenKind = "identifier"
	SyntaxTokenKeyword         SyntaxTokenKind = "keyword"
	SyntaxTokenOperator        SyntaxTokenKind = "operator"
	SyntaxTokenNumber          SyntaxTokenKind = "number"
	SyntaxTokenBoolean         SyntaxTokenKind = "boolean"
	SyntaxTokenString          SyntaxTokenKind = "string"
	SyntaxTokenStringDelimiter SyntaxTokenKind = "stringDelimiter"
	SyntaxTokenStringPrefix    SyntaxTokenKind = "stringPrefix"
	SyntaxTokenStringEscape    SyntaxTokenKind = "stringEscape"
	SyntaxTokenCommand         SyntaxTokenKind = "command"
	SyntaxTokenCharacter       SyntaxTokenKind = "character"
	SyntaxTokenComment         SyntaxTokenKind = "comment"
	SyntaxTokenInterpolation   SyntaxTokenKind = "interpolation"
	SyntaxTokenMacro           SyntaxTokenKind = "macro"
	SyntaxTokenPunctuation     SyntaxTokenKind = "punctuation"
	SyntaxTokenError           SyntaxTokenKind = "error"
)

// SyntaxToken is one highlighted Julia source span.
type SyntaxToken struct {
	Kind  SyntaxTokenKind `json:"kind"`
	Text  string          `json:"text"`
	Range SyntaxRange     `json:"range"`
}

// SyntaxDiagnostic describes a syntax error recovered by the Julia parser.
type SyntaxDiagnostic struct {
	Message string      `json:"message"`
	Range   SyntaxRange `json:"range"`
}

// DocumentSymbolsParams are the parameters of MethodIndexSymbols. Text is the
// current editor buffer, so symbol requests do not depend on disk state.
type DocumentSymbolsParams struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

// DocumentSymbolKind classifies a Julia declaration.
type DocumentSymbolKind string

const (
	DocumentSymbolModule   DocumentSymbolKind = "module"
	DocumentSymbolStruct   DocumentSymbolKind = "struct"
	DocumentSymbolType     DocumentSymbolKind = "type"
	DocumentSymbolFunction DocumentSymbolKind = "function"
	DocumentSymbolMacro    DocumentSymbolKind = "macro"
	DocumentSymbolConstant DocumentSymbolKind = "constant"
	DocumentSymbolVariable DocumentSymbolKind = "variable"
	DocumentSymbolField    DocumentSymbolKind = "field"
)

// DocumentSymbol is a declaration in the parsed document.
type DocumentSymbol struct {
	Name           string             `json:"name"`
	Kind           DocumentSymbolKind `json:"kind"`
	Range          SyntaxRange        `json:"range"`
	SelectionRange SyntaxRange        `json:"selectionRange"`
	Children       []DocumentSymbol   `json:"children,omitempty"`
}

// DocumentSymbolsResult is the result of MethodIndexSymbols.
type DocumentSymbolsResult struct {
	URI         string             `json:"uri"`
	Symbols     []DocumentSymbol   `json:"symbols"`
	Diagnostics []SyntaxDiagnostic `json:"diagnostics"`
}

// SyntaxHighlightsParams are the parameters of MethodSyntaxHighlights.
type SyntaxHighlightsParams struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

// SyntaxHighlightsResult is the result of MethodSyntaxHighlights.
type SyntaxHighlightsResult struct {
	URI         string             `json:"uri"`
	Tokens      []SyntaxToken      `json:"tokens"`
	Diagnostics []SyntaxDiagnostic `json:"diagnostics"`
}

// SyntaxFoldsParams are the parameters of MethodSyntaxFolds.
type SyntaxFoldsParams struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

// SyntaxFoldKind classifies a foldable Julia source range.
type SyntaxFoldKind string

const (
	SyntaxFoldBlock   SyntaxFoldKind = "block"
	SyntaxFoldComment SyntaxFoldKind = "comment"
	SyntaxFoldString  SyntaxFoldKind = "string"
)

// SyntaxFoldRange is a multiline region an editor may collapse. Line numbers
// are zero-based and inclusive.
type SyntaxFoldRange struct {
	Kind      SyntaxFoldKind `json:"kind"`
	StartLine int            `json:"startLine"`
	EndLine   int            `json:"endLine"`
	Range     SyntaxRange    `json:"range"`
}

// SyntaxFoldsResult is the result of MethodSyntaxFolds.
type SyntaxFoldsResult struct {
	URI         string             `json:"uri"`
	Folds       []SyntaxFoldRange  `json:"folds"`
	Diagnostics []SyntaxDiagnostic `json:"diagnostics"`
}

// LSPState classifies the lifecycle state of the language server.
type LSPState string

const (
	// LSPStateStopped means that no language server process runs.
	LSPStateStopped LSPState = "stopped"
	// LSPStateStarting means that the engine spawns the language server and
	// performs the initialize handshake.
	LSPStateStarting LSPState = "starting"
	// LSPStateRunning means that the language server is initialized and
	// answers the requests of the shell.
	LSPStateRunning LSPState = "running"
	// LSPStateRestarting means that the language server died and that the
	// engine waits for the backoff delay before it spawns it again.
	LSPStateRestarting LSPState = "restarting"
)

// LSPPosition is a zero-based source position of the language server.
//
// The character counts UTF-16 code units, the unit of the JavaScript string
// type, so that the positions match the ones of the editor.
type LSPPosition struct {
	// Line is the zero-based line of the position.
	Line int `json:"line"`
	// Character is the zero-based column of the position.
	Character int `json:"character"`
}

// LSPRange is a half-open source range of the language server.
type LSPRange struct {
	// Start is the first position of the range.
	Start LSPPosition `json:"start"`
	// End is the position behind the last position of the range.
	End LSPPosition `json:"end"`
}

// LSPLocation is a range inside a document of the workspace.
type LSPLocation struct {
	// URI is the URI of the document.
	URI string `json:"uri"`
	// Range is the range inside the document.
	Range LSPRange `json:"range"`
}

// LSPDiagnosticSeverity classifies a diagnostic of the language server.
type LSPDiagnosticSeverity int

const (
	// LSPDiagnosticError marks an error.
	LSPDiagnosticError LSPDiagnosticSeverity = 1
	// LSPDiagnosticWarning marks a warning.
	LSPDiagnosticWarning LSPDiagnosticSeverity = 2
	// LSPDiagnosticInformation marks an information.
	LSPDiagnosticInformation LSPDiagnosticSeverity = 3
	// LSPDiagnosticHint marks a hint.
	LSPDiagnosticHint LSPDiagnosticSeverity = 4
)

// LSPDiagnostic is a problem that the language server reports for a
// document.
type LSPDiagnostic struct {
	// Range is the range of the document that holds the problem.
	Range LSPRange `json:"range"`
	// Severity classifies the problem.
	Severity LSPDiagnosticSeverity `json:"severity,omitempty"`
	// Code is the code of the problem. It is empty when the server does not
	// report one.
	Code string `json:"code,omitempty"`
	// Source is the name of the tool that reported the problem.
	Source string `json:"source,omitempty"`
	// Message describes the problem.
	Message string `json:"message"`
}

// LSPMarkupKind classifies the format of a markup text.
type LSPMarkupKind string

const (
	// LSPMarkupPlainText marks unformatted text.
	LSPMarkupPlainText LSPMarkupKind = "plaintext"
	// LSPMarkupMarkdown marks markdown text.
	LSPMarkupMarkdown LSPMarkupKind = "markdown"
)

// LSPMarkupContent is a formatted text with its kind.
type LSPMarkupContent struct {
	// Kind classifies the format of the value.
	Kind LSPMarkupKind `json:"kind"`
	// Value is the text itself.
	Value string `json:"value"`
}

// LSPHover is the answer of a hover request.
type LSPHover struct {
	// Contents is the documentation of the symbol under the cursor.
	Contents LSPMarkupContent `json:"contents"`
	// Range is the range of the symbol under the cursor. It is absent when
	// the server does not report one.
	Range *LSPRange `json:"range,omitempty"`
}

// LSPCompletionItemKind classifies a completion item. The values are the
// ones of the language server protocol.
type LSPCompletionItemKind int

const (
	LSPCompletionText          LSPCompletionItemKind = 1
	LSPCompletionMethod        LSPCompletionItemKind = 2
	LSPCompletionFunction      LSPCompletionItemKind = 3
	LSPCompletionConstructor   LSPCompletionItemKind = 4
	LSPCompletionField         LSPCompletionItemKind = 5
	LSPCompletionVariable      LSPCompletionItemKind = 6
	LSPCompletionClass         LSPCompletionItemKind = 7
	LSPCompletionInterface     LSPCompletionItemKind = 8
	LSPCompletionModule        LSPCompletionItemKind = 9
	LSPCompletionProperty      LSPCompletionItemKind = 10
	LSPCompletionUnit          LSPCompletionItemKind = 11
	LSPCompletionValue         LSPCompletionItemKind = 12
	LSPCompletionEnum          LSPCompletionItemKind = 13
	LSPCompletionKeyword       LSPCompletionItemKind = 14
	LSPCompletionSnippet       LSPCompletionItemKind = 15
	LSPCompletionColor         LSPCompletionItemKind = 16
	LSPCompletionFile          LSPCompletionItemKind = 17
	LSPCompletionReference     LSPCompletionItemKind = 18
	LSPCompletionFolder        LSPCompletionItemKind = 19
	LSPCompletionEnumMember    LSPCompletionItemKind = 20
	LSPCompletionConstant      LSPCompletionItemKind = 21
	LSPCompletionStruct        LSPCompletionItemKind = 22
	LSPCompletionEvent         LSPCompletionItemKind = 23
	LSPCompletionOperator      LSPCompletionItemKind = 24
	LSPCompletionTypeParameter LSPCompletionItemKind = 25
)

// LSPCompletionItem is one entry of a completion list.
type LSPCompletionItem struct {
	// Label is the text that the editor shows and inserts.
	Label string `json:"label"`
	// Kind classifies the item.
	Kind LSPCompletionItemKind `json:"kind,omitempty"`
	// Detail is the short description of the item.
	Detail string `json:"detail,omitempty"`
	// Documentation is the long description of the item.
	Documentation string `json:"documentation,omitempty"`
	// SortText orders the item in the list.
	SortText string `json:"sortText,omitempty"`
	// FilterText filters the item while the user types.
	FilterText string `json:"filterText,omitempty"`
	// InsertText is the text that the editor inserts when it differs from
	// the label.
	InsertText string `json:"insertText,omitempty"`
	// TextEdit replaces the text range selected by the server when the
	// completion item is accepted. Text is the replacement text.
	TextEdit *LSPTextChange `json:"textEdit,omitempty"`
	// AdditionalTextEdits are non-overlapping edits applied with the main
	// completion edit.
	AdditionalTextEdits []LSPTextChange `json:"additionalTextEdits,omitempty"`
	// InsertTextFormat is 1 for plain text or 2 for an LSP snippet.
	InsertTextFormat int `json:"insertTextFormat,omitempty"`
}

// LSPTextChange is one change of a document.
//
// A change without a range replaces the whole document, a change with a
// range replaces the text inside the range.
type LSPTextChange struct {
	// Range is the range that the change replaces. It is absent when the
	// change replaces the whole document.
	Range *LSPRange `json:"range,omitempty"`
	// Text is the new text.
	Text string `json:"text"`
}

// LSPStartParams are the parameters of MethodLSPStart.
type LSPStartParams struct {
	// WorkspaceID identifies the workspace whose language server starts.
	WorkspaceID string `json:"workspaceId"`
	// RootURI is the root folder of the workspace, as an URI. It is
	// optional: the engine uses the root of the workspace when it is
	// absent.
	RootURI string `json:"rootUri,omitempty"`
	// ProjectPath is the path of the Julia project of the workspace. It
	// selects the environment of the language server.
	ProjectPath string `json:"projectPath,omitempty"`
}

// LSPStartResult is the result of MethodLSPStart.
type LSPStartResult struct {
	// State is the state of the language server after the request.
	State LSPState `json:"state"`
	// JuliaVersion is the version of the Julia runtime that runs the
	// server.
	JuliaVersion string `json:"juliaVersion,omitempty"`
	// JuliaChannel is the juliaup channel of the runtime.
	JuliaChannel string `json:"juliaChannel,omitempty"`
	// Executable is the path of the Julia executable.
	Executable string `json:"executable,omitempty"`
}

// LSPStopParams are the parameters of MethodLSPStop.
type LSPStopParams struct {
	// WorkspaceID identifies the workspace whose language server stops.
	WorkspaceID string `json:"workspaceId"`
}

// LSPStopResult is the result of MethodLSPStop.
type LSPStopResult struct {
	// State is the state of the language server after the request.
	State LSPState `json:"state"`
}

// LSPStatusParams are the parameters of MethodLSPStatus.
type LSPStatusParams struct {
	// WorkspaceID identifies the workspace whose language server is asked
	// for.
	WorkspaceID string `json:"workspaceId"`
}

// LSPStatusResult is the result of MethodLSPStatus.
type LSPStatusResult struct {
	// State is the lifecycle state of the language server.
	State LSPState `json:"state"`
	// JuliaVersion is the version of the Julia runtime that runs the
	// server.
	JuliaVersion string `json:"juliaVersion,omitempty"`
	// JuliaChannel is the juliaup channel of the runtime.
	JuliaChannel string `json:"juliaChannel,omitempty"`
	// Executable is the path of the Julia executable.
	Executable string `json:"executable,omitempty"`
	// RestartAttempts is the number of restarts since the last successful
	// start.
	RestartAttempts int `json:"restartAttempts,omitempty"`
	// LastError describes the last failure of the language server.
	LastError string `json:"lastError,omitempty"`
}

// LSPHoverParams are the parameters of MethodLSPHover.
type LSPHoverParams struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Line is the zero-based line of the position.
	Line int `json:"line"`
	// Character is the zero-based column of the position.
	Character int `json:"character"`
}

// LSPHoverResult is the result of MethodLSPHover.
type LSPHoverResult struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Hover holds the documentation of the symbol under the cursor. It is
	// absent when the server has nothing to say about the position.
	Hover *LSPHover `json:"hover,omitempty"`
}

// LSPCompletionParams are the parameters of MethodLSPCompletion.
type LSPCompletionParams struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Line is the zero-based line of the position.
	Line int `json:"line"`
	// Character is the zero-based column of the position.
	Character int `json:"character"`
}

// LSPCompletionResult is the result of MethodLSPCompletion.
type LSPCompletionResult struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// IsIncomplete reports that the list is only a prefix of the real one
	// and that the shell should ask again while the user types.
	IsIncomplete bool `json:"isIncomplete,omitempty"`
	// Items holds the completion items.
	Items []LSPCompletionItem `json:"items"`
}

// LSPReferencesParams are the parameters of MethodLSPReferences.
type LSPReferencesParams struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Line is the zero-based line of the position.
	Line int `json:"line"`
	// Character is the zero-based column of the position.
	Character int `json:"character"`
	// IncludeDeclaration asks for the declaration of the symbol too.
	IncludeDeclaration bool `json:"includeDeclaration,omitempty"`
}

// LSPReferencesResult is the result of MethodLSPReferences.
type LSPReferencesResult struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Locations holds the references.
	Locations []LSPLocation `json:"locations"`
}

// LSPDefinitionParams are the parameters of MethodLSPDefinition.
type LSPDefinitionParams struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Line is the zero-based line of the position.
	Line int `json:"line"`
	// Character is the zero-based column of the position.
	Character int `json:"character"`
}

// LSPDefinitionResult is the result of MethodLSPDefinition.
type LSPDefinitionResult struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Locations holds the definitions.
	Locations []LSPLocation `json:"locations"`
}

// LSPRenameParams are the parameters of MethodLSPRename.
type LSPRenameParams struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Line is the zero-based line of the position.
	Line int `json:"line"`
	// Character is the zero-based UTF-16 column of the position.
	Character int `json:"character"`
	// NewName is the replacement identifier.
	NewName string `json:"newName"`
}

// LSPRenameResult is the workspace edit returned by LanguageServer.jl.
type LSPRenameResult struct {
	// Changes maps document URIs to edits in that document.
	Changes map[string][]LSPTextChange `json:"changes,omitempty"`
	// DocumentChanges preserves document versions for servers that return
	// versioned text document edits.
	DocumentChanges []LSPDocumentChange `json:"documentChanges,omitempty"`
}

// LSPDocumentChange is a versioned group of edits to one document.
type LSPDocumentChange struct {
	// URI identifies the edited document.
	URI string `json:"uri"`
	// Version is the version used by the server, when known.
	Version *int `json:"version,omitempty"`
	// Edits are the replacements to apply to the document.
	Edits []LSPTextChange `json:"edits"`
}

// LSPDiagnosticsParams are the parameters of MethodLSPDiagnostics.
type LSPDiagnosticsParams struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Diagnostics holds the problems that the language server reports for
	// the document.
	Diagnostics []LSPDiagnostic `json:"diagnostics"`
}

// ConsoleOpenParams are the parameters of MethodConsoleOpen.
type ConsoleOpenParams struct {
	ProjectPath      string `json:"projectPath,omitempty"`
	WorkingDirectory string `json:"workingDirectory,omitempty"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
}

// ConsoleSessionParams identify one console session.
type ConsoleSessionParams struct {
	SessionID string `json:"sessionId"`
}

// ConsoleInputParams are the parameters of MethodConsoleInput.
type ConsoleInputParams struct {
	SessionID string `json:"sessionId"`
	Input     string `json:"input"`
}

// ConsoleResizeParams are the parameters of MethodConsoleResize.
type ConsoleResizeParams struct {
	SessionID string `json:"sessionId"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// ConsoleOutputParams are output events published by the engine.
type ConsoleOutputParams struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data,omitempty"`
	Exited    bool   `json:"exited,omitempty"`
	ExitCode  int    `json:"exitCode,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ConsoleSessionInfo describes a running or archived session.
type ConsoleSessionInfo struct {
	SessionID        string   `json:"sessionId"`
	ProjectPath      string   `json:"projectPath,omitempty"`
	WorkingDirectory string   `json:"workingDirectory"`
	ProcessID        int      `json:"processId,omitempty"`
	State            string   `json:"state"`
	CreatedAt        string   `json:"createdAt"`
	ReopenedFrom     string   `json:"reopenedFrom,omitempty"`
	History          []string `json:"history"`
}

// ConsoleOpenResult is the started console session.
type ConsoleOpenResult struct {
	Session ConsoleSessionInfo `json:"session"`
}

// ConsoleSessionsResult lists active and archived sessions.
type ConsoleSessionsResult struct {
	Sessions []ConsoleSessionInfo `json:"sessions"`
}

// ConsoleHistoryResult returns the commands recorded for one session.
type ConsoleHistoryResult struct {
	SessionID string   `json:"sessionId"`
	History   []string `json:"history"`
}

// DAPStartParams selects DebugAdapter.jl from a Julia project or an explicit
// adapter executable. ProjectPath takes precedence when supplied.
type DAPStartParams struct {
	ProjectPath      string   `json:"projectPath,omitempty"`
	WorkingDirectory string   `json:"workingDirectory,omitempty"`
	Executable       string   `json:"executable,omitempty"`
	Arguments        []string `json:"arguments,omitempty"`
	Directory        string   `json:"directory,omitempty"`
}

// DAPStartResult returns capabilities negotiated during initialize.
type DAPStartResult struct {
	Capabilities DAPCapabilities `json:"capabilities"`
}

// DAPCapabilities lists capabilities advertised by the adapter.
type DAPCapabilities struct {
	SupportsConfigurationDoneRequest  bool                           `json:"supportsConfigurationDoneRequest,omitempty"`
	SupportsFunctionBreakpoints       bool                           `json:"supportsFunctionBreakpoints,omitempty"`
	SupportsConditionalBreakpoints    bool                           `json:"supportsConditionalBreakpoints,omitempty"`
	SupportsHitConditionalBreakpoints bool                           `json:"supportsHitConditionalBreakpoints,omitempty"`
	SupportsEvaluateForHovers         bool                           `json:"supportsEvaluateForHovers,omitempty"`
	SupportsStepBack                  bool                           `json:"supportsStepBack,omitempty"`
	SupportsSetVariable               bool                           `json:"supportsSetVariable,omitempty"`
	SupportsRestartFrame              bool                           `json:"supportsRestartFrame,omitempty"`
	SupportsStepInTargetsRequest      bool                           `json:"supportsStepInTargetsRequest,omitempty"`
	SupportsExceptionInfoRequest      bool                           `json:"supportsExceptionInfoRequest,omitempty"`
	SupportsTerminateRequest          bool                           `json:"supportsTerminateRequest,omitempty"`
	SupportsRestartRequest            bool                           `json:"supportsRestartRequest,omitempty"`
	SupportsLogPoints                 bool                           `json:"supportsLogPoints,omitempty"`
	SupportsDataBreakpoints           bool                           `json:"supportsDataBreakpoints,omitempty"`
	SupportsVariablePaging            bool                           `json:"supportsVariablePaging,omitempty"`
	SupportsVariableType              bool                           `json:"supportsVariableType,omitempty"`
	ExceptionBreakpointFilters        []DAPExceptionBreakpointFilter `json:"exceptionBreakpointFilters,omitempty"`
}

// DAPExceptionBreakpointFilter describes a named adapter exception filter.
type DAPExceptionBreakpointFilter struct {
	Filter      string `json:"filter"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

// DAPArgumentsParams carries adapter-specific launch or attach arguments.
type DAPArgumentsParams struct {
	Arguments map[string]any `json:"arguments"`
}

// DAPEmptyResult represents a successful DAP operation with an empty body.
type DAPEmptyResult struct{}

// DAPSource identifies a source file in the debuggee.
type DAPSource struct {
	Name            string `json:"name,omitempty"`
	Path            string `json:"path,omitempty"`
	SourceReference int    `json:"sourceReference,omitempty"`
}

// DAPSourceBreakpoint is a requested source breakpoint.
type DAPSourceBreakpoint struct {
	Line         int    `json:"line"`
	Column       int    `json:"column,omitempty"`
	Condition    string `json:"condition,omitempty"`
	HitCondition string `json:"hitCondition,omitempty"`
	LogMessage   string `json:"logMessage,omitempty"`
}

// DAPFunctionBreakpoint is a requested function breakpoint.
type DAPFunctionBreakpoint struct {
	Name         string `json:"name"`
	Condition    string `json:"condition,omitempty"`
	HitCondition string `json:"hitCondition,omitempty"`
}

// DAPBreakpoint is a breakpoint returned by the adapter.
type DAPBreakpoint struct {
	ID       int        `json:"id,omitempty"`
	Verified bool       `json:"verified"`
	Message  string     `json:"message,omitempty"`
	Source   *DAPSource `json:"source,omitempty"`
	Line     int        `json:"line,omitempty"`
	Column   int        `json:"column,omitempty"`
}

// DAPBreakpointsResult returns adapter-verified breakpoints.
type DAPBreakpointsResult struct {
	Breakpoints []DAPBreakpoint `json:"breakpoints"`
}

// DAPSetBreakpointsParams replaces the source breakpoints for a file.
type DAPSetBreakpointsParams struct {
	Source      DAPSource             `json:"source"`
	Breakpoints []DAPSourceBreakpoint `json:"breakpoints"`
}

// DAPFunctionBreakpointsParams replaces function breakpoints.
type DAPFunctionBreakpointsParams struct {
	Breakpoints []DAPFunctionBreakpoint `json:"breakpoints"`
}

// DAPExceptionBreakpointsParams selects adapter-advertised exception filters.
type DAPExceptionBreakpointsParams struct {
	Filters []string `json:"filters"`
}

// DAPThreadParams identifies a thread for stepping or continuation.
type DAPThreadParams struct {
	ThreadID int `json:"threadId"`
	TargetID int `json:"targetId,omitempty"`
}

// DAPFrameParams identifies a frame for scope inspection.
type DAPFrameParams struct {
	FrameID int `json:"frameId"`
}

// DAPVariablesParams identifies the variable container to expand.
type DAPVariablesParams struct {
	VariablesReference int `json:"variablesReference"`
	Start              int `json:"start,omitempty"`
	Count              int `json:"count,omitempty"`
}

// DAPSetVariableParams assigns an expression to a named scope variable.
type DAPSetVariableParams struct {
	VariablesReference int    `json:"variablesReference"`
	Name               string `json:"name"`
	Value              string `json:"value"`
}

// DAPSetVariableResult describes the assigned value returned by the adapter.
type DAPSetVariableResult struct {
	Value              string `json:"value"`
	Type               string `json:"type,omitempty"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables,omitempty"`
	IndexedVariables   int    `json:"indexedVariables,omitempty"`
}

// DAPEvaluateParams requests expression evaluation.
type DAPEvaluateParams struct {
	Expression string `json:"expression"`
	FrameID    int    `json:"frameId,omitempty"`
	Context    string `json:"context,omitempty"`
}

// DAPDisconnectParams closes a debug session.
type DAPDisconnectParams struct {
	TerminateDebuggee bool `json:"terminateDebuggee,omitempty"`
}

// DAPEventParams forwards an adapter event.
type DAPEventParams struct {
	Event string          `json:"event"`
	Body  json.RawMessage `json:"body,omitempty"`
}

// DAPStackTraceParams identifies a thread's stack page.
type DAPStackTraceParams struct {
	ThreadID   int `json:"threadId"`
	StartFrame int `json:"startFrame,omitempty"`
	Levels     int `json:"levels,omitempty"`
}

// DAPThreadsParams is an empty request object for listing threads.
type DAPThreadsParams struct{}

// DAPThread describes one debuggee thread.
type DAPThread struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// DAPThreadsResult lists the current debuggee threads.
type DAPThreadsResult struct {
	Threads []DAPThread `json:"threads"`
}

// DAPScope groups variables visible to a frame.
type DAPScope struct {
	Name               string `json:"name"`
	PresentationHint   string `json:"presentationHint,omitempty"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables,omitempty"`
	IndexedVariables   int    `json:"indexedVariables,omitempty"`
	Expensive          bool   `json:"expensive"`
}

// DAPScopesResult lists scopes available on a stack frame.
type DAPScopesResult struct {
	Scopes []DAPScope `json:"scopes"`
}

// DAPVariable describes a debuggee variable.
type DAPVariable struct {
	Name               string `json:"name"`
	Value              string `json:"value"`
	Type               string `json:"type,omitempty"`
	EvaluateName       string `json:"evaluateName,omitempty"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables,omitempty"`
	IndexedVariables   int    `json:"indexedVariables,omitempty"`
}

// DAPVariablesResult lists the expanded variables.
type DAPVariablesResult struct {
	Variables []DAPVariable `json:"variables"`
}

// DAPEvaluateResult is the formatted result of evaluating an expression.
type DAPEvaluateResult struct {
	Result             string `json:"result"`
	Type               string `json:"type,omitempty"`
	VariablesReference int    `json:"variablesReference"`
}

// DAPContinueResult reports whether all threads resumed.
type DAPContinueResult struct {
	AllThreadsContinued bool `json:"allThreadsContinued,omitempty"`
}

// DAPExceptionInfoResult describes the exception associated with a stop.
type DAPExceptionInfoResult struct {
	ExceptionID string               `json:"exceptionId"`
	Description string               `json:"description,omitempty"`
	BreakMode   string               `json:"breakMode"`
	Details     *DAPExceptionDetails `json:"details,omitempty"`
}

// DAPExceptionDetails contains optional structured exception information.
type DAPExceptionDetails struct {
	Message      string `json:"message,omitempty"`
	TypeName     string `json:"typeName,omitempty"`
	FullTypeName string `json:"fullTypeName,omitempty"`
	EvaluateName string `json:"evaluateName,omitempty"`
	StackTrace   string `json:"stackTrace,omitempty"`
}

// DAPStackFrame is a frame returned by the adapter.
type DAPStackFrame struct {
	ID     int        `json:"id"`
	Name   string     `json:"name"`
	Source *DAPSource `json:"source,omitempty"`
	Line   int        `json:"line"`
	Column int        `json:"column"`
}

// DAPStackTraceResult is the adapter stack response.
type DAPStackTraceResult struct {
	StackFrames []DAPStackFrame `json:"stackFrames"`
	TotalFrames int             `json:"totalFrames,omitempty"`
}

// DAPStepInTarget identifies a call site available for step-in.
type DAPStepInTarget struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

// DAPStepInTargetsResult lists call sites available for step-in.
type DAPStepInTargetsResult struct {
	Targets []DAPStepInTarget `json:"targets"`
}

// DAPBreakpointEventParams publishes the current breakpoint set.
type DAPBreakpointEventParams struct {
	Reason     string        `json:"reason"`
	Breakpoint DAPBreakpoint `json:"breakpoint"`
}

// RunConfiguration describes a built-in or user-defined Julia run target.
type RunConfiguration struct {
	ID               string                     `json:"id,omitempty"`
	Name             string                     `json:"name"`
	Type             string                     `json:"type"`
	ProjectPath      string                     `json:"projectPath,omitempty"`
	WorkingDirectory string                     `json:"workingDirectory,omitempty"`
	Program          string                     `json:"program,omitempty"`
	Module           string                     `json:"module,omitempty"`
	Arguments        []string                   `json:"arguments,omitempty"`
	Environment      map[string]string          `json:"environment,omitempty"`
	Properties       map[string]json.RawMessage `json:"properties,omitempty"`
}

// RunExecuteParams starts one run configuration.
type RunExecuteParams struct {
	Configuration RunConfiguration `json:"configuration"`
}

// RunExecuteResult identifies the launched process.
type RunExecuteResult struct {
	RunID string `json:"runId"`
	PID   int    `json:"pid"`
}

// RunTerminateParams identifies the process to stop.
type RunTerminateParams struct {
	RunID string `json:"runId"`
}

// RunTerminateResult confirms a termination request.
type RunTerminateResult struct {
	Terminated bool `json:"terminated"`
}

// RunInputParams sends text to an interactive run process.
type RunInputParams struct {
	RunID string `json:"runId"`
	Input string `json:"input"`
}

// RunInputResult confirms that input was written to stdin.
type RunInputResult struct {
	Written bool `json:"written"`
}

// RunOutputParams publishes one stdout or stderr chunk.
type RunOutputParams struct {
	RunID  string `json:"runId"`
	Stream string `json:"stream"`
	Data   string `json:"data"`
}

// RunExitedParams publishes the final process status.
type RunExitedParams struct {
	RunID    string `json:"runId"`
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error,omitempty"`
}

// TestRunExecuteParams starts a Julia test file in a project environment.
type TestRunExecuteParams struct {
	ProjectPath      string   `json:"projectPath,omitempty"`
	WorkingDirectory string   `json:"workingDirectory,omitempty"`
	TestFile         string   `json:"testFile"`
	Arguments        []string `json:"arguments,omitempty"`
}

// TestRunExecuteResult identifies the asynchronous Test Explorer run.
type TestRunExecuteResult struct {
	RunID string `json:"runId"`
}

// TestRunEventParams streams test-set, assertion, output, and run completion
// data. SourcePath and Line are populated for assertion navigation.
type TestRunEventParams struct {
	RunID      string `json:"runId"`
	Kind       string `json:"kind"`
	Name       string `json:"name,omitempty"`
	Status     string `json:"status,omitempty"`
	Message    string `json:"message,omitempty"`
	SourcePath string `json:"sourcePath,omitempty"`
	Line       int    `json:"line,omitempty"`
	Passed     int    `json:"passed,omitempty"`
	Failed     int    `json:"failed,omitempty"`
	Errors     int    `json:"errors,omitempty"`
	Skipped    int    `json:"skipped,omitempty"`
	Broken     int    `json:"broken,omitempty"`
	ExitCode   int    `json:"exitCode,omitempty"`
	Stream     string `json:"stream,omitempty"`
	Data       string `json:"data,omitempty"`
}

// PkgEnvironmentsParams are the parameters of MethodPkgEnvironments.
type PkgEnvironmentsParams struct {
	// Roots are directories or Project.toml files to search.
	Roots []string `json:"roots"`
}

// PkgEnvironment describes one discovered Julia project environment.
type PkgEnvironment struct {
	// Root is the directory containing Project.toml.
	Root string `json:"root"`
	// Project is the parsed project metadata.
	Project JuliaProjectInfo `json:"project"`
	// Manifest is the parsed manifest, absent when no manifest exists.
	Manifest *JuliaManifestInfo `json:"manifest,omitempty"`
}

// PkgEnvironmentsResult is the result of MethodPkgEnvironments.
type PkgEnvironmentsResult struct {
	Environments []PkgEnvironment `json:"environments"`
}

// PkgOperationParams are the parameters of add, remove and upgrade requests.
type PkgOperationParams struct {
	// ProjectPath is a Project.toml path or its containing directory.
	ProjectPath string `json:"projectPath"`
	// Package is the package name. It may be empty only for an upgrade-all operation.
	Package string `json:"package,omitempty"`
}

// PkgOperationResult is the result of a Pkg subprocess operation.
type PkgOperationResult struct {
	Output string `json:"output"`
}

// PkgManifestDiffParams are the parameters of MethodPkgManifestDiff.
type PkgManifestDiffParams struct {
	BeforePath string `json:"beforePath"`
	AfterPath  string `json:"afterPath"`
}

// PkgPackageUpdate describes one changed resolved package.
type PkgPackageUpdate struct {
	Before JuliaManifestPackageInfo `json:"before"`
	After  JuliaManifestPackageInfo `json:"after"`
}

// PkgManifestDiffResult is the result of MethodPkgManifestDiff.
type PkgManifestDiffResult struct {
	Added   []JuliaManifestPackageInfo `json:"added"`
	Removed []JuliaManifestPackageInfo `json:"removed"`
	Updated []PkgPackageUpdate         `json:"updated"`
}

// PkgDependencyGraphParams are the parameters of MethodPkgDependencyGraph.
type PkgDependencyGraphParams struct {
	ProjectPath string `json:"projectPath"`
}

// PkgDependencyNode is one resolved package and its direct dependencies.
type PkgDependencyNode struct {
	Name         string   `json:"name"`
	UUID         string   `json:"uuid,omitempty"`
	Version      string   `json:"version,omitempty"`
	Dependencies []string `json:"dependencies"`
}

// PkgDependencyGraphResult is the result of MethodPkgDependencyGraph.
type PkgDependencyGraphResult struct {
	DirectDependencies []string            `json:"directDependencies"`
	Nodes              []PkgDependencyNode `json:"nodes"`
}
