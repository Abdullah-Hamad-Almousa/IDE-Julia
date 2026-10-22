package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/jide-dev/jide/internal/jlproc"
	jide "github.com/jide-dev/jide/proto/jide"
)

// State is the lifecycle state of the language server.
type State string

const (
	// StateStopped means that no server process runs.
	StateStopped State = "stopped"
	// StateStarting means that the client spawns the server and performs
	// the initialize handshake.
	StateStarting State = "starting"
	// StateRunning means that the server is initialized and answers the
	// requests of the shell.
	StateRunning State = "running"
	// StateRestarting means that the server died and that the client waits
	// for the delay of the backoff policy before it spawns it again.
	StateRestarting State = "restarting"
)

// Errors of the client.
var (
	// ErrNotRunning reports that the language server does not run.
	ErrNotRunning = errors.New("lsp: the language server does not run")
	// ErrAlreadyRunning reports that the language server already runs.
	ErrAlreadyRunning = errors.New("lsp: the language server already runs")
	// ErrDocumentNotOpen reports that a document is not open.
	ErrDocumentNotOpen = errors.New("lsp: the document is not open")
)

// ResponseError is the error of a JSON-RPC response of the server.
type ResponseError struct {
	// Code is the JSON-RPC error code of the server.
	Code int
	// Message describes the failure.
	Message string
	// Data is the structured payload of the error.
	Data json.RawMessage
}

// Error implements the error interface.
func (e *ResponseError) Error() string {
	return fmt.Sprintf("lsp: the server answered with the error %d: %s", e.Code, e.Message)
}

// Options configure a Client.
type Options struct {
	// Command starts the language server. When it is nil the client
	// discovers the Julia runtime and starts LanguageServer.jl.
	Command *Command
	// ProjectPath is the path of the Julia project of the workspace. It
	// selects the environment of the server when the client starts
	// LanguageServer.jl itself.
	ProjectPath string
	// RootURI is the root folder of the workspace, as an URI. It is sent in
	// the initialize request.
	RootURI string
	// Backoff is the restart policy of the client.
	Backoff BackoffPolicy
	// Clock is the time source of the restart supervisor. It defaults to
	// SystemClock.
	Clock Clock
	// InitializeTimeout bounds the initialize handshake. The language
	// server of Julia needs seconds to start, because it compiles itself.
	InitializeTimeout time.Duration
	// RequestTimeout bounds a single request.
	RequestTimeout time.Duration
	// Stderr receives the standard error of the server process. It
	// defaults to io.Discard.
	Stderr io.Writer
	// OnDiagnostics is called for every set of diagnostics that the server
	// publishes.
	OnDiagnostics func(uri string, diagnostics []jide.LSPDiagnostic)
	// OnStateChange is called for every transition of the state.
	OnStateChange func(state State)
}

// document is the state of a document that the client keeps in sync with
// the server.
type document struct {
	uri        string
	languageID string
	version    int
	text       string
}

// response is the answer of the server to a request of the client.
type response struct {
	result json.RawMessage
	err    *responseError
}

// process is one generation of the language server process.
type process struct {
	cmd    *exec.Cmd
	stdin  *os.File
	stdout *os.File
	writer *frameWriter
	done   chan error
}

// Client drives a language server over the standard input and the standard
// output of a process.
//
// The client owns the lifecycle of the process: it performs the initialize
// handshake, keeps the document synchronization in sync and restarts the
// server when it dies unexpectedly.
type Client struct {
	opts Options

	mu          sync.Mutex
	state       State
	command     Command
	runtime     *jlproc.Runtime
	proc        *process
	nextID      int64
	pending     map[string]chan *response
	documents   map[string]*document
	diagnostics map[string][]jide.LSPDiagnostic
	restarts    int
	lastError   string
	stopping    bool
	cancel      context.CancelFunc
}

