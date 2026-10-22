// Package jide defines the wire protocol spoken between the jide UI shell and
// the jide engine daemon.
//
// # Transport
//
// The protocol is JSON-RPC 2.0. The canonical transport is a WebSocket bound
// to the "jide://" scheme, but the envelope defined here is transport
// agnostic: it can be framed over a pipe, a Unix domain socket or a TCP
// connection without any change to the message types.
//
// # Design rules
//
//   - The engine owns the state. The UI shell is a thin client, which keeps
//     every engine behavior testable headlessly (see the "protocol is the
//     seam" rule in AGENTS.md).
//   - The envelope is closed and typed. There is no free-form "registry key"
//     style extension point in v1; new capabilities are added as new methods
//     with a documented parameter and result object.
//   - This package has no dependency on any UI, editor or Julia runtime
//     library. It is pure data.
//
// # Envelope
//
// A request is a JSON object with "jsonrpc", "method" and optional "id" and
// "params" members. A request without an "id" member is a notification and
// must not be answered. A response is a JSON object with "jsonrpc", "id" and
// exactly one of "result" or "error".
//
// The JSON Schema for the envelope, the core payload objects and the method
// catalog is exported by JSONSchema.
package jide
