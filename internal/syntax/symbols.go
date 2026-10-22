package syntax

// SymbolKind classifies a Julia declaration exposed as a document symbol.
type SymbolKind string

const (
	SymbolModule   SymbolKind = "module"
	SymbolStruct   SymbolKind = "struct"
	SymbolType     SymbolKind = "type"
	SymbolFunction SymbolKind = "function"
	SymbolMacro    SymbolKind = "macro"
	SymbolConstant SymbolKind = "constant"
	SymbolVariable SymbolKind = "variable"
	SymbolField    SymbolKind = "field"
)

// DocumentSymbol is a declaration in a Julia document. Children represent
// declarations nested in the lexical body of this symbol.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Kind           SymbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// Symbols extracts common Julia declarations, including nested declarations
// and variables assigned in their enclosing scope.
func (d *Document) Symbols() []DocumentSymbol {
	if d == nil {
		return nil
	}
	return collectSymbols(d.root, "", "", d.source)
}

func collectSymbols(node Node, parentKind string, enclosingKind SymbolKind, source string) []DocumentSymbol {
	if symbol, ok := symbolForNode(node, parentKind, enclosingKind, source); ok {
		symbol.Children = collectChildrenSymbols(node, node.Kind, symbol.Kind, source)
		return []DocumentSymbol{symbol}
	}
	return collectChildrenSymbols(node, node.Kind, enclosingKind, source)
}

func collectChildrenSymbols(node Node, parentKind string, enclosingKind SymbolKind, source string) []DocumentSymbol {
	var symbols []DocumentSymbol
	for _, child := range node.Children {
		symbols = append(symbols, collectSymbols(child, parentKind, enclosingKind, source)...)
	}
	return symbols
}

func symbolForNode(node Node, parentKind string, enclosingKind SymbolKind, source string) (DocumentSymbol, bool) {
	switch node.Kind {
	case "module_definition":
		name := directName(node)
		return symbolFromName(node, SymbolModule, name, source), name != nil
	case "struct_definition":
		name := typeName(node)
		return symbolFromName(node, SymbolStruct, name, source), name != nil
	case "abstract_definition", "primitive_definition":
		name := typeName(node)
		return symbolFromName(node, SymbolType, name, source), name != nil
	case "function_definition":
		name := definitionName(node)
		return symbolFromName(node, SymbolFunction, name, source), name != nil
	case "macro_definition":
		name := definitionName(node)
		if name == nil {
			return DocumentSymbol{}, false
		}
		symbol := symbolFromName(node, SymbolMacro, name, source)
		symbol.Name = "@" + symbol.Name
		return symbol, true
	case "const_statement":
		name := assignedName(node)
		return symbolFromName(node, SymbolConstant, name, source), name != nil
	case "assignment":
		if parentKind == "const_statement" {
			return DocumentSymbol{}, false
		}
		lhs := firstNamedChild(node)
		if lhs == nil {
			return DocumentSymbol{}, false
		}
		if lhs.Kind == "call_expression" {
			name := callName(lhs)
			return symbolFromName(node, SymbolFunction, name, source), name != nil
		}
		if lhs.Kind == "identifier" {
			return symbolFromName(node, SymbolVariable, lhs, source), true
		}
	case "typed_expression":
		if enclosingKind == SymbolStruct {
			name := firstDescendant(&node, "identifier")
			return symbolFromName(node, SymbolField, name, source), name != nil
		}
	}
	return DocumentSymbol{}, false
}

func symbolFromName(node Node, kind SymbolKind, name *Node, source string) DocumentSymbol {
	if name == nil {
		return DocumentSymbol{}
	}
	return DocumentSymbol{
		Name:           textFor(source, name.Range),
		Kind:           kind,
		Range:          node.Range,
		SelectionRange: name.Range,
	}
}

func directName(node Node) *Node {
	for i := range node.Children {
		if node.Children[i].Field == "name" {
			return &node.Children[i]
		}
	}
	for i := range node.Children {
		if node.Children[i].Kind == "identifier" {
			return &node.Children[i]
		}
	}
	return nil
}

func typeName(node Node) *Node {
	for i := range node.Children {
		if node.Children[i].Kind == "type_head" {
			return firstDescendant(&node.Children[i], "identifier")
		}
	}
	return nil
}

func definitionName(node Node) *Node {
	for i := range node.Children {
		if node.Children[i].Kind == "signature" {
			for j := range node.Children[i].Children {
				if node.Children[i].Children[j].Kind == "call_expression" {
					return callName(&node.Children[i].Children[j])
				}
			}
		}
	}
	return nil
}

func callName(call *Node) *Node {
	if call == nil {
		return nil
	}
	lhs := firstNamedChild(*call)
	if lhs == nil {
		return nil
	}
	if lhs.Kind == "identifier" || lhs.Kind == "operator" {
		return lhs
	}
	if lhs.Kind == "field_expression" {
		return lastDescendant(lhs, "identifier")
	}
	return firstDescendant(lhs, "identifier")
}

func assignedName(node Node) *Node {
	for i := range node.Children {
		if node.Children[i].Kind == "assignment" {
			lhs := firstNamedChild(node.Children[i])
			if lhs != nil && lhs.Kind == "identifier" {
				return lhs
			}
		}
	}
	return nil
}

func firstNamedChild(node Node) *Node {
	for i := range node.Children {
		if node.Children[i].Named {
			return &node.Children[i]
		}
	}
	return nil
}

func firstDescendant(node *Node, kind string) *Node {
	if node == nil {
		return nil
	}
	if node.Kind == kind {
		return node
	}
	for i := range node.Children {
		if found := firstDescendant(&node.Children[i], kind); found != nil {
			return found
		}
	}
	return nil
}

func lastDescendant(node *Node, kind string) *Node {
	if node == nil {
		return nil
	}
	var found *Node
	if node.Kind == kind {
		found = node
	}
	for i := range node.Children {
		if child := lastDescendant(&node.Children[i], kind); child != nil {
			found = child
		}
	}
	return found
}
