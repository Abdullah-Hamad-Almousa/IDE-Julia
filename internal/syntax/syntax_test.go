package syntax

import (
	"encoding/json"
	"testing"

	jide "github.com/jide-dev/jide/proto/jide"
)

func TestParseProvidesDetachedTreeAndSourceRanges(t *testing.T) {
	source := "module Demo\nconst X = 1\nend\n"
	document, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := document.Source(); got != source {
		t.Fatalf("Source() = %q, want %q", got, source)
	}
	root := document.Root()
	if root.Kind != "source_file" {
		t.Fatalf("root kind = %q, want source_file", root.Kind)
	}
	if got := document.Text(root.Range); got != source {
		t.Errorf("root text = %q, want %q", got, source)
	}
	if document.SExpression() == "" {
		t.Error("SExpression() is empty")
	}
	if got := document.Diagnostics(); len(got) != 0 {
		t.Errorf("valid source has diagnostics: %+v", got)
	}
	root.Children[0].Kind = "modified"
	if got := document.Root().Children[0].Kind; got == "modified" {
		t.Error("Root() exposed the document's mutable internal tree")
	}
}

func TestParseReportsIncompleteSourceWithoutFailing(t *testing.T) {
	document, err := Parse("function incomplete(x\n  x + 1\n")
	if err != nil {
		t.Fatalf("Parse() returned an error for recoverable syntax: %v", err)
	}
	if diagnostics := document.Diagnostics(); len(diagnostics) == 0 {
		t.Fatalf("incomplete source has no diagnostics; tree: %s", document.SExpression())
	}
}

func TestHighlightsJuliaTokensAndInterpolations(t *testing.T) {
	source := "# heading\nfunction fn(x)\n    value = \"hi $name $(value + 1) escaped \\$value\"\n    raw\"literal $value\"\n    cmd = `echo $value`\n    return value\nend\n"
	document, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	tokens := document.Highlights()
	count := func(kind TokenKind, text string) int {
		t.Helper()
		found := 0
		for _, token := range tokens {
			if token.Kind == kind && token.Text == text {
				found++
			}
		}
		return found
	}
	if count(TokenComment, "# heading") != 1 {
		t.Error("line comment was not highlighted")
	}
	if count(TokenIdentifier, "value") == 0 || count(TokenOperator, "=") == 0 || count(TokenNumber, "1") == 0 {
		t.Error("identifier/operator/number tokens are missing")
	}
	if count(TokenInterpolation, "$") != 3 {
		t.Errorf("interpolation markers = %d, want 3 (two in string, one in command)", count(TokenInterpolation, "$"))
	}
	if count(TokenStringPrefix, "raw") != 1 {
		t.Error("raw string prefix was not tokenized")
	}
	if count(TokenStringDelimiter, `"`) != 4 {
		t.Errorf("ordinary and raw string delimiters = %d, want 4", count(TokenStringDelimiter, `"`))
	}
	if count(TokenStringDelimiter, "`") != 2 {
		t.Errorf("command delimiters = %d, want 2", count(TokenStringDelimiter, "`"))
	}
	if count(TokenCommand, "echo ") != 1 {
		t.Error("command literal content was not tokenized")
	}
	if count(TokenKeyword, "function") != 1 || count(TokenKeyword, "return") != 1 || count(TokenKeyword, "end") != 1 {
		t.Error("Julia keywords were not tokenized")
	}
	for i := 1; i < len(tokens); i++ {
		if tokens[i].Range.StartByte < tokens[i-1].Range.EndByte {
			t.Errorf("highlight tokens overlap or are unordered: %+v then %+v", tokens[i-1], tokens[i])
		}
	}
}

