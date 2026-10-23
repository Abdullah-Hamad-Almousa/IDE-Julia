package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
	jide "github.com/jide-dev/jide/proto/jide"
)

func TestDecodeMarkupOfAString(t *testing.T) {
	kind, value, err := decodeMarkup(json.RawMessage(`"plain"`))
	if err != nil {
		t.Fatalf("decodeMarkup() error = %v", err)
	}
	if kind != "plaintext" {
		t.Errorf("the kind = %q, want %q", kind, "plaintext")
	}
	if value != "plain" {
		t.Errorf("the value = %q, want %q", value, "plain")
	}
}

func TestDecodeMarkupOfAnObject(t *testing.T) {
	kind, value, err := decodeMarkup(json.RawMessage(`{"kind":"markdown","value":"# title"}`))
	if err != nil {
		t.Fatalf("decodeMarkup() error = %v", err)
	}
	if kind != "markdown" {
		t.Errorf("the kind = %q, want %q", kind, "markdown")
	}
	if value != "# title" {
		t.Errorf("the value = %q, want %q", value, "# title")
	}
}

func TestDecodeMarkupOfAList(t *testing.T) {
	kind, value, err := decodeMarkup(json.RawMessage(`["first","second"]`))
	if err != nil {
		t.Fatalf("decodeMarkup() error = %v", err)
	}
	if kind != "plaintext" {
		t.Errorf("the kind = %q, want %q", kind, "plaintext")
	}
	if value != "first\nsecond" {
		t.Errorf("the value = %q, want %q", value, "first\nsecond")
	}
}

func TestDecodeMarkupOfAnEmptyAnswer(t *testing.T) {
	kind, value, err := decodeMarkup(nil)
	if err != nil {
		t.Fatalf("decodeMarkup() error = %v", err)
	}
	if kind != "" || value != "" {
		t.Errorf("decodeMarkup() = %q, %q, want two empty strings", kind, value)
	}
}

func TestProtocolDiagnostics(t *testing.T) {
	diagnostics := protocolDiagnostics([]lspDiagnostic{{
		Range:    lspRange{Start: position{Line: 1, Character: 2}, End: position{Line: 1, Character: 5}},
		Severity: 2,
		Code:     "E42",
		Source:   "julia",
		Message:  "a warning",
	}})
	if len(diagnostics) != 1 {
		t.Fatalf("the conversion holds %d diagnostics, want 1", len(diagnostics))
	}
	if got, want := diagnostics[0].Severity, jide.LSPDiagnosticWarning; got != want {
		t.Errorf("the severity = %v, want %v", got, want)
	}
	if got, want := diagnostics[0].Message, "a warning"; got != want {
		t.Errorf("the message = %q, want %q", got, want)
	}
	if got, want := diagnostics[0].Range.Start.Character, 2; got != want {
		t.Errorf("the character of the range = %d, want %d", got, want)
	}
}

func TestProtocolHoverOfAnEmptyAnswer(t *testing.T) {
	hover, err := protocolHover(json.RawMessage("null"))
	if err != nil {
		t.Fatalf("protocolHover() error = %v", err)
	}
	if hover != nil {
		t.Errorf("protocolHover() = %v, want nil", hover)
	}
}

