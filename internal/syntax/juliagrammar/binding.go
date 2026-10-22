package juliagrammar

/*
#cgo CFLAGS: -std=c11 -I.
#include "julia.h"
*/
import "C" //nolint:gocritic // cgo requires this standalone pseudo-import; it is not unsafe.

import "unsafe" //nolint:gocritic // gocritic mistakes cgo's pseudo-import for a duplicate.

// Language returns the vendored Tree-sitter Julia grammar language handle.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_julia())
}
