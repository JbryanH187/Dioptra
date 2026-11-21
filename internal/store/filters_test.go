package store_test

import (
	"context"
	"testing"
	"time"

	"dioptra/internal/store"
)

func seed(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	items := []store.PublishParams{
		{Title: "Auditoria A", Project: "Cartera Core", Tool: "Genius Shell", Model: "claude-sonnet-5-5", Markdown: "# A\n\na", Now: base},
		{Title: "Auditoria B", Project: "cartera-core", Tool: "Codex", Model: "gpt-6", Markdown: "# B\n\nb", Now: base.Add(time.Minute)},
		{Title: "Plan C", Project: "Otro", Tool: "Genius Shell", Model: "gpt-6", Markdown: "# C\n\nc", Now: base.Add(2 * time.Minute)},
	}
	for _, p := range items {
		if _, err := st.Publish(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStore_ListReportsFiltered_CombinesWithAND(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seed(t, st)
	ctx := context.Background()

	cases := []struct {
		name                    string
		project, tool, model, q string
		want                    int
	}{
		{"sin filtros", "", "", "", "", 3},
		{"solo proyecto (grafia alterna)", "CARTERA_CORE", "", "", "", 2},
		{"solo tool sin mayusculas", "", "genius shell", "", "", 2},
		{"solo modelo", "", "", "GPT-6", "", 2},
		{"proyecto + tool", "cartera core", "Codex", "", "", 1},
		{"tool + modelo", "", "Genius Shell", "gpt-6", "", 1},
		{"proyecto + tool + modelo sin match", "Otro", "Codex", "gpt-6", "", 0},
		{"texto libre en titulo", "", "", "", "Plan", 1},
		{"texto libre + proyecto", "cartera-core", "", "", "Auditoria B", 1},
	}
	for _, c := range cases {
		got, err := st.ListReportsFiltered(ctx, c.project, c.tool, c.model, c.q, 50)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(got) != c.want {
			t.Errorf("%s: got %d, want %d", c.name, len(got), c.want)
		}
	}
}

func TestStore_ListProjects_GroupsByKeyWithToolsAndModels(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seed(t, st)

	ps, err := st.ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("esperaba 2 proyectos, got %d: %+v", len(ps), ps)
	}
	// Más reciente primero: "Otro" (publicado al final)
	if ps[0].Key != "otro" || ps[0].Reports != 1 {
		t.Fatalf("orden/conteo inesperado: %+v", ps[0])
	}
	cc := ps[1]
	if cc.Key != "cartera-core" || cc.Name != "Cartera Core" || cc.Reports != 2 {
		t.Fatalf("agrupación de grafías falló: %+v", cc)
	}
	if len(cc.Tools) != 2 || len(cc.Models) != 2 {
		t.Fatalf("tools/models agregados mal: %+v", cc)
	}
}
