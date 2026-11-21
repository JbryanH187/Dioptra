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

	"dioptra/internal/e2e"
)

// Observador E2E: un agente documenta una prueba de punta a punta que hace una persona.
// Las descripciones van en inglés (los modelos las siguen mejor); el reporte sale en español.

// E2EStartInput abre una corrida.
type E2EStartInput struct {
	Project     string `json:"project" jsonschema:"Repository root folder name (e.g. payments-api)"`
	Title       string `json:"title" jsonschema:"Flow under test, e.g. 'Checkout · QA'"`
	Environment string `json:"environment,omitempty" jsonschema:"Environment: local, dev, QA, staging…"`
	Agent       string `json:"agent,omitempty" jsonschema:"Your CLI/agent name"`
}

// E2ERecordInput registra un evento en la corrida.
type E2ERecordInput struct {
	Run  string `json:"run" jsonschema:"Run id returned by e2e_start"`
	Kind string `json:"kind" jsonschema:"case | step | service | finding | note"`

	Case     int    `json:"case,omitempty" jsonschema:"Case number (default: current). With kind=case and an existing number, updates that case"`
	Title    string `json:"title,omitempty" jsonschema:"case: name of the test case; step: short description"`
	Expected string `json:"expected,omitempty" jsonschema:"case: expected outcome"`
	Result   string `json:"result,omitempty" jsonschema:"case: observed outcome (send when the case ends)"`

	StepKind   string `json:"step_kind,omitempty" jsonschema:"step: request | query | validation | log | ui | note"`
	From       string `json:"from,omitempty" jsonschema:"step: caller (e.g. web-frontend)"`
	Service    string `json:"service,omitempty" jsonschema:"step/service/finding: service name"`
	Method     string `json:"method,omitempty"`
	URL        string `json:"url,omitempty"`
	Status     int    `json:"status,omitempty" jsonschema:"HTTP status or exit code"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Request    string `json:"request,omitempty" jsonschema:"Request body/headers or SQL (sensitive data is masked server-side)"`
	Response   string `json:"response,omitempty"`
	Log        string `json:"log,omitempty" jsonschema:"Relevant log excerpt for this step"`
	Verdict    string `json:"verdict,omitempty" jsonschema:"ok | warn | error | info (derived from status if omitted)"`
	Detail     string `json:"detail,omitempty" jsonschema:"What was validated or observed; note: the note text"`

	BaseURL     string `json:"base_url,omitempty" jsonschema:"service: base URL"`
	Environment string `json:"environment,omitempty" jsonschema:"service: environment"`
	Port        string `json:"port,omitempty" jsonschema:"service: local port"`
	LogSource   string `json:"log_source,omitempty" jsonschema:"service: where its logs are (path, container, app log)"`

	Severity   string `json:"severity,omitempty" jsonschema:"finding: info | warning | error"`
	Issue      string `json:"issue,omitempty" jsonschema:"finding: what is missing or wrong in the logs"`
	Where      string `json:"where,omitempty" jsonschema:"finding: file:line, endpoint or step"`
	Suggestion string `json:"suggestion,omitempty" jsonschema:"finding: concrete log to add or fix"`
}

// E2EIngestHARInput convierte un HAR del navegador en pasos.
type E2EIngestHARInput struct {
	Run  string `json:"run"`
	Case int    `json:"case,omitempty" jsonschema:"Case the requests belong to (default: current)"`
	Path string `json:"path" jsonschema:"Absolute path to the .har exported from DevTools › Network › Save all as HAR"`
}

// E2EFinishInput cierra la corrida.
type E2EFinishInput struct {
	Run     string `json:"run"`
	Summary string `json:"summary" jsonschema:"3-6 lines: what was tested, result per case, main risks and log gaps"`
}

// E2EBackend es opcional: los backends que lo implementan exponen las herramientas E2E.
type E2EBackend interface {
	E2EStart(ctx context.Context, in E2EStartInput) (*e2e.Result, error)
	E2ERecord(ctx context.Context, in E2ERecordInput) (*e2e.Result, error)
	E2EIngestHAR(ctx context.Context, in E2EIngestHARInput) (*e2e.Result, error)
	E2EFinish(ctx context.Context, in E2EFinishInput) (*e2e.Result, error)
}

// EventFromInput traduce la entrada MCP al evento del dominio.
func EventFromInput(in E2ERecordInput) e2e.Event {
	return e2e.Event{
		Kind: in.Kind, Case: in.Case, Title: in.Title, Expected: in.Expected, Result: in.Result,
		StepKind: in.StepKind, From: in.From, Service: in.Service, Method: in.Method, URL: in.URL,
		Status: in.Status, DurationMs: in.DurationMs, Request: in.Request, Response: in.Response,
		Log: in.Log, Verdict: in.Verdict, Detail: in.Detail,
		BaseURL: in.BaseURL, Environment: in.Environment, Port: in.Port, LogSource: in.LogSource,
		Severity: in.Severity, Issue: in.Issue, Where: in.Where, Suggestion: in.Suggestion,
	}
}

const e2eStartDescription = `Start an E2E observation run: you document an end-to-end test that the USER performs (you do not drive the test). ` +
	`Ask first: "¿Quieres que genere el reporte end-to-end en Dioptra para tener mayor observabilidad?". ` +
	`Then map services (kind=service), open a case per test (kind=case), record steps with requests/responses, ` +
	`validations, inter-service calls and log excerpts, ingest the browser HAR, flag missing or weak logs (kind=finding) and call e2e_finish. ` +
	`Returns run id and live report url. Sensitive data is masked server-side.`

func registerE2ETools(srv *mcp.Server, b E2EBackend, notDestructive *mcp.ToolAnnotations) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "e2e_start",
		Description: e2eStartDescription,
		Annotations: notDestructive,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in E2EStartInput) (*mcp.CallToolResult, e2e.Result, error) {
		return e2eReply(b.E2EStart(ctx, in))
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "e2e_record",
		Description: "Record one event in an E2E run. kind=service {service, base_url, environment, port, log_source}; " +
			"kind=case {title, expected} opens test 1,2,3… (send case+result to close it); " +
			"kind=step {step_kind, from, service, method, url, status, duration_ms, request, response, log, verdict, detail}; " +
			"kind=finding {service, severity, issue, where, suggestion} for log-quality gaps (missing correlation id, input/output, business-rule outcome, levels, error context, or sensitive data in logs); kind=note {detail}.",
		Annotations: notDestructive,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in E2ERecordInput) (*mcp.CallToolResult, e2e.Result, error) {
		return e2eReply(b.E2ERecord(ctx, in))
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "e2e_ingest_har",
		Description: "Turn a browser HAR (DevTools › Network › Save all as HAR) into steps of a case: XHR/fetch/document requests with status, timing, request and response; static assets and CORS preflights are skipped; secrets and personal data are masked. Zero tokens: parsing happens in Dioptra.",
		Annotations: notDestructive,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in E2EIngestHARInput) (*mcp.CallToolResult, e2e.Result, error) {
		return e2eReply(b.E2EIngestHAR(ctx, in))
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "e2e_finish",
		Description: "Close an E2E run with a short summary; the report switches to 'terminado'. Give the user the url and the counts.",
		Annotations: notDestructive,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in E2EFinishInput) (*mcp.CallToolResult, e2e.Result, error) {
		return e2eReply(b.E2EFinish(ctx, in))
	})
}

func e2eReply(res *e2e.Result, err error) (*mcp.CallToolResult, e2e.Result, error) {
	if err != nil {
		return nil, e2e.Result{}, err
	}
	return nil, *res, nil
}

// --- Puente HTTP (modo stdio → demonio) ---

func (b *HTTPBridgeBackend) postE2E(ctx context.Context, path string, in any) (*e2e.Result, error) {
	if b.EnsureDaemon != nil {
		if err := b.EnsureDaemon(ctx); err != nil {
			return nil, err
		}
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(b.BaseURL, "/")+path, bytes.NewReader(body))
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
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBytes, &e) == nil && e.Error != "" {
			return nil, fmt.Errorf("%s", e.Error)
		}
		return nil, fmt.Errorf("error HTTP %d desde demonio dioptra: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}
	var out e2e.Result
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *HTTPBridgeBackend) E2EStart(ctx context.Context, in E2EStartInput) (*e2e.Result, error) {
	return b.postE2E(ctx, "/api/e2e/start", in)
}

func (b *HTTPBridgeBackend) E2ERecord(ctx context.Context, in E2ERecordInput) (*e2e.Result, error) {
	return b.postE2E(ctx, "/api/e2e/record", in)
}

func (b *HTTPBridgeBackend) E2EIngestHAR(ctx context.Context, in E2EIngestHARInput) (*e2e.Result, error) {
	return b.postE2E(ctx, "/api/e2e/har", in)
}

func (b *HTTPBridgeBackend) E2EFinish(ctx context.Context, in E2EFinishInput) (*e2e.Result, error) {
	return b.postE2E(ctx, "/api/e2e/finish", in)
}
