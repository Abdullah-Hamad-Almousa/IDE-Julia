# Fix WP4 — Julia language intelligence recovery plan

**Status:** implementation applied; headless and build verification complete;
interactive Windows Wails acceptance remains open.
**Scope:** restore Julia server startup, diagnostics, hover, completion, and
references in the user's source-only workspace on Windows.
**Authority:** `Plan.md` §1.1, §2, §4, and §5 (M2 WP4), plus `AGENTS.md`.

This document began as the investigation plan and now records the applied fix
and verification. The implementation did not modify the user's `A:\test`
files, terminate pre-existing Wails processes, commit, or push.

## 1. Problem and evidence

The user reports that the server remains stopped, hover and diagnostics do
not work, and Ctrl+Space and Shift+F12 do nothing. The source-only startup
change in commit `85513c5` did not produce an observable improvement.

Confirmed from the current source and previous inspection:

- The target workspace contains Julia files without a root `Project.toml`.
- `cmd/jide/app.go` now selects source-only workspaces for startup, constructs
  an LSP client, and exposes Start/Retry methods. This proves selection logic
  exists; it does not prove runtime discovery or server initialization works.
- The existing source-only test checks the selection predicate, not successful
  process startup, document synchronization, or UI behavior.
- `run-jide.cmd` invokes `run-jide.ps1`, which launches `wails dev`.
  A separate desktop executable was observed to predate the frontend build.
  Old `wails dev -noreload` processes were also observed. Neither observation
  proves which app the user was actually using.
- In `ui/src/App.svelte`, the startup error is still placed in tooltips rather
  than rendered as visible explanatory text. The earlier claim that errors
  were displayed directly was incomplete.
- `ui/src/lib/syntaxBridge.ts` registers providers but imports Monaco's editor
  API entry point. Whether the required hover, suggest, and references UI
  contributions are loaded must be verified; provider registration alone is
  not proof that editor actions exist.
- Provider exceptions are converted into empty results, hiding the distinction
  between no results and a failed backend call.
- The real Julia test permits empty hover/completion results, does not assert
  useful reference locations, and does not test published diagnostics. It
  uses a project fixture, not a source-only folder.
- Previous frontend and LSP checks passed, but desktop checks were blocked by
  CGO/compiler invocation problems. A green package summary also does not
  establish whether the real Julia test ran or skipped.

**Confirmed issues:** source-only environment selection and Windows file-URI
drive-colon encoding were incorrect; editor providers could fail silently;
completion replacement metadata was dropped; status errors were hidden in
tooltips; and Monaco UI contributions/actions were not reliably wired. The
real Julia test also showed that default LanguageServer.jl lint does not
promise to report arbitrary undefined identifiers, so an empty diagnostic set
is not evidence of a broken push-diagnostics path.

## 2. Investigation findings

### Phase A — Identify and reproduce the running application

1. Record the exact launch command, executable identity, frontend source
   (Vite or embedded bundle), and active workspace returned by the Go bridge.
   Keep machine-specific paths in local diagnostic output, not committed docs.
2. Check whether the live bridge actually exposes `GetLSPStatusView`,
   `StartLanguageServer`, and `RetryLanguageServer`; inspect its returned state
   and error rather than relying only on the footer label.
3. Use one identified fresh build/session for reproduction. Preserve unsaved
   documents before closing any app; do not kill unrelated development servers.
4. Resolve the desktop build blocker using Go 1.24 and the launcher's GCC/PATH
   setup. Prove the compiler works with a build, rather than guessing short
   paths or treating `-CheckOnly` as proof of a successful compilation.
5. Reproduce against a temporary source-only fixture, then confirm against the
   user's workspace without modifying its project metadata or source files.

**Finding:** `run-jide.cmd` launches the dev app from source; older Wails
processes and the separate embedded executable can be stale. Those processes
were left untouched. Native interactive verification still must identify a
fresh session.

### Phase B — Trace real Julia startup independently of the editor

Trace these boundaries in order:

1. Workspace selection and LSP client creation.
2. Julia/juliaup discovery and supported runtime version.
3. Per-user LanguageServer.jl environment setup and package load.
4. Server command construction, working directory, analysis environment,
   symbol-cache path, and compatibility with the installed package API.
5. Child process creation, stderr, exit status, and stdio framing.
6. `initialize` response, advertised capabilities, `initialized` notification,
   server configuration requests, and analysis readiness.
7. `didOpen` followed by a real semantic request and diagnostic notification.

Use the actual startup command and a bounded headless reproduction. Capture
failure output before changing startup arguments or increasing timeouts.
Distinguish missing runtime, package setup failure, process exit, handshake
timeout, and analysis still warming up.

