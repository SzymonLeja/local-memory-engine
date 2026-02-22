package jobs

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

type Job struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Error     *string   `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Service) GetByID(ctx context.Context, id string) (*Job, error) {
	var j Job
	err := s.db.QueryRow(ctx,
		`SELECT id, status, error, created_at, updated_at FROM jobs WHERE id = $1`, id,
	).Scan(&j.ID, &j.Status, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("job not found: %w", err)
	}
	return &j, nil
}
