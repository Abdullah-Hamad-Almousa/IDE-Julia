package syntax

import "sort"

// FoldKind identifies the broad syntax category that can be folded.
type FoldKind string

const (
	FoldBlock   FoldKind = "block"
	FoldComment FoldKind = "comment"
	FoldString  FoldKind = "string"
)

// FoldRange is a multiline region that an editor may collapse. Line numbers
// are zero-based and inclusive; Range uses half-open byte offsets.
type FoldRange struct {
	Kind      FoldKind `json:"kind"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Range     Range    `json:"range"`
}

// FoldRanges returns multiline Julia blocks, comments, strings and commands.
func (d *Document) FoldRanges() []FoldRange {
	if d == nil {
		return nil
	}
	var folds []FoldRange
	collectFolds(d.root, &folds)
	sort.SliceStable(folds, func(i, j int) bool {
		if folds[i].StartLine == folds[j].StartLine {
			return folds[i].EndLine < folds[j].EndLine
		}
		return folds[i].StartLine < folds[j].StartLine
	})
	return folds
}

func collectFolds(node Node, folds *[]FoldRange) {
	kind, ok := foldKind(node.Kind)
	if ok {
		endLine := node.Range.End.Line
		if node.Range.End.Column == 0 && endLine > node.Range.Start.Line {
			endLine--
		}
		if endLine > node.Range.Start.Line {
			*folds = append(*folds, FoldRange{
				Kind:      kind,
				StartLine: node.Range.Start.Line,
				EndLine:   endLine,
				Range:     node.Range,
			})
		}
	}
	for _, child := range node.Children {
		collectFolds(child, folds)
	}
}

func foldKind(nodeKind string) (FoldKind, bool) {
	switch nodeKind {
	case "module_definition", "function_definition", "macro_definition", "struct_definition",
		"abstract_definition", "primitive_definition", "compound_statement", "if_statement",
		"for_statement", "while_statement", "try_statement", "let_statement", "quote_statement", "do_clause":
		return FoldBlock, true
	case "block_comment":
		return FoldComment, true
	case "string_literal", "prefixed_string_literal", "command_literal", "prefixed_command_literal":
		return FoldString, true
	default:
		return "", false
	}
}
