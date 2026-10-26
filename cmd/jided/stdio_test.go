package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	jide "github.com/jide-dev/jide/proto/jide"
)

const mockDAPAdapterEnv = "JIDE_JIDED_MOCK_DAP_ADAPTER"

func TestMain(m *testing.M) {
	if os.Getenv(mockDAPAdapterEnv) == "1" {
		os.Exit(runMockDAPAdapter(os.Stdin, os.Stdout))
	}
	os.Exit(m.Run())
}

func TestStdioDAPDispatchLifecycle(t *testing.T) {
	t.Setenv(mockDAPAdapterEnv, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	start, err := jide.NewRequest(idForTest("start"), jide.MethodDAPStart, jide.DAPStartParams{
		Executable: executable, Arguments: []string{"-test.run=TestMockDAPAdapterProcess"},
	})
	if err != nil {
		t.Fatal(err)
	}
	launch, err := jide.NewRequest(idForTest("launch"), jide.MethodDAPLaunch, jide.DAPArgumentsParams{Arguments: map[string]any{"program": "demo.jl"}})
	if err != nil {
		t.Fatal(err)
	}
	setBreakpoints, err := jide.NewRequest(idForTest("breakpoints"), jide.MethodDAPSetBreakpoints, jide.DAPSetBreakpointsParams{
		Source: jide.DAPSource{Path: "demo.jl"}, Breakpoints: []jide.DAPSourceBreakpoint{{Line: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	configurationDone, err := jide.NewRequest(idForTest("configured"), jide.MethodDAPConfigurationDone, nil)
	if err != nil {
		t.Fatal(err)
	}
	threads, err := jide.NewRequest(idForTest("threads"), jide.MethodDAPThreads, jide.DAPThreadsParams{})
	if err != nil {
		t.Fatal(err)
	}
	disconnect, err := jide.NewRequest(idForTest("disconnect"), jide.MethodDAPDisconnect, jide.DAPDisconnectParams{})
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := jide.NewRequest(idForTest("unknown"), jide.MethodWorkspaceOpen, jide.WorkspaceOpenParams{Path: "unused"})
	if err != nil {
		t.Fatal(err)
	}
	var input bytes.Buffer
	for _, request := range []any{start, launch, setBreakpoints, configurationDone, threads, disconnect, unknown} {
		line, marshalErr := json.Marshal(request)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		input.Write(line)
		input.WriteByte('\n')
	}
	var output bytes.Buffer
	if err := runWithIO([]string{"--stdio"}, &input, &output); err != nil {
		t.Fatalf("runWithIO(--stdio) error = %v", err)
	}

	responses := make(map[string]jide.Response)
	var events []jide.DAPEventParams
	scanner := bufio.NewScanner(&output)
	for scanner.Scan() {
		var envelope struct {
			ID     *jide.ID            `json:"id"`
			Method jide.Method         `json:"method"`
			Params jide.DAPEventParams `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			t.Fatalf("decode response %q: %v", scanner.Text(), err)
		}
		if envelope.Method == jide.MethodDAPEvent {
			events = append(events, envelope.Params)
			continue
		}
		var response jide.Response
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil || response.ID == nil {
			t.Fatalf("decode JSON-RPC response %q: %v", scanner.Text(), err)
		}
		responses[response.ID.String()] = response
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(responses) != 7 {
		t.Fatalf("response count = %d, want 7: %s", len(responses), output.String())
	}
	var started jide.DAPStartResult
	if err := json.Unmarshal(responses["start"].Result, &started); err != nil || !started.Capabilities.SupportsFunctionBreakpoints {
		t.Fatalf("dap/start result = %+v, decode error = %v", started, err)
	}
	var listed jide.DAPThreadsResult
	if err := json.Unmarshal(responses["threads"].Result, &listed); err != nil || len(listed.Threads) != 0 {
		t.Fatalf("dap/threads result = %+v, decode error = %v", listed, err)
	}
	if responses["disconnect"].Error != nil {
		t.Fatalf("dap/disconnect error = %+v", responses["disconnect"].Error)
	}
	var verified jide.DAPBreakpointsResult
	if err := json.Unmarshal(responses["breakpoints"].Result, &verified); err != nil || len(verified.Breakpoints) != 1 || !verified.Breakpoints[0].Verified {
		t.Fatalf("dap/setBreakpoints result = %+v, decode error = %v", verified, err)
	}
	foundInitialized := false
	for _, event := range events {
		if event.Event == "initialized" {
			foundInitialized = true
		}
	}
	if !foundInitialized {
		t.Fatalf("adapter event was not forwarded over the protocol: %+v", events)
	}
	if errorObject := responses["unknown"].Error; errorObject == nil || errorObject.Code != jide.CodeMethodNotFound {
		t.Fatalf("unknown method response error = %+v, want methodNotFound", errorObject)
	}
}

func TestStdioRejectsMalformedAndInvalidDAPParameters(t *testing.T) {
	input := strings.NewReader("not-json\n{\"jsonrpc\":\"2.0\",\"id\":\"bad\",\"method\":\"dap/start\",\"params\":{\"executable\":42}}\n")
	var output bytes.Buffer
	if err := serveStdio(input, &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("error response count = %d, output = %q", len(lines), output.String())
	}
	var parseResponse, invalidParams jide.Response
	if err := json.Unmarshal([]byte(lines[0]), &parseResponse); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &invalidParams); err != nil {
		t.Fatal(err)
	}
	if parseResponse.Error == nil || parseResponse.Error.Code != jide.CodeParseError {
		t.Errorf("parse response error = %+v", parseResponse.Error)
	}
	if invalidParams.Error == nil || invalidParams.Error.Code != jide.CodeInvalidParams {
		t.Errorf("invalid params response error = %+v", invalidParams.Error)
	}
}

func TestMockDAPAdapterProcess(t *testing.T) {
	if os.Getenv(mockDAPAdapterEnv) != "1" {
		t.Skip("helper process for the stdio DAP dispatch test")
	}
}

func idForTest(value string) *jide.ID {
	id := jide.NewStringID(value)
	return &id
}

func runMockDAPAdapter(input io.Reader, output io.Writer) int {
	reader := bufio.NewReader(input)
	pendingLaunch := 0
	for {
		message, err := readMockDAPMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0
			}
			return 1
		}
		var request struct {
			Seq     int    `json:"seq"`
			Command string `json:"command"`
		}
		if json.Unmarshal(message, &request) != nil {
			return 2
		}
		var body any = map[string]any{}
		switch request.Command {
		case "initialize":
			body = map[string]any{"supportsFunctionBreakpoints": true, "supportsConfigurationDoneRequest": true}
		case "threads":
			body = map[string]any{"threads": []any{}}
		case "launch", "attach":
			pendingLaunch = request.Seq
			if writeMockDAPMessage(output, map[string]any{"seq": 501, "type": "event", "event": "initialized"}) != nil {
				return 4
			}
			continue
		case "setBreakpoints":
			body = map[string]any{"breakpoints": []any{map[string]any{"verified": true, "line": 2}}}
		}
		response := map[string]any{
			"seq": request.Seq + 100, "type": "response", "request_seq": request.Seq,
			"success": true, "command": request.Command, "body": body,
		}
		if writeMockDAPMessage(output, response) != nil {
			return 3
		}
		if request.Command == "configurationDone" && pendingLaunch != 0 {
			launchResponse := map[string]any{
				"seq": request.Seq + 101, "type": "response", "request_seq": pendingLaunch,
				"success": true, "command": "launch", "body": map[string]any{},
			}
			if writeMockDAPMessage(output, launchResponse) != nil {
				return 5
			}
			pendingLaunch = 0
		}
		if request.Command == "disconnect" {
			return 0
		}
	}
}

func readMockDAPMessage(reader *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(name, "Content-Length") {
			return nil, fmt.Errorf("invalid DAP header %q", line)
		}
		length, err = strconv.Atoi(strings.TrimSpace(value))
		if err != nil || length < 0 {
			return nil, fmt.Errorf("invalid DAP content length %q", value)
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("missing DAP content length")
	}
	message := make([]byte, length)
	_, err := io.ReadFull(reader, message)
	return message, err
}

func writeMockDAPMessage(writer io.Writer, value any) error {
	message, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n%s", len(message), message)
	return err
}
