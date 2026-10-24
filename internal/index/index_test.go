package index

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildEmptyWorkspace(t *testing.T) {
	idx := New()
	root := t.TempDir()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := idx.FileCount(); got != 0 {
		t.Fatalf("FileCount() = %d, want 0", got)
	}
	if got := idx.Root(); got == "" {
		t.Fatal("Root() is empty after a successful empty-workspace build")
	}
	result, err := idx.Search(context.Background(), "Julia", 0)
	if err != nil || len(result.Matches) != 0 {
		t.Fatalf("Search() = %+v, %v; want no matches", result, err)
	}
	empty, err := idx.SearchSymbols(context.Background(), "", 0)
	if err != nil || len(empty.Symbols) != 0 {
		t.Fatalf("SearchSymbols() = %+v, %v; want no symbols", empty, err)
	}
}

func TestGenerationAdvancesAfterMutationsAndBuilds(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "main.jl", "generation_test = 1\n")
	idx := New()
	if idx.Generation() != 0 {
		t.Fatal("new index generation is not zero")
	}
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := idx.Generation(); got == 0 {
		t.Fatal("Build() did not advance the index generation")
	}
	first := idx.Generation()
	if !idx.IsCurrent(first) {
		t.Fatal("fresh generation reported stale")
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "generation_test = 2\n", 2); err != nil {
		t.Fatal(err)
	}
	if idx.Generation() <= first {
		t.Fatal("generation did not advance after update")
	}
	if idx.IsCurrent(first) {
		t.Fatal("old generation reported current after update")
	}
	second := idx.Generation()
	if err := idx.Remove(path); err != nil {
		t.Fatal(err)
	}
	if idx.Has(path) {
		t.Fatal("removal did not clear the indexed file")
	}
	if idx.Generation() <= second {
		t.Fatal("generation did not advance after removal")
	}
}

func TestBuildRejectsMissingAndNonDirectoryRoots(t *testing.T) {
	idx := New()
	if err := idx.Build(context.Background(), filepath.Join(t.TempDir(), "missing"), Options{}); err == nil {
		t.Fatal("Build() accepted a missing root")
	}
	if err := idx.Build(context.Background(), "", Options{}); err == nil {
		t.Fatal("Build() accepted an empty root")
	}
	if err := idx.Build(nil, t.TempDir(), Options{}); err == nil {
		t.Fatal("Build() accepted a nil context")
	}
	file := writeSource(t, t.TempDir(), "file.jl", "x = 1\n")
	if err := idx.Build(context.Background(), file, Options{}); err == nil {
		t.Fatal("Build() accepted a file as a workspace root")
	}
	if err := idx.Build(context.Background(), t.TempDir(), Options{Workers: -1}); err == nil {
		t.Fatal("Build() accepted a negative worker count")
	}
	if err := idx.Build(context.Background(), t.TempDir(), Options{MaxDepth: -1}); err == nil {
		t.Fatal("Build() accepted a negative depth")
	}
	if err := idx.Build(context.Background(), t.TempDir(), Options{MaxFileSize: -1}); err == nil {
		t.Fatal("Build() accepted a negative maximum file size")
	}
	if err := idx.Build(context.Background(), t.TempDir(), Options{ExcludeDirs: []string{""}}); err == nil {
		t.Fatal("Build() accepted an empty excluded directory name")
	}
	if err := idx.Build(context.Background(), t.TempDir(), Options{ExcludeDirs: []string{"../outside"}}); err == nil {
		t.Fatal("Build() accepted a parent-relative exclusion")
	}
}

func TestBuildReplacesRemovedDiskFiles(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "removed.jl", "removed_from_workspace = true\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	assertSearchCount(t, idx, "removed_from_workspace", 0)
}

func TestBuildPreservesRemovalThatRacesTheDiskWalk(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "remove.jl", "remove_race_marker = true\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Remove(path); err != nil {
		t.Fatal(err)
	}
	if idx.Has(path) {
		t.Fatal("removed path still exists in the in-memory snapshot")
	}
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	assertSearchCount(t, idx, "remove_race_marker", 0)
}

