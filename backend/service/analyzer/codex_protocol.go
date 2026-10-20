package analyzer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

const (
	maxCodexFrameBytes        = 8 * 1024 * 1024
	maxCodexConcurrentRPCs    = 16
	maxCodexNotificationQueue = 256
	maxCodexNotificationBytes = 8 * 1024 * 1024
)

var ErrCodexFrameTooLarge = errors.New("Codex app-server frame exceeds 8 MiB")

type codexRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (err *codexRPCError) Error() string { return "Codex app-server rejected a request" }

type codexRPCFrame struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *codexRPCError  `json:"error"`
}

type codexPendingResponse struct {
	result json.RawMessage
	err    error
}

type codexNotification struct {
	method string
	params json.RawMessage
}

type codexProtocol struct {
	reader io.Reader
	writer io.Writer

	requestHandler      func(context.Context, string, json.RawMessage) (any, error)
	notificationHandler func(context.Context, string, json.RawMessage)

	writeMu           sync.Mutex
	mu                sync.Mutex
	pending           map[string]chan codexPendingResponse
	closed            bool
	failureErr        error
	nextID            atomic.Uint64
	closeOnce         sync.Once
	done              chan struct{}
	notifications     chan codexNotification
	requestSlots      chan struct{}
	notificationBytes int
}

func newCodexProtocol(
	reader io.Reader,
	writer io.Writer,
	requestHandler func(context.Context, string, json.RawMessage) (any, error),
	notificationHandler func(context.Context, string, json.RawMessage),
) *codexProtocol {
	return &codexProtocol{
		reader:              reader,
		writer:              writer,
		requestHandler:      requestHandler,
		notificationHandler: notificationHandler,
		pending:             make(map[string]chan codexPendingResponse),
		done:                make(chan struct{}),
		notifications:       make(chan codexNotification, maxCodexNotificationQueue),
		requestSlots:        make(chan struct{}, maxCodexConcurrentRPCs),
	}
}

func (protocol *codexProtocol) start() {
	go protocol.readLoop()
	go protocol.notificationLoop()
}

func (protocol *codexProtocol) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := protocol.nextID.Add(1)
	idBytes, _ := json.Marshal(id)
	key := string(idBytes)
	response := make(chan codexPendingResponse, 1)
	protocol.mu.Lock()
	if protocol.closed {
		err := protocol.failureErr
		protocol.mu.Unlock()
		return nil, err
	}
	protocol.pending[key] = response
	protocol.mu.Unlock()

	if err := protocol.write(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		protocol.removePending(key)
		return nil, err
	}
	select {
	case completed := <-response:
		return completed.result, completed.err
	case <-ctx.Done():
		protocol.removePending(key)
		return nil, ctx.Err()
	case <-protocol.done:
		protocol.removePending(key)
		return nil, protocol.failure()
	}
}

func (protocol *codexProtocol) notify(method string, params any) error {
	return protocol.write(map[string]any{"method": method, "params": params})
}

func (protocol *codexProtocol) write(value any) error {
	frame, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode Codex app-server frame: %w", err)
	}
	if len(frame) > maxCodexFrameBytes {
		return ErrCodexFrameTooLarge
	}
	frame = append(frame, '\n')
	protocol.writeMu.Lock()
	defer protocol.writeMu.Unlock()
	select {
	case <-protocol.done:
		return protocol.failure()
	default:
	}
	written, err := protocol.writer.Write(frame)
	if err != nil {
		return fmt.Errorf("write Codex app-server frame: %w", err)
	}
	if written != len(frame) {
		return io.ErrShortWrite
	}
	return nil
}

