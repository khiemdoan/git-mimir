package embedder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Embedder generates embeddings for text strings.
type Embedder interface {
	Embed(texts []string) ([][]float32, error)
}

// OllamaEmbedder calls a local Ollama server.
type OllamaEmbedder struct {
	BaseURL string
	Model   string
	client  *http.Client
}

func NewOllamaEmbedder() *OllamaEmbedder {
	base := os.Getenv("MIMIR_OLLAMA_URL")
	if base == "" {
		base = "http://localhost:11434"
	}
	model := os.Getenv("MIMIR_EMBED_MODEL")
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaEmbedder{
		BaseURL: base,
		Model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OllamaEmbedder) Embed(texts []string) ([][]float32, error) {
	type request struct {
		Model  string   `json:"model"`
		Input  []string `json:"input"`
	}
	type response struct {
		Embeddings [][]float32 `json:"embeddings"`
	}

	body, err := json.Marshal(request{Model: o.Model, Input: texts})
	if err != nil {
		return nil, err
	}

	resp, err := o.client.Post(o.BaseURL+"/api/embed", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed: status %d: %s", resp.StatusCode, b)
	}

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama decode: %w", err)
	}
	return result.Embeddings, nil
}

// OpenAIEmbedder calls the OpenAI embeddings API.
type OpenAIEmbedder struct {
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

func NewOpenAIEmbedder() *OpenAIEmbedder {
	base := os.Getenv("OPENAI_BASE_URL")
	if base == "" {
		base = "https://api.openai.com"
	}
	return &OpenAIEmbedder{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		Model:   "text-embedding-3-small",
		BaseURL: base,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OpenAIEmbedder) Embed(texts []string) ([][]float32, error) {
	type request struct {
		Input []string `json:"input"`
		Model string   `json:"model"`
	}
	type embeddingData struct {
		Embedding []float32 `json:"embedding"`
	}
	type response struct {
		Data []embeddingData `json:"data"`
	}

	body, err := json.Marshal(request{Input: texts, Model: o.Model})
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequest("POST", o.BaseURL+"/v1/embeddings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embed: status %d: %s", resp.StatusCode, b)
	}

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai decode: %w", err)
	}

	embeddings := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}
	return embeddings, nil
}

// NoopEmbedder returns zero vectors (for --skip-embeddings).
type NoopEmbedder struct {
	Dims int
}

func (n *NoopEmbedder) Embed(texts []string) ([][]float32, error) {
	dims := n.Dims
	if dims == 0 {
		dims = 384
	}
	result := make([][]float32, len(texts))
	for i := range result {
		result[i] = make([]float32, dims)
	}
	return result, nil
}

// NewEmbedder creates an Embedder based on MIMIR_EMBED_PROVIDER env var.
// Valid values: "ollama" (default), "openai", "noop".
func NewEmbedder() Embedder {
	provider := os.Getenv("MIMIR_EMBED_PROVIDER")
	switch provider {
	case "openai":
		return NewOpenAIEmbedder()
	case "noop", "none":
		return &NoopEmbedder{}
	default:
		return NewOllamaEmbedder()
	}
}
