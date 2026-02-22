package ingest

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type IngestResult struct {
	JobID        string `json:"job_id"`
	NewFiles     int    `json:"new_files"`
	UpdatedFiles int    `json:"updated_files"`
	Skipped      int    `json:"skipped"`
}

func (s *Service) IngestPath(ctx context.Context, relPath string) (*IngestResult, error) {
	entries, err := WalkVault(s.cfg.VaultRoot, relPath)
	if err != nil {
		return nil, fmt.Errorf("walk vault: %w", err)
	}

	jobID := uuid.New().String()
	_, err = s.db.Exec(ctx,
		`INSERT INTO jobs (id, status) VALUES ($1, 'running')`, jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}

	result := &IngestResult{JobID: jobID}

	for _, entry := range entries {
		action, fileID, err := s.upsertFile(ctx, entry)
		if err != nil {
			_, _ = s.db.Exec(ctx,
				`UPDATE jobs SET status = 'error', error = $1, updated_at = NOW() WHERE id = $2`,
				err.Error(), jobID,
			)
			return nil, fmt.Errorf("upsertFile %s: %w", entry.Path, err)
		}

		switch action {
		case "skip":
			result.Skipped++
		case "new":
			result.NewFiles++
			absPath := filepath.Join(s.cfg.VaultRoot, entry.Path)
			if err := s.indexFile(ctx, fileID, entry.Path, absPath); err != nil {
				_, _ = s.db.Exec(ctx,
					`UPDATE jobs SET status = 'error', error = $1, updated_at = NOW() WHERE id = $2`,
					err.Error(), jobID,
				)
				return nil, err
			}
		case "updated":
			result.UpdatedFiles++
			absPath := filepath.Join(s.cfg.VaultRoot, entry.Path)
			if err := s.indexFile(ctx, fileID, entry.Path, absPath); err != nil {
				_, _ = s.db.Exec(ctx,
					`UPDATE jobs SET status = 'error', error = $1, updated_at = NOW() WHERE id = $2`,
					err.Error(), jobID,
				)
				return nil, err
			}
		}
	}

	_, _ = s.db.Exec(ctx,
		`UPDATE jobs SET status = 'done', updated_at = NOW() WHERE id = $1`, jobID,
	)
	result.JobID = jobID

	return result, nil
}

func (s *Service) upsertFile(ctx context.Context, entry FileEntry) (string, string, error) {
	var existingID, existingHash string
	err := s.db.QueryRow(ctx,
		`SELECT id, file_hash FROM files WHERE path = $1`,
		filepath.ToSlash(entry.Path),
	).Scan(&existingID, &existingHash)

	if err != nil && err.Error() != "no rows in result set" {
		return "", "", err
	}

	slashPath := filepath.ToSlash(entry.Path)

	if existingID == "" {
		var newID string
		err := s.db.QueryRow(ctx,
			`INSERT INTO files (path, file_hash, last_modified, status)
			 VALUES ($1, $2, $3, 'pending') RETURNING id`,
			slashPath, entry.Hash, entry.LastModified,
		).Scan(&newID)
		return "new", newID, err
	}

	if existingHash == entry.Hash {
		return "skip", existingID, nil
	}

	_, err = s.db.Exec(ctx,
		`UPDATE files SET file_hash = $1, version = version + 1,
		 last_modified = $2, status = 'pending' WHERE id = $3`,
		entry.Hash, time.Now(), existingID,
	)
	return "updated", existingID, err
}