func (protocol *codexProtocol) readLoop() {
	reader := bufio.NewReaderSize(protocol.reader, 32*1024)
	for {
		line, err := readCodexFrame(reader)
		if err != nil {
			protocol.shutdown(err)
			return
		}
		var frame codexRPCFrame
		if err := json.Unmarshal(line, &frame); err != nil {
			protocol.shutdown(fmt.Errorf("decode Codex app-server frame: %w", err))
			return
		}
		if frame.Method == "" {
			protocol.routeResponse(frame)
			continue
		}
		if len(frame.ID) == 0 || string(frame.ID) == "null" {
			notification := codexNotification{method: frame.Method, params: frame.Params}
			protocol.mu.Lock()
			if protocol.notificationBytes+len(notification.params) > maxCodexNotificationBytes {
				protocol.mu.Unlock()
				protocol.shutdown(errors.New("Codex app-server notification queue exceeded its byte limit"))
				return
			}
			protocol.notificationBytes += len(notification.params)
			protocol.mu.Unlock()
			select {
			case protocol.notifications <- notification:
			case <-protocol.done:
				return
			default:
				protocol.mu.Lock()
				protocol.notificationBytes -= len(notification.params)
				protocol.mu.Unlock()
				protocol.shutdown(errors.New("Codex app-server notification queue is full"))
				return
			}
			continue
		}
		select {
		case protocol.requestSlots <- struct{}{}:
			go protocol.handleServerRequest(frame)
		case <-protocol.done:
			return
		default:
			_ = protocol.writeError(frame.ID, -32000, "too many concurrent Codex requests")
		}
	}
}

func readCodexFrame(reader *bufio.Reader) ([]byte, error) {
	line := make([]byte, 0, 4096)
	for {
		part, err := reader.ReadSlice('\n')
		total := len(line) + len(part)
		terminated := len(part) > 0 && part[len(part)-1] == '\n'
		if total > maxCodexFrameBytes+1 || (total > maxCodexFrameBytes && !terminated) {
			return nil, ErrCodexFrameTooLarge
		}
		line = append(line, part...)
		if err == nil {
			return line[:len(line)-1], nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(line) == 0 {
				return nil, io.EOF
			}
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
}

func (protocol *codexProtocol) routeResponse(frame codexRPCFrame) {
	if len(frame.ID) == 0 || string(frame.ID) == "null" {
		return
	}
	key := string(frame.ID)
	protocol.mu.Lock()
	response := protocol.pending[key]
	delete(protocol.pending, key)
	protocol.mu.Unlock()
	if response == nil {
		return
	}
	if frame.Error != nil {
		response <- codexPendingResponse{err: frame.Error}
		return
	}
	response <- codexPendingResponse{result: frame.Result}
}

func (protocol *codexProtocol) handleServerRequest(frame codexRPCFrame) {
	defer func() { <-protocol.requestSlots }()
	if protocol.requestHandler == nil {
		_ = protocol.writeError(frame.ID, -32601, "Codex request is not supported")
		return
	}
	var params json.RawMessage
	if len(frame.Params) != 0 {
		params = append(json.RawMessage(nil), frame.Params...)
	}
	result, err := protocol.requestHandler(context.Background(), frame.Method, params)
	if err != nil {
		_ = protocol.writeError(frame.ID, -32000, "Codex request was rejected")
		return
	}
	_ = protocol.write(map[string]any{"id": json.RawMessage(frame.ID), "result": result})
}

func (protocol *codexProtocol) writeError(id json.RawMessage, code int, message string) error {
	return protocol.write(map[string]any{
		"id":    json.RawMessage(id),
		"error": codexRPCError{Code: code, Message: message},
	})
}

func (protocol *codexProtocol) notificationLoop() {
	for {
		select {
		case notification := <-protocol.notifications:
			protocol.mu.Lock()
			protocol.notificationBytes -= len(notification.params)
			protocol.mu.Unlock()
			if protocol.notificationHandler != nil {
				protocol.notificationHandler(context.Background(), notification.method, notification.params)
			}
		case <-protocol.done:
			return
		}
	}
}

func (protocol *codexProtocol) removePending(key string) {
	protocol.mu.Lock()
	delete(protocol.pending, key)
	protocol.mu.Unlock()
}

func (protocol *codexProtocol) shutdown(err error) {
	protocol.closeOnce.Do(func() {
		if err == nil {
			err = io.EOF
		}
		protocol.mu.Lock()
		protocol.closed = true
		protocol.failureErr = err
		pending := protocol.pending
		protocol.pending = make(map[string]chan codexPendingResponse)
		protocol.mu.Unlock()
		close(protocol.done)
		for _, response := range pending {
			response <- codexPendingResponse{err: err}
		}
	})
}

func (protocol *codexProtocol) failure() error {
	protocol.mu.Lock()
	defer protocol.mu.Unlock()
	return protocol.failureErr
}
