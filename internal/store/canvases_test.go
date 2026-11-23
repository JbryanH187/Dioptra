package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDesignCanvasesAndAnnotations(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	canvasDir := t.TempDir()
	projDir := filepath.Join(canvasDir, "project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "canvas.json"), []byte(`{"title":"Tsugi","pages":[{"id":"v3","name":"v3"}],"boards":{"Hero.dc.html":{"x":0,"y":0,"w":390,"h":844,"page":"v3"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Registrar lienzo opt-in en tsugi (auto-detecta project/canvas.json).
	c, err := st.UpsertDesignCanvas(ctx, "Tsugi", "tsugi-mvp", "Tsugi MVP", canvasDir, "")
	if err != nil {
		t.Fatalf("UpsertDesignCanvas: %v", err)
	}
	if c.ProjectKey != "tsugi" || c.CanvasID != "tsugi-mvp" || c.EntryJSON != "project/canvas.json" {
		t.Fatalf("canvas inesperado: %+v", c)
	}

	// Rechazar entry_json con path traversal.
	if _, err := st.UpsertDesignCanvas(ctx, "tsugi", "bad", "Bad", canvasDir, "../secret.json"); err == nil {
		t.Fatal("esperaba rechazo para entry_json con ..")
	}

	// Sub-equipo tsugi-ux hereda el lienzo de su workspace padre tsugi.
	if _, err := st.SetWorkspaceGroup(ctx, "tsugi-ux", "tsugi", "ux"); err != nil {
		t.Fatalf("SetWorkspaceGroup: %v", err)
	}
	inherited, err := st.ListDesignCanvases(ctx, "tsugi-ux")
	if err != nil || len(inherited) != 1 || inherited[0].CanvasID != "tsugi-mvp" {
		t.Fatalf("tsugi-ux debía heredar lienzo del padre tsugi: %+v err=%v", inherited, err)
	}

	// Crear anotaciones Pin, Caja y Marcador.
	ann1, err := st.CreateCanvasAnnotation(ctx, CanvasAnnotation{
		ProjectKey:     "tsugi-ux", // vía sub-equipo debe guardarse sobre el lienzo del padre
		CanvasID:       "tsugi-mvp",
		PageID:         "v3",
		BoardFile:      "Hero.dc.html",
		Kind:           "box",
		Geometry:       json.RawMessage(`{"x":0.12,"y":0.62,"w":0.76,"h":0.12}`),
		TargetSelector: "button.cta-primary",
		TargetText:     "Reservar ahora",
		Comment:        "Cambiar a 'Confirmar cita' y subir contraste",
		Author:         "usuario",
	})
	if err != nil {
		t.Fatalf("CreateCanvasAnnotation: %v", err)
	}
	if ann1.ProjectKey != "tsugi" || ann1.Status != "open" || ann1.ID == "" {
		t.Fatalf("anotación inesperada: %+v", ann1)
	}

	// Listar abiertas y comprobar conteo en ListDesignCanvases.
	openList, err := st.ListCanvasAnnotations(ctx, "tsugi", "tsugi-mvp", "open", "")
	if err != nil || len(openList) != 1 {
		t.Fatalf("esperaba 1 abierta, obtuvo %d (err=%v)", len(openList), err)
	}
	canvases, err := st.ListDesignCanvases(ctx, "tsugi")
	if err != nil || len(canvases) != 1 || canvases[0].OpenAnnotations != 1 {
		t.Fatalf("OpenAnnotations esperaba 1: %+v (err=%v)", canvases, err)
	}

	// Marcar como applied por el agente y verificar filtro open -> 0, applied -> 1.
	upd, err := st.UpdateCanvasAnnotation(ctx, "tsugi", "tsugi-mvp", ann1.ID, "applied", "", "Actualizado CTA en Hero.dc.html:44")
	if err != nil {
		t.Fatalf("UpdateCanvasAnnotation: %v", err)
	}
	if upd.Status != "applied" || upd.ResolutionNote == "" {
		t.Fatalf("anotación actualizada inesperada: %+v", upd)
	}
	openAfter, _ := st.ListCanvasAnnotations(ctx, "tsugi", "tsugi-mvp", "open", "")
	if len(openAfter) != 0 {
		t.Fatalf("esperaba 0 abiertas tras aplicar, obtuvo %d", len(openAfter))
	}

	// Borrar anotación.
	deleted, err := st.DeleteCanvasAnnotation(ctx, "tsugi", "tsugi-mvp", ann1.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteCanvasAnnotation falló: deleted=%v err=%v", deleted, err)
	}
}
