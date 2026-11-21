package store_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"dioptra/internal/store"
)

// TestSyncDiskChanges_DetectsManualEdits cubre el hallazgo 8 de la auditoría:
// antes, editar un .md ya indexado nunca actualizaba la vista.
func TestSyncDiskChanges_DetectsManualEdits(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	res, err := st.Publish(ctx, store.PublishParams{Title: "Plan", Project: "disco", Markdown: "# Plan\n\nVersión original."})
	if err != nil {
		t.Fatal(err)
	}
	if ch, err := st.SyncDiskChanges(ctx); err != nil || len(ch) != 0 {
		t.Fatalf("la primera pasada solo registra mtimes: changes=%v err=%v", ch, err)
	}

	edited := "# Plan\n\nVersión original.\n\nPárrafo agregado a mano en el editor."
	if err := os.WriteFile(res.MarkdownPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(res.MarkdownPath, future, future)

	changes, err := st.SyncDiskChanges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Action != "updated" || changes[0].Summary.ID != res.ID {
		t.Fatalf("se esperaba 1 actualización del mismo reporte, obtuvo %+v", changes)
	}
	if changes[0].Summary.Project != "disco" || changes[0].Summary.Title != "Plan" {
		t.Fatalf("la edición debe conservar proyecto y título: %+v", changes[0].Summary)
	}
	state, err := st.GetReportState(ctx, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(state.Markdown, "agregado a mano") {
		t.Fatal("el estado del reporte no refleja la edición en disco")
	}

	// Sin cambios nuevos no debe volver a dispararse (aunque Publish reescriba el archivo).
	if again, _ := st.SyncDiskChanges(ctx); len(again) != 0 {
		t.Fatalf("sin ediciones nuevas no debe haber cambios: %+v", again)
	}
}

// TestSyncDiskChanges_PreservesBlocksOverrideOnUpdate verifica la regresión donde
// actualizar un reporte con blocks_override/free-canvas disparaba SyncDiskChanges
// segundos después y lo degradaba a fast_goldmark ("Editado directamente en disco").
func TestSyncDiskChanges_PreservesBlocksOverrideOnUpdate(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	mdV1 := "# Visor Térmico\n\nTotal de tickets: 3\n\n- [ ] Revisar folio 67467"
	blocksV1 := []any{
		map[string]any{"t": "kpi", "items": []any{map[string]any{"label": "Tickets", "value": 3}}},
		map[string]any{"t": "prose", "md": "Total de tickets: 3"},
		map[string]any{"t": "canvas", "h": 360, "html": "<div>ticket-v1</div>"},
		map[string]any{"t": "checklist", "items": []any{map[string]any{"id": "chk-1", "text": "Revisar folio 67467", "done": false}}},
	}
	r1, err := st.Publish(ctx, store.PublishParams{
		Title:          "Visor Térmico",
		Project:        "abono",
		Markdown:       mdV1,
		BlocksOverride: blocksV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ch, err := st.SyncDiskChanges(ctx); err != nil || len(ch) != 0 {
		t.Fatalf("primera pasada no debe degradar blocks_override: ch=%+v err=%v", ch, err)
	}

	// Rev. 2: actualiza blocks_override manteniendo el mismo markdown y cambiando el canvas.
	blocksV2 := []any{
		map[string]any{"t": "kpi", "items": []any{map[string]any{"label": "Tickets", "value": 3}}},
		map[string]any{"t": "prose", "md": "Total de tickets: 3"},
		map[string]any{"t": "canvas", "h": 420, "html": "<div>ticket-v2</div>"},
		map[string]any{"t": "checklist", "items": []any{map[string]any{"id": "chk-1", "text": "Revisar folio 67467", "done": false}}},
	}
	r2, err := st.Publish(ctx, store.PublishParams{
		ID:             r1.ID,
		Title:          "Visor Térmico",
		Project:        "abono",
		Markdown:       mdV1,
		BlocksOverride: blocksV2,
		ChangeNote:     "Ajuste visual en canvas v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Action != "updated" || r2.Revision != 2 || r2.ModeUsed != "blocks_override" {
		t.Fatalf("rev2 inesperada: action=%s rev=%d mode=%s", r2.Action, r2.Revision, r2.ModeUsed)
	}

	// Incluso si el filesystem adelanta el mtime del .md tras el update, SyncDiskChanges NO debe pisar blocks_override.
	future := time.Now().Add(3 * time.Second)
	_ = os.Chtimes(r2.MarkdownPath, future, future)
	if ch, err := st.SyncDiskChanges(ctx); err != nil || len(ch) != 0 {
		t.Fatalf("SyncDiskChanges no debe generar actualización fantasma tras Publish blocks_override: ch=%+v err=%v", ch, err)
	}

	env, err := st.GetEnvelope(ctx, r1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if env.Report.Revision != 2 || env.Report.LastChange != "Ajuste visual en canvas v2" {
		t.Fatalf("revisión/nota alteradas por SyncDiskChanges: rev=%d lastChange=%q", env.Report.Revision, env.Report.LastChange)
	}
	hasCanvas := false
	for _, b := range env.Report.Blocks {
		if b["t"] == "canvas" {
			hasCanvas = true
		}
	}
	if !hasCanvas {
		t.Fatalf("el bloque free-canvas fue borrado por SyncDiskChanges: %+v", env.Report.Blocks)
	}
	if html, ok := st.LookupCanvasHTML(ctx, r1.ID, "b_1"); !ok || !strings.Contains(html, "ticket-v2") {
		t.Fatalf("el HTML del canvas v2 se perdió: ok=%v html=%q", ok, html)
	}

	// Rev. 3: actualiza BOTH markdown y blocks_override → SyncDiskChanges debe respetar la escritura propia.
	mdV3 := "# Visor Térmico\n\nTotal de tickets: 4\n\n- [ ] Revisar folio 67467"
	blocksV3 := []any{
		map[string]any{"t": "kpi", "items": []any{map[string]any{"label": "Tickets", "value": 4}}},
		map[string]any{"t": "prose", "md": "Total de tickets: 4"},
		map[string]any{"t": "canvas", "h": 420, "html": "<div>ticket-v3</div>"},
		map[string]any{"t": "checklist", "items": []any{map[string]any{"id": "chk-1", "text": "Revisar folio 67467", "done": false}}},
	}
	r3, err := st.Publish(ctx, store.PublishParams{
		ID:             r1.ID,
		Title:          "Visor Térmico",
		Project:        "abono",
		Markdown:       mdV3,
		BlocksOverride: blocksV3,
		ChangeNote:     "Agrega cuarto ticket",
	})
	if err != nil || r3.Revision != 3 {
		t.Fatalf("rev3 inesperada: r3=%+v err=%v", r3, err)
	}
	if ch, err := st.SyncDiskChanges(ctx); err != nil || len(ch) != 0 {
		t.Fatalf("SyncDiskChanges no debe re-publicar rev3 propia: ch=%+v err=%v", ch, err)
	}
	if html, ok := st.LookupCanvasHTML(ctx, r1.ID, "b_1"); !ok || !strings.Contains(html, "ticket-v3") {
		t.Fatalf("el HTML del canvas v3 se perdió: ok=%v html=%q", ok, html)
	}
}

// TestSyncDiskChanges_PreservesBlocksOverrideAfterChecklistToggle verifica que marcar una
// casilla desde la UI no dispare SyncDiskChanges ni borre el bloque free-canvas.
func TestSyncDiskChanges_PreservesBlocksOverrideAfterChecklistToggle(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	md := "# Checklist Canvas\n\nEstado inicial.\n\n- [ ] Aprobar corrección"
	blocks := []any{
		map[string]any{"t": "prose", "md": "Estado inicial."},
		map[string]any{"t": "canvas", "h": 300, "html": "<div>visor-intacto</div>"},
		map[string]any{"t": "checklist", "items": []any{map[string]any{"id": "chk-1", "text": "Aprobar corrección", "done": false}}},
	}
	r, err := st.Publish(ctx, store.PublishParams{
		Title:          "Checklist Canvas",
		Project:        "abono",
		Markdown:       md,
		BlocksOverride: blocks,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = st.SyncDiskChanges(ctx)

	if _, _, err := st.UpdateChecklistItem(ctx, r.ID, "chk-1", true); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(3 * time.Second)
	_ = os.Chtimes(r.MarkdownPath, future, future)

	if ch, err := st.SyncDiskChanges(ctx); err != nil || len(ch) != 0 {
		t.Fatalf("SyncDiskChanges no debe pisar reporte tras UpdateChecklistItem: ch=%+v err=%v", ch, err)
	}
	if html, ok := st.LookupCanvasHTML(ctx, r.ID, "b_1"); !ok || !strings.Contains(html, "visor-intacto") {
		t.Fatalf("UpdateChecklistItem + SyncDiskChanges borró el free-canvas: ok=%v html=%q", ok, html)
	}
	state, err := st.GetReportState(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Checklists) != 1 || !state.Checklists[0].Done {
		t.Fatalf("la casilla aprobada no persistió: %+v", state.Checklists)
	}
}
