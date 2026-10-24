module github.com/jide-dev/jide/internal/index

go 1.24

require (
	github.com/jide-dev/jide/internal/syntax v0.0.0
	github.com/jide-dev/jide/proto/jide v0.0.0
)

require (
	github.com/mattn/go-pointer v0.0.1 // indirect
	github.com/tree-sitter/go-tree-sitter v0.25.0 // indirect
)

replace github.com/jide-dev/jide/internal/syntax => ../syntax

replace github.com/jide-dev/jide/proto/jide => ../../proto/jide
