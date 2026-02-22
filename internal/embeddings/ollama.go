package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type OllamaClient struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{},
	}
}

type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embedResponse struct {
	Embedding []float64 `json:"embedding"`
}

func (c *OllamaClient) Embed(ctx context.Context, text string) ([]float64, error) {
	const maxRetries = 3
	backoff := 500 * time.Millisecond

	var lastErr error
	for attempt := range maxRetries {
		vector, err := c.embed(ctx, text)
		if err == nil {
			return vector, nil
		}
		lastErr = err
		log.Printf("ollama embed attempt %d/%d failed: %v – retrying in %v", attempt+1, maxRetries, err, backoff)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return nil, fmt.Errorf("ollama embed failed after %d attempts: %w", maxRetries, lastErr)
}

func (c *OllamaClient) embed(ctx context.Context, text string) ([]float64, error) {
	body, err := json.Marshal(embedRequest{
		Model:  c.model,
		Prompt: text,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: status %d", resp.StatusCode)
	}

	var result embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama: empty embedding")
	}

	return result.Embedding, nil
}
