package jide

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Method is the name of a jide protocol method.
//
// Most method names are namespaced with a single "/" separator, for example
// "workspace/open". The namespace groups methods by the engine subsystem that
// owns them. A handful of global methods, such as "ping", have no namespace.
type Method string

// Direction describes which peer is allowed to initiate a method call.
//
// The "client" is the UI shell, the "server" is the engine daemon.
type Direction string

const (
	// DirectionClientToServer marks a method the UI shell may call.
	DirectionClientToServer Direction = "clientToServer"
	// DirectionServerToClient marks a method the engine daemon may call.
	DirectionServerToClient Direction = "serverToClient"
	// DirectionBidirectional marks a method both peers may call.
	DirectionBidirectional Direction = "bidirectional"
)

// MethodInfo describes a single protocol method.
type MethodInfo struct {
	Name      Method    `json:"name"`
	Direction Direction `json:"direction"`
	Summary   string    `json:"summary"`
	Params    string    `json:"params,omitempty"`
	Result    string    `json:"result,omitempty"`
}

// Protocol method names.
//
// The catalog is the single source of truth for the method surface of the
// protocol; the JSON Schema enumerates it.
const (
	// MethodPing is a liveness probe. It always succeeds.
	MethodPing Method = "ping"

	// MethodWorkspaceOpen opens a workspace in the engine daemon.
	MethodWorkspaceOpen Method = "workspace/open"
	// MethodWorkspaceClose closes a previously opened workspace.
	MethodWorkspaceClose Method = "workspace/close"
	// MethodWorkspaceState returns the state of the open workspace.
	MethodWorkspaceState Method = "workspace/state"
	// MethodWorkspaceTree lists the file tree of the open workspace.
	MethodWorkspaceTree Method = "workspace/tree"
	// MethodWorkspaceDidChangeFiles publishes file-system changes of the
	// open workspace.
	MethodWorkspaceDidChangeFiles Method = "workspace/didChangeFiles"

	// MethodDocumentOpen opens a document for editing.
	MethodDocumentOpen Method = "document/open"
	// MethodDocumentClose closes an open document.
	MethodDocumentClose Method = "document/close"
	// MethodDocumentDidChange notifies the engine of a document edit.
	MethodDocumentDidChange Method = "document/didChange"
	// MethodDocumentDidSave notifies the engine that a document was saved.
	MethodDocumentDidSave Method = "document/didSave"
	// MethodDocumentRead returns the content of an open document.
	MethodDocumentRead Method = "document/read"
	// MethodDocumentSave writes a document to disk and returns the version of
	// the stored snapshot.
	MethodDocumentSave Method = "document/save"

	// MethodIndexSearch searches workspace source text with literal matching.
	MethodIndexSearch Method = "index/search"
	// MethodIndexSymbols returns the symbols of a document.
	MethodIndexSymbols Method = "index/symbols"
	// MethodIndexWorkspaceSymbols searches declarations in the workspace.
	MethodIndexWorkspaceSymbols Method = "index/workspaceSymbols"

	// MethodSyntaxHighlights returns lexer-level Julia highlighting tokens.
	MethodSyntaxHighlights Method = "syntax/highlights"
	// MethodSyntaxFolds returns the foldable ranges of a Julia document.
	MethodSyntaxFolds Method = "syntax/folds"

	// MethodLSPStart starts the Julia language server of a workspace.
	MethodLSPStart Method = "lsp/start"
	// MethodLSPStop stops the Julia language server of a workspace.
	MethodLSPStop Method = "lsp/stop"
	// MethodLSPStatus reports the state of the Julia language server.
	MethodLSPStatus Method = "lsp/status"
	// MethodLSPHover asks the language server for the documentation of the
	// symbol at a position.
	MethodLSPHover Method = "lsp/hover"
	// MethodLSPCompletion asks the language server for the completion
	// items at a position.
	MethodLSPCompletion Method = "lsp/completion"
	// MethodLSPReferences asks the language server for the references of the
	// symbol at a position.
	MethodLSPReferences Method = "lsp/references"
	// MethodLSPDefinition asks the language server for the definition of
	// the symbol at a position and falls back to the workspace index.
	MethodLSPDefinition Method = "lsp/definition"
	// MethodLSPRename asks the language server to rename the symbol at a
	// position throughout the workspace.
	MethodLSPRename Method = "lsp/rename"
	// MethodLSPDiagnostics publishes diagnostics for a document.
	MethodLSPDiagnostics Method = "lsp/diagnostics"

	// MethodConsoleOpen opens a REPL session in a Julia environment.
	MethodConsoleOpen Method = "console/open"
	// MethodConsoleInput sends user input to a REPL session.
	MethodConsoleInput Method = "console/input"
	// MethodConsoleOutput publishes REPL output.
	MethodConsoleOutput Method = "console/output"
	// MethodConsoleReopen restarts a previous REPL session with its history.
	MethodConsoleReopen Method = "console/reopen"
	// MethodConsoleClose stops a REPL session.
	MethodConsoleClose Method = "console/close"
	// MethodConsoleSessions lists active and archived REPL sessions.
	MethodConsoleSessions Method = "console/sessions"
	// MethodConsoleHistory returns command history for a REPL session.
	MethodConsoleHistory Method = "console/history"
	// MethodConsoleResize changes the terminal size of a REPL session.
	MethodConsoleResize Method = "console/resize"

	// MethodRunExecute starts a run configuration.
	MethodRunExecute Method = "run/execute"
	// MethodRunTerminate stops a running process.
	MethodRunTerminate Method = "run/terminate"
	// MethodRunInput writes text to an interactive run process.
	MethodRunInput Method = "run/input"
	// MethodRunOutput publishes a stdout/stderr chunk from a run process.
	MethodRunOutput Method = "run/output"
	// MethodRunExited publishes the final process status.
	MethodRunExited Method = "run/exited"

	// MethodDAPStart starts and initializes a debug adapter.
	MethodDAPStart Method = "dap/start"
	// MethodDAPLaunch launches the configured debuggee.
	MethodDAPLaunch Method = "dap/launch"
	// MethodDAPAttach attaches to a running debuggee.
	MethodDAPAttach Method = "dap/attach"
	// MethodDAPSetBreakpoints replaces source breakpoints.
	MethodDAPSetBreakpoints Method = "dap/setBreakpoints"
	// MethodDAPSetFunctionBreakpoints replaces function breakpoints.
	MethodDAPSetFunctionBreakpoints Method = "dap/setFunctionBreakpoints"
	// MethodDAPSetExceptionBreakpoints selects adapter exception filters.
	MethodDAPSetExceptionBreakpoints Method = "dap/setExceptionBreakpoints"
	// MethodDAPConfigurationDone signals that configuration is complete.
	MethodDAPConfigurationDone Method = "dap/configurationDone"
	// MethodDAPStackTrace returns a thread's stack frames.
	MethodDAPStackTrace Method = "dap/stackTrace"
	// MethodDAPThreads returns the adapter's active threads.
	MethodDAPThreads Method = "dap/threads"
	// MethodDAPContinue resumes a stopped thread.
	MethodDAPContinue Method = "dap/continue"
	// MethodDAPNext steps over the current source line.
	MethodDAPNext Method = "dap/next"
	// MethodDAPStepIn enters a function call.
	MethodDAPStepIn Method = "dap/stepIn"
	// MethodDAPStepOut exits the current function.
	MethodDAPStepOut Method = "dap/stepOut"
	// MethodDAPStepInTargets lists function call targets on the current line.
	MethodDAPStepInTargets Method = "dap/stepInTargets"
	// MethodDAPPause interrupts a running thread.
	MethodDAPPause Method = "dap/pause"
	// MethodDAPScopes requests scopes for a stack frame.
	MethodDAPScopes Method = "dap/scopes"
	// MethodDAPVariables requests variable values.
	MethodDAPVariables Method = "dap/variables"
	// MethodDAPEvaluate evaluates an expression in a debug context.
	MethodDAPEvaluate Method = "dap/evaluate"
	// MethodDAPSetVariable assigns a value to a debug variable.
	MethodDAPSetVariable Method = "dap/setVariable"
	// MethodDAPExceptionInfo returns details for the stopped exception.
	MethodDAPExceptionInfo Method = "dap/exceptionInfo"
	// MethodDAPTerminate terminates the active debuggee.
	MethodDAPTerminate Method = "dap/terminate"
	// MethodDAPDisconnect ends a debug session.
	MethodDAPDisconnect Method = "dap/disconnect"
	// MethodDAPEvent publishes a DAP event to the shell.
	MethodDAPEvent Method = "dap/event"
	// MethodTestRunExecute starts a test run.
	MethodTestRunExecute Method = "testrun/execute"
	// MethodTestRunEvent publishes a streamed Test Explorer update.
	MethodTestRunEvent Method = "testrun/event"

	// MethodExtensionList lists the registered extension bundles.
	MethodExtensionList Method = "extension/list"

	// MethodPkgEnvironments lists Julia project environments under the requested roots.
	MethodPkgEnvironments Method = "pkg/environments"
	// MethodPkgAdd adds a package to a Julia project environment.
	MethodPkgAdd Method = "pkg/add"
	// MethodPkgRemove removes a package from a Julia project environment.
	MethodPkgRemove Method = "pkg/remove"
	// MethodPkgUpgrade updates some or all packages in a Julia project environment.
	MethodPkgUpgrade Method = "pkg/upgrade"
	// MethodPkgManifestDiff compares two Julia manifests.
	MethodPkgManifestDiff Method = "pkg/manifestDiff"
	// MethodPkgDependencyGraph returns the dependency graph of a Julia environment.
	MethodPkgDependencyGraph Method = "pkg/dependencyGraph"
)

