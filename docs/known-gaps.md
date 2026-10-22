# Known gaps

Every work package records what it could not finish, what it could not
verify, and what would close the gap. A gap is only closed when the missing
piece exists *and* is verified by an executable test.

Format: one section per work package, newest first within a milestone.

---

## M2 WP4 — Julia LanguageServer integration

### Daemon transport and dispatcher remain unavailable

* **What is missing.** The Wails shell calls the headless LSP service through
  its existing in-process bridge; it does not reach `cmd/jided` through
  JSON-RPC.
* **Why.** `cmd/jided` is still a version stub and has no transport or method
  dispatcher, which is outside WP4's current daemon scope.
* **How to close the gap.** Implement the engine daemon transport/dispatcher
  and replace the temporary Wails adapter with a protocol client while keeping
  the LSP service headless.

---

## M1 WP3 — Wails UI shell

### Workspace operations use a temporary in-process Wails adapter

* **What is missing.** The UI calls the Wails Go binding, which currently
  delegates to the existing `workspace`, `vfs`, and `syntax` packages in the
  shell process. It does not yet call `cmd/jided` over the JSON-RPC protocol.
* **Why.** The daemon is still a version stub; its transport and dispatcher
  are tracked as gaps in WP1/WP2. WP3 needs a usable open-edit-save surface
  before that transport exists.
* **How to close the gap.** Implement daemon request dispatch for workspace,
  document, and syntax methods, then replace the in-process bridge with a
  protocol client and run the UI acceptance flow end to end.

### Native desktop smoke has not been run

* **What is missing.** `npm run check`, the production bundle build, and
  headless Go open-edit-save tests pass, but a Wails window has not been
  launched and exercised by automation in this environment.
* **Why.** The development harness can build the Windows shell but does not
  provide a controllable desktop/webview session for a scripted interaction.
* **How to close the gap.** Run the smoke steps in [`ui/README.md`](../ui/README.md)
  on a desktop with the WebView2 runtime, then add platform GUI automation when
  a stable Wails test driver is selected.

### Julia runtime selection is a scaffold

* **What is missing.** The status-bar picker switches between detected
  `Project.toml` metadata and a default option; it does not discover Julia
  installations or launch a runtime.
* **Why.** `juliaup` discovery and environment supervision belong to WP4.
* **How to close the gap.** Populate picker options from the runtime supervisor
  and persist/select the resolved Julia executable in WP4.

---

## M1 WP2 — Julia syntax

### Syntax RPC methods are not served by the daemon yet

* **What is missing.** The protocol defines `index/symbols`,
  `syntax/highlights` and `syntax/folds`, and `internal/syntax` creates their
  result objects, but the current `cmd/jided` version stub does not dispatch
  these methods over a transport.
* **Why.** WP2 adds the headless parser and protocol surface. The daemon
  transport and method dispatcher are not part of this work package.
* **How to close the gap.** Add a daemon dispatcher when the transport is
  implemented and exercise the three requests end to end over that transport.

---

## M1 WP1 — workspace, vfs, protocol surface

### The symbolic links of Windows are not exercised

* **What is missing.** The test suite of `internal/workspace` creates the
  links of the fixture project with `os.Symlink`. On Windows the creation of a
  symbolic link needs the developer mode or administrator rights, so the
  tests fall back to a directory junction, which is a reparse point that needs
  no privilege. The scanner handles both the same way — the link is detected
  by reading its target, the target is resolved and a target outside the root
  is reported — but the behaviour of a *symbolic* link on Windows, with a
  relative target and with a broken target, is not verified on that platform.
* **Why.** The development machine of this work package runs Windows without
  the developer mode and without administrator rights.
* **How to close the gap.** Enable the developer mode on the Windows runners
  of the CI, or run the suite once on a Windows machine with the developer
  mode enabled. The tests then use the symbolic links and the junction
  fallback is not needed.

### The polling watcher cannot report a rename

* **What is missing.** The polling backend of `internal/vfs` reports a
  renamed file as the removal of the old path and the creation of the new
  path. Only the native backend, which is built on the notifications of the
  operating system, reports the `rename` operation.
* **Why.** A poller compares the state of the tree with the state of the
  previous scan; it cannot tell a rename from a removal and a creation
  without the identity of the file, which the portable API of the standard
  library does not offer.
* **How to close the gap.** Use the native backend on the platforms that
  offer it, which is the default of `vfs.NewFSNotifyWatcher`, and keep the
  polling backend as the fallback. A poller that detects the renames needs
  the file identity of the platform, for example the file index of Windows or
  the device and the inode of Unix.

### The kqueue backend of the BSD systems does not report the modifications

* **What is missing.** The test of the native watcher asserts the creation
  and the removal of a file on every platform, but not the modification: the
  kqueue backend of the BSD systems, which macOS uses, reports the changes
  of the entries of a watched directory, not the changes of the files inside
  it.
* **Why.** The limitation belongs to the notifications of the operating
  system, not to the engine.
* **How to close the gap.** Watch the files themselves on the BSD systems, or
  fall back to the polling backend for the modifications there.

### The polling watcher does not see a rewrite of the same size

