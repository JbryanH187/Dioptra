package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dioptra/internal/mcpserver"
	"dioptra/internal/store"
)

func TestDesignCanvasHTTP_SecurityAndMCPFlow(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	canvasRoot := t.TempDir()
	projDir := filepath.Join(canvasRoot, "project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "canvas.json"), []byte(`{
		"title":"Tsugi Test",
		"pages":[{"id":"v3","name":"v3"}],
		"boards":{"Hero.dc.html":{"x":0,"y":0,"w":390,"h":844,"title":"Hero","page":"v3","is_interactive":true}}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "Hero.dc.html"), []byte(`<!doctype html><html><body><button class="cta">Reservar</button></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canvasRoot, ".env"), []byte("SECRET=supersecret"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New(Options{Store: st, BaseURL: "http://127.0.0.1:7474"})
	ts := httptest.NewServer(srv)
	defer ts.Close()
	bridge := &mcpserver.HTTPBridgeBackend{BaseURL: ts.URL, Client: ts.Client()}
	ctx := context.Background()

	// 1. Registrar lienzo vía MCP bridge.
	regOut, err := bridge.CanvasRegister(ctx, mcpserver.CanvasRegisterInput{
		Project:  "tsugi",
		CanvasID: "tsugi-mvp",
		Title:    "Tsugi MVP",
		DirPath:  canvasRoot,
	})
	if err != nil || !regOut.OK {
		t.Fatalf("CanvasRegister: %v %+v", err, regOut)
	}

	// 2. Servir pantalla .dc.html: verifica DesignCanvasCSP e inyección en vuelo de dxProbePoint (sin mutar disco).
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7474/canvases/tsugi/tsugi-mvp/project/Hero.dc.html", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("asset status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotCSP := rec.Header().Get("Content-Security-Policy"); gotCSP != DesignCanvasCSP {
		t.Fatalf("CSP inesperada: %q", gotCSP)
	}
	if !strings.Contains(rec.Body.String(), "dxProbePoint") {
		t.Fatalf("faltó sonda de solo lectura dxProbePoint en respuesta HTML")
	}
	onDisk, _ := os.ReadFile(filepath.Join(projDir, "Hero.dc.html"))
	if strings.Contains(string(onDisk), "dxProbePoint") {
		t.Fatal("el archivo en disco NO debe modificarse nunca")
	}

	// 3. Seguridad: path traversal, %2e%2e, backslash y .env bloqueados.
	for _, bad := range []string{
		"/canvases/tsugi/tsugi-mvp/.env",
		"/canvases/tsugi/tsugi-mvp/project/%2e%2e/%2e%2e/windows/win.ini",
		"/canvases/tsugi/tsugi-mvp/project/..\\..\\windows\\win.ini",
	} {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7474"+bad, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden && w.Code != http.StatusNotFound && w.Code != http.StatusTemporaryRedirect && w.Code != http.StatusMovedPermanently {
			t.Fatalf("ruta %s debía bloquearse, obtuvo %d", bad, w.Code)
		}
	}
	if _, err := readContainedCanvasFile(canvasRoot, "../../windows/win.ini"); err == nil {
		t.Fatal("readContainedCanvasFile debía rechazar ..")
	}

	// 4. Seguridad: Origin: null (iframe sandboxed) NO puede crear anotaciones (403).
	annPayload := []byte(`{"pageId":"v3","boardFile":"Hero.dc.html","kind":"pin","geometry":{"x":0.5,"y":0.5},"comment":"Hola"}`)
	rNull := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7474/api/canvases/tsugi/tsugi-mvp/annotations", bytes.NewReader(annPayload))
	rNull.Header.Set("Content-Type", "application/json")
	rNull.Header.Set("Origin", "null")
	wNull := httptest.NewRecorder()
	srv.ServeHTTP(wNull, rNull)
	if wNull.Code != http.StatusForbidden {
		t.Fatalf("Origin: null debía rechazarse con 403, obtuvo %d", wNull.Code)
	}

	// 5. Crear anotación legítima desde la UI (mismo origen), leerla vía MCP y resolverla vía MCP.
	rCreate := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7474/api/canvases/tsugi/tsugi-mvp/annotations", bytes.NewReader(annPayload))
	rCreate.Header.Set("Content-Type", "application/json")
	rCreate.Header.Set("Origin", "http://127.0.0.1:7474")
	wCreate := httptest.NewRecorder()
	srv.ServeHTTP(wCreate, rCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("create annotation status=%d body=%s", wCreate.Code, wCreate.Body.String())
	}
	var created struct {
		Annotation store.CanvasAnnotation `json:"annotation"`
	}
	if err := json.Unmarshal(wCreate.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	fb, err := bridge.CanvasGetFeedback(ctx, mcpserver.CanvasFeedbackInput{Project: "tsugi", CanvasID: "tsugi-mvp"})
	if err != nil || fb.Count != 1 || !strings.Contains(fb.Summary, "Hero.dc.html") {
		t.Fatalf("CanvasGetFeedback inesperado: err=%v fb=%+v", err, fb)
	}

	resOut, err := bridge.CanvasResolveFeedback(ctx, mcpserver.CanvasResolveInput{
		Project:      "tsugi",
		CanvasID:     "tsugi-mvp",
		AnnotationID: created.Annotation.ID,
		Status:       "applied",
		Note:         "Botón ajustado en Hero.dc.html:1",
	})
	if err != nil || !resOut.OK || resOut.Annotation.Status != "applied" {
		t.Fatalf("CanvasResolveFeedback falló: err=%v out=%+v", err, resOut)
	}

	// 6. Verificar manifest endpoint.
	rMan := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7474/api/canvases/tsugi/tsugi-mvp/manifest", nil)
	wMan := httptest.NewRecorder()
	srv.ServeHTTP(wMan, rMan)
	if wMan.Code != http.StatusOK || !strings.Contains(wMan.Body.String(), "/canvases/tsugi/tsugi-mvp/project/") {
		t.Fatalf("manifest inesperado: %d %s", wMan.Code, wMan.Body.String())
	}
}
