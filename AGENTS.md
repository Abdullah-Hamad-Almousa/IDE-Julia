# AGENTS.md — conventions for agents and humans working on `jide`

`jide` is a Julia IDE written in Go. `Plan.md` at the repository root is the
authoritative plan: it holds the architecture, the roadmap (work packages,
"WPs") and the resolved decisions D1–D5. Read the WP table in `Plan.md` §5
before you start, and read the section of `Plan.md` that owns your WP.

This file describes *how* to work in this repository. It is binding for every
agent that touches the tree.

---

## 1. Repository layout

The repository is a Go multi-module monorepo coordinated by a `go.work` file
at the root.

```
jide/
  go.work                    workspace file: lists every module
  .golangci.yml              lint configuration (shared by all modules)
  .github/workflows/ci.yml   build + test + vet + lint on ubuntu/macos/windows
  AGENTS.md                  this file
  Plan.md                    the plan (owned by the orchestrator)
  cmd/jided/                 engine daemon entry point   (module)
  cmd/jide/                  desktop shell entry point    (module)
  proto/jide/                JSON-RPC 2.0 protocol        (module)
  internal/workspace/        project model, Project.toml / Manifest.toml
  internal/vfs/              file watching, snapshots, save pipeline
  internal/syntax/           tree-sitter-julia parsing
  internal/index/            trigram text index, symbol index
  internal/ext/              extension-point registry
  internal/lsp/              LanguageServer.jl client
  internal/dap/              DebugAdapter.jl client
  internal/jlproc/           juliaup discovery, env supervisor, PTY console
  internal/pkg/              Pkg integration model
  internal/runconf/          run-configuration registry
  internal/testrun/          @testset runner
  ui/                        Svelte + TypeScript + Monaco frontend (Wails assets)
  docs/                      protocol reference, known gaps, ADRs
```

Module path convention: `github.com/jide-dev/jide/<relative path>`. The
`proto/jide` module is therefore imported as
`github.com/jide-dev/jide/proto/jide` and its package name is `jide`.

---

## 2. Adding a new module

1. Create the directory and its `go.mod`. Every module declares **Go 1.24**
   (decision D4) and uses the `github.com/jide-dev/jide/...` path:

   ```go
   module github.com/jide-dev/jide/internal/example

   go 1.24
   ```

2. Register the module in the workspace file at the repository root, keeping
   the list sorted:

   ```
   go 1.24

   use (
       ./cmd/jide
       ./cmd/jided
       ./internal/example
       ./proto/jide
   )
   ```

3. If the new module imports another module of this repository, add a
   `require` plus a `replace` with a relative path so that the module also
   builds outside of the workspace:

   ```
   require github.com/jide-dev/jide/proto/jide v0.0.0

   replace github.com/jide-dev/jide/proto/jide => ../../proto/jide
   ```

4. Add the module to `.github/workflows/ci.yml`: to the `module` matrix of the
   `build` job, to the `module` matrix of the `lint` job and to the explicit
   package patterns of the `workspace` job. CI then really builds, tests and
   lints the new module.
5. Run `go work sync` and commit the resulting `go.work` / `go.mod` changes.

A new module is a public API change. Other agents review the public API diff
before the work package is handed over (`Plan.md` §5, "Agent operating rules").

---

## 3. Building, testing, vetting, linting

Run the commands **from the repository root** unless stated otherwise.

### Per module (always required)

A single `go build ./...` at the root only covers the module that contains the
root directory, so every module has to be checked explicitly. The CI matrix
does exactly this.

```bash
for module in proto/jide cmd/jided cmd/jide; do
  ( cd "$module" && go build ./... && go test ./... && go vet ./... ) || exit 1
done
```

### Workspace wide

`go build ./...` **cannot** be used at the repository root: the root is not a
module, so the pattern matches nothing and Go reports
`directory prefix . does not contain modules listed in go.work`. Address the
modules by their path prefix instead:

```bash
go work sync        # must succeed, no diff outside go.mod/go.sum
go list -m all      # lists the workspace modules
go build ./proto/jide/... ./cmd/jided/... ./cmd/jide/...
go test  ./proto/jide/... ./cmd/jided/... ./cmd/jide/...
go vet   ./proto/jide/... ./cmd/jided/... ./cmd/jide/...
gofmt -l .          # must print nothing
```

