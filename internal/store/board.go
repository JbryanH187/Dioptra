package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// SaveBoardTask guarda (inserta o reemplaza) una tarea del tablero de equipo.
func (s *Store) SaveBoardTask(ctx context.Context, projectKey, id, status string, doc []byte) error {
	if projectKey == "" || id == "" {
		return errors.New("tarea sin proyecto o id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO board_tasks (project_key, id, status, updated_at, doc) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_key, id) DO UPDATE SET status = excluded.status,
			updated_at = excluded.updated_at, doc = excluded.doc`,
		projectKey, id, status, time.Now().UTC().Format(time.RFC3339), string(doc))
	if err != nil {
		return fmt.Errorf("guardando tarea %s: %w", id, err)
	}
	return nil
}

// LoadBoardTasks devuelve los documentos JSON de todas las tareas de un proyecto.
func (s *Store) LoadBoardTasks(ctx context.Context, projectKey string) ([][]byte, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT doc FROM board_tasks WHERE project_key = ? ORDER BY rowid`, projectKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var doc string
		if err := rows.Scan(&doc); err != nil {
			return nil, err
		}
		out = append(out, []byte(doc))
	}
	return out, rows.Err()
}
