package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StaticSkills are the 4 generic skills always installed
var StaticSkills = map[string]string{
	"exploring.md":      exploringSkill,
	"debugging.md":      debuggingSkill,
	"impact-analysis.md": impactSkill,
	"refactoring.md":    refactoringSkill,
}

// InstallStaticSkills writes the 4 core mimir skills to .claude/skills/mimir/
func InstallStaticSkills(repoPath string) error {
	dir := filepath.Join(repoPath, ".claude", "skills", "mimir")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for name, content := range StaticSkills {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

// InstallDynamicSkills generates one skill per cluster with repo-specific context
func InstallDynamicSkills(repoPath string, clusters []Community) error {
	dir := filepath.Join(repoPath, ".claude", "skills", "mimir", "modules")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, c := range clusters {
		if c.CohesionScore < 0.3 {
			continue // skip low-cohesion clusters — not meaningful modules
		}
		content := generateModuleSkill(c)
		name := sanitizeFilename(c.Label) + ".md"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

// Community represents a code cluster/module (mirrors store.Community)
type Community struct {
	ID            string
	Label         string
	CohesionScore float64
	Members       []string
}

func generateModuleSkill(c Community) string {
	return fmt.Sprintf(`---
name: mimir-module-%s
description: >
  Context for the %s module. Use when working in files: %s.
  Key entry points: %s. Cohesion score: %.2f.
---

# Module: %s

## Key files
%s

## Entry points
%s

## Known dependencies
Use mimir context tool to get live dependency info.
`, c.Label, c.Label, topFiles(c, 5), topEntryPoints(c, 3), c.CohesionScore,
		c.Label, fileList(c), entryPointList(c))
}

func topFiles(c Community, n int) string {
	if len(c.Members) == 0 {
		return "none"
	}
	end := n
	if end > len(c.Members) {
		end = len(c.Members)
	}
	var sb strings.Builder
	for i := 0; i < end; i++ {
		sb.WriteString(fmt.Sprintf("- %s\n", c.Members[i]))
	}
	return strings.TrimSpace(sb.String())
}

func topEntryPoints(c Community, n int) string {
	// For now, return first n members as entry points
	return topFiles(c, n)
}

func fileList(c Community) string {
	if len(c.Members) == 0 {
		return "No files in this module."
	}
	var sb strings.Builder
	for _, f := range c.Members {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	return strings.TrimSpace(sb.String())
}

func entryPointList(c Community) string {
	return topFiles(c, 3)
}

func sanitizeFilename(label string) string {
	// Replace invalid filename characters
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
		" ", "-",
	)
	return strings.ToLower(replacer.Replace(label))
}

const exploringSkill = `---
name: mimir-exploring
description: >
  Navigate unfamiliar code using the Mimir knowledge graph.
  Use when asked to explore a codebase, find where something is implemented,
  understand how a feature works, or trace a data flow.
---

## Prerequisite

**Ensure the MCP daemon is running** before using these tools:
` + "```bash" + `
mimir daemon start
` + "```" + `

## When to use mimir tools for exploration

1. Start with resources: read ` + "`mimir://repo/{name}/context`" + ` for codebase overview
2. Use ` + "`query()`" + ` tool for natural language search: "authentication flow", "database connection"
3. Use ` + "`context()`" + ` tool for 360-degree view of any symbol
4. Use ` + "`cypher()`" + ` tool for custom graph traversal if needed

## Exploration pattern

` + "```" + `
query("entry points for user authentication")
→ find relevant processes
→ context("handleLogin") for full call chain
→ follow outgoing calls to understand the flow
` + "```" + `
`

const debuggingSkill = `---
name: mimir-debugging
description: >
  Trace bugs through call chains using Mimir.
  Use when debugging an error, tracing unexpected behavior,
  or finding what calls a broken function.
---

## Prerequisite

**Ensure the MCP daemon is running** before using these tools:
` + "```bash" + `
mimir daemon start
` + "```" + `

## Debugging with graph context

1. ` + "`context(brokenFunction)`" + ` → see all callers
2. ` + "`impact(brokenFunction, \"upstream\")`" + ` → full blast radius
3. ` + "`query(errorMessage)`" + ` → find related code by semantic search
4. ` + "`detect_changes()`" + ` → check if recent changes caused the bug
`

const impactSkill = `---
name: mimir-impact
description: >
  Analyse blast radius before making any code change.
  Use before editing any function, class, or interface.
  Always run impact analysis before refactoring shared utilities.
---

## Prerequisite

**Ensure the MCP daemon is running** before using these tools:
` + "```bash" + `
mimir daemon start
` + "```" + `

## Pre-edit impact check pattern

BEFORE editing any symbol:
1. ` + "`impact(symbolName, \"upstream\", minConfidence: 0.7)`" + `
2. Review depth-1 callers (WILL BREAK)
3. Review depth-2 (LIKELY AFFECTED)
4. Inform user of risk level before proceeding
`

const refactoringSkill = `---
name: mimir-refactoring
description: >
  Plan safe refactors using dependency mapping.
  Use for renames, interface changes, module splits, or any
  structural change that affects multiple files.
---

## Prerequisite

**Ensure the MCP daemon is running** before using these tools:
` + "```bash" + `
mimir daemon start
` + "```" + `

## Safe refactor pattern

1. ` + "`impact(target, \"both\")`" + ` — understand full scope
2. ` + "`rename(old, new, dry_run: true)`" + ` — preview all affected files
3. Check ` + "`text_search_edits`" + ` (dynamic references, not in graph)
4. Execute in order: deepest dependents first, then the symbol itself
`
