package lsp

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

func TestFrameWriterAndReaderRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	writer := newFrameWriter(&buffer)
	payload := []byte(`{"jsonrpc":"2.0","method":"initialize"}`)
	if err := writer.write(payload); err != nil {
		t.Fatalf("write() error = %v", err)
	}
	expected := "Content-Length: " + strconv.Itoa(len(payload)) + "\r\n\r\n" + string(payload)
	if got := buffer.String(); got != expected {
		t.Errorf("the frame = %q, want %q", got, expected)
	}

	reader := newFrameReader(bytes.NewReader(buffer.Bytes()))
	read, err := reader.read()
	if err != nil {
		t.Fatalf("read() error = %v", err)
	}
	if !bytes.Equal(read, payload) {
		t.Errorf("the message = %q, want %q", string(read), string(payload))
	}
}

func TestFrameReaderReadsSeveralMessages(t *testing.T) {
	var buffer bytes.Buffer
	writer := newFrameWriter(&buffer)
	for _, payload := range []string{"first", "second", "third"} {
		if err := writer.write([]byte(payload)); err != nil {
			t.Fatalf("write() error = %v", err)
		}
	}
	reader := newFrameReader(bytes.NewReader(buffer.Bytes()))
	for _, want := range []string{"first", "second", "third"} {
		read, err := reader.read()
		if err != nil {
			t.Fatalf("read() error = %v", err)
		}
		if string(read) != want {
			t.Errorf("the message = %q, want %q", string(read), want)
		}
	}
	if _, err := reader.read(); !errors.Is(err, io.EOF) {
		t.Errorf("read() error = %v, want io.EOF", err)
	}
}

func TestFrameReaderKeepsTheBodyIntact(t *testing.T) {
	var buffer bytes.Buffer
	writer := newFrameWriter(&buffer)
	payload := []byte("{\"text\":\"a\r\nb\"}")
	if err := writer.write(payload); err != nil {
		t.Fatalf("write() error = %v", err)
	}
	reader := newFrameReader(bytes.NewReader(buffer.Bytes()))
	read, err := reader.read()
	if err != nil {
		t.Fatalf("read() error = %v", err)
	}
	if !bytes.Equal(read, payload) {
		t.Errorf("the message = %q, want %q", string(read), string(payload))
	}
}

func TestFrameReaderRejectsAMalformedHeader(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{name: "no length", input: "Content-Type: application/vscode-jsonrpc\r\n\r\n{}"},
		{name: "not a number", input: "Content-Length: many\r\n\r\n{}"},
		{name: "no value", input: "Content-Length\r\n\r\n{}"},
	}
	for _, testCase := range cases {
		reader := newFrameReader(strings.NewReader(testCase.input))
		if _, err := reader.read(); err == nil {
			t.Errorf("%s: read() error = nil, want an error", testCase.name)
		} else if !errors.Is(err, ErrMalformedFrame) {
			t.Errorf("%s: read() error = %v, want it to wrap ErrMalformedFrame", testCase.name, err)
		}
	}
}

func TestFrameReaderRejectsATooLargeMessage(t *testing.T) {
	input := "Content-Length: 999999999999\r\n\r\n"
	reader := newFrameReader(strings.NewReader(input))
	if _, err := reader.read(); err == nil {
		t.Fatal("read() error = nil, want an error")
	}
}