// methodCatalog is the ordered list of every method known to the protocol.
var methodCatalog = []MethodInfo{
	{
		Name:      MethodPing,
		Direction: DirectionBidirectional,
		Summary:   "Liveness probe; always succeeds.",
	},
	{
		Name:      MethodWorkspaceOpen,
		Direction: DirectionClientToServer,
		Summary:   "Open a workspace (folder) in the engine daemon.",
		Params:    "workspaceOpenParams",
	},
	{
		Name:      MethodWorkspaceClose,
		Direction: DirectionClientToServer,
		Summary:   "Close a previously opened workspace.",
		Params:    "workspaceCloseParams",
	},
	{
		Name:      MethodWorkspaceState,
		Direction: DirectionClientToServer,
		Summary:   "Return the state of the open workspace.",
		Params:    "workspaceStateParams",
	},
	{
		Name:      MethodWorkspaceTree,
		Direction: DirectionClientToServer,
		Summary:   "List the file tree of the open workspace.",
		Params:    "workspaceTreeParams",
	},
	{
		Name:      MethodWorkspaceDidChangeFiles,
		Direction: DirectionServerToClient,
		Summary:   "Publish file-system changes of the open workspace.",
		Params:    "workspaceDidChangeFilesParams",
	},
	{
		Name:      MethodDocumentOpen,
		Direction: DirectionClientToServer,
		Summary:   "Open a document for editing.",
		Params:    "documentOpenParams",
	},
	{
		Name:      MethodDocumentClose,
		Direction: DirectionClientToServer,
		Summary:   "Close an open document.",
		Params:    "documentCloseParams",
	},
	{
		Name:      MethodDocumentDidChange,
		Direction: DirectionClientToServer,
		Summary:   "Notify the engine of a document edit.",
		Params:    "documentChangeParams",
	},
	{
		Name:      MethodDocumentDidSave,
		Direction: DirectionClientToServer,
		Summary:   "Notify the engine that a document was saved.",
		Params:    "documentSaveParams",
	},
	{
		Name:      MethodDocumentRead,
		Direction: DirectionClientToServer,
		Summary:   "Return the content of an open document.",
		Params:    "documentReadParams",
	},
	{
		Name:      MethodDocumentSave,
		Direction: DirectionClientToServer,
		Summary:   "Write an open document to disk.",
		Params:    "documentSaveParams",
	},
	{
		Name:      MethodIndexSearch,
		Direction: DirectionClientToServer,
		Summary:   "Search the workspace text index.",
		Params:    "textSearchParams",
	},
	{
		Name:      MethodIndexSymbols,
		Direction: DirectionClientToServer,
		Summary:   "Return the symbols of a document.",
		Params:    "documentSymbolsParams",
	},
	{
		Name:      MethodIndexWorkspaceSymbols,
		Direction: DirectionClientToServer,
		Summary:   "Search declarations in the workspace symbol index.",
		Params:    "workspaceSymbolsParams",
	},
	{
		Name:      MethodSyntaxHighlights,
		Direction: DirectionClientToServer,
		Summary:   "Return lexer-level Julia highlighting tokens and syntax diagnostics.",
		Params:    "syntaxHighlightsParams",
	},
	{
		Name:      MethodSyntaxFolds,
		Direction: DirectionClientToServer,
		Summary:   "Return foldable Julia source ranges and syntax diagnostics.",
		Params:    "syntaxFoldsParams",
	},
	{
		Name:      MethodLSPStart,
		Direction: DirectionClientToServer,
		Summary:   "Start the Julia language server of a workspace.",
		Params:    "lspStartParams",
	},
	{
		Name:      MethodLSPStop,
		Direction: DirectionClientToServer,
		Summary:   "Stop the Julia language server of a workspace.",
		Params:    "lspStopParams",
	},
	{
		Name:      MethodLSPStatus,
		Direction: DirectionClientToServer,
		Summary:   "Report the state of the Julia language server.",
		Params:    "lspStatusParams",
	},
	{
		Name:      MethodLSPDiagnostics,
		Direction: DirectionServerToClient,
		Summary:   "Publish diagnostics for a document.",
		Params:    "lspDiagnosticsParams",
	},
	{
		Name:      MethodLSPHover,
		Direction: DirectionClientToServer,
		Summary:   "Return the documentation of the symbol at a position.",
		Params:    "lspHoverParams",
	},
	{
		Name:      MethodLSPCompletion,
		Direction: DirectionClientToServer,
		Summary:   "Return the completion items at a position.",
		Params:    "lspCompletionParams",
	},
	{
		Name:      MethodLSPReferences,
		Direction: DirectionClientToServer,
		Summary:   "Return the references of the symbol at a position.",
		Params:    "lspReferencesParams",
	},
	{
		Name:      MethodLSPDefinition,
		Direction: DirectionClientToServer,
		Summary:   "Return the definition of the symbol at a position.",
		Params:    "lspDefinitionParams",
	},
	{
		Name:      MethodLSPRename,
		Direction: DirectionClientToServer,
		Summary:   "Rename the symbol at a position using LanguageServer.jl.",
		Params:    "lspRenameParams",
	},
	{
		Name:      MethodConsoleOpen,
		Direction: DirectionClientToServer,
		Summary:   "Open a REPL session for a Julia environment.",
		Params:    "consoleOpenParams",
	},
	{
		Name:      MethodConsoleInput,
		Direction: DirectionClientToServer,
		Summary:   "Send user input to a REPL session.",
		Params:    "consoleInputParams",
	},
	{
		Name:      MethodConsoleOutput,
		Direction: DirectionServerToClient,
		Summary:   "Publish REPL output.",
		Params:    "consoleOutputParams",
	},
	{
		Name:      MethodConsoleReopen,
		Direction: DirectionClientToServer,
		Summary:   "Reopen a previous REPL session in its Julia environment.",
		Params:    "consoleSessionParams",
	},
	{
		Name:      MethodConsoleClose,
		Direction: DirectionClientToServer,
		Summary:   "Stop a REPL session and preserve its history.",
		Params:    "consoleSessionParams",
	},
	{
		Name:      MethodConsoleSessions,
		Direction: DirectionClientToServer,
		Summary:   "List active and archived REPL sessions.",
	},
	{
		Name:      MethodConsoleHistory,
		Direction: DirectionClientToServer,
		Summary:   "Return command history for a REPL session.",
		Params:    "consoleSessionParams",
	},
	{
		Name:      MethodConsoleResize,
		Direction: DirectionClientToServer,
		Summary:   "Resize a REPL session terminal.",
		Params:    "consoleResizeParams",
	},
	{
		Name:      MethodRunExecute,
		Direction: DirectionClientToServer,
		Summary:   "Start a run configuration.",
		Params:    "runExecuteParams",
		Result:    "runExecuteResult",
	},
	{
		Name:      MethodRunTerminate,
		Direction: DirectionClientToServer,
		Summary:   "Stop a running process.",
		Params:    "runTerminateParams",
		Result:    "runTerminateResult",
	},
	{
		Name:      MethodRunInput,
		Direction: DirectionClientToServer,
		Summary:   "Write stdin data to an interactive run process.",
		Params:    "runInputParams",
		Result:    "runInputResult",
	},
	{
		Name:      MethodRunOutput,
		Direction: DirectionServerToClient,
		Summary:   "Publish process output from a run configuration.",
		Params:    "runOutputParams",
	},
	{
		Name:      MethodRunExited,
		Direction: DirectionServerToClient,
		Summary:   "Publish the final process status of a run configuration.",
		Params:    "runExitedParams",
	},
	{Name: MethodDAPStart, Direction: DirectionClientToServer, Summary: "Discover and initialize DebugAdapter.jl or start an explicit adapter.", Params: "dapStartParams", Result: "dapStartResult"},
	{Name: MethodDAPLaunch, Direction: DirectionClientToServer, Summary: "Launch a debuggee through the adapter.", Params: "dapArgumentsParams", Result: "dapEmptyResult"},
	{Name: MethodDAPAttach, Direction: DirectionClientToServer, Summary: "Attach to a debuggee through the adapter.", Params: "dapArgumentsParams", Result: "dapEmptyResult"},
	{Name: MethodDAPConfigurationDone, Direction: DirectionClientToServer, Summary: "Finish configuring a debug session.", Result: "dapEmptyResult"},
	{Name: MethodDAPStackTrace, Direction: DirectionClientToServer, Summary: "Return stack frames for a debuggee thread.", Params: "dapStackTraceParams", Result: "dapStackTraceResult"},
	{Name: MethodDAPThreads, Direction: DirectionClientToServer, Summary: "List threads in the debuggee.", Params: "dapThreadsParams", Result: "dapThreadsResult"},
	{Name: MethodDAPContinue, Direction: DirectionClientToServer, Summary: "Continue a stopped debuggee thread.", Params: "dapThreadParams", Result: "dapContinueResult"},
	{Name: MethodDAPNext, Direction: DirectionClientToServer, Summary: "Step over the current source line.", Params: "dapThreadParams", Result: "dapEmptyResult"},
	{Name: MethodDAPStepIn, Direction: DirectionClientToServer, Summary: "Step into the current function call.", Params: "dapThreadParams", Result: "dapEmptyResult"},
	{Name: MethodDAPStepOut, Direction: DirectionClientToServer, Summary: "Step out of the current function.", Params: "dapThreadParams", Result: "dapEmptyResult"},
	{Name: MethodDAPStepInTargets, Direction: DirectionClientToServer, Summary: "List step-in targets for a stack frame.", Params: "dapFrameParams", Result: "dapStepInTargetsResult"},
	{Name: MethodDAPPause, Direction: DirectionClientToServer, Summary: "Pause a running debuggee thread.", Params: "dapThreadParams", Result: "dapEmptyResult"},
	{Name: MethodDAPScopes, Direction: DirectionClientToServer, Summary: "Request scopes for a stack frame.", Params: "dapFrameParams", Result: "dapScopesResult"},
	{Name: MethodDAPVariables, Direction: DirectionClientToServer, Summary: "Request variables in a scope or value.", Params: "dapVariablesParams", Result: "dapVariablesResult"},
	{Name: MethodDAPEvaluate, Direction: DirectionClientToServer, Summary: "Evaluate an expression in the debug context.", Params: "dapEvaluateParams", Result: "dapEvaluateResult"},
	{Name: MethodDAPSetBreakpoints, Direction: DirectionClientToServer, Summary: "Replace source-line breakpoints.", Params: "dapSetBreakpointsParams", Result: "dapBreakpointsResult"},
	{Name: MethodDAPSetFunctionBreakpoints, Direction: DirectionClientToServer, Summary: "Replace function breakpoints.", Params: "dapFunctionBreakpointsParams", Result: "dapBreakpointsResult"},
	{Name: MethodDAPDisconnect, Direction: DirectionClientToServer, Summary: "Disconnect the debuggee and adapter.", Params: "dapDisconnectParams", Result: "dapEmptyResult"},
	{Name: MethodDAPSetExceptionBreakpoints, Direction: DirectionClientToServer, Summary: "Select adapter exception breakpoint filters.", Params: "dapExceptionBreakpointsParams", Result: "dapBreakpointsResult"},
	{Name: MethodDAPSetVariable, Direction: DirectionClientToServer, Summary: "Assign a value to a debug variable.", Params: "dapSetVariableParams", Result: "dapSetVariableResult"},
	{Name: MethodDAPExceptionInfo, Direction: DirectionClientToServer, Summary: "Return details about a stopped exception.", Params: "dapThreadParams", Result: "dapExceptionInfoResult"},
	{Name: MethodDAPTerminate, Direction: DirectionClientToServer, Summary: "Terminate the active debuggee.", Result: "dapEmptyResult"},
	{Name: MethodDAPEvent, Direction: DirectionServerToClient, Summary: "Publish an adapter event.", Params: "dapEventParams"},
	{
		Name:      MethodTestRunExecute,
		Direction: DirectionClientToServer,
		Summary:   "Start a Julia @testset run and return its run identifier.",
		Params:    "testRunExecuteParams",
		Result:    "testRunExecuteResult",
	},
	{
		Name:      MethodTestRunEvent,
		Direction: DirectionServerToClient,
		Summary:   "Stream test-set, assertion, output, and completion updates.",
		Params:    "testRunEventParams",
	},
	{
		Name:      MethodExtensionList,
		Direction: DirectionClientToServer,
		Summary:   "List the registered extension bundles.",
	},
	{
		Name:      MethodPkgEnvironments,
		Direction: DirectionClientToServer,
		Summary:   "List Julia project environments below one or more roots.",
		Params:    "pkgEnvironmentsParams",
	},
	{
		Name:      MethodPkgAdd,
		Direction: DirectionClientToServer,
		Summary:   "Add a package to a Julia project environment through Pkg.",
		Params:    "pkgOperationParams",
	},
	{
		Name:      MethodPkgRemove,
		Direction: DirectionClientToServer,
		Summary:   "Remove a package from a Julia project environment through Pkg.",
		Params:    "pkgOperationParams",
	},
	{
		Name:      MethodPkgUpgrade,
		Direction: DirectionClientToServer,
		Summary:   "Update a named package or all packages through Pkg.",
		Params:    "pkgOperationParams",
	},
	{
		Name:      MethodPkgManifestDiff,
		Direction: DirectionClientToServer,
		Summary:   "Compare two Julia Manifest.toml files.",
		Params:    "pkgManifestDiffParams",
	},
	{
		Name:      MethodPkgDependencyGraph,
		Direction: DirectionClientToServer,
		Summary:   "Return the resolved package dependency graph of an environment.",
		Params:    "pkgDependencyGraphParams",
	},
}

