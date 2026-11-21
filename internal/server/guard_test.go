package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"dioptra/internal/layoutagent"
	"dioptra/internal/mcpserver"
)

const guardMD = "# Guardia\n\n- [ ] Aprobar despliegue\n- [ ] Borrar tabla\n"

// publishForGuard publica un reporte por la vía legítima y devuelve su id.
func publishForGuard(t *testing.T, baseURL string) string {
	t.Helper()
	body, _ := json.Marshal(mcpserver.PublishReportInput{Title: "Guardia", Project: "guard", Markdown: guardMD})
	resp, err := http.Post(baseURL+"/api/reports", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("publish legítimo falló: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("publish legítimo: esperaba 201, obtuvo %d: %s", resp.StatusCode, b)
	}
	var out mcpserver.PublishReportOutput
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.ID
}

type guardCase struct {
	name    string
	method  string
	path    string // se le antepone baseURL; "{id}" se reemplaza
	body    string
	headers map[string]string
	host    string // Host override (DNS rebinding)
	want    int
}

func doGuard(t *testing.T, baseURL, id string, c guardCase) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(c.method, baseURL+strings.ReplaceAll(c.path, "{id}", id), strings.NewReader(c.body))
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	if c.host != "" {
		req.Host = c.host
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s: %v", c.name, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// TestGuard_RejectsCrossSiteAndRebinding reproduce los ataques confirmados en la
// auditoría etapa 2 (docs/AUDITORIA_ETAPA2.md, hallazgos 1-4).
func TestGuard_RejectsCrossSiteAndRebinding(t *testing.T) {
	ts, _ := setupTestServer(t, layoutagent.Config{})
	id := publishForGuard(t, ts.URL)
	pub := `{"title":"x","project":"evil","markdown":"# x"}`
	chk := `{"id":"chk-1","done":true}`
	evil := "http://evil.example"

	cases := []guardCase{
		{"publish text/plain + Origin ajeno", "POST", "/api/reports", pub, map[string]string{"Content-Type": "text/plain", "Origin": evil}, "", http.StatusForbidden},
		{"publish text/plain sin Origin", "POST", "/api/reports", pub, map[string]string{"Content-Type": "text/plain"}, "", http.StatusUnsupportedMediaType},
		{"publish form-urlencoded", "POST", "/api/reports", pub, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "", http.StatusUnsupportedMediaType},
		{"publish JSON + Origin ajeno", "POST", "/api/reports", pub, map[string]string{"Content-Type": "application/json", "Origin": evil}, "", http.StatusForbidden},
		{"publish Origin null (iframe /canvas)", "POST", "/api/reports", pub, map[string]string{"Content-Type": "application/json", "Origin": "null"}, "", http.StatusForbidden},
		{"publish Sec-Fetch-Site cross-site", "POST", "/api/reports", pub, map[string]string{"Content-Type": "application/json", "Sec-Fetch-Site": "cross-site"}, "", http.StatusForbidden},
		{"publish DNS rebinding", "POST", "/api/reports", pub, map[string]string{"Content-Type": "application/json"}, "evil.example:7474", http.StatusForbidden},
		{"check text/plain + Origin ajeno", "PATCH", "/api/reports/{id}/check", chk, map[string]string{"Content-Type": "text/plain", "Origin": evil}, "", http.StatusForbidden},
		{"check text/plain sin Origin", "PATCH", "/api/reports/{id}/check", chk, map[string]string{"Content-Type": "text/plain"}, "", http.StatusUnsupportedMediaType},
		{"check DNS rebinding", "PATCH", "/api/reports/{id}/check", chk, map[string]string{"Content-Type": "application/json"}, "evil.example", http.StatusForbidden},
		{"lectura de reportes por DNS rebinding", "GET", "/api/reports", "", nil, "evil.example:7474", http.StatusForbidden},
		{"lectura de estado por DNS rebinding", "GET", "/api/reports/{id}/state", "", nil, "attacker.localhost.evil.example", http.StatusForbidden},
		{"/mcp + Origin ajeno", "POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, map[string]string{"Content-Type": "application/json", "Accept": "application/json, text/event-stream", "Origin": evil}, "", http.StatusForbidden},
	}
	for _, c := range cases {
		got, body := doGuard(t, ts.URL, id, c)
		if got != c.want {
			t.Errorf("%s: esperaba %d, obtuvo %d (%s)", c.name, c.want, got, body)
		}
	}

	// Ninguna casilla debe haber quedado aprobada por los intentos anteriores.
	resp, err := http.Get(ts.URL + "/api/reports/" + id + "/state")
	if err != nil {
		t.Fatalf("GET state: %v", err)
	}
	defer resp.Body.Close()
	var st struct {
		Checklists []struct {
			ID   string `json:"id"`
			Done bool   `json:"done"`
		} `json:"checklists"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if len(st.Checklists) == 0 {
		t.Fatalf("el reporte de prueba no tiene casillas")
	}
	for _, c := range st.Checklists {
		if c.Done {
			t.Errorf("la casilla %s quedó aprobada por una solicitud rechazada", c.ID)
		}
	}
}

// TestGuard_AllowsLegitimateLocalClients asegura que la UI propia, el puente MCP
// y la CLI siguen funcionando.
func TestGuard_AllowsLegitimateLocalClients(t *testing.T) {
	ts, _ := setupTestServer(t, layoutagent.Config{})
	id := publishForGuard(t, ts.URL) // sin Origin, JSON: igual que HTTPBridgeBackend
	sameOrigin := ts.URL             // http://127.0.0.1:<puerto>

	cases := []guardCase{
		{"UI: PATCH mismo origen", "PATCH", "/api/reports/{id}/check", `{"id":"chk-1","done":true}`,
			map[string]string{"Content-Type": "application/json", "Origin": sameOrigin, "Sec-Fetch-Site": "same-origin"}, "", http.StatusOK},
		{"UI: publish JSON charset mismo origen", "POST", "/api/reports", `{"title":"ui","project":"guard","markdown":"# ui"}`,
			map[string]string{"Content-Type": "application/json; charset=utf-8", "Origin": sameOrigin}, "", http.StatusCreated},
		{"Host localhost", "GET", "/api/health", "", nil, strings.Replace(strings.TrimPrefix(ts.URL, "http://"), "127.0.0.1", "localhost", 1), http.StatusOK},
		{"Host [::1]", "GET", "/api/health", "", nil, "[::1]:7474", http.StatusOK},
		{"GET UI raíz", "GET", "/", "", nil, "", http.StatusOK},
	}
	for _, c := range cases {
		if got, body := doGuard(t, ts.URL, id, c); got != c.want {
			t.Errorf("%s: esperaba %d, obtuvo %d (%s)", c.name, c.want, got, body)
		}
	}
}

// TestStatic_ETagAndFontCaching: los módulos se revalidan con ETag (304) y las
// fuentes embebidas se cachean como inmutables.
func TestStatic_ETagAndFontCaching(t *testing.T) {
	ts, _ := setupTestServer(t, layoutagent.Config{})
	resp, err := http.Get(ts.URL + "/app.js")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	etag := resp.Header.Get("ETag")
	if resp.StatusCode != http.StatusOK || etag == "" || resp.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("app.js: status=%d etag=%q cache=%q", resp.StatusCode, etag, resp.Header.Get("Cache-Control"))
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/app.js", nil)
	req.Header.Set("If-None-Match", etag)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("revalidación con ETag: esperaba 304, obtuvo %d", resp2.StatusCode)
	}
	font, err := http.Get(ts.URL + "/fonts/cinzel-latin.woff2")
	if err != nil {
		t.Fatal(err)
	}
	font.Body.Close()
	if font.StatusCode != http.StatusOK || !strings.Contains(font.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("fuente: status=%d cache=%q", font.StatusCode, font.Header.Get("Cache-Control"))
	}
}
