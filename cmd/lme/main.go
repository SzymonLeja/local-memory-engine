package main

import (
	"context"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"github.com/SzymonLeja/local-memory-engine/internal/config"
	"github.com/SzymonLeja/local-memory-engine/internal/db"
	"github.com/SzymonLeja/local-memory-engine/internal/embeddings"
	"github.com/SzymonLeja/local-memory-engine/internal/ingest"
	"github.com/SzymonLeja/local-memory-engine/internal/jobs"
	lmemiddleware "github.com/SzymonLeja/local-memory-engine/internal/middleware"
	"github.com/SzymonLeja/local-memory-engine/internal/provenance"
	"github.com/SzymonLeja/local-memory-engine/internal/query"
	"github.com/SzymonLeja/local-memory-engine/internal/vector"
)

func main() {
	cfg := config.Load()

	dbConn, err := db.Connect(cfg.PostgresDSN)
	if err != nil {
		log.Fatal("DB connect:", err)
	}
	defer dbConn.Close()

	if err := db.Migrate(dbConn); err != nil {
		log.Fatal("Migrate error:", err)
	}

	qdrantClient := vector.NewQdrantClient(cfg.QdrantURL, cfg.QdrantCollection)
	if err := qdrantClient.EnsureCollection(context.Background(), 768); err != nil {
		log.Fatal("Qdrant EnsureCollection:", err)
	}
	log.Println("Qdrant collection OK")

	ollamaClient := embeddings.NewOllamaClient(cfg.OllamaURL, cfg.EmbeddingModel)

	ingestSvc := ingest.NewService(dbConn, cfg, ollamaClient, qdrantClient)
	querySvc := query.NewService(dbConn, cfg, ollamaClient, qdrantClient)

	provenanceSvc := provenance.NewService(dbConn)
	jobsSvc := jobs.NewService(dbConn)
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.New(cors.Options{
		AllowedOrigins: cfg.AllowedOrigins,
		AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "X-API-Key"},
	}).Handler)

	r.Use(lmemiddleware.APIKeyAuth(cfg.ApiKey))
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/health", healthHandler)
	r.Post("/ingest", ingestSvc.IngestHandler)
	r.Post("/query", querySvc.QueryHandler)
	r.Get("/provenance/{id}", provenanceSvc.GetHandler)
	r.Get("/status/{job_id}", jobsSvc.GetHandler)
	r.Get("/file/{filename}", ingestSvc.GetFileHandler)
	r.Patch("/ingest", ingestSvc.PatchIngestHandler)

	log.Printf("LME listening on %s", cfg.ListenAddr)

	if cfg.WatchPath != "" {
		if err := ingestSvc.StartWatcher(context.Background(), cfg.WatchPath); err != nil {
			log.Printf("watcher failed to start: %v", err)
		} else {
			log.Printf("watcher started on: %s", cfg.WatchPath)
		}
	}
	log.Fatal(http.ListenAndServe(cfg.ListenAddr, r))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
