package jide

import (
	"encoding/json"
	"fmt"
	"io"
)

// SchemaDraft is the JSON Schema dialect the exported schema conforms to.
const SchemaDraft = "https://json-schema.org/draft/2020-12/schema"

// SchemaID is the canonical identifier of the jide protocol JSON Schema.
const SchemaID = "https://jide.dev/schema/jide-protocol.json"

// SchemaTitle is the human readable title of the exported schema.
const SchemaTitle = "jide protocol envelope"

// schemaDocument is the root of the exported JSON Schema.
type schemaDocument struct {
	Schema               string         `json:"$schema"`
	ID                   string         `json:"$id"`
	Title                string         `json:"title"`
	Description          string         `json:"description"`
	Type                 string         `json:"type"`
	Required             []string       `json:"required"`
	AdditionalProperties bool           `json:"additionalProperties"`
	Properties           map[string]any `json:"properties"`
	Defs                 map[string]any `json:"$defs"`
	Extensions           schemaExt      `json:"x-jide"`
}

// schemaExt carries the jide specific parts of the schema that are not
// expressible in plain JSON Schema.
type schemaExt struct {
	Version    string         `json:"version"`
	Transport  string         `json:"transport"`
	Methods    []MethodInfo   `json:"methods"`
	ErrorCodes map[string]int `json:"errorCodes"`
}

// JSONSchema returns the JSON Schema of the jide protocol.
//
// The schema describes the JSON-RPC 2.0 envelope, the core parameter and
// result objects and, under "x-jide", the method catalog and the standard
// error codes. The returned bytes are valid JSON and are stable across calls.
func JSONSchema() ([]byte, error) {
	raw, err := json.MarshalIndent(buildSchema(), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("jide: marshal schema: %w", err)
	}
	return raw, nil
}

// WriteJSONSchema writes the JSON Schema of the jide protocol to w.
func WriteJSONSchema(w io.Writer) error {
	raw, err := JSONSchema()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("jide: write schema: %w", err)
	}
	return nil
}

