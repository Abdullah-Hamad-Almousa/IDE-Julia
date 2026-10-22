package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// makeLink creates a link at the given path that points to the target.
//
// A symbolic link is used when the platform allows it. On Windows a symbolic
// link needs the developer mode or administrator rights, so a directory
// junction is used instead: it is a reparse point that needs no privilege,
// and the engine resolves it like a symbolic link.
//
// The test is skipped when the platform allows neither of them.
func makeLink(t *testing.T, target, link string) {
	t.Helper()
	err := os.Symlink(target, link)
	if err == nil {
		return
	}
	if runtime.GOOS != "windows" {
		t.Skipf("the platform does not allow the creation of a symbolic link: %v", err)
	}
	// A junction holds an absolute target.
	absolute := target
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(filepath.Dir(link), absolute)
	}
	command := exec.Command("cmd", "/c", "mklink", "/J", link, absolute)
	if output, junctionErr := command.CombinedOutput(); junctionErr != nil {
		t.Skipf("the platform does not allow the creation of a directory junction: %v: %s", junctionErr, output)
	}
}

// writeFile writes a file in a directory and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	return path
}

// entryPaths returns the relative paths of the entries of a tree.
func entryPaths(tree *Tree) []string {
	paths := make([]string, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		paths = append(paths, entry.Path)
	}
	return paths
}

// findEntry returns the entry with the given relative path.
func findEntry(t *testing.T, tree *Tree, relative string) Entry {
	t.Helper()
	for _, entry := range tree.Entries {
		if entry.Path == relative {
			return entry
		}
	}
	t.Fatalf("the tree holds no entry %q; it holds %v", relative, entryPaths(tree))
	return Entry{}
}

// TestScanListsTheTree checks the entries of a scanned tree.
func TestScanListsTheTree(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "Project.toml", "name = \"Demo\"\n")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("create the source folder: %v", err)
	}
	writeFile(t, filepath.Join(root, "src"), "Demo.jl", "x = 1\n")

	tree, err := Scan(root, ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if got, want := tree.Root, filepath.Clean(root); got != want {
		t.Errorf("tree root = %q, want %q", got, want)
	}
	want := []string{"Project.toml", "src", "src/Demo.jl"}
	got := entryPaths(tree)
	for _, path := range want {
		if !slices.Contains(got, path) {
			t.Errorf("the tree holds %v, want the entry %q", got, path)
		}
	}
	if len(got) != len(want) {
		t.Errorf("the tree holds %v, want %v", got, want)
	}
	if len(tree.External) != 0 {
		t.Errorf("the tree holds the external links %v, want none", tree.External)
	}
}

// TestScanRespectsTheMaximumDepth checks the depth limit of a scan.
func TestScanRespectsTheMaximumDepth(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("create the deep folder: %v", err)
	}
	writeFile(t, deep, "deep.jl", "x = 1\n")

	tree, err := Scan(root, ScanOptions{MaxDepth: 2})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	got := entryPaths(tree)
	if !slices.Contains(got, "a") {
		t.Errorf("the tree holds %v, want the entry %q", got, "a")
	}
	if !slices.Contains(got, "a/b") {
		t.Errorf("the tree holds %v, want the entry %q", got, "a/b")
	}
	if slices.Contains(got, "a/b/c") {
		t.Errorf("the tree holds %v, want no entry below the depth limit", got)
	}
}

// TestScanExcludesTheNames checks the exclusion of a scan.
func TestScanExcludesTheNames(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create the folder: %v", err)
	}
	writeFile(t, filepath.Join(root, ".git"), "config", "")
	writeFile(t, root, "a.jl", "x = 1\n")

	tree, err := Scan(root, ScanOptions{Exclude: []string{".git"}})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	got := entryPaths(tree)
	if slices.Contains(got, ".git") {
		t.Errorf("the tree holds %v, want no entry %q", got, ".git")
	}
	if !slices.Contains(got, "a.jl") {
		t.Errorf("the tree holds %v, want the entry %q", got, "a.jl")
	}
}

// TestScanRejectsAFile checks the error of a scan of a file.
func TestScanRejectsAFile(t *testing.T) {
	path := writeFile(t, t.TempDir(), "a.jl", "x = 1\n")
	if _, err := Scan(path, ScanOptions{}); err == nil {
		t.Fatal("Scan() of a file must fail")
	}
}

