package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"

	"dioptra/internal/board"
	"dioptra/internal/contract"
	"dioptra/internal/e2e"
	"dioptra/internal/layoutagent"
	"dioptra/internal/mcpserver"
	"dioptra/internal/store"
	"dioptra/ui"
)

const (
	// MainCSP es la política de seguridad inmutable de la aplicación principal (Requerimiento 3).
	MainCSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'"

	// CanvasIsolatedCSP es la política desacoplada exclusiva para /canvas/{docId}/{blockId} (§4.9).
	CanvasIsolatedCSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:"
)

// canvasPostMessageReceiver es el receptor ligero (~6 líneas) más ResizeObserver (§4.9)
// que alterna data-theme en vivo sin recargar el marco ni perder estado interno.
const canvasPostMessageReceiver = `
addEventListener("message",e=>{if(e.source!==parent||!e.data||typeof e.data.dxTheme!=="string")return;document.documentElement.dataset.theme=e.data.dxTheme;if(typeof e.data.dxExpanded==="boolean")document.documentElement.toggleAttribute("data-dx-expanded",e.data.dxExpanded);dispatchEvent(new CustomEvent("dx-canvas-theme",{detail:{theme:e.data.dxTheme}}));});
addEventListener("DOMContentLoaded",()=>{let last=0;const send=()=>{const h=Math.ceil(document.body.getBoundingClientRect().height||320);if(Math.abs(h-last)>1){last=h;parent.postMessage({dxCanvasHeight:h},"*");}};new ResizeObserver(send).observe(document.body);send();parent.postMessage({dxCanvasReady:true},"*");});
`

// AppServer encapsula el almacén, hub SSE, configuración y enrutador HTTP/MCP.
type AppServer struct {
	store        *store.Store
	hub          *SSEHub
	baseURL      string
	layoutCfg    layoutagent.Config
	allowedHosts map[string]struct{}
	mux          *http.ServeMux

	e2eOnce sync.Once
	e2e     *e2e.Manager

	boardOnce sync.Once
	board     *board.Service
}

// Options configura una instancia de AppServer.
type Options struct {
	Store        *store.Store
	BaseURL      string
	LayoutCfg    layoutagent.Config
	AllowedHosts []string
}

