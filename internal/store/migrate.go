package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// dbExec lo satisfacen *sql.DB y *sql.Tx, para reutilizar la misma migración dentro de una transacción.
type dbExec interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
}

type schemaMigration struct {
	version int
	name    string
	up      func(tx *sql.Tx) error
}

// schemaMigrations es la lista ordenada y append-only de cambios de esquema.
// Reglas: nunca editar una entrada ya publicada, nunca DROP, cada `up` debe ser idempotente
// (las bases previas a user_version arrancan en 0 pero ya contienen las columnas de la v1).
var schemaMigrations = []schemaMigration{
	{1, "columnas de versionado, tool/model/project_key, índices y backfill", func(tx *sql.Tx) error {
		return ensureReportColumns(tx)
	}},
	{2, "tabla project_aliases (unificación de proyectos equivalentes)", func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS project_aliases (
				alias_key     TEXT PRIMARY KEY,
				canonical_key TEXT NOT NULL,
				created_at    TEXT NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_project_aliases_canonical ON project_aliases(canonical_key);`)
		return err
	}},
	{3, "columna reports.md_hash (hash puro del markdown para evitar falsos SyncDiskChanges)", func(tx *sql.Tx) error {
		return ensureReportColumn(tx, "md_hash", `ALTER TABLE reports ADD COLUMN md_hash TEXT NOT NULL DEFAULT ''`)
	}},
	{4, "tabla e2e_runs (Observador E2E: corridas como documento JSON)", func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS e2e_runs (
				id         TEXT PRIMARY KEY,
				project    TEXT NOT NULL DEFAULT '',
				title      TEXT NOT NULL DEFAULT '',
				status     TEXT NOT NULL DEFAULT '',
				updated_at TEXT NOT NULL,
				doc        TEXT NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_e2e_runs_updated ON e2e_runs(updated_at);`)
		return err
	}},
	{5, "tabla board_tasks (tablero de equipo Dev · Tester · PO)", func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS board_tasks (
				project_key TEXT NOT NULL,
				id          TEXT NOT NULL,
				status      TEXT NOT NULL DEFAULT '',
				updated_at  TEXT NOT NULL,
				doc         TEXT NOT NULL,
				PRIMARY KEY (project_key, id)
			);`)
		return err
	}},
}

// LatestSchemaVersion es la versión de esquema que deja Open tras migrar.
func LatestSchemaVersion() int {
	return schemaMigrations[len(schemaMigrations)-1].version
}

// SchemaVersion lee PRAGMA user_version.
func (s *Store) SchemaVersion() (int, error) {
	var v int
	err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
	return v, err
}

// migrate aplica en orden las migraciones pendientes, cada una en su propia transacción
// junto con el avance de user_version (todo o nada). Si otro proceso (serve o puente MCP)
// migra a la vez y esta conexión pierde la carrera, se relee la versión y se reintenta.
func (s *Store) migrate() error {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 150 * time.Millisecond)
		}
		lastErr = s.migrateOnce()
		if lastErr == nil {
			return nil
		}
		msg := strings.ToLower(lastErr.Error())
		if !strings.Contains(msg, "locked") && !strings.Contains(msg, "busy") {
			return lastErr
		}
	}
	return lastErr
}

func (s *Store) migrateOnce() error {
	cur, err := s.SchemaVersion()
	if err != nil {
		return fmt.Errorf("leyendo user_version: %w", err)
	}
	for _, m := range schemaMigrations {
		if m.version <= cur {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("migración %d: iniciando transacción: %w", m.version, err)
		}
		if err := m.up(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migración %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, m.version)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migración %d: actualizando user_version: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migración %d: commit: %w", m.version, err)
		}
		cur = m.version
	}
	return nil
}