func TestHighlightsTripleStringsEscapesAndUnicodeByteColumns(t *testing.T) {
	source := "α = \"\"\"first\nsecond\\n\"\"\"\n"
	document, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	tokens := document.Highlights()
	foundUnicode := false
	foundTripleDelimiter := 0
	foundEscape := false
	for _, token := range tokens {
		switch {
		case token.Kind == TokenIdentifier && token.Text == "α":
			foundUnicode = token.Range.Start.Column == 0 && token.Range.End.Column == 2
		case token.Kind == TokenStringDelimiter && token.Text == `"""`:
			foundTripleDelimiter++
		case token.Kind == TokenStringEscape:
			foundEscape = true
		}
	}
	if !foundUnicode {
		t.Error("Unicode identifier range did not use UTF-8 byte columns")
	}
	if foundTripleDelimiter != 2 {
		t.Errorf("triple string delimiters = %d, want 2", foundTripleDelimiter)
	}
	if !foundEscape {
		t.Error("string escape was not tokenized")
	}
}

func TestDocumentSymbolsIncludeNestedDeclarations(t *testing.T) {
	source := `module Outer
const ANSWER = 42
mutable struct Pair{T}
    left::T
    right::T
end
abstract type AbstractThing end
primitive type Bits 8 end
function outer(x)
    local_value = x + 1
    function inner(y)
        y + local_value
    end
    inner(x)
end
macro trace(ex)
    esc(ex)
end
short(x) = x + 1
end
`
	document, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	symbols := document.Symbols()
	if len(symbols) != 1 || symbols[0].Name != "Outer" || symbols[0].Kind != SymbolModule {
		t.Fatalf("top-level symbols = %+v, want Outer module", symbols)
	}
	outer := symbols[0]
	assertSymbol := func(parent DocumentSymbol, name string, kind SymbolKind) DocumentSymbol {
		t.Helper()
		for _, symbol := range parent.Children {
			if symbol.Name == name && symbol.Kind == kind {
				return symbol
			}
		}
		t.Fatalf("%s does not contain %s %q; children: %+v", parent.Name, kind, name, parent.Children)
		return DocumentSymbol{}
	}
	assertSymbol(outer, "ANSWER", SymbolConstant)
	pair := assertSymbol(outer, "Pair", SymbolStruct)
	assertSymbol(pair, "left", SymbolField)
	assertSymbol(pair, "right", SymbolField)
	assertSymbol(outer, "AbstractThing", SymbolType)
	assertSymbol(outer, "Bits", SymbolType)
	function := assertSymbol(outer, "outer", SymbolFunction)
	assertSymbol(function, "local_value", SymbolVariable)
	assertSymbol(function, "inner", SymbolFunction)
	assertSymbol(outer, "@trace", SymbolMacro)
	assertSymbol(outer, "short", SymbolFunction)
	for _, symbol := range symbols {
		if symbol.SelectionRange.StartByte < symbol.Range.StartByte || symbol.SelectionRange.EndByte > symbol.Range.EndByte {
			t.Errorf("selection range is outside symbol range: %+v", symbol)
		}
	}
}

func TestFoldRangesCoverBlocksCommentsAndStrings(t *testing.T) {
	source := "function f(x)\n#= comment\nmore =#\ntext = \"\"\"line one\nline two\"\"\"\n  x + 1\nend\n"
	document, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	folds := document.FoldRanges()
	want := map[FoldKind]bool{FoldBlock: false, FoldComment: false, FoldString: false}
	for _, fold := range folds {
		want[fold.Kind] = true
		if fold.EndLine <= fold.StartLine {
			t.Errorf("non-multiline fold: %+v", fold)
		}
	}
	for kind, found := range want {
		if !found {
			t.Errorf("no %s fold found: %+v", kind, folds)
		}
	}
}

func TestProtocolAdaptersAreJSONMarshalable(t *testing.T) {
	document, err := Parse("module Demo\nfunction run(x)\n x + 1\nend\nend\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	results := []any{
		document.ProtocolHighlights("file:///Demo.jl"),
		document.ProtocolSymbols("file:///Demo.jl"),
		document.ProtocolFolds("file:///Demo.jl"),
	}
	for _, result := range results {
		if _, err := json.Marshal(result); err != nil {
			t.Errorf("protocol result %T is not JSON-marshable: %v", result, err)
		}
	}
	got := document.ProtocolHighlights("file:///Demo.jl")
	if got.URI != "file:///Demo.jl" || len(got.Tokens) == 0 {
		t.Errorf("protocol highlight result = %+v", got)
	}
	var _ jide.SyntaxHighlightsResult = got
}
