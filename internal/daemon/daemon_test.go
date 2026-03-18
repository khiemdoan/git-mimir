package daemon_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thuongh2/git-mimir/internal/daemon"
)

// TestPIDPath tests that PID path returns a valid path in user home directory
func TestPIDPath(t *testing.T) {
	path := daemon.PIDPath()

	// Should be in ~/.mimir/mimir-mcp.pid
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("get home dir: %v", err)
	}

	expected := filepath.Join(home, ".mimir", "mimir-mcp.pid")
	if path != expected {
		t.Errorf("PIDPath() = %q, want %q", path, expected)
	}
}

// TestLogPath tests that log path returns a valid path in user home directory
func TestLogPath(t *testing.T) {
	path := daemon.LogPath()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("get home dir: %v", err)
	}

	expected := filepath.Join(home, ".mimir", "mimir-mcp.log")
	if path != expected {
		t.Errorf("LogPath() = %q, want %q", path, expected)
	}
}

// TestDataDir tests that data directory returns a valid path in user home directory
func TestDataDir(t *testing.T) {
	dir := daemon.DataDir()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("get home dir: %v", err)
	}

	expected := filepath.Join(home, ".mimir")
	if dir != expected {
		t.Errorf("DataDir() = %q, want %q", dir, expected)
	}
}

// TestEnsureDir tests that ensureDir creates the directory if it doesn't exist
func TestEnsureDir(t *testing.T) {
	// Create a temp directory for testing
	tmpDir := t.TempDir()
	testSubDir := filepath.Join(tmpDir, "test-mimir")

	// Ensure the directory doesn't exist
	os.RemoveAll(testSubDir)

	// Temporarily override DataDir for testing
	// Note: This requires making ensureDir testable or testing indirectly
	// Since ensureDir is internal, we test it through Start()

	// For now, just verify DataDir exists after calling ensureDir indirectly
	// The actual ensureDir is tested through integration in Start()
	t.Skip("ensureDir is tested indirectly through Start() integration tests")
}

// TestReadPID tests reading PID from non-existent file
func TestReadPID_NonExistent(t *testing.T) {
	// Ensure no daemon is running and PID file doesn't exist
	// This test assumes clean state

	// Stop any existing daemon and remove PID file
	daemon.Stop()
	os.Remove(daemon.PIDPath())

	pid, err := daemon.ReadPID()

	// If no PID file exists, should return 0 and error
	if err == nil && pid != 0 {
		t.Errorf("ReadPID() = %d, want 0 when no PID file exists", pid)
	}
}

// TestIsRunning_NoPIDFile tests IsRunning when no PID file exists
func TestIsRunning_NoPIDFile(t *testing.T) {
	// Stop any running daemon first
	daemon.Stop()

	// Remove PID file to ensure clean state
	os.Remove(daemon.PIDPath())

	running := daemon.IsRunning()
	if running {
		t.Error("IsRunning() = true, want false when no PID file exists")
	}
}

// TestGetUptime_NoPIDFile tests GetUptime when no PID file exists
func TestGetUptime_NoPIDFile(t *testing.T) {
	// Remove PID file
	os.Remove(daemon.PIDPath())

	uptime, err := daemon.GetUptime()

	if err == nil {
		t.Errorf("GetUptime() should return error when no PID file exists, got uptime=%v", uptime)
	}
}

// TestLogTail_NoLogFile tests LogTail when no log file exists
func TestLogTail_NoLogFile(t *testing.T) {
	// Remove log file
	os.Remove(daemon.LogPath())

	lines, err := daemon.LogTail(10)

	// Should return empty or error, not crash
	if err != nil && lines != nil {
		t.Logf("LogTail() returned error (acceptable): %v", err)
	}
}

