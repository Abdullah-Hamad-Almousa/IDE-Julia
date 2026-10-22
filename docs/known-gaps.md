# Known gaps

Every work package records what it could not finish, what it could not
verify, and what would close the gap. A gap is only closed when the missing
piece exists *and* is verified by an executable test.

Format: one section per work package, newest first within a milestone.

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
