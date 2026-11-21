package layoutagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"dioptra/internal/contract"
	"dioptra/internal/cost"
	"dioptra/internal/layoutagent"
	"dioptra/internal/store"
	"dioptra/ui"
)

const fixtureMD = "# Auditoría de latencia\n\n" +
	"La latencia p95 subió a 340 ms en internal/store/store.go:622 tras el cambio; el throughput cayó a 1,250 req/s y el error rate es 2.5%.\n\n" +
	"> [!WARNING] Riesgo de saturación\n> La cola supera 80% de capacidad.\n\n" +
	"```go\nfunc Publish() error {\n\treturn nil\n}\n```\n\n" +
	"- [ ] Revisar índice FTS5 @dba\n" +
	"- [x] Medir p95 en staging\n" +
	"- [ ] Ajustar pool de conexiones\n"

func faithfulBlocks() []any {
	return []any{
		map[string]any{"t": "kpi", "items": []any{
			map[string]any{"label": "Latencia p95", "value": 340, "unit": "ms", "sev": "warning"},
			map[string]any{"label": "Throughput", "value": "1,250", "unit": "req/s"},
			map[string]any{"label": "Error rate", "value": "2.5%"},
		}},
		map[string]any{"t": "callout", "sev": "warning", "title": "Riesgo de saturación", "body": "La cola supera 80% de capacidad."},
		map[string]any{"t": "prose", "md": "La latencia p95 subió a 340 ms en internal/store/store.go:622 tras el cambio; el throughput cayó a 1,250 req/s y el error rate es 2.5%."},
		map[string]any{"t": "code", "lang": "go", "src": "func Publish() error {\n\treturn nil\n}"},
		map[string]any{"t": "checklist", "items": []any{
			map[string]any{"id": "chk-1", "text": "Revisar índice FTS5", "done": false, "owner": "@dba"},
			map[string]any{"id": "chk-2", "text": "Medir p95 en staging", "done": true},
			map[string]any{"id": "chk-3", "text": "Ajustar pool de conexiones", "done": false},
		}},
	}
}

func envelopeJSON(t *testing.T, blocks []any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"v":      1,
		"report": map[string]any{"title": "Auditoría de latencia", "ts": "2026-11-18T10:00:00Z", "blocks": blocks},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func input() layoutagent.Input {
	return layoutagent.Input{Title: "Auditoría de latencia", Project: "suite", TS: "2026-11-18T10:00:00Z", Markdown: fixtureMD}
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	raw, _ := io.ReadAll(r.Body)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("cuerpo no es JSON: %v", err)
	}
	return m
}

func checklistIDs(env *contract.Envelope) []string {
	var ids []string
	for _, c := range contract.CollectChecklists(env.Report.Blocks) {
		ids = append(ids, c.ID)
	}
	return ids
}

func findBlocks(blocks []map[string]any, t string) []map[string]any {
	var out []map[string]any
	for _, b := range blocks {
		if bt, _ := b["t"].(string); bt == t {
			out = append(out, b)
		}
		switch sub := b["blocks"].(type) {
		case []map[string]any:
			out = append(out, findBlocks(sub, t)...)
		case []any:
			var conv []map[string]any
			for _, x := range sub {
				if m, ok := x.(map[string]any); ok {
					conv = append(conv, m)
				}
			}
			out = append(out, findBlocks(conv, t)...)
		}
	}
	return out
}

