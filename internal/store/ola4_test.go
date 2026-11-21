package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"dioptra/internal/store"
)

func openStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, dir
}

func pub(t *testing.T, st *store.Store, title, project, tool, model, md string, at time.Time) *store.PublishResult {
	t.Helper()
	r, err := st.Publish(context.Background(), store.PublishParams{Title: title, Project: project, Tool: tool, Model: model, Markdown: md, Now: at})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMigrate_SetsUserVersionAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := st.SchemaVersion()
	if err != nil || v != store.LatestSchemaVersion() {
		t.Fatalf("user_version=%d err=%v, esperaba %d", v, err, store.LatestSchemaVersion())
	}
	_ = st.Close()

	st2, err := store.Open(dir) // reabrir no debe fallar ni cambiar la versión
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if v2, _ := st2.SchemaVersion(); v2 != v {
		t.Fatalf("la versión cambió al reabrir: %d -> %d", v, v2)
	}
}

func TestMigrate_UpgradesLegacyDatabaseWithoutLosingRows(t *testing.T) {
	dir := t.TempDir()
	idx := filepath.Join(dir, ".index")
	if err := mkdir(idx); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(idx, "dioptra.db")))
	if err != nil {
		t.Fatal(err)
	}
	// Esquema mínimo previo a Ola 1 (sin tool/model/project_key) y user_version = 0.
	_, err = db.Exec(`CREATE TABLE reports (
		id TEXT PRIMARY KEY, slug TEXT NOT NULL, project TEXT NOT NULL, title TEXT NOT NULL, ts TEXT NOT NULL,
		max_sev TEXT NOT NULL, excerpt TEXT NOT NULL, md_rel_path TEXT NOT NULL UNIQUE, envelope_json TEXT NOT NULL,
		checks_json TEXT NOT NULL DEFAULT '[]', warnings_count INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL DEFAULT '');
		INSERT INTO reports (id,slug,project,title,ts,max_sev,excerpt,md_rel_path,envelope_json,updated_at)
		VALUES ('r1','viejo','Dioptra Engine','Viejo','2026-11-01T10:00:00Z','info','x','2026-11/viejo.md','{}','2026-11-01T10:00:00Z');`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("migrar base legacy: %v", err)
	}
	defer st.Close()
	got, err := st.ListReportsFiltered(context.Background(), "dioptra-engine", "", "", "", 10)
	if err != nil || len(got) != 1 || got[0].ID != "r1" {
		t.Fatalf("se perdió la fila legacy o no se hizo backfill: %+v err=%v", got, err)
	}
}

