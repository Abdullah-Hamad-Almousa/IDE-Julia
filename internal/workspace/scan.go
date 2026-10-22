package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// ScanOptions configures a scan of a workspace tree.
type ScanOptions struct {
	// MaxDepth limits how many directory levels below the root are listed.
	// Zero means the whole tree is listed.
	MaxDepth int
	// Exclude holds the names of the files and the directories that are
	// skipped.
	Exclude []string
}

// Entry is one node of a scanned workspace tree.
type Entry struct {
	// Path is the path of the entry relative to the root, in slash form.
	Path string
	// AbsolutePath is the absolute path of the entry as it was reached.
	AbsolutePath string
	// RealPath is the absolute path of the entry with the symbolic links
	// resolved.
	RealPath string
	// IsDir reports whether the entry is a directory.
	IsDir bool
	// Symlink reports whether the entry itself is a symbolic link.
	Symlink bool
	// OutsideRoot reports whether the entry points outside the root.
	OutsideRoot bool
	// LinkTarget is the raw target of a symbolic link.
	LinkTarget string
	// Alias reports that the entry points at a real path that the scan
	// already visited. The entry is listed, but its content is not scanned
	// a second time.
	Alias bool
	// Size is the size of the file in bytes.
	Size int64
}

// ExternalSymlink records a symbolic link that points outside the root of a
// scan.
type ExternalSymlink struct {
	// Path is the path of the link relative to the root, in slash form.
	Path string
	// Target is the raw target of the link.
	Target string
	// ResolvedPath is the absolute path the link points to.
	ResolvedPath string
}

// Tree is the result of a scan of a workspace tree.
type Tree struct {
	// Root is the absolute path of the scanned root.
	Root string
	// Entries holds the entries of the tree in depth-first order.
	Entries []Entry
	// External holds the symbolic links that point outside the root.
	External []ExternalSymlink
}

// Scan walks the tree rooted at root and returns its entries.
//
// The scan resolves the links of the tree: the symbolic links of Unix and the
// junctions of Windows, which are reparse points. A link that points outside
// the root is reported in the External list of the result and is not
// followed. A real path is visited at most once, so a link that points at an
// ancestor of the root cannot make the scan loop.
//
// A directory that cannot be read is skipped; the rest of the tree is still
// listed.
func Scan(root string, options ScanOptions) (*Tree, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: resolve %q: %w", root, err)
	}
	absolute = filepath.Clean(absolute)

	info, err := os.Stat(absolute)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("workspace: %w: %s", ErrNotFound, absolute)
		}
		return nil, fmt.Errorf("workspace: stat %q: %w", absolute, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace: %w: %s", ErrNotADirectory, absolute)
	}

	realRoot := absolute
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		realRoot = resolved
	}

	excluded := make(map[string]bool, len(options.Exclude))
	for _, name := range options.Exclude {
		excluded[name] = true
	}

	scanner := &scanner{
		realRoot: realRoot,
		options:  options,
		excluded: excluded,
		visited:  map[string]bool{normalizePath(realRoot): true},
	}
	scanner.scanDir(absolute, realRoot, "", 1)
	return &Tree{Root: absolute, Entries: scanner.entries, External: scanner.external}, nil
}

// scanner holds the state of a scan.
type scanner struct {
	realRoot string
	options  ScanOptions
	excluded map[string]bool
	visited  map[string]bool
	entries  []Entry
	external []ExternalSymlink
}

// scanDir lists the content of a directory.
//
// The level is the level of the entries the directory holds: the entries of
// the root are on the first level.
func (s *scanner) scanDir(directory, realDirectory, relative string, level int) {
	items, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	for _, item := range items {
		if s.excluded[item.Name()] {
			continue
		}
		s.scanItem(directory, realDirectory, relative, item, level)
	}
}

