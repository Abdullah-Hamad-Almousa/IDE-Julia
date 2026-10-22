# jide desktop frontend

The Svelte + TypeScript + Monaco source lives here. Vite emits the production
bundle to `cmd/jide/frontend/dist`, which is embedded in the Wails executable.
The generated bundle is checked in so `go build ./...` works directly from a
fresh checkout; CI rebuilds it and verifies that it matches the source.
During development Vite binds to `127.0.0.1:3173`; Wails auto-discovers that
server.

## Development

```powershell
npm ci
npm run check
npm run build
```

Run the desktop shell with the Wails v2 CLI from the repository root:

```powershell
(cd cmd/jide && wails dev)
```

On Windows, use the repository launcher instead:

```powershell
.\run-jide.cmd
```

## Desktop smoke check

1. Start `run-jide.cmd` on Windows, or run `wails dev` from `cmd/jide`, then
   open a Julia workspace.
2. Expand the explorer and open a `.jl` file; confirm the Julia tokens render
   with Tree-sitter colors and the tab becomes active.
3. Edit the buffer, save with **Ctrl+S** (or the Save button), then verify the
   text is written to disk and the unsaved marker clears.
4. Change the status-bar environment selector between the detected project and
   the default environment.

The Go tests exercise the same open-edit-save path headlessly. The visual
desktop smoke remains a manual check on a machine with Wails prerequisites.

The frontend uses the Wails Go bindings as its workspace/VFS/syntax bridge.
The file tree and editor mirror engine results; the Go services retain the
versioned document snapshots. Julia runtime selection is a UI scaffold until
WP4 adds `juliaup` discovery.