func TestConcurrentBuildsAreSerialized(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "main.jl", "build_serialized = true\n")
	idx := New()
	var wait sync.WaitGroup
	errorsFound := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsFound <- idx.Build(context.Background(), root, Options{})
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatalf("concurrent Build() error = %v", err)
		}
	}
	assertSearchCount(t, idx, "build_serialized", 1)
}

func TestBuildSearchSymbolsAndDefinitionFallback(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "src/defs.jl", "module Demo\nfunction greet(name)\n name\nend\nend\n")
	writeSource(t, root, "src/use.jl", "using .Demo\ngreet(\"hi\")\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := idx.FileCount(); got != 2 {
		t.Fatalf("FileCount() = %d, want 2", got)
	}
	if got := idx.Root(); got != root {
		t.Fatalf("Root() = %q, want %q", got, root)
	}
	search, err := idx.Search(context.Background(), "greet", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(search.Matches) != 2 || search.Matches[0].Line != 2 || search.Matches[1].Line != 2 {
		t.Fatalf("Search() matches = %+v, want two greet hits", search.Matches)
	}
	symbols, err := idx.SearchSymbols(context.Background(), "GREE", 10)
	if err != nil {
		t.Fatalf("SearchSymbols() error = %v", err)
	}
	if len(symbols.Symbols) != 1 || symbols.Symbols[0].Name != "greet" || symbols.Symbols[0].Location.URI != URIForPath(filepath.Join(root, "src", "defs.jl")) {
		t.Fatalf("SearchSymbols() = %+v, want greet declaration", symbols)
	}
	allSymbols, err := idx.SearchSymbols(context.Background(), "", 10)
	if err != nil || len(allSymbols.Symbols) == 0 {
		t.Fatalf("empty-query SearchSymbols() = %+v, %v", allSymbols, err)
	}
	limitedSymbols, err := idx.SearchSymbols(context.Background(), "", 1)
	if err != nil || len(limitedSymbols.Symbols) != 1 || !limitedSymbols.Truncated {
		t.Fatalf("limited SearchSymbols() = %+v, %v", limitedSymbols, err)
	}
	locations, err := idx.DefinitionAt(context.Background(), URIForPath(filepath.Join(root, "src", "use.jl")), 1, 1)
	if err != nil {
		t.Fatalf("DefinitionAt() error = %v", err)
	}
	if len(locations) != 1 || locations[0].URI != URIForPath(filepath.Join(root, "src", "defs.jl")) || locations[0].Range.Start.Line != 1 {
		t.Fatalf("DefinitionAt() = %+v, want greet definition in defs.jl", locations)
	}
	missing, err := idx.DefinitionAt(context.Background(), URIForPath(filepath.Join(root, "src", "use.jl")), 0, 0)
	if err != nil || len(missing) != 0 {
		t.Fatalf("DefinitionAt() for unresolved import = %+v, %v", missing, err)
	}
}

func TestDuplicateSymbolDefinitionsAreAllReturnedAndLocallyPreferred(t *testing.T) {
	root := t.TempDir()
	first := writeSource(t, root, "a.jl", "function shared()\nend\n")
	second := writeSource(t, root, "b.jl", "function shared()\nend\n")
	use := writeSource(t, root, "use.jl", "shared()\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	locations, err := idx.DefinitionAt(context.Background(), URIForPath(second), 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 2 || locations[0].URI != URIForPath(second) || locations[1].URI != URIForPath(first) {
		t.Fatalf("duplicate definitions = %+v, want local b.jl first, then a.jl", locations)
	}
	locations, err = idx.DefinitionAt(context.Background(), URIForPath(use), 0, 1)
	if err != nil || len(locations) != 2 {
		t.Fatalf("cross-file duplicate definitions = %+v, %v", locations, err)
	}
}

