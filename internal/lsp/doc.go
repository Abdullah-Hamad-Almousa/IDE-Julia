// Package lsp drives the Julia language server over the Language Server
// Protocol 3.17.
//
// The package owns the lifecycle of the server process: it spawns the
// server, performs the initialize handshake, keeps the document
// synchronization in sync, answers the requests of the shell and restarts
// the server when it dies unexpectedly.
//
// The transport is the standard input and the standard output of the
// process, framed with the "Content-Length" header of the base protocol.
// The server is LanguageServer.jl, the semantic analysis of the Julia
// ecosystem; the package does not implement any Julia semantics itself.
//
// The package is headless: it never opens a window and it never depends on
// a user interface. Every behavior is reachable through the payload objects
// of the jide protocol, so that the engine can be tested without a GUI and
// without a Julia installation.
package lsp
