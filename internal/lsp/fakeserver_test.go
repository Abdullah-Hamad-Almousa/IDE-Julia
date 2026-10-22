package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The tests of the package drive the client with a fake language server.
// The fake is a copy of the test binary: when it runs with the marker in
// its environment it speaks the protocol instead of running the tests.

const (
	// fakeServerEnv marks the test binary as the fake language server.
	fakeServerEnv = "JIDE_LSP_FAKE_SERVER"
	// fakeServerLogEnv names the file the fake server writes its trace to.
	fakeServerLogEnv = "JIDE_LSP_FAKE_LOG"
)

// TestMain turns the test binary into the fake language server when the
// environment asks for it.
func TestMain(m *testing.M) {
	if os.Getenv(fakeServerEnv) != "" {
		os.Exit(runFakeServer())
	}
	os.Exit(m.Run())
}

// runFakeServer answers the requests of the client.
//
// The server writes a line into its trace for every message it receives, so
// that the tests can assert the flow of the session.
func runFakeServer() int {
	logPath := os.Getenv(fakeServerLogEnv)
	appendTrace(logPath, "spawn")
	reader := newFrameReader(os.Stdin)
	writer := newFrameWriter(os.Stdout)
	for {
		payload, err := reader.read()
		if err != nil {
			return 0
		}
		var msg message
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}
		appendTrace(logPath, "message "+msg.Method)
		switch msg.Method {
		case "initialize":
			writeFakeResult(writer, msg.ID, initializeResult{
				ServerInfo: &serverInfo{Name: "fake-language-server", Version: "1.0.0"},
			})
		case "shutdown":
			writeFakeResult(writer, msg.ID, struct{}{})
		case "exit":
			return 0
		case "textDocument/didOpen":
			writeFakeDiagnostics(writer, "file:///workspace/main.jl")
		case "textDocument/hover":
			writeFakeResult(writer, msg.ID, lspHover{
				Contents: json.RawMessage(`{"kind":"markdown","value":"the hover text"}`),
			})
		case "textDocument/completion":
			writeFakeResult(writer, msg.ID, lspCompletionList{
				Items: []lspCompletionItem{{Label: "alpha", Kind: 3, Detail: "the alpha function"}},
			})
		case "textDocument/references":
			writeFakeResult(writer, msg.ID, []lspLocation{{
				URI:   "file:///workspace/main.jl",
				Range: lspRange{Start: position{Line: 1}, End: position{Line: 1, Character: 4}},
			}})
		case "textDocument/definition":
			writeFakeResult(writer, msg.ID, []lspLocation{{
				URI:   "file:///workspace/other.jl",
				Range: lspRange{Start: position{Line: 2}, End: position{Line: 2, Character: 8}},
			}})
		}
	}
}

// writeFakeResult sends the answer of the fake server.
func writeFakeResult(writer *frameWriter, id json.RawMessage, result any) {
	raw, err := json.Marshal(result)
	if err != nil {
		return
	}
	payload, err := json.Marshal(outgoingResponse{JSONRPC: "2.0", ID: id, Result: raw})
	if err != nil {
		return
	}
	_ = writer.write(payload)
}

// writeFakeDiagnostics publishes a diagnostic for a document.
func writeFakeDiagnostics(writer *frameWriter, uri string) {
	raw, err := json.Marshal(publishDiagnosticsParams{
		URI: uri,
		Diagnostics: []lspDiagnostic{{
			Range:    lspRange{Start: position{Line: 0, Character: 0}, End: position{Line: 0, Character: 1}},
			Severity: 1,
			Source:   "fake",
			Message:  "undefined variable",
		}},
	})
	if err != nil {
		return
	}
	payload, err := json.Marshal(outgoingNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params:  raw,
	})
	if err != nil {
		return
	}
	_ = writer.write(payload)
}

// appendTrace adds a line to the trace of the fake server.
func appendTrace(path, line string) {
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintln(file, line)
}

// fakeServerCommand returns the command that starts the fake language
// server and the path of its trace.
func fakeServerCommand(t *testing.T, directory string) (command *Command, logPath string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	target := filepath.Join(directory, "fake-language-server")
	if runtime.GOOS == "windows" {
		target += ".exe"
	}
	if err := copyExecutable(executable, target); err != nil {
		t.Fatalf("copy the test binary to %q: %v", target, err)
	}
	logPath = filepath.Join(directory, "server.log")
	command = &Command{
		Executable: target,
		Args:       []string{"--fake-language-server"},
		Env: append(os.Environ(),
			fakeServerEnv+"=1",
			fakeServerLogEnv+"="+logPath,
		),
	}
	return command, logPath
}

// copyExecutable copies a file and keeps the executable bit.
func copyExecutable(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer output.Close()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Close()
}

// serverTrace returns the lines that the fake server wrote.
func serverTrace(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		t.Fatalf("read the trace of the fake server: %v", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// waitForTrace waits until the trace of the fake server holds the wanted
// number of entries that start with the prefix.
func waitForTrace(t *testing.T, path, prefix string, count int) []string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		trace := serverTrace(t, path)
		if countEntries(trace, prefix) >= count {
			return trace
		}
		if time.Now().After(deadline) {
			t.Fatalf("the trace of the fake server holds %d entries that start with %q, want %d: %v",
				countEntries(trace, prefix), prefix, count, trace)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// countEntries returns the number of lines of the trace that start with the
// prefix.
func countEntries(trace []string, prefix string) int {
	count := 0
	for _, line := range trace {
		if strings.HasPrefix(line, prefix) {
			count++
		}
	}
	return count
}