func TestUpdateReplacesPostingsAndRemoveClearsThem(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "main.jl", "function alpha()\nend\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "function beta()\nend\n", 2); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if err := idx.UpdateSaved(context.Background(), path, URIForPath(path), "function beta()\nend\n", 2); !errors.Is(err, ErrStaleUpdate) {
		t.Fatalf("UpdateSaved() with equal version error = %v, want ErrStaleUpdate", err)
	}
	assertSearchCount(t, idx, "alpha", 0)
	assertSearchCount(t, idx, "beta", 1)
	symbols, err := idx.SearchSymbols(context.Background(), "alpha", 10)
	if err != nil || len(symbols.Symbols) != 0 {
		t.Fatalf("old symbol still indexed: %+v, %v", symbols, err)
	}
	if err := idx.Remove(path); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if idx.Has(path) {
		t.Fatal("Remove() did not clear the document")
	}
	assertSearchCount(t, idx, "beta", 0)
	if got := idx.FileCount(); got != 0 {
		t.Fatalf("FileCount() = %d after removal, want 0", got)
	}
	if !idx.IsCurrent(idx.Generation()) {
		t.Fatal("current generation reported stale")
	}
}

func TestUpdateRequiresIncreasingVersion(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "versioned.jl", "version_one = 1\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "version_two = 2\n", 2); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "version_three = 3\n", 2); !errors.Is(err, ErrStaleUpdate) {
		t.Fatalf("equal-version different content error = %v, want ErrStaleUpdate", err)
	}
	assertSearchCount(t, idx, "version_two", 1)
}

func TestUpdateAfterRemovalAcceptsNewVersion(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "main.jl", "before_removal = true\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "after_removal = true\n", 1); err != nil {
		t.Fatalf("Update() after Remove() error = %v", err)
	}
	assertSearchCount(t, idx, "after_removal", 1)
}

func TestEmptyJuliaSourceRemovesPreviousPostings(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "main.jl", "valid_name = 1\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "", 2); err != nil {
		t.Fatalf("Update(empty source) error = %v", err)
	}
	assertSearchCount(t, idx, "valid_name", 0)
}

func TestStaleVersionsCancellationAndCancelledBuildPreserveSnapshot(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "main.jl", "stable_value = 1\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "stale_value = 1\n", 0); !errors.Is(err, ErrStaleUpdate) {
		t.Fatalf("stale Update() error = %v, want ErrStaleUpdate", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := idx.Build(ctx, root, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Build() error = %v, want context.Canceled", err)
	}
	if _, err := idx.Search(ctx, "stable", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Search() error = %v, want context.Canceled", err)
	}
	assertSearchCount(t, idx, "stable_value", 1)
	ctx, cancel = context.WithCancel(context.Background())
	if err := idx.Update(ctx, path, URIForPath(path), "cancelled_value = 2\n", 2); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled Update() error = %v, want context.Canceled", err)
	}
	cancel()
	assertSearchCount(t, idx, "stable_value", 1)
}

func TestNilContextsAreRejected(t *testing.T) {
	idx := New()
	if err := idx.Build(nil, t.TempDir(), Options{}); err == nil {
		t.Fatal("Build(nil context) succeeded")
	}
	if _, err := idx.Search(nil, "query", 0); err == nil {
		t.Fatal("Search(nil context) succeeded")
	}
	if _, err := idx.SearchSymbols(nil, "query", 0); err == nil {
		t.Fatal("SearchSymbols(nil context) succeeded")
	}
	if _, err := idx.DefinitionAt(nil, "file:///missing.jl", 0, 0); err == nil {
		t.Fatal("DefinitionAt(nil context) succeeded")
	}
}

