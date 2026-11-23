package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dioptra/internal/mcpserver"
	"dioptra/internal/store"
)

func TestWorkspacesAPIAndMCP(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	srv := New(Options{Store: st, BaseURL: "http://127.0.0.1:7474"})
	ctx := context.Background()

	for _, p := range []struct{ proj, title string }{
		{"SnapProof", "Plan 1"},
		{"snapshooter", "Guía T6"},
		{"tsugi", "Brecha"},
		{"tsugi-backend", "Tablero"},
		{"tsugi-ux", "Tablero"},
	} {
		if _, err := st.Publish(ctx, store.PublishParams{
			Project:  p.proj,
			Title:    p.title,
			Markdown: "# " + p.title + "\n\nTexto.",
		}); err != nil {
			t.Fatalf("Publish(%s): %v", p.proj, err)
		}
	}

	// 1. GET /api/workspaces/audit
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7474/api/workspaces/audit", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("audit status=%d body=%s", rec.Code, rec.Body.String())
	}
	var audit store.WorkspaceAuditResult
	if err := json.Unmarshal(rec.Body.Bytes(), &audit); err != nil {
		t.Fatalf("json audit: %v", err)
	}
	if len(audit.Duplicates) == 0 || len(audit.Subteams) < 2 {
		t.Fatalf("esperaba duplicados y >=2 subteams, obtuvo: %+v", audit)
	}

	// 2. POST /api/workspaces/organize (merge_alias + group_subteam)
	postOrg := func(in mcpserver.WorkspaceOrganizeInput) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(in)
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7474/api/workspaces/organize", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}

	if w := postOrg(mcpserver.WorkspaceOrganizeInput{Action: "merge_alias", Alias: "snapshooter", Canonical: "snapproof"}); w.Code != http.StatusOK {
		t.Fatalf("merge_alias falló: %d %s", w.Code, w.Body.String())
	}
	if w := postOrg(mcpserver.WorkspaceOrganizeInput{Action: "group_subteam", Project: "tsugi-backend", Parent: "tsugi"}); w.Code != http.StatusOK {
		t.Fatalf("group_subteam falló: %d %s", w.Code, w.Body.String())
	}

	// 3. Verificar guardia CSRF: Origin null debe ser 403.
	raw, _ := json.Marshal(mcpserver.WorkspaceOrganizeInput{Action: "ignore_pair", A: "tsugi", B: "snapproof"})
	rBad := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7474/api/workspaces/organize", bytes.NewReader(raw))
	rBad.Header.Set("Content-Type", "application/json")
	rBad.Header.Set("Origin", "null")
	wBad := httptest.NewRecorder()
	srv.ServeHTTP(wBad, rBad)
	if wBad.Code != http.StatusForbidden {
		t.Fatalf("Origin null debía ser 403, obtuvo %d", wBad.Code)
	}

	// 4. Comprobar HTTPBridgeBackend contra httptest.Server
	ts := httptest.NewServer(srv)
	defer ts.Close()
	bridge := &mcpserver.HTTPBridgeBackend{BaseURL: ts.URL, Client: ts.Client()}
	resAudit, err := bridge.WorkspaceAudit(ctx)
	if err != nil {
		t.Fatalf("bridge.WorkspaceAudit: %v", err)
	}
	if len(resAudit.Aliases) != 1 || len(resAudit.Groups) != 1 {
		t.Fatalf("esperaba 1 alias y 1 grupo tras organize: %+v", resAudit)
	}
}
