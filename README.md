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

The project has completed **M1 WP3**: a Wails desktop shell with a Svelte and
Monaco editor, workspace explorer, tabs, save flow, syntax highlighting, and
Julia environment status picker. The daemon protocol dispatcher and Julia
runtime integration are still pending.

| Module | Path | Purpose |
|---|---|---|
| `proto/jide` | [`proto/jide`](proto/jide) | JSON-RPC 2.0 envelope, method catalog, JSON Schema export |
| `cmd/jided` | [`cmd/jided`](cmd/jided) | Engine daemon entry point (stub) |
| `cmd/jide` | [`cmd/jide`](cmd/jide) | Wails desktop shell and Go-to-UI bridge |
| `ui` | [`ui`](ui) | Svelte + TypeScript + Monaco frontend |

## Requirements

* Go 1.24 or newer (decision D4 in `Plan.md`).
* Node.js 24 and npm for building the frontend bundle.
* A platform C toolchain and Wails prerequisites. Linux builds need GTK 3,
  WebKit2GTK 4.1, and `pkg-config`; macOS needs Xcode command-line tools;
  Windows needs a C compiler and the WebView2 runtime.
* Julia 1.10 LTS or newer is not required for WP3. `juliaup` discovery and
  runtime features arrive in WP4.

## Building

```bash
go work sync
npm --prefix ui ci
npm --prefix ui run build

for module in proto/jide internal/syntax internal/vfs internal/workspace cmd/jided cmd/jide; do
  ( cd "$module" && go build ./... && go test ./... && go vet ./... ) || exit 1
done
```

Install the Wails v2 CLI and launch the desktop shell from the repository root:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
(cd cmd/jide && wails dev)
```

On Windows, run `.\run-jide.cmd` from PowerShell. It checks the
Go/Node/GCC prerequisites, installs the pinned Wails CLI if needed, and
launches the app. Run `.\run-jide.cmd -CheckOnly` to validate the toolchain
without opening a window.

## License

See the repository metadata. All rights reserved until a license is chosen.