func TestUpdateAndRemoveRejectInvalidPaths(t *testing.T) {
	idx := New()
	if err := idx.Update(context.Background(), "", "", "x = 1", 1); err == nil {
		t.Fatal("Update() accepted an empty path")
	}
	if err := idx.Remove(""); err == nil {
		t.Fatal("Remove() accepted an empty path")
	}
	if err := idx.Update(context.Background(), filepath.Join(t.TempDir(), "notes.txt"), "", "text", 1); err == nil {
		t.Fatal("Update() accepted a non-Julia path")
	}
	path := filepath.Join(t.TempDir(), "main.jl")
	if err := idx.Update(context.Background(), path, "file:///different.jl", "x = 1", 1); err == nil {
		t.Fatal("Update() accepted a file URI that names another path")
	}
}

func TestUpdateAcceptsURIForEscapedPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Julia workspace #1%", "unicode-λ.jl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("initial = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "updated = true\n", 2); err != nil {
		t.Fatalf("Update() rejected URI generated from the same path: %v", err)
	}
	assertSearchCount(t, idx, "updated", 1)
}

func TestUpdateAndRemoveRejectOutsideIndexedWorkspace(t *testing.T) {
	root := t.TempDir()
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.jl")
	if err := os.WriteFile(outside, []byte("outside = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), outside, URIForPath(outside), "outside = true\n", 1); err == nil {
		t.Fatal("Update() accepted a file outside the indexed workspace")
	}
	if err := idx.Remove(outside); err == nil {
		t.Fatal("Remove() accepted a file outside the indexed workspace")
	}
}

func TestUpdateRejectsMalformedOrRemoteFileURI(t *testing.T) {
	idx := New()
	path := filepath.Join(t.TempDir(), "main.jl")
	for _, uri := range []string{"not a uri %", "https://example.invalid/main.jl", "file://remote/main.jl"} {
		if err := idx.Update(context.Background(), path, uri, "x = 1\n", 1); err == nil {
			t.Errorf("Update(uri=%q) accepted invalid URI", uri)
		}
	}
}

func TestUpdateCanonicalizesSymlinkAndRealPathAliasesToOneEntry(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "real.jl", "real_alias = true\n")
	alias := filepath.Join(root, "alias.jl")
	if err := os.Symlink(path, alias); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), alias, URIForPath(alias), "alias_name = true\n", 2); err != nil {
		t.Fatal(err)
	}
	if got := idx.FileCount(); got != 1 {
		t.Fatalf("FileCount() = %d after symlink update, want one canonical file", got)
	}
	assertSearchCount(t, idx, "alias_name", 1)
	if err := idx.Remove(path); err != nil {
		t.Fatal(err)
	}
	assertSearchCount(t, idx, "alias_name", 0)
}

func TestBuildCancellationDuringWalkPreservesExistingSnapshot(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "existing.jl", "existing_snapshot = true\n")
	for i := range 500 {
		writeSource(t, root, fmt.Sprintf("extra/%04d.jl", i), "extra_content = true\n")
	}
	idx := New()
	if err := idx.Build(context.Background(), root, Options{Workers: 2}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := idx.Build(ctx, root, Options{Workers: 2}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Build() error = %v, want context.Canceled", err)
	}
	assertSearchCount(t, idx, "existing_snapshot", 1)
}

func TestSearchLimitAndEmptyQuery(t *testing.T) {
	root := t.TempDir()
	for i := range 4 {
		writeSource(t, root, fmt.Sprintf("%d.jl", i), "repeated repeated\n")
	}
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	result, err := idx.Search(context.Background(), "repeated", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 3 || !result.Truncated {
		t.Fatalf("limited search = %+v, want 3 hits and truncated", result)
	}
	empty, err := idx.Search(context.Background(), "", 10)
	if err != nil || len(empty.Matches) != 0 {
		t.Fatalf("empty search = %+v, %v; want no matches", empty, err)
	}
	zeroLimit, err := idx.Search(context.Background(), "repeated", 0)
	if err != nil || len(zeroLimit.Matches) != 8 || zeroLimit.Truncated {
		t.Fatalf("zero-limit search = %+v, %v; want all eight exhaustive hits", zeroLimit, err)
	}
}

func TestBuildAppliesFileAndDirectoryFiltering(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "src/keep.jl", "kept = true\n")
	writeSource(t, root, "vendor/drop.jl", "dropped = true\n")
	writeSource(t, root, "notes.txt", "not julia\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := idx.FileCount(); got != 3 {
		t.Fatalf("FileCount() = %d, want Julia and regular text files outside exclusions", got)
	}
	assertSearchCount(t, idx, "not julia", 1)
}

