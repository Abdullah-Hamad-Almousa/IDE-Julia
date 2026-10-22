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
		"workspaceOpenParams":  schemaObject(workspaceOpenParamsSchema),
		"workspaceOpenResult":  schemaObject(workspaceOpenResultSchema),
		"workspaceCloseParams": schemaObject(workspaceCloseParamsSchema),
		"documentOpenParams":   schemaObject(documentOpenParamsSchema),
		"documentOpenResult":   schemaObject(documentOpenResultSchema),
		"documentCloseParams":  schemaObject(documentCloseParamsSchema),
		"documentChangeParams": schemaObject(documentChangeParamsSchema),
		"documentSaveParams":   schemaObject(documentSaveParamsSchema),
		"textSearchParams":     schemaObject(textSearchParamsSchema),
		"textSearchResult":     schemaObject(textSearchResultSchema),
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
)