// New returns a client that is not started yet.
//
// The zero value of the options is valid: the client then discovers the
// Julia runtime of the machine and starts LanguageServer.jl with it.
func New(opts Options) *Client {
	if opts.Backoff == (BackoffPolicy{}) {
		opts.Backoff = DefaultBackoffPolicy()
	}
	if opts.Clock == nil {
		opts.Clock = SystemClock{}
	}
	if opts.InitializeTimeout <= 0 {
		opts.InitializeTimeout = 5 * time.Minute
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 30 * time.Second
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	return &Client{
		opts:        opts,
		state:       StateStopped,
		pending:     make(map[string]chan *response),
		documents:   make(map[string]*document),
		diagnostics: make(map[string][]jide.LSPDiagnostic),
	}
}

// Start spawns the language server and performs the initialize handshake.
//
// The method returns when the server answered the initialize request. The
// document synchronization and the restart supervisor start with it.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.state == StateRunning || c.state == StateStarting {
		c.mu.Unlock()
		return fmt.Errorf("lsp: %w", ErrAlreadyRunning)
	}
	c.state = StateStarting
	c.restarts = 0
	c.lastError = ""
	c.stopping = false
	c.mu.Unlock()
	c.notifyState(StateStarting)
	startCtx, cancelStart := context.WithCancel(ctx)
	defer cancelStart()
	go func() {
		select {
		case <-startCtx.Done():
		case <-time.After(c.opts.InitializeTimeout + c.opts.InitializeTimeout + c.opts.InitializeTimeout):
			if c.State() == StateStarting {
				c.killProcess()
				c.setLastError("lsp: language server startup timed out")
				c.setState(StateStopped)
			}
		}
	}()

	command, resolved, err := c.resolve(startCtx)
	if err != nil {
		c.setLastError(err.Error())
		c.setState(StateStopped)
		return err
	}
	if c.opts.Command == nil {
		if err := ensureLanguageServer(startCtx, command); err != nil {
			c.setLastError(err.Error())
			c.setState(StateStopped)
			return err
		}
		command = JuliaCommand(*resolved, c.opts.ProjectPath)
	}
	c.mu.Lock()
	c.command = command
	c.runtime = resolved
	c.mu.Unlock()

	if err := c.startProcess(); err != nil {
		c.setLastError(err.Error())
		c.setState(StateStopped)
		return err
	}
	if startCtx.Err() != nil {
		c.killProcess()
		c.setLastError(startCtx.Err().Error())
		c.setState(StateStopped)
		return startCtx.Err()
	}
	if err := c.initialize(startCtx); err != nil {
		c.killProcess()
		c.setLastError(err.Error())
		c.setState(StateStopped)
		return err
	}
	c.setState(StateRunning)

	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()
	go c.supervise(runCtx)
	return nil
}

// Stop shuts the language server down.
//
// The client asks the server to stop, waits for the process to end and kills
// it when it does not. A client that is already stopped can be stopped
// again.
func (c *Client) Stop(ctx context.Context) error {
	c.mu.Lock()
	proc := c.proc
	cancel := c.cancel
	if proc == nil && c.state == StateStopped {
		c.mu.Unlock()
		return nil
	}
	c.stopping = true
	c.mu.Unlock()

	var err error
	if proc != nil {
		if c.State() != StateStarting {
			shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 5*time.Second)
			_, _ = c.request(shutdownCtx, "shutdown", struct{}{})
			shutdownCancel()
			_ = c.notify(ctx, "exit", struct{}{})
		}
		select {
		case <-proc.done:
		case <-ctx.Done():
			err = ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	if cancel != nil {
		cancel()
	}
	c.killProcess()
	c.failPending(errors.New("lsp: the language server stopped"))
	c.setState(StateStopped)
	return err
}

// currentProcess returns the process of the running server.
//
// The method exists for the tests, which kill the server to exercise the
// restart supervisor.
func (c *Client) currentProcess() *process {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.proc
}

// State returns the lifecycle state of the server.
func (c *Client) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Status returns the state of the server for the lsp/status method.
func (c *Client) Status() jide.LSPStatusResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	status := jide.LSPStatusResult{
		State:           jide.LSPState(c.state),
		RestartAttempts: c.restarts,
		LastError:       c.lastError,
	}
	if c.runtime != nil {
		status.JuliaVersion = c.runtime.Version.String()
		status.JuliaChannel = string(c.runtime.Channel)
		status.Executable = c.runtime.Executable
	}
	return status
}

// Diagnostics returns the diagnostics that the server published for a
// document.
//
// The returned slice is a copy: the caller may keep it.
func (c *Client) Diagnostics(uri string) []jide.LSPDiagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	diagnostics := c.diagnostics[uri]
	if diagnostics == nil {
		return nil
	}
	result := make([]jide.LSPDiagnostic, len(diagnostics))
	copy(result, diagnostics)
	return result
}

