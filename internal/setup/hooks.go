package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	// PreSearchHook is the mimir-pre-search.sh script
	PreSearchHook = `#!/usr/bin/env bash
# PreToolUse hook: intercepts grep/glob/bash tool calls
# Injects graph context for the search term into Claude's environment
# Exit 0 = allow tool to proceed (we only augment, never block)

INPUT=$(cat)
TOOL=$(echo "$INPUT" | jq -r '.tool_name // empty')
QUERY=""

case "$TOOL" in
  Bash)
    # Extract search term from grep/rg/find commands
    CMD=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
    if echo "$CMD" | grep -qE '(grep|rg|ripgrep|find|fd)\s'; then
      # Parse first non-flag argument as the search query
      QUERY=$(echo "$CMD" | grep -oP '(?<=(grep|rg)\s+(-\w+\s+)*)"\K[^"]+|(?<=(grep|rg)\s+(-\w+\s+)*)\S+' | head -1)
    fi
    ;;
  Glob)
    QUERY=$(echo "$INPUT" | jq -r '.tool_input.pattern // empty')
    ;;
esac

# If we extracted a query, call mimir for context and append to stdout
# Claude Code merges hook stdout into tool context
if [ -n "$QUERY" ] && [ ${#QUERY} -gt 2 ]; then
  CONTEXT=$(mimir query --json --quiet "$QUERY" 2>/dev/null)
  if [ -n "$CONTEXT" ] && [ "$CONTEXT" != "null" ]; then
    # Output as hookSpecificOutput — Claude Code surfaces this as context
    jq -n \
      --arg ctx "$CONTEXT" \
      '{
        hookSpecificOutput: {
          hookEventName: "PreToolUse",
          additionalContext: ("Graph context for search:\n" + $ctx)
        }
      }'
  fi
fi

exit 0
`

	// PostWriteHook is the mimir-post-write.sh script
	PostWriteHook = `#!/usr/bin/env bash
# PostToolUse hook: triggers incremental reindex after file writes or git commits
# Always async (fire-and-forget) — never blocks Claude

INPUT=$(cat)
TOOL=$(echo "$INPUT" | jq -r '.tool_name // empty')
REPO_DIR="${CLAUDE_PROJECT_DIR:-$(pwd)}"

should_reindex=false
changed_file=""

case "$TOOL" in
  Write|Edit|MultiEdit)
    # File was written — get the path
    changed_file=$(echo "$INPUT" | jq -r '.tool_input.file_path // .tool_input.path // empty')
    if [ -n "$changed_file" ]; then
      should_reindex=true
    fi
    ;;
  Bash)
    # Check if it was a git commit
    CMD=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
    if echo "$CMD" | grep -qE 'git\s+(commit|merge|rebase|cherry-pick)'; then
      should_reindex=true
    fi
    ;;
esac

if [ "$should_reindex" = true ]; then
  # Fire async incremental reindex — doesn't block Claude
  # Pass changed file hint for faster patch planning
  if [ -n "$changed_file" ]; then
    nohup mimir analyze --incremental --hint "$changed_file" \
      --repo "$REPO_DIR" \
      > /tmp/mimir-reindex.log 2>&1 &
  else
    nohup mimir analyze --incremental \
      --repo "$REPO_DIR" \
      > /tmp/mimir-reindex.log 2>&1 &
  fi
fi

exit 0
`
)

// HookSettings represents the .claude/settings.json hook configuration
type HookSettings struct {
	Hooks HookEvents `json:"hooks"`
}

// HookEvents defines PreToolUse and PostToolUse hook matchers
type HookEvents struct {
	PreToolUse  []HookMatcher `json:"PreToolUse,omitempty"`
	PostToolUse []HookMatcher `json:"PostToolUse,omitempty"`
}

// HookMatcher matches tool names and specifies hooks to run
type HookMatcher struct {
	Matcher string        `json:"matcher"`
	Hooks   []HookHandler `json:"hooks"`
}

// HookHandler defines a single hook handler
type HookHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Async   bool   `json:"async,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// InstallClaudeCodeHooks writes .claude/settings.json with mimir hooks
// and copies hook scripts to .claude/hooks/
func InstallClaudeCodeHooks(repoPath string) error {
	claudeDir := repoPath
	// Check if .claude exists, if not use repoPath/.claude
	claudeDir = repoPath
	if repoPath != "" {
		claudeDir = repoPath
	}

	// For global setup, we don't have a repo path - skip
	if claudeDir == "" {
		return nil
	}

	hooksDir := filepath.Join(claudeDir, ".claude", "hooks")

	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return err
	}

	// Write hook scripts
	scripts := map[string]string{
		"mimir-pre-search.sh":  PreSearchHook,
		"mimir-post-write.sh":  PostWriteHook,
	}
	for name, content := range scripts {
		path := filepath.Join(hooksDir, name)
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			return err
		}
	}

	// Build settings.json hook config
	settings := HookSettings{
		Hooks: HookEvents{
			PreToolUse: []HookMatcher{
				{
					Matcher: "Bash|Glob",
					Hooks: []HookHandler{{
						Type:    "command",
						Command: ".claude/hooks/mimir-pre-search.sh",
					}},
				},
			},
			PostToolUse: []HookMatcher{
				{
					Matcher: "Write|Edit|MultiEdit|Bash",
					Hooks: []HookHandler{{
						Type:    "command",
						Command: ".claude/hooks/mimir-post-write.sh",
						Async:   true, // fire-and-forget, never blocks Claude
					}},
				},
			},
		},
	}

	// Merge with existing settings.json (don't overwrite other hooks)
	settingsPath := filepath.Join(claudeDir, ".claude", "settings.json")
	return mergeSettings(settingsPath, settings)
}

// mergeSettings reads existing settings.json and adds mimir hooks without
// removing any existing hooks from other tools.
func mergeSettings(path string, newSettings HookSettings) error {
	existing := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &existing)
	}

	// Deep merge: hooks.PreToolUse → append mimir matchers only
	hooks, _ := existing["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	appendMatchers(hooks, "PreToolUse", newSettings.Hooks.PreToolUse)
	appendMatchers(hooks, "PostToolUse", newSettings.Hooks.PostToolUse)
	existing["hooks"] = hooks

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// appendMatchers appends new matchers to an existing hooks map
func appendMatchers(hooks map[string]any, key string, newMatchers []HookMatcher) {
	existing, ok := hooks[key].([]any)
	if !ok {
		existing = []any{}
	}

	// Convert new matchers to map representation
	for _, m := range newMatchers {
		matchMap := map[string]any{
			"matcher": m.Matcher,
			"hooks":   []any{},
		}
		for _, h := range m.Hooks {
			hookMap := map[string]any{
				"type":    h.Type,
				"command": h.Command,
			}
			if h.Async {
				hookMap["async"] = true
			}
			if h.Timeout > 0 {
				hookMap["timeout"] = h.Timeout
			}
			matchMap["hooks"] = append(matchMap["hooks"].([]any), hookMap)
		}
		existing = append(existing, matchMap)
	}

	hooks[key] = existing
}
