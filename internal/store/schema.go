package store

// Esquema SQLite, columnas, backfill y diagnóstico.
// Movido ÍNTEGRO desde store.go (F7: partir archivos gigantes).

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Diagnostics resume el estado del índice SQLite para `dioptra doctor`.
type Diagnostics struct {
	DBPath      string
	JournalMode string
	QuickCheck  string
	Reports     int
	FTS         bool
}

// Diagnose ejecuta comprobaciones de solo lectura sobre el índice SQLite.
func (s *Store) Diagnose(ctx context.Context) (*Diagnostics, error) {
	d := &Diagnostics{
		DBPath: filepath.Join(s.baseDir, ".index", "dioptra.db"),
		FTS:    s.hasFTS,
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&d.JournalMode); err != nil {
		return d, fmt.Errorf("leyendo journal_mode: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&d.QuickCheck); err != nil {
		return d, fmt.Errorf("ejecutando quick_check: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reports`).Scan(&d.Reports); err != nil {
		return d, fmt.Errorf("contando reportes: %w", err)
	}
	return d, nil
}

func (s *Store) initSchema() error {
	ddl := `
	CREATE TABLE IF NOT EXISTS reports (
		id             TEXT PRIMARY KEY,
		slug           TEXT NOT NULL,
		project        TEXT NOT NULL,
		title          TEXT NOT NULL,
		ts             TEXT NOT NULL,
		max_sev        TEXT NOT NULL,
		excerpt        TEXT NOT NULL,
		md_rel_path    TEXT NOT NULL UNIQUE,
		envelope_json  TEXT NOT NULL,
		checks_json    TEXT NOT NULL,
		warnings_count INTEGER NOT NULL DEFAULT 0,
		updated_at     TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_reports_ts ON reports(ts DESC);
	CREATE INDEX IF NOT EXISTS idx_reports_project ON reports(project);
	CREATE INDEX IF NOT EXISTS idx_reports_title ON reports(title);

	CREATE TABLE IF NOT EXISTS canvas_blocks (
		doc_id   TEXT NOT NULL,
		block_id TEXT NOT NULL,
		html     TEXT NOT NULL,
		PRIMARY KEY (doc_id, block_id)
	);
	CREATE TABLE IF NOT EXISTS publish_metrics (
		id                   INTEGER PRIMARY KEY AUTOINCREMENT,
		report_id            TEXT NOT NULL,
		mode                 TEXT NOT NULL,
		model                TEXT NOT NULL,
		is_exact             INTEGER NOT NULL DEFAULT 0,
		baseline_out_tokens  INTEGER NOT NULL DEFAULT 0,
		dioptra_in_tokens    INTEGER NOT NULL DEFAULT 0,
		dioptra_out_tokens   INTEGER NOT NULL DEFAULT 0,
		extra_out_tokens     INTEGER NOT NULL DEFAULT 0,
		delta_pct            REAL NOT NULL DEFAULT 0,
		baseline_usd         REAL NOT NULL DEFAULT 0,
		dioptra_usd          REAL NOT NULL DEFAULT 0,
		delta_usd            REAL NOT NULL DEFAULT 0,
		created_at           TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_publish_metrics_created ON publish_metrics(created_at DESC);
	`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("inicializando esquema SQLite: %w", err)
	}

	// Migraciones numeradas (PRAGMA user_version): columnas, índices, backfill y tablas nuevas.
	if err := s.migrate(); err != nil {
		return err
	}

	// Índice de búsqueda FTS5 con ranking BM25 (tabla nueva `reports_search`; la antigua reports_fts no se toca).
	s.ensureSearchIndex(context.Background())
	return nil
}

func ensureReportColumns(q dbExec) error {
	existing := make(map[string]bool)
	rows, err := q.Query(`PRAGMA table_info(reports)`)
	if err != nil {
		return fmt.Errorf("inspeccionando columnas de reports: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err == nil {
			existing[strings.ToLower(name)] = true
		}
	}
	migrations := []struct {
		name string
		ddl  string
	}{
		{"idempotency_key", `ALTER TABLE reports ADD COLUMN idempotency_key TEXT NOT NULL DEFAULT ''`},
		{"content_hash", `ALTER TABLE reports ADD COLUMN content_hash TEXT NOT NULL DEFAULT ''`},
		{"revision", `ALTER TABLE reports ADD COLUMN revision INTEGER NOT NULL DEFAULT 1`},
		{"last_change", `ALTER TABLE reports ADD COLUMN last_change TEXT NOT NULL DEFAULT ''`},
		{"tool", `ALTER TABLE reports ADD COLUMN tool TEXT NOT NULL DEFAULT ''`},
		{"model", `ALTER TABLE reports ADD COLUMN model TEXT NOT NULL DEFAULT ''`},
		{"project_key", `ALTER TABLE reports ADD COLUMN project_key TEXT NOT NULL DEFAULT ''`},
	}
	for _, m := range migrations {
		if existing[m.name] {
			continue
		}
		if _, err := q.Exec(m.ddl); err != nil {
			return fmt.Errorf("migrando columna reports.%s: %w", m.name, err)
		}
	}
	idxDDL := `
	CREATE INDEX IF NOT EXISTS idx_reports_idempotency ON reports(idempotency_key);
	CREATE INDEX IF NOT EXISTS idx_reports_project_slug ON reports(project, slug);
	CREATE INDEX IF NOT EXISTS idx_reports_updated_at ON reports(updated_at DESC);
	CREATE INDEX IF NOT EXISTS idx_reports_project_key ON reports(project_key);
	CREATE INDEX IF NOT EXISTS idx_reports_project_key_slug ON reports(project_key, slug);
	`
	if _, err := q.Exec(idxDDL); err != nil {
		return fmt.Errorf("creando índices de idempotencia: %w", err)
	}
	return backfillProjectKeys(q)
}

func ensureReportColumn(q dbExec, colName, ddl string) error {
	rows, err := q.Query(`PRAGMA table_info(reports)`)
	if err != nil {
		return fmt.Errorf("inspeccionando columnas de reports: %w", err)
	}
	defer rows.Close()
	want := strings.ToLower(colName)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err == nil {
			if strings.ToLower(name) == want {
				return nil
			}
		}
	}
	if _, err := q.Exec(ddl); err != nil {
		return fmt.Errorf("migrando columna reports.%s: %w", colName, err)
	}
	return nil
}

// backfillProjectKeys rellena project_key en filas previas a la migración (solo UPDATE, nunca DROP).
// Es idempotente: solo toca filas con project_key vacío. Unifica el display al primero publicado por clave.
func backfillProjectKeys(q dbExec) error {
	rows, err := q.Query(`SELECT DISTINCT project FROM reports WHERE project_key = ''`)
	if err != nil {
		return fmt.Errorf("leyendo proyectos sin project_key: %w", err)
	}
	var projects []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil {
			projects = append(projects, p)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	for _, p := range projects {
		if _, err := q.Exec(`UPDATE reports SET project_key = ? WHERE project = ? AND project_key = ''`, ProjectKey(p), p); err != nil {
			return fmt.Errorf("backfill project_key: %w", err)
		}
	}
	return nil
}
