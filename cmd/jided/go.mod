module github.com/jide-dev/jide/cmd/jided

go 1.24

require (
	github.com/jide-dev/jide/internal/dap v0.0.0
	github.com/jide-dev/jide/internal/jlproc v0.0.0
	github.com/jide-dev/jide/internal/runconf v0.0.0
	github.com/jide-dev/jide/internal/testrun v0.0.0
	github.com/jide-dev/jide/proto/jide v0.0.0
)

require (
	github.com/UserExistsError/conpty v0.1.4 // indirect
	github.com/creack/pty v1.1.24 // indirect
	golang.org/x/sys v0.30.0 // indirect
)

// Local replace directives keep the protocol, DAP, and Julia-process modules
// buildable from this module outside of the workspace.
replace github.com/jide-dev/jide/proto/jide => ../../proto/jide

replace github.com/jide-dev/jide/internal/dap => ../../internal/dap

replace github.com/jide-dev/jide/internal/jlproc => ../../internal/jlproc

replace github.com/jide-dev/jide/internal/runconf => ../../internal/runconf

replace github.com/jide-dev/jide/internal/testrun => ../../internal/testrun
