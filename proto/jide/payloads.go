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

// LSPDiagnosticsParams are the parameters of MethodLSPDiagnostics.
type LSPDiagnosticsParams struct {
	// URI is the document URI.
	URI string `json:"uri"`
	// Diagnostics holds the problems that the language server reports for
	// the document.
	Diagnostics []LSPDiagnostic `json:"diagnostics"`
}
