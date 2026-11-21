package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dioptra/internal/contract"
	"dioptra/internal/layoutagent"
	"dioptra/internal/mcpserver"
	"dioptra/internal/server"
	"dioptra/internal/store"
)

func setupTestServer(t *testing.T, layoutCfg layoutagent.Config) (*httptest.Server, *store.Store) {
	t.Helper()
	tmpDir := t.TempDir()
	st, err := store.Open(tmpDir)
	if err != nil {
		t.Fatalf("store.Open falló: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	var ts *httptest.Server
	appSrv := server.New(server.Options{
		Store:     st,
		BaseURL:   "http://127.0.0.1:7474",
		LayoutCfg: layoutCfg,
	})
	ts = httptest.NewServer(appSrv)
	t.Cleanup(ts.Close)
	return ts, st
}

func TestHTTP_SeparatedCSPHeadersAndCanvasIsolation(t *testing.T) {
	ts, _ := setupTestServer(t, layoutagent.Config{})

	// 1. Verificar GET / sirve index.html con MainCSP estricta y bloquea preview.html
	respRoot, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / falló: %v", err)
	}
	defer respRoot.Body.Close()

	if respRoot.StatusCode != http.StatusOK {
		t.Fatalf("esperaba 200 en /, obtuvo %d", respRoot.StatusCode)
	}
	gotMainCSP := respRoot.Header.Get("Content-Security-Policy")
	if gotMainCSP != server.MainCSP {
		t.Fatalf("MainCSP incorrecta:\nobtuvo:  %s\nesperaba: %s", gotMainCSP, server.MainCSP)
	}
	rootBody, _ := io.ReadAll(respRoot.Body)
	if strings.Contains(string(rootBody), `data-dx-mode="preview"`) {
		t.Fatal("index.html de producción NO debe incluir data-dx-mode=\"preview\"")
	}

	respPreview, err := http.Get(ts.URL + "/preview.html")
	if err != nil {
		t.Fatalf("GET /preview.html falló: %v", err)
	}
	_ = respPreview.Body.Close()
	if respPreview.StatusCode != http.StatusNotFound {
		t.Fatalf("preview.html debe estar excluido en producción (esperaba 404, obtuvo %d)", respPreview.StatusCode)
	}

	// 2. Publicar un reporte con bloque free-canvas y lista de tareas
	mdWithCanvas := "# Simulador Red\n\n" +
		"```html-canvas\n<button id=\"b\" onclick=\"window.count=(window.count||0)+1\">Pulsa</button>\n```\n\n" +
		"- [ ] Aprobar despliegue en producción @sre\n- [x] Auditar cabeceras CSP @sec\n"

	pubBody, _ := json.Marshal(mcpserver.PublishReportInput{
		Title:    "Simulador Red",
		Project:  "infra-net",
		Markdown: mdWithCanvas,
	})
	respPub, err := http.Post(ts.URL+"/api/reports", "application/json", bytes.NewReader(pubBody))
	if err != nil {
		t.Fatalf("POST /api/reports falló: %v", err)
	}
	defer respPub.Body.Close()
	if respPub.StatusCode != http.StatusCreated {
		t.Fatalf("esperaba 201 Created, obtuvo %d", respPub.StatusCode)
	}

	var pubOut mcpserver.PublishReportOutput
	if err := json.NewDecoder(respPub.Body).Decode(&pubOut); err != nil {
		t.Fatalf("decode PublishReportOutput: %v", err)
	}
	if _, err := os.Stat(pubOut.MarkdownPath); err != nil {
		t.Fatalf("el archivo .md fuente de verdad no existe en disco (%s): %v", pubOut.MarkdownPath, err)
	}

	// 3. Verificar que GET /api/reports/{id} NO expone el HTML crudo del agente (solo src="/canvas/...")
	respDoc, err := http.Get(ts.URL + "/api/reports/" + pubOut.ID)
	if err != nil {
		t.Fatalf("GET /api/reports/{id} falló: %v", err)
	}
	defer respDoc.Body.Close()
	docBytes, _ := io.ReadAll(respDoc.Body)
	docStr := string(docBytes)
	if strings.Contains(docStr, "onclick=") {
		t.Fatalf("fuga de seguridad: el HTML crudo del agente apareció en el JSON del documento principal: %s", docStr)
	}

	var env struct {
		Report struct {
			Blocks []map[string]any `json:"blocks"`
		} `json:"report"`
	}
	_ = json.Unmarshal(docBytes, &env)
	var canvasSrc string
	for _, b := range env.Report.Blocks {
		if b["t"] == "canvas" {
			canvasSrc, _ = b["src"].(string)
		}
	}
	if !strings.HasPrefix(canvasSrc, "/canvas/"+pubOut.ID+"/") {
		t.Fatalf("src de canvas inesperada: %q", canvasSrc)
	}

	// 4. Verificar GET /canvas/{docId}/{blockId}?theme=light sirve CanvasIsolatedCSP + tokens.css + postMessage
	respCanvas, err := http.Get(ts.URL + canvasSrc + "?theme=light")
	if err != nil {
		t.Fatalf("GET %s falló: %v", canvasSrc, err)
	}
	defer respCanvas.Body.Close()
	if respCanvas.StatusCode != http.StatusOK {
		t.Fatalf("esperaba 200 en ruta canvas, obtuvo %d", respCanvas.StatusCode)
	}
	gotCanvasCSP := respCanvas.Header.Get("Content-Security-Policy")
	if gotCanvasCSP != server.CanvasIsolatedCSP {
		t.Fatalf("CanvasIsolatedCSP incorrecta:\nobtuvo:  %s\nesperaba: %s", gotCanvasCSP, server.CanvasIsolatedCSP)
	}
	canvasBody, _ := io.ReadAll(respCanvas.Body)
	canvasHTML := string(canvasBody)
	if !strings.Contains(canvasHTML, `data-theme="light"`) ||
		!strings.Contains(canvasHTML, `--color-bg-body`) ||
		!strings.Contains(canvasHTML, `dxTheme`) ||
		!strings.Contains(canvasHTML, `onclick="window.count`) {
		t.Fatalf("documento /canvas incompleto: %s", canvasHTML[:min(300, len(canvasHTML))])
	}

	// 5. Verificar sincronización bidireccional PATCH /api/reports/{id}/check -> SQLite y archivo .md
	patchPayload := `{"id":"chk-1","done":true}`
	reqPatch, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/reports/"+pubOut.ID+"/check", strings.NewReader(patchPayload))
	reqPatch.Header.Set("Content-Type", "application/json")
	respPatch, err := http.DefaultClient.Do(reqPatch)
	if err != nil {
		t.Fatalf("PATCH check falló: %v", err)
	}
	_ = respPatch.Body.Close()
	if respPatch.StatusCode != http.StatusOK {
		t.Fatalf("esperaba 200 en PATCH check, obtuvo %d", respPatch.StatusCode)
	}

	diskMD, err := os.ReadFile(pubOut.MarkdownPath)
	if err != nil {
		t.Fatalf("leyendo .md actualizado: %v", err)
	}
	if !strings.Contains(string(diskMD), "- [x] Aprobar despliegue en producción @sre") {
		t.Fatalf("el archivo .md en disco no reflejó el cambio a [x]:\n%s", string(diskMD))
	}
}

func TestMCP_StreamableHTTPAndInteractiveFallback(t *testing.T) {
	// Configuramos una URL inválida para comprobar la degradación limpia a goldmark cuando interactive_mode=true falla
	badLayoutCfg := layoutagent.Config{
		APIURL: "http://127.0.0.1:1/unreachable",
		Model:  "test-model",
		Client: &http.Client{Timeout: 100 * time.Millisecond},
	}
	ts, _ := setupTestServer(t, badLayoutCfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-cli", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: ts.URL + "/mcp",
	}, nil)
	if err != nil {
		t.Fatalf("conectando cliente MCP oficial a /mcp: %v", err)
	}
	defer session.Close()

	// 1. Llamar publish_report con interactive_mode=true (debe degradar a goldmark sin abortar)
	callRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "publish_report",
		Arguments: map[string]any{
			"title":            "Prueba MCP Oficial",
			"project":          "mcp-suite",
			"markdown":         "# Prueba MCP Oficial\n\n- [ ] Verificar estado desde consola @agent\n",
			"interactive_mode": true,
		},
	})
	if err != nil {
		t.Fatalf("CallTool publish_report falló: %v", err)
	}
	if callRes.IsError {
		t.Fatalf("publish_report devolvió IsError=true: %+v", callRes.Content)
	}

	rawJSON, _ := json.Marshal(callRes.StructuredContent)
	var pubOut mcpserver.PublishReportOutput
	if err := json.Unmarshal(rawJSON, &pubOut); err != nil {
		t.Fatalf("decodificando salida estructurada de publish_report: %v", err)
	}
	if !pubOut.OK || pubOut.ID == "" || len(pubOut.Warnings) == 0 {
		t.Fatalf("salida inesperada de publish_report con fallback: %+v", pubOut)
	}

	// 2. Llamar list_reports
	listRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_reports",
		Arguments: map[string]any{"project": "mcp-suite"},
	})
	if err != nil || listRes.IsError {
		t.Fatalf("CallTool list_reports falló: err=%v res=%+v", err, listRes)
	}

	// 3. Llamar get_report_state
	stateRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_report_state",
		Arguments: map[string]any{"id": pubOut.ID},
	})
	if err != nil || stateRes.IsError {
		t.Fatalf("CallTool get_report_state falló: err=%v res=%+v", err, stateRes)
	}
	stateRaw, _ := json.Marshal(stateRes.StructuredContent)
	var stateOut mcpserver.GetReportStateOutput
	_ = json.Unmarshal(stateRaw, &stateOut)
	if len(stateOut.Checklists) != 1 || stateOut.Checklists[0].ID != "chk-1" || stateOut.Checklists[0].Done {
		t.Fatalf("checklists inesperadas en get_report_state: %+v", stateOut.Checklists)
	}
}

