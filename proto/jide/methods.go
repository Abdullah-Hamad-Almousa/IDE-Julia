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

	// MethodIndexSearch runs a text search over the workspace index.
	MethodIndexSearch Method = "index/search"
	// MethodIndexSymbols returns the symbols of a document.
	MethodIndexSymbols Method = "index/symbols"

	// MethodLSPStatus reports the state of the Julia language server.
	MethodLSPStatus Method = "lsp/status"
	// MethodLSPDiagnostics publishes diagnostics for a document.
	MethodLSPDiagnostics Method = "lsp/diagnostics"

	// MethodConsoleOpen opens a REPL session in a Julia environment.
	MethodConsoleOpen Method = "console/open"
	// MethodConsoleInput sends user input to a REPL session.
	MethodConsoleInput Method = "console/input"
	// MethodConsoleOutput publishes REPL output.
	MethodConsoleOutput Method = "console/output"

	// MethodRunExecute starts a run configuration.
	MethodRunExecute Method = "run/execute"
	// MethodRunTerminate stops a running process.
	MethodRunTerminate Method = "run/terminate"

	// MethodDAPBreakpoints publishes the debugger breakpoint set.
	MethodDAPBreakpoints Method = "dap/breakpoints"
	// MethodDAPStackTrace publishes a debugger stack trace.
	MethodDAPStackTrace Method = "dap/stackTrace"

	// MethodTestRunExecute starts a test run.
	MethodTestRunExecute Method = "testrun/execute"

	// MethodExtensionList lists the registered extension bundles.
	MethodExtensionList Method = "extension/list"
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
		Name:      MethodLSPStatus,
		Direction: DirectionClientToServer,
		Summary:   "Report the state of the Julia language server.",
	},
	{
		Name:      MethodLSPDiagnostics,
		Direction: DirectionServerToClient,
		Summary:   "Publish diagnostics for a document.",
		Params:    "diagnosticsParams",
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
		Name:      MethodRunExecute,
		Direction: DirectionClientToServer,
		Summary:   "Start a run configuration.",
		Params:    "runExecuteParams",
	},
	{
		Name:      MethodRunTerminate,
		Direction: DirectionClientToServer,
		Summary:   "Stop a running process.",
		Params:    "runTerminateParams",
	},
	{
		Name:      MethodDAPBreakpoints,
		Direction: DirectionServerToClient,
		Summary:   "Publish the debugger breakpoint set.",
		Params:    "breakpointsParams",
	},
	{
		Name:      MethodDAPStackTrace,
		Direction: DirectionServerToClient,
		Summary:   "Publish a debugger stack trace.",
		Params:    "stackTraceParams",
	},
	{
		Name:      MethodTestRunExecute,
		Direction: DirectionClientToServer,
		Summary:   "Start a test run.",
		Params:    "testRunExecuteParams",
	},
	{
		Name:      MethodExtensionList,
		Direction: DirectionClientToServer,
		Summary:   "List the registered extension bundles.",
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