// scanItem lists a single entry of a directory.
func (s *scanner) scanItem(directory, realDirectory, relative string, item fs.DirEntry, level int) {
	name := item.Name()
	absolute := filepath.Join(directory, name)
	relativePath := path.Join(relative, name)

	info, err := item.Info()
	if err != nil {
		return
	}

	// A link is a symbolic link or a reparse point of Windows. The mode of a
	// junction is not reported by the operating system, so the target of the
	// link is read to detect it.
	if target, isLink := linkTarget(absolute); isLink {
		s.scanLink(absolute, relativePath, target, level)
		return
	}

	realPath := filepath.Join(realDirectory, name)
	entry := Entry{
		Path:         relativePath,
		AbsolutePath: absolute,
		RealPath:     realPath,
		IsDir:        info.IsDir(),
		Size:         info.Size(),
	}

	// No real path is visited twice: the entry that reaches it first owns it
	// and the other entries are aliases.
	key := normalizePath(realPath)
	if s.visited[key] {
		entry.Alias = true
		s.entries = append(s.entries, entry)
		return
	}
	s.visited[key] = true
	s.entries = append(s.entries, entry)

	if !entry.IsDir {
		return
	}
	if s.options.MaxDepth <= 0 || level < s.options.MaxDepth {
		s.scanDir(absolute, realPath, relativePath, level+1)
	}
}

// scanLink lists a symbolic link or a reparse point.
//
// The link is resolved. A link that points outside the root is reported and
// not followed, and a link that points at an already visited real path is
// listed as an alias.
func (s *scanner) scanLink(absolute, relative, target string, level int) {
	resolved, err := resolveLink(absolute, target)
	if err != nil {
		// A broken link is listed, but it is not followed.
		s.entries = append(s.entries, Entry{
			Path:         relative,
			AbsolutePath: absolute,
			RealPath:     absolute,
			Symlink:      true,
			LinkTarget:   target,
		})
		return
	}

	entry := Entry{
		Path:         relative,
		AbsolutePath: absolute,
		RealPath:     resolved,
		Symlink:      true,
		LinkTarget:   target,
	}

	if !withinRoot(s.realRoot, resolved) {
		entry.OutsideRoot = true
		s.entries = append(s.entries, entry)
		s.external = append(s.external, ExternalSymlink{
			Path:         relative,
			Target:       target,
			ResolvedPath: resolved,
		})
		return
	}

	key := normalizePath(resolved)
	if s.visited[key] {
		entry.Alias = true
		s.entries = append(s.entries, entry)
		return
	}

	info, err := os.Stat(resolved)
	if err != nil {
		s.entries = append(s.entries, entry)
		return
	}
	entry.IsDir = info.IsDir()
	entry.Size = info.Size()
	if !entry.IsDir {
		// No real path is visited twice: the entry that reaches it first
		// owns it and the other entries are aliases.
		fileKey := normalizePath(resolved)
		if s.visited[fileKey] {
			entry.Alias = true
			s.entries = append(s.entries, entry)
			return
		}
		s.visited[fileKey] = true
		s.entries = append(s.entries, entry)
		return
	}

	s.visited[key] = true
	s.entries = append(s.entries, entry)
	if s.options.MaxDepth <= 0 || level < s.options.MaxDepth {
		s.scanDir(absolute, resolved, relative, level+1)
	}
}

// linkTarget returns the target of a link and reports whether the path is a
// link.
//
// A symbolic link and a junction of Windows are both detected: the mode of a
// junction is not reported by the operating system, so the target of the
// link is read.
func linkTarget(path string) (string, bool) {
	target, err := os.Readlink(path)
	if err != nil {
		return "", false
	}
	return target, true
}

// resolveLink returns the real path of a link.
//
// The target of a symbolic link is relative to the folder of the link. The
// target of a junction of Windows is absolute.
func resolveLink(path, target string) (string, error) {
	candidate := target
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(filepath.Dir(path), candidate)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve the link %q: %w", path, err)
	}
	return resolved, nil
}

// withinRoot reports whether the path lies inside the root.
func withinRoot(root, candidate string) bool {
	normalizedRoot := normalizePath(root)
	normalizedCandidate := normalizePath(candidate)
	if normalizedCandidate == normalizedRoot {
		return true
	}
	return strings.HasPrefix(normalizedCandidate, normalizedRoot+string(filepath.Separator))
}

// normalizePath returns the comparable form of a path.
//
// The paths of Windows are case insensitive, so the comparison folds the
// case of the path.
func normalizePath(value string) string {
	cleaned := filepath.Clean(value)
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned
}
