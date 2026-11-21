package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"dioptra/internal/roots"
)

const (
	sourceMaxBytes       = 2 << 20 // 2 MiB
	sourceBinaryProbe    = 8 << 10 // 8 KiB
	sourceDefaultContext = 40
	sourceMaxContext     = 120
)

// sourceHandler sirve GET /api/source: fragmentos de código bajo raíces registradas
// explícitamente en roots.json (nunca adivina rutas).
type sourceHandler struct {
	rootsFile string
	scheme    string
	hosts     map[string]bool // Host válidos: 127.0.0.1:<port>, localhost:<port>

	mu      sync.Mutex
	reg     *roots.Registry
	modTime time.Time
	size    int64
	loaded  bool
}

func newSourceHandler(reportsDir, baseURL string) *sourceHandler {
	h := &sourceHandler{
		rootsFile: roots.FilePath(reportsDir),
		scheme:    "http",
		hosts:     map[string]bool{},
	}
	port := "7474"
	if u, err := url.Parse(baseURL); err == nil {
		if u.Scheme != "" {
			h.scheme = strings.ToLower(u.Scheme)
		}
		if p := u.Port(); p != "" {
			port = p
		}
	}
	h.hosts["127.0.0.1:"+port] = true
	h.hosts["localhost:"+port] = true
	return h
}

// registry devuelve el registro vigente, recargando roots.json si cambió mtime/tamaño.
func (h *sourceHandler) registry() (*roots.Registry, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fi, err := os.Stat(h.rootsFile)
	var mt time.Time
	var sz int64 = -1
	if err == nil {
		mt, sz = fi.ModTime(), fi.Size()
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if h.loaded && mt.Equal(h.modTime) && sz == h.size {
		return h.reg, nil
	}
	reg, err := roots.LoadFile(h.rootsFile)
	if err != nil {
		return nil, err
	}
	h.reg, h.modTime, h.size, h.loaded = reg, mt, sz, true
	return reg, nil
}

func sourceError(w http.ResponseWriter, status int, code, msg string, extra map[string]any) {
	body := map[string]any{"error": code, "message": msg}
	for k, v := range extra {
		body[k] = v
	}
	writeJSON(w, status, body)
}

func (h *sourceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Anti DNS-rebinding: Host exacto con puerto.
	if !h.hosts[strings.ToLower(r.Host)] {
		sourceError(w, http.StatusForbidden, "invalid_host", "Host no permitido", nil)
		return
	}
	// 2. Origin, si viene, debe ser el mismo origen.
	if o := r.Header.Get("Origin"); o != "" && o != h.scheme+"://"+r.Host {
		sourceError(w, http.StatusForbidden, "invalid_origin", "Origin no permitido", nil)
		return
	}

	q := r.URL.Query()
	project := roots.Key(q.Get("project"))
	if project == "" {
		sourceError(w, http.StatusBadRequest, "missing_project", "Falta el parámetro project", nil)
		return
	}
	rel, ok := cleanRelPath(q.Get("path"))
	if !ok {
		sourceError(w, http.StatusBadRequest, "invalid_path", "Ruta inválida: debe ser relativa a la raíz, sin '..' ni NUL", nil)
		return
	}
	line := 1
	if v := q.Get("line"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			sourceError(w, http.StatusBadRequest, "invalid_line", "line debe ser un entero >= 1", nil)
			return
		}
		line = n
	}
	ctxLines := sourceDefaultContext
	if v := q.Get("context"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			sourceError(w, http.StatusBadRequest, "invalid_context", "context debe ser un entero >= 0", nil)
			return
		}
		ctxLines = min(n, sourceMaxContext)
	}

	// 3. Raíz registrada (recarga en caliente).
	reg, err := h.registry()
	if err != nil {
		sourceError(w, http.StatusInternalServerError, "roots_unreadable", "No se pudo leer roots.json", nil)
		return
	}
	root, ok := reg.Resolve(project)
	if !ok {
		sourceError(w, http.StatusNotFound, "root_not_configured", "No hay raíz de código registrada para el proyecto",
			map[string]any{"project": project, "hint": "dioptra roots set " + project + " <ruta>"})
		return
	}

	// 4. Lista de bloqueo sobre la ruta solicitada.
	if isBlockedPath(rel) {
		sourceError(w, http.StatusForbidden, "blocked_path", "Archivo bloqueado por política de seguridad", nil)
		return
	}

	// 5. Resolución de symlinks y contención.
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		sourceError(w, http.StatusNotFound, "root_unavailable", "La raíz registrada no existe o no es accesible",
			map[string]any{"project": project})
		return
	}
	realPath, err := filepath.EvalSymlinks(filepath.Join(realRoot, filepath.FromSlash(rel)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			sourceError(w, http.StatusNotFound, "file_not_found", "Archivo no encontrado", nil)
		} else {
			sourceError(w, http.StatusForbidden, "forbidden_path", "Ruta no accesible", nil)
		}
		return
	}
	inner, err := filepath.Rel(realRoot, realPath)
	if err != nil || inner == ".." || strings.HasPrefix(inner, ".."+string(filepath.Separator)) || filepath.IsAbs(inner) {
		sourceError(w, http.StatusForbidden, "outside_root", "La ruta resuelta queda fuera de la raíz", nil)
		return
	}
	if isBlockedPath(filepath.ToSlash(inner)) {
		sourceError(w, http.StatusForbidden, "blocked_path", "Archivo bloqueado por política de seguridad", nil)
		return
	}
	fi, err := os.Stat(realPath)
	if err != nil {
		sourceError(w, http.StatusNotFound, "file_not_found", "Archivo no encontrado", nil)
		return
	}
	if !fi.Mode().IsRegular() {
		sourceError(w, http.StatusBadRequest, "not_a_file", "La ruta no es un archivo regular", nil)
		return
	}
	if fi.Size() > sourceMaxBytes {
		sourceError(w, http.StatusUnsupportedMediaType, "file_too_large", "Archivo mayor a 2 MiB", nil)
		return
	}

	// 6. Lectura acotada (protege contra crecimiento entre Stat y lectura).
	f, err := os.Open(realPath)
	if err != nil {
		sourceError(w, http.StatusForbidden, "forbidden_path", "No se pudo abrir el archivo", nil)
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, sourceMaxBytes+1))
	_ = f.Close()
	if err != nil {
		sourceError(w, http.StatusInternalServerError, "read_error", "Error leyendo el archivo", nil)
		return
	}
	if len(data) > sourceMaxBytes {
		sourceError(w, http.StatusUnsupportedMediaType, "file_too_large", "Archivo mayor a 2 MiB", nil)
		return
	}
	if bytes.IndexByte(data[:min(len(data), sourceBinaryProbe)], 0) >= 0 {
		sourceError(w, http.StatusUnsupportedMediaType, "binary_file", "Archivo binario no soportado", nil)
		return
	}

	all := splitSourceLines(data)
	total := len(all)
	start, end := 1, 0
	out := []string{}
	if total > 0 {
		line = min(line, total)
		start = max(1, line-ctxLines)
		end = min(total, line+ctxLines)
		out = all[start-1 : end]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project":    project,
		"path":       rel,
		"absPath":    realPath,
		"line":       line,
		"startLine":  start,
		"endLine":    end,
		"totalLines": total,
		"language":   languageFor(rel),
		"lines":      out,
	})
}

