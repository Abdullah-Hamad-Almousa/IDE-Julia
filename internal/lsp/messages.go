package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// This file holds the wire types of the subset of the Language Server
// Protocol 3.17 that jide speaks. The types are private to the package:
// the answers are mapped onto the payload objects of the jide protocol
// before they leave the client.

// message is a JSON-RPC 2.0 message of the base protocol.
//
// A message with a method and without an identifier is a notification, a
// message with a method and an identifier is a request of the server and a
// message without a method is the answer of the server.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

// responseError is the error object of a JSON-RPC response.
type responseError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// outgoingRequest is a request of the client.
type outgoingRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// outgoingNotification is a notification of the client.
type outgoingNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// outgoingResponse is the answer of the client to a request of the server.
type outgoingResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

// position is a zero-based position of the language server protocol. The
// character counts UTF-16 code units, the unit of the JavaScript string
// type.
type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// lspRange is a half-open range of the language server protocol.
type lspRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

// lspLocation is a range inside a document.
type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

// lspDiagnostic is a problem that the server reports for a document.
type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity,omitempty"`
	Code     string   `json:"code,omitempty"`
	Source   string   `json:"source,omitempty"`
	Message  string   `json:"message"`
}

// lspMarkupContent is the formatted documentation of a symbol.
type lspMarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// lspHover is the answer of a hover request.
type lspHover struct {
	Contents json.RawMessage `json:"contents"`
	Range    *lspRange       `json:"range,omitempty"`
}

// lspCompletionItem is one entry of a completion list.
type lspCompletionItem struct {
	Label               string          `json:"label"`
	Kind                int             `json:"kind,omitempty"`
	Detail              string          `json:"detail,omitempty"`
	Documentation       json.RawMessage `json:"documentation,omitempty"`
	SortText            string          `json:"sortText,omitempty"`
	FilterText          string          `json:"filterText,omitempty"`
	InsertText          string          `json:"insertText,omitempty"`
	InsertTextFormat    int             `json:"insertTextFormat,omitempty"`
	TextEdit            *lspTextEdit    `json:"textEdit,omitempty"`
	AdditionalTextEdits []lspTextEdit   `json:"additionalTextEdits,omitempty"`
}

type lspTextEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

type renameParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     position               `json:"position"`
	NewName      string                 `json:"newName"`
}

type lspWorkspaceEdit struct {
	Changes         map[string][]lspTextEdit `json:"changes,omitempty"`
	DocumentChanges []json.RawMessage        `json:"documentChanges,omitempty"`
}

type lspTextDocumentEdit struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version"`
	} `json:"textDocument"`
	Edits []lspTextEdit `json:"edits"`
}

// lspCompletionList is the answer of a completion request.
type lspCompletionList struct {
	IsIncomplete bool                `json:"isIncomplete"`
	Items        []lspCompletionItem `json:"items"`
}

// textDocumentIdentifier identifies a document.
type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

// versionedTextDocumentIdentifier identifies a version of a document.
type versionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// textDocumentItem is a document that the client opens.
type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// contentChange is one change of a document. A change without a range
// replaces the whole document.
type contentChange struct {
	Range *lspRange `json:"range,omitempty"`
	Text  string    `json:"text"`
}

// didOpenParams are the parameters of the didOpen notification.
type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

// didChangeParams are the parameters of the didChange notification.
type didChangeParams struct {
	TextDocument   versionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []contentChange                 `json:"contentChanges"`
}

// didSaveParams are the parameters of the didSave notification.
type didSaveParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Text         string                 `json:"text,omitempty"`
}

// didCloseParams are the parameters of the didClose notification.
type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

// textDocumentPositionParams are the parameters of the requests that ask
// the server about a position of a document.
type textDocumentPositionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     position               `json:"position"`
}

// referenceParams are the parameters of the references request.
type referenceParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     position               `json:"position"`
	Context      referenceContext       `json:"context"`
}

// referenceContext selects the references that the server reports.
type referenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// publishDiagnosticsParams are the parameters of the diagnostics
// notification.
type publishDiagnosticsParams struct {
	URI         string          `json:"uri"`
	Diagnostics []lspDiagnostic `json:"diagnostics"`
}

// initializeParams are the parameters of the initialize request.
type initializeParams struct {
	ProcessID             int                `json:"processId"`
	RootURI               string             `json:"rootUri,omitempty"`
	RootPath              string             `json:"rootPath,omitempty"`
	Capabilities          clientCapabilities `json:"capabilities"`
	ClientInfo            clientInfo         `json:"clientInfo"`
	WorkspaceFolders      []workspaceFolder  `json:"workspaceFolders,omitempty"`
	InitializationOptions map[string]any     `json:"initializationOptions,omitempty"`
	Trace                 string             `json:"trace,omitempty"`
}

// clientInfo names the client of the session.
type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// workspaceFolder is one root folder of the session.
type workspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// initializeResult is the answer of the initialize request.
type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   *serverInfo        `json:"serverInfo,omitempty"`
}

