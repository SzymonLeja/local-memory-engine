package jobs

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Service) GetHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "job_id")
	if id == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}

	job, err := s.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}