func itemsOf(b map[string]any) []map[string]any {
	switch x := b["items"].(type) {
	case []map[string]any:
		return x
	case []any:
		var out []map[string]any
		for _, it := range x {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// ---------------------------------------------------------------------------
// Proveedores
// ---------------------------------------------------------------------------

func TestGemini_NativeGenerateContentForcesJSON(t *testing.T) {
	env := envelopeJSON(t, faithfulBlocks())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1beta/models/gemini-test:generateContent") {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "k-gem" {
			t.Errorf("falta cabecera x-goog-api-key")
		}
		if r.URL.Query().Get("key") != "" {
			t.Errorf("la API key no debe viajar en la query string")
		}
		body := decodeBody(t, r)
		gen, _ := body["generationConfig"].(map[string]any)
		if gen["responseMimeType"] != "application/json" {
			t.Errorf("responseMimeType no forzado: %v", gen)
		}
		sys, _ := json.Marshal(body["systemInstruction"])
		if !strings.Contains(string(sys), "Agente Único de Dioptra") {
			t.Errorf("systemInstruction sin prompt maestro")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []any{map[string]any{
				"content": map[string]any{"parts": []any{
					map[string]any{"text": "razonamiento interno {no json}", "thought": true},
					map[string]any{"text": env},
				}},
				"finishReason": "STOP",
			}},
			"usageMetadata": map[string]any{"promptTokenCount": 3200, "candidatesTokenCount": 900, "cachedContentTokenCount": 2800},
		})
	}))
	defer srv.Close()

	cfg := layoutagent.Config{Provider: "gemini", APIURL: srv.URL, APIKey: "k-gem", Model: "gemini-test", Timeout: 5 * time.Second}
	res, err := layoutagent.Synthesize(context.Background(), cfg, input())
	if err != nil {
		t.Fatalf("Synthesize gemini: %v", err)
	}
	if res.Meta.Provider != layoutagent.ProviderGemini || res.Meta.InputTokens != 3200 || res.Meta.CacheReadTokens != 2800 {
		t.Fatalf("meta inesperada: %+v", res.Meta)
	}
	if got := strings.Join(checklistIDs(res.Envelope), ","); got != "chk-1,chk-2,chk-3" {
		t.Fatalf("ids de checklist: %s", got)
	}
	if res.Meta.FidelityRepairs != 0 {
		t.Fatalf("una salida fiel no debe requerir reparaciones: %+v", res.Warnings)
	}
}

func TestAnthropic_ToolUseWithEphemeralPromptCacheAndAutodetect(t *testing.T) {
	var envObj map[string]any
	_ = json.Unmarshal([]byte(envelopeJSON(t, faithfulBlocks())), &envObj)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "k-ant" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("cabeceras anthropic incorrectas: %v", r.Header)
		}
		body := decodeBody(t, r)
		sys, _ := body["system"].([]any)
		first, _ := sys[0].(map[string]any)
		cc, _ := first["cache_control"].(map[string]any)
		if cc["type"] != "ephemeral" {
			t.Errorf("bloque de sistema sin cache_control efímero: %v", first)
		}
		tc, _ := body["tool_choice"].(map[string]any)
		if tc["type"] != "tool" || tc["name"] != "emit_dioptra_envelope" {
			t.Errorf("tool_choice no fuerza la herramienta estructurada: %v", tc)
		}
		if _, ok := body["max_tokens"]; !ok {
			t.Errorf("anthropic exige max_tokens")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":     []any{map[string]any{"type": "tool_use", "name": "emit_dioptra_envelope", "input": envObj}},
			"stop_reason": "tool_use",
			"usage":       map[string]any{"input_tokens": 420, "output_tokens": 950, "cache_read_input_tokens": 3100},
		})
	}))
	defer srv.Close()

	// Sin DIOPTRA_LAYOUT_PROVIDER: se autodetecta anthropic por el nombre del modelo
	cfg := layoutagent.Config{APIURL: srv.URL, APIKey: "k-ant", Model: "claude-test", Timeout: 5 * time.Second}
	res, err := layoutagent.Synthesize(context.Background(), cfg, input())
	if err != nil {
		t.Fatalf("Synthesize anthropic: %v", err)
	}
	if res.Meta.Provider != layoutagent.ProviderAnthropic || res.Meta.CacheReadTokens != 3100 {
		t.Fatalf("meta inesperada: %+v", res.Meta)
	}
	if len(findBlocks(res.Envelope.Report.Blocks, "kpi")) != 1 {
		t.Fatalf("se esperaba kpi-grid en la salida")
	}
}

