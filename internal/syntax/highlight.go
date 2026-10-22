package syntax

import "sort"

// TokenKind is a stable, editor-neutral syntax highlighting category.
type TokenKind string

const (
	TokenIdentifier      TokenKind = "identifier"
	TokenKeyword         TokenKind = "keyword"
	TokenOperator        TokenKind = "operator"
	TokenNumber          TokenKind = "number"
	TokenBoolean         TokenKind = "boolean"
	TokenString          TokenKind = "string"
	TokenStringDelimiter TokenKind = "stringDelimiter"
	TokenStringPrefix    TokenKind = "stringPrefix"
	TokenStringEscape    TokenKind = "stringEscape"
	TokenCommand         TokenKind = "command"
	TokenCharacter       TokenKind = "character"
	TokenComment         TokenKind = "comment"
	TokenInterpolation   TokenKind = "interpolation"
	TokenMacro           TokenKind = "macro"
	TokenPunctuation     TokenKind = "punctuation"
	TokenError           TokenKind = "error"
)

// Token is one highlighted source span. Ranges are half-open and use
// zero-based UTF-8 byte offsets and byte columns.
type Token struct {
	Kind  TokenKind `json:"kind"`
	Text  string    `json:"text"`
	Range Range     `json:"range"`
}

// Highlights returns lexer-level tokens for the parsed document. Tokens are
// sorted by their source ranges and do not overlap. In interpolated strings,
// the dollar marker has TokenInterpolation and the embedded Julia expression
// receives its ordinary token categories.
func (d *Document) Highlights() []Token {
	if d == nil {
		return nil
	}
	tokens := make([]Token, 0)
	collectTokens(d.root, d.source, modeNormal, &tokens)
	sort.SliceStable(tokens, func(i, j int) bool {
		if tokens[i].Range.StartByte == tokens[j].Range.StartByte {
			return tokens[i].Range.EndByte < tokens[j].Range.EndByte
		}
		return tokens[i].Range.StartByte < tokens[j].Range.StartByte
	})
	return tokens
}

// Highlight parses source and returns its highlighting tokens and parser
// diagnostics in one call.
func Highlight(source string) ([]Token, []Diagnostic, error) {
	document, err := Parse(source)
	if err != nil {
		return nil, nil, err
	}
	return document.Highlights(), document.Diagnostics(), nil
}

type lexicalMode uint8

const (
	modeNormal lexicalMode = iota
	modeString
	modeCommand
)

func collectTokens(node Node, source string, mode lexicalMode, tokens *[]Token) {
	switch node.Kind {
	case "line_comment", "block_comment":
		appendToken(tokens, source, TokenComment, node.Range)
		return
	case "character_literal":
		appendToken(tokens, source, TokenCharacter, node.Range)
		return
	case "string_interpolation", "interpolation_expression":
		for _, child := range node.Children {
			if child.Kind == "$" {
				appendToken(tokens, source, TokenInterpolation, child.Range)
				continue
			}
			collectTokens(child, source, modeNormal, tokens)
		}
		return
	case "string_literal", "prefixed_string_literal":
		collectStringChildren(node, source, modeString, tokens)
		return
	case "command_literal", "prefixed_command_literal":
		collectStringChildren(node, source, modeCommand, tokens)
		return
	}

	if node.Kind == "macro_identifier" {
		appendToken(tokens, source, TokenMacro, node.Range)
		return
	}
	if node.IsError {
		appendToken(tokens, source, TokenError, node.Range)
		return
	}
	if len(node.Children) == 0 {
		if kind, ok := classifyLeaf(node, source, mode); ok {
			appendToken(tokens, source, kind, node.Range)
		}
		return
	}
	for _, child := range node.Children {
		collectTokens(child, source, mode, tokens)
	}
}

func collectStringChildren(node Node, source string, mode lexicalMode, tokens *[]Token) {
	firstToken := len(*tokens)
	for _, child := range node.Children {
		if child.Field == "prefix" {
			appendToken(tokens, source, TokenStringPrefix, child.Range)
			continue
		}
		collectTokens(child, source, mode, tokens)
	}
	if delimiter, r, ok := terminalDelimiter(node.Range, source, mode); ok {
		found := false
		for _, token := range (*tokens)[firstToken:] {
			if token.Kind == TokenStringDelimiter && token.Range.StartByte == r.StartByte && token.Range.EndByte == r.EndByte {
				found = true
				break
			}
		}
		if !found && delimiter != "" {
			appendToken(tokens, source, TokenStringDelimiter, r)
		}
	}
}

