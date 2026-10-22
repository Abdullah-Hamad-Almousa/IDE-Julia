# Plan.md — Building `jide`: a Julia Programming IDE, implemented in Go

Status: draft v1 · Date: 2026-10-22 · Workspace: `A:\IDE\OpenCode-folder\REA-PyCharm`
Primary implementation language: **Go** (all IDE logic)
Target language served: **Julia**
Build model: **AI agents** work in parallel work packages (WPs) against a monorepo.

---

## 0. Why this plan is grounded in evidence

PyCharm (JetBrains' Python IDE) was reverse-engineered with **REA** (shipped-artifact
static analysis of its installed distribution). Findings below distinguish
`observed` (REA Evidence / shipped metadata), `inferred` (from evidence), and
`unknown` (not established). The plan translates this architecture into a
Go-native design — it adopts PyCharm's *structure*, not its implementation
stack (no Java/JVM; no Java-style plugin loading).

### 0.1 PyCharm identity (shipped metadata)

| Fact | Value |
|---|---|
| Product | PyCharm Professional-capable distribution, `product-info.json` name "PyCharm", version 2026.2.3 |
| Build | `PY-262.10968.92` (`build.txt`) |
| Runtime | JVM app; requires Java ≥ 25; bundles `jbr/` (JetBrains Runtime) |
| Launcher | `bin/pycharm64.exe` → `jbr/bin/java.exe`; `bin/pycharm64.exe.vmoptions`; also `pycharm.bat`, `remote-dev-server.exe`, `fsnotifier.exe`, `WinShellIntegrationBridge.dll` |
| Composition | `product-info.json` lists ~hundreds of modules with kinds `plugin`, `productModuleV2`, `moduleV2`; 114 bundled plugin directories |
| Data dir | `%LOCALAPPDATA%\JetBrains\PyCharm2026.2` (per-version state separation) |

### 0.2 How PyCharm actually works (observed architecture)

All JAR inspection is ZIP-format static analysis; Evidence IDs cited in Appendix A.

1. **Boot classpath + modular jars.** `platform-loader.jar` is the bootstrap
   (129 entries, ZIP inventory observed). The classpath itself is declared in
   `product-info.json` (`bootClassPathJarNames`: ~100 lib jars including
   `util-8.jar`, `product-backend.jar`, `fleet.*.jar` — a split-frontend/remote
   codepath is present). Largest platform jars: `intellij.platform.ide.impl.jar`
   74.7 MB, `intellij.platform.lang.impl.jar` 55.2 MB.

2. **Descriptor-driven composition, not code.** There is no hard-coded product;
   the product is *assembled from XML descriptors*:
   - `META-INF/PyCharmCorePlugin.xml` (in `intellij.pycharm.community.jar`) is
     auto-generated ("DO NOT EDIT — Generate Product Layouts"): it declares
     module aliases (`com.intellij.modules.pycharm`, `...pycharm.community`,
     `...python-core-capable`) and `xi:include`s `PlatformLangPlugin.xml`,
     `pycharm-core.xml`, and moduleSet files (`intellij.moduleSets.ide.common.xml`…).
   - `META-INF/PythonPlugin.xml` (in `intellij.pycharm.pro.jar`, ~1.4 MB) is an
     aggregate of 22 embedded `<module>` fragments (profiler, charts, onboarding,
     settings-transfer, a DAP debugger adapter, …).
   - Platform core declares its machinery in split files inside
     `intellij.platform.ide.impl.jar`: `PlatformExtensionPoints.xml` (54 KB),
     `PlatformExtensions.xml` (178 KB), `LangExtensionPoints.xml` (61 KB),
     `LangExtensions.xml` (146 KB), `PlatformLangComponents.xml`, plus layered
     splits (`CodeStyle.xml`, `Analysis.xml`, `Indexing.xml`, `VcsExtensionPoints.xml`…).

