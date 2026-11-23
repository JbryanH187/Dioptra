package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"dioptra/internal/mcpserver"
	"dioptra/internal/store"
)

const (
	// DesignCanvasCSP aísla las pantallas de un lienzo multipantalla: permite scripts
	// propios del bundle local y tipografías web de Google Fonts (solo lectura visual),
	// pero bloquea scripts externos, llamadas de red (connect-src 'none') y montaje en
	// orígenes ajenos (frame-ancestors 'self'). La UI además monta cada pantalla en
	// <iframe sandbox="allow-scripts"> (origen opaco null).
	DesignCanvasCSP = "default-src 'self' data: blob:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; img-src 'self' https: data: blob:; font-src 'self' https://fonts.gstatic.com data:; connect-src 'none'; frame-ancestors 'self'"

	designCanvasMaxFileBytes = 8 << 20 // 8 MiB por archivo estático del lienzo
)

// designCanvasProbeScript se inyecta únicamente en la respuesta HTTP de pantallas .html
// del lienzo (nunca toca los archivos en disco). Permite que el modal de Dioptra consulte
// de solo lectura qué elemento HTML está debajo del punto anotado y detecte navegación en
// modo Prototipo, manteniendo sandbox="allow-scripts" (origen opaco null).
const designCanvasProbeScript = `<script>(function(){
function cssPath(el){
  if(!el||el.nodeType!==1)return "";
  if(el.id)return "#"+el.id;
  var parts=[];var cur=el;
  for(var d=0;cur&&cur.nodeType===1&&d<4;d++){
    var seg=cur.tagName.toLowerCase();
    if(cur.classList&&cur.classList.length){
      var cls=[];for(var i=0;i<cur.classList.length&&cls.length<2;i++){var c=cur.classList[i];if(c&&!/^dx-/.test(c))cls.push(c);}
      if(cls.length)seg+="."+cls.join(".");
    }
    parts.unshift(seg);
    if(cur.id)break;
    cur=cur.parentElement;
  }
  return parts.join(" > ");
}
addEventListener("message",function(e){
  if(e.source!==parent||!e.data||!e.data.dxProbePoint)return;
  var p=e.data.dxProbePoint;
  var w=window.innerWidth||document.documentElement.clientWidth||1;
  var h=window.innerHeight||document.documentElement.clientHeight||1;
  var x=Math.max(0,Math.min(w-1,Math.round((Number(p.x)||0)*w)));
  var y=Math.max(0,Math.min(h-1,Math.round((Number(p.y)||0)*h)));
  var el=document.elementFromPoint(x,y);
  var txt=(el&&el.innerText?el.innerText:(el&&el.textContent?el.textContent:"")).replace(/\s+/g," ").trim().slice(0,120);
  parent.postMessage({dxProbeHit:{reqId:p.reqId||"",selector:cssPath(el),tag:el?el.tagName.toLowerCase():"",text:txt}},"*");
});
addEventListener("DOMContentLoaded",function(){
  var file=location.pathname.split("/").pop()||"";
  try{parent.postMessage({dxCanvasNav:{file:file,title:document.title||file}},"*");}catch(_){}
});
})();</script>`

func (s *AppServer) registerCanvasRoutes() {
	s.mux.HandleFunc("GET /api/canvases", s.handleListCanvases)
	s.mux.HandleFunc("POST /api/canvases/register", s.handleRegisterCanvas)
	s.mux.HandleFunc("GET /api/canvases/{project}/{canvasId}/manifest", s.handleCanvasManifest)
	s.mux.HandleFunc("GET /api/canvases/{project}/{canvasId}/annotations", s.handleListCanvasAnnotations)
	s.mux.HandleFunc("POST /api/canvases/{project}/{canvasId}/annotations", s.handleCreateCanvasAnnotation)
	s.mux.HandleFunc("PATCH /api/canvases/{project}/{canvasId}/annotations/{annId}", s.handleUpdateCanvasAnnotation)
	s.mux.HandleFunc("DELETE /api/canvases/{project}/{canvasId}/annotations/{annId}", s.handleDeleteCanvasAnnotation)
	s.mux.HandleFunc("GET /canvases/{project}/{canvasId}/{file...}", s.handleServeCanvasAsset)
}

