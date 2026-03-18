package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// GetEditors returns the list of all supported editors
func GetEditors() []Editor {
	return Editors
}

// DetectBinary checks if a binary exists in PATH
func DetectBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Editor represents a code editor that supports MCP configuration
type Editor struct {
	Name       string
	ConfigPath func() string // returns absolute path to MCP config file
	ConfigKey  string        // JSON key where mcpServers lives
	Detect     func() bool   // returns true if editor is installed
}

// Editors is the registry of all supported editors
var Editors = []Editor{
	{
		Name: "Claude Code",
		ConfigPath: func() string {
			home, _ := os.UserHomeDir()
			if runtime.GOOS == "windows" {
				return filepath.Join(os.Getenv("APPDATA"), "Claude", "claude_desktop_config.json")
			}
			return filepath.Join(home, ".claude", "mcp.json")
		},
		ConfigKey: "mcpServers",
		Detect: func() bool {
			_, err := exec.LookPath("claude")
			return err == nil
		},
	},
	{
		Name: "Cursor",
		ConfigPath: func() string {
			home, _ := os.UserHomeDir()
			if runtime.GOOS == "windows" {
				return filepath.Join(os.Getenv("APPDATA"), "Cursor", "User", "mcp.json")
			}
			if runtime.GOOS == "darwin" {
				return filepath.Join(home, "Library", "Application Support", "Cursor", "User", "mcp.json")
			}
			return filepath.Join(home, ".config", "Cursor", "User", "mcp.json")
		},
		ConfigKey: "mcpServers",
		Detect: func() bool {
			_, err := exec.LookPath("cursor")
			return err == nil
		},
	},
	{
		Name: "Windsurf",
		ConfigPath: func() string {
			home, _ := os.UserHomeDir()
			if runtime.GOOS == "darwin" {
				return filepath.Join(home, "Library", "Application Support", "Windsurf", "User", "mcp.json")
			}
			return filepath.Join(home, ".config", "Windsurf", "User", "mcp.json")
		},
		ConfigKey: "mcpServers",
		Detect: func() bool {
			_, err := exec.LookPath("windsurf")
			return err == nil
		},
	},
	{
		Name: "OpenCode",
		ConfigPath: func() string {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, ".config", "opencode", "config.json")
		},
		ConfigKey: "mcp",
		Detect: func() bool {
			_, err := exec.LookPath("opencode")
			return err == nil
		},
	},
	{
		Name: "VS Code (Copilot)",
		ConfigPath: func() string {
			home, _ := os.UserHomeDir()
			if runtime.GOOS == "darwin" {
				return filepath.Join(home, "Library", "Application Support", "Code", "User", "settings.json")
			}
			return filepath.Join(home, ".config", "Code", "User", "settings.json")
		},
		ConfigKey: "github.copilot.mcpServers",
		Detect: func() bool {
			_, err := exec.LookPath("code")
			return err == nil
		},
	},
	{
		Name: "Antigravity",
		ConfigPath: func() string {
			home, _ := os.UserHomeDir()
			if runtime.GOOS == "darwin" {
				return filepath.Join(home, "Library", "Application Support", "Antigravity", "User", "settings.json")
			}
			return filepath.Join(home, ".config", "Antigravity", "User", "settings.json")
		},
		ConfigKey: "github.copilot.mcpServers",
		Detect: func() bool {
			if _, err := exec.LookPath("agy"); err == nil {
				return true
			}
			home, _ := os.UserHomeDir()
			_, err := os.Stat(filepath.Join(home, ".antigravity"))
			return err == nil
		},
	},
	{
		Name: "Zed",
		ConfigPath: func() string {
			home, _ := os.UserHomeDir()
			if runtime.GOOS == "darwin" {
				return filepath.Join(home, "Library", "Application Support", "Zed", "settings.json")
			}
			return filepath.Join(home, ".config", "zed", "settings.json")
		},
		ConfigKey: "context_servers",
		Detect: func() bool {
			_, err := exec.LookPath("zed")
			return err == nil
		},
	},
}
