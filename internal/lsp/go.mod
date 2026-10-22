module github.com/jide-dev/jide/internal/lsp

go 1.24

require (
	github.com/jide-dev/jide/internal/jlproc v0.0.0
	github.com/jide-dev/jide/proto/jide v0.0.0
)

replace github.com/jide-dev/jide/internal/jlproc => ../jlproc

// The workspace at the repository root resolves the modules of this
// repository to the local checkout. The replace directives keep the module
// buildable on its own, outside of the workspace.
replace github.com/jide-dev/jide/proto/jide => ../../proto/jide
