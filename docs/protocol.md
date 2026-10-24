# The `jide://` protocol

> **Status: stub.** This page describes the envelope that `proto/jide`
> implements today. The method catalog is a starting point, not a frozen
> contract: it grows with the work packages that own the subsystems.

The source of truth for the protocol is the Go package
[`proto/jide`](../proto/jide):

| Concern | Where it lives |
|---|---|
| Request / response / error envelope | [`proto/jide/protocol.go`](../proto/jide/protocol.go) |
| Method names and catalog | [`proto/jide/methods.go`](../proto/jide/methods.go) |
| Parameter and result objects | [`proto/jide/payloads.go`](../proto/jide/payloads.go) |
| JSON Schema export | [`proto/jide/schema.go`](../proto/jide/schema.go) |

When this page and the Go package disagree, the Go package is correct and this
page is a bug. Change both in the same work package.

## Transport

`jide://` is a JSON-RPC 2.0 protocol. The canonical transport is a WebSocket
carrying the `jide` scheme, opened by the shell against the engine daemon.
The envelope is transport agnostic: the same messages travel over a pipe, a
Unix domain socket or a TCP connection without modification.

Framing, reconnection, authentication and the dispatch loop arrive with the
work packages that own them; WP0 only defines the messages.

## Envelope

A **request** is a JSON object:

```json
{
  "jsonrpc": "2.0",
  "id": "req-1",
  "method": "workspace/open",
  "params": { "path": "/home/user/project" }
}
```

* `jsonrpc` is always the string `"2.0"`.
* `id` is optional. A request **without** an `id` is a *notification*: the
  peer must not answer it. When present, `id` is a string, a number or
  `null`.
* `method` is a namespaced method name, `namespace/verb`, for example
  `workspace/open` or `document/didChange`.
* `params` is optional and is always a JSON object.

A **response** is a JSON object:

```json
{ "jsonrpc": "2.0", "id": "req-1", "result": { "workspaceId": "w1", "root": "/home/user/project" } }
```

```json
{ "jsonrpc": "2.0", "id": "req-1", "error": { "code": -32601, "message": "unknown method" } }
```

* `id` is always present. It is `null` when the request identifier could not
  be determined, for example when the request could not be parsed.
* Exactly one of `result` and `error` is set.

## Error codes

The standard JSON-RPC 2.0 codes are used:

| Code | Name | Meaning |
|---|---|---|
| `-32700` | `parseError` | invalid JSON was received |
| `-32600` | `invalidRequest` | the payload is not a valid request object |
| `-32601` | `methodNotFound` | the requested method is unknown |
| `-32602` | `invalidParams` | the method parameters are invalid |
| `-32603` | `internalError` | an internal error in the engine |
| `-32000` | `serverError` | base of the implementation defined range (`-32000`…`-32099`) |

The `error` object may carry an optional `data` member with a structured
payload of any shape.

## Method catalog

The catalog is defined in `proto/jide/methods.go` and exported in the JSON
Schema. Each entry records the direction that may initiate the call
(`clientToServer` — the shell calls the engine, `serverToClient` — the engine
calls the shell, `bidirectional`) and the name of its parameter object.