// cleanRelPath valida y normaliza una ruta relativa (separadores / o \).
// Rechaza vacías, NUL, absolutas (/, \, UNC, unidad C:), ':' y cualquier segmento "..".
func cleanRelPath(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" || strings.ContainsRune(raw, 0) || strings.Contains(raw, ":") {
		return "", false
	}
	p := strings.ReplaceAll(raw, `\`, "/")
	if strings.HasPrefix(p, "/") || filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" {
		return "", false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", false
		}
	}
	c := path.Clean(p)
	if c == "." || c == "" {
		return "", false
	}
	return c, true
}

// isBlockedPath aplica la lista de bloqueo (sin distinguir mayúsculas) a una ruta con '/'.
func isBlockedPath(rel string) bool {
	segs := strings.Split(strings.ToLower(rel), "/")
	for _, s := range segs {
		if s == ".git" {
			return true
		}
	}
	name := segs[len(segs)-1]
	switch {
	case strings.HasPrefix(name, ".env"),
		strings.HasPrefix(name, "id_rsa"),
		strings.HasPrefix(name, "secrets."):
		return true
	}
	for _, ext := range []string{".pem", ".key", ".pfx", ".p12", ".kdbx"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func splitSourceLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	s := strings.TrimSuffix(string(data), "\n")
	parts := strings.Split(s, "\n")
	for i, l := range parts {
		parts[i] = strings.TrimSuffix(l, "\r")
	}
	return parts
}

var sourceLanguages = map[string]string{
	".go": "go", ".py": "python", ".ts": "typescript", ".tsx": "tsx", ".js": "javascript",
	".mjs": "javascript", ".cjs": "javascript", ".jsx": "jsx", ".json": "json", ".md": "markdown",
	".css": "css", ".scss": "scss", ".html": "html", ".htm": "html", ".xml": "xml", ".yaml": "yaml",
	".yml": "yaml", ".toml": "toml", ".sql": "sql", ".sh": "bash", ".bash": "bash", ".ps1": "powershell",
	".java": "java", ".kt": "kotlin", ".cs": "csharp", ".c": "c", ".h": "c", ".cpp": "cpp", ".hpp": "cpp",
	".cc": "cpp", ".rs": "rust", ".rb": "ruby", ".php": "php", ".swift": "swift", ".vue": "vue",
	".svelte": "svelte", ".lua": "lua", ".dart": "dart", ".scala": "scala", ".r": "r",
}

func languageFor(rel string) string {
	base := strings.ToLower(path.Base(rel))
	switch base {
	case "dockerfile":
		return "dockerfile"
	case "makefile":
		return "makefile"
	}
	if lang, ok := sourceLanguages[path.Ext(base)]; ok {
		return lang
	}
	return "text"
}
