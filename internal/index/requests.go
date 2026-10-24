package index

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/jide-dev/jide/internal/syntax"
	jide "github.com/jide-dev/jide/proto/jide"
)

// SearchRequest handles the typed text-search payload for an open workspace.
func (idx *Index) SearchRequest(ctx context.Context, params jide.TextSearchParams) (jide.TextSearchResult, error) {
	return idx.Search(ctx, params.Query, params.Limit)
}

// SearchSymbolsRequest is the protocol-facing alias for SearchRequest.
func (idx *Index) SearchTextRequest(ctx context.Context, params jide.TextSearchParams) (jide.TextSearchResult, error) {
	return idx.SearchRequest(ctx, params)
}

// WorkspaceSymbolsRequest handles the typed workspace-symbol payload.
func (idx *Index) WorkspaceSymbolsRequest(ctx context.Context, params jide.WorkspaceSymbolsParams) (jide.WorkspaceSymbolsResult, error) {
	return idx.SearchSymbols(ctx, params.Query, params.Limit)
}

// SearchSymbolsRequest searches declarations using the protocol payload.
func (idx *Index) SearchSymbolsRequest(ctx context.Context, params jide.WorkspaceSymbolsParams) (jide.WorkspaceSymbolsResult, error) {
	return idx.WorkspaceSymbolsRequest(ctx, params)
}

// DefinitionRequest resolves the declaration at a typed LSP position using
// the syntax index. It is the lexical fallback, not Julia semantic analysis.
func (idx *Index) DefinitionRequest(ctx context.Context, params jide.LSPDefinitionParams) (jide.LSPDefinitionResult, error) {
	locations, err := idx.DefinitionAt(ctx, params.URI, params.Line, params.Character)
	return jide.LSPDefinitionResult{URI: params.URI, Locations: locations}, err
}

// DocumentSymbolsRequest parses current editor text and returns its typed
// document-symbol payload without relying on disk state.
func DocumentSymbolsRequest(params jide.DocumentSymbolsParams) (jide.DocumentSymbolsResult, error) {
	if !utf8.ValidString(params.Text) {
		return jide.DocumentSymbolsResult{}, fmt.Errorf("index: document is not valid UTF-8 text")
	}
	parsed, err := syntax.Parse(params.Text)
	if err != nil {
		return jide.DocumentSymbolsResult{}, fmt.Errorf("index: parse document symbols: %w", err)
	}
	return parsed.ProtocolSymbols(params.URI), nil
}