// DidOpen notifies the server that a document was opened.
func (c *Client) DidOpen(ctx context.Context, uri, languageID, text string, version int) error {
	if filepath.Ext(PathFromURI(uri)) != ".jl" {
		return nil
	}
	params := didOpenParams{TextDocument: textDocumentItem{
		URI:        uri,
		LanguageID: languageID,
		Version:    version,
		Text:       text,
	}}
	if err := c.notify(ctx, "textDocument/didOpen", params); err != nil {
		return err
	}
	c.mu.Lock()
	c.documents[uri] = &document{uri: uri, languageID: languageID, version: version, text: text}
	c.mu.Unlock()
	return nil
}

// DidChange notifies the server of an edit of a document.
//
// The changes are the incremental changes of the protocol. A change without
// a range replaces the whole document.
func (c *Client) DidChange(ctx context.Context, uri string, version int, changes []jide.LSPTextChange) error {
	if filepath.Ext(PathFromURI(uri)) != ".jl" {
		return nil
	}
	c.mu.Lock()
	doc, known := c.documents[uri]
	var text string
	if known {
		text = doc.text
	}
	c.mu.Unlock()
	if !known {
		return fmt.Errorf("lsp: %w: %s", ErrDocumentNotOpen, uri)
	}
	updated, err := applyChanges(text, changes)
	if err != nil {
		return err
	}
	wire := make([]contentChange, 0, len(changes))
	for _, change := range changes {
		wire = append(wire, contentChange{Range: protocolRange(change.Range), Text: change.Text})
	}
	params := didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: version},
		ContentChanges: wire,
	}
	if err := c.notify(ctx, "textDocument/didChange", params); err != nil {
		return err
	}
	c.mu.Lock()
	doc.version = version
	doc.text = updated
	c.mu.Unlock()
	return nil
}

// DidSave notifies the server that a document was saved.
func (c *Client) DidSave(ctx context.Context, uri, text string) error {
	if filepath.Ext(PathFromURI(uri)) != ".jl" {
		return nil
	}
	params := didSaveParams{TextDocument: textDocumentIdentifier{URI: uri}, Text: text}
	return c.notify(ctx, "textDocument/didSave", params)
}

// DidClose notifies the server that a document was closed.
func (c *Client) DidClose(ctx context.Context, uri string) error {
	if filepath.Ext(PathFromURI(uri)) != ".jl" {
		return nil
	}
	params := didCloseParams{TextDocument: textDocumentIdentifier{URI: uri}}
	if err := c.notify(ctx, "textDocument/didClose", params); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.documents, uri)
	c.mu.Unlock()
	return nil
}

// Hover asks the server for the documentation of the symbol at a position.
//
// The position is zero based and its column counts UTF-16 code units. The
// answer is nil when the server has nothing to say about the position.
func (c *Client) Hover(ctx context.Context, uri string, line, character int) (*jide.LSPHover, error) {
	result, err := c.request(ctx, "textDocument/hover", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     position{Line: line, Character: character},
	})
	if err != nil {
		return nil, err
	}
	return protocolHover(result)
}

// Completion asks the server for the completion items at a position.
func (c *Client) Completion(ctx context.Context, uri string, line, character int) (jide.LSPCompletionResult, error) {
	result, err := c.request(ctx, "textDocument/completion", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     position{Line: line, Character: character},
	})
	if err != nil {
		return jide.LSPCompletionResult{}, err
	}
	return protocolCompletion(result)
}

// References asks the server for the references of the symbol at a
// position.
func (c *Client) References(ctx context.Context, uri string, line, character int, includeDeclaration bool) ([]jide.LSPLocation, error) {
	result, err := c.request(ctx, "textDocument/references", referenceParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     position{Line: line, Character: character},
		Context:      referenceContext{IncludeDeclaration: includeDeclaration},
	})
	if err != nil {
		return nil, err
	}
	return protocolLocations(result)
}

// Definition asks the server for the definition of the symbol at a
// position.
func (c *Client) Definition(ctx context.Context, uri string, line, character int) ([]jide.LSPLocation, error) {
	result, err := c.request(ctx, "textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     position{Line: line, Character: character},
	})
	if err != nil {
		return nil, err
	}
	return protocolLocations(result)
}

