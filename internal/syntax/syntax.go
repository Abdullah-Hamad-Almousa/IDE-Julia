package syntax

import (
	"fmt"

	juliagrammar "github.com/jide-dev/jide/internal/syntax/juliagrammar"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Position is a zero-based source position. Column counts UTF-8 bytes, as it
// does in Tree-sitter and the JSON-RPC protocol.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Range is a half-open source range. Byte offsets and columns are UTF-8 byte
// counts; end positions do not include the byte at EndByte.
type Range struct {
	StartByte int      `json:"startByte"`
	EndByte   int      `json:"endByte"`
	Start     Position `json:"start"`
	End       Position `json:"end"`
}

// Node is a value representation of one node in the Julia syntax tree. It is
// detached from Tree-sitter's native memory and remains valid after Parse
// returns.
type Node struct {
	Kind     string `json:"kind"`
	Field    string `json:"field,omitempty"`
	Named    bool   `json:"named"`
	HasError bool   `json:"hasError,omitempty"`
	IsError  bool   `json:"isError,omitempty"`
	Missing  bool   `json:"missing,omitempty"`
	Range    Range  `json:"range"`
	Children []Node `json:"children,omitempty"`
}

// Diagnostic describes syntax recovery performed by Tree-sitter. Parse
// returns a document even when the source contains errors; callers can use
// Diagnostics to underline incomplete or malformed source while editing.
type Diagnostic struct {
	Message string `json:"message"`
	Range   Range  `json:"range"`
}

// Document is an immutable parse result for a Julia source string.
type Document struct {
	source      string
	root        Node
	sexpression string
	diagnostics []Diagnostic
}

// Parse parses Julia source. Grammar errors are reported in the returned
// document's Diagnostics; the error return is reserved for parser setup or
// execution failures.
func Parse(source string) (*Document, error) {
	parser := tree_sitter.NewParser()
	if parser == nil {
		return nil, fmt.Errorf("syntax: create Tree-sitter parser")
	}
	defer parser.Close()

	language := tree_sitter.NewLanguage(juliagrammar.Language())
	if err := parser.SetLanguage(language); err != nil {
		return nil, fmt.Errorf("syntax: set Julia grammar: %w", err)
	}

	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		return nil, fmt.Errorf("syntax: Tree-sitter returned no syntax tree")
	}
	defer tree.Close()

	root := tree.RootNode()
	document := &Document{
		source:      source,
		root:        convertNode(root, ""),
		sexpression: root.ToSexp(),
	}
	collectDiagnostics(document.root, &document.diagnostics)
	return document, nil
}

// Source returns the source string used to create the document.
func (d *Document) Source() string {
	if d == nil {
		return ""
	}
	return d.source
}

// Root returns a detached copy of the root syntax node.
func (d *Document) Root() Node {
	if d == nil {
		return Node{}
	}
	return cloneNode(d.root)
}

// SExpression returns Tree-sitter's canonical parenthesized representation of
// the parse tree. It is primarily useful in golden tests and diagnostics.
func (d *Document) SExpression() string {
	if d == nil {
		return ""
	}
	return d.sexpression
}

// Diagnostics returns a copy of the syntax recovery diagnostics.
func (d *Document) Diagnostics() []Diagnostic {
	if d == nil {
		return nil
	}
	return append([]Diagnostic(nil), d.diagnostics...)
}

// Text returns the source bytes covered by r. Invalid or out-of-bounds ranges
// produce an empty string rather than a panic.
func (d *Document) Text(r Range) string {
	if d == nil || r.StartByte < 0 || r.EndByte < r.StartByte || r.EndByte > len(d.source) {
		return ""
	}
	return d.source[r.StartByte:r.EndByte]
}

func convertNode(node *tree_sitter.Node, field string) Node {
	r := node.Range()
	converted := Node{
		Kind:     node.Kind(),
		Field:    field,
		Named:    node.IsNamed(),
		HasError: node.HasError(),
		IsError:  node.IsError(),
		Missing:  node.IsMissing(),
		Range: Range{
			StartByte: int(r.StartByte),
			EndByte:   int(r.EndByte),
			Start:     Position{Line: int(r.StartPoint.Row), Column: int(r.StartPoint.Column)},
			End:       Position{Line: int(r.EndPoint.Row), Column: int(r.EndPoint.Column)},
		},
	}
	converted.Children = make([]Node, 0, node.ChildCount())
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		converted.Children = append(converted.Children, convertNode(child, node.FieldNameForChild(uint32(i))))
	}
	return converted
}

func cloneNode(node Node) Node {
	copyNode := node
	if node.Children != nil {
		copyNode.Children = make([]Node, len(node.Children))
		for i := range node.Children {
			copyNode.Children[i] = cloneNode(node.Children[i])
		}
	}
	return copyNode
}

func collectDiagnostics(node Node, diagnostics *[]Diagnostic) {
	switch {
	case node.IsError:
		*diagnostics = append(*diagnostics, Diagnostic{Message: "unexpected syntax", Range: node.Range})
	case node.Missing:
		*diagnostics = append(*diagnostics, Diagnostic{Message: "missing " + node.Kind, Range: node.Range})
	}
	for _, child := range node.Children {
		collectDiagnostics(child, diagnostics)
	}
}

func textFor(source string, r Range) string {
	if r.StartByte < 0 || r.EndByte < r.StartByte || r.EndByte > len(source) {
		return ""
	}
	return source[r.StartByte:r.EndByte]
}
