package analyzer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestCodexProtocolRoutesResponsesAndConcurrentServerRequests(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	protocol := newCodexProtocol(clientConn, clientConn, func(context.Context, string, json.RawMessage) (any, error) {
		close(requestStarted)
		<-releaseRequest
		return map[string]any{"success": true}, nil
	}, nil)
	protocol.start()
	server := bufio.NewReader(serverConn)

	initializeResult := make(chan error, 1)
	go func() {
		result, err := protocol.request(context.Background(), "initialize", map[string]any{"capabilities": map[string]any{"experimentalApi": true}})
		if err == nil && string(result) != `{"ok":true}` {
			err = errors.New("unexpected initialize response")
		}
		initializeResult <- err
	}()
	initialize, err := readProbeFrame(server)
	if err != nil {
		t.Fatal(err)
	}
	if string(initialize["method"]) != `"initialize"` || initialize["id"] == nil {
		t.Fatalf("initialize frame = %#v", initialize)
	}
	if err := writeProbeFrame(serverConn, map[string]any{"id": initialize["id"], "result": map[string]any{"ok": true}}); err != nil {
		t.Fatal(err)
	}
	if err := <-initializeResult; err != nil {
		t.Fatal(err)
	}

	response := make(chan json.RawMessage, 1)
	responseErr := make(chan error, 1)
	go func() {
		result, err := protocol.request(context.Background(), "turn/start", map[string]any{"threadId": "thread-1"})
		response <- result
		responseErr <- err
	}()
	turnRequest, err := readProbeFrame(server)
	if err != nil {
		t.Fatal(err)
	}
	if string(turnRequest["method"]) != `"turn/start"` {
		t.Fatalf("turn request = %#v", turnRequest)
	}
	if err := writeProbeFrame(serverConn, map[string]any{"id": "tool-request-1", "method": "item/tool/call", "params": map[string]any{"callId": "call-1"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("server request handler did not start")
	}
	if err := writeProbeFrame(serverConn, map[string]any{"id": turnRequest["id"], "result": map[string]any{"turnId": "turn-1"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-response:
		if string(got) != `{"turnId":"turn-1"}` {
			t.Fatalf("turn response = %s", got)
		}
		if err := <-responseErr; err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending response was blocked by a server request")
	}
	close(releaseRequest)
	toolResponse, err := readProbeFrame(server)
	if err != nil {
		t.Fatal(err)
	}
	if string(toolResponse["id"]) != `"tool-request-1"` {
		t.Fatalf("server request response id = %s", toolResponse["id"])
	}
	if string(toolResponse["result"]) != `{"success":true}` {
		t.Fatalf("server request result = %s", toolResponse["result"])
	}
}

func TestCodexProtocolRoutesUnknownNotificationsAndRejectsUnknownRequests(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	notifications := make(chan string, 1)
	protocol := newCodexProtocol(clientConn, clientConn, nil, func(_ context.Context, method string, _ json.RawMessage) {
		notifications <- method
	})
	protocol.start()
	if err := writeProbeFrame(serverConn, map[string]any{"method": "unknown/notification", "params": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	select {
	case method := <-notifications:
		if method != "unknown/notification" {
			t.Fatalf("notification method = %q", method)
		}
	case <-time.After(time.Second):
		t.Fatal("notification was not delivered")
	}
	if err := writeProbeFrame(serverConn, map[string]any{"id": 21, "method": "unknown/request", "params": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	frame, err := readProbeFrame(bufio.NewReader(serverConn))
	if err != nil {
		t.Fatal(err)
	}
	if frame["error"] == nil || frame["id"] == nil {
		t.Fatalf("unknown request response = %#v", frame)
	}
}

func TestCodexProtocolRejectsOversizedFramesAndWrites(t *testing.T) {
	protocol := newCodexProtocol(strings.NewReader(strings.Repeat("x", maxCodexFrameBytes+1)), io.Discard, nil, nil)
	protocol.start()
	select {
	case <-protocol.done:
		if !errors.Is(protocol.failure(), ErrCodexFrameTooLarge) {
			t.Fatalf("failure = %v, want oversized frame", protocol.failure())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("oversized frame did not stop the reader")
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	protocol = newCodexProtocol(clientConn, clientConn, nil, nil)
	if err := protocol.notify("large", strings.Repeat("x", maxCodexFrameBytes)); !errors.Is(err, ErrCodexFrameTooLarge) {
		t.Fatalf("oversized write error = %v", err)
	}
}

func TestCodexProtocolEOFFailsPendingRequest(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	protocol := newCodexProtocol(clientConn, clientConn, nil, nil)
	protocol.start()
	result := make(chan error, 1)
	go func() {
		_, err := protocol.request(context.Background(), "turn/start", map[string]any{})
		result <- err
	}()
	if _, err := readProbeFrame(bufio.NewReader(serverConn)); err != nil {
		t.Fatal(err)
	}
	_ = serverConn.Close()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("pending request completed without a response")
		}
	case <-time.After(time.Second):
		t.Fatal("pending request did not fail on EOF")
	}
	_ = clientConn.Close()
}

func TestCodexProtocolMatchesOutOfOrderResponsesByID(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	protocol := newCodexProtocol(clientConn, clientConn, nil, nil)
	protocol.start()
	type completed struct {
		method string
		result json.RawMessage
		err    error
	}
	completedRequests := make(chan completed, 2)
	for _, method := range []string{"first/request", "second/request"} {
		method := method
		go func() {
			result, err := protocol.request(context.Background(), method, map[string]any{})
			completedRequests <- completed{method: method, result: result, err: err}
		}()
	}
	server := bufio.NewReader(serverConn)
	first, err := readProbeFrame(server)
	if err != nil {
		t.Fatal(err)
	}
	second, err := readProbeFrame(server)
	if err != nil {
		t.Fatal(err)
	}
	firstOrder := "first"
	if string(first["method"]) == `"second/request"` {
		firstOrder = "second"
	}
	secondOrder := "first"
	if string(second["method"]) == `"second/request"` {
		secondOrder = "second"
	}
	if err := writeProbeFrame(serverConn, map[string]any{"id": second["id"], "result": map[string]any{"order": secondOrder}}); err != nil {
		t.Fatal(err)
	}
	if err := writeProbeFrame(serverConn, map[string]any{"id": first["id"], "result": map[string]any{"order": firstOrder}}); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]string)
	for range 2 {
		select {
		case result := <-completedRequests:
			if result.err != nil {
				t.Fatal(result.err)
			}
			var value map[string]string
			if err := json.Unmarshal(result.result, &value); err != nil {
				t.Fatal(err)
			}
			seen[result.method] = value["order"]
		case <-time.After(time.Second):
			t.Fatal("out-of-order responses did not complete both requests")
		}
	}
	if seen["first/request"] != "first" || seen["second/request"] != "second" {
		t.Fatalf("response mapping = %#v", seen)
	}
}

func TestCodexStderrTailBufferIsBounded(t *testing.T) {
	buffer := &tailBuffer{limit: 4}
	if _, err := buffer.Write([]byte("123456")); err != nil {
		t.Fatal(err)
	}
	if got := buffer.String(); got != "3456" {
		t.Fatalf("stderr tail = %q, want last four bytes", got)
	}
}

func readProbeFrame(reader *bufio.Reader) (map[string]json.RawMessage, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var frame map[string]json.RawMessage
	err = json.Unmarshal(line, &frame)
	return frame, err
}

func writeProbeFrame(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = writer.Write(data)
	return err
}
