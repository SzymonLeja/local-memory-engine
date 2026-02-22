package query

import (
	"encoding/json"
	"net/http"
)

type queryRequest struct {
	Q    string `json:"q"`
	TopK int    `json:"top_k"`
}

func (s *Service) QueryHandler(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Q == "" {
		http.Error(w, "q is required", http.StatusBadRequest)
		return
	}

	if req.TopK <= 0 {
		req.TopK = 5
	}

	result, err := s.Query(r.Context(), req.Q, req.TopK)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