func TestAnthropic_PrefillModeRebuildsJSON(t *testing.T) {
	env := envelopeJSON(t, faithfulBlocks())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(t, r)
		if _, hasTools := body["tools"]; hasTools {
			t.Errorf("modo prefill no debe declarar herramientas")
		}
		msgs, _ := body["messages"].([]any)
		last, _ := msgs[len(msgs)-1].(map[string]any)
		if last["role"] != "assistant" || last["content"] != "{" {
			t.Errorf("falta prellenado '{': %v", last)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":     []any{map[string]any{"type": "text", "text": strings.TrimPrefix(env, "{")}},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()

	cfg := layoutagent.Config{Provider: "anthropic", APIURL: srv.URL, APIKey: "k-ant", Model: "claude-test", AnthropicMode: "prefill", Timeout: 5 * time.Second}
	if _, err := layoutagent.Synthesize(context.Background(), cfg, input()); err != nil {
		t.Fatalf("Synthesize anthropic prefill: %v", err)
	}
}

func TestOpenAICompatible_JSONModeAndDirtyFences(t *testing.T) {
	env := envelopeJSON(t, faithfulBlocks())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer k-oai" {
			t.Errorf("falta Authorization Bearer")
		}
		body := decodeBody(t, r)
		rf, _ := body["response_format"].(map[string]any)
		if rf["type"] != "json_object" {
			t.Errorf("response_format no forzado: %v", rf)
		}
		dirty := "Claro, aquí tienes el sobre:\n```json\n" + env + "\n```\nQuedo atento."
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": dirty}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 3000, "completion_tokens": 800, "prompt_tokens_details": map[string]any{"cached_tokens": 2048}},
		})
	}))
	defer srv.Close()

	cfg := layoutagent.Config{Provider: "openai-compatible", APIURL: srv.URL + "/v1", APIKey: "k-oai", Model: "local-llm", Timeout: 5 * time.Second}
	res, err := layoutagent.Synthesize(context.Background(), cfg, input())
	if err != nil {
		t.Fatalf("Synthesize openai-compatible con cercas sucias: %v", err)
	}
	if res.Meta.CacheReadTokens != 2048 || res.Meta.Provider != layoutagent.ProviderOpenAI {
		t.Fatalf("meta inesperada: %+v", res.Meta)
	}
}

func TestRetryOnTransient503(t *testing.T) {
	env := envelopeJSON(t, faithfulBlocks())
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, `{"error":"overloaded"}`, http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": env}, "finish_reason": "stop"}},
		})
	}))
	defer srv.Close()

	cfg := layoutagent.Config{APIURL: srv.URL + "/v1/chat/completions", Model: "gw-model", Timeout: 5 * time.Second}
	res, err := layoutagent.Synthesize(context.Background(), cfg, input())
	if err != nil {
		t.Fatalf("debía recuperarse tras un 503: %v", err)
	}
	if res.Meta.Attempts != 2 {
		t.Fatalf("intentos esperados 2, obtenidos %d", res.Meta.Attempts)
	}
}

func TestTruncatedOutputIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": `{"v":1,"report":{"title":"x","blocks":[`}, "finish_reason": "length"}},
		})
	}))
	defer srv.Close()
	cfg := layoutagent.Config{APIURL: srv.URL + "/v1/chat/completions", Model: "gw-model", Timeout: 5 * time.Second}
	_, err := layoutagent.Synthesize(context.Background(), cfg, input())
	if !errors.Is(err, layoutagent.ErrTruncated) {
		t.Fatalf("se esperaba ErrTruncated, obtenido: %v", err)
	}
}

func TestTimeoutIsBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	defer srv.Close()
	cfg := layoutagent.Config{APIURL: srv.URL + "/v1/chat/completions", Model: "slow", Timeout: 300 * time.Millisecond}
	started := time.Now()
	_, err := layoutagent.Synthesize(context.Background(), cfg, input())
	if err == nil || !strings.Contains(err.Error(), "tiempo límite") {
		t.Fatalf("se esperaba error de tiempo límite, obtenido: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("el tiempo límite no se respetó: %s", elapsed)
	}
}

func TestProviderErrorsNeverLeakAPIKey(t *testing.T) {
	const secret = "sk-super-secret-123456"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid key `+secret+`"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	cfg := layoutagent.Config{APIURL: srv.URL + "/v1/chat/completions", APIKey: secret, Model: "m", Timeout: 2 * time.Second}
	_, err := layoutagent.Synthesize(context.Background(), cfg, input())
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "401") {
		t.Fatalf("error debe reportar 401 sin filtrar la clave: %v", err)
	}
}