3. **Extension-point / extension model is the integration contract.**
   `PythonPlugin.xml` uses ~600 distinct extension point (EP) names. Usage
   distribution (counted): `applicationService` 444, `registryKey` 193,
   `projectService` 190, `statistics.*` collectors 131, `notificationGroup` 84,
   `advancedSetting` 68, `editorActionHandler` 66, `postStartupActivity` 36,
   `highlightingPassFactory` 22, `completion.contributor` 14, … . EPs expose
   language-agnostic hooks (parser definitions, syntax highlighters, indexers,
   inspections, intentions, refactoring processors, run configuration types,
   executors/program runners, file-based indexes/stub indexes, structure view…).

4. **Language support is a layered module stack.** Python CE modules
   (`plugins/python-ce/lib/modules/*.jar`) split concerns explicitly:
   - `intellij.python.parser` — `fileType` registration: `Python` (.py;pyw,
     hashBang "python"), `PythonStub` (.pyi); lexer stack classes
     (`PythonLexer`/`_PythonLexer` JFlex-style, string/f-string specialized
     lexers, indenting lexer), recursive-descent-style parser classes
     (`parsing/PyParser`, `PyLazyParser`, `StatementParsing`,
     `ExpressionParsing`, `FunctionParsing`, `PatternParsing`).
   - `intellij.python.ast` — a standalone immutable `PyAst*` element hierarchy
     (60+ node classes) decoupled from PSI.
   - `intellij.python.psi` (+ `psi.impl`) — the **PSI contract layer**: plugin's
     own extension points under a `Pythonid.*` namespace
     (`pyReferenceResolveProvider`, `typeProvider`, `pyImportResolver`,
     `pySuperMethodsSearch`, `pyClassInheritorsSearch`, `typeCheckerExtension`,
     …). `psi.impl` adds the workhorses: **92 local inspections, 40 intention
     actions, 22 completion contributors, 9 stub indexes, 12 type providers,
     8 annotators** (counts from descriptor).
   - `intellij.python.syntax.core` — formatter (`lang.formatter`
     `PythonFormattingModelBuilder`, pre/post-format processors), commenter,
     docstring engines (Google/NumPy/Sphinx styles), code style services.
   - `intellij.python.community.impl` (38 MB, 7 112 entries) — feature wiring:
     run configuration types (`PythonConfigurationType`, Django/Flask/FastAPI/Dbt
     locked types, `PythonTestConfigurationType`, tox, uv), program runners
     (`PythonRunner`, `PythonDebugProgramRunner`), breakpoint types
     (`PyLineBreakpointType`, `PyExceptionBreakpointType`), console filters,
     SDK **flavor providers** (conda, pipenv, system, Maya…), **package manager
     providers** (pip, conda, pipenv, poetry, uv, hatch), scientific stack,
     auxiliary languages (Requirements parser, TOML/Poetry dependency completion,
     `PythonRegExp` dialect via injection).

5. **Debugger is protocol-based (DAP).** An embedded DAP module declares
   `com.intellij.platform.dap.debugAdapterSupportProvider` and
   `dap.timeout.*` keys (init/launch/attach/evaluate/breakpoint-verification
   defaults 10–30 s) — i.e. PyCharm talks **Debug Adapter Protocol** to a
   separate adapter process, not in-process debugging.

6. **Modern split architecture is present.** Module naming shows
   `*.frontend` / `*.backend` / `*.shared` splits and Remote Development
   (`intellij.rd.*` jars, `remote-dev-server.exe`) — UI, engine, and process
   model are being factored apart with an RPC protocol between them. Fleet
   jars on the boot classpath corroborate this direction.

7. **Runtime environment (configuration observed, behavior inferred).**
   `idea.properties`, per-version data dirs, `fsnotifier.exe` (native file
   watcher for VFS), JCEF (embedded Chromium) for HTML panes, and registry
   keys (`registryKey`) as massive compile-time-configurable tuning surface.

**Unknowns (not established by this analysis):** no live-process/runtime
observation was performed (no decompilation of JVM bytecode to method level;
thread/EDT model is inferred from platform knowledge, not observed). Fleet
jar usage in PyCharm's boot is observed but unexplored.

### 0.3 Lessons adopted for `jide`

