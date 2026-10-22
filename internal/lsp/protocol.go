package lsp

import (
	"encoding/json"
	"fmt"

	jide "github.com/jide-dev/jide/proto/jide"
)

// The functions of this file adapt the answers of the language server to
// the payload objects of the jide protocol. They are the seam between the
// engine and the user interface: everything a shell needs to show about the
// semantic analysis of a document is reachable through them.

// protocolDiagnostics converts the diagnostics of the server.
func protocolDiagnostics(diagnostics []lspDiagnostic) []jide.LSPDiagnostic {
	if diagnostics == nil {
		return nil
	}
	converted := make([]jide.LSPDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		converted = append(converted, jide.LSPDiagnostic{
			Range:    protocolRangeValue(diagnostic.Range),
			Severity: jide.LSPDiagnosticSeverity(diagnostic.Severity),
			Code:     diagnostic.Code,
			Source:   diagnostic.Source,
			Message:  diagnostic.Message,
		})
	}
	return converted
}

// protocolHover converts the answer of a hover request.
//
// The answer is nil when the server has nothing to say about the position.
func protocolHover(raw json.RawMessage) (*jide.LSPHover, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var hover lspHover
	if err := json.Unmarshal(raw, &hover); err != nil {
		return nil, fmt.Errorf("lsp: read the answer of the hover request: %w", err)
	}
	kind, value, err := decodeMarkup(hover.Contents)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, nil
	}
	converted := &jide.LSPHover{
		Contents: jide.LSPMarkupContent{Kind: jide.LSPMarkupKind(kind), Value: value},
	}
	if hover.Range != nil {
		converted.Range = protocolRangePointer(*hover.Range)
	}
	return converted, nil
}

// protocolCompletion converts the answer of a completion request.
//
// The protocol answers with a list of items or with an object that holds a
// list of items and the flag that tells the client to ask again.
func protocolCompletion(raw json.RawMessage) (jide.LSPCompletionResult, error) {
	result := jide.LSPCompletionResult{Items: []jide.LSPCompletionItem{}}
	if len(raw) == 0 || string(raw) == "null" {
		return result, nil
	}
	trimmed := raw
	if len(trimmed) > 0 && trimmed[0] == ' ' {
		trimmed = trimmed[1:]
	}
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []lspCompletionItem
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return jide.LSPCompletionResult{}, fmt.Errorf("lsp: read the answer of the completion request: %w", err)
		}
		result.Items = protocolCompletionItems(items)
		return result, nil
	}
	var list lspCompletionList
	if err := json.Unmarshal(trimmed, &list); err != nil {
		return jide.LSPCompletionResult{}, fmt.Errorf("lsp: read the answer of the completion request: %w", err)
	}
	result.IsIncomplete = list.IsIncomplete
	result.Items = protocolCompletionItems(list.Items)
	return result, nil
}

// protocolCompletionItems converts the items of a completion list.
func protocolCompletionItems(items []lspCompletionItem) []jide.LSPCompletionItem {
	converted := make([]jide.LSPCompletionItem, 0, len(items))
	for _, item := range items {
		entry := jide.LSPCompletionItem{
			Label:      item.Label,
			Kind:       jide.LSPCompletionItemKind(item.Kind),
			Detail:     item.Detail,
			SortText:   item.SortText,
			FilterText: item.FilterText,
			InsertText: item.InsertText,
		}
		if len(item.Documentation) > 0 {
			_, value, err := decodeMarkup(item.Documentation)
			if err == nil {
				entry.Documentation = value
			}
		}
		converted = append(converted, entry)
	}
	return converted
}

// protocolLocations converts the answer of a references or of a definition
// request.
//
// The protocol answers with a list of locations, with a single location or
// with a list of links between a location and the origin of the symbol.
func protocolLocations(raw json.RawMessage) ([]jide.LSPLocation, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var list []lspLocation
	if err := json.Unmarshal(raw, &list); err == nil {
		return protocolLocationList(list), nil
	}
	var single lspLocation
	if err := json.Unmarshal(raw, &single); err == nil && single.URI != "" {
		return protocolLocationList([]lspLocation{single}), nil
	}
	var links []locationLink
	if err := json.Unmarshal(raw, &links); err == nil {
		locations := make([]lspLocation, 0, len(links))
		for _, link := range links {
			locations = append(locations, lspLocation{URI: link.TargetURI, Range: link.TargetRange})
		}
		return protocolLocationList(locations), nil
	}
	return nil, fmt.Errorf("lsp: read the answer of the location request: %s", string(raw))
}

// locationLink is the answer of a definition request that names the origin
// of the symbol.
type locationLink struct {
	OriginSelectionRange *lspRange `json:"originSelectionRange,omitempty"`
	TargetURI            string    `json:"targetUri"`
	TargetRange          lspRange  `json:"targetRange"`
	TargetSelectionRange lspRange  `json:"targetSelectionRange"`
}

// protocolLocationList converts a list of locations.
func protocolLocationList(locations []lspLocation) []jide.LSPLocation {
	if locations == nil {
		return nil
	}
	converted := make([]jide.LSPLocation, 0, len(locations))
	for _, location := range locations {
		converted = append(converted, jide.LSPLocation{
			URI:   location.URI,
			Range: protocolRangeValue(location.Range),
		})
	}
	return converted
}

// protocolRangeValue converts a range of the protocol.
func protocolRangeValue(source lspRange) jide.LSPRange {
	return jide.LSPRange{
		Start: jide.LSPPosition{Line: source.Start.Line, Character: source.Start.Character},
		End:   jide.LSPPosition{Line: source.End.Line, Character: source.End.Character},
	}
}

// protocolRangePointer converts an optional range of the protocol.
func protocolRangePointer(source lspRange) *jide.LSPRange {
	converted := protocolRangeValue(source)
	return &converted
}

// protocolRange converts an optional range of the jide protocol into the
// range of the protocol.
func protocolRange(source *jide.LSPRange) *lspRange {
	if source == nil {
		return nil
	}
	return &lspRange{
		Start: position{Line: source.Start.Line, Character: source.Start.Character},
		End:   position{Line: source.End.Line, Character: source.End.Character},
	}
}