// New construye el servidor HTTP completo con todas las rutas REST, SSE, /canvas y /mcp.
func New(opts Options) *AppServer {
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = "http://127.0.0.1:7474"
	}
	s := &AppServer{
		store:        opts.Store,
		hub:          NewSSEHub(),
		baseURL:      base,
		layoutCfg:    opts.LayoutCfg,
		allowedHosts: normalizeAllowedHosts(opts.AllowedHosts),
		mux:          http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Hub expone el concentrador SSE para pruebas o emisiones externas.
func (s *AppServer) Hub() *SSEHub {
	return s.hub
}

// ServeHTTP implementa http.Handler. Toda solicitud pasa primero por la
// guardia anti DNS-rebinding / CSRF (guard.go).
func (s *AppServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if status, code, msg := guardRequest(r, s.allowedHosts); status != 0 {
		writeJSON(w, status, map[string]any{"error": code, "message": msg})
		return
	}
	s.mux.ServeHTTP(w, r)
}

// PublishReport implementa mcpserver.Backend directamente en memoria y notifica vía SSE.
func (s *AppServer) PublishReport(ctx context.Context, in mcpserver.PublishReportInput) (*mcpserver.PublishReportOutput, error) {
	res, err := s.store.Publish(ctx, store.PublishParams{
		ID:              in.ID,
		IdempotencyKey:  in.IdempotencyKey,
		ChangeNote:      in.ChangeNote,
		ForceNew:        in.ForceNew,
		Title:           in.Title,
		Project:         in.Project,
		Tool:            in.Tool,
		Model:           in.Model,
		Markdown:        in.Markdown,
		InteractiveMode: in.InteractiveMode,
		BlocksOverride:  in.BlocksOverride,
		LayoutCfg:       s.layoutCfg,
	})
	if err != nil {
		return nil, err
	}

	switch res.Action {
	case "updated":
		s.hub.Broadcast("report_updated", map[string]any{
			"summary": res.Summary,
			"source":  "publish",
		})
	case "unchanged":
		// Publicación idempotente sin cambios: no duplicar eventos SSE ni re-renderizar.
	default:
		s.hub.Broadcast("report_published", map[string]any{
			"summary": res.Summary,
		})
	}

	warnings := res.Warnings
	if warnings == nil {
		warnings = []contract.Warning{}
	}

	return &mcpserver.PublishReportOutput{
		OK:           true,
		ID:           res.ID,
		Action:       res.Action,
		Revision:     res.Revision,
		ChangeNote:   res.ChangeNote,
		URL:          fmt.Sprintf("%s/?report=%s", s.baseURL, res.ID),
		MarkdownPath: res.MarkdownPath,
		ModeUsed:     res.ModeUsed,
		Summary:      res.Summary,
		Warnings:     warnings,
		Layout:       res.Layout,
		Fidelity:     res.Fidelity,
		Cost:         res.Cost,
	}, nil
}

// ListReports implementa mcpserver.Backend.
func (s *AppServer) ListReports(ctx context.Context, in mcpserver.ListReportsInput) (*mcpserver.ListReportsOutput, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	items, err := s.store.ListReportsFiltered(ctx, in.Project, in.Tool, in.Model, in.Query, limit)
	if err != nil {
		return nil, err
	}
	return &mcpserver.ListReportsOutput{
		Count:   len(items),
		Reports: items,
	}, nil
}

// GetReportState implementa mcpserver.Backend.
func (s *AppServer) GetReportState(ctx context.Context, in mcpserver.GetReportStateInput) (*mcpserver.GetReportStateOutput, error) {
	st, err := s.store.GetReportState(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &mcpserver.GetReportStateOutput{
		ID:           st.Summary.ID,
		Title:        st.Summary.Title,
		Project:      st.Summary.Project,
		TS:           st.Summary.TS,
		MarkdownPath: st.Summary.MarkdownPath,
		Checklists:   st.Checklists,
		Markdown:     st.Markdown,
	}, nil
}

func (s *AppServer) registerRoutes() {
	// 1. Salud del demonio local
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"service": "dioptra",
			"dir":     s.store.BaseDir(),
		})
	})

	// 2. Endpoint MCP HTTP sin estado (/mcp)
	mcpHandler := mcpserver.NewStatelessHTTPHandler(s)
	s.mux.Handle("/mcp", mcpHandler)

	// 3. Eventos en vivo SSE
	s.mux.Handle("GET /api/events", s.hub)

	// 4. API REST de reportes
	s.mux.HandleFunc("GET /api/reports", s.handleListReports)
	s.mux.HandleFunc("POST /api/reports", s.handlePublishReportHTTP)
	s.mux.HandleFunc("GET /api/stats", s.handleCostStats)
	s.mux.HandleFunc("GET /api/projects", s.handleListProjects)
	s.mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	s.registerWorkspaceRoutes()
	s.registerCanvasRoutes()
	s.registerE2ERoutes()
	s.registerBoardRoutes()
	s.mux.HandleFunc("GET /api/reports/{id}", s.handleGetReport)
	s.mux.HandleFunc("GET /api/reports/{id}/state", s.handleGetReportState)
	s.mux.HandleFunc("PATCH /api/reports/{id}/check", s.handlePatchCheck)
	s.mux.Handle("GET /api/source", newSourceHandler(s.store.BaseDir(), s.baseURL))

	// 5. Ruta aislada para <dx-free-canvas> (§4.9) con su propia cabecera CSP independiente
	s.mux.HandleFunc("GET /canvas/{docId}/{blockId}", s.handleCanvasFrame)

	// 6. Aplicación web empaquetada (//go:embed ui.Assets, sin preview.html/preview.js)
	fileServer := http.FileServer(http.FS(ui.Assets))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Security-Policy", MainCSP)
		w.Header().Set("X-Content-Type-Options", "nosniff")

		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "" || cleanPath == "." {
			if writeCacheHeaders(w, r, "index.html") {
				return
			}
			indexBytes, err := fs.ReadFile(ui.Assets, "index.html")
			if err != nil {
				http.Error(w, "index.html no encontrado", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(indexBytes)
			return
		}

		// Bloquear explícitamente cualquier intento de pedir preview.html / preview.js en producción
		if strings.HasPrefix(cleanPath, "preview.") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(ui.Assets, cleanPath); err != nil {
			http.NotFound(w, r)
			return
		}
		if writeCacheHeaders(w, r, cleanPath) {
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *AppServer) handleListReports(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	query := r.URL.Query().Get("q")
	limit := 200
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	items, err := s.store.ListReportsFiltered(r.Context(), project, r.URL.Query().Get("tool"), r.URL.Query().Get("model"), query, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleListProjects alimenta la barra lateral de "spaces": proyectos con conteo, última actividad,
// herramientas y modelos observados.
func (s *AppServer) handleListProjects(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListProjects(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *AppServer) handleCostStats(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.CostSummary(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *AppServer) handlePublishReportHTTP(w http.ResponseWriter, r *http.Request) {
	var in mcpserver.PublishReportInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024*1024))
	if err := dec.Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("JSON inválido: %v", err)})
		return
	}
	out, err := s.PublishReport(r.Context(), in)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *AppServer) handleGetReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	env, err := s.store.GetEnvelope(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "reporte no encontrado"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func (s *AppServer) handleGetReportState(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	out, err := s.GetReportState(r.Context(), mcpserver.GetReportStateInput{ID: id})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "reporte no encontrado"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *AppServer) handlePatchCheck(w http.ResponseWriter, r *http.Request) {
	reportID := r.PathValue("id")
	var payload struct {
		ID      string `json:"id"`
		CheckID string `json:"checkId"`
		Done    bool   `json:"done"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := dec.Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("JSON inválido: %v", err)})
		return
	}
	checkID := strings.TrimSpace(payload.ID)
	if checkID == "" {
		checkID = strings.TrimSpace(payload.CheckID)
	}
	if checkID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "falta id de casilla"})
		return
	}

	summary, checks, err := s.store.UpdateChecklistItem(r.Context(), reportID, checkID, payload.Done)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "reporte no encontrado"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}

	s.hub.Broadcast("report_updated", map[string]any{
		"summary": summary,
		"source":  "check",
	})
	// Tablero de equipo: la casilla puede ser «Aprobar Tn» o el modo automático.
	s.syncBoardApprovals(r.Context(), summary.Title, summary.Project)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"summary":    summary,
		"checklists": checks,
	})
}

func (s *AppServer) handleCanvasFrame(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("docId")
	blockID := r.PathValue("blockId")

	rawHTML, ok := s.store.LookupCanvasHTML(r.Context(), docID, blockID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	theme := "dark"
	if r.URL.Query().Get("theme") == "light" {
		theme = "light"
	}

	w.Header().Set("Content-Security-Policy", CanvasIsolatedCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	baseReset := "*,*::before,*::after{box-sizing:border-box}html,body{margin:0;padding:0;background:var(--color-bg-body);color:var(--color-text-primary);font:400 var(--fs-small)/var(--lh-body) var(--font-sans)}html[data-dx-expanded] body{min-height:100vh}"
	safeTokens := strings.ReplaceAll(ui.TokensCSS, "</style", "<\\/style")
	safeReceiver := strings.ReplaceAll(canvasPostMessageReceiver, "</script", "<\\/script")

	var doc strings.Builder
	doc.WriteString("<!doctype html>")
	fmt.Fprintf(&doc, `<html lang="es" data-theme="%s"><head><meta charset="utf-8">`, theme)
	fmt.Fprintf(&doc, `<style>%s%s</style>`, safeTokens, baseReset)
	fmt.Fprintf(&doc, `<script>%s</script>`, safeReceiver)
	doc.WriteString("</head><body>")
	doc.WriteString(rawHTML)
	doc.WriteString("</body></html>")

	_, _ = w.Write([]byte(doc.String()))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