func TestProtocolCompletionOfAList(t *testing.T) {
	result, err := protocolCompletion(json.RawMessage(`[{"label":"alpha","kind":3}]`))
	if err != nil {
		t.Fatalf("protocolCompletion() error = %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("the completion holds %d items, want 1", len(result.Items))
	}
	if got, want := result.Items[0].Label, "alpha"; got != want {
		t.Errorf("the label = %q, want %q", got, want)
	}
	if got, want := result.Items[0].Kind, jide.LSPCompletionFunction; got != want {
		t.Errorf("the kind = %v, want %v", got, want)
	}
}

func TestProtocolCompletionOfAnObject(t *testing.T) {
	result, err := protocolCompletion(json.RawMessage(`{"isIncomplete":true,"items":[{"label":"beta"}]}`))
	if err != nil {
		t.Fatalf("protocolCompletion() error = %v", err)
	}
	if !result.IsIncomplete {
		t.Error("IsIncomplete = false, want true")
	}
	if len(result.Items) != 1 {
		t.Fatalf("the completion holds %d items, want 1", len(result.Items))
	}
}

func TestProtocolCompletionPreservesReplacementEdits(t *testing.T) {
	result, err := protocolCompletion(json.RawMessage(`{"items":[{"label":"function","insertTextFormat":2,"textEdit":{"range":{"start":{"line":0,"character":1},"end":{"line":0,"character":3}},"newText":"function ${1:name}()"},"additionalTextEdits":[{"range":{"start":{"line":1,"character":0},"end":{"line":1,"character":0}},"newText":"using Base\n"}]}]}`))
	if err != nil {
		t.Fatalf("protocolCompletion() error = %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(result.Items))
	}
	item := result.Items[0]
	if item.InsertTextFormat != 2 || item.TextEdit == nil || item.TextEdit.Text != "function ${1:name}()" {
		t.Errorf("completion edit not preserved: %+v", item)
	}
	if item.TextEdit.Range.Start.Character != 1 || item.TextEdit.Range.End.Character != 3 {
		t.Errorf("replacement range = %+v", item.TextEdit.Range)
	}
	if len(item.AdditionalTextEdits) != 1 || item.AdditionalTextEdits[0].Text != "using Base\n" {
		t.Errorf("additional edits = %+v", item.AdditionalTextEdits)
	}
}

func TestJuliaCommandUsesToolEnvironmentAndWorkspaceDirectory(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	workspaceRoot := filepath.Join(t.TempDir(), "source-only")
	command := JuliaWorkspaceCommand(jlproc.Runtime{Executable: "julia"}, "", workspaceRoot)
	if command.Dir != workspaceRoot {
		t.Errorf("working directory = %q, want %q", command.Dir, workspaceRoot)
	}
	if got, want := command.Args[len(command.Args)-3], "--"; got != want {
		t.Fatalf("separator arg = %q, want %q; args=%v", got, want, command.Args)
	}
	if got, want := command.Args[len(command.Args)-2], ""; got != want {
		t.Errorf("source-only analysis environment = %q, want empty autodetection", got)
	}
	toolEnvironment := ""
	for _, argument := range command.Args {
		if strings.HasPrefix(argument, "--project=") {
			toolEnvironment = strings.TrimPrefix(argument, "--project=")
		}
	}
	if toolEnvironment == workspaceRoot || toolEnvironment == "" {
		t.Errorf("LanguageServer package environment = %q, must be separate from workspace %q", toolEnvironment, workspaceRoot)
	}
	if _, err := os.Stat(filepath.Join(toolEnvironment, "symbolcache")); err != nil {
		t.Errorf("symbol cache directory: %v", err)
	}
}

func TestJuliaProjectPathForWorkspace(t *testing.T) {
	root := t.TempDir()
	if got := juliaProjectPathForWorkspace("", root); got != "" {
		t.Errorf("source-only environment = %q, want empty autodetection path", got)
	}
	project := filepath.Join(root, "Project.toml")
	if err := os.WriteFile(project, []byte("[deps]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := juliaProjectPathForWorkspace(project, root); got != root {
		t.Errorf("project environment = %q, want %q", got, root)
	}
}

func TestFileURICanonicalizesWindowsDriveLetter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows file-URI normalization applies only on Windows")
	}
	got := FileURI(`C:\Users\Example\a #1.jl`)
	if !strings.HasPrefix(got, "file:///c:/") {
		t.Errorf("FileURI() = %q, want lowercase drive-letter URI", got)
	}
	if !strings.Contains(got, "%20") || !strings.Contains(got, "%23") || strings.Contains(got, "c%3A") {
		t.Errorf("FileURI() did not escape path characters: %q", got)
	}
}

func TestProtocolLocationsOfAList(t *testing.T) {
	locations, err := protocolLocations(json.RawMessage(`[{"uri":"file:///a.jl","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}}]`))
	if err != nil {
		t.Fatalf("protocolLocations() error = %v", err)
	}
	if len(locations) != 1 {
		t.Fatalf("the locations hold %d entries, want 1", len(locations))
	}
	if got, want := locations[0].URI, "file:///a.jl"; got != want {
		t.Errorf("the URI = %q, want %q", got, want)
	}
	if got, want := locations[0].Range.End.Character, 4; got != want {
		t.Errorf("the character = %d, want %d", got, want)
	}
}

func TestProtocolLocationsOfASingleLocation(t *testing.T) {
	locations, err := protocolLocations(json.RawMessage(`{"uri":"file:///b.jl","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}}}`))
	if err != nil {
		t.Fatalf("protocolLocations() error = %v", err)
	}
	if len(locations) != 1 {
		t.Fatalf("the locations hold %d entries, want 1", len(locations))
	}
}

func TestBackoffPolicyDelay(t *testing.T) {
	policy := BackoffPolicy{InitialDelay: time.Second, Multiplier: 2, MaxDelay: 5 * time.Second, MaxAttempts: 5}
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 0},
		{attempt: 1, want: time.Second},
		{attempt: 2, want: 2 * time.Second},
		{attempt: 3, want: 4 * time.Second},
		{attempt: 4, want: 5 * time.Second},
		{attempt: 9, want: 5 * time.Second},
	}
	for _, testCase := range cases {
		if got := policy.Delay(testCase.attempt); got != testCase.want {
			t.Errorf("Delay(%d) = %s, want %s", testCase.attempt, got, testCase.want)
		}
	}
}

func TestDefaultBackoffPolicy(t *testing.T) {
	policy := DefaultBackoffPolicy()
	if got, want := policy.InitialDelay, time.Second; got != want {
		t.Errorf("the initial delay = %s, want %s", got, want)
	}
	if got, want := policy.MaxAttempts, 5; got != want {
		t.Errorf("the number of attempts = %d, want %d", got, want)
	}
}
