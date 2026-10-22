package lsp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// maxMessageSize is the largest message that the client accepts. A message
// that is larger is a protocol error: the length header comes from the
// server and a broken server must not exhaust the memory of the engine.
const maxMessageSize = 64 << 20

// ErrMalformedFrame reports a message whose header cannot be read.
var ErrMalformedFrame = errors.New("lsp: the message header is malformed")

// frameWriter writes the messages of the base protocol.
//
// The base protocol frames every message with a "Content-Length" header
// that states the size of the body in bytes. The header is terminated by an
// empty line.
type frameWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// newFrameWriter returns a writer that frames the messages for w.
func newFrameWriter(w io.Writer) *frameWriter {
	return &frameWriter{w: w}
}

// write sends one message.
func (f *frameWriter) write(payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	header := "Content-Length: " + strconv.Itoa(len(payload)) + "\r\n\r\n"
	if _, err := io.WriteString(f.w, header); err != nil {
		return fmt.Errorf("lsp: write the frame header: %w", err)
	}
	if _, err := f.w.Write(payload); err != nil {
		return fmt.Errorf("lsp: write the frame: %w", err)
	}
	return nil
}

// frameReader reads the messages of the base protocol.
type frameReader struct {
	r *bufio.Reader
}

// newFrameReader returns a reader that reads the frames from r.
func newFrameReader(r io.Reader) *frameReader {
	return &frameReader{r: bufio.NewReader(r)}
}

// read returns the body of the next message.
//
// The reader reports io.EOF when the stream ends between two messages.
func (f *frameReader) read() ([]byte, error) {
	length := -1
	for {
		line, err := f.r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && strings.TrimSpace(line) == "" {
				return nil, io.EOF
			}
			return nil, err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			break
		}
		name, value, found := strings.Cut(trimmed, ":")
		if !found {
			return nil, fmt.Errorf("%w: the header %q has no value", ErrMalformedFrame, trimmed)
		}
		if !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		size, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("%w: the Content-Length %q is not a number", ErrMalformedFrame, value)
		}
		length = size
	}
	if length < 0 {
		return nil, fmt.Errorf("%w: the message has no Content-Length header", ErrMalformedFrame)
	}
	if length > maxMessageSize {
		return nil, fmt.Errorf("%w: the message of %d bytes is too large", ErrMalformedFrame, length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(f.r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
