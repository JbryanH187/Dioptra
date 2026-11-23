package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dioptra/internal/store"
)

// WorkspaceAuditInput no requiere parámetros: inspecciona todos los proyectos vigentes.
type WorkspaceAuditInput struct{}

// WorkspaceOrganizeInput aplica una decisión confirmada por el usuario sobre workspaces.
type WorkspaceOrganizeInput struct {
	Action    string `json:"action" jsonschema:"merge_alias | remove_alias | group_subteam | ungroup_subteam | ignore_pair"`
	Alias     string `json:"alias,omitempty" jsonschema:"merge_alias/remove_alias: duplicate project key to fold into canonical"`
	Canonical string `json:"canonical,omitempty" jsonschema:"merge_alias: surviving canonical project key"`
	Project   string `json:"project,omitempty" jsonschema:"group_subteam/ungroup_subteam: sub-team project key (e.g. tsugi-backend)"`
	Parent    string `json:"parent,omitempty" jsonschema:"group_subteam: parent workspace key (e.g. tsugi)"`
	Label     string `json:"label,omitempty" jsonschema:"group_subteam: optional short label (defaults to suffix after parent-)"`
	A         string `json:"a,omitempty" jsonschema:"ignore_pair: first project key to keep separate"`
	B         string `json:"b,omitempty" jsonschema:"ignore_pair: second project key to keep separate"`
}

// WorkspaceOrganizeOutput confirma la acción aplicada.
type WorkspaceOrganizeOutput struct {
	OK      bool   `json:"ok"`
	Action  string `json:"action"`
	Summary string `json:"summary"`
}

// WorkspaceBackend es opcional: los backends que lo implementan exponen auditoría y organización de workspaces.
type WorkspaceBackend interface {
	WorkspaceAudit(ctx context.Context) (*store.WorkspaceAuditResult, error)
	WorkspaceOrganize(ctx context.Context, in WorkspaceOrganizeInput) (*WorkspaceOrganizeOutput, error)
}

func registerWorkspaceTools(srv *mcp.Server, b WorkspaceBackend, readOnly, notDestructive *mcp.ToolAnnotations) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "workspace_audit",
		Description: "Audit Dioptra projects to detect likely duplicates (e.g. SnapProof vs snapshooter, genius-shell vs GeniusShell) " +
			"and sub-teams belonging to a parent workspace (e.g. tsugi-backend, tsugi-mobile -> tsugi). " +
			"Read-only: present findings to the user and ask for confirmation before calling workspace_organize.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ WorkspaceAuditInput) (*mcp.CallToolResult, store.WorkspaceAuditResult, error) {
		res, err := b.WorkspaceAudit(ctx)
		if err != nil {
			return nil, store.WorkspaceAuditResult{}, err
		}
		return nil, *res, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name: "workspace_organize",
		Description: "Apply a user-confirmed workspace organization decision (additive and reversible; never deletes or moves .md reports). " +
			"Actions: merge_alias {alias, canonical}, remove_alias {alias}, group_subteam {project, parent, label?}, ungroup_subteam {project}, ignore_pair {a, b}. " +
			"Always confirm with the user first.",
		Annotations: notDestructive,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in WorkspaceOrganizeInput) (*mcp.CallToolResult, WorkspaceOrganizeOutput, error) {
		res, err := b.WorkspaceOrganize(ctx, in)
		if err != nil {
			return nil, WorkspaceOrganizeOutput{}, err
		}
		return nil, *res, nil
	})
}

func (b *HTTPBridgeBackend) WorkspaceAudit(ctx context.Context) (*store.WorkspaceAuditResult, error) {
	if b.EnsureDaemon != nil {
		if err := b.EnsureDaemon(ctx); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(b.BaseURL, "/")+"/api/workspaces/audit", nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("error HTTP %d auditando workspaces: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out store.WorkspaceAuditResult
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *HTTPBridgeBackend) WorkspaceOrganize(ctx context.Context, in WorkspaceOrganizeInput) (*WorkspaceOrganizeOutput, error) {
	if b.EnsureDaemon != nil {
		if err := b.EnsureDaemon(ctx); err != nil {
			return nil, err
		}
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(b.BaseURL, "/")+"/api/workspaces/organize", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("error HTTP %d organizando workspaces: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out WorkspaceOrganizeOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
