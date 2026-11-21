package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dioptra/internal/layoutagent"
	"dioptra/internal/mcpserver"
	"dioptra/internal/roots"
)

func TestBoard_FlujoDevTesterPO(t *testing.T) {
	ts, st := setupTestServer(t, layoutagent.Config{})
	ctx := context.Background()
	task := func(in mcpserver.BoardTaskInput) map[string]any {
		t.Helper()
		code, out := postJSON(t, ts.URL+"/api/board/task", in)
		if code != 200 {
			t.Fatalf("%s %s: %d %v", in.Action, in.ID, code, out)
		}
		return out
	}

	created := task(mcpserver.BoardTaskInput{Action: "create", Project: "abonos", Title: "Rechazar abono con NSF", Criteria: []string{"422 si saldo < monto"}, By: "po · genius"})
	reportID := created["report_id"].(string)
	task(mcpserver.BoardTaskInput{Action: "claim", Project: "abonos", ID: "T1", By: "dev · genius"})
	task(mcpserver.BoardTaskInput{Action: "move", Project: "abonos", ID: "T1", To: "en_revision", Evidence: "go test ./... ok", By: "dev · genius"})
	task(mcpserver.BoardTaskInput{Action: "claim", Project: "abonos", ID: "T1", By: "tester · genius"})
	task(mcpserver.BoardTaskInput{Action: "move", Project: "abonos", ID: "T1", To: "rechazada", Reason: "saldo 0 responde 500", By: "tester · genius"})

	code, out := postJSON(t, ts.URL+"/api/board/task", mcpserver.BoardTaskInput{Action: "move", Project: "abonos", ID: "T1", To: "hecha", By: "dev"})
	if code != 400 || !strings.Contains(out["error"].(string), "no se puede pasar") {
		t.Errorf("transición inválida: %d %v", code, out)
	}

	resp, err := http.Get(ts.URL + "/api/board?" + url.Values{"project": {"abonos"}, "role": {"dev"}}.Encode())
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Tasks []struct{ ID, Status, Role string }
	}
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list.Tasks) != 1 || list.Tasks[0].Status != "rechazada" || list.Tasks[0].Role != "dev" {
		t.Errorf("lo rechazado vuelve a la cola de Dev: %+v", list.Tasks)
	}

	state, err := st.GetReportState(ctx, reportID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Tablero · abonos", "Rechazadas | 1", "saldo 0 responde 500"} {
		if !strings.Contains(state.Markdown, want) {
			t.Errorf("falta %q en el tablero publicado", want)
		}
	}
}

func TestBoard_HerramientasMCPYPuente(t *testing.T) {
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
	found := map[string]bool{}
	for _, tl := range tools.Tools {
		if tl.Name == "board_list" {
			found["list"] = tl.Annotations != nil && tl.Annotations.ReadOnlyHint
		}
		if tl.Name == "board_task" {
			found["task"] = true
		}
	}
	if !found["list"] || !found["task"] {
		t.Errorf("herramientas del tablero: %v (board_list debe ser de solo lectura)", found)
	}

	bridge := &mcpserver.HTTPBridgeBackend{BaseURL: ts.URL}
	res, err := bridge.BoardTask(ctx, mcpserver.BoardTaskInput{Action: "create", Project: "p", Title: "Tarea", Role: "QA"})
	if err != nil || res.Task.Role != "tester" {
		t.Fatalf("puente create: %+v %v", res, err)
	}
	got, err := bridge.BoardList(ctx, mcpserver.BoardListInput{Project: "p", Role: "tester"})
	if err != nil || len(got.Tasks) != 1 {
		t.Fatalf("puente list: %+v %v", got, err)
	}
	if _, err := bridge.BoardTask(ctx, mcpserver.BoardTaskInput{Action: "claim", Project: "p", ID: "T7"}); err == nil || !strings.Contains(err.Error(), "no existe la tarea") {
		t.Errorf("error legible por el puente: %v", err)
	}
}