// --- Implementación mcpserver.CanvasBackend ---

func (s *AppServer) CanvasRegister(ctx context.Context, in mcpserver.CanvasRegisterInput) (*mcpserver.CanvasRegisterOutput, error) {
	c, err := s.store.UpsertDesignCanvas(ctx, in.Project, in.CanvasID, in.Title, in.DirPath, in.EntryJSON)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast("workspaces_updated", map[string]any{
		"action":  "canvas_register",
		"summary": fmt.Sprintf("Lienzo %s registrado en %s", c.Title, c.ProjectKey),
	})
	s.hub.Broadcast("canvas_updated", map[string]any{
		"project":  c.ProjectKey,
		"canvasId": c.CanvasID,
		"title":    c.Title,
	})
	return &mcpserver.CanvasRegisterOutput{
		OK:     true,
		Canvas: *c,
		URL:    fmt.Sprintf("%s/?project=%s&canvas=%s", s.baseURL, url.QueryEscape(c.ProjectKey), url.QueryEscape(c.CanvasID)),
	}, nil
}

func (s *AppServer) CanvasList(ctx context.Context, in mcpserver.CanvasListInput) (*mcpserver.CanvasListOutput, error) {
	list, err := s.store.ListDesignCanvases(ctx, in.Project)
	if err != nil {
		return nil, err
	}
	return &mcpserver.CanvasListOutput{
		Count:    len(list),
		Canvases: list,
	}, nil
}

func (s *AppServer) CanvasGetFeedback(ctx context.Context, in mcpserver.CanvasFeedbackInput) (*mcpserver.CanvasFeedbackOutput, error) {
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "open"
	}
	canvasID := strings.TrimSpace(in.CanvasID)
	if canvasID == "_" {
		canvasID = ""
	}
	list, err := s.store.ListCanvasAnnotations(ctx, in.Project, canvasID, status, in.BoardFile)
	if err != nil {
		return nil, err
	}
	return &mcpserver.CanvasFeedbackOutput{
		Project:     s.store.ResolveProjectKey(ctx, in.Project),
		CanvasID:    canvasID,
		Count:       len(list),
		Annotations: list,
		Summary:     formatAnnotationsSummary(list),
	}, nil
}

func (s *AppServer) CanvasResolveFeedback(ctx context.Context, in mcpserver.CanvasResolveInput) (*mcpserver.CanvasResolveOutput, error) {
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "applied"
	}
	ann, err := s.store.UpdateCanvasAnnotation(ctx, in.Project, in.CanvasID, in.AnnotationID, status, "", in.Note)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast("canvas_annotations_updated", map[string]any{
		"project":   ann.ProjectKey,
		"canvasId":  ann.CanvasID,
		"boardFile": ann.BoardFile,
		"id":        ann.ID,
		"status":    ann.Status,
	})
	return &mcpserver.CanvasResolveOutput{
		OK:         true,
		Annotation: *ann,
	}, nil
}

func formatAnnotationsSummary(list []store.CanvasAnnotation) string {
	if len(list) == 0 {
		return "Sin comentarios pendientes en este lienzo."
	}
	var b strings.Builder
	for i, a := range list {
		if i > 0 {
			b.WriteByte('\n')
		}
		target := ""
		if a.TargetSelector != "" || a.TargetText != "" {
			target = fmt.Sprintf(" [el: %s %q]", a.TargetSelector, a.TargetText)
		}
		fmt.Fprintf(&b, "- [%s · %s · %s · %s]%s geom=%s — %s",
			a.ID, a.BoardFile, a.Kind, a.Status, target, string(a.Geometry), a.Comment)
	}
	return b.String()
}

