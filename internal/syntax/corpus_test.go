package syntax

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

const corpusGoldenPath = "testdata/corpus.golden.json"

// TestCorpusParseTreesMatchGoldens parses each distinct Julia fixture and
// compares its canonical Tree-sitter tree to the checked-in golden.
func TestCorpusParseTreesMatchGoldens(t *testing.T) {
	files, err := filepath.Glob("testdata/corpus/*.jl")
	if err != nil {
		t.Fatalf("list the Julia corpus: %v", err)
	}
	if len(files) != 100 {
		t.Fatalf("Julia corpus has %d files, want 100", len(files))
	}
	updating := os.Getenv("JIDE_UPDATE_SYNTAX_GOLDENS") == "1"
	var expected map[string]string
	if !updating {
		raw, readErr := os.ReadFile(corpusGoldenPath)
		if readErr != nil {
			t.Fatalf("read parse-tree goldens: %v", readErr)
		}
		if err := json.Unmarshal(raw, &expected); err != nil {
			t.Fatalf("decode parse-tree goldens: %v", err)
		}
		if len(expected) != len(files) {
			t.Fatalf("golden corpus has %d entries, want %d", len(expected), len(files))
		}
	}

	actual := make(map[string]string, len(files))
	seenSources := make(map[string]string, len(files))
	for _, path := range files {
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read Julia fixture %q: %v", path, readErr)
		}
		if previous, exists := seenSources[string(source)]; exists {
			t.Errorf("%s duplicates the source of %s", path, previous)
		}
		seenSources[string(source)] = filepath.Base(path)
		document, parseErr := Parse(string(source))
		if parseErr != nil {
			t.Errorf("parse %s: %v", path, parseErr)
			continue
		}
		if diagnostics := document.Diagnostics(); len(diagnostics) != 0 {
			t.Errorf("%s has syntax diagnostics: %+v\ntree: %s", path, diagnostics, document.SExpression())
			continue
		}
		name := filepath.Base(path)
		actual[name] = document.SExpression()
		if !updating {
			want, ok := expected[name]
			if !ok {
				t.Errorf("no parse-tree golden for %s", name)
				continue
			}
			if got := document.SExpression(); got != want {
				t.Errorf("parse tree changed for %s\nsource:\n%s\nwant: %s\n got: %s", name, source, want, got)
			}
		}
	}
	if updating {
		if t.Failed() {
			return
		}
		names := make([]string, 0, len(actual))
		for name := range actual {
			names = append(names, name)
		}
		sort.Strings(names)
		ordered := make(map[string]string, len(actual))
		for _, name := range names {
			ordered[name] = actual[name]
		}
		raw, marshalErr := json.MarshalIndent(ordered, "", "  ")
		if marshalErr != nil {
			t.Fatalf("marshal parse-tree goldens: %v", marshalErr)
		}
		raw = append(raw, '\n')
		if writeErr := os.WriteFile(corpusGoldenPath, raw, 0o644); writeErr != nil {
			t.Fatalf("write parse-tree goldens: %v", writeErr)
		}
		t.Skipf("updated %d parse-tree goldens; rerun without JIDE_UPDATE_SYNTAX_GOLDENS", len(actual))
	}
}