| PyCharm mechanism | `jide` decision (Go-native) |
|---|---|
| Descriptors + extension points | **YAML/JSON extension manifests** registered through in-process Go interfaces (no reflection XML loading). EP registry is a typed Go API. |
| application/project services | Go interfaces + small DI container per app/project scope; services resolved at startup, not lazily through XML classpaths |
| Bootstrap jar + classpath | Single static binary (or thin launcher); no dynamic classpath. Components are Go packages with dependency manifests still kept declarative for the *plugin loader* |
| PSI + AST layering | **Two-level tree**: cheap syntactic tree from an embedded parser for instant response + authoritative semantic model served by the Julia language server |
| JFlex lexer + hand parser | **tree-sitter-julia** embedded via `go-tree-sitter` for highlights/folding/symbols/day-1 speed; no hand-written Julia lexer/parser in v1 |
| Stub/file-based indexes | Go-native content + symbol indexes (trigram index for text search; symbol/name index from tree-sitter; refreshed incrementally) |
| Run configurations (typed) | Typed, JSON-marshalable run-config registry (`julia-run`, `julia-script`, `julia-test`, `julia-repl`, plus user-defined types) |
| Executors / runners | `Executor` + `Runner` interfaces with goroutine-based process supervision (PTY console streaming) |
| DAP debugger module | First-class **DAP client** (timeout policy + adapter discovery like PyCharm's DAP EP model); we will *not* implement Julia debugging logic ourselves |
| `*.frontend/*.backend/*.shared` + RD | **Protocol-first split**: an engine daemon (Go) speaking JSON-RPC 2.0 over a documented schema; UI shell is a client. Enables agents to test engine headless |
| Startup activities / registries (`registryKey`, `advancedSetting`) | Closed set of typed settings documented as Go structs exported as JSON Schema; no open-ended `registryKey` sprawl in v1 |

---

## 1. Product definition

**Name:** `jide` (working title: *Julia IDE in Go*).
**Goal:** a local, first-party-feeling Julia IDE with: project/workspace model
aware of `Project.toml`/`Manifest.toml` environments; a fast editor; code
intelligence; REPL console; package management UI; debugger; test explorer;
extensible via Go.

**Non-goals for v1:** remote development/deployment, Jupyter/notebook apps,
AI-completion plumbing, collaboration features, marketplace distribution.

### 1.1 Julia ecosystem contract (external processes `jide` talks to)

| Concern | Provider (do not re-implement) | Transport |
|---|---|---|
| Semantic analysis | `LanguageServer.jl` (julia-vscode stack) | LSP 3.17 over stdio |
| Debugging | `DebugAdapter.jl` (uses `JuliaInterpreter.jl`) | DAP over socket/stdio |
| REPL / tasks | `julia` process (juliaup-discovered) | ConPTY / PTY streaming |
| Packages | `Pkg` invoked within the project env | REPL subprocess + structured output |
| Formatting | `JuliaFormatter.jl` (invoked via LSP `textDocument/formatting` request to LS or direct) | LSP |

Key risk call-out: Julia's first-run latency (JIT + LS startup is slow) and
environment switching are the UX-critical processes to engineer around —
same class of problem PyCharm solved with interpreter flavors + flavors-independent caches.

---

## 2. System architecture (Go)

```
┌───────────────────────────────────────────────────────────────┐
│  UI shell (desktop)  [Wails v2: Go core + webview front]       │
│   editor · tree · console · debugger panes · settings          │
└───────────────▲───────────────────────────────────────────────┘
                │ jide:// JSON-RPC 2.0 (WebSocket, typed schema)
┌───────────────▼───────────────────────────────────────────────┐
│  engine daemon (Go)                                            │
│  workspace · vfs · index · syntax(tree-sitter) · extension mgr  │
├───────────────────────────────────────────────────────────────┤
│  julia runtime supervisor (Go)                                 │
│   LanguageServer.jl (LSP client) │ Console (PTY) │ DAP client   │
│   Pkg runner                     │ Test runner (structured I/O)  │
└───────────────────────────────────────────────────────────────┘
```

Monorepo layout (Go multi-module, `go.work`):

```
jide/
  cmd/jided/          daemon entry
  cmd/jide/           desktop shell entry
  proto/jide/         JSON-RPC schema (Go structs + JSON Schema export)
  internal/workspace/ project model, Project.toml/Manifest.toml awareness
  internal/vfs/       file watching (fsnotify), snapshots, save pipeline
  internal/syntax/    tree-sitter-julia: parse, highlight, folds, symbols
  internal/index/     trigram text index, symbol index
  internal/ext/       extension-point registry + manifest loader
  internal/lsp/       LSP client for LanguageServer.jl
  internal/dap/       DAP client for DebugAdapter.jl
  internal/jlproc/    Julia discovery (juliaup), env supervisor, PTY console
  internal/pkg/       Pkg integration UI model layer
  internal/runconf/   run-configuration registry
  internal/testrun/   @testset runner
  ui/                 frontend (Svelte + TypeScript, Monaco) rendered by Wails
  docs/               this Plan.md, protocol docs, ADRs
```

**Threading/rx model (replaces EDT):** engine single binary with
`errgroup`-scoped goroutines; document store uses snapshot arcs
(edit → debounced reparse → index update → LSP didChange); UI thread does
rendering only and never blocks on Julia processes. Every blocking Julia
operation runs cancellable with progress reporting.

---

## 3. Extension model (the "plugin.xml" successor)

- Manifest: `jide-extension.yaml` per in-tree or external bundle:
  `name, version, engine:"^2", extensionPoints:[…], contributes:[…]`.
- Typed contribution points in Go (declared in `internal/ext`), each a
  function/factory type so loading a bundle is `ext.Register(manifest, impls)`.
- v1 built-in bundles mirror PyCharm layering:
  `julia-core` (fileTypes, syntax, navigation),
  `julia-lsp` (server integration),
  `julia-run` (run configs, executors),
  `julia-debug` (DAP),
  `julia-pkg` (package manager),
  `julia-test` (test explorer),
  `julia-console` (REPL).
- External bundles: run as out-of-process commands that speak a capped
  JSON-RPC protocol (*decided, D3*); keeps the main process stable —
  the lesson from PyCharm's in-process plugin failure modes is not re-created.
  In-process Go `plugin` loading is out of scope for v1.

---

## 4. UI decisions

- Shell: **Wails v2** (stable, Go-centric desktop, webview) — *decided (D1)*.
  Frontend is replaceable because the engine owns the state; this deliberately
  mirrors the observed `frontend/backend.split` architecture.
- Frontend framework: **Svelte** — *decided (D1a)*. Wails v2 supports Svelte
  templates natively; small runtime bundle, compile-time reactivity.
- Editor: **Monaco Editor** — *decided (D2)*. Monaco ships its own editing
  engine; `internal/syntax` feeds it via a Monarch-like tokenizer bridge for
  tree-sitter highlights, plus `DocumentSymbol` soon; LSP features gated by
  capability flags. Monaco brings multi-cursor and a built-in diff view
  (reusable for the pkg diff view) out of the box.
- Fallback option (decision record ADR-002): pure-Go UI kit (Fyne) if
  webview/GPU dependency proves fragile on Windows; UI protocol unchanged.

---

## 5. Roadmap — agent work packages

Each WP: separate module directory, public API reviewed by another agent,
`go test ./...` green, no TODO-leaking interfaces. Agents do **not** commit
secrets; PR-sized commits; acceptance criteria are executable tests where
possible.

| WP | Scope | Key acceptance tests | Status |
|---|---|---|---|
| **M0 WP0** | Repo, `go.work`, CI (GitHub Actions: build+test+lint on 3 OS), AGENTS.md conventions, `proto/jide` skeleton | CI green; `go vet`/golangci clean | ✅ done (`cd2d37c`, `586b687`) |
| **M1 WP1** | `workspace` + `vfs`: open folder, detect Julia env (`Project.toml`), fs watching, snapshots | integration test on fixture repo incl. symlinked deps | ✅ done (`cccb1f1`) |
| **M1 WP2** | `syntax`: vendor tree-sitter-julia via `go-tree-sitter`; lexer-level highlighting API, `DocumentSymbol`, f-string parsing exploits (string interpolation tasks), fold ranges | golden parse-tree tests on corpus of 100 Julia files | ✅ done (`ea37112`) |
| **M1 WP3** | UI shell: Wails window, Svelte app skeleton (Vite build, typed stores), Monaco editor component wired to `syntax` highlights via Monarch bridge, editor tabs, file tree, status bar (Julia env picker) | manual smoke scripted via Wails; e2e open-edit-save file | ⬜ todo |
| **M2 WP4** | `lsp`: spawn/discover LanguageServer.jl via juliaup; LSP client; map diagnostics/hover/completion/references to UI; crash-restart w/ backoff | e2e with real Julia 1.10+ + LanguageServer.jl in CI (tolerance: skip if Julia missing) | ⬜ todo |
| **M2 WP5** | `index`: trigram search + symbol index; go-to-def fallback when LS warming up; rename (LSP) | bench: 10k-file workspace warm-up < 30 s | ⬜ todo |
| **M3 WP6** | `pkg`: envs list, add/remove/upgrade packages via Pkg subprocess, Manifest diff view, dependency graph | fixture env operations in temp dir; deterministic output parsing | ⬜ todo |
| **M4 WP7** | `console`: ConPTY/PTY REPL, multiple sessions per env, re-open sessions, command history | process lifecycle tests incl. kill/restart on both Windows ConPTY and Unix PTY | ⬜ todo |
| **M5 WP8** | `dap`: DAP client, adapter discovery of DebugAdapter.jl, breakpoints/stepping/stack panels with PyCharm's measured timeout policy (init 10 s, launch 10 s, attach 20 s, evaluate 30 s) as initial defaults; function-breakpoint support | mock-DAP goldens + optional real adapter e2e | ⬜ todo |
| **M6 WP9** | `runconf` + `testrun`: typed run configs (script/module/REPL), Test Explorer streaming `@testset` results, failure navigation | fixture project test runs across pass/fail/skipped | ⬜ todo |
| **M7 WP10** | polish: settings UI (JSON-Schema-driven), error reporter, crash logs, i18n-ready strings | settings round-trip tests | ⬜ todo |
| **M8 WP11** | packaging: native installers (Windows/nsis, macOS/dmg, Linux/AppImage), auto-update opt-out channel, code-signing pipeline doc | package smoke runs on CI artifacts | ⬜ todo |
| **M8 WP12** | docs: user guide, protocol reference, extension-author guide | docs build green | ⬜ todo |

**Agent operating rules (for the build agents):**
- Each WP owner passes ownership only when acceptance tests are green; other
  agents review public API diff before merge.
- All engine behavior must be testable headless without the UI (the protocol
  is the seam) — same discipline REA needs: evidence over claims.
- Bounded negative searches: if a Julia dependency is unresolvable (e.g. no
  Julia in CI), mark the test skipped and record in `docs/known-gaps.md`.

---

## 6. Risk register

| Risk | Mitigation |
|---|---|
| Julia LS startup latency (~10–40 s cold) | pre-warm on project open, progress indicator in status bar, index-based fallback nav while LS warming |
| Julia version fragmentation (1.10 LTS … current stable) | pin minimum Julia 1.10; juliaup-spawned; graceful capability degradation |
| DAP adapter availability/drift | mock-DAP goldens bound in CI; version negotiate per DebugAdapter.jl docs |
| Windows ConPTY instability | ConPTY isolated in `jlproc` behind interface; Unix PTY shares behavior with `creack/pty` |
| Wails webview GPU issues on some hardware | ADR-002 Fyne fallback; engine remains UI-agnostic |
| Agent-induced scope creep | WP ownership contract; public-API review gate; CI size budget for binaries |
| Secrets | no tokens in code; settings file storage sandboxed to user config dir |

---

## 7. Decisions

All resolved 2026-10-22 (expert group confirmed recommended defaults for D3–D5):

1. **D1 — UI stack: Wails v2** *(decided)*. Fyne remains fallback only
   (ADR-002). Svelte is the approved frontend framework within D1.
2. **D1a — Frontend framework: Svelte** *(decided)*. Typed via svelte-check,
   built by Vite into the Wails asset pipeline.
3. **D2 — Editor surface: Monaco** *(decided — CodeMirror 6 rejected)*.
   Trade-off accepted: larger bundle size in exchange for mature editing and
   built-in diff view.
4. **D3 — External plugin loading: out-of-process JSON-RPC only for v1**
   *(decided — recommended option accepted)*. In-process Go `plugin` libs are
   rejected for v1 (crash isolation + no Windows support). Not revisited
   before M4 unless an engine-level need requires it.
5. **D4 — Minimum Go version: 1.24** *(decided — recommended option
   accepted)*. Pinned in `go.mod` files and CI matrix at WP0.
6. **D5 — Default Julia channel: juliaup stable, minimum release 1.10 LTS**
   *(decided — recommended option accepted)*. IDE requires Julia ≥ 1.10;
   cap is juliaup stable current. LS/DAP feature assumptions documented per
   DAP in `internal/dap` docs.

---

## Appendix A — REA Evidence appendix

| Evidence ID | Operation | Subject | Notes |
|---|---|---|---|
| `ev_495920079440ccfaebc2c1606ff9cab89c88eccbdfcde3ce75811abc6540f181` | inspect_artifact | `lib/platform-loader.jar` | ZIP inventory, 129 entries, sha256 `5507d865…c586d` |
| `ev_9fa49520d70587b4ecd7aa728d133d42db41da3e0697b30ef0d42eac6fb9832d` | extract_artifact | `plugins/python-ce/lib/modules/intellij.python.community.jar` | module descriptor + class inventory |
| `ev_e32f69edbbeb0776fce0a058e2dce7ad74f33fe37670f6bf22fc8d0c1dc4c4be` | extract_artifact | `lib/intellij.pycharm.pro.jar` | `META-INF/PythonPlugin.xml` (1.4 MB), `pycharm-customization.xml` |
| `ev_3efb1b2cdd5f962a61aaa594e0a1e36f00a62a61d047c0ddbcf02ad74d135316` | extract_artifact | `lib/intellij.pycharm.community.jar` | `PyCharmCorePlugin.xml`, `pycharm-core.xml` |
| `ev_2b0dbd551b82b0d7e1cad0c47ed61de79783e175f582f4d11ddb9a1e87dd7555` | extract_artifact | `plugins/python-ce/lib/modules/intellij.python.ast.jar` | `PyAst*` hierarchy |
| `ev_6dbb8a24845d309f50b89d9347044028c2f0c08a7bab50ee8239b43c9230e490` | extract_artifact | `.../intellij.python.parser.jar` | fileType/lexer/parser classes |
| `ev_f997fa6f47eaf9002ca2c296c52f03f6819fcafd5b2f6121dbbf15dfcf59ede3` | extract_artifact | `.../intellij.python.syntax.core.jar` | formatter/commenter/docstrings |
| `ev_012b89d074021b91ea746aec6119719bf3b3bb5ffe30b6bd0513414ee8594298` | extract_artifact | `.../intellij.python.psi.jar` | `Pythonid.*` extension points |
| `ev_43aa9844adcddf9aec794ea7c73ebb39cd0ca53e51bc2c539d761a59114539f2` | extract_artifact | `.../intellij.python.psi.impl.jar` | 92 inspections/40 intentions/22 completions/9 stubIndex |
| `ev_742fe26fe4df7a1882468e774cd404421097989e67eb9e6a44bcf4e802e4767c` | extract_artifact | `.../intellij.python.community.impl.jar` | 7 112 entries; run/debug/pkg wiring |

Shell-observation labels (not REA evidence): `product-info.json`, `build.txt`,
`bin/` listing, lib jar size ranking, descriptor-to-jar mapping scan
(.NET ZIP reader), extension-name corpus counts from `PythonPlugin.xml`.

Evidence storage in this workspace: `evidence\*.json` (raw REA envelopes);
descriptors and class inventories kept under REA-managed extraction dirs in
`%LOCALAPPDATA%\Temp\rea-extracted-*`.
