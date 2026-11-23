package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dioptra/internal/contract"
	"dioptra/internal/layoutagent"
)

// PublishReportInput define los argumentos de la herramienta MCP `publish_report`.
// Las descripciones van en inglés: los modelos anfitriones las siguen con más
// precisión y con menos tokens. Los reportes y la UI siguen en el idioma del autor.
type PublishReportInput struct {
	ID              string `json:"id,omitempty" jsonschema:"Existing report id (e.g. r_a1b2c3d4e5f6) to update that same document in place"`
	IdempotencyKey  string `json:"idempotency_key,omitempty" jsonschema:"Stable key (e.g. the source file path); republishing with the same key updates the same report"`
	ChangeNote      string `json:"change_note,omitempty" jsonschema:"One line describing what changed since the previous revision (auto-computed if omitted)"`
	ForceNew        bool   `json:"force_new,omitempty" jsonschema:"true to create a new report even if one with the same project and title exists"`
	Title           string `json:"title" jsonschema:"Short descriptive title"`
	Project         string `json:"project,omitempty" jsonschema:"Name of the current repository root folder (e.g. DioptraEngine). Always the same value for the same repo"`
	Tool            string `json:"tool,omitempty" jsonschema:"Your CLI/agent (e.g. ClaudeCode, Genius Shell, Codex)"`
	Model           string `json:"model,omitempty" jsonschema:"Your active model id (e.g. claude-opus-5-5); used for the cost line"`
	Markdown        string `json:"markdown" jsonschema:"Full report in GitHub Markdown (source of truth saved to disk). Required even with blocks_override"`
	InteractiveMode bool   `json:"interactive_mode,omitempty" jsonschema:"Only if the server has its own layout LLM configured; otherwise ignored. Prefer plain Markdown or blocks_override"`
	BlocksOverride  []any  `json:"blocks_override,omitempty" jsonschema:"Optional custom blocks (report.blocks of Contract B) built with get_layout_contract; validated and audited against markdown"`
}

// PublishReportOutput es la respuesta estructurada de `publish_report` (§2 ADR 1).
type PublishReportOutput struct {
	OK           bool                   `json:"ok"`
	ID           string                 `json:"id"`
	Action       string                 `json:"action,omitempty"` // "created" | "updated" | "unchanged"
	Revision     int                    `json:"revision,omitempty"`
	ChangeNote   string                 `json:"change_note,omitempty"`
	URL          string                 `json:"url"`
	MarkdownPath string                 `json:"markdown_path"`
	ModeUsed     string                 `json:"mode_used"`
	Summary      contract.ReportSummary `json:"summary"`
	Warnings     []contract.Warning     `json:"warnings"`
	Layout       *contract.LayoutMeta   `json:"layout,omitempty"`
	Fidelity     *contract.FidelityMeta `json:"fidelity,omitempty"`
	Cost         *contract.CostReport   `json:"cost,omitempty"`
}

// GetLayoutContractInput: los tokens CSS solo hacen falta para diseñar free-canvas.
type GetLayoutContractInput struct {
	IncludeCanvasTokens bool `json:"include_canvas_tokens,omitempty" jsonschema:"true only when you will build a free-canvas block (adds ~600 tokens of CSS variable names)"`
}

// ServerInstructions es la guía que el cliente MCP inyecta al modelo anfitrión.
const ServerInstructions = `Dioptra turns long technical answers into an interactive report the user reads in the browser (http://127.0.0.1:7474). It runs locally and needs no credentials.

When to publish: deliverables over ~40 lines or when closing a task (audits, plans, architecture, code reviews, postmortems, benchmarks). Short answers stay in chat.

How: publish_report {title, project: <repo root folder name>, tool, model, markdown}. Write normal GitHub Markdown; no JSON is needed. It renders deterministically in milliseconds:
- "## Heading" -> section; "###" -> subheading inside it
- tables -> sortable tables (numeric, severity and file:line columns are detected)
- "> [!WARNING] Title" (NOTE, TIP, IMPORTANT, CAUTION, ERROR) -> severity callout
- "- [ ] task @owner [warning]" -> checklist the user can approve; read approvals with get_report_state before acting
- mermaid fences (flowchart, sequenceDiagram, stateDiagram, erDiagram, classDiagram, etc.) -> diagrams; a "kpi" fence with lines "Label | value | unit | delta | sev" -> KPI cards; "diff" fences -> highlighted diffs
- "<details><summary>Title</summary> ... </details>" -> collapsed section; file:line references become clickable

Updating: send the same id or idempotency_key plus change_note; never publish duplicates.
Workspaces sync: when the user asks to sync/deduplicate Dioptra workspaces, call workspace_audit, ask the user which duplicates to merge or sub-teams to group, then apply confirmed choices with workspace_organize.
Design canvases (opt-in): use canvas_register to expose a local multi-screen UI/UX canvas in Dioptra, canvas_get_feedback to read human pin/box/highlight comments, and canvas_resolve_feedback after applying changes.
Team board: when agents work as roles (dev, tester, po, ux), coordinate through board_list / board_task instead of chatting.
E2E observation: when the user is about to test a flow end to end, offer to document it with e2e_start / e2e_record / e2e_ingest_har / e2e_finish (live report by test case, services, logs and log-quality gaps).
After publishing: give the user a 3-5 line summary, the url and cost.consoleLine.
Custom layouts (blocks_override, free-canvas) are optional: call get_layout_contract first.`