// buildSchema assembles the schema document.
func buildSchema() schemaDocument {
	names := MethodNames()

	properties := map[string]any{
		"jsonrpc": map[string]any{
			"type":        "string",
			"const":       Version,
			"description": "JSON-RPC protocol version, always \"2.0\".",
		},
		"id": map[string]any{
			"$ref": "#/$defs/id",
		},
		"method": map[string]any{
			"type":        "string",
			"enum":        names,
			"description": "Protocol method name.",
		},
		"params": map[string]any{
			"type":        "object",
			"description": "Method parameters.",
		},
	}

	defs := map[string]any{
		"id": map[string]any{
			"description": "Request identifier. A request without an identifier is a notification.",
			"oneOf": []any{
				map[string]any{"type": "string"},
				map[string]any{"type": "number"},
				map[string]any{"type": "null"},
			},
		},
		"request": map[string]any{
			"type":                 "object",
			"description":          "A JSON-RPC 2.0 request message.",
			"required":             []string{"jsonrpc", "method"},
			"additionalProperties": false,
			"properties": map[string]any{
				"jsonrpc": map[string]any{"const": Version},
				"id":      map[string]any{"$ref": "#/$defs/id"},
				"method":  map[string]any{"type": "string", "enum": names},
				"params":  map[string]any{"type": "object"},
			},
		},
		"response": map[string]any{
			"type":                 "object",
			"description":          "A JSON-RPC 2.0 response message.",
			"required":             []string{"jsonrpc", "id"},
			"additionalProperties": false,
			"properties": map[string]any{
				"jsonrpc": map[string]any{"const": Version},
				"id":      map[string]any{"$ref": "#/$defs/id"},
				"result":  map[string]any{"description": "Successful result payload."},
				"error":   map[string]any{"$ref": "#/$defs/error"},
			},
		},
		"error": map[string]any{
			"type":                 "object",
			"description":          "A JSON-RPC 2.0 error object.",
			"required":             []string{"code", "message"},
			"additionalProperties": false,
			"properties": map[string]any{
				"code": map[string]any{
					"type":        "integer",
					"description": "JSON-RPC error code.",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Short description of the error.",
				},
				"data": map[string]any{
					"description": "Optional structured error payload.",
				},
			},
		},
		"workspaceOpenParams":           schemaObject(workspaceOpenParamsSchema),
		"workspaceOpenResult":           schemaObject(workspaceOpenResultSchema),
		"workspaceCloseParams":          schemaObject(workspaceCloseParamsSchema),
		"workspaceStateParams":          schemaObject(workspaceStateParamsSchema),
		"workspaceStateResult":          schemaObject(workspaceStateResultSchema),
		"workspaceTreeParams":           schemaObject(workspaceTreeParamsSchema),
		"workspaceTreeResult":           schemaObject(workspaceTreeResultSchema),
		"workspaceDidChangeFilesParams": schemaObject(workspaceDidChangeFilesParamsSchema),
		"workspaceFileEntry":            schemaObject(workspaceFileEntrySchema),
		"externalSymlink":               schemaObject(externalSymlinkSchema),
		"documentOpenParams":            schemaObject(documentOpenParamsSchema),
		"documentOpenResult":            schemaObject(documentOpenResultSchema),
		"documentCloseParams":           schemaObject(documentCloseParamsSchema),
		"documentChangeParams":          schemaObject(documentChangeParamsSchema),
		"documentSaveParams":            schemaObject(documentSaveParamsSchema),
		"documentReadParams":            schemaObject(documentReadParamsSchema),
		"documentReadResult":            schemaObject(documentReadResultSchema),
		"documentSaveResult":            schemaObject(documentSaveResultSchema),
		"textSearchParams":              schemaObject(textSearchParamsSchema),
		"textSearchResult":              schemaObject(textSearchResultSchema),
		"syntaxPosition":                schemaObject(syntaxPositionSchema),
		"syntaxRange":                   schemaObject(syntaxRangeSchema),
		"syntaxToken":                   schemaObject(syntaxTokenSchema),
		"syntaxDiagnostic":              schemaObject(syntaxDiagnosticSchema),
		"documentSymbolsParams":         schemaObject(documentSymbolsParamsSchema),
		"documentSymbolsResult":         schemaObject(documentSymbolsResultSchema),
		"documentSymbol":                schemaObject(documentSymbolSchema),
		"syntaxHighlightsParams":        schemaObject(syntaxHighlightsParamsSchema),
		"syntaxHighlightsResult":        schemaObject(syntaxHighlightsResultSchema),
		"syntaxFoldsParams":             schemaObject(syntaxFoldsParamsSchema),
		"syntaxFoldsResult":             schemaObject(syntaxFoldsResultSchema),
		"syntaxFoldRange":               schemaObject(syntaxFoldRangeSchema),
		"lspStartParams":                schemaObject(lspStartParamsSchema),
		"lspStartResult":                schemaObject(lspStartResultSchema),
		"lspStopParams":                 schemaObject(lspStopParamsSchema),
		"lspStopResult":                 schemaObject(lspStopResultSchema),
		"lspStatusParams":               schemaObject(lspStatusParamsSchema),
		"lspStatusResult":               schemaObject(lspStatusResultSchema),
		"lspHoverParams":                schemaObject(lspHoverParamsSchema),
		"lspHoverResult":                schemaObject(lspHoverResultSchema),
		"lspCompletionParams":           schemaObject(lspCompletionParamsSchema),
		"lspCompletionResult":           schemaObject(lspCompletionResultSchema),
		"lspReferencesParams":           schemaObject(lspReferencesParamsSchema),
		"lspReferencesResult":           schemaObject(lspReferencesResultSchema),
		"lspDefinitionParams":           schemaObject(lspDefinitionParamsSchema),
		"lspDefinitionResult":           schemaObject(lspDefinitionResultSchema),
		"lspDiagnosticsParams":          schemaObject(lspDiagnosticsParamsSchema),
		"lspPosition":                   schemaObject(lspPositionSchema),
		"lspRange":                      schemaObject(lspRangeSchema),
		"lspLocation":                   schemaObject(lspLocationSchema),
		"lspDiagnostic":                 schemaObject(lspDiagnosticSchema),
		"lspMarkupContent":              schemaObject(lspMarkupContentSchema),
		"lspHover":                      schemaObject(lspHoverSchema),
		"lspCompletionItem":             schemaObject(lspCompletionItemSchema),
		"lspTextChange":                 schemaObject(lspTextChangeSchema),
		"juliaProjectInfo":              schemaObject(juliaProjectInfoSchema),
		"juliaDependencyInfo":           schemaObject(juliaDependencyInfoSchema),
		"juliaManifestInfo":             schemaObject(juliaManifestInfoSchema),
		"juliaManifestPackageInfo":      schemaObject(juliaManifestPackageInfoSchema),
	}

	return schemaDocument{
		Schema:               SchemaDraft,
		ID:                   SchemaID,
		Title:                SchemaTitle,
		Description:          "jide protocol: JSON-RPC 2.0 envelope exchanged between the jide UI shell and the jide engine daemon over the jide:// transport.",
		Type:                 "object",
		Required:             []string{"jsonrpc", "method"},
		AdditionalProperties: false,
		Properties:           properties,
		Defs:                 defs,
		Extensions: schemaExt{
			Version:    Version,
			Transport:  "jide://",
			Methods:    Methods(),
			ErrorCodes: ErrorCodeNames,
		},
	}
}

