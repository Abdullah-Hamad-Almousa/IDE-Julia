package jide

import (
	"slices"
	"strings"
	"testing"
)

func TestMethodCatalogIsConsistent(t *testing.T) {
	catalog := Methods()
	if len(catalog) == 0 {
		t.Fatal("the method catalog is empty")
	}
	seen := make(map[Method]bool, len(catalog))
	for _, info := range catalog {
		if seen[info.Name] {
			t.Errorf("method %q appears twice in the catalog", info.Name)
		}
		seen[info.Name] = true

		if err := info.Name.Validate(); err != nil {
			t.Errorf("method %q does not follow the naming convention: %v", info.Name, err)
		}
		if !info.Name.IsKnown() {
			t.Errorf("method %q is not known", info.Name)
		}
		switch info.Direction {
		case DirectionClientToServer, DirectionServerToClient, DirectionBidirectional:
		default:
			t.Errorf("method %q has the invalid direction %q", info.Name, info.Direction)
		}
		if info.Summary == "" {
			t.Errorf("method %q has no summary", info.Name)
		}
	}
}

// TestLSPMethodsAreCatalogued checks that the methods of the language
// server surface are part of the catalog.
func TestLSPMethodsAreCatalogued(t *testing.T) {
	for _, method := range []Method{
		MethodLSPStart,
		MethodLSPStop,
		MethodLSPStatus,
		MethodLSPDiagnostics,
		MethodLSPHover,
		MethodLSPCompletion,
		MethodLSPReferences,
		MethodLSPDefinition,
		MethodLSPRename,
	} {
		if !method.IsKnown() {
			t.Errorf("LSP method %q is not in catalog", method)
		}
		if err := method.Validate(); err != nil {
			t.Errorf("LSP method %q does not follow the naming convention: %v", method, err)
		}
	}
}

// TestMethodCatalogReferencesTheLanguageServerPayloads checks that the
// methods of the language server surface name a parameter object that the
// module defines.
func TestMethodCatalogReferencesTheLanguageServerPayloads(t *testing.T) {
	known := map[string]bool{
		"lspStartParams":       true,
		"lspStopParams":        true,
		"lspStatusParams":      true,
		"lspDiagnosticsParams": true,
		"lspHoverParams":       true,
		"lspCompletionParams":  true,
		"lspReferencesParams":  true,
		"lspDefinitionParams":  true,
		"lspRenameParams":      true,
	}
	for _, info := range Methods() {
		if !strings.HasPrefix(string(info.Name), "lsp/") {
			continue
		}
		if !known[info.Params] {
			t.Errorf("method %q refers to the unknown parameter object %q", info.Name, info.Params)
		}
	}
}

func TestIndexMethodsAreCatalogued(t *testing.T) {
	for _, method := range []Method{MethodIndexSearch, MethodIndexSymbols, MethodIndexWorkspaceSymbols} {
		if !method.IsKnown() {
			t.Errorf("index method %q is not in catalog", method)
		}
	}
}

func TestIndexAndRenameMethodsPointToTypedParameters(t *testing.T) {
	want := map[Method]string{
		MethodIndexSearch:           "textSearchParams",
		MethodIndexSymbols:          "documentSymbolsParams",
		MethodIndexWorkspaceSymbols: "workspaceSymbolsParams",
		MethodLSPDefinition:         "lspDefinitionParams",
		MethodLSPRename:             "lspRenameParams",
	}
	for method, params := range want {
		info, ok := method.Info()
		if !ok || info.Params != params {
			t.Errorf("%q params = %q, %v; want %q", method, info.Params, ok, params)
		}
	}
}

