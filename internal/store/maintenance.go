package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DBReport es el resultado de solo lectura de `dioptra db check`.
type DBReport struct {
	DBPath        string   `json:"dbPath"`
	SchemaVersion int      `json:"schemaVersion"`
	LatestVersion int      `json:"latestVersion"`
	JournalMode   string   `json:"journalMode"`
	QuickCheck    string   `json:"quickCheck"`
	Reports       int      `json:"reports"`
	SearchRows    int      `json:"searchRows"`
	FTS           bool     `json:"fts"`
	Aliases       int      `json:"aliases"`
	MissingMD     []string `json:"missingMarkdown"`
	NotIndexed    int      `json:"mdNotIndexed"`
}

// CheckDB ejecuta comprobaciones de solo lectura: integridad, versión de esquema, índice de búsqueda
// y consistencia entre filas y archivos .md (fuente de verdad).
func (s *Store) CheckDB(ctx context.Context) (*DBReport, error) {
	d, err := s.Diagnose(ctx)
	if err != nil {
		return nil, err
	}
	r := &DBReport{DBPath: d.DBPath, JournalMode: d.JournalMode, QuickCheck: d.QuickCheck, Reports: d.Reports, FTS: s.hasFTS, LatestVersion: LatestSchemaVersion(), MissingMD: []string{}}
	if r.SchemaVersion, err = s.SchemaVersion(); err != nil {
		return nil, err
	}
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_aliases`).Scan(&r.Aliases)
	if s.hasFTS {
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reports_search`).Scan(&r.SearchRows)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, md_rel_path FROM reports`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, rel string
		if rows.Scan(&id, &rel) != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(s.baseDir, filepath.FromSlash(rel))); err != nil {
			r.MissingMD = append(r.MissingMD, id+" → "+rel)
		}
	}
	return r, rows.Err()
}

// RebuildSearchIndex reconstruye reports_search desde la BD (título, proyecto, tool, modelo y cuerpo del .md).
// Es la única operación de escritura de mantenimiento y no toca la tabla reports ni los .md.
func (s *Store) RebuildSearchIndex(ctx context.Context) (int, error) {
	if !s.hasFTS {
		return 0, errors.New("FTS5 no está disponible en esta build")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, project, tool, model, excerpt, md_rel_path FROM reports`)
	if err != nil {
		return 0, err
	}
	type doc struct{ id, title, project, tool, model, body string }
	var docs []doc
	for rows.Next() {
		var d doc
		var excerpt, rel string
		if rows.Scan(&d.id, &d.title, &d.project, &d.tool, &d.model, &excerpt, &rel) != nil {
			continue
		}
		d.body = excerpt
		if b, err := os.ReadFile(filepath.Join(s.baseDir, filepath.FromSlash(rel))); err == nil {
			d.body = string(b)
		}
		docs = append(docs, d)
	}
	_ = rows.Close()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reports_search`); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	for _, d := range docs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO reports_search (id, title, project, tool, model, body) VALUES (?, ?, ?, ?, ?, ?)`,
			d.id, d.title, d.project, d.tool, d.model, d.body); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
	}
	return len(docs), tx.Commit()
}

// BackupTo genera una copia consistente (VACUUM INTO) incluso con WAL activo y escritores vivos.
// Rechaza sobrescribir un archivo existente y verifica la copia con quick_check.
func (s *Store) BackupTo(ctx context.Context, dest string) error {
	dest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("el destino ya existe y no se sobrescribe: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO `+quoteSQLString(dest)); err != nil {
		return fmt.Errorf("VACUUM INTO: %w", err)
	}
	return verifyBackup(ctx, dest)
}

func quoteSQLString(v string) string {
	return "'" + strings.ReplaceAll(filepath.ToSlash(v), "'", "''") + "'"
}