func TestAliases_UnifyProjectsWithoutRewritingRows(t *testing.T) {
	st, _ := openStore(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	pub(t, st, "Uno", "dioptra", "Genius Shell", "m1", "# Uno\n\na", now)
	pub(t, st, "Dos", "DioptraEngine", "Codex", "m2", "# Dos\n\nb", now.Add(time.Minute))

	if ps, _ := st.ListProjects(ctx); len(ps) != 2 {
		t.Fatalf("antes del alias esperaba 2 proyectos, got %d", len(ps))
	}

	if _, err := st.SetProjectAlias(ctx, "DioptraEngine", "dioptra"); err != nil {
		t.Fatal(err)
	}
	ps, err := st.ListProjects(ctx)
	if err != nil || len(ps) != 1 || ps[0].Key != "dioptra" || ps[0].Reports != 2 || len(ps[0].Aliases) != 1 {
		t.Fatalf("agrupación con alias falló: %+v err=%v", ps, err)
	}
	for _, name := range []string{"dioptra", "DioptraEngine", "Dioptra-Engine"} {
		got, _ := st.ListReportsFiltered(ctx, name, "", "", "", 10)
		want := 2
		if name == "Dioptra-Engine" { // otra grafía distinta: no es alias
			want = 0
		}
		if len(got) != want {
			t.Errorf("filtro %q: got %d want %d", name, len(got), want)
		}
	}

	// Retirar el alias revierte la agrupación (las filas nunca se tocaron).
	if ok, err := st.RemoveProjectAlias(ctx, "DioptraEngine"); err != nil || !ok {
		t.Fatalf("remove: ok=%v err=%v", ok, err)
	}
	if ps, _ := st.ListProjects(ctx); len(ps) != 2 {
		t.Fatalf("tras retirar el alias esperaba 2 proyectos, got %d", len(ps))
	}

	// Con el alias de nuevo, republicar actualiza in-place el reporte histórico, sin duplicar.
	if _, err := st.SetProjectAlias(ctx, "DioptraEngine", "dioptra"); err != nil {
		t.Fatal(err)
	}
	r := pub(t, st, "Dos", "DioptraEngine", "Codex", "m2", "# Dos\n\nb v2", now.Add(time.Hour))
	if r.Action != "updated" {
		t.Fatalf("debía actualizar in-place, action=%s", r.Action)
	}
	if all, _ := st.ListReportsFiltered(ctx, "dioptra", "", "", "", 10); len(all) != 2 {
		t.Fatalf("se duplicó al republicar: %d", len(all))
	}
	// Documentado: lo republicado queda bajo la clave canónica y retirar el alias no lo revierte.
	_, _ = st.RemoveProjectAlias(ctx, "DioptraEngine")
	if ps, _ := st.ListProjects(ctx); len(ps) != 1 {
		t.Fatalf("lo republicado bajo la canónica debía quedarse en ella, proyectos=%d", len(ps))
	}
}

func TestAliases_RejectChainsCyclesAndSlugCollisions(t *testing.T) {
	st, _ := openStore(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	pub(t, st, "Mismo titulo", "a", "", "", "# T\n\na", now)
	pub(t, st, "Mismo titulo", "b", "", "", "# T\n\nb", now)

	if _, err := st.SetProjectAlias(ctx, "a", "a"); err == nil {
		t.Error("alias igual a la canónica debía rechazarse")
	}
	if _, err := st.SetProjectAlias(ctx, "a", "b"); err == nil || !strings.Contains(err.Error(), "colisión") {
		t.Errorf("colisión de slug debía rechazarse, err=%v", err)
	}
	pub(t, st, "Otro", "c", "", "", "# O\n\nc", now)
	if _, err := st.SetProjectAlias(ctx, "c", "b"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetProjectAlias(ctx, "b", "c"); err == nil {
		t.Error("el ciclo b->c con c->b debía rechazarse")
	}
	if _, err := st.SetProjectAlias(ctx, "x", "c"); err == nil {
		t.Error("apuntar a un alias (cadena) debía rechazarse")
	}
}

func TestSearch_BM25RanksTitleAboveBodyAndKeepsFilters(t *testing.T) {
	st, _ := openStore(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	pub(t, st, "Notas varias", "p", "Codex", "m1", "# Notas\n\nse menciona latencia una vez", now)
	pub(t, st, "Latencia del gateway", "p", "Genius Shell", "m1", "# Latencia\n\nlatencia latencia", now.Add(time.Minute))
	pub(t, st, "Otra cosa", "p", "Codex", "m1", "# Otra\n\nsin relacion", now.Add(2*time.Minute))

	got, err := st.ListReportsFiltered(ctx, "", "", "", "latencia", 10)
	if err != nil || len(got) != 2 {
		t.Fatalf("esperaba 2 coincidencias, got %d err=%v", len(got), err)
	}
	if got[0].Title != "Latencia del gateway" {
		t.Errorf("BM25 debía poner primero el título: %q", got[0].Title)
	}
	// prefijo y acentos
	if g, _ := st.ListReportsFiltered(ctx, "", "", "", "latenc", 10); len(g) != 2 {
		t.Errorf("búsqueda por prefijo: %d", len(g))
	}
	// AND con filtros
	if g, _ := st.ListReportsFiltered(ctx, "", "Codex", "", "latencia", 10); len(g) != 1 || g[0].Title != "Notas varias" {
		t.Errorf("AND con tool falló: %+v", g)
	}
	// La sintaxis FTS del usuario no debe romper ni inyectar operadores
	for _, q := range []string{`latencia"`, `OR AND NOT`, `*`, `título: (x`} {
		if _, err := st.ListReportsFiltered(ctx, "", "", "", q, 10); err != nil {
			t.Errorf("consulta %q devolvió error: %v", q, err)
		}
	}
}