// TestMCP_HostAgentFlowWithoutCredentials verifica el flujo principal sin variables de entorno:
// get_layout_contract → publish_report con blocks_override → Guardián de Fidelidad → SSE →
// sincronización bidireccional de casillas en el .md por ordinal canónico.
func TestMCP_HostAgentFlowWithoutCredentials(t *testing.T) {
	for _, k := range []string{"DIOPTRA_LAYOUT_PROVIDER", "DIOPTRA_LAYOUT_CLI", "DIOPTRA_LAYOUT_API_KEY", "DIOPTRA_LAYOUT_API_URL", "DIOPTRA_LAYOUT_MODEL"} {
		t.Setenv(k, "")
	}
	ts, _ := setupTestServer(t, layoutagent.Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- Suscriptor SSE (se registra antes de publicar: espera ": connected") ---
	sseReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("conectando SSE: %v", err)
	}
	defer sseResp.Body.Close()
	sseReader := bufio.NewReader(sseResp.Body)
	if first, _ := sseReader.ReadString('\n'); !strings.Contains(first, "connected") {
		t.Fatalf("handshake SSE inesperado: %q", first)
	}
	published := make(chan string, 8)
	go func() {
		defer close(published)
		event := ""
		for {
			line, err := sseReader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: ") && event == "report_published":
				published <- strings.TrimPrefix(line, "data: ")
			}
		}
	}()

	// --- Sesión MCP del agente anfitrión ---
	session, err := mcp.NewClient(&mcp.Implementation{Name: "host-agent", Version: "1.0.0"}, nil).
		Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("conectando cliente MCP: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	hasContractTool := false
	for _, tl := range tools.Tools {
		if tl.Name == "get_layout_contract" {
			hasContractTool = true
		}
		if tl.Name == "publish_report" && !strings.Contains(tl.Description, "blocks_override") {
			t.Fatalf("la descripción de publish_report debe instruir el uso de blocks_override")
		}
	}
	if !hasContractTool {
		t.Fatalf("falta la herramienta get_layout_contract")
	}

	rr, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: layoutagent.LayoutContractURI})
	if err != nil || len(rr.Contents) == 0 || !strings.Contains(rr.Contents[0].Text, contract.CompactSchemaB) {
		t.Fatalf("recurso %s inválido: %v", layoutagent.LayoutContractURI, err)
	}

	cr, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_layout_contract", Arguments: map[string]any{}})
	if err != nil || cr.IsError {
		t.Fatalf("get_layout_contract: err=%v res=%+v", err, cr)
	}
	rawContract, _ := json.Marshal(cr.StructuredContent)
	var lc layoutagent.LayoutContract
	if err := json.Unmarshal(rawContract, &lc); err != nil || !strings.Contains(lc.Rules, "chk-N") || len(lc.CSSTokens) != 0 {
		t.Fatalf("contrato devuelto incompleto: %v", err)
	}

	// --- publish_report con blocks_override deliberadamente infiel ---
	md := "# Plan de capacidad\n\n" +
		"El pool actual atiende 1,250 req/s con p95 de 340 ms en internal/store/store.go:622.\n\n" +
		"```go\nfunc Publish() error {\n\treturn nil\n}\n```\n\n" +
		"- [ ] Revisar índice FTS5 @dba\n" +
		"- [x] Medir p95 en staging\n" +
		"- [ ] Ajustar pool de conexiones\n"
	blocks := []any{
		map[string]any{"t": "kpi", "items": []any{
			map[string]any{"label": "p95", "value": 340, "unit": "ms"},
			map[string]any{"label": "Ahorro", "value": 999, "unit": "USD"}, // cifra inventada
		}},
		map[string]any{"t": "prose", "md": "El pool actual atiende 1,250 req/s con p95 de 340 ms en internal/store/store.go:622."},
		map[string]any{"t": "checklist", "items": []any{
			map[string]any{"id": "x", "text": "Ajustar pool de conexiones", "done": false}, // id erróneo
			map[string]any{"id": "y", "text": "Migrar a otra nube", "done": false},         // inventada
		}},
	}
	callRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "publish_report",
		Arguments: map[string]any{
			"title": "Plan de capacidad", "project": "host-flow", "model": "claude-sonnet-4", "markdown": md, "blocks_override": blocks,
		},
	})
	if err != nil || callRes.IsError {
		t.Fatalf("publish_report: err=%v res=%+v", err, callRes)
	}
	rawPub, _ := json.Marshal(callRes.StructuredContent)
	var pubOut mcpserver.PublishReportOutput
	if err := json.Unmarshal(rawPub, &pubOut); err != nil {
		t.Fatal(err)
	}
	if pubOut.ModeUsed != "blocks_override" {
		t.Fatalf("modo esperado blocks_override, obtenido %s (%+v)", pubOut.ModeUsed, pubOut.Warnings)
	}
	if pubOut.Cost == nil || pubOut.Cost.ExtraOutputTokens <= 0 || pubOut.Cost.DeltaUSD <= 0 || !strings.Contains(pubOut.Cost.ConsoleLine, "USD") {
		t.Fatalf("reporte de costo ausente o incompleto en publish_report: %+v", pubOut.Cost)
	}
	if pubOut.Fidelity == nil || pubOut.Fidelity.Verdict != "repaired" || pubOut.Fidelity.Repairs < 4 || pubOut.Fidelity.SourceTasks != 3 || pubOut.Fidelity.Hint == "" {
		t.Fatalf("métricas del Guardián incompletas: %+v", pubOut.Fidelity)
	}
	if len(pubOut.Warnings) == 0 {
		t.Fatalf("el agente debe recibir las advertencias de reparación")
	}

	// --- SSE emitido tras la auditoría ---
	select {
	case data := <-published:
		if !strings.Contains(data, pubOut.ID) {
			t.Fatalf("evento SSE de otro reporte: %s", data)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no llegó report_published por SSE")
	}

	// --- Sobre persistido = sobre reparado ---
	envResp, err := http.Get(ts.URL + "/api/reports/" + pubOut.ID)
	if err != nil {
		t.Fatal(err)
	}
	var env contract.Envelope
	_ = json.NewDecoder(envResp.Body).Decode(&env)
	envResp.Body.Close()

	byText := map[string]contract.ChecklistItemState{}
	for _, c := range contract.CollectChecklists(env.Report.Blocks) {
		byText[c.Text] = c
	}
	if len(byText) != 3 || byText["Ajustar pool de conexiones"].ID != "chk-3" || !byText["Medir p95 en staging"].Done || byText["Revisar índice FTS5"].ID != "chk-1" {
		t.Fatalf("tareas no reconciliadas al ordinal canónico: %+v", byText)
	}
	// Solo los bloques renderizados: warnings[] cita a propósito lo eliminado para autocorrección
	rawEnv, _ := json.Marshal(env.Report.Blocks)
	if strings.Contains(string(rawEnv), `"value":999`) || strings.Contains(string(rawEnv), "Migrar a otra nube") {
		t.Fatalf("contenido inventado persistido: %s", rawEnv)
	}
	if !strings.Contains(string(rawEnv), "return nil") {
		t.Fatalf("el código omitido por el agente no fue preservado")
	}

	// --- La casilla reconciliada sincroniza la línea correcta del .md ---
	patchReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/reports/"+pubOut.ID+"/check", strings.NewReader(`{"id":"chk-3","done":true}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH check: err=%v status=%v", err, patchResp)
	}
	patchResp.Body.Close()
	mdBytes, err := os.ReadFile(pubOut.MarkdownPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mdBytes), "- [x] Ajustar pool de conexiones") || !strings.Contains(string(mdBytes), "- [ ] Revisar índice FTS5") {
		t.Fatalf("la casilla se sincronizó en la línea equivocada:\n%s", mdBytes)
	}

	// --- Resumen encubierto: el Guardián rechaza y degrada a goldmark sin perder contenido ---
	longMD := "# Informe nocturno\n\n" +
		"El servicio orquestador presenta degradación sostenida durante ventanas nocturnas. " +
		"Analizamos registros, trazas distribuidas, métricas de memoria, conexiones abiertas, " +
		"contención de bloqueos, latencias de disco, reintentos idempotentes, colas persistentes, " +
		"particiones calientes, compactación diferida, replicación asíncrona, certificados expirados, " +
		"balanceadores saturados, despliegues canarios, banderas experimentales, cachés frías, " +
		"índices fragmentados, consultas pesadas, presupuestos excedidos y alertas silenciadas.\n"
	covertRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "publish_report",
		Arguments: map[string]any{
			"title": "Informe nocturno", "project": "host-flow", "markdown": longMD,
			"blocks_override": []any{map[string]any{"t": "prose", "md": "Todo bien en general."}},
		},
	})
	if err != nil || covertRes.IsError {
		t.Fatalf("publish_report (resumen): err=%v", err)
	}
	rawCovert, _ := json.Marshal(covertRes.StructuredContent)
	var covertOut mcpserver.PublishReportOutput
	_ = json.Unmarshal(rawCovert, &covertOut)
	if covertOut.ModeUsed != "fast_goldmark" || covertOut.Fidelity == nil || covertOut.Fidelity.Verdict != "rejected" {
		t.Fatalf("un resumen encubierto debe rechazarse y degradar a goldmark: %+v", covertOut)
	}
	if covertOut.Cost == nil || covertOut.Cost.Mode != "fast_goldmark" || covertOut.Cost.ConsoleLine == "" {
		t.Fatalf("costo de ruta degradada faltante: %+v", covertOut.Cost)
	}

	// --- GET /api/stats refleja las 2 publicaciones medidas ---
	statsResp, err := http.Get(ts.URL + "/api/stats")
	if err != nil || statsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/stats falló: err=%v status=%v", err, statsResp)
	}
	defer statsResp.Body.Close()
	var statsSum store.CostSummary
	if err := json.NewDecoder(statsResp.Body).Decode(&statsSum); err != nil {
		t.Fatal(err)
	}
	if statsSum.TotalReports != 2 || len(statsSum.ByMode) != 2 || statsSum.DeltaUSD <= 0 {
		t.Fatalf("acumulado de /api/stats inesperado: %+v", statsSum)
	}
}

