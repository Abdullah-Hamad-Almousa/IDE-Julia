package jide

import (
	"encoding/json"
	"testing"
)

// TestLSPStatusResultMarshals checks the wire format of the state of the
// language server.
func TestLSPStatusResultMarshals(t *testing.T) {
	result := LSPStatusResult{
		State:           LSPStateRunning,
		JuliaVersion:    "1.10.3",
		JuliaChannel:    "stable",
		Executable:      "/usr/bin/julia",
		RestartAttempts: 2,
		LastError:       "the server died",
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := decoded["state"], string(LSPStateRunning); got != want {
		t.Errorf("state = %v, want %v", got, want)
	}
	if got, want := decoded["juliaVersion"], "1.10.3"; got != want {
		t.Errorf("juliaVersion = %v, want %v", got, want)
	}
	if got, want := decoded["restartAttempts"], float64(2); got != want {
		t.Errorf("restartAttempts = %v, want %v", got, want)
	}
}

// TestLSPDiagnosticsParamsMarshal checks the wire format of the published
// diagnostics.
func TestLSPDiagnosticsParamsMarshal(t *testing.T) {
	params := LSPDiagnosticsParams{
		URI: "file:///project/main.jl",
		Diagnostics: []LSPDiagnostic{{
			Range: LSPRange{
				Start: LSPPosition{Line: 1, Character: 2},
				End:   LSPPosition{Line: 1, Character: 5},
			},
			Severity: LSPDiagnosticError,
			Code:     "E42",
			Source:   "julia",
			Message:  "undefined variable",
		}},
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := decoded["uri"], "file:///project/main.jl"; got != want {
		t.Errorf("uri = %v, want %v", got, want)
	}
	diagnostics, ok := decoded["diagnostics"].([]any)
	if !ok {
		t.Fatalf("diagnostics is %T, want array", decoded["diagnostics"])
	}
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics has %d entries, want 1", len(diagnostics))
	}
	first, ok := diagnostics[0].(map[string]any)
	if !ok {
		t.Fatalf("diagnostics[0] is %T, want object", diagnostics[0])
	}
	if got, want := first["message"], "undefined variable"; got != want {
		t.Errorf("diagnostics[0].message = %v, want %v", got, want)
	}
	if got, want := first["severity"], float64(LSPDiagnosticError); got != want {
		t.Errorf("diagnostics[0].severity = %v, want %v", got, want)
	}
	rangeObject, ok := first["range"].(map[string]any)
	if !ok {
		t.Fatalf("diagnostics[0].range is %T, want object", first["range"])
	}
	start, ok := rangeObject["start"].(map[string]any)
	if !ok {
		t.Fatalf("diagnostics[0].range.start is %T, want object", rangeObject["start"])
	}
	if got, want := start["line"], float64(1); got != want {
		t.Errorf("diagnostics[0].range.start.line = %v, want %v", got, want)
	}
	if got, want := start["character"], float64(2); got != want {
		t.Errorf("diagnostics[0].range.start.character = %v, want %v", got, want)
	}
}

// TestLSPHoverResultMarshals checks the wire format of the hover answer.
func TestLSPHoverResultMarshals(t *testing.T) {
	hover := &LSPHover{
		Contents: LSPMarkupContent{Kind: LSPMarkupMarkdown, Value: "# f"},
		Range:    &LSPRange{Start: LSPPosition{Line: 0, Character: 0}},
	}
	result := LSPHoverResult{URI: "file:///project/main.jl", Hover: hover}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	hoverObject, ok := decoded["hover"].(map[string]any)
	if !ok {
		t.Fatalf("hover is %T, want object", decoded["hover"])
	}
	contents, ok := hoverObject["contents"].(map[string]any)
	if !ok {
		t.Fatalf("hover.contents is %T, want object", hoverObject["contents"])
	}
	if got, want := contents["kind"], string(LSPMarkupMarkdown); got != want {
		t.Errorf("hover.contents.kind = %v, want %v", got, want)
	}
	if got, want := contents["value"], "# f"; got != want {
		t.Errorf("hover.contents.value = %v, want %v", got, want)
	}
}

// TestLSPTextChangeMarshals checks the wire format of an incremental change
// of a document.
func TestLSPTextChangeMarshals(t *testing.T) {
	full := LSPTextChange{Text: "x = 1\n"}
	raw, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := decoded["range"]; ok {
		t.Error("the change of the whole document holds a range")
	}

	partial := LSPTextChange{
		Range: &LSPRange{
			Start: LSPPosition{Line: 0, Character: 0},
			End:   LSPPosition{Line: 0, Character: 1},
		},
		Text: "y",
	}
	raw, err = json.Marshal(partial)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	decoded = map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := decoded["range"]; !ok {
		t.Error("the change of a range holds no range")
	}
}
