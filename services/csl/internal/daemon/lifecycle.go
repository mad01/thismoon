package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/mad01/thismoon/services/csl"
)

// DefaultSocketPath returns the search server's Unix socket inside csl's
// state directory.
func DefaultSocketPath() string {
	return statePath("search-daemon.sock")
}

// DefaultPIDPath returns the search server's PID file inside csl's state
// directory.
func DefaultPIDPath() string {
	return statePath("search-daemon.pid")
}

// DefaultLogPath returns the search server's rotated log inside csl's state
// directory.
func DefaultLogPath() string {
	return statePath(csl.DaemonLogFile)
}

// statePath resolves name under csl's state directory. These three paths are
// consumed as plain strings by a dozen call sites, so an unresolvable state
// directory yields "" rather than a signature change: opening an empty path
// fails, which is the same outcome as the missing directory would have been,
// and never writes a socket or PID file into the working directory.
func statePath(name string) string {
	path, err := csl.StatePath(name)
	if err != nil {
		return ""
	}
	return path
}

// WritePID writes the current process ID to the PID file.
func WritePID(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create pid directory: %w", err)
	}
	data := []byte(strconv.Itoa(os.Getpid()))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	return nil
}

// ReadPID reads the PID from file.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse pid: %w", err)
	}
	return pid, nil
}

// IsRunning checks if the process in the PID file is alive (signal 0).
func IsRunning(pidPath string) bool {
	pid, err := ReadPID(pidPath)
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks existence without actually sending a signal.
	return proc.Signal(syscall.Signal(0)) == nil
}

// RemoveStale removes stale socket and PID files if the daemon process is dead.
func RemoveStale(socketPath, pidPath string) error {
	if IsRunning(pidPath) {
		return nil
	}
	for _, p := range []string{socketPath, pidPath} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale file %s: %w", p, err)
		}
	}
	return nil
}

// StartBackground forks `csl search --serve` as a background process.
// Uses the same pattern as agent.go: exec.Command, Setpgid: true,
// stdout/stderr redirected to log file.
func StartBackground() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	logPath := DefaultLogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.Command(exePath, "search", "--serve")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start background daemon: %w", err)
	}

	// Detach — don't wait for the child.
	_ = logFile.Close()
	return nil
}
