package dap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const maxMessageSize = 16 << 20
const maxHeaderSize = 8 << 10

func writeMessage(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("dap: encode message: %w", err)
	}
	if len(data) > maxMessageSize {
		return fmt.Errorf("dap: message exceeds %d bytes", maxMessageSize)
	}
	header := []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data)))
	if err := writeFull(writer, header); err != nil {
		return err
	}
	return writeFull(writer, data)
}

func readMessage(reader *bufio.Reader) (json.RawMessage, error) {
	contentLength := -1
	headerSize := 0
	for {
		lineBytes, err := reader.ReadSlice('\n')
		if err != nil {
			if errors.Is(err, bufio.ErrBufferFull) || headerSize+len(lineBytes) > maxHeaderSize {
				return nil, fmt.Errorf("dap: message header exceeds %d bytes", maxHeaderSize)
			}
			return nil, err
		}
		headerSize += len(lineBytes)
		if headerSize > maxHeaderSize {
			return nil, fmt.Errorf("dap: message header exceeds %d bytes", maxHeaderSize)
		}
		line := strings.TrimRight(string(lineBytes), "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("dap: malformed header %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			if contentLength >= 0 {
				return nil, errors.New("dap: duplicate Content-Length header")
			}
			length, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || length < 0 || length > maxMessageSize {
				return nil, fmt.Errorf("dap: invalid Content-Length %q", strings.TrimSpace(value))
			}
			contentLength = length
		}
	}
	if contentLength < 0 {
		return nil, errors.New("dap: missing Content-Length header")
	}
	data := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}
	if !json.Valid(data) {
		return nil, errors.New("dap: invalid JSON message")
	}
	return bytes.Clone(data), nil
}

func writeFull(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if written < 0 || written > len(data) {
			return errors.New("dap: writer returned an invalid byte count")
		}
		data = data[written:]
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
