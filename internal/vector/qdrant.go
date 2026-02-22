package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type QdrantClient struct {
	baseURL    string
	collection string
	client     *http.Client
}

func NewQdrantClient(baseURL, collection string) *QdrantClient {
	return &QdrantClient{
		baseURL:    baseURL,
		collection: collection,
		client:     &http.Client{},
	}
}

type createCollectionRequest struct {
	Vectors vectorsConfig `json:"vectors"`
}

type vectorsConfig struct {
	Size     int    `json:"size"`
	Distance string `json:"distance"`
}

func (c *QdrantClient) EnsureCollection(ctx context.Context, vectorSize int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/collections/%s", c.baseURL, c.collection), nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body, err := json.Marshal(createCollectionRequest{
		Vectors: vectorsConfig{Size: vectorSize, Distance: "Cosine"},
	})
	if err != nil {
		return err
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/collections/%s", c.baseURL, c.collection),
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp2, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return fmt.Errorf("qdrant create collection: status %d", resp2.StatusCode)
	}
	return nil
}

func (c *QdrantClient) Delete(ctx context.Context, pointID string) error {
	url := fmt.Sprintf("%s/collections/%s/points/delete", c.baseURL, c.collection)
	body := map[string]any{
		"points": []string{pointID},
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
