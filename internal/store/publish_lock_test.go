package store_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"dioptra/internal/layoutagent"
	"dioptra/internal/store"
)

// TestPublish_SlowLayoutAgentDoesNotBlockOtherPublishes reproduce el hallazgo 7 de
// docs/AUDITORIA_ETAPA2.md: antes, una publicación interactive_mode mantenía s.mu
// durante toda la llamada al LLM y cualquier otra publicación esperaba (16 s medidos).
func TestPublish_SlowLayoutAgentDoesNotBlockOtherPublishes(t *testing.T) {
	const llmDelay = 2 * time.Second
	var llmCalls atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalls.Add(1)
		select {
		case <-time.After(llmDelay):
		case <-r.Context().Done():
		}
		http.Error(w, `{"error":"fake"}`, http.StatusInternalServerError)
	}))
	defer llm.Close()

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	ctx := context.Background()
	cfg := layoutagent.Config{
		Provider: "openai-compatible",
		APIURL:   llm.URL + "/v1/chat/completions",
		APIKey:   "test",
		Model:    "fake",
		Timeout:  10 * time.Second,
	}

	slowDone := make(chan *store.PublishResult, 1)
	slowErr := make(chan error, 1)
	go func() {
		res, err := st.Publish(ctx, store.PublishParams{
			Title: "Lento", Project: "lock", Markdown: "# Lento\n\ncontenido", InteractiveMode: true, LayoutCfg: cfg,
		})
		slowErr <- err
		slowDone <- res
	}()

	// Esperar a que el LLM falso esté atendiendo la síntesis.
	deadline := time.Now().Add(3 * time.Second)
	for llmCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if llmCalls.Load() == 0 {
		t.Fatal("el LLM falso nunca recibió la síntesis")
	}

	start := time.Now()
	if _, err := st.Publish(ctx, store.PublishParams{Title: "Rápido", Project: "lock", Markdown: "# Rápido\n\nok"}); err != nil {
		t.Fatalf("publish rápido: %v", err)
	}
	if el := time.Since(start); el > llmDelay/2 {
		t.Fatalf("una publicación normal esperó %v detrás de la síntesis lenta (debería ser inmediata)", el)
	}

	if err := <-slowErr; err != nil {
		t.Fatalf("publish lento: %v", err)
	}
	res := <-slowDone
	if res.ModeUsed != "fast_goldmark" {
		t.Fatalf("con LLM fallido debía degradar a fast_goldmark, obtuvo %q", res.ModeUsed)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("se esperaba un warning de degradación de interactive_mode")
	}

	// Republicar el mismo contenido no debe volver a llamar al LLM (idempotencia).
	before := llmCalls.Load()
	again, err := st.Publish(ctx, store.PublishParams{
		Title: "Lento", Project: "lock", Markdown: "# Lento\n\ncontenido", InteractiveMode: true, LayoutCfg: cfg,
	})
	if err != nil {
		t.Fatalf("republish: %v", err)
	}
	if again.Action != "unchanged" {
		t.Fatalf("esperaba action=unchanged, obtuvo %q", again.Action)
	}
	if got := llmCalls.Load(); got != before {
		t.Fatalf("republicar contenido idéntico llamó al LLM %d vez(es) más", got-before)
	}
}
