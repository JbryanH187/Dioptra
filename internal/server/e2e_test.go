package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dioptra/internal/e2e"
	"dioptra/internal/layoutagent"
	"dioptra/internal/mcpserver"
)

func postJSON(t *testing.T, url string, in any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(in)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestE2E_HTTPFlujoCompletoYCasillaConservada(t *testing.T) {
	ts, st := setupTestServer(t, layoutagent.Config{})
	ctx := context.Background()

	code, start := postJSON(t, ts.URL+"/api/e2e/start", mcpserver.E2EStartInput{Project: "abonos", Title: "Abono · QA", Environment: "QA", Agent: "Genius"})
	if code != 200 || !strings.HasPrefix(start["run"].(string), "e2e_") || !strings.Contains(start["url"].(string), "?report=") {
		t.Fatalf("start %d %v", code, start)
	}
	run, reportID := start["run"].(string), start["report_id"].(string)

	steps := []mcpserver.E2ERecordInput{
		{Run: run, Kind: "service", Service: "api-pagos", BaseURL: "http://localhost:8095", Environment: "QA", LogSource: "logs/pagos.log"},
		{Run: run, Kind: "case", Title: "Abono con saldo insuficiente", Expected: "rechazo NSF"},
		{Run: run, Kind: "finding", Service: "api-pagos", Issue: "no registra el folio del abono", Suggestion: "loguear folio y regla NSF"},
	}
	for _, s := range steps {
		if code, out := postJSON(t, ts.URL+"/api/e2e/record", s); code != 200 {
			t.Fatalf("record %s: %d %v", s.Kind, code, out)
		}
	}
	code, har := postJSON(t, ts.URL+"/api/e2e/har", mcpserver.E2EIngestHARInput{Run: run, Case: 1, Path: filepath.Join("..", "e2e", "testdata", "abono.har")})
	if code != 200 || har["added"].(float64) != 4 {
		t.Fatalf("har %d %v", code, har)
	}

	// El equipo marca la mejora de logs en el visor…
	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/reports/"+reportID+"/check", strings.NewReader(`{"id":"chk-1","done":true}`))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 200 {
		t.Fatalf("PATCH check: %v %v", err, resp)
	}
	// …y el agente sigue registrando: la casilla no se desmarca.
	if code, out := postJSON(t, ts.URL+"/api/e2e/record", mcpserver.E2ERecordInput{Run: run, Kind: "note", Detail: "NSF confirmado en pantalla"}); code != 200 {
		t.Fatalf("note %d %v", code, out)
	}
	code, fin := postJSON(t, ts.URL+"/api/e2e/finish", mcpserver.E2EFinishInput{Run: run, Summary: "Caso NSF rechazado correctamente."})
	if code != 200 || fin["status"] != e2e.StatusFinished || fin["report_id"] != reportID {
		t.Fatalf("finish %d %v", code, fin)
	}

	state, err := st.GetReportState(ctx, reportID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Checklists) != 1 || !state.Checklists[0].Done {
		t.Errorf("la mejora aprobada debe seguir marcada: %+v", state.Checklists)
	}
	for _, want := range []string{"Estado: **terminado**", "## Caso 1 · Abono con saldo insuficiente", "api-pagos", "422", "NSF confirmado"} {
		if !strings.Contains(state.Markdown, want) {
			t.Errorf("falta %q en el reporte", want)
		}
	}
	if strings.Contains(state.Markdown, "4111111111111111") || strings.Contains(state.Markdown, "eyJhbGci") {
		t.Error("el reporte no debe contener datos sensibles")
	}

	if code, out := postJSON(t, ts.URL+"/api/e2e/record", mcpserver.E2ERecordInput{Run: run, Kind: "note", Detail: "tarde"}); code != 400 || !strings.Contains(out["error"].(string), "ya terminó") {
		t.Errorf("tras finish: %d %v", code, out)
	}
	resp, err := http.Get(ts.URL + "/api/e2e")
	if err != nil {
		t.Fatal(err)
	}
	var list struct{ Count int }
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if list.Count != 1 {
		t.Errorf("GET /api/e2e count = %d", list.Count)
	}
}

func TestE2E_HerramientasMCPYPuente(t *testing.T) {
	ts, _ := setupTestServer(t, layoutagent.Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	session, err := mcp.NewClient(&mcp.Implementation{Name: "agente", Version: "1"}, nil).
		Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range tools.Tools {
		names[tl.Name] = true
	}
	for _, want := range []string{"e2e_start", "e2e_record", "e2e_ingest_har", "e2e_finish"} {
		if !names[want] {
			t.Errorf("falta la herramienta %s", want)
		}
	}

	// El puente stdio → demonio usa los mismos endpoints.
	bridge := &mcpserver.HTTPBridgeBackend{BaseURL: ts.URL}
	res, err := bridge.E2EStart(ctx, mcpserver.E2EStartInput{Project: "p", Title: "Flujo"})
	if err != nil || res.Status != e2e.StatusRecording {
		t.Fatalf("puente start: %+v %v", res, err)
	}
	if _, err := bridge.E2ERecord(ctx, mcpserver.E2ERecordInput{Run: res.Run, Kind: "rara"}); err == nil || !strings.Contains(err.Error(), "kind desconocido") {
		t.Errorf("el puente debe propagar el error legible: %v", err)
	}
	if _, err := bridge.E2EIngestHAR(ctx, mcpserver.E2EIngestHARInput{Run: res.Run, Path: "C:/Windows/win.ini"}); err == nil {
		t.Error("solo acepta .har")
	}
}
