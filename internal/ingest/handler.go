package ingest

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

type ingestRequest struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Content  string `json:"content"`
	Format   string `json:"format"`
}

func (s *Service) IngestHandler(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		s.ingestMultipart(w, r)
		return
	}

	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var result *IngestResult
	var err error

	if req.Filename != "" {
		if req.Format != "" && req.Format != "md" {
			http.Error(w, "only format=md is supported in v0.1", http.StatusBadRequest)
			return
		}
		result, err = s.IngestDirect(r.Context(), req.Filename, req.Path, req.Content)
	} else if req.Path != "" {
		result, err = s.IngestPath(r.Context(), req.Path)
	} else {
		http.Error(w, "path or filename is required", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Service) GetFileHandler(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	format := r.URL.Query().Get("format")
	path := r.URL.Query().Get("path")

	if format == "" {
		format = "md"
	}
	if format != "md" {
		http.Error(w, "only format=md is supported in v0.1", http.StatusBadRequest)
		return
	}

	content, filePath, updatedAt, err := s.GetFile(r.Context(), filename, format, path)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrMultipleMatches) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMultipleChoices)
			json.NewEncoder(w).Encode(err.(*MultipleMatchesError).Matches)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"filename":   filename,
		"format":     format,
		"path":       filePath,
		"content":    content,
		"updated_at": updatedAt,
	})
}

func (s *Service) PatchIngestHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
		Format   string `json:"format"`
		Path     string `json:"path"`
		Content  string `json:"content"`
		Append   string `json:"append"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Filename == "" {
		http.Error(w, "filename is required", http.StatusBadRequest)
		return
	}
	if req.Format != "" && req.Format != "md" {
		http.Error(w, "only format=md is supported in v0.1", http.StatusBadRequest)
		return
	}
	if req.Content == "" && req.Append == "" {
		http.Error(w, "content or append is required", http.StatusBadRequest)
		return
	}

	result, err := s.EditFile(r.Context(), req.Filename, req.Format, req.Path, req.Content, req.Append)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrMultipleMatches) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMultipleChoices)
			json.NewEncoder(w).Encode(err.(*MultipleMatchesError).Matches)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Service) ingestMultipart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "failed to parse multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".md" {
		http.Error(w, "only .md files are supported in v0.1", http.StatusBadRequest)
		return
	}

	filename := r.FormValue("filename")
	if filename == "" {
		filename = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	}
	path := r.FormValue("path")

	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read file", http.StatusInternalServerError)
		return
	}

	result, err := s.IngestDirect(r.Context(), filename, path, string(content))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
