module github.com/jide-dev/jide/cmd/jided

go 1.24

require github.com/jide-dev/jide/proto/jide v0.0.0

// The workspace at the repository root resolves github.com/jide-dev/jide/proto/jide
// to the local checkout. The replace directive keeps the module buildable on its
// own, outside of the workspace.
replace github.com/jide-dev/jide/proto/jide => ../../proto/jide
