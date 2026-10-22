// Package syntax parses Julia source with the Tree-sitter Julia grammar and
// exposes syntax trees, highlighting tokens, document symbols, diagnostics,
// and fold ranges. It has no dependency on the desktop UI. The grammar is C
// source, so builds require cgo and a C compiler (GCC or Clang).
package syntax
