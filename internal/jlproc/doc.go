// Package jlproc discovers the Julia runtime of the machine.
//
// The package discovers the Julia runtime, reports its supported version, and
// supervises interactive REPL sessions over a platform PTY.
//
// Decision D5 of the plan fixes the contract of the package: the runtime
// comes from juliaup, the stable channel is the default and Julia 1.10 is the
// oldest release that jide supports. A runtime that is older than the
// minimum is reported as a typed error instead of being used silently.
//
// The package is headless: console output is delivered through callbacks, and
// every process lifecycle operation is cancellable.
package jlproc