func TestBoardYE2E_PublicanSinAvisos(t *testing.T) {
	ts, _ := setupTestServer(t, layoutagent.Config{})
	_, out := postJSON(t, ts.URL+"/api/board/task", mcpserver.BoardTaskInput{Action: "create", Project: "p", Title: "Tarea"})
	_, e2e := postJSON(t, ts.URL+"/api/e2e/start", mcpserver.E2EStartInput{Project: "p", Title: "Flujo"})
	for _, id := range []string{out["report_id"].(string), e2e["report_id"].(string)} {
		resp, err := http.Get(ts.URL + "/api/reports/" + id)
		if err != nil {
			t.Fatal(err)
		}
		var env struct {
			Warnings []map[string]any `json:"warnings"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&env)
		resp.Body.Close()
		if len(env.Warnings) != 0 {
			t.Errorf("%s publica con avisos: %v", id, env.Warnings)
		}
	}
}

func TestBoard_ComentariosVisiblesEnElTablero(t *testing.T) {
	ts, st := setupTestServer(t, layoutagent.Config{})
	_, created := postJSON(t, ts.URL+"/api/board/task", mcpserver.BoardTaskInput{Action: "create", Project: "abonos", Title: "API NSF"})
	code, out := postJSON(t, ts.URL+"/api/board/task", mcpserver.BoardTaskInput{Action: "comment", Project: "abonos", ID: "T1", Tag: "acuerdo", Text: "NSF incluye saldo 0", By: "po · genius"})
	if code != 200 || !strings.Contains(out["message"].(string), "Acuerdo registrado en T1") {
		t.Fatalf("comment: %d %v", code, out)
	}
	if code, out := postJSON(t, ts.URL+"/api/board/task", mcpserver.BoardTaskInput{Action: "comment", Project: "abonos", ID: "T1", Text: ""}); code != 400 || !strings.Contains(out["error"].(string), "requiere text") {
		t.Errorf("comentario vacío: %d %v", code, out)
	}
	state, err := st.GetReportState(context.Background(), created["report_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(state.Markdown, "- **Acuerdo** · T1 · po · genius") {
		t.Errorf("el acuerdo debe verse en el tablero:\n%s", state.Markdown)
	}
}

func TestBoard_ModeradorDetectaEvidenciaInventada(t *testing.T) {
	ts, st := setupTestServer(t, layoutagent.Config{})
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "internal", "abonos"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "internal", "abonos", "handler.go"), []byte(strings.Repeat("// línea\n", 80)), 0o644)
	reg, err := roots.Load(st.BaseDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := reg.Set("abonos", repo); err != nil {
		t.Fatal(err)
	}
	if err := reg.Save(); err != nil {
		t.Fatal(err)
	}

	do := func(in mcpserver.BoardTaskInput) {
		t.Helper()
		in.Project = "abonos"
		if code, out := postJSON(t, ts.URL+"/api/board/task", in); code != 200 {
			t.Fatalf("%s: %d %v", in.Action, code, out)
		}
	}
	do(mcpserver.BoardTaskInput{Action: "create", Title: "Real", Criteria: []string{"c"}})
	do(mcpserver.BoardTaskInput{Action: "create", Title: "Inventada", Criteria: []string{"c"}})
	for _, id := range []string{"T1", "T2"} {
		do(mcpserver.BoardTaskInput{Action: "claim", ID: id, By: "dev"})
	}
	do(mcpserver.BoardTaskInput{Action: "move", ID: "T1", To: "en_revision", Evidence: "go test ok (5 tests) · internal/abonos/handler.go:40", By: "dev"})
	do(mcpserver.BoardTaskInput{Action: "move", ID: "T2", To: "en_revision", Evidence: "go test ok (5 tests) · internal/abonos/validador.go:12", By: "dev"})

	resp, err := http.Get(ts.URL + "/api/board?project=abonos")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Alerts []struct{ Level, Task, Kind, Message string }
	}
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list.Alerts) != 1 || list.Alerts[0].Task != "T2" || list.Alerts[0].Kind != "evidencia_inexistente" || list.Alerts[0].Level != "error" {
		t.Fatalf("solo T2 cita un archivo que no existe: %+v", list.Alerts)
	}

	code, out := postJSON(t, ts.URL+"/api/board/task", mcpserver.BoardTaskInput{Action: "move", Project: "abonos", ID: "T1", To: "hecha", Evidence: "ok", By: "dev"})
	if code != 400 || !strings.Contains(out["error"].(string), "propio trabajo") {
		t.Errorf("auto-aprobación: %d %v", code, out)
	}
}

func TestBoard_AprobacionesDelUsuarioManualYAuto(t *testing.T) {
	ts, st := setupTestServer(t, layoutagent.Config{})
	ctx := context.Background()
	do := func(in mcpserver.BoardTaskInput) (int, map[string]any) {
		t.Helper()
		in.Project = "SnapProof"
		return postJSON(t, ts.URL+"/api/board/task", in)
	}
	_, created := do(mcpserver.BoardTaskInput{Action: "create", Title: "Grabar video", Criteria: []string{"graba .webm"}, By: "po · genius"})
	boardID := created["report_id"].(string)
	do(mcpserver.BoardTaskInput{Action: "claim", ID: "T1", By: "dev · genius"})
	do(mcpserver.BoardTaskInput{Action: "move", ID: "T1", To: "en_revision", Evidence: "jest 128 passed", By: "dev · genius"})
	do(mcpserver.BoardTaskInput{Action: "claim", ID: "T1", By: "tester · genius"})
	do(mcpserver.BoardTaskInput{Action: "move", ID: "T1", To: "en_revision", NextRole: "po", Evidence: "criterios verificados: jest 128 passed", By: "tester · genius"})

	// Ningún agente firma como usuario (lo que pasó en uso real: «po · Bryan»).
	if code, out := do(mcpserver.BoardTaskInput{Action: "move", ID: "T1", To: "hecha", Evidence: "ok", By: "usuario · Bryan"}); code != 400 || !strings.Contains(out["error"].(string), "ningún agente actúa como usuario") {
		t.Fatalf("firma de usuario: %d %v", code, out)
	}
	// Modo manual (por defecto): el PO aprueba y la tarea espera al usuario.
	code, out := do(mcpserver.BoardTaskInput{Action: "move", ID: "T1", To: "hecha", Evidence: "criterios del PO cubiertos", By: "po · genius"})
	if code != 200 || !strings.Contains(out["message"].(string), "esperando la aprobación del usuario") {
		t.Fatalf("modo manual: %d %v", code, out)
	}
	state, _ := st.GetReportState(ctx, boardID)
	approve, auto := "", ""
	for _, c := range state.Checklists {
		if strings.HasPrefix(c.Text, "Aprobar T1") {
			approve = c.ID
		}
		if strings.HasPrefix(c.Text, "Modo automático") {
			auto = c.ID
		}
	}
	if approve == "" || auto == "" {
		t.Fatalf("faltan casillas en el tablero: %+v", state.Checklists)
	}
	// La persona marca «Aprobar T1» en el visor → la tarea se cierra a su nombre.
	patch := func(id string, done bool) {
		t.Helper()
		body := `{"id":"` + id + `","done":` + map[bool]string{true: "true", false: "false"}[done] + `}`
		req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/reports/"+boardID+"/check", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("PATCH %s: %v %v", id, err, resp)
		}
		resp.Body.Close()
	}
	patch(approve, true)
	resp, _ := http.Get(ts.URL + "/api/board?project=SnapProof")
	var list struct {
		Tasks []struct {
			ID, Status string
			History    []struct{ By string }
		}
	}
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if list.Tasks[0].Status != "hecha" || list.Tasks[0].History[len(list.Tasks[0].History)-1].By != "usuario · visor de Dioptra" {
		t.Fatalf("aprobada por el usuario: %+v", list.Tasks[0])
	}

	// Modo automático: la persona marca la casilla y el PO cierra directo.
	state, _ = st.GetReportState(ctx, boardID)
	for _, c := range state.Checklists {
		if strings.HasPrefix(c.Text, "Modo automático") {
			auto = c.ID
		}
	}
	patch(auto, true)
	do(mcpserver.BoardTaskInput{Action: "create", Title: "Recortar", Criteria: []string{"c"}, By: "po · genius"})
	do(mcpserver.BoardTaskInput{Action: "claim", ID: "T2", By: "dev · genius"})
	do(mcpserver.BoardTaskInput{Action: "move", ID: "T2", To: "en_revision", Evidence: "jest ok (3 tests)", By: "dev · genius"})
	do(mcpserver.BoardTaskInput{Action: "claim", ID: "T2", By: "tester · genius"})
	do(mcpserver.BoardTaskInput{Action: "move", ID: "T2", To: "en_revision", NextRole: "po", Evidence: "ok (3 tests)", By: "tester · genius"})
	if code, out := do(mcpserver.BoardTaskInput{Action: "move", ID: "T2", To: "hecha", Evidence: "criterios cubiertos", By: "po · genius"}); code != 200 || !strings.Contains(out["message"].(string), "→ Hecha") {
		t.Fatalf("modo automático: %d %v", code, out)
	}
	state, _ = st.GetReportState(ctx, boardID)
	if !strings.Contains(state.Markdown, "Modo **automático**") {
		t.Error("el tablero muestra el modo automático")
	}
}
