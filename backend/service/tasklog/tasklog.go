// Package tasklog writes human-readable diagnostic logs for cleaning tasks.
package tasklog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Service is assembled and owned by the root service manager.
type Service struct {
	executable func() (string, error)
}

// Session owns one task's output. Its zero value and nil pointer are safe no-ops.
type Session struct {
	// mu protects whole-event writes, usage totals and closing the file.
	mu         sync.Mutex
	file       io.WriteCloser
	started    time.Time
	total      int64
	incomplete bool
	closed     bool
}

func NewService() *Service { return &Service{executable: os.Executable} }

// Open never turns a logging failure into a task failure.
func (service *Service) Open(start time.Time, path string) *Session {
	session := &Session{started: start}
	filename, err := service.filename(start)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(filename), 0755)
	}
	if err == nil {
		var file *os.File
		file, err = os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			session.file = file
		}
	}
	if err != nil {
		log.Printf("task log: open failed: %v", err)
		return session
	}
	session.Event("任务开始", "开始时间 = %s; 扫描路径 = %s", start.Local().Format(timestampLayout), path)
	return session
}

// Remove treats an already missing file as successfully removed.
func (service *Service) Remove(start time.Time) error {
	filename, err := service.filename(start)
	if err != nil {
		return err
	}
	err = os.Remove(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (session *Session) Event(name, format string, args ...any) {
	if session == nil {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	session.write(name, fmt.Sprintf(format, args...))
}

// Round records only returned usage. nil means unknown, including a failed request.
func (session *Session) Round(turn int, elapsed time.Duration, input, output, total *int64, summary string) {
	if session == nil {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return
	}
	if total == nil {
		session.incomplete = true
	} else {
		session.total += *total
	}
	status := "完成"
	if summary != "" {
		status = "失败"
	}
	session.write(fmt.Sprintf("第 %d 轮 %s", turn, status), fmt.Sprintf(
		"耗时 = %.3fs; 输入 Token = %s; 输出 Token = %s; 本轮 Token = %s; 累计已知 Token = %d; 用量统计 = %s %s",
		elapsed.Seconds(), tokenText(input), tokenText(output), tokenText(total), session.total, session.completeness(), summary))
}

func (session *Session) ToolRequest(turn int, id, name, arguments string) {
	var formatted bytes.Buffer
	if json.Indent(&formatted, []byte(arguments), "", "  ") == nil {
		arguments = formatted.String()
	}
	session.Event("工具请求", "轮次 = %d; 调用 ID = %s; 工具 = %s\n参数：\n%s", turn, id, name, arguments)
}

// Finish writes the final outcome and closes exactly once.
func (session *Session) Finish(state, summary string) {
	if session == nil {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return
	}
	session.write("任务结束", fmt.Sprintf("状态 = %s; 累计已知 Token = %d; 用量统计 = %s; 总耗时 = %.3fs %s", state, session.total, session.completeness(), time.Since(session.started).Seconds(), summary))
	session.close()
}

// Close is the worker's fallback and does not invent a successful outcome.
func (session *Session) Close() {
	if session == nil {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	session.close()
}

// ErrorSummary avoids copying arbitrary response bodies or credentials from errors.
func ErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "任务已取消"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "请求超时"
	}
	var diagnostic interface{ LogSummary() string }
	if errors.As(err, &diagnostic) {
		return diagnostic.LogSummary()
	}
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return pathError.Error()
	}
	return fmt.Sprintf("操作失败（%T）", err)
}
