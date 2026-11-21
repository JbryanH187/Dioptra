package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// E2ERunSummary es la fila resumida de una corrida del Observador E2E.
type E2ERunSummary struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Updated string `json:"updated"`
}

// SaveE2ERun guarda (inserta o reemplaza) el documento JSON de una corrida E2E.
func (s *Store) SaveE2ERun(ctx context.Context, id, project, title, status string, doc []byte) error {
	if id == "" {
		return errors.New("id de corrida vacío")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO e2e_runs (id, project, title, status, updated_at, doc) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET project = excluded.project, title = excluded.title,
			status = excluded.status, updated_at = excluded.updated_at, doc = excluded.doc`,
		id, project, title, status, time.Now().UTC().Format(time.RFC3339), string(doc))
	if err != nil {
		return fmt.Errorf("guardando corrida E2E %s: %w", id, err)
	}
	return nil
}

// LoadE2ERun lee el documento JSON de una corrida E2E.
func (s *Store) LoadE2ERun(ctx context.Context, id string) ([]byte, error) {
	var doc string
	err := s.db.QueryRowContext(ctx, `SELECT doc FROM e2e_runs WHERE id = ?`, id).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("corrida E2E %s no existe", id)
	}
	if err != nil {
		return nil, err
	}
	return []byte(doc), nil
}

// ListE2ERuns devuelve las corridas más recientes primero.
func (s *Store) ListE2ERuns(ctx context.Context, limit int) ([]E2ERunSummary, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, project, title, status, updated_at FROM e2e_runs ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []E2ERunSummary{}
	for rows.Next() {
		var r E2ERunSummary
		if err := rows.Scan(&r.ID, &r.Project, &r.Title, &r.Status, &r.Updated); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