// TestScanResolvesASymlinkInsideTheRoot checks that a symbolic link that
// points inside the root is resolved and visited exactly once.
func TestScanResolvesASymlinkInsideTheRoot(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "pkg")
	if err := os.MkdirAll(filepath.Join(pkg, "src"), 0o755); err != nil {
		t.Fatalf("create the package folder: %v", err)
	}
	writeFile(t, filepath.Join(pkg, "src"), "Pkg.jl", "x = 1\n")

	deps := filepath.Join(root, "deps")
	if err := os.MkdirAll(deps, 0o755); err != nil {
		t.Fatalf("create the dependencies folder: %v", err)
	}
	makeLink(t, filepath.Join("..", "pkg"), filepath.Join(deps, "Linked"))

	tree, err := Scan(root, ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	link := findEntry(t, tree, "deps/Linked")
	if !link.Symlink {
		t.Error("the entry of the link must be marked as a symbolic link")
	}
	if link.LinkTarget == "" {
		t.Error("the entry of the link must hold the target of the link")
	}
	if got, want := link.RealPath, pkg; got != want {
		t.Errorf("the real path of the link = %q, want %q", got, want)
	}
	if link.OutsideRoot {
		t.Error("the link points inside the root")
	}

	// The content of the target is listed exactly once, no matter whether
	// the link or the folder was reached first.
	occurrences := 0
	for _, entry := range tree.Entries {
		if entry.RealPath == filepath.Join(pkg, "src", "Pkg.jl") {
			occurrences++
		}
	}
	if occurrences != 1 {
		t.Errorf("the content of the target is listed %d times, want once", occurrences)
	}
	if len(tree.External) != 0 {
		t.Errorf("the tree holds the external links %v, want none", tree.External)
	}
}

// TestScanReportsASymlinkOutsideTheRoot checks that a symbolic link that
// points outside the root is reported instead of being followed.
func TestScanReportsASymlinkOutsideTheRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create the root: %v", err)
	}
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("create the outside folder: %v", err)
	}
	writeFile(t, outside, "Outside.jl", "x = 1\n")

	external := filepath.Join(root, "external")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatalf("create the external folder: %v", err)
	}
	makeLink(t, filepath.Join("..", "..", "outside"), filepath.Join(external, "Out"))

	tree, err := Scan(root, ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if got, want := len(tree.External), 1; got != want {
		t.Fatalf("the tree holds %d external links, want %d: %v", got, want, tree.External)
	}
	link := tree.External[0]
	if got, want := link.Path, "external/Out"; got != want {
		t.Errorf("the path of the external link = %q, want %q", got, want)
	}
	if got, want := link.ResolvedPath, outside; got != want {
		t.Errorf("the resolved path of the external link = %q, want %q", got, want)
	}
	if link.Target == "" {
		t.Error("the external link must hold the target of the link")
	}

	entry := findEntry(t, tree, "external/Out")
	if !entry.OutsideRoot {
		t.Error("the entry of the link must be marked as outside the root")
	}
	// The content of the target must not be listed.
	for _, entry := range tree.Entries {
		if entry.RealPath == filepath.Join(outside, "Outside.jl") {
			t.Error("the content of an external link must not be listed")
		}
	}
}

// TestScanDoesNotLoopOnASymlinkToTheRoot checks that a symbolic link that
// points at an ancestor of the root does not make the scan loop.
func TestScanDoesNotLoopOnASymlinkToTheRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.jl", "x = 1\n")
	makeLink(t, ".", filepath.Join(root, "loop"))

	tree, err := Scan(root, ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	occurrences := 0
	for _, entry := range tree.Entries {
		if entry.RealPath == filepath.Join(root, "a.jl") {
			occurrences++
		}
	}
	if occurrences != 1 {
		t.Errorf("the content of the root is listed %d times, want once", occurrences)
	}
	link := findEntry(t, tree, "loop")
	if !link.Alias {
		t.Error("the link that points at the root must be marked as an alias")
	}
}

// TestScanListsABrokenSymlink checks that a symbolic link without a target is
// listed instead of being followed.
func TestScanListsABrokenSymlink(t *testing.T) {
	root := t.TempDir()
	makeLink(t, "missing", filepath.Join(root, "broken"))

	tree, err := Scan(root, ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	entry := findEntry(t, tree, "broken")
	if !entry.Symlink {
		t.Error("the entry of the link must be marked as a symbolic link")
	}
	if !strings.HasSuffix(entry.LinkTarget, "missing") {
		t.Errorf("the target of the link = %q, want a target that ends with %q", entry.LinkTarget, "missing")
	}
}

// TestScanVisitsAFileOnce checks that a file that is reachable through a link
// and through its own path is listed once.
func TestScanVisitsAFileOnce(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.jl", "x = 1\n")
	makeLink(t, "a.jl", filepath.Join(root, "link.jl"))

	tree, err := Scan(root, ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	occurrences := 0
	aliases := 0
	for _, entry := range tree.Entries {
		if entry.RealPath != filepath.Join(root, "a.jl") {
			continue
		}
		occurrences++
		if entry.Alias {
			aliases++
		}
	}
	if occurrences != 2 {
		t.Fatalf("the file is listed %d times, want the two paths that reach it", occurrences)
	}
	if aliases != 1 {
		t.Errorf("%d of the entries are aliases, want one", aliases)
	}
}
