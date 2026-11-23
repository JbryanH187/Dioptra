package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"dioptra/internal/mcpserver"
	"dioptra/internal/roots"
	"dioptra/internal/store"
)

func (s *AppServer) registerWorkspaceRoutes() {
	s.mux.HandleFunc("GET /api/workspaces/audit", s.handleAuditWorkspaces)
	s.mux.HandleFunc("POST /api/workspaces/organize", s.handleOrganizeWorkspace)
}

func (s *AppServer) rootsMap() map[string]string {
	reg, err := roots.Load(s.store.BaseDir())
	if err != nil || reg == nil {
		return nil
	}
	out := map[string]string{}
	for _, e := range reg.List() {
		out[e.Project] = e.Path
	}
	return out
}

// WorkspaceAudit implementa mcpserver.WorkspaceBackend.
func (s *AppServer) WorkspaceAudit(ctx context.Context) (*store.WorkspaceAuditResult, error) {
	return s.store.AuditWorkspaces(ctx, s.rootsMap())
}

// WorkspaceOrganize implementa mcpserver.WorkspaceBackend y difunde cambio por SSE.
func (s *AppServer) WorkspaceOrganize(ctx context.Context, in mcpserver.WorkspaceOrganizeInput) (*mcpserver.WorkspaceOrganizeOutput, error) {
	action := strings.ToLower(strings.TrimSpace(in.Action))
	var summary string
	switch action {
	case "merge_alias":
		a, err := s.store.SetProjectAlias(ctx, in.Alias, in.Canonical)
		if err != nil {
			return nil, err
		}
		summary = fmt.Sprintf("Alias creado: %s → %s", a.AliasKey, a.CanonicalKey)
	case "remove_alias":
		ok, err := s.store.RemoveProjectAlias(ctx, in.Alias)
		if err != nil {
			return nil, err
		}
		if !ok {
			summary = fmt.Sprintf("No existía alias para %q", in.Alias)
		} else {
			summary = fmt.Sprintf("Alias retirado: %s", in.Alias)
		}
	case "group_subteam":
		g, err := s.store.SetWorkspaceGroup(ctx, in.Project, in.Parent, in.Label)
		if err != nil {
			return nil, err
		}
		summary = fmt.Sprintf("Sub-equipo agrupado: %s → %s (%s)", g.ProjectKey, g.ParentKey, g.Label)
	case "ungroup_subteam":
		ok, err := s.store.RemoveWorkspaceGroup(ctx, in.Project)
		if err != nil {
			return nil, err
		}
		if !ok {
			summary = fmt.Sprintf("%q no estaba agrupado", in.Project)
		} else {
			summary = fmt.Sprintf("Sub-equipo desagrupado: %s", in.Project)
		}
	case "ignore_pair":
		pair, err := s.store.IgnoreWorkspacePair(ctx, in.A, in.B)
		if err != nil {
			return nil, err
		}
		summary = fmt.Sprintf("Par marcado como independiente: %s", pair)
	default:
		return nil, fmt.Errorf("acción no válida %q (usa merge_alias, remove_alias, group_subteam, ungroup_subteam o ignore_pair)", in.Action)
	}

	s.hub.Broadcast("workspaces_updated", map[string]any{
		"action":  action,
		"summary": summary,
	})
	return &mcpserver.WorkspaceOrganizeOutput{
		OK:      true,
		Action:  action,
		Summary: summary,
	}, nil
}

func (s *AppServer) handleAuditWorkspaces(w http.ResponseWriter, r *http.Request) {
	res, err := s.WorkspaceAudit(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *AppServer) handleOrganizeWorkspace(w http.ResponseWriter, r *http.Request) {
	var in mcpserver.WorkspaceOrganizeInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := dec.Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("JSON inválido: %v", err)})
		return
	}
	res, err := s.WorkspaceOrganize(r.Context(), in)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
