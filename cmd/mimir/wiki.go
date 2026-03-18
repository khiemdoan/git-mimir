package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/thuongh2/git-mimir/internal/registry"
	"github.com/thuongh2/git-mimir/internal/store"
)

func runWiki(cmd *cobra.Command, args []string) error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}

	var repoName string
	if len(args) > 0 {
		repoName = args[0]
	} else {
		repos := reg.List()
		if len(repos) == 1 {
			repoName = repos[0].Name
		} else if len(repos) == 0 {
			return fmt.Errorf("no repositories indexed")
		} else {
			return fmt.Errorf("multiple repos indexed; specify name: mimir wiki <name>")
		}
	}

	dbPath, err := registry.DBPath(repoName)
	if err != nil {
		return err
	}
	s, err := store.OpenStore(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	clusters, err := s.AllClusters()
	if err != nil {
		return err
	}
	processes, err := s.AllProcesses()
	if err != nil {
		return err
	}

	llmBaseURL := os.Getenv("MIMIR_LLM_BASE_URL")
	if llmBaseURL == "" {
		llmBaseURL = "http://localhost:11434/v1"
	}
	llmModel := os.Getenv("MIMIR_LLM_MODEL")
	if llmModel == "" {
		llmModel = "qwen2.5-coder:7b"
	}

	// Write wiki pages
	repoInfo := reg.Get(repoName)
	wikiDir := ".mimir/wiki"
	if repoInfo != nil {
		wikiDir = filepath.Join(repoInfo.Path, ".mimir", "wiki")
	}
	if err := os.MkdirAll(wikiDir, 0o755); err != nil {
		return err
	}

	var overview strings.Builder
	overview.WriteString(fmt.Sprintf("# %s — Architecture Wiki\n\n", repoName))
	overview.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02")))
	overview.WriteString("## Clusters\n\n")

	for _, c := range clusters {
		label := c.Label
		if label == "" {
			label = c.ID
		}
		overview.WriteString(fmt.Sprintf("- [%s](./%s.md) — %d symbols, cohesion: %.2f\n",
			label, c.ID, len(c.Members), c.CohesionScore))

		// Generate per-cluster wiki page
		prompt := fmt.Sprintf(
			"Write a concise technical wiki page for a code cluster named '%s' "+
				"containing %d symbols with cohesion score %.2f. "+
				"Explain its likely responsibilities and how it fits into the system. "+
				"Keep it under 300 words.",
			label, len(c.Members), c.CohesionScore)

		content, err := callLLM(llmBaseURL, llmModel, prompt)
		if err != nil {
			content = fmt.Sprintf("# %s\n\nMembers: %d\nCohesion: %.2f\n", label, len(c.Members), c.CohesionScore)
		}

		page := fmt.Sprintf("# %s\n\n**Cluster ID:** %s\n**Members:** %d\n**Cohesion:** %.2f\n\n%s\n",
			label, c.ID, len(c.Members), c.CohesionScore, content)
		pagePath := filepath.Join(wikiDir, c.ID+".md")
		if err := os.WriteFile(pagePath, []byte(page), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: write %s: %v\n", pagePath, err)
		}
		fmt.Printf("  Wrote %s\n", pagePath)
	}

	overview.WriteString("\n## Processes\n\n")
	for _, p := range processes {
		overview.WriteString(fmt.Sprintf("- **%s** (%s) — %d steps\n",
			p.Name, p.ProcessType, len(p.Steps)))
	}

	wikiPath := filepath.Join(wikiDir, "WIKI.md")
	if err := os.WriteFile(wikiPath, []byte(overview.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("Wiki generated: %s\n", wikiPath)
	return nil
}

func callLLM(baseURL, model, prompt string) (string, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type request struct {
		Model    string    `json:"model"`
		Messages []message `json:"messages"`
	}
	type choice struct {
		Message message `json:"message"`
	}
	type response struct {
		Choices []choice `json:"choices"`
	}

	body, _ := json.Marshal(request{
		Model:    model,
		Messages: []message{{Role: "user", Content: prompt}},
	})

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(baseURL+"/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result response
	if err := json.Unmarshal(b, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Choices[0].Message.Content, nil
}
