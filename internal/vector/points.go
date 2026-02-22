package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

func toUUID(id string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(id)).String()
}

type upsertRequest struct {
	Points []point `json:"points"`
}

type point struct {
	ID      string         `json:"id"`
	Vector  []float64      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

func (c *QdrantClient) Upsert(ctx context.Context, id string, vector []float64, payload map[string]any) error {
	body, err := json.Marshal(upsertRequest{
		Points: []point{{ID: toUUID(id), Vector: vector, Payload: payload}}, // ← toUUID
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/collections/%s/points", c.baseURL, c.collection),
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qdrant upsert: status %d", resp.StatusCode)
	}
	return nil
}

type SearchResult struct {
	ID      string         `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload"`
}

type searchRequest struct {
	Vector      []float64 `json:"vector"`
	Limit       int       `json:"limit"`
	WithPayload bool      `json:"with_payload"`
}

type searchResponse struct {
	Result []struct {
		ID      string         `json:"id"`
		Score   float64        `json:"score"`
		Payload map[string]any `json:"payload"`
	} `json:"result"`
}

func (c *QdrantClient) Search(ctx context.Context, vector []float64, limit int) ([]SearchResult, error) {
	body, err := json.Marshal(searchRequest{
		Vector:      vector,
		Limit:       limit,
		WithPayload: true,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/collections/%s/points/search", c.baseURL, c.collection),
		bytes.NewReader(body))
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
		return nil, fmt.Errorf("qdrant search: status %d", resp.StatusCode)
	}

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	out := make([]SearchResult, len(result.Result))
	for i, r := range result.Result {
		out[i] = SearchResult{ID: r.ID, Score: r.Score, Payload: r.Payload}
	}
	return out, nil
}
