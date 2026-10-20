package analyzer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

const (
	compatibleCodexVersion = "0.162.1"
	maxCodexStderrBytes    = 64 * 1024
)

var (
	ErrCodexExecutableNotFound = errors.New("Codex CLI executable not found")
	ErrCodexUnsupportedExec    = errors.New("Codex CLI executable wrapper is not supported")
	ErrCodexUnsafeMCPName      = errors.New("Codex MCP server name cannot be safely isolated")
	codexVersionPattern        = regexp.MustCompile(`(?:codex-cli|codex)\s+v?([0-9]+\.[0-9]+\.[0-9]+)`)
	codexMCPNamePattern        = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

type ownedCodexProcess struct {
	cmd          *exec.Cmd
	killTree     func() error
	releaseTree  func() error
	treeMu       sync.Mutex
	treeReleased bool
	stdin        io.WriteCloser
	stdout       io.ReadCloser
	stderr       *tailBuffer
	stderrDone   chan struct{}
	waitOnce     sync.Once
	waitErr      error
}

func parseCodexVersion(output string) (string, error) {
	match := codexVersionPattern.FindStringSubmatch(strings.TrimSpace(output))
	if len(match) != 2 {
		return "", fmt.Errorf("parse Codex CLI version: invalid version output")
	}
	version := match[1]
	if version != compatibleCodexVersion {
		return version, fmt.Errorf("unsupported Codex CLI version %s", version)
	}
	return version, nil
}

func isWindowsScriptWrapper(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".cmd" || extension == ".bat"
}

func resolveCodexExecutable(configured string) (string, error) {
	path := strings.TrimSpace(configured)
	if path == "" {
		resolved, err := exec.LookPath("codex")
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrCodexExecutableNotFound, err)
		}
		path = resolved
	} else if !filepath.IsAbs(path) {
		return "", fmt.Errorf("Codex executable must be an absolute path")
	}
	if runtime.GOOS == "windows" && isWindowsScriptWrapper(path) {
		return "", ErrCodexUnsupportedExec
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrCodexExecutableNotFound, path)
		}
		return "", fmt.Errorf("inspect Codex CLI executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("Codex executable is not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("Codex executable is not executable")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve Codex executable path: %w", err)
	}
	return absolute, nil
}

func codexIsolationArgs(mcpNames []string) ([]string, error) {
	args := []string{
		"--disable", "shell_tool",
		"--disable", "unified_exec",
		"--disable", "apps",
		"--disable", "plugins",
		"--disable", "multi_agent",
		"--disable", "multi_agent_v2",
		"--disable", "browser_use",
		"--disable", "browser_use_external",
		"--disable", "computer_use",
		"--enable", "code_mode_host",
		"--disable", "goals",
		"--disable", "default_mode_request_user_input",
		"--disable", "standalone_web_search",
		"--disable", "web_search_request",
		"--disable", "web_search_cached",
	}
	names := append([]string(nil), mcpNames...)
	sort.Strings(names)
	for _, name := range names {
		if !codexMCPNamePattern.MatchString(name) {
			return nil, ErrCodexUnsafeMCPName
		}
		args = append(args, "--config", "mcp_servers."+name+".enabled=false")
	}
	return append(args, "app-server", "--listen", "stdio://"), nil
}

func newCodexWorkingDirectory() (string, error) {
	directory, err := os.MkdirTemp("", "ai-disk-cleaner-codex-")
	if err != nil {
		return "", fmt.Errorf("create private Codex working directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		_ = os.RemoveAll(directory)
		return "", fmt.Errorf("secure Codex working directory: %w", err)
	}
	return directory, nil
}

func startOwnedCodexProcess(ctx context.Context, executable string, args []string, cwd string, extraEnv []string) (*ownedCodexProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command := exec.Command(executable, args...)
	command.Dir = cwd
	command.Env = append(os.Environ(), extraEnv...)
	configureCodexProcess(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open Codex stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("open Codex stdout: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("open Codex stderr: %w", err)
	}
	process := &ownedCodexProcess{
		cmd:        command,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     &tailBuffer{limit: maxCodexStderrBytes},
		stderrDone: make(chan struct{}),
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("start Codex CLI: %w", err)
	}
	killTree, releaseTree, err := attachCodexProcessTree(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("isolate Codex process tree: %w", err)
	}
	process.killTree = killTree
	process.releaseTree = releaseTree
	go func() {
		defer close(process.stderrDone)
		_, _ = io.Copy(process.stderr, stderr)
	}()
	return process, nil
}

func (process *ownedCodexProcess) wait() error {
	process.waitOnce.Do(func() {
		process.waitErr = process.cmd.Wait()
		_ = process.terminate()
		<-process.stderrDone
		if err := process.releaseProcessTree(); process.waitErr == nil {
			process.waitErr = err
		}
	})
	return process.waitErr
}

func (process *ownedCodexProcess) terminate() error {
	process.treeMu.Lock()
	defer process.treeMu.Unlock()
	if process.treeReleased || process.killTree == nil || process.cmd.Process == nil {
		return nil
	}
	if err := process.killTree(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func (process *ownedCodexProcess) releaseProcessTree() error {
	process.treeMu.Lock()
	defer process.treeMu.Unlock()
	if process.treeReleased {
		return nil
	}
	process.treeReleased = true
	if process.releaseTree == nil {
		return nil
	}
	return process.releaseTree()
}

type tailBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (buffer *tailBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	buffer.data = append(buffer.data, data...)
	if len(buffer.data) > buffer.limit {
		buffer.data = append([]byte(nil), buffer.data[len(buffer.data)-buffer.limit:]...)
	}
	return len(data), nil
}

func (buffer *tailBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return string(append([]byte(nil), buffer.data...))
}
