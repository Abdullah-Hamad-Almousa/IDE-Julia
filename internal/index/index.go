package index

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/jide-dev/jide/internal/syntax"
	jide "github.com/jide-dev/jide/proto/jide"
)

var (
	// ErrStaleUpdate indicates that an update is older than the indexed snapshot.
	ErrStaleUpdate = errors.New("index: stale document update")
)

const defaultMaxFileSize = 8 << 20

// Options configures workspace warm-up.
type Options struct {
	// MaxDepth limits recursion below the workspace root. Zero is unlimited.
	MaxDepth int
	// Workers is the number of concurrent file readers and Julia parsers. Zero
	// selects a conservative value based on GOMAXPROCS.
	Workers int
	// MaxFileSize excludes larger files. Zero uses 8 MiB.
	MaxFileSize int64
	// ExcludeDirs contains directory names that are not source workspace content.
	// When empty, common generated and dependency directories are excluded.
	ExcludeDirs []string
}

type document struct {
	path     string
	uri      string
	text     string
	version  int64
	parsed   *syntax.Document
	trigrams map[string]struct{}
	symbols  []jide.WorkspaceSymbol
}

// Index is a concurrent-safe, in-memory index of workspace documents.
// Updates parse outside the lock, then atomically replace the previous
// document and its postings. Search results are snapshots and never expose
// internal index state.
type Index struct {
	buildMu      sync.Mutex
	mu           sync.RWMutex
	root         string
	documents    map[string]*document
	postings     map[string]map[string]struct{}
	symbols      map[string][]jide.WorkspaceSymbol
	overlays     map[string]bool
	pathRevision map[string]uint64
	generation   uint64
}

// New returns an empty workspace text and Julia symbol index.
func New() *Index {
	return &Index{
		documents:    make(map[string]*document),
		postings:     make(map[string]map[string]struct{}),
		symbols:      make(map[string][]jide.WorkspaceSymbol),
		overlays:     make(map[string]bool),
		pathRevision: make(map[string]uint64),
	}
}

