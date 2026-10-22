package lsp

import (
	"fmt"
	"strings"
	"unicode/utf8"

	jide "github.com/jide-dev/jide/proto/jide"
)

// The positions of the language server protocol count the characters of a
// line in UTF-16 code units, the unit of the JavaScript string type. The
// helpers of this file convert such a position into the byte offset that
// the Go string operations use.

// offsetOf returns the byte offset of a position in the text.
//
// The offset of a position behind the end of the text is the end of the
// text. A position outside of the text is an error.
func offsetOf(text string, line, character int) (int, error) {
	if line < 0 || character < 0 {
		return 0, fmt.Errorf("lsp: the position %d:%d is negative", line, character)
	}
	offset := 0
	for current := 0; current < line; current++ {
		index := strings.IndexByte(text[offset:], '\n')
		if index < 0 {
			return 0, fmt.Errorf("lsp: the line %d is outside the text", line)
		}
		offset += index + 1
	}
	units := 0
	for offset < len(text) {
		if units >= character {
			return offset, nil
		}
		character, size := utf8.DecodeRuneInString(text[offset:])
		if character == '\n' {
			return offset, nil
		}
		units += utf16Units(character)
		offset += size
	}
	if units >= character {
		return offset, nil
	}
	return 0, fmt.Errorf("lsp: the column %d is outside the line %d", character, line)
}

// utf16Units returns the number of UTF-16 code units of a rune.
func utf16Units(character rune) int {
	if character > 0xFFFF {
		return 2
	}
	return 1
}

// applyChange returns the text of a document after a change.
//
// A change without a range replaces the whole document. A change with a
// range replaces the text inside the range.
func applyChange(text string, change jide.LSPTextChange) (string, error) {
	if change.Range == nil {
		return change.Text, nil
	}
	start, err := offsetOf(text, change.Range.Start.Line, change.Range.Start.Character)
	if err != nil {
		return "", err
	}
	end, err := offsetOf(text, change.Range.End.Line, change.Range.End.Character)
	if err != nil {
		return "", err
	}
	if end < start {
		return "", fmt.Errorf("lsp: the range of the change ends before it starts")
	}
	return text[:start] + change.Text + text[end:], nil
}

// applyChanges returns the text of a document after a list of changes.
func applyChanges(text string, changes []jide.LSPTextChange) (string, error) {
	updated := text
	for _, change := range changes {
		result, err := applyChange(updated, change)
		if err != nil {
			return "", err
		}
		updated = result
	}
	return updated, nil
}