func TestPackageMethodsAreCataloguedWithTypedParameters(t *testing.T) {
	want := map[Method]string{
		MethodPkgEnvironments:    "pkgEnvironmentsParams",
		MethodPkgAdd:             "pkgOperationParams",
		MethodPkgRemove:          "pkgOperationParams",
		MethodPkgUpgrade:         "pkgOperationParams",
		MethodPkgManifestDiff:    "pkgManifestDiffParams",
		MethodPkgDependencyGraph: "pkgDependencyGraphParams",
	}
	for method, params := range want {
		info, ok := method.Info()
		if !ok || info.Direction != DirectionClientToServer || info.Params != params {
			t.Errorf("%q info = %#v, %v; want client-to-server params %q", method, info, ok, params)
		}
	}
}

func TestConsoleMethodsAreCataloguedWithTypedParameters(t *testing.T) {
	want := map[Method]string{
		MethodConsoleOpen:    "consoleOpenParams",
		MethodConsoleInput:   "consoleInputParams",
		MethodConsoleOutput:  "consoleOutputParams",
		MethodConsoleReopen:  "consoleSessionParams",
		MethodConsoleClose:   "consoleSessionParams",
		MethodConsoleHistory: "consoleSessionParams",
		MethodConsoleResize:  "consoleResizeParams",
	}
	for method, params := range want {
		info, ok := method.Info()
		if !ok || info.Params != params {
			t.Errorf("%q params = %q, %v; want %q", method, info.Params, ok, params)
		}
	}
	if info, ok := MethodConsoleSessions.Info(); !ok || info.Direction != DirectionClientToServer {
		t.Errorf("console/sessions info = %#v, %v", info, ok)
	}
}

func TestRunAndTestRunMethodsHaveTypedDirectionAndPayloads(t *testing.T) {
	want := map[Method]struct {
		direction Direction
		params    string
		result    string
	}{
		MethodRunExecute:     {DirectionClientToServer, "runExecuteParams", "runExecuteResult"},
		MethodRunTerminate:   {DirectionClientToServer, "runTerminateParams", "runTerminateResult"},
		MethodRunInput:       {DirectionClientToServer, "runInputParams", "runInputResult"},
		MethodRunOutput:      {DirectionServerToClient, "runOutputParams", ""},
		MethodRunExited:      {DirectionServerToClient, "runExitedParams", ""},
		MethodTestRunExecute: {DirectionClientToServer, "testRunExecuteParams", "testRunExecuteResult"},
		MethodTestRunEvent:   {DirectionServerToClient, "testRunEventParams", ""},
	}
	for method, expected := range want {
		info, ok := method.Info()
		if !ok || info.Direction != expected.direction || info.Params != expected.params || info.Result != expected.result {
			t.Errorf("%q info = %#v, %v; want direction=%q params=%q result=%q", method, info, ok, expected.direction, expected.params, expected.result)
		}
	}
}