// methodIndex is the lookup table built from methodCatalog.
var methodIndex = buildMethodIndex()

func buildMethodIndex() map[Method]MethodInfo {
	index := make(map[Method]MethodInfo, len(methodCatalog))
	for _, info := range methodCatalog {
		index[info.Name] = info
	}
	return index
}

// Methods returns a copy of the method catalog in declaration order.
func Methods() []MethodInfo {
	out := make([]MethodInfo, len(methodCatalog))
	copy(out, methodCatalog)
	return out
}

// MethodNames returns the names of every known method, sorted alphabetically.
func MethodNames() []string {
	names := make([]string, 0, len(methodCatalog))
	for _, info := range methodCatalog {
		names = append(names, string(info.Name))
	}
	sort.Strings(names)
	return names
}

// Info returns the catalog entry for the method.
func (m Method) Info() (MethodInfo, bool) {
	info, ok := methodIndex[m]
	return info, ok
}

// IsKnown reports whether the method is part of the protocol catalog.
func (m Method) IsKnown() bool {
	_, ok := methodIndex[m]
	return ok
}

// String returns the method name.
func (m Method) String() string {
	return string(m)
}

// Validate returns an error when the method name does not follow the
// protocol naming convention.
func (m Method) Validate() error {
	return ValidateMethodName(string(m))
}

