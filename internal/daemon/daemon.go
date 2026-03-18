package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	pidFile  = "mimir-mcp.pid"
	logFile  = "mimir-mcp.log"
	dataDir  = ".mimir"
)

// PIDPath returns the path to the PID file in ~/.mimir/
func PIDPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, dataDir, pidFile)
}

// LogPath returns the path to the log file in ~/.mimir/
func LogPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, dataDir, logFile)
}

// DataDir returns the path to the ~/.mimir directory
func DataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, dataDir)
}

// ensureDir creates the ~/.mimir directory if it doesn't exist
func ensureDir() error {
	return os.MkdirAll(DataDir(), 0755)
}

// Start launches mimir mcp as a detached background process.
// Returns early if already running.
func Start(mimirBin string) error {
	if IsRunning() {
		return nil // already up
	}

	if err := ensureDir(); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	logF, err := os.OpenFile(LogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}

	cmd := exec.Command(mimirBin, "mcp")
	cmd.Stdout = logF
	cmd.Stderr = logF
	// Detach from parent process group so it survives terminal close
	cmd.SysProcAttr = sysProcDetach()

	if err := cmd.Start(); err != nil {
		logF.Close()
		return fmt.Errorf("start daemon: %w", err)
	}

	// Write PID
	pidStr := strconv.Itoa(cmd.Process.Pid)
	if err := os.WriteFile(PIDPath(), []byte(pidStr), 0644); err != nil {
		logF.Close()
		return fmt.Errorf("write pid: %w", err)
	}

	return nil
}

// Stop sends SIGINT to the daemon process.
func Stop() error {
	pid, err := readPID()
	if err != nil {
		return nil // not running
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(PIDPath())
		return nil
	}

	os.Remove(PIDPath())
	return proc.Signal(os.Interrupt)
}

// Restart stops and then starts the daemon.
func Restart(mimirBin string) error {
	Stop()
	// Give it a moment to fully stop
	time.Sleep(100 * time.Millisecond)
	return Start(mimirBin)
}

// IsRunning checks whether the daemon PID is alive.
// Cleans up stale PID file if process is not running.
func IsRunning() bool {
	pid, err := readPID()
	if err != nil {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(PIDPath()) // Clean up stale PID file
		return false
	}

	// Signal 0 = check existence only
	if proc.Signal(syscall.Signal(0)) != nil {
		os.Remove(PIDPath()) // Clean up stale PID file
		return false
	}
	return true
}

// WatchAndRestart keeps the daemon alive — call from a supervisor goroutine.
func WatchAndRestart(mimirBin string, interval time.Duration) {
	for {
		time.Sleep(interval)
		if !IsRunning() {
			if err := Start(mimirBin); err != nil {
				// Log error but keep trying
				fmt.Fprintf(os.Stderr, "daemon restart failed: %v\n", err)
			}
		}
	}
}

// ReadPID returns the current daemon PID.
func ReadPID() (int, error) {
	return readPID()
}

// GetUptime returns how long the daemon has been running.
func GetUptime() (time.Duration, error) {
	if _, err := readPID(); err != nil {
		return 0, fmt.Errorf("not running")
	}

	// Use PID file modification time as proxy for process start time
	if stat, err := os.Stat(PIDPath()); err == nil {
		return time.Since(stat.ModTime()), nil
	}

	return 0, fmt.Errorf("could not determine uptime")
}

// LogTail returns the last N lines of the daemon log.
func LogTail(lines int) ([]string, error) {
	logPath := LogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	allLines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(allLines) <= lines {
		return allLines, nil
	}

	return allLines[len(allLines)-lines:], nil
}

func readPID() (int, error) {
	b, err := os.ReadFile(PIDPath())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}
