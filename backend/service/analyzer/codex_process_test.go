package analyzer

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCodexProcessHelper(t *testing.T) {
	if os.Getenv("AI_DISK_CODEX_PROCESS_HELPER") != "1" {
		return
	}
	if os.Getenv("AI_DISK_CODEX_PROCESS_BLOCK") == "1" {
		select {}
	}
	_, _ = io.WriteString(os.Stdout, "CODEX_STDOUT_ONLY\n")
	_, _ = io.WriteString(os.Stderr, "CODEX_STDERR_ONLY\n")
	os.Exit(0)
}

func TestOwnedCodexProcessTerminatesOnlyItsTreeAndWaitsOnce(t *testing.T) {
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := newCodexWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(cwd) })
	process, err := startOwnedCodexProcess(context.Background(), source, []string{"-test.run=^TestCodexProcessHelper$"}, cwd, []string{"AI_DISK_CODEX_PROCESS_HELPER=1", "AI_DISK_CODEX_PROCESS_BLOCK=1"})
	if err != nil {
		t.Fatalf("startOwnedCodexProcess() error = %v", err)
	}
	if err := process.terminate(); err != nil {
		t.Fatalf("terminate() error = %v", err)
	}
	first := process.wait()
	if first == nil {
		t.Fatal("wait() error = nil after termination")
	}
	if second := process.wait(); second != first {
		t.Fatalf("second wait() = %v, first wait() = %v", second, first)
	}
}

func TestParseCodexVersionRequiresReviewedBaseline(t *testing.T) {
	for _, test := range []struct {
		output string
		want   string
		valid  bool
	}{
		{output: "codex-cli 0.162.1\n", want: "0.162.1", valid: true},
		{output: "codex-cli 0.162.2", want: "0.162.2", valid: false},
		{output: "not a Codex version", valid: false},
	} {
		got, err := parseCodexVersion(test.output)
		if test.valid && (err != nil || got != test.want) {
			t.Errorf("parseCodexVersion(%q) = %q, %v", test.output, got, err)
		}
		if !test.valid && err == nil {
			t.Errorf("parseCodexVersion(%q) error = nil", test.output)
		}
	}
}

func TestCodexExecutableResolutionSupportsSpacesAndMissingCLI(t *testing.T) {
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	spacedDir := filepath.Join(dir, "Codex CLI with spaces")
	if err := os.Mkdir(spacedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "codex helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	want := filepath.Join(spacedDir, name)
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, contents, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := resolveCodexExecutable(want)
	if err != nil || got != want {
		t.Fatalf("resolveCodexExecutable(spaced path) = %q, %v", got, err)
	}

	t.Setenv("PATH", t.TempDir())
	if _, err := resolveCodexExecutable(""); !errors.Is(err, ErrCodexExecutableNotFound) {
		t.Fatalf("resolveCodexExecutable(empty) error = %v, want not found", err)
	}
}

func TestCodexWindowsScriptWrappersAreRecognized(t *testing.T) {
	for _, path := range []string{`C:\Program Files\Codex\codex.cmd`, `C:\tools\codex.BAT`} {
		if !isWindowsScriptWrapper(path) {
			t.Errorf("isWindowsScriptWrapper(%q) = false", path)
		}
	}
	for _, path := range []string{`C:\Codex\codex.exe`, `/usr/local/bin/codex`} {
		if isWindowsScriptWrapper(path) {
			t.Errorf("isWindowsScriptWrapper(%q) = true", path)
		}
	}
}

func TestCodexIsolationArgsUseFixedArgvAndDisableMCPs(t *testing.T) {
	args, err := codexIsolationArgs([]string{"synthetic_probe"})
	if err != nil {
		t.Fatalf("codexIsolationArgs() error = %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--disable", "shell_tool", "apps", "plugins", "multi_agent", "mcp_servers.synthetic_probe.enabled=false", "app-server", "stdio://"} {
		if !strings.Contains(joined, want) {
			t.Errorf("arguments %q missing %q", joined, want)
		}
	}
	for _, want := range []string{"sh -c", "powershell -Command"} {
		if strings.Contains(strings.ToLower(joined), want) {
			t.Errorf("arguments unexpectedly contain shell command %q", want)
		}
	}
	if _, err := codexIsolationArgs([]string{"name.with.dot"}); err == nil {
		t.Fatal("codexIsolationArgs() accepted an unsafe config key")
	}
}

func TestOwnedCodexProcessKeepsStdoutAndStderrSeparate(t *testing.T) {
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := newCodexWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(cwd) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	process, err := startOwnedCodexProcess(ctx, source, []string{"-test.run=^TestCodexProcessHelper$"}, cwd, []string{"AI_DISK_CODEX_PROCESS_HELPER=1"})
	if err != nil {
		t.Fatalf("startOwnedCodexProcess() error = %v", err)
	}
	stdout, err := io.ReadAll(process.stdout)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := process.wait(); err != nil {
		t.Fatalf("wait() error = %v", err)
	}
	if string(stdout) != "CODEX_STDOUT_ONLY\n" || process.stderr.String() != "CODEX_STDERR_ONLY\n" {
		t.Fatalf("stdout=%q stderr=%q", stdout, process.stderr.String())
	}
}

func TestCodexIsolationRetainsDynamicToolHost(t *testing.T) {
	args, err := codexIsolationArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--enable code_mode_host") || strings.Contains(joined, "--disable code_mode_host") {
		t.Fatalf("dynamic tools need the code-mode host: %s", joined)
	}
}