// resolve returns the command that starts the language server and the Julia
// runtime that runs it.
//
// The runtime is nil when the caller supplies the command itself.
func (c *Client) resolve(ctx context.Context) (Command, *jlproc.Runtime, error) {
	if c.opts.Command != nil {
		return *c.opts.Command, nil, nil
	}
	resolved, err := jlproc.NewDiscoverer().Discover(ctx)
	if err != nil {
		return Command{}, nil, err
	}
	return JuliaCommand(*resolved, c.opts.ProjectPath), resolved, nil
}

// startProcess spawns the server and starts the goroutines that read its
// output.
func (c *Client) startProcess() error {
	c.mu.Lock()
	command := c.command
	c.mu.Unlock()

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("lsp: open the standard input of the server: %w", err)
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		closeFiles(stdinReader, stdinWriter)
		return fmt.Errorf("lsp: open the standard output of the server: %w", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		closeFiles(stdinReader, stdinWriter, stdoutReader, stdoutWriter)
		return fmt.Errorf("lsp: open the standard error of the server: %w", err)
	}

	cmd := exec.Command(command.Executable, command.Args...)
	cmd.Dir = command.Dir
	if command.Env != nil {
		cmd.Env = command.Env
	}
	cmd.Stdin = stdinReader
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	if err := cmd.Start(); err != nil {
		closeFiles(stdinReader, stdinWriter, stdoutReader, stdoutWriter, stderrReader, stderrWriter)
		return fmt.Errorf("lsp: start %q: %w", command.Executable, err)
	}
	// The child holds its own copy of the ends of the pipes.
	closeFiles(stdinReader, stdoutWriter, stderrWriter)

	proc := &process{
		cmd:    cmd,
		stdin:  stdinWriter,
		stdout: stdoutReader,
		writer: newFrameWriter(stdinWriter),
		done:   make(chan error, 1),
	}
	c.mu.Lock()
	c.proc = proc
	c.mu.Unlock()

	go c.readLoop(proc)
	go c.drainStderr(stderrReader)
	go func() {
		proc.done <- cmd.Wait()
		close(proc.done)
	}()
	return nil
}

// readLoop reads the messages of the server until the stream ends.
func (c *Client) readLoop(proc *process) {
	reader := newFrameReader(proc.stdout)
	for {
		payload, err := reader.read()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				c.setLastError(fmt.Sprintf("lsp: read a message of the server: %v", err))
			}
			return
		}
		c.handleMessage(payload)
	}
}

// drainStderr copies the standard error of the server into the writer of
// the options.
func (c *Client) drainStderr(reader io.ReadCloser) {
	defer reader.Close()
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fmt.Fprintf(c.opts.Stderr, "lsp: %s\n", scanner.Text())
	}
}

// handleMessage dispatches one message of the server.
func (c *Client) handleMessage(payload []byte) {
	var msg message
	if err := json.Unmarshal(payload, &msg); err != nil {
		c.setLastError(fmt.Sprintf("lsp: the server sent an invalid message: %v", err))
		return
	}
	switch {
	case msg.Method == "":
		c.handleResponse(msg)
	case len(msg.ID) == 0:
		c.handleNotification(msg)
	default:
		c.handleServerRequest(msg)
	}
}

// handleResponse delivers the answer of the server to the caller that waits
// for it.
func (c *Client) handleResponse(msg message) {
	key := string(msg.ID)
	c.mu.Lock()
	pending, known := c.pending[key]
	delete(c.pending, key)
	c.mu.Unlock()
	if !known {
		return
	}
	pending <- &response{result: msg.Result, err: msg.Error}
}

// handleNotification handles the notifications of the server.
func (c *Client) handleNotification(msg message) {
	switch msg.Method {
	case "textDocument/publishDiagnostics":
		c.handleDiagnostics(msg.Params)
	default:
		// The other notifications of the protocol, such as the log
		// messages, do not concern the engine.
	}
}

// handleServerRequest answers the requests of the server.
//
// The engine implements no request of the server. It answers them anyway,
// so that the server does not wait for an answer that never comes.
func (c *Client) handleServerRequest(msg message) {
	result := json.RawMessage("null")
	if msg.Method == "workspace/configuration" {
		result = json.RawMessage("[]")
	}
	payload, err := json.Marshal(outgoingResponse{JSONRPC: "2.0", ID: msg.ID, Result: result})
	if err != nil {
		c.setLastError(fmt.Sprintf("lsp: encode the answer of the server request: %v", err))
		return
	}
	c.write(payload)
}

