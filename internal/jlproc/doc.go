// Package jlproc discovers the Julia runtime of the machine.
//
// The package is the first half of the Julia process supervisor of the
// engine. It resolves the runtime that the language server, the debugger and
// the REPL run on, and it reports the version of that runtime so that the
// engine can refuse a runtime that is too old.
//
// Decision D5 of the plan fixes the contract of the package: the runtime
// comes from juliaup, the stable channel is the default and Julia 1.10 is the
// oldest release that jide supports. A runtime that is older than the
// minimum is reported as a typed error instead of being used silently.
//
// The package is headless: it starts no user interface and it never blocks
// the engine longer than the probe of the runtime takes. The PTY console and
// the environment supervisor belong to the work package that owns them.
package jlproc
