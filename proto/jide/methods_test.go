package jide

import (
	"slices"
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