func TestNetworkFailureDegradesCleanlyToGoldmark(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := srv.URL
	srv.Close() // caída de red: conexión rechazada

	cfg := layoutagent.Config{Provider: "openai-compatible", APIURL: deadURL + "/v1/chat/completions", Model: "m", Timeout: 2 * time.Second}
	if _, err := layoutagent.Synthesize(context.Background(), cfg, input()); err == nil {
		t.Fatalf("se esperaba error de red")
	}

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	res, err := st.Publish(context.Background(), store.PublishParams{
		Title: "Auditoría de latencia", Project: "suite", Markdown: fixtureMD,
		InteractiveMode: true, LayoutCfg: cfg,
	})
	if err != nil {
		t.Fatalf("Publish no debe abortar ante caída del agente: %v", err)
	}
	if res.ModeUsed != "fast_goldmark" {
		t.Fatalf("modo esperado fast_goldmark, obtenido %s", res.ModeUsed)
	}
	degraded := false
	for _, w := range res.Warnings {
		if w.Path == "interactive_mode" && w.Action == "degraded" {
			degraded = true
		}
	}
	if !degraded {
		t.Fatalf("falta advertencia de degradación: %+v", res.Warnings)
	}
	if res.Layout == nil || res.Layout.Attempts != 2 {
		t.Fatalf("telemetría del intento fallido ausente: %+v", res.Layout)
	}
	if got := strings.Join(checklistIDs(res.Envelope), ","); got != "chk-1,chk-2,chk-3" {
		t.Fatalf("la ruta goldmark debe conservar las tareas: %s", got)
	}
}

// ---------------------------------------------------------------------------
// Guardián de Fidelidad
// ---------------------------------------------------------------------------

func TestFidelityGuardRepairsUnfaithfulModelOutput(t *testing.T) {
	unfaithful := []any{
		map[string]any{"t": "kpi", "items": []any{
			map[string]any{"label": "Latencia p95", "value": 340, "unit": "ms"},
			map[string]any{"label": "Ahorro estimado", "value": 999, "unit": "USD"}, // inventado
		}},
		map[string]any{"t": "callout", "sev": "warning", "title": "Riesgo de saturación", "body": "La cola supera 80% de capacidad."},
		map[string]any{"t": "prose", "md": "La latencia p95 subió a 340 ms tras el cambio; el throughput cayó a 1,250 req/s y el error rate es 2.5%."},
		map[string]any{"t": "code", "lang": "go", "src": "func Publish() error { ... }"}, // resumido
		map[string]any{"t": "checklist", "items": []any{
			map[string]any{"id": "a", "text": "Medir p95 en staging", "done": false},     // id y estado erróneos
			map[string]any{"id": "b", "text": "Migrar todo a Kubernetes", "done": false}, // inventada
		}},
		map[string]any{"t": "canvas", "html": "<div style=\"color:#fff\">sim</div>", "h": 300},
		map[string]any{"t": "canvas", "html": "<script src=\"https://cdn.x/lib.js\"></script>", "h": 300},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": envelopeJSON(t, unfaithful)}, "finish_reason": "stop"}},
		})
	}))
	defer srv.Close()

	cfg := layoutagent.Config{APIURL: srv.URL + "/v1/chat/completions", Model: "m", Timeout: 5 * time.Second}
	res, err := layoutagent.Synthesize(context.Background(), cfg, input())
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	env := res.Envelope

	// F4: ids canónicos, estado del .md, sin tareas inventadas y con las omitidas reinyectadas
	checks := contract.CollectChecklists(env.Report.Blocks)
	byID := map[string]contract.ChecklistItemState{}
	for _, c := range checks {
		byID[c.ID] = c
	}
	if len(checks) != 3 || !byID["chk-2"].Done || byID["chk-1"].Text != "Revisar índice FTS5" || byID["chk-3"].ID == "" {
		t.Fatalf("reconciliación de tareas incorrecta: %+v", checks)
	}
	for _, c := range checks {
		if strings.Contains(c.Text, "Kubernetes") {
			t.Fatalf("tarea inventada no eliminada")
		}
	}

	// F2: KPI inventado eliminado, KPI real conservado
	kpis := findBlocks(env.Report.Blocks, "kpi")
	if len(kpis) != 1 || len(itemsOf(kpis[0])) != 1 {
		t.Fatalf("KPI inventado no eliminado: %+v", kpis)
	}

	// F1: código íntegro preservado
	foundFull := false
	for _, c := range findBlocks(env.Report.Blocks, "code") {
		if src, _ := c["src"].(string); strings.Contains(src, "return nil") {
			foundFull = true
		}
	}
	if !foundFull {
		t.Fatalf("el código original no fue preservado")
	}

	// free-canvas: máximo 1 y advertencia de color literal
	if n := len(findBlocks(env.Report.Blocks, "canvas")); n != 1 {
		t.Fatalf("canvas esperados 1, obtenidos %d", n)
	}
	joined, _ := json.Marshal(res.Warnings)
	if !strings.Contains(string(joined), "color literal") {
		t.Fatalf("falta advertencia de color literal: %s", joined)
	}
	if res.Meta.FidelityRepairs < 5 {
		t.Fatalf("reparaciones esperadas >= 5, obtenidas %d", res.Meta.FidelityRepairs)
	}

	// La salida reparada debe seguir siendo válida para el validador ADR 1
	raw, _ := json.Marshal(env)
	if _, _, err := contract.ValidateAndNormalizeRawJSON(raw); err != nil {
		t.Fatalf("salida reparada inválida: %v", err)
	}
}

