package jide

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// TestWorkspaceAndDocumentMethodsAreKnown checks that the methods added for
// the workspace and document surface are part of the catalog and follow the
// naming convention.
func TestWorkspaceAndDocumentMethodsAreKnown(t *testing.T) {
	methods := []Method{
		MethodWorkspaceState,
		MethodWorkspaceTree,
		MethodWorkspaceDidChangeFiles,
		MethodDocumentRead,
		MethodDocumentSave,
		MethodDocumentDidChange,
		MethodDocumentDidSave,
		MethodIndexSymbols,
		MethodSyntaxHighlights,
		MethodSyntaxFolds,
	}
	for _, method := range methods {
		if !method.IsKnown() {
			t.Errorf("method %q is not part of the catalog", method)
		}
		if err := method.Validate(); err != nil {
			t.Errorf("method %q does not follow the naming convention: %v", method, err)
		}
	}
}

// TestMethodCatalogKeepsTheDocumentMethodsApart checks that the request
// methods and the notification methods of the document surface are distinct.
func TestMethodCatalogKeepsTheDocumentMethodsApart(t *testing.T) {
	pairs := [][2]Method{
		{MethodDocumentDidChange, MethodDocumentDidSave},
		{MethodDocumentRead, MethodDocumentSave},
		{MethodDocumentSave, MethodDocumentDidSave},
	}
	for _, pair := range pairs {
		if pair[0] == pair[1] {
			t.Errorf("the document methods %q and %q are the same", pair[0], pair[1])
		}
	}
}

// TestMethodCatalogReferencesThePayloadObjects checks that the methods of the
// workspace and document surface name a parameter object that the module
// defines. The catalog also holds methods of later work packages whose
// parameter objects do not exist yet.
func TestMethodCatalogReferencesThePayloadObjects(t *testing.T) {
	known := map[string]bool{
		"workspaceOpenParams":           true,
		"workspaceCloseParams":          true,
		"workspaceStateParams":          true,
		"workspaceTreeParams":           true,
		"workspaceDidChangeFilesParams": true,
		"documentOpenParams":            true,
		"documentCloseParams":           true,
		"documentChangeParams":          true,
		"documentSaveParams":            true,
		"documentReadParams":            true,
		"documentSymbolsParams":         true,
		"textSearchParams":              true,
		"syntaxHighlightsParams":        true,
		"syntaxFoldsParams":             true,
	}
	for _, info := range Methods() {
		if info.Params == "" {
			continue
		}
		if !strings.HasPrefix(string(info.Name), "workspace/") &&
			!strings.HasPrefix(string(info.Name), "document/") &&
			!strings.HasPrefix(string(info.Name), "index/") &&
			!strings.HasPrefix(string(info.Name), "syntax/") {
			continue
		}
		if !known[info.Params] {
			t.Errorf("method %q refers to the unknown parameter object %q", info.Name, info.Params)
		}
	}
}

func TestJSONSchemaContainsSyntaxPayloads(t *testing.T) {
	doc := decodeSchema(t)
	defs, ok := doc["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("$defs is %T, want object", doc["$defs"])
	}
	for _, name := range []string{
		"syntaxPosition", "syntaxRange", "syntaxToken", "syntaxDiagnostic",
		"documentSymbolsParams", "documentSymbolsResult", "documentSymbol",
		"syntaxHighlightsParams", "syntaxHighlightsResult", "syntaxFoldsParams",
		"syntaxFoldsResult", "syntaxFoldRange",
	} {
		if _, ok := defs[name]; !ok {
			t.Errorf("$defs is missing %q", name)
		}
	}
}

// TestJSONSchemaContainsTheWorkspacePayloads checks that the exported schema
// describes the payload objects of the workspace and document surface.
func TestJSONSchemaContainsTheWorkspacePayloads(t *testing.T) {
	doc := decodeSchema(t)
	defs, ok := doc["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("$defs is %T, want object", doc["$defs"])
	}
	want := []string{
		"workspaceStateParams",
		"workspaceStateResult",
		"workspaceTreeParams",
		"workspaceTreeResult",
		"workspaceDidChangeFilesParams",
		"workspaceFileEntry",
		"externalSymlink",
		"documentReadParams",
		"documentReadResult",
		"documentSaveResult",
		"juliaProjectInfo",
		"juliaDependencyInfo",
		"juliaManifestInfo",
		"juliaManifestPackageInfo",
	}
	for _, name := range want {
		if _, ok := defs[name]; !ok {
			t.Errorf("$defs is missing %q", name)
		}
	}
}

// TestJuliaProjectInfoMarshals checks the wire format of the Julia project
// payload.
func TestJuliaProjectInfoMarshals(t *testing.T) {
	info := JuliaProjectInfo{
		Path:        "/tmp/project/Project.toml",
		Name:        "Demo",
		UUID:        "6f6c8b12-1f1a-4f0a-9c0a-1f1a4f0a9c0a",
		JuliaCompat: "1.10",
		Compat:      map[string]string{"julia": "1.10", "Demo": "0.1"},
		Dependencies: []JuliaDependencyInfo{
			{Name: "Bar", UUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Version: "1.2.3"},
		},
		WeakDependencies: []JuliaDependencyInfo{
			{Name: "Baz", UUID: "11111111-2222-3333-4444-555555555555", Weak: true},
		},
		Extras: []JuliaDependencyInfo{
			{Name: "Test", UUID: "8dfed614-e22c-5e08-85e1-65c5234f0b40"},
		},
	}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := decoded["name"], "Demo"; got != want {
		t.Errorf("name = %v, want %v", got, want)
	}
	if got, want := decoded["juliaCompat"], "1.10"; got != want {
		t.Errorf("juliaCompat = %v, want %v", got, want)
	}
	deps, ok := decoded["dependencies"].([]any)
	if !ok {
		t.Fatalf("dependencies is %T, want array", decoded["dependencies"])
	}
	if len(deps) != 1 {
		t.Fatalf("dependencies has %d entries, want 1", len(deps))
	}
	first, ok := deps[0].(map[string]any)
	if !ok {
		t.Fatalf("dependencies[0] is %T, want object", deps[0])
	}
	if got, want := first["version"], "1.2.3"; got != want {
		t.Errorf("dependencies[0].version = %v, want %v", got, want)
	}
	weak, ok := decoded["weakDependencies"].([]any)
	if !ok {
		t.Fatalf("weakDependencies is %T, want array", decoded["weakDependencies"])
	}
	weakFirst, ok := weak[0].(map[string]any)
	if !ok {
		t.Fatalf("weakDependencies[0] is %T, want object", weak[0])
	}
	if got, want := weakFirst["weak"], true; got != want {
		t.Errorf("weakDependencies[0].weak = %v, want %v", got, want)
	}
}

