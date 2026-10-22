module github.com/jide-dev/jide/internal/workspace

go 1.24

require (
	github.com/jide-dev/jide/internal/vfs v0.0.0
	github.com/jide-dev/jide/proto/jide v0.0.0
	github.com/pelletier/go-toml/v2 v2.2.3
)

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
)

// The workspace at the repository root resolves the modules of this
// repository to the local checkout. The replace directives keep the module
// buildable on its own, outside of the workspace.
replace github.com/jide-dev/jide/internal/vfs => ../vfs

replace github.com/jide-dev/jide/proto/jide => ../../proto/jide
