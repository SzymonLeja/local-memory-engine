package ingest

import (
	"github.com/SzymonLeja/local-memory-engine/internal/config"
	"github.com/SzymonLeja/local-memory-engine/internal/embeddings"
	"github.com/SzymonLeja/local-memory-engine/internal/vector"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db     *pgxpool.Pool
	cfg    *config.Config
	ollama *embeddings.OllamaClient
	qdrant *vector.QdrantClient
}

func NewService(
	db *pgxpool.Pool,
	cfg *config.Config,
	ollama *embeddings.OllamaClient,
	qdrant *vector.QdrantClient,
) *Service {
	return &Service{db: db, cfg: cfg, ollama: ollama, qdrant: qdrant}
}
