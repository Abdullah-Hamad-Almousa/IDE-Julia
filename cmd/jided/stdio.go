package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/jide-dev/jide/internal/jlproc"
	jide "github.com/jide-dev/jide/proto/jide"
)

const maxStdioMessage = 16 << 20

func serveStdio(input io.Reader, output io.Writer) error {
	var writeMu sync.Mutex
	write := func(message any) error {
		data, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("jided: encode stdio message: %w", err)
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		if _, err := output.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("jided: write stdio message: %w", err)
		}
		return nil
	}
	dispatcher := NewDAPDispatcher(func(params jide.DAPEventParams) {
		writeNotification(write, jide.MethodDAPEvent, params)
	})
	defer dispatcher.Close()
	runRuntime := jlproc.Runtime{Executable: "julia"}
	if discovered, discoverErr := jlproc.NewDiscoverer().Discover(context.Background()); discoverErr == nil {
		runRuntime = *discovered
	}
	runDispatcher := NewRunDispatcher(runRuntime, func(method jide.Method, params any) {
		writeNotification(write, method, params)
	})
	defer runDispatcher.Close(context.Background())

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), maxStdioMessage)
	for scanner.Scan() {
		line := scanner.Bytes()
		var request jide.Request
		if err := json.Unmarshal(line, &request); err != nil {
			if writeErr := write(jide.NewErrorResponse(nil, jide.NewError(jide.CodeParseError, "invalid JSON request"))); writeErr != nil {
				return writeErr
			}
			continue
		}
		if request.JSONRPC != jide.Version || request.Method == "" {
			if request.IsNotification() {
				continue
			}
			id := request.ID
			if id == nil {
				id = new(jide.ID)
			}
			if err := write(jide.NewErrorResponse(id, jide.NewError(jide.CodeInvalidRequest, "invalid JSON-RPC request"))); err != nil {
				return err
			}
			continue
		}
		if !isDAPRequest(request.Method) && !isRunRequest(request.Method) {
			if request.IsNotification() {
				continue
			}
			if err := write(jide.NewErrorResponse(request.ID, jide.NewError(jide.CodeMethodNotFound, "method is not available on the stdio DAP endpoint"))); err != nil {
				return err
			}
			continue
		}
		var result any
		var err error
		if isDAPRequest(request.Method) {
			result, err = dispatcher.Handle(context.Background(), request.Method, request.Params)
		} else {
			result, err = runDispatcher.Handle(context.Background(), request.Method, request.Params)
		}
		if request.IsNotification() {
			continue
		}
		if err != nil {
			code := jide.CodeServerError
			if strings.HasPrefix(err.Error(), "jided: decode DAP parameters:") || strings.HasPrefix(err.Error(), "jided: decode run parameters:") {
				code = jide.CodeInvalidParams
			}
			if write(jide.NewErrorResponse(request.ID, jide.NewError(code, err.Error()))) != nil {
				return err
			}
			continue
		}
		response, err := jide.NewResultResponse(request.ID, result)
		if err != nil {
			return err
		}
		if err := write(response); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("jided: read stdio request: %w", err)
	}
	return nil
}

func isDAPRequest(method jide.Method) bool {
	info, ok := method.Info()
	return ok && strings.HasPrefix(string(method), "dap/") && info.Direction == jide.DirectionClientToServer
}

func isRunRequest(method jide.Method) bool {
	return method == jide.MethodRunExecute || method == jide.MethodRunTerminate || method == jide.MethodRunInput || method == jide.MethodTestRunExecute
}

func writeNotification(write func(any) error, method jide.Method, params any) {
	request, err := jide.NewRequest(nil, method, params)
	if err == nil {
		_ = write(request)
	}
}
