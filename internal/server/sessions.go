package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dioptra/internal/roots"
	"dioptra/internal/store"
)

// Vista "Sesiones" (adaptación de Gentle Tower): lista las sesiones abiertas de
// Genius Shell / Gentle Shell leyendo sus archivos de estado universales
// `gentle-shell.agent-status/v1` (<home>/agent-status/<sha256(sesión)>.json).
// Solo lectura, local y acotada.

const (
	agentStatusSchema   = "gentle-shell.agent-status/v1"
	maxStatusFileBytes  = 64 * 1024
	maxSessionTitleRead = 512 * 1024
	maxSessionsReturned = 200
	staleAfter          = 10 * time.Minute // sin actualizaciones: probablemente cerrada o colgada
	dropAfter           = 24 * time.Hour   // demasiado vieja para mostrarse
)

// AgentSession es lo que la UI necesita de cada sesión.
type AgentSession struct {
	Agent            string `json:"agent"`
	Session          string `json:"session"`
	Project          string `json:"project"`
	CanonicalProject string `json:"canonicalProject,omitempty"`
	ParentWorkspace  string `json:"parentWorkspace,omitempty"`
	Cwd              string `json:"cwd"`
	State            string `json:"state"` // working | blocked | idle
	Task             string `json:"task,omitempty"`
	SessionTitle     string `json:"sessionTitle,omitempty"`
	UpdatedAt        string `json:"updatedAt"`
	Stale            bool   `json:"stale"`
}

type agentStatusFile struct {
	Schema  string `json:"schema"`
	Agent   string `json:"agent"`
	Session string `json:"session"`
	Cwd     string `json:"cwd"`
	State   string `json:"state"`
	Task    *struct {
		Title string `json:"title"`
	} `json:"task"`
	UpdatedAt string `json:"updatedAt"`
}

// sessionStatusDirs: directorios donde buscar estados. DIOPTRA_SESSION_DIRS
// (separados por el separador de rutas del SO) reemplaza los predeterminados.
func sessionStatusDirs() []string {
	if v := strings.TrimSpace(os.Getenv("DIOPTRA_SESSION_DIRS")); v != "" {
		return filepath.SplitList(v)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".genius_code", "data", "agent-status"),
		filepath.Join(home, ".gentle-pi", "agent-status"),
	}
}

// ReadAgentSessions lee y normaliza los estados de sesión de los directorios dados.
func ReadAgentSessions(dirs []string, now time.Time) []AgentSession {
	var out []AgentSession
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			info, err := e.Info()
			if err != nil || !info.Mode().IsRegular() || info.Size() > maxStatusFileBytes {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			var f agentStatusFile
			if json.Unmarshal(raw, &f) != nil || f.Schema != agentStatusSchema || f.Session == "" {
				continue
			}
			updated, err := time.Parse(time.RFC3339, f.UpdatedAt)
			if err != nil {
				updated = info.ModTime()
			}
			age := now.Sub(updated)
			if age > dropAfter {
				continue
			}
			state := f.State
			if state != "working" && state != "blocked" && state != "idle" {
				state = "idle"
			}
			s := AgentSession{
				Agent:     f.Agent,
				Session:   f.Session,
				Cwd:       f.Cwd,
				Project:   filepath.Base(filepath.Clean(f.Cwd)),
				State:     state,
				UpdatedAt: updated.UTC().Format(time.RFC3339),
				Stale:     age > staleAfter,
			}
			if f.Task != nil {
				s.Task = strings.TrimSpace(f.Task.Title)
			}
			if s.Task == "" {
				s.SessionTitle = readGeniusSessionTitle(dir, f.Session)
			}
			out = append(out, s)
		}
	}
	// Primero quien te necesita, luego quien trabaja, luego en espera; dentro, lo más reciente.
	rank := map[string]int{"blocked": 0, "working": 1, "idle": 2}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Stale != out[j].Stale {
			return !out[i].Stale
		}
		if rank[out[i].State] != rank[out[j].State] {
			return rank[out[i].State] < rank[out[j].State]
		}
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	if len(out) > maxSessionsReturned {
		out = out[:maxSessionsReturned]
	}
	return out
}

func readGeniusSessionTitle(statusDir, sessionID string) string {
	id := strings.TrimSpace(sessionID)
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return ""
	}
	sessFile := filepath.Join(filepath.Dir(statusDir), "sessions", id+".json")
	info, err := os.Stat(sessFile)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSessionTitleRead {
		return ""
	}
	raw, err := os.ReadFile(sessFile)
	if err != nil {
		return ""
	}
	var doc struct {
		Title string `json:"title"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	return strings.TrimSpace(doc.Title)
}

func (s *AppServer) enrichAgentSessions(ctx context.Context, list []AgentSession) {
	if len(list) == 0 || s.store == nil {
		return
	}
	var reg *roots.Registry
	if loaded, err := roots.Load(s.store.BaseDir()); err == nil {
		reg = loaded
	}
	for i := range list {
		proj := list[i].Project
		if reg != nil && list[i].Cwd != "" {
			if matched := matchRootProject(reg, list[i].Cwd); matched != "" {
				proj = matched
			}
		}
		canonical := s.store.ResolveProjectKey(ctx, proj)
		if canonical == "" {
			canonical = store.ProjectKey(proj)
		}
		list[i].CanonicalProject = canonical
		if parent := s.store.ParentWorkspaceOf(ctx, canonical); parent != "" {
			list[i].ParentWorkspace = parent
		}
	}
}

func matchRootProject(reg *roots.Registry, cwd string) string {
	cleanCwd := strings.ToLower(filepath.Clean(cwd))
	bestProject := ""
	bestLen := 0
	for _, e := range reg.List() {
		cleanRoot := strings.ToLower(filepath.Clean(e.Path))
		if cleanCwd == cleanRoot || strings.HasPrefix(cleanCwd, cleanRoot+string(filepath.Separator)) {
			if len(cleanRoot) > bestLen {
				bestLen = len(cleanRoot)
				bestProject = e.Project
			}
		}
	}
	return bestProject
}

func (s *AppServer) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := ReadAgentSessions(sessionStatusDirs(), time.Now())
	if sessions == nil {
		sessions = []AgentSession{}
	}
	s.enrichAgentSessions(r.Context(), sessions)
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "count": len(sessions)})
}
