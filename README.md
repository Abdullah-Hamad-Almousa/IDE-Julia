# jide — a Julia IDE written in Go

`jide` is a local, first-party-feeling IDE for the Julia programming
language. The engine is written in Go; the desktop shell is a Wails v2
application with a Svelte + TypeScript frontend and the Monaco editor.

The architecture is **protocol-first**: the engine daemon owns the workspace,
the indexes, the syntax services and the Julia runtime supervisor, and exposes
them over a documented JSON-RPC 2.0 protocol. The shell is a client of that
protocol, which keeps every engine behavior testable headlessly.

* Architecture, roadmap and decisions: [`Plan.md`](Plan.md)
* Agent and contributor conventions: [`AGENTS.md`](AGENTS.md)
* Protocol reference: [`docs/protocol.md`](docs/protocol.md)
* Open gaps and skipped work: [`docs/known-gaps.md`](docs/known-gaps.md)

## Status

The project is at **M0 WP0**: the repository, the Go workspace, the CI
pipeline and the protocol skeleton exist. The engine and the UI are not
implemented yet.

| Module | Path | Purpose |
|---|---|---|
| `proto/jide` | [`proto/jide`](proto/jide) | JSON-RPC 2.0 envelope, method catalog, JSON Schema export |
| `cmd/jided` | [`cmd/jided`](cmd/jided) | Engine daemon entry point (stub) |
| `cmd/jide` | [`cmd/jide`](cmd/jide) | Desktop shell entry point (stub) |

## Requirements

* Go 1.24 or newer (decision D4 in `Plan.md`).
* Julia 1.10 LTS or newer, installed through `juliaup` (decision D5). Not
  needed yet — nothing in the repository talks to Julia at this stage.

## Building

```bash
go work sync

for module in proto/jide cmd/jided cmd/jide; do
  ( cd "$module" && go build ./... && go test ./... && go vet ./... ) || exit 1
done
```

## License

See the repository metadata. All rights reserved until a license is chosen.
