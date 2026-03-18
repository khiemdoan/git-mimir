package mcp

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	logger *log.Logger
	logMu  sync.Mutex
	logOnce sync.Once
)

// initLogger sets up the logger for the MCP server.
// Logs are written to ~/.mimir/mimir-mcp.log
func initLogger() {
	logOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			// Fallback to stderr
			logger = log.New(os.Stderr, "[MCP] ", log.LstdFlags|log.Lmicroseconds)
			return
		}

		logPath := filepath.Join(home, ".mimir", "mimir-mcp.log")

		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
			logger = log.New(os.Stderr, "[MCP] ", log.LstdFlags|log.Lmicroseconds)
			return
		}

		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			logger = log.New(os.Stderr, "[MCP] ", log.LstdFlags|log.Lmicroseconds)
			return
		}

		logger = log.New(f, "[MCP] ", log.LstdFlags|log.Lmicroseconds)
	})
}

// logRequest logs an incoming MCP request.
func logRequest(method string, id *json.RawMessage) {
	logMu.Lock()
	defer logMu.Unlock()
	initLogger()

	var idStr string
	if id != nil {
		idStr = fmt.Sprintf(" id=%s", string(*id))
	}
	logger.Printf("REQ%s method=%s", idStr, method)
}

// logResponse logs an MCP response.
func logResponse(method string, id *json.RawMessage, hasError bool) {
	logMu.Lock()
	defer logMu.Unlock()
	initLogger()

	var idStr string
	if id != nil {
		idStr = fmt.Sprintf(" id=%s", string(*id))
	}
	if hasError {
		logger.Printf("ERR%s method=%s", idStr, method)
	} else {
		logger.Printf("OK%s method=%s", idStr, method)
	}
}

// logToolCall logs a tool invocation.
func logToolCall(toolName string, repoName string) {
	logMu.Lock()
	defer logMu.Unlock()
	initLogger()

	if repoName != "" {
		logger.Printf("TOOL name=%s repo=%s", toolName, repoName)
	} else {
		logger.Printf("TOOL name=%s", toolName)
	}
}

// logToolResult logs a tool result with optional result size.
func logToolResult(toolName string, resultSize int, hasError bool) {
	logMu.Lock()
	defer logMu.Unlock()
	initLogger()

	if hasError {
		logger.Printf("TOOL_ERR name=%s size=%d", toolName, resultSize)
	} else {
		logger.Printf("TOOL_OK name=%s size=%d time=%v", toolName, resultSize, time.Since(startTimes[toolName]))
		delete(startTimes, toolName)
	}
}

// logError logs an error with context.
func logError(context string, err error) {
	logMu.Lock()
	defer logMu.Unlock()
	initLogger()

	logger.Printf("ERROR context=%s err=%v", context, err)
}

// logDebug logs a debug message.
func logDebug(format string, args ...interface{}) {
	logMu.Lock()
	defer logMu.Unlock()
	initLogger()

	logger.Printf("DEBUG "+format, args...)
}

// startTimes tracks when each tool call started for timing.
var startTimes = make(map[string]time.Time)

// logToolStart records the start time for a tool call.
func logToolStart(toolName string) {
	logMu.Lock()
	defer logMu.Unlock()
	initLogger()
	startTimes[toolName] = time.Now()
}
