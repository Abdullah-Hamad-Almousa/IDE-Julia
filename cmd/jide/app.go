package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jide-dev/jide/internal/lsp"
	"github.com/jide-dev/jide/internal/syntax"
	"github.com/jide-dev/jide/internal/vfs"
	"github.com/jide-dev/jide/internal/workspace"
	jide "github.com/jide-dev/jide/proto/jide"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const maxEditorFileSize = 8 << 20

var (
	// ErrWorkspaceNotOpen is returned when a file operation needs an open workspace.
	ErrWorkspaceNotOpen = errors.New("jide: no workspace is open")
	// ErrFileOutsideWorkspace is returned when a file resolves outside the active workspace.
	ErrFileOutsideWorkspace = errors.New("jide: file is outside the workspace")
	// ErrBinaryFile is returned when an editor operation targets non-UTF-8 data.
	ErrBinaryFile = errors.New("jide: file is not valid UTF-8 text")
	// ErrFileTooLarge is returned when an editor operation exceeds the UI limit.
	ErrFileTooLarge = errors.New("jide: file exceeds the 8 MiB editor limit")
	// ErrDirtyDocument is returned when a dirty document is closed without discarding it.
	ErrDirtyDocument = errors.New("jide: document has unsaved changes")
)

// App is the Wails binding between the desktop frontend and the workspace,
// virtual-file, and syntax services. It is a temporary in-process adapter
// until cmd/jided serves these operations over the protocol.
type App struct {
	mu           sync.Mutex
	ctx          context.Context
	ctxCancel    context.CancelFunc
	active       *workspaceSession
	lsp          *lsp.Client
	diagnostics  map[string][]jide.LSPDiagnostic
	lspDocuments map[string]openLSPDocument
}

type openLSPDocument struct {
	text    string
	version int
}

type workspaceSession struct {
	workspace         *workspace.Workspace
	store             *vfs.Store
	activeEnvironment string
}

// WorkspaceView is the current workspace information rendered by the UI.
type WorkspaceView struct {
	Root               string               `json:"root"`
	Name               string               `json:"name"`
	ProjectPath        string               `json:"projectPath,omitempty"`
	JuliaCompat        string               `json:"juliaCompat,omitempty"`
	EnvironmentWarning string               `json:"environmentWarning,omitempty"`
	Entries            []WorkspaceEntryView `json:"entries"`
	Environments       []EnvironmentView    `json:"environments"`
	ActiveEnvironment  string               `json:"activeEnvironment"`
}

// WorkspaceEntryView is a file or directory in the workspace navigator.
type WorkspaceEntryView struct {
	Path         string `json:"path"`
	AbsolutePath string `json:"absolutePath"`
	IsDirectory  bool   `json:"isDirectory"`
	Symlink      bool   `json:"symlink,omitempty"`
	OutsideRoot  bool   `json:"outsideRoot,omitempty"`
	Alias        bool   `json:"alias,omitempty"`
}

// EnvironmentView is a Julia environment that can be selected in the status bar.
type EnvironmentView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Details string `json:"details,omitempty"`
}

// DocumentView is an open text document and its current Tree-sitter result.
type DocumentView struct {
	Path       string                      `json:"path"`
	Name       string                      `json:"name"`
	Text       string                      `json:"text"`
	Version    int64                       `json:"version"`
	Dirty      bool                        `json:"dirty"`
	Highlights jide.SyntaxHighlightsResult `json:"highlights"`
}

// NewApp constructs the UI bridge. The returned app is ready for headless
// workspace and document tests before Wails supplies its runtime context.
func NewApp() *App {
	return &App{diagnostics: make(map[string][]jide.LSPDiagnostic), lspDocuments: make(map[string]openLSPDocument)}
}

// startup stores the Wails context used by native dialogs.
func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
}

// shutdown stops the workspace language server before the desktop exits.
func (a *App) shutdown(_ context.Context) {
	a.mu.Lock()
	manager := a.lsp
	cancel := a.ctxCancel
	a.lsp = nil
	a.ctxCancel = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if manager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = manager.Stop(ctx)
	}
}

// OpenWorkspaceDialog asks the operating system for a project folder.
// Canceling the dialog leaves the current workspace unchanged.
func (a *App) OpenWorkspaceDialog() (*WorkspaceView, error) {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return nil, errors.New("jide: desktop runtime is not ready")
	}
	selected, err := wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{
		Title:                "Open Julia workspace",
		CanCreateDirectories: false,
	})
	if err != nil {
		return nil, fmt.Errorf("jide: choose workspace: %w", err)
	}
	if selected == "" {
		return nil, nil
	}
	return a.OpenWorkspace(selected)
}

