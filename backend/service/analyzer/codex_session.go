package analyzer

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Cold CLI startup can exceed ten seconds on a busy desktop. The caller's
// deadline still bounds startup and the remainder of the operation together.
const codexHandshakeTimeout = 30 * time.Second

type codexAppServerSession struct {
	process  *ownedCodexProcess
	protocol *codexProtocol
	cwd      string
}

func startCodexAppServer(ctx context.Context, executable string, args []string) (*codexAppServerSession, error) {
	cwd, err := newCodexWorkingDirectory()
	if err != nil {
		return nil, err
	}
	process, err := startOwnedCodexProcess(ctx, executable, args, cwd, nil)
	if err != nil {
		_ = os.RemoveAll(cwd)
		return nil, err
	}
	protocol := newCodexProtocol(process.stdout, process.stdin, nil, nil)
	protocol.start()
	session := &codexAppServerSession{process: process, protocol: protocol, cwd: cwd}
	handshakeCtx, cancel := context.WithTimeout(ctx, codexHandshakeTimeout)
	defer cancel()
	if _, err := protocol.request(handshakeCtx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "ai-disk-cleaner",
			"title":   "AI Disk Cleaner",
			"version": "1.0.0",
		},
		"capabilities": map[string]any{"experimentalApi": true},
	}); err != nil {
		session.close()
		return nil, fmt.Errorf("initialize Codex app-server: %w", err)
	}
	if err := protocol.notify("initialized", map[string]any{}); err != nil {
		session.close()
		return nil, fmt.Errorf("finish Codex app-server initialization: %w", err)
	}
	return session, nil
}

func (session *codexAppServerSession) close() {
	if session == nil || session.process == nil {
		return
	}
	_ = session.process.stdin.Close()
	waited := make(chan struct{})
	go func() {
		_ = session.process.wait()
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(codexInterruptGrace):
		_ = session.process.terminate()
		<-waited
	}
	_ = session.process.stdout.Close()
	_ = os.RemoveAll(session.cwd)
}