Inspect `workspace/configuration` responses: the current unconditional empty
array must be checked against the server's requested items. Also inspect
ignored server log/progress notifications and discarded initialize capabilities.

**Finding:** with Julia 1.13.1 and LanguageServer.jl 6.0.1 installed, a real
source-only workspace initialized and returned non-empty hover/completion and
locations for definition and cross-file references. The test did not create a
`Project.toml` in the fixture. The client now separates LanguageServer.jl's
per-user package environment from the workspace working directory and uses a
workspace Project.toml only when present. Windows file URIs retain the drive
colon unescaped, matching Julia/Monaco canonical URIs.

Configuration finding: the previous empty response was not the actual
initialization path. LanguageServer.jl only asks `workspace/configuration`
when the client advertises support; the desktop now omits that capability so
the server falls back to its documented defaults. A defensive server-request
response remains for unexpected requests, and is covered by a direct protocol
test. Per-user settings remain a separately documented gap.

### Phase C — Trace the editor end to end

For each feature, trace editor action → provider → Wails binding → LSP request
→ response/notification → Monaco rendering.

- Verify hover, suggest, and references contributions/actions are present in
  the bundled Monaco build. Test mouse hover, action invocation, and keyboard
  invocation separately to distinguish missing actions from key interception.
- Inspect Windows file URI equivalence, drive-letter case, escaping, model
  lookup, UTF-16 positions, and zero-based/one-based range conversion.
- Verify documents opened during startup are replayed once with their latest
  text and version. Verify edits, closes, retries, and automatic restarts.
- Confirm a semantic request cannot overtake a pending edit synchronization.
- Confirm diagnostics reach the correct model, survive the event/subscription
  startup ordering, and clear when fixed or when the workspace is replaced.
- Check navigation to a reference in a file whose Monaco model is not already
  open, including the Go document-open/synchronization path.

**Finding:** the UI now loads the pinned Monaco hover/suggest/reference
contributions, registers explicit Ctrl+Space and Shift+F12 actions, uses the
references action backed by the LSP provider, and opens cross-file locations
through the Wails document bridge. Completion text edits and snippets are
preserved. Provider exceptions and backend startup errors are surfaced in
visible status panels. Diagnostics are normalized across Windows URI casing
and fetched from backend state after subscription to avoid event races.

## 3. Applied implementation

Applied only fixes supported by the traces above.

### A. Reliable startup and lifecycle

Likely files: `internal/jlproc/discover.go`, `internal/lsp/command.go`,
`internal/lsp/client.go`, and `cmd/jide/app.go`.

- Keep LanguageServer.jl installed in the per-user tooling environment while
  analyzing the selected workspace. Support a directory without creating a
  `Project.toml` in the user's folder.
- Consolidate Start/Retry behavior around one guarded lifecycle operation.
  Concurrent starts must not spawn duplicate servers or corrupt status.
- Make setup, initialization, and shutdown bounded and cancellable. A timeout
  must cancel the underlying operation, not merely change the displayed state.
- Ignore callbacks from superseded workspace/server sessions. Ensure workspace
  switches, retries, crashes, and shutdown cannot leave an old server active.
- Preserve actionable startup errors and useful bounded process output.
- Correct handshake/configuration/capability handling where the real-server
  trace demonstrates a mismatch.

### B. Visible, truthful status

Likely files: `ui/src/App.svelte`, `ui/src/lib/api.ts`, `ui/src/lib/types.ts`,
`ui/src/app.css`, and the backend status producer.

- Display startup stages and failure text directly in a visible status panel
  or banner, with a discoverable Start/Retry action and copyable details.
- Render backend-owned status consistently across initial load, events, and
  polling; surface bridge failures instead of silently showing `unavailable`.
- Explain that analysis can still warm up after the LSP handshake completes.
- Add build identity to diagnostics if Phase A demonstrates ambiguity; verify
  that the frontend and backend identity describe the same running build.

### C. Working Monaco features

Likely files: `ui/src/lib/syntaxBridge.ts`, `ui/src/lib/Editor.svelte`, and
document synchronization methods in `cmd/jide/app.go`.

- If absent, explicitly import the required Monaco editor contributions using
  paths supported by the pinned Monaco version.
- Repair URI, model lifecycle, position conversion, and synchronization issues
  demonstrated by the trace.
- Honor completion replacement ranges/text edits so accepting a suggestion
  replaces the typed prefix instead of duplicating it.
- Route cross-file navigation through the existing document-opening flow.
- Report provider failures through bounded, non-spamming diagnostics; preserve
  legitimate empty results and normal request cancellation.
- Keep syntax-parser markers and LanguageServer diagnostics distinct so a
  syntax underline cannot falsely satisfy the LSP acceptance criterion.

## 4. Acceptance tests