// ValidateMethodName checks that a method name follows the protocol naming
// convention.
//
// A method name is either a global method, a single lowercase word such as
// "ping", or a namespaced method "namespace/verb" such as
// "document/didChange". The namespace is lowercase; the verb may use
// camelCase. Both parts may additionally contain digits, ".", "_" and "-".
func ValidateMethodName(name string) error {
	if name == "" {
		return errors.New("jide: method name is empty")
	}
	namespace, verb, found := strings.Cut(name, "/")
	if !found {
		return validateGlobalMethod(name)
	}
	if strings.Contains(verb, "/") {
		return fmt.Errorf("jide: method name %q has more than one %q separator", name, "/")
	}
	if err := validateNamespace(namespace, name); err != nil {
		return err
	}
	return validateVerb(verb, name)
}

// validateGlobalMethod checks a method name without a namespace.
func validateGlobalMethod(name string) error {
	if name == "" {
		return errors.New("jide: method name is empty")
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return fmt.Errorf("jide: method name %q must start with a letter", name)
			}
		default:
			return fmt.Errorf("jide: method name %q contains the invalid character %q", name, r)
		}
	}
	return nil
}

// validateNamespace checks the namespace part of a method name.
func validateNamespace(namespace, name string) error {
	if namespace == "" {
		return fmt.Errorf("jide: method name %q has an empty namespace", name)
	}
	for i, r := range namespace {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return fmt.Errorf("jide: method name %q must start with a letter", name)
			}
		case r == '.' || r == '_' || r == '-':
		default:
			return fmt.Errorf("jide: method name %q contains the invalid character %q", name, r)
		}
	}
	return nil
}

// validateVerb checks the verb part of a method name.
func validateVerb(verb, name string) error {
	if verb == "" {
		return fmt.Errorf("jide: method name %q has an empty verb", name)
	}
	for i, r := range verb {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return fmt.Errorf("jide: method name %q must start with a letter", name)
			}
		case r == '.' || r == '_' || r == '-':
		default:
			return fmt.Errorf("jide: method name %q contains the invalid character %q", name, r)
		}
	}
	return nil
}
