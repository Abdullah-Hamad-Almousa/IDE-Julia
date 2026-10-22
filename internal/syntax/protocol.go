package syntax

import jide "github.com/jide-dev/jide/proto/jide"

// ProtocolHighlights converts editor-neutral tokens and diagnostics into the
// jide JSON-RPC result for the supplied document URI.
func (d *Document) ProtocolHighlights(uri string) jide.SyntaxHighlightsResult {
	result := jide.SyntaxHighlightsResult{URI: uri}
	if d == nil {
		return result
	}
	tokens := d.Highlights()
	result.Tokens = make([]jide.SyntaxToken, 0, len(tokens))
	for _, token := range tokens {
		result.Tokens = append(result.Tokens, jide.SyntaxToken{
			Kind:  jide.SyntaxTokenKind(token.Kind),
			Text:  token.Text,
			Range: protocolRange(token.Range),
		})
	}
	result.Diagnostics = protocolDiagnostics(d.diagnostics)
	return result
}

// ProtocolSymbols converts document symbols and parser diagnostics into the
// jide JSON-RPC result for the supplied document URI.
func (d *Document) ProtocolSymbols(uri string) jide.DocumentSymbolsResult {
	result := jide.DocumentSymbolsResult{URI: uri}
	if d == nil {
		return result
	}
	symbols := d.Symbols()
	result.Symbols = make([]jide.DocumentSymbol, 0, len(symbols))
	for _, symbol := range symbols {
		result.Symbols = append(result.Symbols, protocolSymbol(symbol))
	}
	result.Diagnostics = protocolDiagnostics(d.diagnostics)
	return result
}

// ProtocolFolds converts fold ranges and parser diagnostics into the jide
// JSON-RPC result for the supplied document URI.
func (d *Document) ProtocolFolds(uri string) jide.SyntaxFoldsResult {
	result := jide.SyntaxFoldsResult{URI: uri}
	if d == nil {
		return result
	}
	folds := d.FoldRanges()
	result.Folds = make([]jide.SyntaxFoldRange, 0, len(folds))
	for _, fold := range folds {
		result.Folds = append(result.Folds, jide.SyntaxFoldRange{
			Kind:      jide.SyntaxFoldKind(fold.Kind),
			StartLine: fold.StartLine,
			EndLine:   fold.EndLine,
			Range:     protocolRange(fold.Range),
		})
	}
	result.Diagnostics = protocolDiagnostics(d.diagnostics)
	return result
}

func protocolSymbol(symbol DocumentSymbol) jide.DocumentSymbol {
	converted := jide.DocumentSymbol{
		Name:           symbol.Name,
		Kind:           jide.DocumentSymbolKind(symbol.Kind),
		Range:          protocolRange(symbol.Range),
		SelectionRange: protocolRange(symbol.SelectionRange),
	}
	if symbol.Children != nil {
		converted.Children = make([]jide.DocumentSymbol, 0, len(symbol.Children))
		for _, child := range symbol.Children {
			converted.Children = append(converted.Children, protocolSymbol(child))
		}
	}
	return converted
}

func protocolDiagnostics(diagnostics []Diagnostic) []jide.SyntaxDiagnostic {
	if diagnostics == nil {
		return nil
	}
	converted := make([]jide.SyntaxDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		converted = append(converted, jide.SyntaxDiagnostic{
			Message: diagnostic.Message,
			Range:   protocolRange(diagnostic.Range),
		})
	}
	return converted
}

func protocolRange(r Range) jide.SyntaxRange {
	return jide.SyntaxRange{
		StartByte: r.StartByte,
		EndByte:   r.EndByte,
		Start:     jide.SyntaxPosition{Line: r.Start.Line, Column: r.Start.Column},
		End:       jide.SyntaxPosition{Line: r.End.Line, Column: r.End.Column},
	}
}