func (s *Service) indexFile(ctx context.Context, fileID, relPath, absPath string) error {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read file %s: %w", absPath, err)
	}

	chunks := ChunkText(filepath.ToSlash(relPath), string(content))

	newIDs := make(map[string]struct{}, len(chunks))
	for _, c := range chunks {
		newIDs[c.ID] = struct{}{}
	}

	oldRows, err := s.db.Query(ctx,
		`SELECT id FROM chunks WHERE file_id = $1`, fileID,
	)
	if err != nil {
		return fmt.Errorf("query old chunks: %w", err)
	}
	defer oldRows.Close()

	var toDelete []string
	for oldRows.Next() {
		var id string
		if scanErr := oldRows.Scan(&id); scanErr == nil {
			if _, exists := newIDs[id]; !exists {
				toDelete = append(toDelete, id)
			}
		}
	}
	oldRows.Close()

	for _, id := range toDelete {
		_ = s.qdrant.Delete(ctx, id)
		_, _ = s.db.Exec(ctx, `DELETE FROM embeddings WHERE chunk_id = $1`, id)
		_, _ = s.db.Exec(ctx, `DELETE FROM chunks WHERE id = $1`, id)
	}

	for _, chunk := range chunks {
		var existing string
		err := s.db.QueryRow(ctx,
			`SELECT id FROM chunks WHERE id = $1`, chunk.ID,
		).Scan(&existing)
		if err == nil {
			continue
		}

		_, err = s.db.Exec(ctx,
			`INSERT INTO chunks (id, file_id, chunk_text, position)
			 VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`,
			chunk.ID, fileID, chunk.Text, chunk.Position,
		)
		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}

		vector, err := s.ollama.Embed(ctx, chunk.Text)
		if err != nil {
			return fmt.Errorf("embed chunk %s: %w", chunk.ID, err)
		}

		payload := map[string]any{
			"chunk_id":  chunk.ID,
			"file_path": relPath,
			"position":  chunk.Position,
		}
		if err := s.qdrant.Upsert(ctx, chunk.ID, vector, payload); err != nil {
			return fmt.Errorf("qdrant upsert: %w", err)
		}

		_, err = s.db.Exec(ctx,
			`INSERT INTO embeddings (chunk_id, vector_id, embedding_model)
			 VALUES ($1, $2, $3)`,
			chunk.ID, chunk.ID, s.cfg.EmbeddingModel,
		)
		if err != nil {
			return fmt.Errorf("insert embedding: %w", err)
		}
	}

	_, err = s.db.Exec(ctx,
		`UPDATE files SET status = 'ready' WHERE id = $1`, fileID,
	)
	return err
}

