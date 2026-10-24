module github.com/jide-dev/jide/internal/pkg

go 1.24

require (
	github.com/jide-dev/jide/internal/jlproc v0.0.0
	github.com/jide-dev/jide/internal/workspace v0.0.0
)

require (
	github.com/UserExistsError/conpty v0.1.4 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/jide-dev/jide/internal/vfs v0.0.0 // indirect
	github.com/jide-dev/jide/proto/jide v0.0.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.3 // indirect
	golang.org/x/sys v0.30.0 // indirect
)

replace github.com/jide-dev/jide/internal/jlproc => ../jlproc

replace github.com/jide-dev/jide/internal/vfs => ../vfs

replace github.com/jide-dev/jide/internal/workspace => ../workspace

replace github.com/jide-dev/jide/proto/jide => ../../proto/jide