const publishReportDescription = `Publish or update a technical report (idempotent). Send {title, project, tool, model, markdown}; plain GitHub Markdown renders as rich blocks (sections, typed tables, callouts, checklists, mermaid, kpi fences, diffs).
- Same id / idempotency_key / project+title updates the same report in place (action "updated", or "unchanged" if identical). Add change_note when updating.
- Optional blocks_override: custom Contract B blocks; call get_layout_contract first. The server audits them against markdown; check "fidelity" and "warnings" in the response.
- Response includes url and cost.consoleLine: show both to the user, outside the report.
Example: {"title":"API audit","project":"payments-api","tool":"ClaudeCode","model":"claude-opus-5-5","markdown":"# API audit\n\n## Findings\n\n| Sev | Issue | Where |\n|---|---|---|\n| error | Unvalidated refund | internal/refunds/handler.go:57 |\n\n- [ ] Patch validation @backend"}`

// ListReportsInput define los filtros opcionales de `list_reports`.
type ListReportsInput struct {
	Project string `json:"project,omitempty" jsonschema:"Filtrar por proyecto (acepta cualquier grafía equivalente: 'Dioptra Engine' = 'dioptra-engine')"`
	Tool    string `json:"tool,omitempty" jsonschema:"Filtrar por herramienta emisora (ej. Genius Shell, Agy, ClaudeCode, Codex); sin distinguir mayúsculas"`
	Model   string `json:"model,omitempty" jsonschema:"Filtrar por modelo exacto (ej. claude-sonnet-5-5); sin distinguir mayúsculas"`
	Query   string `json:"query,omitempty" jsonschema:"Texto de búsqueda libre en título, proyecto, extracto, herramienta o modelo"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Cantidad máxima de reportes a devolver (por defecto 50)"`
}

// ListReportsOutput devuelve la lista resumida de reportes indexados.
type ListReportsOutput struct {
	Count   int                      `json:"count"`
	Reports []contract.ReportSummary `json:"reports"`
}

// DioptraContextInput define los parámetros de `dioptra_context`.
type DioptraContextInput struct {
	Project string `json:"project" jsonschema:"Proyecto (cualquier grafía equivalente o alias, ej. 'dioptra' o 'DioptraEngine')"`
	Query   string `json:"query,omitempty" jsonschema:"Texto opcional para priorizar por relevancia (BM25) dentro del proyecto"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Máximo de reportes (por defecto 8, tope 20)"`
}