// handleDiagnostics stores the diagnostics that the server published.
func (c *Client) handleDiagnostics(raw json.RawMessage) {
	var params publishDiagnosticsParams
	if err := json.Unmarshal(raw, &params); err != nil {
		c.setLastError(fmt.Sprintf("lsp: read the diagnostics of the server: %v", err))
		return
	}
	diagnostics := protocolDiagnostics(params.Diagnostics)
	c.mu.Lock()
	c.diagnostics[params.URI] = diagnostics
	c.mu.Unlock()
	if c.opts.OnDiagnostics != nil {
		c.opts.OnDiagnostics(params.URI, diagnostics)
	}
}

// initialize performs the initialize handshake of the protocol.
func (c *Client) initialize(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.opts.InitializeTimeout+c.opts.InitializeTimeout)
	defer cancel()
	params := initializeParams{
		ProcessID:    os.Getpid(),
		RootURI:      c.opts.RootURI,
		Capabilities: defaultCapabilities(),
		ClientInfo:   clientInfo{Name: "jide"},
	}
	if c.opts.RootURI != "" {
		params.WorkspaceFolders = []workspaceFolder{{URI: c.opts.RootURI, Name: "workspace"}}
	}
	result, err := c.request(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("lsp: initialize the language server: %w", err)
	}
	if len(result) > 0 {
		var answer initializeResult
		if err := json.Unmarshal(result, &answer); err != nil {
			return fmt.Errorf("lsp: read the answer of the initialize request: %w", err)
		}
	}
	return c.notify(ctx, "initialized", struct{}{})
}

// supervise restarts the server when it dies.
//
// The supervisor waits for the delay of the backoff policy before it spawns
// the server again. It gives up after the last attempt of the policy.
func (c *Client) supervise(ctx context.Context) {
	attempt := 0
	for {
		c.mu.Lock()
		proc := c.proc
		stopping := c.stopping
		c.mu.Unlock()
		if stopping {
			return
		}
		if proc == nil {
			attempt++
			if attempt > c.opts.Backoff.MaxAttempts {
				c.setLastError(fmt.Sprintf(
					"lsp: the language server died and was not restarted after %d attempts",
					c.opts.Backoff.MaxAttempts))
				c.setState(StateStopped)
				return
			}
			c.setState(StateRestarting)
			if err := c.opts.Clock.Sleep(ctx, c.opts.Backoff.Delay(attempt)); err != nil {
				return
			}
			if err := c.startProcess(); err != nil {
				c.setLastError(err.Error())
				continue
			}
			if err := c.initialize(ctx); err != nil {
				c.setLastError(err.Error())
				c.killProcess()
				continue
			}
			if err := c.resyncDocuments(ctx); err != nil {
				c.setLastError(err.Error())
			}
			c.mu.Lock()
			c.restarts = attempt
			c.mu.Unlock()
			c.setState(StateRunning)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case err := <-proc.done:
			c.failPending(err)
			c.closeProcessStreams(proc)
			c.mu.Lock()
			c.proc = nil
			stopping = c.stopping
			c.mu.Unlock()
			if stopping {
				return
			}
		}
	}
}

// resyncDocuments tells the server about the documents that are open.
//
// A restarted server knows nothing about the documents of the editor, so
// the client opens them again.
func (c *Client) resyncDocuments(ctx context.Context) error {
	c.mu.Lock()
	documents := make([]*document, 0, len(c.documents))
	for _, doc := range c.documents {
		documents = append(documents, doc)
	}
	c.mu.Unlock()
	for _, doc := range documents {
		params := didOpenParams{TextDocument: textDocumentItem{
			URI:        doc.uri,
			LanguageID: doc.languageID,
			Version:    doc.version,
			Text:       doc.text,
		}}
		if err := c.notify(ctx, "textDocument/didOpen", params); err != nil {
			return err
		}
	}
	return nil
}