func terminalDelimiter(r Range, source string, mode lexicalMode) (string, Range, bool) {
	end := r.EndByte
	if end < 0 || end > len(source) {
		return "", Range{}, false
	}
	delimiter := ""
	if mode == modeCommand {
		if end > r.StartByte && source[end-1:end] == "`" {
			delimiter = "`"
		}
	} else if mode == modeString {
		switch {
		case end-r.StartByte >= 3 && source[end-3:end] == `"""`:
			delimiter = `"""`
		case end > r.StartByte && source[end-1:end] == `"`:
			delimiter = `"`
		}
	}
	if delimiter == "" {
		return "", Range{}, false
	}
	start := end - len(delimiter)
	return delimiter, Range{
		StartByte: start,
		EndByte:   end,
		Start:     Position{Line: r.End.Line, Column: r.End.Column - len(delimiter)},
		End:       r.End,
	}, true
}

func classifyLeaf(node Node, source string, mode lexicalMode) (TokenKind, bool) {
	text := textFor(source, node.Range)
	if text == "" {
		return "", false
	}
	if mode != modeNormal {
		switch node.Kind {
		case "content":
			if mode == modeCommand {
				return TokenCommand, true
			}
			return TokenString, true
		case "escape_sequence":
			return TokenStringEscape, true
		}
		if isStringDelimiter(text) {
			return TokenStringDelimiter, true
		}
	}

	switch node.Kind {
	case "identifier":
		return TokenIdentifier, true
	case "integer_literal", "float_literal":
		return TokenNumber, true
	case "boolean_literal":
		return TokenBoolean, true
	case "character_literal":
		return TokenCharacter, true
	case "operator":
		return TokenOperator, true
	case "escape_sequence":
		return TokenStringEscape, true
	case "ERROR":
		return TokenError, true
	}

	if isKeyword(text) {
		return TokenKeyword, true
	}
	if isOperator(text) {
		return TokenOperator, true
	}
	if isPunctuation(text) {
		return TokenPunctuation, true
	}
	if node.Named {
		return TokenIdentifier, true
	}
	return "", false
}

func appendToken(tokens *[]Token, source string, kind TokenKind, r Range) {
	if r.StartByte < 0 || r.EndByte <= r.StartByte || r.EndByte > len(source) {
		return
	}
	*tokens = append(*tokens, Token{Kind: kind, Text: textFor(source, r), Range: r})
}

func isStringDelimiter(text string) bool {
	return text == "\"" || text == "\"\"\"" || text == "`"
}

func isKeyword(text string) bool {
	_, ok := juliaKeywords[text]
	return ok
}

func isOperator(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		switch r {
		case '+', '-', '*', '/', '\\', '^', '%', '=', '<', '>', '!', '&', '|', '~', ':', '?', '$', '.', '→', '←', '∈', '∉', '≤', '≥', '≠', '×', '÷':
		default:
			return false
		}
	}
	return true
}

func isPunctuation(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		switch r {
		case '(', ')', '[', ']', '{', '}', ',', ';', '\'', '"', '`':
		default:
			return false
		}
	}
	return true
}

var juliaKeywords = map[string]struct{}{
	"abstract": {}, "baremodule": {}, "begin": {}, "break": {}, "catch": {}, "const": {},
	"continue": {}, "do": {}, "else": {}, "elseif": {}, "end": {}, "export": {}, "finally": {},
	"for": {}, "function": {}, "global": {}, "if": {}, "import": {}, "let": {}, "local": {},
	"macro": {}, "module": {}, "mutable": {}, "primitive": {}, "public": {}, "quote": {},
	"return": {}, "struct": {}, "try": {}, "using": {}, "while": {}, "where": {}, "in": {},
	"isa": {}, "outer": {}, "true": {}, "false": {},
}