func (s *Service) IngestDirect(ctx context.Context, filename, relPath, content string) (*IngestResult, error) {
	if relPath == "" {
		relPath = "api-notes"
	}

	absDir, err := sanitizePath(s.cfg.VaultRoot, relPath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	absFile := filepath.Join(absDir, filename+".md")
	if err := os.WriteFile(absFile, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	relFilePath := filepath.ToSlash(filepath.Join(relPath, filename+".md"))
	hash := fmt.Sprintf("%x", sha256sum([]byte(content)))

	entry := FileEntry{
		Path:         relFilePath,
		Hash:         hash,
		LastModified: time.Now(),
	}

	jobID := uuid.New().String()
	_, err = s.db.Exec(ctx,
		`INSERT INTO jobs (id, status) VALUES ($1, 'running')`, jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}

	result := &IngestResult{JobID: jobID}

	action, fileID, err := s.upsertFile(ctx, entry)
	if err != nil {
		_, _ = s.db.Exec(ctx,
			`UPDATE jobs SET status = 'error', error = $1, updated_at = NOW() WHERE id = $2`,
			err.Error(), jobID,
		)
		return nil, err
	}

	switch action {
	case "new":
		result.NewFiles++
		if err := s.indexFile(ctx, fileID, relFilePath, absFile); err != nil {
			_, _ = s.db.Exec(ctx,
				`UPDATE jobs SET status = 'error', error = $1, updated_at = NOW() WHERE id = $2`,
				err.Error(), jobID,
			)
			return nil, err
		}
	case "updated":
		result.UpdatedFiles++
		if err := s.indexFile(ctx, fileID, relFilePath, absFile); err != nil {
			_, _ = s.db.Exec(ctx,
				`UPDATE jobs SET status = 'error', error = $1, updated_at = NOW() WHERE id = $2`,
				err.Error(), jobID,
			)
			return nil, err
		}
	case "skip":
		result.Skipped++
	}

	_, _ = s.db.Exec(ctx,
		`UPDATE jobs SET status = 'done', updated_at = NOW() WHERE id = $1`, jobID,
	)

	return result, nil
}

var ErrNotFound = errors.New("file not found")
var ErrMultipleMatches = errors.New("multiple matches")

type MultipleMatchesError struct {
	Matches []map[string]string
}

func (e *MultipleMatchesError) Error() string { return ErrMultipleMatches.Error() }

func (s *Service) GetFile(ctx context.Context, filename, format, path string) (content, filePath string, updatedAt time.Time, err error) {
	ext := "." + format
	nameWithExt := filename + ext

	var rows []struct {
		Path         string
		LastModified time.Time
	}

	if path != "" {
		relPath := filepath.ToSlash(filepath.Join(path, nameWithExt))
		var f struct {
			Path         string
			LastModified time.Time
		}
		dbErr := s.db.QueryRow(ctx,
			`SELECT path, last_modified FROM files WHERE path = $1`, relPath,
		).Scan(&f.Path, &f.LastModified)
		if dbErr != nil {
			return "", "", time.Time{}, ErrNotFound
		}
		rows = append(rows, f)
	} else {
		dbRows, dbErr := s.db.Query(ctx,
			`SELECT path, last_modified FROM files WHERE path LIKE $1
			 ORDER BY CASE WHEN path LIKE 'api-notes/%' THEN 0 ELSE 1 END`,
			"%/"+nameWithExt,
		)
		if dbErr != nil {
			return "", "", time.Time{}, dbErr
		}
		defer dbRows.Close()
		for dbRows.Next() {
			var f struct {
				Path         string
				LastModified time.Time
			}
			if scanErr := dbRows.Scan(&f.Path, &f.LastModified); scanErr == nil {
				rows = append(rows, f)
			}
		}
	}

	if len(rows) == 0 {
		return "", "", time.Time{}, ErrNotFound
	}
	if len(rows) > 1 {
		matches := make([]map[string]string, len(rows))
		for i, r := range rows {
			matches[i] = map[string]string{
				"path":      filepath.Dir(r.Path),
				"full_path": filepath.Join(s.cfg.VaultRoot, r.Path),
			}
		}
		return "", "", time.Time{}, &MultipleMatchesError{Matches: matches}
	}

	absPath := filepath.Join(s.cfg.VaultRoot, filepath.FromSlash(rows[0].Path))
	data, readErr := os.ReadFile(absPath)
	if readErr != nil {
		return "", "", time.Time{}, readErr
	}

	return string(data), filepath.Dir(rows[0].Path), rows[0].LastModified, nil
}

func (s *Service) EditFile(ctx context.Context, filename, format, path, content, appendText string) (*IngestResult, error) {
	if format == "" {
		format = "md"
	}
	ext := "." + format
	nameWithExt := filename + ext

	var relFilePath string

	if path != "" {
		relFilePath = filepath.ToSlash(filepath.Join(path, nameWithExt))
		var count int
		err := s.db.QueryRow(ctx,
			`SELECT COUNT(*) FROM files WHERE path = $1`, relFilePath,
		).Scan(&count)
		if err != nil || count == 0 {
			return nil, ErrNotFound
		}
	} else {
		rows, err := s.db.Query(ctx,
			`SELECT path FROM files WHERE path LIKE $1
			 ORDER BY CASE WHEN path LIKE 'api-notes/%' THEN 0 ELSE 1 END`,
			"%/"+nameWithExt,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var matches []string
		for rows.Next() {
			var p string
			if scanErr := rows.Scan(&p); scanErr == nil {
				matches = append(matches, p)
			}
		}

		switch len(matches) {
		case 0:
			return nil, ErrNotFound
		case 1:
			relFilePath = matches[0]
		default:
			result := make([]map[string]string, len(matches))
			for i, m := range matches {
				result[i] = map[string]string{
					"path":      filepath.Dir(m),
					"full_path": filepath.Join(s.cfg.VaultRoot, m),
				}
			}
			return nil, &MultipleMatchesError{Matches: result}
		}
	}

	absPath := filepath.Join(s.cfg.VaultRoot, filepath.FromSlash(relFilePath))

	var newContent string
	if appendText != "" {
		existing, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
		newContent = string(existing) + "\n" + appendText
	} else {
		newContent = content
	}

	if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	hash := fmt.Sprintf("%x", sha256sum([]byte(newContent)))
	entry := FileEntry{
		Path:         relFilePath,
		Hash:         hash,
		LastModified: time.Now(),
	}

	jobID := uuid.New().String()
	_, err := s.db.Exec(ctx, `INSERT INTO jobs (id, status) VALUES ($1, 'running')`, jobID)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}

	result := &IngestResult{JobID: jobID}

	action, fileID, err := s.upsertFile(ctx, entry)
	if err != nil {
		_, _ = s.db.Exec(ctx,
			`UPDATE jobs SET status = 'error', error = $1, updated_at = NOW() WHERE id = $2`,
			err.Error(), jobID,
		)
		return nil, err
	}

	if action == "updated" {
		result.UpdatedFiles++
		if err := s.indexFile(ctx, fileID, relFilePath, absPath); err != nil {
			_, _ = s.db.Exec(ctx,
				`UPDATE jobs SET status = 'error', error = $1, updated_at = NOW() WHERE id = $2`,
				err.Error(), jobID,
			)
			return nil, err
		}
	}

	_, _ = s.db.Exec(ctx,
		`UPDATE jobs SET status = 'done', updated_at = NOW() WHERE id = $1`, jobID,
	)

	return result, nil
}

func sha256sum(data []byte) [32]byte {
	return sha256.Sum256(data)
}
