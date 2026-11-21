package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dioptra/internal/store"
)

func TestStore_MarkdownSourceOfTruthAndBidirectionalCheck(t *testing.T) {
	baseDir := t.TempDir()
	st, err := store.Open(baseDir)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	fixedTime := time.Date(2026, 11, 18, 0, 20, 0, 0, time.UTC)

	res, err := st.Publish(ctx, store.PublishParams{
		Title:   "Auditoría Almacén SQLite",
		Project: "cartera-core",
		Markdown: strings.Join([]string{
			"# Auditoría Almacén SQLite",
			"",
			"Verificación de índices en internal/store/store.go:84.",
			"",
			"- [ ] Rotar credenciales de lectura @dba [warning]",
			"- [x] Habilitar modo WAL en SQLite @sre [success]",
		}, "\n"),
		Now: fixedTime,
	})
	if err != nil {
		t.Fatalf("Publish falló: %v", err)
	}

	// 1. Verificar ruta {YYYY-MM}/{slug}.md creada primero en disco
	wantSubdir := filepath.Join(baseDir, "cartera-core", "2026-11", "auditoria-almacen-sqlite.md")
	if res.MarkdownPath != wantSubdir {
		t.Fatalf("ruta .md inesperada:\nobtuvo:  %s\nesperaba: %s", res.MarkdownPath, wantSubdir)
	}
	if _, err := os.Stat(wantSubdir); err != nil {
		t.Fatalf("archivo .md no existe en disco: %v", err)
	}

	// 2. Verificar búsqueda filtrada en SQLite
	list, err := st.ListReports(ctx, "cartera-core", "SQLite", 10)
	if err != nil || len(list) != 1 || list[0].ID != res.ID {
		t.Fatalf("ListReports inesperado: err=%v list=%+v", err, list)
	}

	// 3. Actualizar chk-1 -> true y verificar sincronización en SQLite y en el .md en disco
	_, checks, err := st.UpdateChecklistItem(ctx, res.ID, "chk-1", true)
	if err != nil {
		t.Fatalf("UpdateChecklistItem falló: %v", err)
	}
	if len(checks) != 2 || !checks[0].Done || !checks[1].Done {
		t.Fatalf("estado de casillas inesperado tras marcar chk-1: %+v", checks)
	}

	rawDisk, err := os.ReadFile(wantSubdir)
	if err != nil {
		t.Fatalf("leyendo .md: %v", err)
	}
	if !strings.Contains(string(rawDisk), "- [x] Rotar credenciales de lectura @dba [warning]") {
		t.Fatalf("el .md en disco no se sincronizó a [x]:\n%s", string(rawDisk))
	}

	// 4. Depositar un archivo .md suelto en disco y verificar SyncMarkdownFromDisk
	manualDir := filepath.Join(baseDir, "2026-10")
	_ = os.MkdirAll(manualDir, 0o755)
	_ = os.WriteFile(filepath.Join(manualDir, "nota-suelta.md"), []byte("# Nota Suelta\n\nImportada desde disco.\n"), 0o644)

	if err := st.SyncMarkdownFromDisk(ctx); err != nil {
		t.Fatalf("SyncMarkdownFromDisk falló: %v", err)
	}
	all, err := st.ListReports(ctx, "", "", 20)
	if err != nil || len(all) != 2 {
		t.Fatalf("esperaba 2 reportes tras SyncMarkdownFromDisk, obtuvo %d (err=%v)", len(all), err)
	}
}