// TestSSE_LastEventIDReplayAndCacheHeaders valida que /api/events emite ids monotónicos y
// reproduce eventos perdidos cuando el navegador reconecta con Last-Event-ID, y que /api/*
// junto a app.js evitan cachés obsoletas en el navegador.
func TestSSE_LastEventIDReplayAndCacheHeaders(t *testing.T) {
	ts, _ := setupTestServer(t, layoutagent.Config{})
	bridge := &mcpserver.HTTPBridgeBackend{BaseURL: ts.URL}

	r1, err := bridge.PublishReport(context.Background(), mcpserver.PublishReportInput{
		Title: "Evento 1", Project: "sse", Markdown: "# Uno\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := bridge.PublishReport(context.Background(), mcpserver.PublishReportInput{
		Title: "Evento 2", Project: "sse", Markdown: "# Dos\n",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reproducir únicamente los posteriores a id=1 (debe llegar Evento 2, no Evento 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	req.Header.Set("Last-Event-ID", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events con Last-Event-ID: %v", err)
	}
	defer resp.Body.Close()

	rd := bufio.NewReader(resp.Body)
	if first, _ := rd.ReadString('\n'); !strings.Contains(first, "connected") {
		t.Fatalf("handshake SSE inesperado: %q", first)
	}

	var replayLines []string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := rd.ReadString('\n')
		if err != nil {
			t.Fatalf("leyendo replay SSE: %v", err)
		}
		replayLines = append(replayLines, strings.TrimSpace(line))
		if strings.HasPrefix(line, "data: ") {
			break
		}
	}
	joined := strings.Join(replayLines, "\n")
	if !strings.Contains(joined, "id: 2") || !strings.Contains(joined, r2.ID) || strings.Contains(joined, r1.ID) {
		t.Fatalf("replay Last-Event-ID incorrecto:\n%s", joined)
	}

	// Verificar cabeceras anti-caché en API JSON y JS de la UI
	apiResp, err := http.Get(ts.URL + "/api/reports")
	if err != nil {
		t.Fatal(err)
	}
	apiResp.Body.Close()
	if cc := apiResp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control esperado no-store en /api/reports, obtuvo %q", cc)
	}

	jsResp, err := http.Get(ts.URL + "/app.js")
	if err != nil {
		t.Fatal(err)
	}
	jsResp.Body.Close()
	if cc := jsResp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control esperado no-cache en /app.js, obtuvo %q", cc)
	}
}

