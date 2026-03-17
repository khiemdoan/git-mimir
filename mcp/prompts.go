package mcp

import (
	"encoding/json"
)

// PromptDefinition describes one MCP prompt.
type PromptDefinition struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument is one argument for a prompt.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

func listPrompts() map[string]interface{} {
	return map[string]interface{}{
		"prompts": []PromptDefinition{
			{
				Name:        "detect_impact",
				Description: "Guide an agent through detect_changes → summarize risk and affected processes.",
				Arguments: []PromptArgument{
					{Name: "repo", Description: "Repository name", Required: false},
				},
			},
			{
				Name:        "generate_map",
				Description: "Read clusters + processes and generate a mermaid architecture diagram.",
				Arguments: []PromptArgument{
					{Name: "repo", Description: "Repository name", Required: false},
				},
			},
		},
	}
}

func getPrompt(params json.RawMessage) Response {
	var p struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	repo := p.Arguments["repo"]
	if repo == "" {
		repo = "<repo>"
	}

	switch p.Name {
	case "detect_impact":
		return Response{
			Result: map[string]interface{}{
				"messages": []map[string]interface{}{
					{
						"role": "user",
						"content": map[string]interface{}{
							"type": "text",
							"text": detectImpactPrompt(repo),
						},
					},
				},
			},
		}
	case "generate_map":
		return Response{
			Result: map[string]interface{}{
				"messages": []map[string]interface{}{
					{
						"role": "user",
						"content": map[string]interface{}{
							"type": "text",
							"text": generateMapPrompt(repo),
						},
					},
				},
			},
		}
	default:
		return errResp(ErrInvalidParams, "unknown prompt: "+p.Name)
	}
}

func detectImpactPrompt(repo string) string {
	return `You are a code impact analyst using Mimir.

Steps:
1. Call detect_changes(repo: "` + repo + `") to find recently changed files and symbols.
2. For each changed symbol, call impact(target: <symbol>, direction: "downstream") to understand blast radius.
3. Identify which processes (call chains) are affected.
4. Summarize: what changed, what is at risk, and what should be tested.

Focus on high-confidence edges (>0.8) and flag cross-community impacts as higher risk.`
}

func generateMapPrompt(repo string) string {
	return `You are a software architect using Mimir.

Steps:
1. Read mimir://repo/` + repo + `/clusters to get community clusters.
2. Read mimir://repo/` + repo + `/processes to get execution flows.
3. Generate a Mermaid architecture diagram showing:
   - Each cluster as a subgraph
   - Each process as a directed flow between clusters
   - Edge labels showing relationship types (CALLS, IMPORTS)

Output only valid Mermaid code inside a code block.`
}