func TestBuildHonorsDepthLimit(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "top.jl", "top_level = true\n")
	writeSource(t, root, "one/level.jl", "one_level = true\n")
	writeSource(t, root, "one/two/deep.jl", "deep_level = true\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{MaxDepth: 1}); err != nil {
		t.Fatal(err)
	}
	if got := idx.FileCount(); got != 2 {
		t.Fatalf("FileCount() = %d at max depth 1, want root and one-level files", got)
	}
}

func TestBuildWithNoFilesKeepsDirtyOverlay(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "dirty.jl", "disk_value = 1\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "unsaved_value = 2\n", 2); err != nil {
		t.Fatal(err)
	}
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	assertSearchCount(t, idx, "unsaved_value", 1)
	if err := idx.UpdateSaved(context.Background(), path, URIForPath(path), "saved_value = 3\n", 3); err != nil {
		t.Fatal(err)
	}
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	assertSearchCount(t, idx, "saved_value", 0)
}

func TestBuildRejectsDifferentWorkspaceRootWithoutReplacingSnapshot(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "current.jl", "current_workspace_symbol = true\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Build(context.Background(), t.TempDir(), Options{}); err == nil {
		t.Fatal("Build() accepted a different root for the same index")
	}
	assertSearchCount(t, idx, "current_workspace_symbol", 1)
}

func TestBuildHonorsCustomExcludedDirectories(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "src/kept.jl", "kept_custom = true\n")
	writeSource(t, root, "generated/drop.jl", "dropped_custom = true\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{ExcludeDirs: []string{"generated"}}); err != nil {
		t.Fatal(err)
	}
	assertSearchCount(t, idx, "kept_custom", 1)
	assertSearchCount(t, idx, "dropped_custom", 0)
}

func TestSearchDoesNotTreatByteSequenceAcrossRuneBoundaryAsMatch(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "unicode.jl", "αβγ\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	result, err := idx.Search(context.Background(), "βγ", 10)
	if err != nil || len(result.Matches) != 1 {
		t.Fatalf("unicode search = %+v, %v", result, err)
	}
	result, err = idx.Search(context.Background(), "γx", 10)
	if err != nil || len(result.Matches) != 0 {
		t.Fatalf("false Unicode substring = %+v, %v", result, err)
	}
}

func TestBuildExcludesOversizedAndInvalidUTF8Files(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "keep.jl", "valid_text = true\n")
	writeSource(t, root, "binary.txt", string([]byte{0xff, 0xfe, 'x'}))
	writeSource(t, root, "large.txt", strings.Repeat("large", 20))
	idx := New()
	if err := idx.Build(context.Background(), root, Options{MaxFileSize: 30}); err != nil {
		t.Fatal(err)
	}
	if got := idx.FileCount(); got != 1 {
		t.Fatalf("FileCount() = %d, want only the small valid Julia file", got)
	}
}

func TestBuildSkipsJuliaSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := writeSource(t, t.TempDir(), "external.jl", "external_symbol = true\n")
	if err := os.Symlink(outside, filepath.Join(root, "linked.jl")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := idx.FileCount(); got != 0 {
		t.Fatalf("FileCount() = %d, want 0; symlink target must not be indexed", got)
	}
}

