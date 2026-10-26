package jide

import (
	"encoding/json"
	"reflect"
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
		"workspaceSymbolsParams":        true,
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
			!strings.HasPrefix(string(info.Name), "syntax/") &&
			!strings.HasPrefix(string(info.Name), "dap/") {
			continue
		}
		if info.Params != "" && !known[info.Params] && !strings.HasPrefix(info.Params, "dap") {
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
		"workspaceSymbolsParams", "workspaceSymbolsResult", "workspaceSymbol",
		"lspRenameParams", "lspRenameResult", "lspDocumentChange",
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
		"lspRenameParams",
		"lspRenameResult",
		"lspDocumentChange",
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

func TestDAPStartParamsSupportJuliaDiscoveryAndExplicitAdapters(t *testing.T) {
	params := DAPStartParams{
		ProjectPath:      "C:/workspace/Project.toml",
		WorkingDirectory: "C:/workspace",
		Executable:       "debug-adapter",
		Arguments:        []string{"--stdio"},
		Directory:        "C:/adapter",
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DAPStartParams
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, params) {
		t.Fatalf("DAPStartParams round trip = %+v, want %+v", decoded, params)
	}

	doc := decodeSchema(t)
	defs, ok := doc["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("schema $defs = %T, want object", doc["$defs"])
	}
	start, ok := defs["dapStartParams"].(map[string]any)
	if !ok {
		t.Fatalf("dapStartParams schema = %T, want object", defs["dapStartParams"])
	}
	properties, ok := start["properties"].(map[string]any)
	if !ok {
		t.Fatalf("dapStartParams properties = %T, want object", start["properties"])
	}
	for _, field := range []string{"projectPath", "workingDirectory", "executable", "arguments", "directory"} {
		if _, ok := properties[field]; !ok {
			t.Errorf("dapStartParams schema is missing %q", field)
		}
	}
}

func TestRunAndTestRunPayloadsRoundTrip(t *testing.T) {
	configuration := RunConfiguration{
		ID: "tests", Name: "All tests", Type: "julia-script", ProjectPath: "C:/fixture",
		Program: "C:/fixture/test/runtests.jl", Arguments: []string{"--verbose"},
		Environment: map[string]string{"JIDE_FIXTURE": "1"},
		Properties:  map[string]json.RawMessage{"custom": json.RawMessage(`{"value":1}`)},
	}
	payloads := []any{
		RunExecuteParams{Configuration: configuration}, RunExecuteResult{RunID: "run-1", PID: 42},
		RunTerminateParams{RunID: "run-1"}, RunTerminateResult{Terminated: true},
		RunInputParams{RunID: "run-1", Input: "println(1)\n"}, RunInputResult{Written: true},
		RunOutputParams{RunID: "run-1", Stream: "stdout", Data: "hello\n"},
		RunExitedParams{RunID: "run-1", ExitCode: 0},
		TestRunExecuteParams{ProjectPath: "C:/fixture", TestFile: "C:/fixture/test/runtests.jl"},
		TestRunExecuteResult{RunID: "test-1"},
		TestRunEventParams{RunID: "test-1", Kind: "test", Status: "failed", SourcePath: "C:/fixture/test/runtests.jl", Line: 17, Failed: 1},
	}
	for _, value := range payloads {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Errorf("marshal %T: %v", value, err)
			continue
		}
		if !json.Valid(raw) {
			t.Errorf("marshal %T returned invalid JSON: %s", value, raw)
		}
	}

	raw, err := json.Marshal(RunExecuteParams{Configuration: configuration})
	if err != nil {
		t.Fatal(err)
	}
	var decoded RunExecuteParams
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Configuration.Type != configuration.Type || decoded.Configuration.Program != configuration.Program || string(decoded.Configuration.Properties["custom"]) != string(configuration.Properties["custom"]) {
		t.Fatalf("run configuration payload round trip = %+v", decoded)
	}
	var failure TestRunEventParams
	if err := json.Unmarshal([]byte(`{"runId":"test-1","kind":"test","status":"failed","sourcePath":"/tmp/runtests.jl","line":17}`), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.SourcePath != "/tmp/runtests.jl" || failure.Line != 17 || failure.Status != "failed" {
		t.Fatalf("failure navigation payload = %+v", failure)
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

func TestLSPRenameWorkspaceEditPayloadMarshals(t *testing.T) {
	version := 4
	result := LSPRenameResult{
		Changes: map[string][]LSPTextChange{
			"file:///src/main.jl": {{Range: &LSPRange{Start: LSPPosition{}, End: LSPPosition{Character: 3}}, Text: "new"}},
		},
		DocumentChanges: []LSPDocumentChange{{
			URI: "file:///src/other.jl", Version: &version,
			Edits: []LSPTextChange{{Text: "new"}},
		}},
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded LSPRenameResult
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, result) {
		t.Fatalf("rename round-trip = %+v, want %+v", decoded, result)
	}
}

func TestPkgPayloadsRoundTrip(t *testing.T) {
	result := PkgEnvironmentsResult{Environments: []PkgEnvironment{{
		Root:     "/project",
		Project:  JuliaProjectInfo{Path: "/project/Project.toml", Name: "Demo"},
		Manifest: &JuliaManifestInfo{Path: "/project/Manifest.toml", Packages: []JuliaManifestPackageInfo{{Name: "Dates", UUID: "dates", Version: "1.0"}}},
	}}}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PkgEnvironmentsResult
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, result) {
		t.Fatalf("package environment round-trip = %#v, want %#v", decoded, result)
	}

	operation := PkgOperationParams{ProjectPath: "/project", Package: "Dates"}
	raw, err = json.Marshal(operation)
	if err != nil {
		t.Fatal(err)
	}
	var decodedOperation PkgOperationParams
	if err := json.Unmarshal(raw, &decodedOperation); err != nil || decodedOperation != operation {
		t.Fatalf("package operation round-trip = %#v, %v", decodedOperation, err)
	}
}

func TestConsolePayloadsRoundTrip(t *testing.T) {
	input := ConsoleInputParams{SessionID: "session-1", Input: "println(1)\n"}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var decodedInput ConsoleInputParams
	if err := json.Unmarshal(raw, &decodedInput); err != nil || decodedInput != input {
		t.Fatalf("console input round-trip = %#v, %v", decodedInput, err)
	}

	result := ConsoleSessionsResult{Sessions: []ConsoleSessionInfo{{
		SessionID: "session-1", WorkingDirectory: "C:/project", State: "running",
		CreatedAt: "2026-10-24T12:00:00Z", History: []string{"println(1)"},
	}}}
	raw, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decodedResult ConsoleSessionsResult
	if err := json.Unmarshal(raw, &decodedResult); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodedResult, result) {
		t.Fatalf("console sessions round-trip = %#v, want %#v", decodedResult, result)
	}
}

func TestDAPPayloadsRoundTrip(t *testing.T) {
	payload := DAPSetBreakpointsParams{
		Source:      DAPSource{Name: "main.jl", Path: "C:/project/main.jl"},
		Breakpoints: []DAPSourceBreakpoint{{Line: 12, Condition: "x > 2"}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DAPSetBreakpointsParams
	if err := json.Unmarshal(raw, &decoded); err != nil || !reflect.DeepEqual(decoded, payload) {
		t.Fatalf("DAP breakpoint round-trip = %#v, %v", decoded, err)
	}
	event := DAPEventParams{Event: "stopped", Body: json.RawMessage(`{"reason":"breakpoint","threadId":1}`)}
	raw, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decodedEvent DAPEventParams
	if err := json.Unmarshal(raw, &decodedEvent); err != nil || !reflect.DeepEqual(decodedEvent, event) {
		t.Fatalf("DAP event round-trip = %#v, %v", decodedEvent, err)
	}
	result := DAPStartResult{Capabilities: DAPCapabilities{SupportsFunctionBreakpoints: true, SupportsConfigurationDoneRequest: true}}
	raw, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decodedResult DAPStartResult
	if err := json.Unmarshal(raw, &decodedResult); err != nil || !reflect.DeepEqual(decodedResult, result) {
		t.Fatalf("DAP start result round-trip = %#v, %v", decodedResult, err)
	}
	setVariable := DAPSetVariableParams{VariablesReference: 3, Name: "x", Value: "42"}
	raw, err = json.Marshal(setVariable)
	if err != nil {
		t.Fatal(err)
	}
	var decodedSetVariable DAPSetVariableParams
	if err := json.Unmarshal(raw, &decodedSetVariable); err != nil || decodedSetVariable != setVariable {
		t.Fatalf("DAP set-variable round-trip = %#v, %v", decodedSetVariable, err)
	}
}