// --- Handlers HTTP ---

func (s *AppServer) handleListCanvases(w http.ResponseWriter, r *http.Request) {
	out, err := s.CanvasList(r.Context(), mcpserver.CanvasListInput{Project: r.URL.Query().Get("project")})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *AppServer) handleRegisterCanvas(w http.ResponseWriter, r *http.Request) {
	var in mcpserver.CanvasRegisterInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := dec.Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("JSON inválido: %v", err)})
		return
	}
	out, err := s.CanvasRegister(r.Context(), in)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *AppServer) handleCanvasManifest(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	canvasID := r.PathValue("canvasId")
	c, err := s.store.GetDesignCanvas(r.Context(), project, canvasID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "lienzo no encontrado"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	rawManifest, err := readContainedCanvasFile(c.DirPath, c.EntryJSON)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("manifiesto inaccesible: %v", err)})
		return
	}
	var manifest any
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": fmt.Sprintf("canvas.json inválido: %v", err)})
		return
	}
	annotations, err := s.store.ListCanvasAnnotations(r.Context(), c.ProjectKey, c.CanvasID, "all", "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	// Directorio base relativo donde viven las pantallas *.dc.html respecto a dir_path (p. ej. "project/").
	assetDir := path.Dir(strings.ReplaceAll(c.EntryJSON, `\`, "/"))
	baseHref := fmt.Sprintf("/canvases/%s/%s/", url.PathEscape(c.ProjectKey), url.PathEscape(c.CanvasID))
	if assetDir != "." && assetDir != "" && assetDir != "/" {
		baseHref += strings.Trim(assetDir, "/") + "/"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"canvas":      c,
		"manifest":    manifest,
		"annotations": annotations,
		"baseHref":    baseHref,
	})
}

func (s *AppServer) handleListCanvasAnnotations(w http.ResponseWriter, r *http.Request) {
	out, err := s.CanvasGetFeedback(r.Context(), mcpserver.CanvasFeedbackInput{
		Project:   r.PathValue("project"),
		CanvasID:  r.PathValue("canvasId"),
		Status:    r.URL.Query().Get("status"),
		BoardFile: r.URL.Query().Get("board"),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *AppServer) handleCreateCanvasAnnotation(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		PageID         string          `json:"pageId"`
		BoardFile      string          `json:"boardFile"`
		Kind           string          `json:"kind"`
		Geometry       json.RawMessage `json:"geometry"`
		TargetSelector string          `json:"targetSelector"`
		TargetText     string          `json:"targetText"`
		Comment        string          `json:"comment"`
		Author         string          `json:"author"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024))
	if err := dec.Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("JSON inválido: %v", err)})
		return
	}
	ann, err := s.store.CreateCanvasAnnotation(r.Context(), store.CanvasAnnotation{
		ProjectKey:     r.PathValue("project"),
		CanvasID:       r.PathValue("canvasId"),
		PageID:         payload.PageID,
		BoardFile:      payload.BoardFile,
		Kind:           payload.Kind,
		Geometry:       payload.Geometry,
		TargetSelector: payload.TargetSelector,
		TargetText:     payload.TargetText,
		Comment:        payload.Comment,
		Author:         payload.Author,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.hub.Broadcast("canvas_annotations_updated", map[string]any{
		"project":   ann.ProjectKey,
		"canvasId":  ann.CanvasID,
		"boardFile": ann.BoardFile,
		"id":        ann.ID,
		"status":    ann.Status,
	})
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "annotation": ann})
}