Use disposable fixtures with and without `Project.toml`. Establish a fixture
that produces stable results with the tested Julia/LanguageServer versions;
an arbitrary misspelled word is not a sufficient diagnostic contract.

| Check | Required evidence |
|---|---|
| Source-only startup | Real process initializes for a folder containing `.jl` files; no project/manifest is created in that folder |
| Project startup | Existing project environment continues to initialize and resolve its symbols |
| Hover | Non-empty expected documentation or signature is returned and rendered over a known symbol |
| Completion | Ctrl+Space opens suggestions containing an expected item; acceptance edits the intended prefix correctly |
| References | Shift+F12 shows expected declaration/use locations and opens a reference in another file |
| Definition | Existing definition support still navigates to the expected location |
| Diagnostics | A known server-reported error arrives through `publishDiagnostics`, appears at the correct range, and clears after correction |
| Startup replay | A file opened and edited while starting is analyzed using its latest text |
| Failure visibility | Missing Julia and failed initialization produce visible, persistent, actionable errors |
| Retry and restart | Recovery succeeds, preserves open documents, and creates no duplicate server or stale diagnostics |
| Workspace switch | Old callbacks cannot change the new workspace's state or markers |
| Windows identity | URI encoding/case and non-ASCII positions work for requests, navigation, and diagnostics |

Testing layers:

1. Extend existing fake-server tests for deterministic lifecycle, configuration,
   synchronization, and failure scenarios.
2. Strengthen `internal/lsp/julia_test.go` with meaningful result assertions and
   a source-only case. Use bounded waits for analysis/diagnostics rather than
   arbitrary sleeps. Report actual execution versus skip explicitly.
3. Add an executable frontend integration check for loaded Monaco actions,
   provider invocation, result rendering, and marker lifecycle. A mocked bridge
   verifies UI wiring but must not substitute for the real-server test.
4. Exercise the native Windows Wails app for hover, Ctrl+Space, Shift+F12, and
   failure/retry behavior. Record manual steps/results if desktop automation
   is unavailable; keep that limitation open.

## 5. Verification and handover

- Run build/test/vet per workspace module and workspace-wide using explicit
  module patterns from `go.work`, following `AGENTS.md`.
- Require clean `gofmt` and pinned golangci-lint checks.
- Run `npm ci` and `npm run build` in `ui/`; regenerate embedded assets and
  validate the desktop against those assets as well as the development path.
- Record unsupported or unavailable environments as explicit skips/gaps.
  A skipped real-server or native smoke test does not establish this user
  issue as fixed.
- Update `docs/known-gaps.md` with actual remaining limitations and verification
  evidence. Reconcile outdated WP4 status statements only after acceptance.
- If protocol/status schemas change, update Go protocol types, JSON Schema,
  and `docs/protocol.md` together. Review public API changes as required.
- Keep the daemon-dispatcher gap documented; do not expand this repair into a
  daemon rewrite without evidence that it is needed for the reported failure.
- Preserve unrelated work and the user's files. When implementation is later
  authorized and verified, use one focused Conventional Commit with public API
  changes listed in its body. Do not push.

## 6. Verification outcome and remaining acceptance

Verified in this worktree:

- `internal/lsp`: full `go test ./...`, `go vet ./...`, pinned
  `golangci-lint run ./...`, and the real Julia/LanguageServer integration test.
- `proto/jide`: full test, vet, and pinned lint; completion schema and payload
  include text edits and insert format.
- `cmd/jide`: Windows CGO test, vet, and build pass using the available Octave
  GCC added to PATH.
- `internal/jlproc`, `internal/workspace`, `internal/vfs`, and `cmd/jided`:
  tests and vet pass.
- `ui`: `npm run check` and production build pass; embedded Wails assets were
  regenerated.
- `git diff --check` passes.

Still open:

- The interactive Windows WebView has not been exercised against the user's
  `A:\test` workspace. Confirm hover, Ctrl+Space, Shift+F12, cross-file
  navigation, and visible startup errors in a fresh app session. Two older
  `wails dev` sessions were observed and deliberately left running.
- `golangci-lint` on `cmd/jide` aborts before analysis because this local
  Windows CGO/Tree-sitter setup provides no export data for
  `internal/syntax/juliagrammar`. This is recorded in `docs/known-gaps.md`.
- LanguageServer.jl defaults do not guarantee diagnostics for arbitrary typos;
  the fake-server test validates diagnostics transport, while the real test
  validates semantic requests. Do not interpret an empty diagnostic set as a
  failure unless the server is expected to report that specific issue.
- `cmd/jided` transport/dispatcher remains outside this fix.

No commit or push was made. The completion gate is not fully closed until the
fresh native Wails smoke test is performed.
