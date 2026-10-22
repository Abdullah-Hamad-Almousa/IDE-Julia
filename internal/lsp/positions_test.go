package lsp

import (
	"testing"

	jide "github.com/jide-dev/jide/proto/jide"
)

func TestOffsetOf(t *testing.T) {
	text := "abc\ndef\nghi"
	cases := []struct {
		line      int
		character int
		want      int
	}{
		{line: 0, character: 0, want: 0},
		{line: 0, character: 3, want: 3},
		{line: 1, character: 0, want: 4},
		{line: 1, character: 3, want: 7},
		{line: 2, character: 2, want: 10},
	}
	for _, testCase := range cases {
		got, err := offsetOf(text, testCase.line, testCase.character)
		if err != nil {
			t.Errorf("offsetOf(%d, %d) error = %v", testCase.line, testCase.character, err)
			continue
		}
		if got != testCase.want {
			t.Errorf("offsetOf(%d, %d) = %d, want %d", testCase.line, testCase.character, got, testCase.want)
		}
	}
}

func TestOffsetOfCountsUTF16Units(t *testing.T) {
	// The emoji needs two UTF-16 code units, so the character after it
	// starts at the third unit.
	text := "a😀b"
	got, err := offsetOf(text, 0, 3)
	if err != nil {
		t.Fatalf("offsetOf() error = %v", err)
	}
	if got != len("a😀") {
		t.Errorf("offsetOf() = %d, want %d", got, len("a😀"))
	}
}

func TestOffsetOfRejectsAPositionOutsideTheText(t *testing.T) {
	if _, err := offsetOf("abc", 3, 0); err == nil {
		t.Error("offsetOf() error = nil, want an error")
	}
	if _, err := offsetOf("abc", 0, 9); err == nil {
		t.Error("offsetOf() error = nil, want an error")
	}
	if _, err := offsetOf("abc", -1, 0); err == nil {
		t.Error("offsetOf() error = nil, want an error")
	}
}

func TestApplyChangeReplacesTheWholeDocument(t *testing.T) {
	got, err := applyChange("old text", jide.LSPTextChange{Text: "new text"})
	if err != nil {
		t.Fatalf("applyChange() error = %v", err)
	}
	if got != "new text" {
		t.Errorf("applyChange() = %q, want %q", got, "new text")
	}
}

func TestApplyChangeReplacesARange(t *testing.T) {
	change := jide.LSPTextChange{
		Range: &jide.LSPRange{
			Start: jide.LSPPosition{Line: 0, Character: 5},
			End:   jide.LSPPosition{Line: 0, Character: 8},
		},
		Text: "XYZ",
	}
	got, err := applyChange("hello world", change)
	if err != nil {
		t.Fatalf("applyChange() error = %v", err)
	}
	if want := "helloXYZrld"; got != want {
		t.Errorf("applyChange() = %q, want %q", got, want)
	}
}

func TestApplyChangesAppliesEveryChange(t *testing.T) {
	changes := []jide.LSPTextChange{
		{Text: "one\ntwo\n"},
		{
			Range: &jide.LSPRange{
				Start: jide.LSPPosition{Line: 1, Character: 0},
				End:   jide.LSPPosition{Line: 1, Character: 3},
			},
			Text: "XYZ",
		},
	}
	got, err := applyChanges("zero\nthree\n", changes)
	if err != nil {
		t.Fatalf("applyChanges() error = %v", err)
	}
	if want := "one\nXYZ\n"; got != want {
		t.Errorf("applyChanges() = %q, want %q", got, want)
	}
}

func TestApplyChangesRejectsABrokenRange(t *testing.T) {
	changes := []jide.LSPTextChange{{
		Range: &jide.LSPRange{
			Start: jide.LSPPosition{Line: 5, Character: 0},
			End:   jide.LSPPosition{Line: 6, Character: 0},
		},
		Text: "x",
	}}
	if _, err := applyChanges("zero\n", changes); err == nil {
		t.Error("applyChanges() error = nil, want an error")
	}
}
