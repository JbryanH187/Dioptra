package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"dioptra/internal/store"
)

func TestProjectKey_Normalization(t *testing.T) {
	cases := map[string]string{
		"DioptraEngine":     "dioptraengine",
		"Dioptra Engine":    "dioptra-engine",
		"dioptra-engine":    "dioptra-engine",
		"DIOPTRA_ENGINE":    "dioptra-engine",
		"  Cartera  Core ":  "cartera-core",
		"Auditoría/Almacén": "auditoria-almacen",
		"":                  "",
		"---":               "",
	}
	for in, want := range cases {
		if got := store.ProjectKey(in); got != want {
			t.Errorf("ProjectKey(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestStore_ToolAndModelPersistedAndListed(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	res, err := st.Publish(ctx, store.PublishParams{
		Title: "Informe A", Project: "DioptraEngine", Tool: "Genius Shell", Model: "claude-sonnet-5-5",
		Markdown: "# Informe A\n\ncontenido", Now: time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Tool != "Genius Shell" || res.Summary.Model != "claude-sonnet-5-5" {
		t.Fatalf("summary sin tool/model: %+v", res.Summary)
	}
	list, err := st.ListReports(ctx, "", "", 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
	if list[0].Tool != "Genius Shell" || list[0].Model != "claude-sonnet-5-5" {
		t.Fatalf("list sin tool/model: %+v", list[0])
	}
	// búsqueda por herramienta
	byTool, _ := st.ListReports(ctx, "", "genius", 10)
	if len(byTool) != 1 {
		t.Fatalf("búsqueda por tool esperaba 1, got %d", len(byTool))
	}
}

func TestStore_ProjectVariantsDoNotDuplicate_ToolNotInKey(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	t1 := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

	r1, err := st.Publish(ctx, store.PublishParams{
		Title: "Plan", Project: "Dioptra Engine", Tool: "Codex", Markdown: "# Plan\n\nv1", Now: t1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Misma identidad (proyecto con otra grafía + mismo título) desde OTRA herramienta: actualiza in-place.
	r2, err := st.Publish(ctx, store.PublishParams{
		Title: "Plan", Project: "dioptra-engine", Tool: "ClaudeCode", Markdown: "# Plan\n\nv2", Now: t1.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if r2.ID != r1.ID || r2.Action != "updated" || r2.Revision != 2 {
		t.Fatalf("debía actualizar in-place: id %s vs %s, action=%s rev=%d", r2.ID, r1.ID, r2.Action, r2.Revision)
	}
	if r2.Summary.Project != "Dioptra Engine" {
		t.Fatalf("el display debe conservar la primera grafía, got %q", r2.Summary.Project)
	}
	if r2.Summary.Tool != "ClaudeCode" {
		t.Fatalf("tool debe reflejar la última herramienta, got %q", r2.Summary.Tool)
	}
	all, _ := st.ListReports(ctx, "", "", 10)
	if len(all) != 1 {
		t.Fatalf("esperaba 1 reporte, got %d", len(all))
	}
	// El filtro por proyecto acepta cualquier grafía
	f, _ := st.ListReports(ctx, "DIOPTRA_ENGINE", "", 10)
	if len(f) != 1 {
		t.Fatalf("filtro por grafía alternativa esperaba 1, got %d", len(f))
	}
}

func TestStore_MigrationFromLegacyDB_AdditiveBackfillNoLoss(t *testing.T) {
	dir := t.TempDir()
	idx := filepath.Join(dir, ".index")
	if err := mkdir(idx); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(idx, "dioptra.db"))
	if err != nil {
		t.Fatal(err)
	}
	// Esquema ANTERIOR a Ola 1 (sin tool/model/project_key) con datos reales.
	_, err = db.Exec(`
	CREATE TABLE reports (
		id TEXT PRIMARY KEY, slug TEXT NOT NULL, project TEXT NOT NULL, title TEXT NOT NULL, ts TEXT NOT NULL,
		max_sev TEXT NOT NULL, excerpt TEXT NOT NULL, md_rel_path TEXT NOT NULL UNIQUE, envelope_json TEXT NOT NULL,
		checks_json TEXT NOT NULL, warnings_count INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL,
		idempotency_key TEXT NOT NULL DEFAULT '', content_hash TEXT NOT NULL DEFAULT '',
		revision INTEGER NOT NULL DEFAULT 1, last_change TEXT NOT NULL DEFAULT ''
	);
	INSERT INTO reports (id,slug,project,title,ts,max_sev,excerpt,md_rel_path,envelope_json,checks_json,updated_at)
	VALUES ('r_old1','viejo','Cartera Core','Viejo','2026-11-01T10:00:00Z','info','x','2026-11/viejo.md','{}','[]','2026-11-01T10:00:00Z'),
	       ('r_old2','otro','cartera-core','Otro','2026-11-02T10:00:00Z','info','x','2026-11/otro.md','{}','[]','2026-11-02T10:00:00Z');`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open sobre BD legacy: %v", err)
	}
	defer st.Close()

	chk, err := sql.Open("sqlite", filepath.Join(idx, "dioptra.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer chk.Close()
	var n int
	if err := chk.QueryRow(`SELECT COUNT(*) FROM reports`).Scan(&n); err != nil || n < 2 {
		t.Fatalf("se perdieron filas legacy: n=%d err=%v", n, err)
	}
	var keyCount int
	if err := chk.QueryRow(`SELECT COUNT(*) FROM reports WHERE id IN ('r_old1','r_old2') AND project_key='cartera-core'`).Scan(&keyCount); err != nil || keyCount != 2 {
		t.Fatalf("backfill de project_key falló: %d err=%v", keyCount, err)
	}
	// Reabrir debe ser idempotente (no falla por columnas ya existentes)
	st.Close()
	st2, err := store.Open(dir)
	if err != nil {
		t.Fatalf("reabrir tras migrar: %v", err)
	}
	st2.Close()
}
