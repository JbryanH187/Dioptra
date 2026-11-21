package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Vista "Sesiones" (adaptación de Gentle Tower): lista las sesiones abiertas de
// Genius Shell / Gentle Shell leyendo sus archivos de estado universales
// `gentle-shell.agent-status/v1` (<home>/agent-status/<sha256(sesión)>.json).
// Solo lectura, local y acotada.

const (
	agentStatusSchema   = "gentle-shell.agent-status/v1"
	maxStatusFileBytes  = 64 * 1024
	maxSessionsReturned = 200
	staleAfter          = 10 * time.Minute // sin actualizaciones: probablemente cerrada o colgada
	dropAfter           = 24 * time.Hour   // demasiado vieja para mostrarse
)

// AgentSession es lo que la UI necesita de cada sesión.
type AgentSession struct {
	Agent     string `json:"agent"`
	Session   string `json:"session"`
	Project   string `json:"project"`
	Cwd       string `json:"cwd"`
	State     string `json:"state"` // working | blocked | idle
	Task      string `json:"task,omitempty"`
	UpdatedAt string `json:"updatedAt"`
	Stale     bool   `json:"stale"`
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

func (s *AppServer) handleListSessions(w http.ResponseWriter, _ *http.Request) {
	sessions := ReadAgentSessions(sessionStatusDirs(), time.Now())
	if sessions == nil {
		sessions = []AgentSession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "count": len(sessions)})
}