// ContextItem es el resumen mínimo (progressive disclosure) de un reporte para arrancar sesión.
type ContextItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Project   string `json:"project"`
	TS        string `json:"ts"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	MaxSev    string `json:"maxSev"`
	Excerpt   string `json:"excerpt"`
	Tool      string `json:"tool,omitempty"`
	Model     string `json:"model,omitempty"`
}

// DioptraContextOutput devuelve el contexto reciente del proyecto.
type DioptraContextOutput struct {
	Project string        `json:"project"`
	Count   int           `json:"count"`
	Items   []ContextItem `json:"items"`
}

// GetReportStateInput define los parámetros de `get_report_state`.
type GetReportStateInput struct {
	ID string `json:"id" jsonschema:"Identificador único del reporte (ej. r_a1b2c3d4e5f6)"`
}

// GetReportStateOutput devuelve el estado actual del reporte y sus casillas aprobadas/pendientes.
type GetReportStateOutput struct {
	ID           string                        `json:"id"`
	Title        string                        `json:"title"`
	Project      string                        `json:"project"`
	TS           string                        `json:"ts"`
	MarkdownPath string                        `json:"markdown_path"`
	Checklists   []contract.ChecklistItemState `json:"checklists"`
	Markdown     string                        `json:"markdown"`
}

// Backend abstrae la ejecución directa en memoria (modo serve) o el puente HTTP (modo stdio).
type Backend interface {
	PublishReport(ctx context.Context, in PublishReportInput) (*PublishReportOutput, error)
	ListReports(ctx context.Context, in ListReportsInput) (*ListReportsOutput, error)
	GetReportState(ctx context.Context, in GetReportStateInput) (*GetReportStateOutput, error)
}

func boolPtr(v bool) *bool { return &v }

// ServerVersion es la versión que el servidor MCP anuncia; main la fija con la del binario.
var ServerVersion = "dev"

// NewMCPServer construye el servidor MCP oficial con las herramientas registradas.
func NewMCPServer(backend Backend) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "dioptra",
		Version: ServerVersion,
	}, &mcp.ServerOptions{Instructions: ServerInstructions})

	notDestructive := &mcp.ToolAnnotations{
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(false),
	}
	readOnly := &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(false),
	}

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "publish_report",
		Description: publishReportDescription,
		Annotations: notDestructive,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input PublishReportInput) (*mcp.CallToolResult, PublishReportOutput, error) {
		out, err := backend.PublishReport(ctx, input)
		if err != nil {
			return nil, PublishReportOutput{}, err
		}
		if out.Warnings == nil {
			out.Warnings = []contract.Warning{}
		}
		return nil, *out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_reports",
		Description: "List stored reports, newest first. Optional filters: project (any spelling or alias), tool, model, free-text query, limit.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ListReportsInput) (*mcp.CallToolResult, ListReportsOutput, error) {
		out, err := backend.ListReports(ctx, input)
		if err != nil {
			return nil, ListReportsOutput{}, err
		}
		return nil, *out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_report_state",
		Description: "Current state of a report: its markdown and which checklist items the human approved (done=true) in the viewer. Call it before acting on approvals.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input GetReportStateInput) (*mcp.CallToolResult, GetReportStateOutput, error) {
		if strings.TrimSpace(input.ID) == "" {
			return nil, GetReportStateOutput{}, fmt.Errorf("el parámetro id es obligatorio")
		}
		out, err := backend.GetReportState(ctx, input)
		if err != nil {
			return nil, GetReportStateOutput{}, err
		}
		return nil, *out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "dioptra_context",
		Description: "Session start: the most recent reports of a project (title, excerpt, max severity, date) to recover previous decisions and pending items without reading full documents. Use get_report_state to open one.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input DioptraContextInput) (*mcp.CallToolResult, DioptraContextOutput, error) {
		if strings.TrimSpace(input.Project) == "" {
			return nil, DioptraContextOutput{}, fmt.Errorf("el parámetro project es obligatorio")
		}
		limit := input.Limit
		if limit <= 0 || limit > 20 {
			limit = 8
		}
		out, err := backend.ListReports(ctx, ListReportsInput{Project: input.Project, Query: input.Query, Limit: limit})
		if err != nil {
			return nil, DioptraContextOutput{}, err
		}
		items := make([]ContextItem, 0, len(out.Reports))
		for _, r := range out.Reports {
			items = append(items, ContextItem{ID: r.ID, Title: r.Title, Project: r.Project, TS: r.TS, UpdatedAt: r.UpdatedAt, MaxSev: r.MaxSev, Excerpt: r.Excerpt, Tool: r.Tool, Model: r.Model})
		}
		return nil, DioptraContextOutput{Project: input.Project, Count: len(items), Items: items}, nil
	})

	// Conocimiento visual compilado en el binario: se sirve localmente (también en el
	// puente stdio) sin depender del demonio ni de credenciales.
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_layout_contract",
		Description: "Compact rules and schema (~1.5k tokens) for building blocks_override. Only needed for custom layouts; plain Markdown does not need it. Pass include_canvas_tokens=true when designing a free-canvas.",
		Annotations: readOnly,
	}, func(_ context.Context, _ *mcp.CallToolRequest, in GetLayoutContractInput) (*mcp.CallToolResult, layoutagent.LayoutContract, error) {
		return nil, layoutagent.BuildHostContract(in.IncludeCanvasTokens), nil
	})

	srv.AddResource(&mcp.Resource{
		URI:         layoutagent.LayoutContractURI,
		Name:        "dioptra-layout-contract",
		Title:       "Dioptra layout contract",
		Description: "Compact rules and Contract B schema for building blocks_override.",
		MIMEType:    "text/markdown",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      layoutagent.LayoutContractURI,
				MIMEType: "text/markdown",
				Text:     layoutagent.LayoutContractMarkdown(),
			}},
		}, nil
	})

	// Observador E2E: solo si el backend lo implementa (serve en memoria y puente HTTP).
	if eb, ok := backend.(E2EBackend); ok {
		registerE2ETools(srv, eb, notDestructive)
	}
	// Tablero de equipo (dev · tester · po).
	if bb, ok := backend.(BoardBackend); ok {
		registerBoardTools(srv, bb, readOnly, notDestructive)
	}
	// Auditoría y organización de workspaces (duplicados y sub-equipos).
	if wb, ok := backend.(WorkspaceBackend); ok {
		registerWorkspaceTools(srv, wb, readOnly, notDestructive)
	}
	// Lienzos multipantalla opt-in y feedback visual (anotaciones).
	if cb, ok := backend.(CanvasBackend); ok {
		registerCanvasTools(srv, cb, readOnly, notDestructive)
	}

	return srv
}

// NewStatelessHTTPHandler devuelve el manejador HTTP sin estado para montar en `/mcp`.
func NewStatelessHTTPHandler(backend Backend) http.Handler {
	srv := NewMCPServer(backend)
	return mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
		return srv
	}, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})
}

// RunStdio ejecuta el servidor MCP sobre stdin/stdout usando el SDK oficial.
func RunStdio(ctx context.Context, backend Backend) error {
	srv := NewMCPServer(backend)
	return srv.Run(ctx, &mcp.StdioTransport{MaxLineLength: -1})
}

// HTTPBridgeBackend reenvía las llamadas MCP stdio hacia el demonio activo `dioptra serve`
// para garantizar actualización instantánea por SSE en el navegador abierto.
type HTTPBridgeBackend struct {
	BaseURL      string
	EnsureDaemon func(ctx context.Context) error
	Client       *http.Client
}

func (b *HTTPBridgeBackend) client() *http.Client {
	if b.Client != nil {
		return b.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (b *HTTPBridgeBackend) PublishReport(ctx context.Context, in PublishReportInput) (*PublishReportOutput, error) {
	if b.EnsureDaemon != nil {
		if err := b.EnsureDaemon(ctx); err != nil {
			return nil, err
		}
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(b.BaseURL, "/")+"/api/reports", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("error HTTP %d desde demonio dioptra: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}
	var out PublishReportOutput
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *HTTPBridgeBackend) ListReports(ctx context.Context, in ListReportsInput) (*ListReportsOutput, error) {
	if b.EnsureDaemon != nil {
		if err := b.EnsureDaemon(ctx); err != nil {
			return nil, err
		}
	}
	q := url.Values{}
	if in.Project != "" {
		q.Set("project", in.Project)
	}
	if in.Tool != "" {
		q.Set("tool", in.Tool)
	}
	if in.Model != "" {
		q.Set("model", in.Model)
	}
	if in.Query != "" {
		q.Set("q", in.Query)
	}
	if in.Limit > 0 {
		q.Set("limit", strconv.Itoa(in.Limit))
	}
	endpoint := strings.TrimRight(b.BaseURL, "/") + "/api/reports"
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("error HTTP %d listando reportes: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}
	var payload struct {
		Items []contract.ReportSummary `json:"items"`
	}
	if err := json.Unmarshal(respBytes, &payload); err != nil {
		return nil, err
	}
	if payload.Items == nil {
		payload.Items = []contract.ReportSummary{}
	}
	return &ListReportsOutput{
		Count:   len(payload.Items),
		Reports: payload.Items,
	}, nil
}

func (b *HTTPBridgeBackend) GetReportState(ctx context.Context, in GetReportStateInput) (*GetReportStateOutput, error) {
	if b.EnsureDaemon != nil {
		if err := b.EnsureDaemon(ctx); err != nil {
			return nil, err
		}
	}
	endpoint := fmt.Sprintf("%s/api/reports/%s/state", strings.TrimRight(b.BaseURL, "/"), url.PathEscape(in.ID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("error HTTP %d consultando estado: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}
	var out GetReportStateOutput
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