func TestStore_IdempotentPublishInPlaceUpdateAndDelta(t *testing.T) {
	baseDir := t.TempDir()
	st, err := store.Open(baseDir)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	t1 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 18, 10, 5, 0, 0, time.UTC)
	t3 := time.Date(2026, 11, 18, 10, 10, 0, 0, time.UTC)

	mdV1 := "# Plan de despliegue\n\n## Métricas\n- Latencia p95: 340 ms\n"
	r1, err := st.Publish(ctx, store.PublishParams{
		Title:    "Plan de despliegue",
		Project:  "core-api",
		Markdown: mdV1,
		Now:      t1,
	})
	if err != nil {
		t.Fatalf("Publish v1: %v", err)
	}
	if r1.Action != "created" || r1.Revision != 1 || r1.Summary.UpdatedAt != "" || r1.Summary.LastChange != "" {
		t.Fatalf("creación inicial inesperada: action=%q rev=%d summary=%+v", r1.Action, r1.Revision, r1.Summary)
	}

	// Re-publicar exactamente igual: idempotencia pura (sin duplicar ni subir revisión)
	rSame, err := st.Publish(ctx, store.PublishParams{
		Title:    "Plan de despliegue",
		Project:  "core-api",
		Markdown: mdV1,
		Now:      t2,
	})
	if err != nil {
		t.Fatalf("Publish idéntico: %v", err)
	}
	if rSame.Action != "unchanged" || rSame.ID != r1.ID || rSame.Revision != 1 || rSame.MarkdownPath != r1.MarkdownPath {
		t.Fatalf("idempotencia pura rota: got action=%q id=%s rev=%d path=%s (want id=%s)",
			rSame.Action, rSame.ID, rSame.Revision, rSame.MarkdownPath, r1.ID)
	}

	// Modificar sin ChangeNote: debe actualizar in-place el mismo ID y calcular resumen automático
	mdV2 := "# Plan de despliegue\n\n## Métricas\n- Latencia p95: 190 ms\n- Errores 5xx: 0\n"
	r2, err := st.Publish(ctx, store.PublishParams{
		Title:    "Plan de despliegue",
		Project:  "core-api",
		Markdown: mdV2,
		Now:      t2,
	})
	if err != nil {
		t.Fatalf("Publish v2: %v", err)
	}
	if r2.Action != "updated" || r2.ID != r1.ID || r2.Revision != 2 || r2.MarkdownPath != r1.MarkdownPath {
		t.Fatalf("actualización in-place v2 inesperada: action=%q id=%s rev=%d path=%s", r2.Action, r2.ID, r2.Revision, r2.MarkdownPath)
	}
	if r2.Summary.UpdatedAt != t2.Format(time.RFC3339) || r2.Summary.TS != t1.Format(time.RFC3339) {
		t.Fatalf("timestamps v2 inesperados: created=%q updated=%q", r2.Summary.TS, r2.Summary.UpdatedAt)
	}
	if r2.ChangeNote == "" || !strings.Contains(r2.ChangeNote, "Métricas") {
		t.Fatalf("delta automático esperado con sección tocada, obtuvo %q", r2.ChangeNote)
	}

	// Modificar con ChangeNote explícito y validar persistencia en GetEnvelope + sin duplicados
	r3, err := st.Publish(ctx, store.PublishParams{
		ID:         r1.ID,
		Title:      "Plan de despliegue",
		Project:    "core-api",
		Markdown:   mdV2 + "\n## Cierre\nAprobado en staging.\n",
		ChangeNote: "Se añadió sección de cierre aprobado en staging",
		Now:        t3,
	})
	if err != nil {
		t.Fatalf("Publish v3: %v", err)
	}
	if r3.Action != "updated" || r3.ID != r1.ID || r3.Revision != 3 || r3.ChangeNote != "Se añadió sección de cierre aprobado en staging" {
		t.Fatalf("actualización v3 inesperada: %+v", r3)
	}

	env, err := st.GetEnvelope(ctx, r1.ID)
	if err != nil {
		t.Fatalf("GetEnvelope: %v", err)
	}
	if env.Report.Revision != 3 || env.Report.UpdatedAt != t3.Format(time.RFC3339) || env.Report.LastChange != r3.ChangeNote {
		t.Fatalf("metadatos de actualización ausentes en Envelope: %+v", env.Report)
	}

	list, err := st.ListReports(ctx, "core-api", "", 10)
	if err != nil || len(list) != 1 || list[0].ID != r1.ID || list[0].Revision != 3 {
		t.Fatalf("se duplicaron reportes o faltó revisión en ListReports: err=%v list=%+v", err, list)
	}

	// ForceNew sí debe crear una segunda entrada independiente
	rNew, err := st.Publish(ctx, store.PublishParams{
		Title:    "Plan de despliegue",
		Project:  "core-api",
		Markdown: mdV1,
		ForceNew: true,
		Now:      t3,
	})
	if err != nil {
		t.Fatalf("Publish ForceNew: %v", err)
	}
	if rNew.Action != "created" || rNew.ID == r1.ID {
		t.Fatalf("ForceNew debía crear nuevo reporte: got=%+v", rNew)
	}
	list2, _ := st.ListReports(ctx, "core-api", "", 10)
	if len(list2) != 2 {
		t.Fatalf("esperaba 2 reportes tras ForceNew, obtuvo %d", len(list2))
	}
}