| Method | Direction | Purpose |
|---|---|---|
| `ping` | bidirectional | liveness probe |
| `workspace/open` | client → server | open a workspace folder |
| `workspace/close` | client → server | close a workspace |
| `workspace/state` | client → server | state of the open workspace |
| `workspace/tree` | client → server | file tree of the open workspace |
| `workspace/didChangeFiles` | server → client | file-system changes of the workspace |
| `document/open` | client → server | open a document for editing |
| `document/close` | client → server | close a document |
| `document/didChange` | client → server | notify the engine of an edit |
| `document/didSave` | client → server | notify the engine of a save |
| `document/read` | client → server | content of an open document |
| `document/save` | client → server | write an open document to disk |
| `index/search` | client → server | search the workspace text index |
| `index/symbols` | client → server | symbols of the current document buffer |
| `index/workspaceSymbols` | client → server | declarations matching a workspace query |
| `syntax/highlights` | client → server | Julia highlighting tokens and parser diagnostics |
| `syntax/folds` | client → server | foldable Julia source ranges and parser diagnostics |
| `lsp/start` | client → server | start the Julia language server of a workspace |
| `lsp/stop` | client → server | stop the Julia language server of a workspace |
| `lsp/status` | client → server | state of the Julia language server |
| `lsp/diagnostics` | server → client | publish diagnostics |
| `lsp/hover` | client → server | hover information at a UTF-16 document position |
| `lsp/completion` | client → server | completion items at a UTF-16 document position |
| `lsp/references` | client → server | source references reported by LanguageServer.jl |
| `lsp/definition` | client → server | source definition reported by LanguageServer.jl |
| `lsp/rename` | client → server | workspace edit reported by LanguageServer.jl |
| `console/open` | client → server | open a REPL session |
| `console/input` | client → server | send input to a REPL session |
| `console/output` | server → client | publish REPL output |
| `console/reopen` | client → server | reopen a saved session environment with command history |
| `console/close` | client → server | stop a session while preserving its history |
| `console/sessions` | client → server | list active and archived sessions |
| `console/history` | client → server | return a session's command history |
| `console/resize` | client → server | resize the session's pseudo-terminal |
| `run/execute` | client → server | start a run configuration |
| `run/terminate` | client → server | stop a running process |
| `dap/breakpoints` | server → client | publish the breakpoint set |
| `dap/stackTrace` | server → client | publish a stack trace |
| `testrun/execute` | client → server | start a test run |
| `extension/list` | client → server | list the registered extension bundles |
| `pkg/environments` | client → server | discover Julia project environments below roots |
| `pkg/add` | client → server | add a package with Julia Pkg |
| `pkg/remove` | client → server | remove a package with Julia Pkg |
| `pkg/upgrade` | client → server | update one or all packages with Julia Pkg |
| `pkg/manifestDiff` | client → server | compare two Manifest.toml files |
| `pkg/dependencyGraph` | client → server | return the resolved package dependency graph |

### The workspace and document surface

The `workspace` and the `document` namespaces cover the project model and
the virtual file system of the engine. The payload objects are described in
the `$defs` section of the exported schema.

`workspace/open` answers with the identifier of the workspace, its root
folder and the path of the detected `Project.toml`. `workspace/state` returns
the whole state of the workspace:

```json
{
  "workspaceId": "/home/user/project",
  "root": "/home/user/project",
  "project": {
    "path": "/home/user/project/Project.toml",
    "name": "Demo",
    "uuid": "6f6c8b12-1f1a-4f0a-9c0a-1f1a4f0a9c0a",
    "juliaCompat": "1.10",
    "compat": { "julia": "1.10" },
    "dependencies": [
      { "name": "Dates", "uuid": "ade2ca70-...", "version": "1.10.0" }
    ],
    "weakDependencies": [],
    "extras": [{ "name": "Test", "uuid": "8dfed614-..." }]
  },
  "manifest": {
    "path": "/home/user/project/Manifest.toml",
    "format": "2.0",
    "juliaVersion": "1.10.3",
    "projectHash": "0f1e2d3c...",
    "packages": [
      { "name": "Dates", "uuid": "ade2ca70-...", "version": "1.10.0" }
    ]
  }
}
```

The `project` and the `manifest` members are absent when the workspace has no
Julia environment.

### Julia package management surface

The `pkg` methods are typed in `proto/jide` and implemented headlessly in
`internal/pkg`. `pkg/environments` accepts roots that are directories or
`Project.toml` files; it searches directories without following symlinks,
skips common generated/dependency folders, and returns projects in path order.
Each result includes parsed project metadata and an optional parsed manifest.

`pkg/add`, `pkg/remove`, and `pkg/upgrade` select an environment by
`projectPath`, which may be the project file or its directory. The package
name is passed as a Julia argument (not interpolated into source or run through
a shell). An empty package name on `pkg/upgrade` means update all dependencies.
The result carries combined Pkg output; subprocess errors include that output
for diagnosis. Pkg runs with startup/history files disabled and activates only
the selected project environment.

`pkg/manifestDiff` compares two explicitly supplied manifest paths and reports
added, removed, and changed package records, matching packages by UUID and
sorting each collection deterministically. `pkg/dependencyGraph` reads the
project and manifest adjacent to `projectPath`, returning the project's direct
dependencies and sorted resolved package nodes with their direct edges. These
methods define the protocol contract; as with the other engine features, they
are not served over a daemon transport until the dispatcher work package is
implemented.