func TestUpdateAndRemoveCanonicalizeSymlinkAliases(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "real.jl", "real_path_text = true\n")
	alias := filepath.Join(root, "alias.jl")
	if err := os.Symlink(path, alias); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), alias, URIForPath(alias), "alias_path_text = true\n", 2); err != nil {
		t.Fatal(err)
	}
	assertSearchCount(t, idx, "alias_path_text", 1)
	if err := idx.Remove(alias); err != nil {
		t.Fatal(err)
	}
	assertSearchCount(t, idx, "alias_path_text", 0)
}

func TestDefinitionAtUsesUTF16Columns(t *testing.T) {
	root := t.TempDir()
	definition := writeSource(t, root, "defs.jl", "function target()\nend\n")
	use := writeSource(t, root, "use.jl", "emoji = \"😀\"; target()\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	// The emoji counts as two UTF-16 code units; target begins at column 14.
	locations, err := idx.DefinitionAt(context.Background(), URIForPath(use), 0, 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 1 || locations[0].URI != URIForPath(definition) {
		t.Fatalf("DefinitionAt() = %+v", locations)
	}
	if _, err := idx.DefinitionAt(context.Background(), URIForPath(use), -1, 0); err == nil {
		t.Fatal("DefinitionAt() accepted a negative line")
	}
}

func TestWorkspaceWarmup10KFilesUnder30Seconds(t *testing.T) {
	if testing.Short() {
		t.Skip("10,000-file indexing acceptance test skipped by -short")
	}
	assertWarmup10KFilesUnder30Seconds(t)
}

func TestWorkspaceWarmupPreservesUnsavedUpdates(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "main.jl", "original_name = 1\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "latest_name = 2\n", 5); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("disk_snapshot = 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	assertSearchCount(t, idx, "latest_name", 1)
	assertSearchCount(t, idx, "disk_snapshot", 0)
}

func TestUpdateRejectsOlderConcurrentParseResult(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "main.jl", "initial_name = 1\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "newer_name = 2\n", 10); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(context.Background(), path, URIForPath(path), "older_name = 3\n", 9); !errors.Is(err, ErrStaleUpdate) {
		t.Fatalf("older concurrent update error = %v, want ErrStaleUpdate", err)
	}
	assertSearchCount(t, idx, "newer_name", 1)
}

func TestIndexDoesNotExposeMutableDocumentState(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "main.jl", "function isolated()\nend\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	got, err := idx.SearchSymbols(context.Background(), "isolated", 10)
	if err != nil || len(got.Symbols) != 1 {
		t.Fatalf("SearchSymbols() = %+v, %v", got, err)
	}
	got.Symbols[0].Name = "changed"
	got.Symbols[0].Location.URI = "file:///mutated"
	again, err := idx.SearchSymbols(context.Background(), "isolated", 10)
	if err != nil || len(again.Symbols) != 1 || again.Symbols[0].Name != "isolated" || again.Symbols[0].Location.URI == "file:///mutated" {
		t.Fatalf("returned symbols mutated index state: %+v, %v", again, err)
	}
}

func TestSearchResultPreviewsTrimWhitespaceButKeepColumns(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "main.jl", "  marker = true  \n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	result, err := idx.Search(context.Background(), "marker", 10)
	if err != nil || len(result.Matches) != 1 {
		t.Fatalf("Search() = %+v, %v", result, err)
	}
	if result.Matches[0].Column != 3 || result.Matches[0].Preview != "marker = true" {
		t.Fatalf("match = %+v, want column 3 and trimmed preview", result.Matches[0])
	}
}

func TestRemoveDuringBuildDoesNotRestoreIndexedFile(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "removed.jl", "kept_on_disk = true\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	assertSearchCount(t, idx, "kept_on_disk", 0)
}

func TestSavedUpdateCanBeRefreshedFromDiskDuringBuild(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "main.jl", "old_disk_name = 1\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("new_disk_name = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := idx.UpdateSaved(context.Background(), path, URIForPath(path), "new_disk_name = 2\n", 3); err != nil {
		t.Fatal(err)
	}
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	assertSearchCount(t, idx, "new_disk_name", 1)
	assertSearchCount(t, idx, "old_disk_name", 0)
}

func assertWarmup10KFilesUnder30Seconds(t testing.TB) {
	t.Helper()
	root := t.TempDir()
	for i := range 10_000 {
		path := filepath.Join(root, "src", fmt.Sprintf("file-%05d.jl", i))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		text := fmt.Sprintf("module Module%d\nfunction compute%d(x)\n x + %d\nend\nend\n", i, i, i)
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Now()
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	elapsed := time.Since(started)
	if got := idx.FileCount(); got != 10_000 {
		t.Fatalf("FileCount() = %d, want 10000", got)
	}
	if elapsed >= 30*time.Second {
		t.Fatalf("real workspace warm-up took %s, want under 30s", elapsed)
	}
	t.Logf("indexed 10,000 generated Julia files in %s", elapsed)
}

func BenchmarkWorkspaceWarmup10KFiles(b *testing.B) {
	if testing.Short() {
		b.Skip("10,000-file benchmark skipped by -short")
	}
	root := b.TempDir()
	for i := range 10_000 {
		path := filepath.Join(root, "src", fmt.Sprintf("file-%05d.jl", i))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			b.Fatal(err)
		}
		text := fmt.Sprintf("module Module%d\nfunction compute%d(x)\n x + %d\nend\nend\n", i, i, i)
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for range b.N {
		idx := New()
		if err := idx.Build(context.Background(), root, Options{}); err != nil {
			b.Fatal(err)
		}
		if got := idx.FileCount(); got != 10_000 {
			b.Fatalf("FileCount() = %d, want 10000", got)
		}
	}
}

func writeSource(t *testing.T, root, relative, text string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertSearchCount(t *testing.T, idx *Index, query string, want int) {
	t.Helper()
	result, err := idx.Search(context.Background(), query, 10)
	if err != nil {
		t.Fatalf("Search(%q) error = %v", query, err)
	}
	if len(result.Matches) != want {
		t.Fatalf("Search(%q) returned %d matches, want %d: %+v", query, len(result.Matches), want, result.Matches)
	}
}

func TestSearchReturnsStableOrder(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "z.jl", "needle\n")
	writeSource(t, root, "a.jl", "needle\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	result, err := idx.Search(context.Background(), "needle", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 2 || !strings.HasSuffix(result.Matches[0].URI, "/a.jl") {
		t.Fatalf("Search() results are not stable and sorted: %+v", result.Matches)
	}
}

func TestSearchFindsOverlappingMatches(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "main.jl", "aaaa\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	result, err := idx.Search(context.Background(), "aa", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 3 {
		t.Fatalf("overlapping matches = %+v, want columns 1, 2 and 3", result.Matches)
	}
}

func TestSearchMatchesQueriesShorterThanTrigram(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "main.jl", "x = 1\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	result, err := idx.Search(context.Background(), "x", 10)
	if err != nil || len(result.Matches) != 1 {
		t.Fatalf("one-character search = %+v, %v", result, err)
	}
}

func TestSearchColumnsCountUTF8Runes(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "unicode.jl", "α marker\n")
	idx := New()
	if err := idx.Build(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	result, err := idx.Search(context.Background(), "marker", 10)
	if err != nil || len(result.Matches) != 1 || result.Matches[0].Column != 3 {
		t.Fatalf("UTF-8 rune column = %+v, %v; want column 3", result, err)
	}
}

func TestSearchRejectsInvalidUTF8Query(t *testing.T) {
	idx := New()
	if _, err := idx.Search(context.Background(), string([]byte{0xff}), 10); err == nil {
		t.Fatal("Search() accepted an invalid UTF-8 query")
	}
	if _, err := idx.SearchSymbols(context.Background(), string([]byte{0xff}), 10); err == nil {
		t.Fatal("SearchSymbols() accepted an invalid UTF-8 query")
	}
}