func TestDAPMethodsAreCataloguedWithTypedParameters(t *testing.T) {
	want := map[Method]string{
		MethodDAPStart: "dapStartParams", MethodDAPLaunch: "dapArgumentsParams", MethodDAPAttach: "dapArgumentsParams",
		MethodDAPSetBreakpoints: "dapSetBreakpointsParams", MethodDAPSetFunctionBreakpoints: "dapFunctionBreakpointsParams",
		MethodDAPSetExceptionBreakpoints: "dapExceptionBreakpointsParams",
		MethodDAPContinue:                "dapThreadParams", MethodDAPNext: "dapThreadParams", MethodDAPStepIn: "dapThreadParams",
		MethodDAPStepOut: "dapThreadParams", MethodDAPPause: "dapThreadParams", MethodDAPScopes: "dapFrameParams",
		MethodDAPStepInTargets: "dapFrameParams",
		MethodDAPVariables:     "dapVariablesParams", MethodDAPEvaluate: "dapEvaluateParams",
		MethodDAPDisconnect:  "dapDisconnectParams",
		MethodDAPSetVariable: "dapSetVariableParams", MethodDAPExceptionInfo: "dapThreadParams",
		MethodDAPStackTrace: "dapStackTraceParams", MethodDAPThreads: "dapThreadsParams",
	}
	for method, params := range want {
		info, ok := method.Info()
		if !ok || info.Direction != DirectionClientToServer || info.Params != params {
			t.Errorf("%q info = %#v, %v; want client-to-server params %q", method, info, ok, params)
		}
	}
	if info, ok := MethodDAPEvent.Info(); !ok || info.Direction != DirectionServerToClient || info.Params != "dapEventParams" {
		t.Errorf("DAP event info = %#v, %v", info, ok)
	}
	for method, result := range map[Method]string{
		MethodDAPStart: "dapStartResult", MethodDAPSetBreakpoints: "dapBreakpointsResult",
		MethodDAPSetFunctionBreakpoints: "dapBreakpointsResult", MethodDAPContinue: "dapContinueResult",
		MethodDAPSetExceptionBreakpoints: "dapBreakpointsResult", MethodDAPSetVariable: "dapSetVariableResult",
		MethodDAPExceptionInfo: "dapExceptionInfoResult", MethodDAPLaunch: "dapEmptyResult", MethodDAPAttach: "dapEmptyResult",
		MethodDAPConfigurationDone: "dapEmptyResult", MethodDAPNext: "dapEmptyResult", MethodDAPStepIn: "dapEmptyResult",
		MethodDAPStepOut: "dapEmptyResult", MethodDAPPause: "dapEmptyResult", MethodDAPDisconnect: "dapEmptyResult",
		MethodDAPTerminate:  "dapEmptyResult",
		MethodDAPStackTrace: "dapStackTraceResult", MethodDAPThreads: "dapThreadsResult",
		MethodDAPStepInTargets: "dapStepInTargetsResult",
		MethodDAPScopes:        "dapScopesResult", MethodDAPVariables: "dapVariablesResult", MethodDAPEvaluate: "dapEvaluateResult",
	} {
		if info, ok := method.Info(); !ok || info.Result != result {
			t.Errorf("%q result schema = %q, %v; want %q", method, info.Result, ok, result)
		}
	}
}

func TestMethodsReturnsACopy(t *testing.T) {
	first := Methods()
	first[0].Summary = "mutated"
	if Methods()[0].Summary == "mutated" {
		t.Error("Methods() exposes the catalog; it must return a copy")
	}
}

func TestMethodInfoLookup(t *testing.T) {
	info, ok := MethodWorkspaceOpen.Info()
	if !ok {
		t.Fatal("Info() reported the method as unknown")
	}
	if info.Name != MethodWorkspaceOpen {
		t.Errorf("Info().Name = %q, want %q", info.Name, MethodWorkspaceOpen)
	}
	if info.Direction != DirectionClientToServer {
		t.Errorf("Info().Direction = %q, want %q", info.Direction, DirectionClientToServer)
	}
	if _, ok := Method("does/not").Info(); ok {
		t.Error("Info() reported an unknown method as known")
	}
	if Method("does/not").IsKnown() {
		t.Error("IsKnown() = true for an unknown method")
	}
}

func TestMethodNamesAreSorted(t *testing.T) {
	names := MethodNames()
	if !slices.IsSorted(names) {
		t.Errorf("MethodNames() = %v, want a sorted slice", names)
	}
	if len(names) != len(Methods()) {
		t.Errorf("MethodNames() has %d entries, want %d", len(names), len(Methods()))
	}
}

func TestValidateMethodName(t *testing.T) {
	valid := []string{"ping", "workspace/open", "document/didChange", "index/search", "a/b"}
	for _, name := range valid {
		if err := ValidateMethodName(name); err != nil {
			t.Errorf("ValidateMethodName(%q) error = %v, want nil", name, err)
		}
	}
	invalid := []string{"", "workspace/", "/open", "workspace/open/extra", "Workspace/Open", "workspace/open!", "1abc", "a/1bc"}
	for _, name := range invalid {
		if err := ValidateMethodName(name); err == nil {
			t.Errorf("ValidateMethodName(%q) error = nil, want an error", name)
		}
	}
}
