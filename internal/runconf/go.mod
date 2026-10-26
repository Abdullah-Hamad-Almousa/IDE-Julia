module github.com/jide-dev/jide/internal/runconf

go 1.24

require github.com/jide-dev/jide/internal/jlproc v0.0.0

require (
	github.com/UserExistsError/conpty v0.1.4 // indirect
	github.com/creack/pty v1.1.24 // indirect
	golang.org/x/sys v0.30.0 // indirect
)

replace github.com/jide-dev/jide/internal/jlproc => ../jlproc