func TestFidelityGuardRejectsCovertSummaries(t *testing.T) {
	long := "# Informe\n\n" + strings.Repeat("", 0) +
		"El servicio orquestador presenta degradación sostenida durante ventanas nocturnas. " +
		"Analizamos registros, trazas distribuidas, métricas de memoria, conexiones abiertas, " +
		"contención de bloqueos, latencias de disco, reintentos idempotentes, colas persistentes, " +
		"particiones calientes, compactación diferida, replicación asíncrona, certificados expirados, " +
		"balanceadores saturados, despliegues canarios, banderas experimentales, cachés frías, " +
		"índices fragmentados, consultas pesadas, presupuestos excedidos y alertas silenciadas.\n"
	env, _, err := contract.ValidateAndNormalizeRawJSON([]byte(`{"v":1,"report":{"title":"Informe","ts":"2026-11-18T10:00:00Z","blocks":[{"t":"prose","md":"Todo bien en general."}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = layoutagent.ApplyFidelityGuard(env, nil, long)
	if !errors.Is(err, layoutagent.ErrLowFidelity) {
		t.Fatalf("se esperaba ErrLowFidelity, obtenido: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Configuración, prompt y limpieza
// ---------------------------------------------------------------------------

func TestProviderDetectionAndEndpointResolution(t *testing.T) {
	cases := []struct{ url, model, want string }{
		{"https://api.anthropic.com", "", layoutagent.ProviderAnthropic},
		{"", "claude-x", layoutagent.ProviderAnthropic},
		{"https://generativelanguage.googleapis.com/v1beta", "x", layoutagent.ProviderGemini},
		{"", "gemini-x", layoutagent.ProviderGemini},
		{"http://proxy.local/v1/chat/completions", "llama", layoutagent.ProviderOpenAI},
		{"", "gpt-x", layoutagent.ProviderOpenAI},
	}
	for _, c := range cases {
		if got, _ := layoutagent.DetectProvider(c.url, c.model); got != c.want {
			t.Errorf("DetectProvider(%q,%q)=%q, esperado %q", c.url, c.model, got, c.want)
		}
	}

	r, err := layoutagent.Config{APIKey: "k", Model: "claude-x"}.Resolve()
	if err != nil || r.Endpoint != "https://api.anthropic.com/v1/messages" {
		t.Errorf("endpoint anthropic por defecto: %+v %v", r, err)
	}
	r, err = layoutagent.Config{Provider: "google", APIKey: "k", Model: "gemini-x"}.Resolve()
	if err != nil || r.Endpoint != "https://generativelanguage.googleapis.com/v1beta/models/gemini-x:generateContent" {
		t.Errorf("endpoint gemini por defecto: %+v %v", r, err)
	}
	r, err = layoutagent.Config{APIURL: "http://gw.local/custom/infer", Model: "m"}.Resolve()
	if err != nil || r.Endpoint != "http://gw.local/custom/infer" {
		t.Errorf("las URL completas de pasarela deben respetarse: %+v %v", r, err)
	}
	if _, err := (layoutagent.Config{APIKey: "k"}).Resolve(); err == nil {
		t.Errorf("sin proveedor ni modelo debe fallar la resolución")
	}
	if _, err := (layoutagent.Config{Provider: "anthropic", APIKey: "k"}).Resolve(); err == nil {
		t.Errorf("anthropic sin modelo debe fallar")
	}
	if (layoutagent.Config{}).IsConfigured() {
		t.Errorf("configuración vacía no debe considerarse configurada")
	}
}

func TestSystemPromptUsesRealTokensAndContractKeys(t *testing.T) {
	tokens := layoutagent.CanvasTokens()
	if len(tokens) < 40 {
		t.Fatalf("muy pocos tokens extraídos de tokens.css: %d", len(tokens))
	}
	for _, tok := range tokens {
		if !strings.Contains(ui.TokensCSS, tok+":") {
			t.Fatalf("token %s listado en el prompt no existe en tokens.css", tok)
		}
	}
	prompt := layoutagent.SystemPrompt()
	for _, must := range []string{
		"--color-surface-base", "--color-text-primary", "--color-sev-error-fg", "--font-mono",
		`"kpi"`, `"callout"`, `"table"`, `"section"`, `"code"`, `"diagram"`, `"checklist"`, `"prose"`, `"canvas"`,
		"<source_markdown>", "chk-N", "dx-canvas-theme", contract.CompactSchemaB,
	} {
		if !strings.Contains(prompt, must) {
			t.Fatalf("el prompt maestro no contiene %q", must)
		}
	}
	if strings.Contains(prompt, "{TOKENS}") || strings.Contains(prompt, "{SCHEMA}") {
		t.Fatalf("plantilla del prompt sin resolver")
	}
}

func TestExtractJSONObjectHandlesBracesInsideStrings(t *testing.T) {
	got := string(layoutagent.ExtractJSONObject("ruido previo {\"a\":\"}{\\\"x\",\"b\":{\"c\":1}} ruido final"))
	if got != `{"a":"}{\"x","b":{"c":1}}` {
		t.Fatalf("extracción incorrecta: %s", got)
	}
	got = string(layoutagent.ExtractJSONObject("```json\n{\"v\":1}\n```"))
	if got != `{"v":1}` {
		t.Fatalf("limpieza de cercas incorrecta: %s", got)
	}
}

// ---------------------------------------------------------------------------
// Proveedor local `cli` (sin API keys) y contrato para el anfitrión MCP
// ---------------------------------------------------------------------------

func mustEnvelopeJSON(blocks []any) string {
	raw, err := json.Marshal(map[string]any{
		"v":      1,
		"report": map[string]any{"title": "Auditoría de latencia", "ts": "2026-11-18T10:00:00Z", "blocks": blocks},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// TestHelperCLIProcess no es una prueba real: es la CLI falsa que ejecutan las pruebas
// del proveedor cli (el binario de pruebas se relanza a sí mismo como proceso hijo).
func TestHelperCLIProcess(t *testing.T) {
	mode := os.Getenv("DIOPTRA_FAKE_CLI_MODE")
	if mode == "" {
		return
	}
	in, _ := io.ReadAll(os.Stdin)
	s := string(in)
	if !strings.Contains(s, "Agente Único de Dioptra") || !strings.Contains(s, "<source_markdown>") || !strings.Contains(s, "Revisar índice FTS5") {
		fmt.Fprint(os.Stderr, "prompt incompleto recibido por stdin")
		os.Exit(3)
	}
	switch mode {
	case "ok":
		fmt.Println("Analizando el documento…\n```json")
		fmt.Println(mustEnvelopeJSON(faithfulBlocks()))
		fmt.Println("```")
	case "fail":
		fmt.Fprint(os.Stderr, "error: sesión no autenticada")
		os.Exit(2)
	}
	os.Exit(0)
}

func fakeCLICommand() string {
	return `"` + os.Args[0] + `" -test.run=^TestHelperCLIProcess$`
}

func TestCLIProvider_PipesPromptThroughStdinWithoutAPIKeys(t *testing.T) {
	t.Setenv("DIOPTRA_FAKE_CLI_MODE", "ok")
	// La CLI tiene prioridad sobre el adaptador HTTP aunque existan variables de API key
	cfg := layoutagent.Config{CLICommand: fakeCLICommand(), APIKey: "http-key-secundaria", Model: "claude-x", Timeout: 60 * time.Second}
	r, err := cfg.Resolve()
	if err != nil || r.Provider != layoutagent.ProviderCLI || r.ProviderSource != "auto:cli" {
		t.Fatalf("se esperaba proveedor cli autodetectado: %+v %v", r, err)
	}
	res, err := layoutagent.Synthesize(context.Background(), cfg, input())
	if err != nil {
		t.Fatalf("Synthesize vía CLI: %v", err)
	}
	if res.Meta.Provider != layoutagent.ProviderCLI || res.Meta.Attempts != 1 {
		t.Fatalf("meta inesperada: %+v", res.Meta)
	}
	if got := strings.Join(checklistIDs(res.Envelope), ","); got != "chk-1,chk-2,chk-3" {
		t.Fatalf("ids de checklist: %s", got)
	}
	if res.Meta.FidelityRepairs != 0 {
		t.Fatalf("salida fiel no debe requerir reparaciones: %+v", res.Warnings)
	}
}

func TestCLIProvider_FailureSurfacesStderr(t *testing.T) {
	t.Setenv("DIOPTRA_FAKE_CLI_MODE", "fail")
	cfg := layoutagent.Config{Provider: "cli", CLICommand: fakeCLICommand(), Timeout: 60 * time.Second}
	_, err := layoutagent.Synthesize(context.Background(), cfg, input())
	if err == nil || !strings.Contains(err.Error(), "no autenticada") {
		t.Fatalf("el error debe incluir el stderr de la CLI: %v", err)
	}
}

func TestCLIProvider_MissingBinaryAndDaemonExclusion(t *testing.T) {
	if _, err := (layoutagent.Config{CLICommand: "dioptra-cli-inexistente-xyz -p"}).Resolve(); err == nil {
		t.Fatalf("una CLI inexistente debe fallar la resolución")
	}
	if _, err := (layoutagent.Config{Provider: "cli"}).Resolve(); err == nil {
		t.Fatalf("provider=cli sin DIOPTRA_LAYOUT_CLI debe fallar")
	}
	daemonCfg := layoutagent.Config{Provider: "cli", CLICommand: fakeCLICommand()}.ForDaemon()
	if daemonCfg.IsConfigured() {
		t.Fatalf("el demonio jamás debe lanzar la CLI local")
	}
	if (layoutagent.Config{}).HasHTTPSettings() {
		t.Fatalf("configuración vacía no tiene ajustes HTTP")
	}
}

func TestSplitCommandLineKeepsWindowsPaths(t *testing.T) {
	got, err := layoutagent.SplitCommandLine(`"C:\Program Files\Tools\claude.exe" -p --append 'a b'`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`C:\Program Files\Tools\claude.exe`, "-p", "--append", "a b"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("SplitCommandLine=%q, esperado %q", got, want)
	}
	if _, err := layoutagent.SplitCommandLine(`claude "-p`); err == nil {
		t.Fatalf("comillas sin cerrar deben fallar")
	}
}

func TestLayoutContractExposesRulesTokensAndSchema(t *testing.T) {
	c := layoutagent.BuildLayoutContract()
	if c.Rules != layoutagent.HostRules || c.Schema != contract.CompactSchemaB || len(c.CSSTokens) == 0 || c.Version != layoutagent.PromptVersion {
		t.Fatalf("contrato incompleto: version=%s tokens=%d", c.Version, len(c.CSSTokens))
	}
	if !strings.Contains(c.Usage, "blocks_override") || !strings.Contains(c.Rules, "chk-N") || !strings.Contains(c.Rules, "45%") {
		t.Fatalf("instrucciones de anfitrión incompletas")
	}
	if strings.Contains(c.Rules, "Agente Único") || strings.Contains(c.Rules, "<meta>") {
		t.Fatal("el contrato del anfitrión no debe incluir la persona del sintetizador interno")
	}
	md := layoutagent.LayoutContractMarkdown()
	for _, must := range []string{c.Rules, c.Schema, "include_canvas_tokens"} {
		if !strings.Contains(md, must) {
			t.Fatalf("recurso Markdown sin %q", truncateForMsg(must))
		}
	}
}

// TestHostContractTokenBudget fija el presupuesto del contrato que leen los
// anfitriones: <= 2k tokens sin tokens CSS (antes ~6-7k).
func TestHostContractTokenBudget(t *testing.T) {
	lean := layoutagent.BuildHostContract(false)
	if len(lean.CSSTokens) != 0 {
		t.Fatal("sin include_canvas_tokens no deben enviarse los tokens CSS")
	}
	raw, _ := json.Marshal(lean)
	if est := len(raw) / 4; est > 2000 {
		t.Fatalf("contrato del anfitrión demasiado grande: ~%d tokens (máx 2000)", est)
	}
	if est := len(raw) / 4; est > cost.ContractReadEstTokens*115/100 || est < cost.ContractReadEstTokens*85/100 {
		t.Fatalf("cost.ContractReadEstTokens=%d no refleja el tamaño real ~%d", cost.ContractReadEstTokens, est)
	}
	full := layoutagent.BuildHostContract(true)
	if len(full.CSSTokens) == 0 {
		t.Fatal("con include_canvas_tokens deben venir los tokens CSS")
	}
}

func truncateForMsg(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