### Interactive Julia consoles

`console/open` accepts a project directory or `Project.toml`, an optional
working directory, and terminal dimensions. It starts Julia with startup files
disabled, history enabled, the chosen project activated, and an interactive
REPL attached to a platform PTY (Unix PTY or Windows ConPTY). The returned
session identifier is used for raw terminal input, resize, close, reopen, and
history requests. `console/output` notifications stream UTF-8 output chunks
and process exit information.

Sessions are independent, so several REPLs can run against separate project
environments. Closed/exited session records and completed input lines remain
available for the lifetime of the engine process; reopening starts a new Julia
process in the same environment and carries the prior in-memory history without
replaying commands. Julia's own REPL history file is also enabled. Session
metadata and streaming output have typed schema definitions, but these methods
remain unavailable over `jided` until daemon dispatch/transport is implemented.

`console/reopen` restores a new live process from a retained session's project
and working directory; `console/sessions` and `console/history` expose archived
records while the engine process remains alive. These records are not a
cross-process persistence format.

`workspace/tree` lists the source tree of the workspace. Every entry holds
the path relative to the root, the absolute path, the resolved real path and
the flags of the entry:

| Flag | Meaning |
|---|---|
| `isDirectory` | the entry is a directory |
| `symlink` | the entry is a symbolic link or a junction |
| `outsideRoot` | the entry points outside the workspace root |
| `alias` | the entry points at a real path the scan already visited |
| `linkTarget` | the raw target of a link |

A link that points outside the root is reported in `externalSymlinks` and is
not followed, so the shell can show it without leaving the workspace. A real
path is listed at most once, so a link that points at an ancestor of the root
cannot make the engine loop.

`workspace/didChangeFiles` publishes the changes that the file watcher
observed. Every change holds the absolute path of the file and its kind:
`created`, `changed` or `deleted`.

`document/read` returns the content of an open document with the version of
its snapshot. `document/save` writes the content of the document to disk and
returns the version of the stored snapshot. The `document/didChange` and
`document/didSave` notifications remain the way the shell announces an edit
or a save that it performed itself.

### Julia syntax surface

The syntax methods accept a document URI and the current editor-buffer text.
They are stateless requests, so parsing does not depend on a file having been
saved to disk:

* `index/symbols` returns nested document symbols for modules, types, functions,
  macros, constants, variables and struct fields. Each symbol has a full
  `range`, a name-only `selectionRange`, and optional child symbols.
* `index/workspaceSymbols` searches declarations indexed from `.jl` files,
  case-insensitively (an empty query lists declarations), and returns the
  declaration name, kind and LSP location.
  `index/search` uses trigram postings to narrow literal text matches across
  indexed text files and then verifies the exact case-sensitive substring in
  source; an empty query returns no matches.
  Text-search line/column values are one-based and columns count Unicode code
  points. A zero result limit means unbounded results; callers should provide
  a UI-appropriate cap.
  The UI's Wails adapter consumes engine-owned `SearchText` and
  `SearchWorkspaceSymbols` results; it does not keep a second index.
* `syntax/highlights` returns non-overlapping lexical tokens. Token categories
  include identifiers, keywords, operators, numbers, comments, strings,
  command literals, escapes and interpolation markers. For `$name` and
  `$(expression)`, the dollar marker has the `interpolation` category and the
  embedded Julia expression keeps its regular token categories.
* `syntax/folds` returns multiline blocks, comments, strings and command
  literals. Fold line numbers are zero-based and inclusive.

All syntax ranges are half-open. Byte offsets and columns count UTF-8 bytes;
line and column positions are zero-based, matching Tree-sitter coordinates.
Malformed or incomplete source still produces syntax output plus diagnostics
for error and missing nodes. The Go source types and method catalog are the
source of truth for the payloads and their JSON Schema definitions.

### Julia language-server surface