func (s *AppServer) handleUpdateCanvasAnnotation(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Status         string `json:"status"`
		Comment        string `json:"comment"`
		ResolutionNote string `json:"resolutionNote"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := dec.Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("JSON inválido: %v", err)})
		return
	}
	ann, err := s.store.UpdateCanvasAnnotation(
		r.Context(),
		r.PathValue("project"),
		r.PathValue("canvasId"),
		r.PathValue("annId"),
		payload.Status,
		payload.Comment,
		payload.ResolutionNote,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "anotación no encontrada"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.hub.Broadcast("canvas_annotations_updated", map[string]any{
		"project":   ann.ProjectKey,
		"canvasId":  ann.CanvasID,
		"boardFile": ann.BoardFile,
		"id":        ann.ID,
		"status":    ann.Status,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "annotation": ann})
}

func (s *AppServer) handleDeleteCanvasAnnotation(w http.ResponseWriter, r *http.Request) {
	ok, err := s.store.DeleteCanvasAnnotation(r.Context(), r.PathValue("project"), r.PathValue("canvasId"), r.PathValue("annId"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "anotación no encontrada"})
		return
	}
	s.hub.Broadcast("canvas_annotations_updated", map[string]any{
		"project":  r.PathValue("project"),
		"canvasId": r.PathValue("canvasId"),
		"id":       r.PathValue("annId"),
		"deleted":  true,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *AppServer) handleServeCanvasAsset(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.GetDesignCanvas(r.Context(), r.PathValue("project"), r.PathValue("canvasId"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rawRel := r.PathValue("file")
	rel, ok := cleanRelPath(rawRel)
	if !ok || isBlockedPath(rel) {
		http.Error(w, "ruta no permitida", http.StatusForbidden)
		return
	}
	data, err := readContainedCanvasFile(c.DirPath, rel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "ruta no permitida", http.StatusForbidden)
		return
	}

	ext := strings.ToLower(path.Ext(rel))
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		switch ext {
		case ".html", ".htm":
			ct = "text/html; charset=utf-8"
		case ".js", ".mjs":
			ct = "application/javascript; charset=utf-8"
		case ".css":
			ct = "text/css; charset=utf-8"
		case ".json":
			ct = "application/json; charset=utf-8"
		case ".svg":
			ct = "image/svg+xml"
		default:
			ct = http.DetectContentType(data)
		}
	}

	w.Header().Set("Content-Security-Policy", DesignCanvasCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", ct)

	if ext == ".html" || ext == ".htm" {
		data = injectCanvasProbe(data)
	}
	_, _ = w.Write(data)
}

func readContainedCanvasFile(rootDir, rel string) ([]byte, error) {
	cleanRel, ok := cleanRelPath(rel)
	if !ok || isBlockedPath(cleanRel) {
		return nil, errors.New("ruta bloqueada")
	}
	realRoot, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		return nil, err
	}
	realPath, err := filepath.EvalSymlinks(filepath.Join(realRoot, filepath.FromSlash(cleanRel)))
	if err != nil {
		return nil, err
	}
	inner, err := filepath.Rel(realRoot, realPath)
	if err != nil || inner == ".." || strings.HasPrefix(inner, ".."+string(filepath.Separator)) || filepath.IsAbs(inner) {
		return nil, errors.New("ruta fuera del lienzo")
	}
	if isBlockedPath(filepath.ToSlash(inner)) {
		return nil, errors.New("archivo bloqueado")
	}
	fi, err := os.Stat(realPath)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > designCanvasMaxFileBytes {
		return nil, errors.New("archivo no válido o excede límite")
	}
	f, err := os.Open(realPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, designCanvasMaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > designCanvasMaxFileBytes {
		return nil, errors.New("archivo excede límite")
	}
	return data, nil
}

func injectCanvasProbe(htmlBytes []byte) []byte {
	lower := bytes.ToLower(htmlBytes)
	probe := []byte(designCanvasProbeScript)
	if idx := bytes.LastIndex(lower, []byte("</body>")); idx >= 0 {
		out := make([]byte, 0, len(htmlBytes)+len(probe))
		out = append(out, htmlBytes[:idx]...)
		out = append(out, probe...)
		out = append(out, htmlBytes[idx:]...)
		return out
	}
	return append(htmlBytes, probe...)
}