// Build indexes regular text files below root, using the Julia syntax parser
// for .jl files. The completed snapshot is published atomically; cancellation
// or a parse error leaves the existing index untouched. Updates made during
// the build are merged into the published snapshot.
func (idx *Index) Build(ctx context.Context, root string, options Options) error {
	if idx == nil {
		return errors.New("index: nil index")
	}
	if strings.TrimSpace(root) == "" {
		return errors.New("index: workspace root is empty")
	}
	if ctx == nil {
		return errors.New("index: build context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("index: resolve workspace root: %w", err)
	}
	absoluteRoot, err = filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return fmt.Errorf("index: resolve workspace root links: %w", err)
	}
	absoluteRoot, err = filepath.Abs(absoluteRoot)
	if err != nil {
		return fmt.Errorf("index: resolve real workspace root: %w", err)
	}
	idx.buildMu.Lock()
	defer idx.buildMu.Unlock()
	idx.mu.RLock()
	activeRoot := idx.root
	idx.mu.RUnlock()
	if activeRoot != "" && pathKey(activeRoot) != pathKey(absoluteRoot) {
		return errors.New("index: cannot rebuild an index for a different workspace root")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	idx.mu.RLock()
	startingRevision := make(map[string]uint64, len(idx.pathRevision))
	for key, revision := range idx.pathRevision {
		startingRevision[key] = revision
	}
	idx.mu.RUnlock()
	rootInfo, err := os.Stat(absoluteRoot)
	if err != nil {
		return fmt.Errorf("index: stat workspace root: %w", err)
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("index: workspace root %q is not a directory", absoluteRoot)
	}
	if options.MaxDepth < 0 || options.MaxFileSize < 0 || options.Workers < 0 {
		return errors.New("index: build options cannot be negative")
	}
	excluded := make(map[string]struct{}, len(options.ExcludeDirs))
	if len(options.ExcludeDirs) == 0 {
		options.ExcludeDirs = []string{".git", ".julia", ".vscode", "node_modules", "vendor", "build", "dist"}
	}
	for _, name := range options.ExcludeDirs {
		if strings.TrimSpace(name) == "" {
			return errors.New("index: excluded directory name cannot be empty")
		}
		if filepath.IsAbs(name) || filepath.Clean(name) != name || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("index: excluded directory %q must be a simple directory name", name)
		}
		excluded[normalizeDirectoryName(name)] = struct{}{}
	}
	maxDepth := options.MaxDepth
	maxSize := options.MaxFileSize
	if maxSize == 0 {
		maxSize = defaultMaxFileSize
	}
	paths := make([]string, 0)
	err = filepath.WalkDir(absoluteRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != absoluteRoot {
				if maxDepth > 0 && relativeDepth(absoluteRoot, path) > maxDepth {
					return filepath.SkipDir
				}
				if _, skip := excluded[normalizeDirectoryName(entry.Name())]; skip {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() || info.Size() > maxSize {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("index: walk workspace: %w", err)
	}
	sort.Strings(paths)

	workers := options.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
		workers = min(workers, 32)
		if workers < 1 {
			workers = 1
		}
	}
	if workers > len(paths) {
		workers = len(paths)
	}
	built := make([]*document, len(paths))
	if len(paths) > 0 {
		jobs := make(chan int)
		producerDone := make(chan struct{})
		var workersWG sync.WaitGroup
		var firstErr error
		var errMu sync.Mutex
		for range workers {
			workersWG.Add(1)
			go func() {
				defer workersWG.Done()
				for i := range jobs {
					if ctx.Err() != nil {
						continue
					}
					item, itemErr := readDocument(paths[i], maxSize)
					if itemErr != nil {
						errMu.Lock()
						if firstErr == nil {
							firstErr = itemErr
						}
						errMu.Unlock()
						continue
					}
					if item == nil {
						continue
					}
					if !utf8.ValidString(item.text) {
						continue
					}
					if strings.EqualFold(filepath.Ext(item.path), ".jl") {
						parsed, parseErr := syntax.Parse(item.text)
						if parseErr != nil {
							errMu.Lock()
							if firstErr == nil {
								firstErr = fmt.Errorf("index: parse %q: %w", item.path, parseErr)
							}
							errMu.Unlock()
							continue
						}
						item.parsed = parsed
					}
					item.uri = URIForPath(item.path)
					item.trigrams = makeTrigrams(item.text)
					if item.parsed != nil {
						item.symbols = documentSymbols(item)
						item.parsed = nil
					}
					built[i] = item
				}
			}()
		}
		go func() {
			defer close(producerDone)
			defer close(jobs)
			for i := range paths {
				select {
				case jobs <- i:
				case <-ctx.Done():
					return
				}
			}
		}()
		workersWG.Wait()
		<-producerDone
		if err := ctx.Err(); err != nil {
			return err
		}
		if firstErr != nil {
			return firstErr
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	newDocuments := make(map[string]*document, len(built))
	newPostings := make(map[string]map[string]struct{})
	newSymbols := make(map[string][]jide.WorkspaceSymbol)
	for _, item := range built {
		if item == nil {
			continue
		}
		key := pathKey(item.path)
		newDocuments[key] = item
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	// Preserve dirty editor buffers and edits that arrived after the disk walk
	// began; discard tombstoned paths that were removed during the walk.
	for key, latest := range idx.documents {
		if idx.overlays[key] || idx.pathRevision[key] != startingRevision[key] {
			newDocuments[key] = latest
		}
	}
	for key, revision := range idx.pathRevision {
		if revision != startingRevision[key] && idx.documents[key] == nil {
			delete(newDocuments, key)
		}
	}
	newPostings = make(map[string]map[string]struct{})
	newSymbols = make(map[string][]jide.WorkspaceSymbol)
	for key, item := range newDocuments {
		for trigram := range item.trigrams {
			if newPostings[trigram] == nil {
				newPostings[trigram] = make(map[string]struct{})
			}
			newPostings[trigram][key] = struct{}{}
		}
		for _, symbol := range item.symbols {
			name := strings.ToLower(symbol.Name)
			newSymbols[name] = append(newSymbols[name], symbol)
		}
	}
	idx.documents = newDocuments
	idx.postings = newPostings
	idx.symbols = newSymbols
	idx.generation++
	idx.root = absoluteRoot
	idx.pathRevision = make(map[string]uint64, len(newDocuments)+len(idx.overlays))
	for key := range newDocuments {
		idx.pathRevision[key] = 1
	}
	for key := range idx.overlays {
		if idx.documents[key] != nil {
			idx.pathRevision[key] = 2
		}
	}
	return nil
}

func relativeDepth(root, path string) int {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." {
		return 0
	}
	return strings.Count(relative, string(filepath.Separator)) + 1
}

// Update parses and atomically indexes a document snapshot. Versions must
// increase for an existing path, preventing slower parses from publishing
// stale editor contents.
func (idx *Index) Update(ctx context.Context, path, uri, text string, version int64) error {
	return idx.update(ctx, path, uri, text, version, true)
}

// UpdateSaved indexes a document snapshot that is known to match the file on
// disk. Subsequent workspace builds may refresh it from disk.
func (idx *Index) UpdateSaved(ctx context.Context, path, uri, text string, version int64) error {
	return idx.update(ctx, path, uri, text, version, false)
}

func (idx *Index) update(ctx context.Context, path, uri, text string, version int64, dirty bool) error {
	if idx == nil {
		return errors.New("index: nil index")
	}
	if ctx == nil {
		return errors.New("index: update context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	originalPath := path
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("index: resolve document path: %w", err)
	}
	if !strings.EqualFold(filepath.Ext(originalPath), ".jl") {
		return errors.New("index: editor snapshots are only tracked for Julia files")
	}
	idx.mu.RLock()
	workspaceRoot := idx.root
	idx.mu.RUnlock()
	if workspaceRoot != "" && !isWithinRoot(workspaceRoot, absolute) {
		return errors.New("index: document path is outside the indexed workspace")
	}
	if workspaceRoot != "" {
		resolved, evalErr := filepath.EvalSymlinks(absolute)
		if errors.Is(evalErr, fs.ErrNotExist) {
			return fmt.Errorf("index: resolve document symlink: %w", evalErr)
		}
		if evalErr == nil && !isWithinRoot(workspaceRoot, resolved) {
			return errors.New("index: document symlink resolves outside the indexed workspace")
		}
	}
	if info, statErr := os.Stat(absolute); statErr == nil {
		if !info.Mode().IsRegular() {
			return errors.New("index: document path is not a regular file")
		}
		if info.Size() > defaultMaxFileSize {
			return errors.New("index: document exceeds the maximum indexed file size")
		}
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("index: stat document %q: %w", absolute, statErr)
	}
	if strings.TrimSpace(uri) != "" {
		parsedURI, parseErr := url.Parse(uri)
		if parseErr != nil || parsedURI.Scheme != "file" {
			return errors.New("index: document URI must be a file URI")
		}
		if runtime.GOOS != "windows" && parsedURI.Host != "" && !strings.EqualFold(parsedURI.Host, "localhost") {
			return errors.New("index: document URI host must be local")
		}
	}
	if !utf8.ValidString(text) {
		return errors.New("index: document is not valid UTF-8 text")
	}
	if version < 0 {
		return errors.New("index: document version is negative")
	}
	key := pathKey(absolute)
	idx.mu.RLock()
	if idx.root != "" && pathKey(idx.root) != pathKey(workspaceRoot) {
		idx.mu.RUnlock()
		return errors.New("index: workspace root changed during update")
	}
	current := idx.documents[key]
	if current != nil && version <= 0 {
		idx.mu.RUnlock()
		return errors.New("index: update version must be positive")
	}
	startingRevision := idx.pathRevision[key]
	if current != nil && version < current.version {
		idx.mu.RUnlock()
		return ErrStaleUpdate
	}
	idx.mu.RUnlock()
	var parsed *syntax.Document
	if strings.EqualFold(filepath.Ext(absolute), ".jl") {
		parsed, err = syntax.Parse(text)
		if err != nil {
			return fmt.Errorf("index: parse %q: %w", absolute, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if uri == "" {
		uri = URIForPath(absolute)
	}
	originalAbsolute, absErr := filepath.Abs(originalPath)
	if absErr != nil {
		return fmt.Errorf("index: resolve original document path: %w", absErr)
	}
	uriPath := uriKey(uri)
	if uriPath != pathKey(originalAbsolute) && uriPath != pathKey(absolute) {
		return errors.New("index: document URI path does not match its file path")
	}
	item := &document{path: absolute, uri: uri, text: text, version: version, parsed: parsed, trigrams: makeTrigrams(text)}
	item.symbols = documentSymbols(item)
	if err := ctx.Err(); err != nil {
		return err
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.pathRevision[key] != startingRevision {
		return ErrStaleUpdate
	}
	if current := idx.documents[key]; current != nil && version < current.version {
		return ErrStaleUpdate
	}
	if current := idx.documents[key]; current != nil && version <= current.version {
		if version == current.version && text == current.text {
			return nil
		}
		return ErrStaleUpdate
	}
	idx.removeLocked(key)
	idx.documents[key] = item
	for trigram := range item.trigrams {
		if idx.postings[trigram] == nil {
			idx.postings[trigram] = make(map[string]struct{})
		}
		idx.postings[trigram][key] = struct{}{}
	}
	for _, symbol := range item.symbols {
		name := strings.ToLower(symbol.Name)
		idx.symbols[name] = append(idx.symbols[name], symbol)
	}
	if dirty {
		idx.overlays[key] = true
	} else {
		delete(idx.overlays, key)
	}
	idx.generation++
	idx.pathRevision[key]++
	return nil
}

func isWithinRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Remove removes a document and all of its search and symbol postings. It is
// safe to call for a path that is not currently indexed.
func (idx *Index) Remove(path string) error {
	if idx == nil {
		return errors.New("index: nil index")
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("index: document path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("index: resolve document path: %w", err)
	}
	if !strings.EqualFold(filepath.Ext(absolute), ".jl") {
		return errors.New("index: only Julia source paths can be removed from the symbol index")
	}
	idx.mu.RLock()
	workspaceRoot := idx.root
	idx.mu.RUnlock()
	if workspaceRoot != "" && !isWithinRoot(workspaceRoot, absolute) {
		return errors.New("index: document path is outside the indexed workspace")
	}
	if resolved, evalErr := filepath.EvalSymlinks(absolute); evalErr == nil {
		absolute = resolved
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	key := pathKey(absolute)
	idx.removeLocked(key)
	delete(idx.overlays, key)
	idx.generation++
	idx.pathRevision[key]++
	return nil
}

func (idx *Index) removeLocked(key string) {
	item := idx.documents[key]
	if item == nil {
		return
	}
	delete(idx.documents, key)
	for trigram := range item.trigrams {
		delete(idx.postings[trigram], key)
		if len(idx.postings[trigram]) == 0 {
			delete(idx.postings, trigram)
		}
	}
	for _, symbol := range item.symbols {
		name := strings.ToLower(symbol.Name)
		remaining := make([]jide.WorkspaceSymbol, 0, len(idx.symbols[name]))
		for _, existing := range idx.symbols[name] {
			if existing.Location.URI != item.uri {
				remaining = append(remaining, existing)
			}
		}
		if len(remaining) == 0 {
			delete(idx.symbols, name)
		} else {
			idx.symbols[name] = remaining
		}
	}
}

// Search returns case-sensitive literal substring matches in stable URI,
// line, and column order. Trigrams narrow candidates; strings.Contains checks
// each candidate so the index never returns a false positive.
func (idx *Index) Search(ctx context.Context, query string, limit int) (jide.TextSearchResult, error) {
	result := jide.TextSearchResult{Matches: []jide.TextSearchMatch{}}
	if idx == nil {
		return result, errors.New("index: nil index")
	}
	if !utf8.ValidString(query) {
		return result, errors.New("index: search query is not valid UTF-8")
	}
	if ctx == nil {
		return result, errors.New("index: search context is nil")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	idx.mu.RLock()
	candidates := idx.searchCandidatesLocked(query)
	type searchDocument struct {
		uri  string
		text string
	}
	documents := make([]searchDocument, 0, len(candidates))
	for key := range candidates {
		if item := idx.documents[key]; item != nil {
			documents = append(documents, searchDocument{uri: item.uri, text: item.text})
		}
	}
	idx.mu.RUnlock()
	sort.Slice(documents, func(i, j int) bool { return documents[i].uri < documents[j].uri })
	if len(query) < 3 {
		filtered := documents[:0]
		for _, item := range documents {
			if strings.Contains(item.text, query) {
				filtered = append(filtered, item)
			}
		}
		documents = filtered
	}
	for _, item := range documents {
		if err := ctx.Err(); err != nil {
			return jide.TextSearchResult{}, err
		}
		for lineNumber, line := range strings.Split(item.text, "\n") {
			searchFrom := 0
			for {
				if err := ctx.Err(); err != nil {
					return jide.TextSearchResult{}, err
				}
				if searchFrom > len(line) {
					break
				}
				found := strings.Index(line[searchFrom:], query)
				if found < 0 {
					break
				}
				column := searchFrom + found
				if limit > 0 && len(result.Matches) == limit {
					result.Truncated = true
					return result, nil
				}
				result.Matches = append(result.Matches, jide.TextSearchMatch{
					URI: item.uri, Line: lineNumber + 1, Column: utf8.RuneCountInString(line[:column]) + 1, Preview: strings.TrimSpace(line),
				})
				searchFrom = column + 1
			}
		}
	}
	return result, nil
}

// SearchSymbols returns workspace declarations whose names contain query,
// case-insensitively, in stable name and URI order.
func (idx *Index) SearchSymbols(ctx context.Context, query string, limit int) (jide.WorkspaceSymbolsResult, error) {
	result := jide.WorkspaceSymbolsResult{Symbols: []jide.WorkspaceSymbol{}}
	if idx == nil {
		return result, errors.New("index: nil index")
	}
	if !utf8.ValidString(query) {
		return result, errors.New("index: symbol query is not valid UTF-8")
	}
	if ctx == nil {
		return result, errors.New("index: symbol-search context is nil")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	needle := strings.ToLower(query)
	idx.mu.RLock()
	names := make([]string, 0, len(idx.symbols))
	for name := range idx.symbols {
		if strings.Contains(name, needle) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	symbolsByName := make(map[string][]jide.WorkspaceSymbol, len(names))
	for _, name := range names {
		symbolsByName[name] = append([]jide.WorkspaceSymbol(nil), idx.symbols[name]...)
	}
	idx.mu.RUnlock()
	for _, name := range names {
		for _, symbol := range symbolsByName[name] {
			if err := ctx.Err(); err != nil {
				return jide.WorkspaceSymbolsResult{}, err
			}
			if limit > 0 && len(result.Symbols) == limit {
				result.Truncated = true
				return result, nil
			}
			result.Symbols = append(result.Symbols, symbol)
		}
	}
	sort.Slice(result.Symbols, func(i, j int) bool {
		if result.Symbols[i].Name == result.Symbols[j].Name {
			return result.Symbols[i].Location.URI < result.Symbols[j].Location.URI
		}
		return result.Symbols[i].Name < result.Symbols[j].Name
	})
	return result, nil
}

// DefinitionAt resolves the identifier at a UTF-16 LSP position using the
// current indexed snapshots. All declarations with that exact name are
// returned, with same-file declarations first.
func (idx *Index) DefinitionAt(ctx context.Context, uri string, line, character int) ([]jide.LSPLocation, error) {
	if idx == nil {
		return nil, errors.New("index: nil index")
	}
	if ctx == nil {
		return nil, errors.New("index: definition context is nil")
	}
	if line < 0 || character < 0 {
		return nil, errors.New("index: source position cannot be negative")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	idx.mu.RLock()
	var source *document
	for _, item := range idx.documents {
		if uriKey(item.uri) == uriKey(uri) || strings.EqualFold(item.path, uri) {
			source = item
			break
		}
	}
	if source == nil {
		idx.mu.RUnlock()
		return nil, nil
	}
	name, ok := identifierAt(source.text, line, character)
	if !ok {
		idx.mu.RUnlock()
		return nil, nil
	}
	declarations := append([]jide.WorkspaceSymbol(nil), idx.symbols[strings.ToLower(name)]...)
	sourceURI := source.uri
	idx.mu.RUnlock()
	if len(declarations) == 0 {
		return nil, nil
	}
	sort.Slice(declarations, func(i, j int) bool {
		if (declarations[i].Location.URI == sourceURI) != (declarations[j].Location.URI == sourceURI) {
			return declarations[i].Location.URI == sourceURI
		}
		return declarations[i].Location.URI < declarations[j].Location.URI
	})
	locations := make([]jide.LSPLocation, 0, len(declarations))
	for _, declaration := range declarations {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		locations = append(locations, declaration.Location)
	}
	return locations, nil
}

// FileCount reports how many text files are currently indexed.
func (idx *Index) FileCount() int {
	if idx == nil {
		return 0
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.documents)
}

// Root returns the resolved workspace root associated with the last successful
// Build, or an empty string before the first build.
func (idx *Index) Root() string {
	if idx == nil {
		return ""
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.root
}

// Generation returns a monotonically increasing revision for each successful
// Build, Update or Remove, allowing callers to detect stale snapshots.
func (idx *Index) Generation() uint64 {
	if idx == nil {
		return 0
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.generation
}

// IsCurrent reports whether generation still names the current indexed state.
func (idx *Index) IsCurrent(generation uint64) bool {
	return idx != nil && idx.Generation() == generation
}

// Has reports whether path currently has a document snapshot in the index.
func (idx *Index) Has(path string) bool {
	if idx == nil {
		return false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.documents[pathKey(absolute)] != nil
}

func (idx *Index) searchCandidatesLocked(query string) map[string]struct{} {
	grams := makeTrigrams(query)
	if len(grams) == 0 {
		all := make(map[string]struct{}, len(idx.documents))
		for key := range idx.documents {
			all[key] = struct{}{}
		}
		return all
	}
	ordered := make([]string, 0, len(grams))
	for gram := range grams {
		ordered = append(ordered, gram)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := len(idx.postings[ordered[i]]), len(idx.postings[ordered[j]])
		if left == right {
			return ordered[i] < ordered[j]
		}
		return left < right
	})
	var candidates map[string]struct{}
	for _, gram := range ordered {
		posting := idx.postings[gram]
		if len(posting) == 0 {
			return map[string]struct{}{}
		}
		if candidates == nil {
			candidates = make(map[string]struct{}, len(posting))
			for key := range posting {
				candidates[key] = struct{}{}
			}
			continue
		}
		for key := range candidates {
			if _, present := posting[key]; !present {
				delete(candidates, key)
			}
		}
	}
	return candidates
}

func readDocument(path string, maxSize int64) (*document, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("index: stat %q: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxSize {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("index: read %q: %w", path, err)
	}
	if int64(len(data)) > maxSize {
		return nil, nil
	}
	return &document{path: path, text: string(data)}, nil
}

func makeTrigrams(text string) map[string]struct{} {
	grams := make(map[string]struct{})
	for i := 0; i+3 <= len(text); i++ {
		grams[text[i:i+3]] = struct{}{}
	}
	return grams
}

func documentSymbols(item *document) []jide.WorkspaceSymbol {
	var symbols []jide.WorkspaceSymbol
	if item.parsed == nil {
		return symbols
	}
	var visit func([]syntax.DocumentSymbol)
	visit = func(children []syntax.DocumentSymbol) {
		for _, symbol := range children {
			if symbol.Kind != syntax.SymbolField {
				symbols = append(symbols, jide.WorkspaceSymbol{
					Name: symbol.Name, Kind: jide.DocumentSymbolKind(symbol.Kind),
					Location: jide.LSPLocation{URI: item.uri, Range: byteRangeToLSP(item.text, symbol.SelectionRange)},
				})
			}
			visit(symbol.Children)
		}
	}
	visit(item.parsed.Symbols())
	return symbols
}

func identifierAt(text string, line, character int) (string, bool) {
	if line < 0 || character < 0 {
		return "", false
	}
	lineStart := 0
	for current := 0; current < line; current++ {
		next := strings.IndexByte(text[lineStart:], '\n')
		if next < 0 {
			return "", false
		}
		lineStart += next + 1
	}
	lineEnd := len(text)
	if next := strings.IndexByte(text[lineStart:], '\n'); next >= 0 {
		lineEnd = lineStart + next
	}
	lineText := text[lineStart:lineEnd]
	units, offset := 0, 0
	for offset < len(lineText) && units < character {
		r, size := decodeRune(lineText[offset:])
		units += len(utf16.Encode([]rune{r}))
		offset += size
	}
	if units < character {
		return "", false
	}
	if offset >= len(lineText) || !isIdentifierRune(runeAt(lineText[offset:])) {
		if offset == 0 {
			return "", false
		}
		previous := previousRuneStart(lineText, offset)
		if !isIdentifierRune(runeAt(lineText[previous:])) {
			return "", false
		}
		offset = previous
	}
	start, end := offset, offset
	for start > 0 {
		previous := previousRuneStart(lineText, start)
		if !isIdentifierRune(runeAt(lineText[previous:])) {
			break
		}
		start = previous
	}
	for end < len(lineText) {
		r, size := decodeRune(lineText[end:])
		if !isIdentifierRune(r) {
			break
		}
		end += size
	}
	if start == end || !isIdentifierRune(runeAt(lineText[start:])) {
		return "", false
	}
	return lineText[start:end], true
}

func byteRangeToLSP(text string, source syntax.Range) jide.LSPRange {
	return jide.LSPRange{Start: bytePositionToLSP(text, source.StartByte), End: bytePositionToLSP(text, source.EndByte)}
}

func bytePositionToLSP(text string, offset int) jide.LSPPosition {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	line := strings.Count(text[:offset], "\n")
	lineStart := strings.LastIndex(text[:offset], "\n") + 1
	units := len(utf16.Encode([]rune(text[lineStart:offset])))
	return jide.LSPPosition{Line: line, Character: units}
}

func pathKey(path string) string {
	cleaned := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

func normalizeDirectoryName(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(name)
	}
	return name
}

func uriKey(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "file" || parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		return value
	}
	uriPath := parsed.Path
	if runtime.GOOS == "windows" && len(uriPath) >= 3 && uriPath[0] == '/' && uriPath[2] == ':' {
		uriPath = uriPath[1:]
	}
	path := filepath.FromSlash(uriPath)
	if runtime.GOOS == "windows" && parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		path = `\\` + parsed.Host + filepath.FromSlash(parsed.Path)
	}
	if runtime.GOOS != "windows" && parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		return value
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

// URIForPath returns a percent-escaped file URI for a local path.
func URIForPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	path = filepath.ToSlash(path)
	if runtime.GOOS == "windows" && len(path) >= 2 && path[1] == ':' {
		path = strings.ToLower(path[:1]) + path[1:]
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func decodeRune(text string) (rune, int) {
	return utf8.DecodeRuneInString(text)
}

func runeAt(text string) rune {
	r, _ := decodeRune(text)
	return r
}

func isRuneStart(text string, offset int) bool {
	return offset >= 0 && offset < len(text) && text[offset]&0xc0 != 0x80
}

func previousRuneStart(text string, offset int) int {
	if offset <= 0 {
		return 0
	}
	offset--
	for offset > 0 && !isRuneStart(text, offset) {
		offset--
	}
	return offset
}

func isIdentifierRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}
