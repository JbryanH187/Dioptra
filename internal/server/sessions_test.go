package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dioptra/internal/store"
)

func writeStatus(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadAgentSessions_OrdersFiltersAndMarksStale(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	ts := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	writeStatus(t, dir, "a.json", `{"schema":"gentle-shell.agent-status/v1","agent":"genius","session":"s-work","cwd":"C:/repos/pagos-api","state":"working","task":{"title":"Corrigiendo refund"},"updatedAt":"`+ts(time.Minute)+`"}`)
	writeStatus(t, dir, "b.json", `{"schema":"gentle-shell.agent-status/v1","agent":"genius","session":"s-ask","cwd":"C:/repos/checkout","state":"blocked","task":null,"updatedAt":"`+ts(2*time.Minute)+`"}`)
	writeStatus(t, dir, "c.json", `{"schema":"gentle-shell.agent-status/v1","agent":"genius","session":"s-old","cwd":"C:/repos/viejo","state":"working","updatedAt":"`+ts(30*time.Minute)+`"}`)
	writeStatus(t, dir, "d.json", `{"schema":"gentle-shell.agent-status/v1","agent":"genius","session":"s-gone","cwd":"C:/x","state":"idle","updatedAt":"`+ts(48*time.Hour)+`"}`)
	writeStatus(t, dir, "e.json", `{"schema":"otro/v9","session":"s-ajeno"}`)
	writeStatus(t, dir, "f.json", `no es json`)

	got := ReadAgentSessions([]string{dir, filepath.Join(dir, "no-existe")}, now)
	if len(got) != 3 {
		t.Fatalf("esperaba 3 sesiones válidas y recientes, obtuvo %d: %+v", len(got), got)
	}
	if got[0].Session != "s-ask" || got[0].State != "blocked" {
		t.Fatalf("quien te necesita va primero: %+v", got[0])
	}
	if got[1].Session != "s-work" || got[1].Task != "Corrigiendo refund" || got[1].Project != "pagos-api" {
		t.Fatalf("sesión trabajando incorrecta: %+v", got[1])
	}
	if !got[2].Stale || got[2].Session != "s-old" {
		t.Fatalf("la sesión sin actualizar 30 min debe ir al final marcada stale: %+v", got[2])
	}
}

func TestReadAgentSessions_EnrichesTitleAndWorkspace(t *testing.T) {
	base := t.TempDir()
	statusDir := filepath.Join(base, "agent-status")
	sessDir := filepath.Join(base, "sessions")
	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	writeStatus(t, statusDir, "one.json", `{"schema":"gentle-shell.agent-status/v1","agent":"genius-code","session":"abc-123","cwd":"C:/Dev/tsugi-backend","state":"idle","task":null,"updatedAt":"`+now.Add(-time.Minute).Format(time.RFC3339)+`"}`)
	writeStatus(t, sessDir, "abc-123.json", `{"id":"abc-123","title":"Implementar catalogo de servicios"}`)

	got := ReadAgentSessions([]string{statusDir}, now)
	if len(got) != 1 {
		t.Fatalf("esperaba 1 sesión, obtuvo %d", len(got))
	}
	if got[0].SessionTitle != "Implementar catalogo de servicios" {
		t.Fatalf("esperaba SessionTitle leído de sessions/, obtuvo %q", got[0].SessionTitle)
	}

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.SetWorkspaceGroup(context.Background(), "tsugi-backend", "tsugi", "backend"); err != nil {
		t.Fatal(err)
	}
	srv := New(Options{Store: st})
	srv.enrichAgentSessions(context.Background(), got)
	if got[0].CanonicalProject != "tsugi-backend" || got[0].ParentWorkspace != "tsugi" {
		t.Fatalf("enriquecimiento de workspace inesperado: %+v", got[0])
	}
}

func TestReadAgentSessions_ParsesAndBoundsActivity(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	writeStatus(t, dir, "act.json", `{
		"schema":"gentle-shell.agent-status/v1",
		"agent":"genius-code",
		"session":"s-act",
		"cwd":"C:/Dev/tsugi",
		"state":"working",
		"task":{"title":"Implementando lienzo"},
		"activity":[
			{"kind":"subagent","name":"gentle_ai_worker","state":"running","model":"claude-sonnet-5-5","detail":"editando"},
			{"kind":"job","name":"npm test","state":"done"},
			{"kind":"otro","name":"a3","state":"desconocido"},
			{"kind":"subagent","name":"a4","state":"failed"},
			{"kind":"subagent","name":"a5","state":"done"},
			{"kind":"subagent","name":"a6","state":"done"},
			{"kind":"subagent","name":"a7-overflow","state":"running"}
		],
		"updatedAt":"`+now.Add(-10*time.Second).Format(time.RFC3339)+`"
	}`)
	got := ReadAgentSessions([]string{dir}, now)
	if len(got) != 1 {
		t.Fatalf("esperaba 1 sesión, obtuvo %d", len(got))
	}
	if len(got[0].Activity) != maxActivityItems {
		t.Fatalf("esperaba %d registros acotados, obtuvo %d", maxActivityItems, len(got[0].Activity))
	}
	if got[0].Activity[0].Name != "gentle_ai_worker" || got[0].Activity[0].Model != "claude-sonnet-5-5" {
		t.Fatalf("primer registro inesperado: %+v", got[0].Activity[0])
	}
	if got[0].Activity[2].Kind != "subagent" || got[0].Activity[2].State != "running" {
		t.Fatalf("normalización de kind/state falló: %+v", got[0].Activity[2])
	}
}
