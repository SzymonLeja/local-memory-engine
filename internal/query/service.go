package query

import (
	"context"
	"fmt"
	"time"

	"github.com/SzymonLeja/local-memory-engine/internal/vector"
	"github.com/google/uuid"
)

type QueryResult struct {
	QueryID  string        `json:"query_id"`
	Results  []ChunkResult `json:"results"`
	Duration int           `json:"duration_ms"`
}

type ChunkResult struct {
	ChunkText string  `json:"chunk_text"`
	FilePath  string  `json:"file_path"`
	Position  string  `json:"position"`
	Score     float64 `json:"score"`
}

func (s *Service) Query(ctx context.Context, text string, topK int) (*QueryResult, error) {
	start := time.Now()

	vec, err := s.ollama.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	hits, err := s.qdrant.Search(ctx, vec, topK)
	if err != nil {
		return nil, fmt.Errorf("qdrant search: %w", err)
	}

	results, err := s.enrichResults(ctx, hits)
	if err != nil {
		return nil, err
	}

	duration := int(time.Since(start).Milliseconds())
	queryID := uuid.New().String()

	if err := s.logProvenance(ctx, queryID, text, hits, duration); err != nil {
		fmt.Printf("provenance log error: %v\n", err)
	}

	return &QueryResult{
		QueryID:  queryID,
		Results:  results,
		Duration: duration,
	}, nil
}

func (s *Service) enrichResults(ctx context.Context, hits []vector.SearchResult) ([]ChunkResult, error) {
	var results []ChunkResult

	for _, hit := range hits {
		chunkID, _ := hit.Payload["chunk_id"].(string)
		if chunkID == "" {
			continue
		}

		var chunkText, position string
		var filePath string
		err := s.db.QueryRow(ctx,
			`SELECT c.chunk_text, c.position, f.path
			 FROM chunks c
			 JOIN files f ON f.id = c.file_id
			 WHERE c.id = $1`,
			chunkID,
		).Scan(&chunkText, &position, &filePath)
		if err != nil {
			continue
		}

		results = append(results, ChunkResult{
			ChunkText: chunkText,
			FilePath:  filePath,
			Position:  position,
			Score:     hit.Score,
		})
	}

	return results, nil
}

func (s *Service) logProvenance(ctx context.Context, queryID, text string, hits []vector.SearchResult, duration int) error {
	usedChunks := make([]map[string]any, len(hits))
	for i, h := range hits {
		usedChunks[i] = map[string]any{
			"chunk_id": h.Payload["chunk_id"],
			"score":    h.Score,
		}
	}

	_, err := s.db.Exec(ctx,
		`INSERT INTO provenance_log (id, query_text, used_chunks, query_duration)
		 VALUES ($1, $2, $3, $4)`,
		queryID, text, usedChunks, duration,
	)
	return err
}