Add the new module to those patterns when you add it to the workspace.

### Lint

The lint configuration is `.golangci.yml` at the repository root and applies
to every module. Install the pinned version and run it per module:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
"$(go env GOPATH)/bin/golangci-lint" run ./...            # in the module dir
```

CI runs the same version through `golangci/golangci-lint-action`. Keep the
version in `.github/workflows/ci.yml` and the version you install locally in
sync, otherwise "works locally" means nothing.

`gofmt` and `go vet` must be clean; `golangci-lint` must report no findings.
Do not silence a finding with `//nolint` unless the finding is a false
positive, and say so in the commit message.

---

## 4. Commit conventions

* **Conventional Commits**: `type(scope): subject`, e.g.
  `chore(wp0): scaffold monorepo, go.work, CI, and proto/jide skeleton`,
  `feat(syntax): expose document symbols`, `fix(vfs): debounce save pipeline`.
* Allowed types: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `build`,
  `ci`, `chore`, `revert`.
* The scope is the work package or the subsystem it touches (`wp0`, `syntax`,
  `lsp`, `dap`, `ci`, ...).
* Subject line in the imperative mood, lower case, no trailing period, at
  most 72 characters.
* **One commit per work package.** Do not mix unrelated refactors into a WP
  commit and do not split a WP across several commits.
* Never commit generated binaries, IDE metadata, `node_modules`, or local
  environment files. `.gitignore` already covers them.
* The commit body explains *why*, not *what*; the diff already shows the
  *what*.

---

## 5. Secrets and sensitive data

* **Never read, copy, move or commit `auth.json`** or any other credential
  file. If a task seems to need one, stop and report the blocker.
* No tokens, API keys, passwords, private endpoints or personal paths in
  source code, tests, fixtures, documentation or commit messages.
* Settings that may hold user data are stored in the per-user configuration
  directory at runtime, never in the repository.
* CI needs no secrets: the workflow only runs `go build`, `go test`,
  `go vet` and `golangci-lint`. Keep it that way — no secret-bearing step,
  no `secrets.*` reference, no pull-request target in a fork.

---

## 6. Engine rules

* **The engine is headless.** Packages under `internal/` and the protocol
  module `proto/jide` must build and test without any UI, webview, windowing
  or GPU dependency. The `proto/jide` protocol is the seam between the engine
  and the shell; if a feature cannot be reached through the protocol, it
  cannot be tested headlessly and does not belong in the engine yet.
* **The shell owns no state.** The UI shell (`cmd/jide`, `ui/`) renders what
  the engine reports. Do not cache IDE state in the frontend.
* **No TODO-leaking public interfaces.** A public function, type or constant
  is either implemented or not exported. `TODO`, `FIXME` or `panic("not
  implemented")` inside exported API is not acceptable; a placeholder that
  must exist is documented in `docs/known-gaps.md` instead.
* **Tests are part of the deliverable.** Every work package ships with
  executable acceptance tests. A test that cannot run in the current
  environment (for example because Julia is not installed) is skipped with an
  explicit reason, and the gap is recorded in `docs/known-gaps.md`.
* **Protocol changes are schema changes.** When a method, a parameter object
  or a result object changes, update the Go types in `proto/jide`, the JSON
  Schema export and `docs/protocol.md` in the same work package.

---

## 7. Known gaps

Every unresolved dependency, skipped test or unimplemented acceptance
criterion is recorded in [`docs/known-gaps.md`](docs/known-gaps.md), one
section per work package, with:

* what is missing,
* why it could not be implemented or verified,
* what would close the gap.

Never hide a gap in a commit message alone; the file is the single place
where the state of the plan is tracked.

---

## 8. Definition of done for a work package

1. `go build ./...`, `go test ./...`, `go vet ./...` pass in every module
   and for the whole workspace.
2. `gofmt -l .` prints nothing and `golangci-lint run ./...` reports nothing.
3. The acceptance tests of the work package exist and are executable.
4. Public API changes are listed in the commit message for review.
5. `docs/known-gaps.md` and `docs/protocol.md` are updated where relevant.
6. Exactly one Conventional Commit for the work package.