The language server is a separate Julia child process selected through Julia or
juliaup's stable channel. The engine requires Julia 1.10 or newer and creates a
LanguageServer.jl package environment in the per-user jide configuration area,
installing the package there without modifying the workspace manifest.
It publishes `lsp/status` and streams only the diagnostics the language server
actually publishes; the editor does not assume every typo or undefined name is
diagnosed. LSP positions are zero-based UTF-16 code-unit columns as required by
the LSP specification; they are distinct from the UTF-8 byte columns used by
Tree-sitter syntax methods.

The desktop UI currently consumes the engine's in-process Wails bridge because
`cmd/jided` does not yet implement the protocol transport or dispatcher. The
method catalog and schema define the protocol contract for a future daemon
connection.

### The language-server payloads

The `lsp` namespace describes the semantic analysis of a document. The
positions of the requests are zero based and their column counts UTF-16 code
units, the unit of the language server protocol; they differ from the UTF-8
byte columns of the syntax methods.

| Method | Parameters | Result |
|---|---|---|
| `lsp/start` | `lspStartParams` | `lspStartResult` |
| `lsp/stop` | `lspStopParams` | `lspStopResult` |
| `lsp/status` | `lspStatusParams` | `lspStatusResult` |
| `lsp/hover` | `lspHoverParams` | `lspHoverResult` |
| `lsp/completion` | `lspCompletionParams` | `lspCompletionResult` |
| `lsp/references` | `lspReferencesParams` | `lspReferencesResult` |
| `lsp/definition` | `lspDefinitionParams` | `lspDefinitionResult` |
| `lsp/diagnostics` | `lspDiagnosticsParams` | — |

`lsp/status` reports the lifecycle state of the server: `stopped`,
`starting`, `running` or `restarting`. The result holds the version and the
juliaup channel of the Julia runtime that runs the server, the number of the
restarts since the last successful start and the description of the last
failure. The shell shows that state in its status bar.

`lsp/hover` answers with the documentation of the symbol under the cursor.
The `contents` member holds the text and its format, `markdown` or
`plaintext`; the member is absent when the server has nothing to say about
the position. `lsp/completion` answers with the items that the server
proposes and with the flag that tells the shell to ask again while the user
types. `lsp/references` and `lsp/definition` answer with the source
locations that the server reports.
`lsp/definition` uses the syntax-derived workspace index while the server is
starting/restarting or stopped, and falls back to that index when a running
server returns no definition. This fallback resolves indexed declarations by
name from the currently open workspace and is intentionally best-effort rather
than Julia semantic analysis.
`lsp/rename` asks LanguageServer.jl for a `WorkspaceEdit`; it returns either
URI-keyed edits or versioned document edits. jide does not apply rename edits
implicitly, so the caller can preview and persist them safely. Empty
replacement names are rejected. Unsupported resource-operation edits return an
error instead of being discarded.

The desktop initializes LanguageServer.jl with the workspace folder. It does
not advertise support for `workspace/configuration`, so LanguageServer.jl uses
its own documented defaults. The client retains a defensive JSON-RPC response
path for server requests, but settings negotiation is intentionally disabled
until a user-settings source is implemented. The LanguageServer package itself is installed in
the per-user tooling environment; when the workspace contains a project file,
that project's directory is selected for analysis, and otherwise Julia
autodetects its default environment. No project file is created in the user's
source-only folder.

The engine restarts the server when it dies, with a backoff that starts at
one second and doubles every attempt, up to five attempts. The state of the
server is `restarting` while the engine waits. The documents that the editor
has open are opened again in the restarted server.

Completion items preserve the server's optional replacement `textEdit`,
`additionalTextEdits`, and `insertTextFormat` fields so an editor can replace
the typed prefix and apply import edits without duplicating text. The desktop
uses the workspace directory as the language-server process working directory
for a source-only folder.

## JSON Schema

`proto/jide` exports the schema of the envelope, the parameter and result
objects and the method catalog:

```go
import "github.com/jide-dev/jide/proto/jide"

raw, err := jide.JSONSchema()      // []byte, valid JSON
err = jide.WriteJSONSchema(os.Stdout)
```

The exported document uses JSON Schema draft 2020-12. The top-level keys are
`$schema`, `$id`, `title`, `description`, `type`, `required`,
`additionalProperties`, `properties`, `$defs` and `x-jide`. The `x-jide`
extension carries the protocol version, the transport name, the method
catalog and the error code table.