// TestWorkspaceTreeResultMarshals checks the wire format of the file tree
// payload.
func TestWorkspaceTreeResultMarshals(t *testing.T) {
	result := WorkspaceTreeResult{
		Root: "/tmp/project",
		Entries: []WorkspaceFileEntry{
			{Path: "src", AbsolutePath: "/tmp/project/src", IsDirectory: true},
			{Path: "deps/Linked", AbsolutePath: "/tmp/project/deps/Linked", Symlink: true, LinkTarget: "../pkg"},
		},
		ExternalSymlinks: []ExternalSymlink{
			{Path: "external/Out", Target: "../../out", ResolvedPath: "/tmp/out"},
		},
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	entries, ok := decoded["entries"].([]any)
	if !ok {
		t.Fatalf("entries is %T, want array", decoded["entries"])
	}
	if len(entries) != 2 {
		t.Fatalf("entries has %d entries, want 2", len(entries))
	}
	external, ok := decoded["externalSymlinks"].([]any)
	if !ok {
		t.Fatalf("externalSymlinks is %T, want array", decoded["externalSymlinks"])
	}
	if len(external) != 1 {
		t.Fatalf("externalSymlinks has %d entries, want 1", len(external))
	}
	link, ok := external[0].(map[string]any)
	if !ok {
		t.Fatalf("externalSymlinks[0] is %T, want object", external[0])
	}
	if got, want := link["resolvedPath"], "/tmp/out"; got != want {
		t.Errorf("externalSymlinks[0].resolvedPath = %v, want %v", got, want)
	}
}

// TestWorkspaceDidChangeFilesParamsMarshals checks the wire format of the
// file change notification.
func TestWorkspaceDidChangeFilesParamsMarshals(t *testing.T) {
	params := WorkspaceDidChangeFilesParams{
		WorkspaceID: "w1",
		Changes: []WorkspaceFileChange{
			{Path: "/tmp/project/a.jl", Type: FileChangeCreated},
			{Path: "/tmp/project/b.jl", Type: FileChangeChanged},
			{Path: "/tmp/project/c.jl", Type: FileChangeDeleted},
		},
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	changes, ok := decoded["changes"].([]any)
	if !ok {
		t.Fatalf("changes is %T, want array", decoded["changes"])
	}
	want := []string{"created", "changed", "deleted"}
	for i, change := range changes {
		object, ok := change.(map[string]any)
		if !ok {
			t.Fatalf("changes[%d] is %T, want object", i, change)
		}
		if got, ok := object["type"].(string); !ok || !slices.Contains(want, got) {
			t.Errorf("changes[%d].type = %v, want one of %v", i, object["type"], want)
		}
	}
}

// TestDocumentReadAndSaveResultsMarshal checks the wire format of the
// document read and save results.
func TestDocumentReadAndSaveResultsMarshal(t *testing.T) {
	read := DocumentReadResult{URI: "file:///a.jl", Version: 3, Text: "x = 1\n", Dirty: true}
	raw, err := json.Marshal(read)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decodedRead map[string]any
	if err := json.Unmarshal(raw, &decodedRead); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := decodedRead["version"], float64(3); got != want {
		t.Errorf("version = %v, want %v", got, want)
	}
	if got, want := decodedRead["dirty"], true; got != want {
		t.Errorf("dirty = %v, want %v", got, want)
	}

	save := DocumentSaveResult{URI: "file:///a.jl", Version: 4}
	raw, err = json.Marshal(save)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decodedSave map[string]any
	if err := json.Unmarshal(raw, &decodedSave); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := decodedSave["version"], float64(4); got != want {
		t.Errorf("version = %v, want %v", got, want)
	}
}

func TestLSPCompletionItemMarshalsReplacementEdits(t *testing.T) {
	item := LSPCompletionItem{
		Label:            "alpha",
		InsertTextFormat: 2,
		TextEdit: &LSPTextChange{
			Range: &LSPRange{Start: LSPPosition{Character: 1}, End: LSPPosition{Character: 3}},
			Text:  "alpha()",
		},
		AdditionalTextEdits: []LSPTextChange{{Text: "using Base\n"}},
	}
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["textEdit"]; !ok {
		t.Error("textEdit missing from completion JSON")
	}
	if got, ok := decoded["insertTextFormat"].(float64); !ok || got != 2 {
		t.Errorf("insertTextFormat = %v, want 2", decoded["insertTextFormat"])
	}
	if edits, ok := decoded["additionalTextEdits"].([]any); !ok || len(edits) != 1 {
		t.Errorf("additionalTextEdits = %v, want one edit", decoded["additionalTextEdits"])
	}
}
