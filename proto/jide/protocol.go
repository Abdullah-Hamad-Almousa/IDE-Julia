package jide

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// Version is the JSON-RPC version implemented by the jide protocol.
const Version = "2.0"

// IDKind classifies the JSON representation of a JSON-RPC identifier.
type IDKind string

const (
	// IDKindNull is the JSON null identifier. It is used by responses whose
	// request identifier could not be determined, for example when the
	// request could not be parsed.
	IDKindNull IDKind = "null"
	// IDKindString is a string identifier.
	IDKindString IDKind = "string"
	// IDKindNumber is a numeric identifier.
	IDKindNumber IDKind = "number"
)

// ID identifies a JSON-RPC request and correlates it with its response.
//
// The JSON-RPC 2.0 specification permits a string, a number or null as the
// identifier. ID keeps the original JSON representation so that a response
// can be matched back to its request without a lossy conversion. The zero
// value is the null identifier.
type ID struct {
	value any
}

// NewStringID returns a string identifier.
func NewStringID(v string) ID {
	return ID{value: v}
}

// NewNumberID returns a numeric identifier.
func NewNumberID(v int64) ID {
	return ID{value: v}
}

// Kind reports how the identifier is represented on the wire.
func (id ID) Kind() IDKind {
	switch id.value.(type) {
	case nil:
		return IDKindNull
	case string:
		return IDKindString
	case int64, float64:
		return IDKindNumber
	default:
		return IDKindNull
	}
}

// IsNull reports whether the identifier is the JSON null identifier.
func (id ID) IsNull() bool {
	return id.Kind() == IDKindNull
}

// String returns the textual form of the identifier. A null identifier is
// rendered as "null" and a numeric identifier as its decimal representation.
func (id ID) String() string {
	switch v := id.value.(type) {
	case nil:
		return "null"
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return "null"
	}
}

// MarshalJSON implements json.Marshaler.
func (id ID) MarshalJSON() ([]byte, error) {
	switch v := id.value.(type) {
	case nil:
		return []byte("null"), nil
	case string:
		return json.Marshal(v)
	case int64:
		return json.Marshal(v)
	case float64:
		return json.Marshal(v)
	default:
		return nil, fmt.Errorf("jide: cannot marshal identifier of type %T", id.value)
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (id *ID) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return errors.New("jide: empty identifier")
	}
	switch trimmed[0] {
	case 'n':
		if string(trimmed) != "null" {
			return fmt.Errorf("jide: invalid identifier %q", string(trimmed))
		}
		id.value = nil
		return nil
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return fmt.Errorf("jide: invalid string identifier: %w", err)
		}
		id.value = s
		return nil
	default:
		var num json.Number
		if err := json.Unmarshal(trimmed, &num); err != nil {
			return fmt.Errorf("jide: invalid identifier %q: %w", string(trimmed), err)
		}
		if i, err := num.Int64(); err == nil {
			id.value = i
			return nil
		}
		f, err := num.Float64()
		if err != nil {
			return fmt.Errorf("jide: invalid identifier %q: %w", string(trimmed), err)
		}
		id.value = f
		return nil
	}
}

// Request is a JSON-RPC 2.0 request message.
//
// A request without an identifier is a notification: the peer must not send a
// response for it.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *ID             `json:"id,omitempty"`
	Method  Method          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// NewRequest builds a request for the given method. The params value is
// marshaled with encoding/json; a nil params yields a request without a
// "params" member.
func NewRequest(id *ID, method Method, params any) (*Request, error) {
	req := &Request{JSONRPC: Version, ID: id, Method: method}
	if params == nil {
		return req, nil
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("jide: marshal params for %s: %w", method, err)
	}
	req.Params = raw
	return req, nil
}

// IsNotification reports whether the request expects no response.
func (r Request) IsNotification() bool {
	return r.ID == nil
}

// Response is a JSON-RPC 2.0 response message.
//
// A response carries exactly one of Result or Error. The "id" member is
// always present; it is null when the request identifier could not be
// determined.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *ID             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// NewResultResponse builds a successful response for id, marshaling result
// with encoding/json.
func NewResultResponse(id *ID, result any) (*Response, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("jide: marshal result: %w", err)
	}
	return &Response{JSONRPC: Version, ID: id, Result: raw}, nil
}

// NewErrorResponse builds a failed response for id.
func NewErrorResponse(id *ID, rpcErr *Error) *Response {
	return &Response{JSONRPC: Version, ID: id, Error: rpcErr}
}

// IsError reports whether the response carries an error object.
func (r Response) IsError() bool {
	return r.Error != nil
}

// Error is a JSON-RPC 2.0 error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes used by the jide protocol.
const (
	// CodeParseError indicates invalid JSON was received.
	CodeParseError = -32700
	// CodeInvalidRequest indicates the payload is not a valid request object.
	CodeInvalidRequest = -32600
	// CodeMethodNotFound indicates the requested method is unknown.
	CodeMethodNotFound = -32601
	// CodeInvalidParams indicates the method parameters are invalid.
	CodeInvalidParams = -32602
	// CodeInternalError indicates an internal error in the engine.
	CodeInternalError = -32603
	// CodeServerError is the base of the implementation defined server
	// error range (-32000 to -32099).
	CodeServerError = -32000
)

// NewError returns an error object with the given code and message.
func NewError(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return "jide: <nil rpc error>"
	}
	return fmt.Sprintf("jide: rpc error %d: %s", e.Code, e.Message)
}

// WithData returns a copy of the error with the given structured data
// attached to the "data" member.
func (e *Error) WithData(data any) (*Error, error) {
	if e == nil {
		return nil, errors.New("jide: cannot attach data to a nil error")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("jide: marshal error data: %w", err)
	}
	clone := *e
	clone.Data = raw
	return &clone, nil
}

// ErrorCodeNames maps the standard error codes to their symbolic names. The
// mapping is exported in the JSON Schema under "x-jide.errorCodes".
var ErrorCodeNames = map[string]int{
	"parseError":     CodeParseError,
	"invalidRequest": CodeInvalidRequest,
	"methodNotFound": CodeMethodNotFound,
	"invalidParams":  CodeInvalidParams,
	"internalError":  CodeInternalError,
	"serverError":    CodeServerError,
}
