package jide

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// decodeSchema decodes the exported schema into a generic JSON document.
func decodeSchema(t *testing.T) map[string]any {
	t.Helper()
	raw, err := JSONSchema()
	if err != nil {
		t.Fatalf("JSONSchema() error = %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	return doc
}

func TestJSONSchemaIsValidJSON(t *testing.T) {
	raw, err := JSONSchema()
	if err != nil {
		t.Fatalf("JSONSchema() error = %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("JSONSchema() returned no bytes")
	}
	if !json.Valid(raw) {
		t.Fatalf("JSONSchema() returned invalid JSON:\n%s", raw)
	}
}

func TestJSONSchemaTopLevelKeys(t *testing.T) {
	doc := decodeSchema(t)
	want := []string{
		"$schema",
		"$id",
		"title",
		"description",
		"type",
		"required",
		"additionalProperties",
		"properties",
		"$defs",
		"x-jide",
	}
	for _, key := range want {
		if _, ok := doc[key]; !ok {
			t.Errorf("schema is missing the top-level key %q", key)
		}
	}
	if got, want := doc["$schema"], SchemaDraft; got != want {
		t.Errorf("$schema = %v, want %v", got, want)
	}
	if got, want := doc["$id"], SchemaID; got != want {
		t.Errorf("$id = %v, want %v", got, want)
	}
	if got, want := doc["type"], "object"; got != want {
		t.Errorf("type = %v, want %v", got, want)
	}
}

func TestJSONSchemaProperties(t *testing.T) {
	doc := decodeSchema(t)
	props, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties is %T, want object", doc["properties"])
	}
	for _, key := range []string{"jsonrpc", "id", "method", "params"} {
		if _, ok := props[key]; !ok {
			t.Errorf("properties is missing %q", key)
		}
	}
}

func TestJSONSchemaDefsContainEnvelope(t *testing.T) {
	doc := decodeSchema(t)
	defs, ok := doc["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("$defs is %T, want object", doc["$defs"])
	}
	for _, key := range []string{
		"id", "request", "response", "error", "workspaceOpenParams", "textSearchResult",
		"lspStartParams", "lspStartResult", "lspStopParams", "lspStopResult",
		"lspStatusParams", "lspStatusResult", "lspDiagnosticsParams", "lspDiagnostic",
		"lspHoverParams", "lspHoverResult", "lspHover", "lspMarkupContent",
		"lspCompletionParams", "lspCompletionResult", "lspCompletionItem",
		"lspReferencesParams", "lspReferencesResult", "lspDefinitionParams",
		"lspDefinitionResult", "lspLocation", "lspRange", "lspPosition",
		"lspTextChange",
		"pkgEnvironmentsParams", "pkgEnvironment", "pkgEnvironmentsResult",
		"pkgOperationParams", "pkgOperationResult", "pkgManifestDiffParams",
		"pkgPackageUpdate", "pkgManifestDiffResult", "pkgDependencyGraphParams",
		"pkgDependencyNode", "pkgDependencyGraphResult",
		"runConfiguration", "runExecuteParams", "runExecuteResult", "runTerminateParams",
		"runTerminateResult", "runInputParams", "runInputResult", "runOutputParams", "runExitedParams", "testRunExecuteParams",
		"testRunExecuteResult", "testRunEventParams",
		"consoleOpenParams", "consoleSessionParams", "consoleInputParams",
		"consoleOutputParams", "consoleSessionInfo", "consoleOpenResult",
		"consoleSessionsResult", "consoleHistoryResult", "consoleResizeParams",
		"dapStartParams", "dapArgumentsParams", "dapSource", "dapSourceBreakpoint",
		"dapStartResult", "dapBreakpointsResult", "dapContinueResult", "dapScopesResult", "dapScope", "dapEmptyResult",
		"dapVariablesResult", "dapVariable", "dapEvaluateResult", "dapCapabilities",
		"dapExceptionBreakpointFilter", "dapSetVariableParams", "dapSetVariableResult",
		"dapExceptionBreakpointsParams", "dapExceptionInfoResult", "dapExceptionDetails",
		"dapFunctionBreakpoint", "dapBreakpoint", "dapSetBreakpointsParams",
		"dapFunctionBreakpointsParams", "dapThreadParams", "dapFrameParams",
		"dapVariablesParams", "dapEvaluateParams", "dapDisconnectParams", "dapEventParams",
		"dapStackTraceParams", "dapStackFrame", "dapStackTraceResult", "dapBreakpointEventParams",
		"dapThreadsParams", "dapThread", "dapThreadsResult",
		"dapStepInTarget", "dapStepInTargetsResult",
	} {
		if _, ok := defs[key]; !ok {
			t.Errorf("$defs is missing %q", key)
		}
	}
}

func TestLSPCompletionItemSchemaContainsEdits(t *testing.T) {
	doc := decodeSchema(t)
	defs, ok := doc["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("$defs is %T, want object", doc["$defs"])
	}
	item, ok := defs["lspCompletionItem"].(map[string]any)
	if !ok {
		t.Fatalf("lspCompletionItem is %T, want object", defs["lspCompletionItem"])
	}
	properties, ok := item["properties"].(map[string]any)
	if !ok {
		t.Fatalf("lspCompletionItem.properties is %T, want object", item["properties"])
	}
	for _, property := range []string{"textEdit", "additionalTextEdits", "insertTextFormat"} {
		if _, ok := properties[property]; !ok {
			t.Errorf("lspCompletionItem schema is missing %q", property)
		}
	}
}

func TestJSONSchemaMethodEnumMatchesCatalog(t *testing.T) {
	doc := decodeSchema(t)
	props, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties is %T, want object", doc["properties"])
	}
	method, ok := props["method"].(map[string]any)
	if !ok {
		t.Fatalf("properties.method is %T, want object", props["method"])
	}
	enum, ok := method["enum"].([]any)
	if !ok {
		t.Fatalf("properties.method.enum is %T, want array", method["enum"])
	}
	got := make([]string, 0, len(enum))
	for _, value := range enum {
		name, ok := value.(string)
		if !ok {
			t.Fatalf("enum entry %v is %T, want string", value, value)
		}
		got = append(got, name)
	}
	want := MethodNames()
	if !slices.Equal(got, want) {
		t.Errorf("method enum = %v, want %v", got, want)
	}
	for _, name := range got {
		if !Method(name).IsKnown() {
			t.Errorf("method enum contains the unknown method %q", name)
		}
	}
	defs, ok := doc["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("$defs is %T, want object", doc["$defs"])
	}
	for _, info := range Methods() {
		if info.Result != "" {
			if _, ok := defs[info.Result]; !ok {
				t.Errorf("method %q result schema %q is missing", info.Name, info.Result)
			}
		}
	}
	if !slices.Contains(got, string(MethodWorkspaceOpen)) {
		t.Errorf("method enum does not contain %q", MethodWorkspaceOpen)
	}
}

func TestJSONSchemaExtensions(t *testing.T) {
	doc := decodeSchema(t)
	ext, ok := doc["x-jide"].(map[string]any)
	if !ok {
		t.Fatalf("x-jide is %T, want object", doc["x-jide"])
	}
	if got, want := ext["version"], Version; got != want {
		t.Errorf("x-jide.version = %v, want %v", got, want)
	}
	if got, want := ext["transport"], "jide://"; got != want {
		t.Errorf("x-jide.transport = %v, want %v", got, want)
	}
	methods, ok := ext["methods"].([]any)
	if !ok {
		t.Fatalf("x-jide.methods is %T, want array", ext["methods"])
	}
	if len(methods) != len(Methods()) {
		t.Errorf("x-jide.methods has %d entries, want %d", len(methods), len(Methods()))
	}
	codes, ok := ext["errorCodes"].(map[string]any)
	if !ok {
		t.Fatalf("x-jide.errorCodes is %T, want object", ext["errorCodes"])
	}
	if got, want := codes["methodNotFound"], float64(CodeMethodNotFound); got != want {
		t.Errorf("x-jide.errorCodes.methodNotFound = %v, want %v", got, want)
	}
}

func TestJSONSchemaIsDeterministic(t *testing.T) {
	first, err := JSONSchema()
	if err != nil {
		t.Fatalf("JSONSchema() error = %v", err)
	}
	second, err := JSONSchema()
	if err != nil {
		t.Fatalf("JSONSchema() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("JSONSchema() is not deterministic across calls")
	}
}

func TestJSONSchemaRefsResolve(t *testing.T) {
	doc := decodeSchema(t)
	defs, ok := doc["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("$defs is %T, want object", doc["$defs"])
	}
	refs := collectRefs(doc)
	if len(refs) == 0 {
		t.Fatal("schema contains no $ref entries")
	}
	for _, ref := range refs {
		name, found := strings.CutPrefix(ref, "#/$defs/")
		if !found {
			t.Errorf("unexpected $ref %q", ref)
			continue
		}
		if _, ok := defs[name]; !ok {
			t.Errorf("$ref %q does not resolve to a $defs entry", ref)
		}
	}
}

// collectRefs walks a decoded JSON document and returns every "$ref" value.
func collectRefs(node any) []string {
	var refs []string
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "$ref" {
				if ref, ok := child.(string); ok {
					refs = append(refs, ref)
				}
				continue
			}
			refs = append(refs, collectRefs(child)...)
		}
	case []any:
		for _, child := range value {
			refs = append(refs, collectRefs(child)...)
		}
	}
	return refs
}

func TestWriteJSONSchema(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSONSchema(&buf); err != nil {
		t.Fatalf("WriteJSONSchema() error = %v", err)
	}
	want, err := JSONSchema()
	if err != nil {
		t.Fatalf("JSONSchema() error = %v", err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Error("WriteJSONSchema() output differs from JSONSchema()")
	}
}