func TestHTTP_IdempotentUpdateEmitsReportUpdatedAndPreservesID(t *testing.T) {
	ts, _ := setupTestServer(t, layoutagent.Config{})
	bridge := &mcpserver.HTTPBridgeBackend{BaseURL: ts.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sseReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("conectando SSE: %v", err)
	}
	defer sseResp.Body.Close()

	rd := bufio.NewReader(sseResp.Body)
	if first, _ := rd.ReadString('\n'); !strings.Contains(first, "connected") {
		t.Fatalf("handshake SSE inesperado: %q", first)
	}

	type sseMsg struct {
		event string
		data  string
	}
	events := make(chan sseMsg, 8)
	go func() {
		defer close(events)
		evName := ""
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(line, "event: "):
				evName = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: ") && evName != "":
				events <- sseMsg{event: evName, data: strings.TrimPrefix(line, "data: ")}
				evName = ""
			}
		}
	}()

	p1, err := bridge.PublishReport(ctx, mcpserver.PublishReportInput{
		IdempotencyKey: "file:/docs/audit.md",
		Title:          "Auditoría Live",
		Project:        "dioptra",
		Markdown:       "# Auditoría Live\n\nVersión inicial.\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p1.Action != "created" || p1.Revision != 1 {
		t.Fatalf("esperaba created rev 1, obtuvo %+v", p1)
	}
	select {
	case ev := <-events:
		if ev.event != "report_published" || !strings.Contains(ev.data, p1.ID) {
			t.Fatalf("evento SSE inicial inesperado: %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout esperando report_published")
	}

	// Reenvío idéntico: unchanged sin duplicar
	pSame, err := bridge.PublishReport(ctx, mcpserver.PublishReportInput{
		IdempotencyKey: "file:/docs/audit.md",
		Title:          "Auditoría Live",
		Project:        "dioptra",
		Markdown:       "# Auditoría Live\n\nVersión inicial.\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pSame.Action != "unchanged" || pSame.ID != p1.ID || pSame.Revision != 1 {
		t.Fatalf("esperaba unchanged con mismo ID, obtuvo %+v", pSame)
	}

	// Actualización in-place: debe conservar ID y emitir report_updated con source=publish
	p2, err := bridge.PublishReport(ctx, mcpserver.PublishReportInput{
		IdempotencyKey: "file:/docs/audit.md",
		Title:          "Auditoría Live",
		Project:        "dioptra",
		Markdown:       "# Auditoría Live\n\nVersión actualizada con métricas.\n",
		ChangeNote:     "Se añadieron métricas verificadas",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p2.Action != "updated" || p2.ID != p1.ID || p2.Revision != 2 || p2.ChangeNote != "Se añadieron métricas verificadas" {
		t.Fatalf("actualización HTTP inesperada: %+v", p2)
	}
	select {
	case ev := <-events:
		if ev.event != "report_updated" || !strings.Contains(ev.data, `"source":"publish"`) || !strings.Contains(ev.data, `"revision":2`) || !strings.Contains(ev.data, "Se añadieron métricas verificadas") {
			t.Fatalf("evento SSE de actualización inesperado: %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout esperando report_updated")
	}
}