// OpenWorkspace opens a project folder and returns its file tree and detected
// Julia environment. The file tree is limited to useful source depth and
// excludes common generated/dependency directories.
func (a *App) OpenWorkspace(root string) (*WorkspaceView, error) {
	opened, err := workspace.Open(root)
	if err != nil {
		return nil, fmt.Errorf("jide: open workspace: %w", err)
	}
	session := &workspaceSession{
		workspace: opened,
		store:     vfs.NewStore(),
	}
	project, projectErr := opened.Project()
	if projectErr == nil && project != nil {
		session.activeEnvironment = "project"
	} else {
		session.activeEnvironment = "default"
	}
	tree, err := opened.Tree(workspace.ScanOptions{
		MaxDepth: 12,
		Exclude:  []string{".git", ".julia", ".vscode", "node_modules", "vendor", "build", "dist"},
	})
	if err != nil {
		return nil, fmt.Errorf("jide: scan workspace: %w", err)
	}

	a.mu.Lock()
	oldLSP := a.lsp
	oldCancel := a.ctxCancel
	a.lsp = nil
	a.ctxCancel = nil
	a.diagnostics = make(map[string][]jide.LSPDiagnostic)
	a.lspDocuments = make(map[string]openLSPDocument)
	a.active = session
	view := makeWorkspaceView(session, tree)
	desktopContext := a.ctx
	a.mu.Unlock()
	if oldLSP != nil {
		if oldCancel != nil {
			oldCancel()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = oldLSP.Stop(ctx)
		cancel()
	}
	if opened.ProjectPath() != "" && desktopContext != nil {
		lspContext, cancelLSP := context.WithCancel(context.Background())
		var client *lsp.Client
		client = lsp.New(lsp.Options{
			ProjectPath: opened.ProjectPath(), RootURI: lsp.FileURI(root),
			OnDiagnostics: func(uri string, diagnostics []jide.LSPDiagnostic) {
				params := jide.LSPDiagnosticsParams{URI: uri, Diagnostics: diagnostics}
				a.mu.Lock()
				a.diagnostics[uri] = append([]jide.LSPDiagnostic(nil), diagnostics...)
				ctx := a.ctx
				a.mu.Unlock()
				if ctx != nil {
					wailsruntime.EventsEmit(ctx, "lsp:diagnostics", params)
				}
			},
			OnStateChange: func(state lsp.State) {
				a.mu.Lock()
				ctx := a.ctx
				a.mu.Unlock()
				if ctx != nil {
					wailsruntime.EventsEmit(ctx, "lsp:status", string(state))
				}
				if state != lsp.StateRunning || client.Status().RestartAttempts != 0 {
					return
				}
				go func() {
					a.mu.Lock()
					defer a.mu.Unlock()
					if a.lsp != client {
						return
					}
					for uri, document := range a.lspDocuments {
						_ = client.DidOpen(context.Background(), uri, "julia", document.text, document.version)
					}
				}()
			},
		})
		a.mu.Lock()
		a.lsp = client
		a.ctxCancel = cancelLSP
		a.mu.Unlock()
		go func() {
			if err := client.Start(lspContext); err != nil {
				a.mu.Lock()
				ctx := a.ctx
				a.mu.Unlock()
				if ctx != nil {
					wailsruntime.EventsEmit(ctx, "lsp:error", err.Error())
				}
				return
			}
		}()
	}
	return &view, nil
}

// GetLSPStatusView returns language-server state to the frontend.
func (a *App) GetLSPStatusView() map[string]any {
	manager := a.languageServer()
	if manager == nil {
		return map[string]any{"state": "stopped", "message": "No active Julia workspace"}
	}
	status := manager.Status()
	view := map[string]any{"state": status.State}
	if status.JuliaVersion != "" {
		view["julia"] = status.JuliaVersion
	}
	if status.LastError != "" {
		view["message"] = status.LastError
	}
	return view
}

// LSPHover requests hover information from LanguageServer.jl.
func (a *App) LSPHover(uri string, line, character int) (*jide.LSPHoverResult, error) {
	manager := a.languageServer()
	if manager == nil {
		return nil, errors.New("jide: language server is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hover, err := manager.Hover(ctx, uri, line, character)
	if err != nil {
		return nil, err
	}
	return &jide.LSPHoverResult{URI: uri, Hover: hover}, nil
}

// LSPCompletion requests completion candidates from LanguageServer.jl.
func (a *App) LSPCompletion(uri string, line, character int) (jide.LSPCompletionResult, error) {
	manager := a.languageServer()
	if manager == nil {
		return jide.LSPCompletionResult{}, errors.New("jide: language server is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := manager.Completion(ctx, uri, line, character)
	result.URI = uri
	return result, err
}

// LSPReferences requests source references from LanguageServer.jl.
func (a *App) LSPReferences(uri string, line, character int, includeDeclaration bool) (jide.LSPReferencesResult, error) {
	manager := a.languageServer()
	if manager == nil {
		return jide.LSPReferencesResult{}, errors.New("jide: language server is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	locations, err := manager.References(ctx, uri, line, character, includeDeclaration)
	return jide.LSPReferencesResult{URI: uri, Locations: locations}, err
}

// LSPDefinition asks LanguageServer.jl for symbol definitions.
func (a *App) LSPDefinition(uri string, line, character int) (jide.LSPDefinitionResult, error) {
	manager := a.languageServer()
	if manager == nil {
		return jide.LSPDefinitionResult{}, errors.New("jide: language server is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	locations, err := manager.Definition(ctx, uri, line, character)
	return jide.LSPDefinitionResult{URI: uri, Locations: locations}, err
}

func (a *App) languageServer() *lsp.Client { a.mu.Lock(); defer a.mu.Unlock(); return a.lsp }

// GetWorkspace returns the current workspace view, or nil when none is open.
func (a *App) GetWorkspace() (*WorkspaceView, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active == nil {
		return nil, nil
	}
	tree, err := scanSession(a.active)
	if err != nil {
		return nil, err
	}
	view := makeWorkspaceView(a.active, tree)
	return &view, nil
}

// RefreshWorkspace rescans the active workspace's file tree.
func (a *App) RefreshWorkspace() (*WorkspaceView, error) {
	return a.GetWorkspace()
}

// SelectEnvironment selects a workspace or default Julia environment in the
// status bar. Runtime discovery and execution are supplied by WP4.
func (a *App) SelectEnvironment(id string) (*WorkspaceView, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active == nil {
		return nil, ErrWorkspaceNotOpen
	}
	view, err := makeWorkspaceViewFromSession(a.active)
	if err != nil {
		return nil, err
	}
	for _, environment := range view.Environments {
		if environment.ID == id {
			a.active.activeEnvironment = id
			view.ActiveEnvironment = id
			return &view, nil
		}
	}
	return nil, fmt.Errorf("jide: unknown Julia environment %q", id)
}

// OpenFile opens a workspace text file in the virtual file store.
func (a *App) OpenFile(path string) (*DocumentView, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	session, err := a.currentSession()
	if err != nil {
		return nil, err
	}
	absolute, err := session.filePath(path)
	if err != nil {
		return nil, err
	}
	file, err := session.store.Open(absolute)
	if err != nil {
		return nil, fmt.Errorf("jide: open file: %w", err)
	}
	snapshot, err := file.Read()
	if err != nil {
		return nil, fmt.Errorf("jide: read open file: %w", err)
	}
	view, err := documentView(snapshot)
	if err == nil && a.lsp != nil && strings.EqualFold(filepath.Ext(view.Path), ".jl") {
		uri := pathToURI(view.Path)
		a.lspDocuments[uri] = openLSPDocument{text: view.Text, version: int(view.Version)}
		if a.lsp.State() == lsp.StateRunning {
			_ = a.lsp.DidOpen(context.Background(), uri, "julia", view.Text, int(view.Version))
		}
	}
	return view, err
}

// UpdateFile replaces the in-memory content of an open document and refreshes
// its Julia syntax result. The file is not written to disk until SaveFile.
func (a *App) UpdateFile(path, text string) (*DocumentView, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	session, err := a.currentSession()
	if err != nil {
		return nil, err
	}
	absolute, err := session.filePath(path)
	if err != nil {
		return nil, err
	}
	if len(text) > maxEditorFileSize {
		return nil, ErrFileTooLarge
	}
	if !utf8.ValidString(text) {
		return nil, ErrBinaryFile
	}
	file, err := session.store.Open(absolute)
	if err != nil {
		return nil, fmt.Errorf("jide: open file for editing: %w", err)
	}
	previous, err := file.Read()
	if err != nil {
		return nil, fmt.Errorf("jide: read file for editing: %w", err)
	}
	if previous.String() != text {
		if _, err := file.Write([]byte(text)); err != nil {
			return nil, fmt.Errorf("jide: update file: %w", err)
		}
	}
	snapshot, err := file.Read()
	if err != nil {
		return nil, fmt.Errorf("jide: read updated file: %w", err)
	}
	view, err := documentView(snapshot)
	if err == nil && a.lsp != nil {
		uri := pathToURI(view.Path)
		if _, open := a.lspDocuments[uri]; open {
			a.lspDocuments[uri] = openLSPDocument{text: view.Text, version: int(view.Version)}
			if a.lsp.State() == lsp.StateRunning {
				_ = a.lsp.DidChange(context.Background(), uri, int(view.Version), []jide.LSPTextChange{{Text: view.Text}})
			}
		}
	}
	return view, err
}

// SaveFile updates and saves a document to disk. The text argument is the
// latest editor buffer, so a save cannot race a pending debounced update.
func (a *App) SaveFile(path, text string) (*DocumentView, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	session, err := a.currentSession()
	if err != nil {
		return nil, err
	}
	absolute, err := session.filePath(path)
	if err != nil {
		return nil, err
	}
	if len(text) > maxEditorFileSize {
		return nil, ErrFileTooLarge
	}
	if !utf8.ValidString(text) {
		return nil, ErrBinaryFile
	}
	file, err := session.store.Open(absolute)
	if err != nil {
		return nil, fmt.Errorf("jide: open file for saving: %w", err)
	}
	previous, err := file.Read()
	if err != nil {
		return nil, fmt.Errorf("jide: read file for saving: %w", err)
	}
	if previous.String() != text {
		if _, err := file.Write([]byte(text)); err != nil {
			return nil, fmt.Errorf("jide: update file before save: %w", err)
		}
	}
	snapshot, err := file.Save()
	if err != nil {
		return nil, fmt.Errorf("jide: save file: %w", err)
	}
	view, err := documentView(snapshot)
	if err == nil && a.lsp != nil {
		documentURI := pathToURI(view.Path)
		if _, open := a.lspDocuments[documentURI]; open {
			a.lspDocuments[documentURI] = openLSPDocument{text: view.Text, version: int(view.Version)}
			if a.lsp.State() == lsp.StateRunning {
				_ = a.lsp.DidChange(context.Background(), documentURI, int(view.Version), []jide.LSPTextChange{{Text: view.Text}})
				_ = a.lsp.DidSave(context.Background(), documentURI, view.Text)
			}
		}
	}
	return view, err
}

// CloseFile drops a document from the virtual file store. A dirty buffer must
// be saved or explicitly discarded first.
func (a *App) CloseFile(path string, discard bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	session, err := a.currentSession()
	if err != nil {
		return err
	}
	absolute, err := session.filePath(path)
	if err != nil {
		return err
	}
	if snapshot, ok := session.store.Snapshot(absolute); ok && snapshot.Dirty() && !discard {
		return ErrDirtyDocument
	}
	if _, ok := session.store.Snapshot(absolute); !ok {
		return nil
	}
	if a.lsp != nil {
		_ = a.lsp.DidClose(context.Background(), pathToURI(absolute))
	}
	delete(a.lspDocuments, pathToURI(absolute))
	return session.store.Close(absolute)
}

// GetSyntaxHighlights parses current editor text for Monaco's token bridge.
func (a *App) GetSyntaxHighlights(uri, text string) (jide.SyntaxHighlightsResult, error) {
	if len(text) > maxEditorFileSize {
		return jide.SyntaxHighlightsResult{}, ErrFileTooLarge
	}
	if !utf8.ValidString(text) {
		return jide.SyntaxHighlightsResult{}, ErrBinaryFile
	}
	document, err := syntax.Parse(text)
	if err != nil {
		return jide.SyntaxHighlightsResult{}, fmt.Errorf("jide: parse Julia buffer: %w", err)
	}
	return document.ProtocolHighlights(uri), nil
}

func (a *App) currentSession() (*workspaceSession, error) {
	if a.active == nil {
		return nil, ErrWorkspaceNotOpen
	}
	return a.active, nil
}

func (s *workspaceSession) filePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("jide: file path is empty")
	}
	absolute := path
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(s.workspace.Root(), absolute)
	}
	absolute, err := filepath.Abs(absolute)
	if err != nil {
		return "", fmt.Errorf("jide: resolve file path: %w", err)
	}
	absolute = filepath.Clean(absolute)
	if !pathWithin(s.workspace.Root(), absolute) {
		return "", ErrFileOutsideWorkspace
	}
	realPath, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("jide: resolve file link: %w", err)
	}
	if !pathWithin(s.workspace.RealRoot(), realPath) {
		return "", ErrFileOutsideWorkspace
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("jide: stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("jide: selected path is not a regular file")
	}
	if info.Size() > maxEditorFileSize {
		return "", ErrFileTooLarge
	}
	return absolute, nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	if relative == "." {
		return true
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func pathToURI(path string) string {
	return lsp.FileURI(path)
}

func scanSession(session *workspaceSession) (*workspace.Tree, error) {
	tree, err := session.workspace.Tree(workspace.ScanOptions{
		MaxDepth: 12,
		Exclude:  []string{".git", ".julia", ".vscode", "node_modules", "vendor", "build", "dist"},
	})
	if err != nil {
		return nil, fmt.Errorf("jide: scan workspace: %w", err)
	}
	return tree, nil
}

func makeWorkspaceViewFromSession(session *workspaceSession) (WorkspaceView, error) {
	tree, err := scanSession(session)
	if err != nil {
		return WorkspaceView{}, err
	}
	return makeWorkspaceView(session, tree), nil
}

func makeWorkspaceView(session *workspaceSession, tree *workspace.Tree) WorkspaceView {
	view := WorkspaceView{
		Root:              session.workspace.Root(),
		Name:              filepath.Base(session.workspace.Root()),
		ProjectPath:       session.workspace.ProjectPath(),
		ActiveEnvironment: session.activeEnvironment,
		Entries:           make([]WorkspaceEntryView, 0, len(tree.Entries)),
		Environments: []EnvironmentView{{
			ID:      "default",
			Name:    "Julia stable",
			Details: "Selected from the installed juliaup runtime, when available",
		}},
	}
	for _, entry := range tree.Entries {
		view.Entries = append(view.Entries, WorkspaceEntryView{
			Path:         entry.Path,
			AbsolutePath: entry.AbsolutePath,
			IsDirectory:  entry.IsDir,
			Symlink:      entry.Symlink,
			OutsideRoot:  entry.OutsideRoot,
			Alias:        entry.Alias,
		})
	}
	project, err := session.workspace.Project()
	if err != nil {
		view.EnvironmentWarning = err.Error()
	}
	if project != nil {
		view.JuliaCompat = project.JuliaCompat()
		name := project.Name
		if name == "" {
			name = "Workspace project"
		}
		details := project.Path
		if project.JuliaCompat() != "" {
			details += " · Julia " + project.JuliaCompat()
		}
		view.Environments = append(view.Environments, EnvironmentView{
			ID:      "project",
			Name:    name,
			Path:    project.Path,
			Details: details,
		})
	}
	if session.activeEnvironment == "project" && project == nil {
		view.ActiveEnvironment = "default"
	}
	return view
}

func documentView(snapshot vfs.Snapshot) (*DocumentView, error) {
	if len(snapshot.Bytes()) > maxEditorFileSize {
		return nil, ErrFileTooLarge
	}
	text := snapshot.String()
	if !utf8.ValidString(text) {
		return nil, ErrBinaryFile
	}
	path := snapshot.Path()
	highlights := jide.SyntaxHighlightsResult{
		URI:         path,
		Tokens:      []jide.SyntaxToken{},
		Diagnostics: []jide.SyntaxDiagnostic{},
	}
	if strings.EqualFold(filepath.Ext(path), ".jl") {
		parsed, err := syntax.Parse(text)
		if err != nil {
			return nil, fmt.Errorf("jide: parse Julia document: %w", err)
		}
		highlights = parsed.ProtocolHighlights(path)
	}
	return &DocumentView{
		Path:       path,
		Name:       filepath.Base(path),
		Text:       text,
		Version:    snapshot.Version(),
		Dirty:      snapshot.Dirty(),
		Highlights: highlights,
	}, nil
}