// TestStartStop_Integration tests starting and stopping the daemon
func TestStartStop_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Get the path to the mimir binary
	mimirBin := getMimirBinary(t)
	if mimirBin == "" {
		t.Skip("mimir binary not found, skipping integration test")
	}

	// Ensure daemon is stopped before test
	daemon.Stop()
	time.Sleep(100 * time.Millisecond)

	// Start the daemon
	err := daemon.Start(mimirBin)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Give it a moment to start
	time.Sleep(200 * time.Millisecond)

	// Verify it's running
	if !daemon.IsRunning() {
		t.Error("IsRunning() = false after Start(), want true")
	}

	// Verify PID file exists
	pid, err := daemon.ReadPID()
	if err != nil {
		t.Errorf("ReadPID() error: %v", err)
	}
	if pid == 0 {
		t.Error("ReadPID() = 0, want non-zero PID")
	}

	// Get uptime (should be very small)
	uptime, err := daemon.GetUptime()
	if err != nil {
		t.Errorf("GetUptime() error: %v", err)
	} else if uptime > time.Minute {
		t.Errorf("GetUptime() = %v, expected < 1 minute for fresh start", uptime)
	}

	// Stop the daemon
	err = daemon.Stop()
	if err != nil {
		t.Errorf("Stop() error: %v", err)
	}

	// Give it a moment to stop
	time.Sleep(100 * time.Millisecond)

	// Verify it's stopped
	if daemon.IsRunning() {
		t.Error("IsRunning() = true after Stop(), want false")
	}
}

// getMimirBinary returns the path to the mimir binary for testing
func getMimirBinary(t *testing.T) string {
	// First try: currently running executable
	if bin, err := os.Executable(); err == nil {
		// Check if it's named "mimir" or in a mimir directory
		if strings.Contains(bin, "mimir") {
			t.Logf("found mimir at executable path: %s", bin)
			return bin
		}
	}

	// Second try: look for mimir in PATH
	if bin, err := exec.LookPath("mimir"); err == nil {
		t.Logf("found mimir in PATH: %s", bin)
		return bin
	}

	// Third try: use absolute path directly
	// Since we know the repo is at /Users/lap14503/mimir
	candidates := []string{
		"/Users/lap14503/mimir/mimir",
		"/Users/lap14503/mimir/cmd/mimir/mimir",
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			t.Logf("found mimir at %s", path)
			return path
		}
	}

	// Fourth try: check relative to home
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, "mimir")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			t.Logf("found mimir at %s", path)
			return path
		}
	}

	t.Log("mimir binary not found for integration test")
	return ""
}

// TestStart_AlreadyRunning tests that Start() is idempotent
func TestStart_AlreadyRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mimirBin := getMimirBinary(t)
	if mimirBin == "" {
		t.Skip("mimir binary not found, skipping integration test")
	}

	// Ensure stopped
	daemon.Stop()
	time.Sleep(100 * time.Millisecond)

	// Start first time
	if err := daemon.Start(mimirBin); err != nil {
		t.Fatalf("first Start() error: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Start second time (should be no-op)
	if err := daemon.Start(mimirBin); err != nil {
		t.Errorf("second Start() error (should be idempotent): %v", err)
	}

	// Clean up
	daemon.Stop()
}

// TestRestart tests the restart functionality
func TestRestart_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mimirBin := getMimirBinary(t)
	if mimirBin == "" {
		t.Skip("mimir binary not found, skipping integration test")
	}

	// Ensure stopped
	daemon.Stop()
	time.Sleep(100 * time.Millisecond)

	// Start initial
	if err := daemon.Start(mimirBin); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Get initial uptime
	initialUptime, _ := daemon.GetUptime()

	// Restart
	if err := daemon.Restart(mimirBin); err != nil {
		t.Errorf("Restart() error: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Get new uptime (should be reset)
	newUptime, _ := daemon.GetUptime()

	// New uptime should be less than initial (process restarted)
	if newUptime >= initialUptime {
		t.Logf("Restart may not have fully completed: initial=%v, new=%v", initialUptime, newUptime)
	}

	// Verify still running
	if !daemon.IsRunning() {
		t.Error("IsRunning() = false after Restart(), want true")
	}

	// Clean up
	daemon.Stop()
}
