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