* **What is missing.** The polling backend of `internal/vfs` detects the
  modification of a file by its size and by its modification time. A file
  that is rewritten with the same size inside the resolution of the file
  system — about 15 milliseconds on Windows — keeps the same modification
  time, so the change stays invisible until the next change of the file.
* **Why.** The portable API of the standard library offers no identity and
  no content hash of a file, so a poller has nothing else to compare.
* **How to close the gap.** Use the native backend, which is the default of
  `vfs.NewFSNotifyWatcher`, or hash the content of the files that the
  poller watches.

### The engine daemon does not serve the new methods yet

* **What is missing.** The methods `workspace/state`, `workspace/tree`,
  `workspace/didChangeFiles`, `document/read` and `document/save` exist in
  the protocol catalog and the engine packages produce their payload
  objects, but no daemon answers them: the dispatch loop of the engine
  arrives with the work package that owns it.
* **Why.** WP1 delivers the engine packages and the protocol surface; the
  daemon of WP0 is a version stub.
* **How to close the gap.** Implement the dispatch loop of the engine daemon
  and register the handlers of the workspace and the document methods. The
  acceptance test of that work package must exercise the methods end to end
  over the transport.

### The tree hash of a package is empty for the manifest format 2.0

* **What is missing.** The tree hash of a resolved package is only available
  for the manifest format 1.0, which records it as `git-tree-sha1`. The
  manifest format 2.0 does not record it.
* **Why.** The tree hash of the format 2.0 lives in the `Package.toml` of the
  package in the depot of Julia, which the project model does not read.
* **How to close the gap.** Read the depot of Julia, which belongs to the
  work package that owns the package manager (M3 WP6), and fill the tree
  hash from the `Package.toml` of the package.

### The save of a document is not atomic

* **What is missing.** The store of `internal/vfs` writes the content of a
  document to the file itself, so a crash during the save can leave a
  truncated file behind.
* **Why.** The atomic write needs a temporary file in the folder of the
  document and a rename, which changes the identity of the file and the
  behaviour of the file watcher.
* **How to close the gap.** Write the content to a temporary file in the
  folder of the document and rename it over the document. The file watcher
  then reports a rename instead of a write, which the protocol surface
  already maps onto a deletion and a creation.

---

## M0 WP0 — repository, workspace, CI, protocol skeleton

### CI has not been executed on GitHub

* **What is missing.** The GitHub Actions workflow
  (`.github/workflows/ci.yml`) is committed but has never run on GitHub.
* **Why.** WP0 is developed in a local workspace. Nothing was pushed from
  this workspace, so the workflow has no run to point at. The `build`,
  `workspace` and `lint` jobs — 3 operating systems × 3 modules, plus the
  workspace-wide job — are therefore *unverified on GitHub*.
* **How to close the gap.** Push the branch to a GitHub remote and check that
  the `CI` workflow is green on `ubuntu-latest`, `macos-latest` and
  `windows-latest`. Until then, the local reproduction in
  "How the CI steps are reproduced locally" below is the only evidence.

### The JSON Schema is not validated against the JSON Schema meta-schema

* **What is missing.** `proto/jide.JSONSchema()` is checked for being valid
  JSON, for its top-level keys, for a resolvable set of `$ref` pointers and
  for determinism, but it is not validated against
  `https://json-schema.org/draft/2020-12/schema`.
* **Why.** The protocol module deliberately has no third-party dependency, so
  that it stays a dependency-free, headless, trivially vendored module.
  Adding a JSON Schema validator would pull a dependency into every consumer
  of the protocol.
* **How to close the gap.** Add the validator as a *test-only* dependency of
  the `proto/jide` module (`go.mod` test requirements are not inherited by
  consumers) and validate the exported schema in `schema_test.go`.

### `jide://` transport, dispatch loop and authentication are undefined

* **What is missing.** The protocol defines the messages only. The WebSocket
  transport, the method dispatch loop, the cancellation model and the
  authentication of the `jide://` URL are not specified.
* **Why.** They belong to the work packages that own the engine daemon and
  the desktop shell (M1 WP3 and the engine WPs), and they depend on decisions
  that are not taken yet.
* **How to close the gap.** Specify the transport in `docs/protocol.md`
  together with the implementation of the daemon, and record the decision as
  an ADR under `docs/`.

### `cmd/jided` and `cmd/jide` are version stubs

* **What is missing.** The daemon does not open a socket, the shell does not
  open a window, and neither of them implements a single method of the
  catalog.
* **Why.** WP0 is the scaffolding work package. The engine and the UI arrive
  with the WPs that own them.
* **How to close the gap.** Implement the daemon dispatch loop and the Wails
  shell in their respective work packages; the acceptance test of each
  relevant WP must exercise the protocol end to end.

### Julia is not exercised yet

* **What is missing.** No test in the repository needs a Julia runtime, so
  no test is skipped for a missing Julia installation.
* **Why.** WP0 has no Julia-facing code. The first Julia dependency arrives
  with the language server (M2 WP4), which must skip its end-to-end test
  when no Julia runtime is available and record it in this file.
* **How to close the gap.** Add a Julia-dependent work package section to
  this file the moment such a test exists.
