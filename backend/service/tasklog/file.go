package tasklog

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"
)

const timestampLayout = "2006-01-02 15:04:05.000"

func (service *Service) filename(start time.Time) (string, error) {
	if service == nil || service.executable == nil {
		return "", fmt.Errorf("task log service is not configured")
	}
	executable, err := service.executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(executable), "logs", start.Local().Format("2006-01-02_15-04-05.000")+".log"), nil
}

func tokenText(value *int64) string {
	if value == nil {
		return "未知"
	}
	return fmt.Sprint(*value)
}

func (session *Session) completeness() string {
	if session.incomplete {
		return "不完整"
	}
	return "完整"
}

func (session *Session) write(name, message string) {
	if session.closed || session.file == nil {
		return
	}
	message = strings.ReplaceAll(strings.ReplaceAll(message, "\r\n", "\n"), "\r", "\n")
	message = strings.ReplaceAll(message, "\n", "\n    ")
	line := fmt.Sprintf("%s [%s] %s\n", time.Now().Format(timestampLayout), name, message)
	n, err := io.WriteString(session.file, line)
	if err == nil && n != len(line) {
		err = io.ErrShortWrite
	}
	if err != nil {
		log.Printf("task log: write failed, disabling session: %v", err)
		session.closeFile()
	}
}

func (session *Session) closeFile() {
	if session.file != nil {
		if err := session.file.Close(); err != nil {
			log.Printf("task log: close failed: %v", err)
		}
		session.file = nil
	}
}

func (session *Session) close() {
	if session.closed {
		return
	}
	session.closeFile()
	session.closed = true
}
