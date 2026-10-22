# The `jide://` protocol

> **Status: stub.** This page describes the envelope that `proto/jide`
> implements today. The method catalog is a starting point, not a frozen
> contract: it grows with the work packages that own the subsystems.

The source of truth for the protocol is the Go package
[`proto/jide`](../proto/jide):

| Concern | Where it lives |
|---|---|
| Request / response / error envelope | [`proto/jide/protocol.go`](../proto/jide/protocol.go) |
| Method names and catalog | [`proto/jide/methods.go`](../proto/jide/methods.go) |
| Parameter and result objects | [`proto/jide/payloads.go`](../proto/jide/payloads.go) |
| JSON Schema export | [`proto/jide/schema.go`](../proto/jide/schema.go) |

When this page and the Go package disagree, the Go package is correct and this
page is a bug. Change both in the same work package.

## Transport

`jide://` is a JSON-RPC 2.0 protocol. The canonical transport is a WebSocket
carrying the `jide` scheme, opened by the shell against the engine daemon.
The envelope is transport agnostic: the same messages travel over a pipe, a
Unix domain socket or a TCP connection without modification.

Framing, reconnection, authentication and the dispatch loop arrive with the
work packages that own them; WP0 only defines the messages.

## Envelope

A **request** is a JSON object:

```json
{
  "jsonrpc": "2.0",
  "id": "req-1",
  "method": "workspace/open",
  "params": { "path": "/home/user/project" }
}
```

* `jsonrpc` is always the string `"2.0"`.
* `id` is optional. A request **without** an `id` is a *notification*: the
  peer must not answer it. When present, `id` is a string, a number or
  `null`.
* `method` is a namespaced method name, `namespace/verb`, for example
  `workspace/open` or `document/didChange`.
* `params` is optional and is always a JSON object.

A **response** is a JSON object:

```json
{ "jsonrpc": "2.0", "id": "req-1", "result": { "workspaceId": "w1", "root": "/home/user/project" } }
```

```json
{ "jsonrpc": "2.0", "id": "req-1", "error": { "code": -32601, "message": "unknown method" } }
```

* `id` is always present. It is `null` when the request identifier could not
  be determined, for example when the request could not be parsed.
* Exactly one of `result` and `error` is set.

## Error codes

The standard JSON-RPC 2.0 codes are used:

| Code | Name | Meaning |
|---|---|---|
| `-32700` | `parseError` | invalid JSON was received |
| `-32600` | `invalidRequest` | the payload is not a valid request object |
| `-32601` | `methodNotFound` | the requested method is unknown |
| `-32602` | `invalidParams` | the method parameters are invalid |
| `-32603` | `internalError` | an internal error in the engine |
| `-32000` | `serverError` | base of the implementation defined range (`-32000`…`-32099`) |

The `error` object may carry an optional `data` member with a structured
payload of any shape.

## Method catalog

The catalog is defined in `proto/jide/methods.go` and exported in the JSON
Schema. Each entry records the direction that may initiate the call
(`clientToServer` — the shell calls the engine, `serverToClient` — the engine
calls the shell, `bidirectional`) and the name of its parameter object.

| Method | Direction | Purpose |
|---|---|---|
| `ping` | bidirectional | liveness probe |
| `workspace/open` | client → server | open a workspace folder |
| `workspace/close` | client → server | close a workspace |
| `workspace/state` | client → server | state of the open workspace |
| `document/open` | client → server | open a document for editing |
| `document/close` | client → server | close a document |
| `document/didChange` | client → server | notify the engine of an edit |
| `document/didSave` | client → server | notify the engine of a save |
| `index/search` | client → server | search the workspace text index |
| `index/symbols` | client → server | symbols of a document |
| `lsp/status` | client → server | state of the Julia language server |
| `lsp/diagnostics` | server → client | publish diagnostics |
| `console/open` | client → server | open a REPL session |
| `console/input` | client → server | send input to a REPL session |
| `console/output` | server → client | publish REPL output |
| `run/execute` | client → server | start a run configuration |
| `run/terminate` | client → server | stop a running process |
| `dap/breakpoints` | server → client | publish the breakpoint set |
| `dap/stackTrace` | server → client | publish a stack trace |
| `testrun/execute` | client → server | start a test run |
| `extension/list` | client → server | list the registered extension bundles |

## JSON Schema

`proto/jide` exports the schema of the envelope, the parameter and result
objects and the method catalog:

```go
import "github.com/jide-dev/jide/proto/jide"

raw, err := jide.JSONSchema()      // []byte, valid JSON
err = jide.WriteJSONSchema(os.Stdout)
```

The exported document uses JSON Schema draft 2020-12. The top-level keys are
`$schema`, `$id`, `title`, `description`, `type`, `required`,
`additionalProperties`, `properties`, `$defs` and `x-jide`. The `x-jide`
extension carries the protocol version, the transport name, the method
catalog and the error code table.