// schemaObject builds a JSON Schema object node from its properties.
func schemaObject(properties map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
}

func requiredString(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

// optionalString describes a string member that a payload may omit.
func optionalString(description string) map[string]any {
	return requiredString(description)
}

// lspStateSchema describes the lifecycle state of the language server.
func lspStateSchema() map[string]any {
	return map[string]any{
		"type": "string",
		"enum": []string{
			string(LSPStateStopped), string(LSPStateStarting),
			string(LSPStateRunning), string(LSPStateRestarting),
		},
		"description": "Lifecycle state of the language server.",
	}
}

var (
	workspaceOpenParamsSchema = map[string]any{
		"path": requiredString("Absolute path of the folder to open."),
		"createIfMissing": map[string]any{
			"type":        "boolean",
			"description": "Create the folder when it does not exist.",
		},
	}

	workspaceOpenResultSchema = map[string]any{
		"workspaceId": requiredString("Identifier of the opened workspace."),
		"root":        requiredString("Absolute path of the workspace root folder."),
		"juliaProject": map[string]any{
			"type":        "string",
			"description": "Absolute path of the detected Project.toml.",
		},
		"juliaVersion": map[string]any{
			"type":        "string",
			"description": "Version of the selected Julia runtime.",
		},
	}

	workspaceCloseParamsSchema = map[string]any{
		"workspaceId": requiredString("Identifier of the workspace to close."),
	}

	workspaceStateParamsSchema = map[string]any{
		"workspaceId": requiredString("Identifier of the workspace whose state is requested."),
	}

	workspaceStateResultSchema = map[string]any{
		"workspaceId": requiredString("Identifier of the workspace."),
		"root":        requiredString("Absolute path of the workspace root folder."),
		"project": map[string]any{
			"$ref":        "#/$defs/juliaProjectInfo",
			"description": "The detected Julia project, absent when the workspace has none.",
		},
		"manifest": map[string]any{
			"$ref":        "#/$defs/juliaManifestInfo",
			"description": "The detected Julia manifest, absent when the workspace has none.",
		},
	}

	workspaceTreeParamsSchema = map[string]any{
		"workspaceId": requiredString("Identifier of the workspace to list."),
		"maxDepth": map[string]any{
			"type":        "integer",
			"description": "Maximum number of directory levels below the root; zero lists the whole tree.",
		},
	}

	workspaceTreeResultSchema = map[string]any{
		"root": requiredString("Absolute path of the workspace root folder."),
		"entries": map[string]any{
			"type": "array",
			"items": map[string]any{
				"$ref": "#/$defs/workspaceFileEntry",
			},
		},
		"externalSymlinks": map[string]any{
			"type": "array",
			"items": map[string]any{
				"$ref": "#/$defs/externalSymlink",
			},
		},
	}

	workspaceDidChangeFilesParamsSchema = map[string]any{
		"workspaceId": requiredString("Identifier of the workspace the changes belong to."),
		"changes": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type":     "object",
				"required": []string{"path", "type"},
				"properties": map[string]any{
					"path": requiredString("Absolute path of the changed file."),
					"type": map[string]any{
						"type":        "string",
						"enum":        []string{string(FileChangeCreated), string(FileChangeChanged), string(FileChangeDeleted)},
						"description": "Kind of the change.",
					},
				},
			},
		},
	}

	workspaceFileEntrySchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":         requiredString("Path of the entry relative to the workspace root, in slash form."),
			"absolutePath": requiredString("Absolute path of the entry."),
			"isDirectory": map[string]any{
				"type":        "boolean",
				"description": "True when the entry is a directory.",
			},
			"symlink": map[string]any{
				"type":        "boolean",
				"description": "True when the entry itself is a symbolic link.",
			},
			"outsideRoot": map[string]any{
				"type":        "boolean",
				"description": "True when the entry points outside the workspace root.",
			},
			"linkTarget": map[string]any{
				"type":        "string",
				"description": "Raw target of a symbolic link.",
			},
			"alias": map[string]any{
				"type":        "boolean",
				"description": "True when the entry points at an already visited real path.",
			},
			"size": map[string]any{
				"type":        "integer",
				"description": "Size of the file in bytes.",
			},
		},
	}

	externalSymlinkSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":         requiredString("Path of the link relative to the workspace root, in slash form."),
			"target":       requiredString("Raw target of the link."),
			"resolvedPath": requiredString("Absolute path the link points to."),
		},
	}

	documentOpenParamsSchema = map[string]any{
		"workspaceId": requiredString("Identifier of the owning workspace."),
		"uri":         requiredString("Document URI."),
		"text": map[string]any{
			"type":        "string",
			"description": "Initial document content; empty means read from disk.",
		},
	}

	documentOpenResultSchema = map[string]any{
		"uri":        requiredString("Document URI."),
		"version":    map[string]any{"type": "integer", "description": "Initial document version."},
		"languageId": requiredString("Language identifier of the document."),
	}

	documentCloseParamsSchema = map[string]any{
		"uri": requiredString("Document URI."),
	}

	documentChangeParamsSchema = map[string]any{
		"uri":     requiredString("Document URI."),
		"version": map[string]any{"type": "integer", "description": "New document version."},
		"text":    requiredString("Full new document content."),
	}

	documentSaveParamsSchema = map[string]any{
		"uri":  requiredString("Document URI."),
		"text": requiredString("Full document content at save time."),
	}

	documentReadParamsSchema = map[string]any{
		"uri": requiredString("Document URI."),
	}

	documentReadResultSchema = map[string]any{
		"uri":     requiredString("Document URI."),
		"version": map[string]any{"type": "integer", "description": "Version of the returned snapshot."},
		"text":    requiredString("Full document content."),
		"dirty": map[string]any{
			"type":        "boolean",
			"description": "True when the content differs from the file on disk.",
		},
	}

	documentSaveResultSchema = map[string]any{
		"uri":     requiredString("Document URI."),
		"version": map[string]any{"type": "integer", "description": "Version of the snapshot written to disk."},
	}

	juliaProjectInfoSchema = map[string]any{
		"path": requiredString("Absolute path of the Project.toml file."),
		"name": requiredString("Package name of the project."),
		"uuid": map[string]any{
			"type":        "string",
			"description": "Package UUID of the project.",
		},
		"version": map[string]any{
			"type":        "string",
			"description": "Package version of the project.",
		},
		"juliaCompat": map[string]any{
			"type":        "string",
			"description": "Julia version compatibility of the project.",
		},
		"compat": map[string]any{
			"type":                 "object",
			"additionalProperties": map[string]any{"type": "string"},
			"description":          "The whole [compat] section of the project.",
		},
		"dependencies": map[string]any{
			"type": "array",
			"items": map[string]any{
				"$ref": "#/$defs/juliaDependencyInfo",
			},
		},
		"weakDependencies": map[string]any{
			"type": "array",
			"items": map[string]any{
				"$ref": "#/$defs/juliaDependencyInfo",
			},
		},
		"extras": map[string]any{
			"type": "array",
			"items": map[string]any{
				"$ref": "#/$defs/juliaDependencyInfo",
			},
		},
	}

	juliaDependencyInfoSchema = map[string]any{
		"name": requiredString("Package name."),
		"uuid": map[string]any{
			"type":        "string",
			"description": "Package UUID.",
		},
		"version": map[string]any{
			"type":        "string",
			"description": "Resolved version, empty when it is unknown.",
		},
		"weak": map[string]any{
			"type":        "boolean",
			"description": "True when the dependency is a weak dependency.",
		},
		"dependencies": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Names of the direct dependencies of the package.",
		},
	}

	juliaManifestInfoSchema = map[string]any{
		"path": requiredString("Absolute path of the Manifest.toml file."),
		"format": map[string]any{
			"type":        "string",
			"description": "Manifest format version, for example \"2.0\".",
		},
		"juliaVersion": map[string]any{
			"type":        "string",
			"description": "Julia version the manifest was resolved with.",
		},
		"projectHash": map[string]any{
			"type":        "string",
			"description": "Identifier of the project the manifest belongs to.",
		},
		"packages": map[string]any{
			"type": "array",
			"items": map[string]any{
				"$ref": "#/$defs/juliaManifestPackageInfo",
			},
		},
	}

	juliaManifestPackageInfoSchema = map[string]any{
		"name": requiredString("Package name."),
		"uuid": map[string]any{
			"type":        "string",
			"description": "Package UUID.",
		},
		"version": map[string]any{
			"type":        "string",
			"description": "Resolved version.",
		},
		"treeHash": map[string]any{
			"type":        "string",
			"description": "Source tree hash of the package; empty for manifest format 2.0.",
		},
		"dependencies": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Names of the direct dependencies of the package.",
		},
	}

	textSearchParamsSchema = map[string]any{
		"workspaceId": requiredString("Identifier of the workspace to search in."),
		"query":       requiredString("Search text."),
		"limit": map[string]any{
			"type":        "integer",
			"description": "Maximum number of matches; zero means the engine default.",
		},
	}

	textSearchResultSchema = map[string]any{
		"matches": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"uri":     requiredString("URI of the file containing the hit."),
					"line":    map[string]any{"type": "integer", "description": "One-based line of the hit."},
					"column":  map[string]any{"type": "integer", "description": "One-based column of the hit."},
					"preview": requiredString("Trimmed source line of the hit."),
				},
			},
		},
		"truncated": map[string]any{
			"type":        "boolean",
			"description": "True when the engine dropped further matches.",
		},
	}

	syntaxPositionSchema = map[string]any{
		"line":   map[string]any{"type": "integer", "description": "Zero-based line number."},
		"column": map[string]any{"type": "integer", "description": "Zero-based UTF-8 byte column."},
	}

	syntaxRangeSchema = map[string]any{
		"startByte": map[string]any{"type": "integer", "description": "Inclusive UTF-8 byte offset."},
		"endByte":   map[string]any{"type": "integer", "description": "Exclusive UTF-8 byte offset."},
		"start":     map[string]any{"$ref": "#/$defs/syntaxPosition"},
		"end":       map[string]any{"$ref": "#/$defs/syntaxPosition"},
	}

	syntaxTokenSchema = map[string]any{
		"kind": map[string]any{
			"type": "string",
			"enum": []string{
				string(SyntaxTokenIdentifier), string(SyntaxTokenKeyword), string(SyntaxTokenOperator),
				string(SyntaxTokenNumber), string(SyntaxTokenBoolean), string(SyntaxTokenString),
				string(SyntaxTokenStringDelimiter), string(SyntaxTokenStringPrefix), string(SyntaxTokenStringEscape),
				string(SyntaxTokenCommand), string(SyntaxTokenCharacter), string(SyntaxTokenComment),
				string(SyntaxTokenInterpolation), string(SyntaxTokenMacro), string(SyntaxTokenPunctuation),
				string(SyntaxTokenError),
			},
			"description": "Stable editor-neutral syntax token category.",
		},
		"text":  requiredString("Source text covered by the token."),
		"range": map[string]any{"$ref": "#/$defs/syntaxRange"},
	}

	syntaxDiagnosticSchema = map[string]any{
		"message": requiredString("Short syntax recovery message."),
		"range":   map[string]any{"$ref": "#/$defs/syntaxRange"},
	}

	documentSymbolsParamsSchema = map[string]any{
		"uri":  requiredString("Document URI."),
		"text": requiredString("Current document text."),
	}

	documentSymbolSchema = map[string]any{
		"name": requiredString("Declaration name."),
		"kind": map[string]any{
			"type": "string",
			"enum": []string{
				string(DocumentSymbolModule), string(DocumentSymbolStruct), string(DocumentSymbolType),
				string(DocumentSymbolFunction), string(DocumentSymbolMacro), string(DocumentSymbolConstant),
				string(DocumentSymbolVariable), string(DocumentSymbolField),
			},
			"description": "Julia declaration category.",
		},
		"range":          map[string]any{"$ref": "#/$defs/syntaxRange"},
		"selectionRange": map[string]any{"$ref": "#/$defs/syntaxRange"},
		"children": map[string]any{
			"type":  "array",
			"items": map[string]any{"$ref": "#/$defs/documentSymbol"},
		},
	}

	documentSymbolsResultSchema = map[string]any{
		"uri": requiredString("Document URI."),
		"symbols": map[string]any{
			"type":  "array",
			"items": map[string]any{"$ref": "#/$defs/documentSymbol"},
		},
		"diagnostics": schemaArrayRef("syntaxDiagnostic"),
	}

	syntaxHighlightsParamsSchema = map[string]any{
		"uri":  requiredString("Document URI."),
		"text": requiredString("Current document text."),
	}

	syntaxHighlightsResultSchema = map[string]any{
		"uri":         requiredString("Document URI."),
		"tokens":      schemaArrayRef("syntaxToken"),
		"diagnostics": schemaArrayRef("syntaxDiagnostic"),
	}

	syntaxFoldsParamsSchema = map[string]any{
		"uri":  requiredString("Document URI."),
		"text": requiredString("Current document text."),
	}

	syntaxFoldRangeSchema = map[string]any{
		"kind": map[string]any{
			"type": "string",
			"enum": []string{string(SyntaxFoldBlock), string(SyntaxFoldComment), string(SyntaxFoldString)},
		},
		"startLine": map[string]any{"type": "integer", "description": "Zero-based inclusive start line."},
		"endLine":   map[string]any{"type": "integer", "description": "Zero-based inclusive end line."},
		"range":     map[string]any{"$ref": "#/$defs/syntaxRange"},
	}

	syntaxFoldsResultSchema = map[string]any{
		"uri":         requiredString("Document URI."),
		"folds":       schemaArrayRef("syntaxFoldRange"),
		"diagnostics": schemaArrayRef("syntaxDiagnostic"),
	}

	lspStartParamsSchema = map[string]any{
		"workspaceId": requiredString("Identifier of the workspace whose language server starts."),
		"rootUri": map[string]any{
			"type":        "string",
			"description": "Root folder of the workspace, as an URI.",
		},
		"projectPath": map[string]any{
			"type":        "string",
			"description": "Path of the Julia project that selects the environment of the server.",
		},
	}

	lspStartResultSchema = map[string]any{
		"state":        lspStateSchema(),
		"juliaVersion": optionalString("Version of the Julia runtime that runs the language server."),
		"juliaChannel": optionalString("juliup channel of the Julia runtime."),
		"executable":   optionalString("Path of the Julia executable."),
	}

	lspStopParamsSchema = map[string]any{
		"workspaceId": requiredString("Identifier of the workspace whose language server stops."),
	}

	lspStopResultSchema = map[string]any{
		"state": lspStateSchema(),
	}

	lspStatusParamsSchema = map[string]any{
		"workspaceId": requiredString("Identifier of the workspace whose language server is asked for."),
	}

	lspStatusResultSchema = map[string]any{
		"state":           lspStateSchema(),
		"juliaVersion":    optionalString("Version of the Julia runtime that runs the language server."),
		"juliaChannel":    optionalString("juliup channel of the Julia runtime."),
		"executable":      optionalString("Path of the Julia executable."),
		"restartAttempts": map[string]any{"type": "integer", "description": "Number of restarts since the last successful start."},
		"lastError":       optionalString("Description of the last failure of the language server."),
	}

	lspHoverParamsSchema = map[string]any{
		"uri":       requiredString("Document URI."),
		"line":      map[string]any{"type": "integer", "description": "Zero-based line of the position."},
		"character": map[string]any{"type": "integer", "description": "Zero-based UTF-16 column of the position."},
	}

	lspHoverResultSchema = map[string]any{
		"uri": requiredString("Document URI."),
		"hover": map[string]any{
			"$ref":        "#/$defs/lspHover",
			"description": "Documentation of the symbol under the cursor; absent when there is none.",
		},
	}

	lspCompletionParamsSchema = map[string]any{
		"uri":       requiredString("Document URI."),
		"line":      map[string]any{"type": "integer", "description": "Zero-based line of the position."},
		"character": map[string]any{"type": "integer", "description": "Zero-based UTF-16 column of the position."},
	}

	lspCompletionResultSchema = map[string]any{
		"uri": requiredString("Document URI."),
		"isIncomplete": map[string]any{
			"type":        "boolean",
			"description": "True when the list is only a prefix of the real one.",
		},
		"items": schemaArrayRef("lspCompletionItem"),
	}

	lspReferencesParamsSchema = map[string]any{
		"uri":       requiredString("Document URI."),
		"line":      map[string]any{"type": "integer", "description": "Zero-based line of the position."},
		"character": map[string]any{"type": "integer", "description": "Zero-based UTF-16 column of the position."},
		"includeDeclaration": map[string]any{
			"type":        "boolean",
			"description": "True when the declaration of the symbol is part of the answer.",
		},
	}

	lspReferencesResultSchema = map[string]any{
		"uri":       requiredString("Document URI."),
		"locations": schemaArrayRef("lspLocation"),
	}

	lspDefinitionParamsSchema = map[string]any{
		"uri":       requiredString("Document URI."),
		"line":      map[string]any{"type": "integer", "description": "Zero-based line of the position."},
		"character": map[string]any{"type": "integer", "description": "Zero-based UTF-16 column of the position."},
	}

	lspDefinitionResultSchema = map[string]any{
		"uri":       requiredString("Document URI."),
		"locations": schemaArrayRef("lspLocation"),
	}

	lspDiagnosticsParamsSchema = map[string]any{
		"uri":         requiredString("Document URI."),
		"diagnostics": schemaArrayRef("lspDiagnostic"),
	}

	lspPositionSchema = map[string]any{
		"line":      map[string]any{"type": "integer", "description": "Zero-based line number."},
		"character": map[string]any{"type": "integer", "description": "Zero-based UTF-16 code-unit column."},
	}

	lspRangeSchema = map[string]any{
		"start": map[string]any{"$ref": "#/$defs/lspPosition"},
		"end":   map[string]any{"$ref": "#/$defs/lspPosition"},
	}

	lspLocationSchema = map[string]any{
		"uri":   requiredString("Document URI."),
		"range": map[string]any{"$ref": "#/$defs/lspRange"},
	}

	lspDiagnosticSchema = map[string]any{
		"range":    map[string]any{"$ref": "#/$defs/lspRange"},
		"severity": map[string]any{"type": "integer", "description": "Severity of the diagnostic."},
		"code":     optionalString("Code of the diagnostic."),
		"source":   optionalString("Tool that reported the diagnostic."),
		"message":  requiredString("Description of the problem."),
	}

	lspMarkupContentSchema = map[string]any{
		"kind": map[string]any{
			"type":        "string",
			"enum":        []string{string(LSPMarkupPlainText), string(LSPMarkupMarkdown)},
			"description": "Format of the text.",
		},
		"value": requiredString("The text itself."),
	}

	lspHoverSchema = map[string]any{
		"contents": map[string]any{"$ref": "#/$defs/lspMarkupContent"},
		"range": map[string]any{
			"$ref":        "#/$defs/lspRange",
			"description": "Range of the symbol under the cursor; absent when the server reports none.",
		},
	}

	lspCompletionItemSchema = map[string]any{
		"label":         requiredString("Text that the editor shows."),
		"kind":          map[string]any{"type": "integer", "description": "Category of the item."},
		"detail":        optionalString("Short description of the item."),
		"documentation": optionalString("Long description of the item."),
		"sortText":      optionalString("Text that orders the item in the list."),
		"filterText":    optionalString("Text that filters the item while the user types."),
		"insertText":    optionalString("Text that the editor inserts when it differs from the label."),
		"textEdit": map[string]any{
			"$ref":        "#/$defs/lspTextChange",
			"description": "Optional replacement text and range supplied by the language server.",
		},
		"additionalTextEdits": map[string]any{
			"type":        "array",
			"items":       map[string]any{"$ref": "#/$defs/lspTextChange"},
			"description": "Non-overlapping edits applied with the completion edit.",
		},
		"insertTextFormat": map[string]any{"type": "integer", "description": "LSP insert text format: 1 plain text, 2 snippet."},
	}

	lspTextChangeSchema = map[string]any{
		"range": map[string]any{
			"$ref":        "#/$defs/lspRange",
			"description": "Range that the change replaces; absent when the whole document changes.",
		},
		"text": requiredString("The new text."),
	}
)

func schemaArrayRef(definition string) map[string]any {
	return map[string]any{
		"type":  "array",
		"items": map[string]any{"$ref": "#/$defs/" + definition},
	}
}
