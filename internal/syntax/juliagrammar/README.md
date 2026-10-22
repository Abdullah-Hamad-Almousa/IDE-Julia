# Vendored Tree-sitter Julia grammar

This directory contains the generated C parser and external scanner from
[`tree-sitter-julia`](https://github.com/tree-sitter/tree-sitter-julia),
release `v0.25.0` (upstream commit
`e0f9dcd180fdcfcfa8d79a3531e11d99e79321d3`). The grammar is compiled through
the `github.com/tree-sitter/go-tree-sitter` Go bindings. `binding.go` exposes
the grammar's `TSLanguage` handle to that binding.

The C parser and scanner are distributed under the MIT license; see
[`LICENSE`](LICENSE). To update the grammar, copy `src/parser.c`,
`src/scanner.c`, and the headers under `src/tree_sitter/` from the selected
upstream release, update the version and commit above, and run the syntax
golden corpus and highlighting/symbol/folding tests.
