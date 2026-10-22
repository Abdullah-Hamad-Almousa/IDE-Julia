package jide

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestRoundTrip(t *testing.T) {
	id := NewStringID("req-1")
	req, err := NewRequest(&id, MethodWorkspaceOpen, WorkspaceOpenParams{Path: "/tmp/project"})
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"jsonrpc":"2.0","id":"req-1","method":"workspace/open","params":{"path":"/tmp/project"}}`
	if got := string(raw); got != want {
		t.Errorf("request JSON = %s, want %s", got, want)
	}

	var decoded Request
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.JSONRPC != Version {
		t.Errorf("JSONRPC = %q, want %q", decoded.JSONRPC, Version)
	}
	if decoded.Method != MethodWorkspaceOpen {
		t.Errorf("Method = %q, want %q", decoded.Method, MethodWorkspaceOpen)
	}
	if decoded.ID == nil || decoded.ID.String() != "req-1" {
		t.Fatalf("ID = %v, want req-1", decoded.ID)
	}
	var params WorkspaceOpenParams
	if err := json.Unmarshal(decoded.Params, &params); err != nil {
		t.Fatalf("Unmarshal(params) error = %v", err)
	}
	if params.Path != "/tmp/project" {
		t.Errorf("params.Path = %q, want %q", params.Path, "/tmp/project")
	}
	if decoded.IsNotification() {
		t.Error("IsNotification() = true, want false")
	}
}

func TestRequestWithoutIDIsNotification(t *testing.T) {
	req, err := NewRequest(nil, MethodPing, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got := string(raw); strings.Contains(got, `"id"`) {
		t.Errorf("notification JSON = %s, want no id member", got)
	}
	if !req.IsNotification() {
		t.Error("IsNotification() = false, want true")
	}
}

func TestNewRequestMarshalError(t *testing.T) {
	if _, err := NewRequest(nil, MethodPing, make(chan int)); err == nil {
		t.Fatal("NewRequest() error = nil, want an error for an unmarshalable param")
	}
}

func TestResponseAlwaysCarriesID(t *testing.T) {
	resp := NewErrorResponse(nil, NewError(CodeMethodNotFound, "unknown method"))
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"jsonrpc":"2.0","id":null,"error":{"code":-32601,"message":"unknown method"}}`
	if got := string(raw); got != want {
		t.Errorf("response JSON = %s, want %s", got, want)
	}
	var decoded Response
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !decoded.IsError() {
		t.Error("IsError() = false, want true")
	}
	if decoded.Error == nil || decoded.Error.Code != CodeMethodNotFound {
		t.Fatalf("Error = %v, want code %d", decoded.Error, CodeMethodNotFound)
	}
}

func TestNewResultResponse(t *testing.T) {
	id := NewNumberID(7)
	resp, err := NewResultResponse(&id, WorkspaceOpenResult{WorkspaceID: "w1", Root: "/tmp/project"})
	if err != nil {
		t.Fatalf("NewResultResponse() error = %v", err)
	}
	if resp.IsError() {
		t.Error("IsError() = true, want false")
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded struct {
		Result WorkspaceOpenResult `json:"result"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.Result.WorkspaceID != "w1" {
		t.Errorf("result.workspaceId = %q, want %q", decoded.Result.WorkspaceID, "w1")
	}
}

func TestIDMarshaling(t *testing.T) {
	tests := []struct {
		name    string
		id      ID
		want    string
		kind    IDKind
		notNull bool
	}{
		{name: "string", id: NewStringID("abc"), want: `"abc"`, kind: IDKindString, notNull: true},
		{name: "number", id: NewNumberID(42), want: `42`, kind: IDKindNumber, notNull: true},
		{name: "null", id: ID{}, want: `null`, kind: IDKindNull, notNull: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(test.id)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if got := string(raw); got != test.want {
				t.Errorf("Marshal() = %s, want %s", got, test.want)
			}
			if got := test.id.Kind(); got != test.kind {
				t.Errorf("Kind() = %q, want %q", got, test.kind)
			}
			if got := test.id.IsNull(); got == test.notNull {
				t.Errorf("IsNull() = %v, want %v", got, !test.notNull)
			}

			var decoded ID
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if got := decoded.Kind(); got != test.kind {
				t.Errorf("decoded Kind() = %q, want %q", got, test.kind)
			}
		})
	}
}

func TestIDUnmarshalInvalid(t *testing.T) {
	for _, raw := range []string{`true`, `[]`, `"`, `nope`} {
		var id ID
		if err := json.Unmarshal([]byte(raw), &id); err == nil {
			t.Errorf("Unmarshal(%q) error = nil, want an error", raw)
		}
	}
}

func TestIDString(t *testing.T) {
	if got, want := NewStringID("x").String(), "x"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := NewNumberID(-3).String(), "-3"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := (ID{}).String(), "null"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestErrorMessage(t *testing.T) {
	err := NewError(CodeInvalidParams, "bad params")
	if got, want := err.Error(), "jide: rpc error -32602: bad params"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	var nilErr *Error
	if got, want := nilErr.Error(), "jide: <nil rpc error>"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrorWithData(t *testing.T) {
	base := NewError(CodeInvalidParams, "bad params")
	withData, err := base.WithData(map[string]int{"line": 3})
	if err != nil {
		t.Fatalf("WithData() error = %v", err)
	}
	if len(base.Data) != 0 {
		t.Error("WithData() mutated the receiver")
	}
	if !strings.Contains(string(withData.Data), `"line":3`) {
		t.Errorf("WithData() data = %s, want it to contain the line number", withData.Data)
	}
	if _, err := base.WithData(make(chan int)); err == nil {
		t.Error("WithData() error = nil, want an error for an unmarshalable value")
	}
	var nilErr *Error
	if _, err := nilErr.WithData("x"); err == nil {
		t.Error("WithData() on a nil error returned no error")
	}
}
