// Package dap implements a headless Debug Adapter Protocol client for Julia's
// DebugAdapter.jl. It owns the adapter process, DAP framing, request timeouts,
// and typed debug operations; presentation and daemon dispatch are consumers.
package dap