// request sends a request to the server and waits for its answer.
func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.proc == nil || !c.acceptsMessages() {
		c.mu.Unlock()
		return nil, fmt.Errorf("lsp: %w", ErrNotRunning)
	}
	c.nextID++
	id := c.nextID
	key := strconv.FormatInt(id, 10)
	pending := make(chan *response, 1)
	c.pending[key] = pending
	proc := c.proc
	c.mu.Unlock()

	raw, err := marshalParams(params)
	if err != nil {
		c.forget(key)
		return nil, err
	}
	payload, err := json.Marshal(outgoingRequest{JSONRPC: "2.0", ID: id, Method: method, Params: raw})
	if err != nil {
		c.forget(key)
		return nil, fmt.Errorf("lsp: encode the request %q: %w", method, err)
	}
	if err := proc.writer.write(payload); err != nil {
		c.forget(key)
		return nil, err
	}

	timer := time.NewTimer(c.opts.RequestTimeout)
	defer timer.Stop()
	select {
	case answer := <-pending:
		if answer.err != nil {
			return nil, &ResponseError{Code: answer.err.Code, Message: answer.err.Message, Data: answer.err.Data}
		}
		return answer.result, nil
	case <-ctx.Done():
		c.forget(key)
		return nil, ctx.Err()
	case <-timer.C:
		c.forget(key)
		return nil, fmt.Errorf("lsp: the request %q timed out after %s", method, c.opts.RequestTimeout)
	}
}

// notify sends a notification to the server.
func (c *Client) notify(_ context.Context, method string, params any) error {
	c.mu.Lock()
	proc := c.proc
	accepted := c.acceptsMessages()
	c.mu.Unlock()
	if proc == nil || !accepted {
		return fmt.Errorf("lsp: %w", ErrNotRunning)
	}
	raw, err := marshalParams(params)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(outgoingNotification{JSONRPC: "2.0", Method: method, Params: raw})
	if err != nil {
		return fmt.Errorf("lsp: encode the notification %q: %w", method, err)
	}
	return proc.writer.write(payload)
}

// write sends a raw message to the server.
func (c *Client) write(payload []byte) {
	c.mu.Lock()
	proc := c.proc
	c.mu.Unlock()
	if proc == nil {
		return
	}
	if err := proc.writer.write(payload); err != nil {
		c.setLastError(err.Error())
	}
}

// forget removes a request that the server did not answer.
func (c *Client) forget(key string) {
	c.mu.Lock()
	delete(c.pending, key)
	c.mu.Unlock()
}

// failPending answers every request that waits for an answer.
func (c *Client) failPending(cause error) {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[string]chan *response)
	c.mu.Unlock()
	for _, channel := range pending {
		channel <- &response{err: &responseError{Code: -32000, Message: cause.Error()}}
	}
}

// killProcess stops the process of the server.
func (c *Client) killProcess() {
	c.mu.Lock()
	proc := c.proc
	c.proc = nil
	c.mu.Unlock()
	if proc == nil {
		return
	}
	if proc.cmd.Process != nil {
		_ = proc.cmd.Process.Kill()
	}
	c.closeProcessStreams(proc)
}

func (c *Client) closeProcessStreams(proc *process) {
	if proc == nil {
		return
	}
	_ = proc.stdin.Close()
	_ = proc.stdout.Close()
}

// acceptsMessages reports whether the client may talk to the server in its
// current state.
//
// The client talks to the server while it starts it and while it restarts
// it: the handshake of a restarted server needs the initialize request and
// the documents of the editor are opened again.
func (c *Client) acceptsMessages() bool {
	switch c.state {
	case StateRunning, StateStarting, StateRestarting:
		return true
	default:
		return false
	}
}

// setState records the state of the server.
func (c *Client) setState(state State) {
	c.mu.Lock()
	if c.state == state {
		c.mu.Unlock()
		return
	}
	c.state = state
	c.mu.Unlock()
	c.notifyState(state)
}

// notifyState tells the caller about a transition of the state.
func (c *Client) notifyState(state State) {
	if c.opts.OnStateChange != nil {
		c.opts.OnStateChange(state)
	}
}

// setLastError records the description of the last failure.
func (c *Client) setLastError(message string) {
	c.mu.Lock()
	c.lastError = message
	c.mu.Unlock()
}

// marshalParams encodes the parameters of a message.
func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("lsp: encode the parameters: %w", err)
	}
	return raw, nil
}

// closeFiles closes the files of a pipe and ignores the errors.
func closeFiles(files ...*os.File) {
	for _, file := range files {
		if file == nil {
			continue
		}
		_ = file.Close()
	}
}
