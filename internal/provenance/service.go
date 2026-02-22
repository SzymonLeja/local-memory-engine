package provenance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

type ProvenanceLog struct {
	ID            string           `json:"id"`
	QueryText     string           `json:"query_text"`
	UsedChunks    []map[string]any `json:"used_chunks"`
	QueryDuration int              `json:"duration_ms"`
	CreatedAt     time.Time        `json:"created_at"`
}

func (s *Service) GetByID(ctx context.Context, id string) (*ProvenanceLog, error) {
	var p ProvenanceLog
	err := s.db.QueryRow(ctx,
		`SELECT id, query_text, used_chunks, query_duration, created_at
		 FROM provenance_log WHERE id = $1`, id,
	).Scan(&p.ID, &p.QueryText, &p.UsedChunks, &p.QueryDuration, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("provenance not found: %w", err)
	}
	return &p, nil
}