// serverInfo names the server of the session.
type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// serverCapabilities describes the features that the server supports.
type serverCapabilities struct {
	TextDocumentSync   json.RawMessage `json:"textDocumentSync,omitempty"`
	HoverProvider      json.RawMessage `json:"hoverProvider,omitempty"`
	CompletionProvider json.RawMessage `json:"completionProvider,omitempty"`
	ReferencesProvider json.RawMessage `json:"referencesProvider,omitempty"`
	DefinitionProvider json.RawMessage `json:"definitionProvider,omitempty"`
	RenameProvider     json.RawMessage `json:"renameProvider,omitempty"`
}

// clientCapabilities describes the features that the engine supports.
type clientCapabilities struct {
	TextDocument textDocumentCapabilities `json:"textDocument"`
	Workspace    workspaceCapabilities    `json:"workspace"`
}

// textDocumentCapabilities describes the document features that the engine
// supports.
type textDocumentCapabilities struct {
	Synchronization    synchronizationCapabilities    `json:"synchronization"`
	Hover              hoverCapabilities              `json:"hover"`
	Completion         completionCapabilities         `json:"completion"`
	References         referenceCapabilities          `json:"references"`
	Definition         definitionCapabilities         `json:"definition"`
	Rename             renameCapabilities             `json:"rename"`
	PublishDiagnostics publishDiagnosticsCapabilities `json:"publishDiagnostics"`
}

// synchronizationCapabilities describes the document synchronization that
// the engine supports.
type synchronizationCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration"`
	DidSave             bool `json:"didSave"`
	WillSave            bool `json:"willSave"`
	WillSaveWaitUntil   bool `json:"willSaveWaitUntil"`
}

// hoverCapabilities describes the hover support of the engine.
type hoverCapabilities struct {
	ContentFormat []string `json:"contentFormat"`
}

// completionCapabilities describes the completion support of the engine.
type completionCapabilities struct {
	CompletionItem completionItemCapabilities `json:"completionItem"`
}

// completionItemCapabilities describes the completion items that the engine
// understands.
type completionItemCapabilities struct {
	DocumentationFormat []string `json:"documentationFormat"`
	SnippetSupport      bool     `json:"snippetSupport"`
}

// referenceCapabilities describes the reference support of the engine.
type referenceCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration"`
}

// definitionCapabilities describes the definition support of the engine.
type definitionCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration"`
}

type renameCapabilities struct {
	PrepareSupport bool `json:"prepareSupport"`
}

// publishDiagnosticsCapabilities describes the diagnostics that the engine
// understands.
type publishDiagnosticsCapabilities struct {
	RelatedInformation bool `json:"relatedInformation"`
}

// workspaceCapabilities describes the workspace features that the engine
// supports.
type workspaceCapabilities struct {
	WorkspaceFolders bool  `json:"workspaceFolders"`
	Configuration    *bool `json:"configuration,omitempty"`
}

// defaultCapabilities returns the capabilities that the engine announces.
//
// The engine synchronizes the documents incrementally, asks for the hover,
// the completion, the reference and the definition of a position and
// understands the diagnostics of the server.
func defaultCapabilities() clientCapabilities {
	return clientCapabilities{
		TextDocument: textDocumentCapabilities{
			Synchronization: synchronizationCapabilities{
				DidSave:           true,
				WillSave:          true,
				WillSaveWaitUntil: false,
			},
			Hover: hoverCapabilities{ContentFormat: []string{"markdown", "plaintext"}},
			Completion: completionCapabilities{CompletionItem: completionItemCapabilities{
				DocumentationFormat: []string{"markdown", "plaintext"},
			}},
			References:         referenceCapabilities{},
			Definition:         definitionCapabilities{},
			Rename:             renameCapabilities{PrepareSupport: false},
			PublishDiagnostics: publishDiagnosticsCapabilities{RelatedInformation: true},
		},
		Workspace: workspaceCapabilities{WorkspaceFolders: true},
	}
}

// decodeMarkup reads the markup of a hover or of a completion item.
//
// The protocol allows a plain string, a markup object and a list of the two
// for the documentation of a symbol. The engine keeps the text and the kind
// of the markup.
func decodeMarkup(raw json.RawMessage) (kind, value string, err error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", "", nil
	}
	switch trimmed[0] {
	case '"':
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return "", "", fmt.Errorf("lsp: read the markup: %w", err)
		}
		return "plaintext", text, nil
	case '{':
		var content lspMarkupContent
		if err := json.Unmarshal(trimmed, &content); err != nil {
			return "", "", fmt.Errorf("lsp: read the markup: %w", err)
		}
		if content.Kind == "" {
			content.Kind = "plaintext"
		}
		return content.Kind, content.Value, nil
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return "", "", fmt.Errorf("lsp: read the markup list: %w", err)
		}
		parts := make([]string, 0, len(items))
		for _, item := range items {
			_, itemValue, err := decodeMarkup(item)
			if err != nil {
				return "", "", err
			}
			if itemValue == "" {
				continue
			}
			parts = append(parts, itemValue)
		}
		return "plaintext", strings.Join(parts, "\n"), nil
	default:
		return "", "", fmt.Errorf("lsp: the markup %s is neither a string nor an object", string(trimmed))
	}
}
