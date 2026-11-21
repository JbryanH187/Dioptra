package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dioptra/internal/store"
)

func TestStore_DiskPathIsProjectKeyThenMonth(t *testing.T) {
	base := t.TempDir()
	st, err := store.Open(base)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	now := time.Date(2026, 12, 3, 9, 0, 0, 0, time.UTC)

	r1, err := st.Publish(ctx, store.PublishParams{Title: "Informe", Project: "Dioptra Engine", Markdown: "# Informe\n\nv1", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "dioptra-engine", "2026-12", "informe.md")
	if r1.MarkdownPath != want {
		t.Fatalf("ruta: got %s want %s", r1.MarkdownPath, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("archivo no existe: %v", err)
	}

	// Otra grafía del proyecto, mes siguiente: actualiza in-place y NO mueve el archivo
	r2, err := st.Publish(ctx, store.PublishParams{Title: "Informe", Project: "DIOPTRA_ENGINE", Markdown: "# Informe\n\nv2", Now: now.AddDate(0, 1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if r2.ID != r1.ID || r2.MarkdownPath != want || r2.Action != "updated" {
		t.Fatalf("debía actualizar in-place sin mover: %+v", r2)
	}

	// Mismo título en otro proyecto: archivo distinto, sin colisión
	r3, err := st.Publish(ctx, store.PublishParams{Title: "Informe", Project: "Cartera", Markdown: "# Informe\n\nc", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if r3.ID == r1.ID || r3.MarkdownPath != filepath.Join(base, "cartera", "2026-12", "informe.md") {
		t.Fatalf("aislamiento por proyecto roto: %+v", r3)
	}
}
