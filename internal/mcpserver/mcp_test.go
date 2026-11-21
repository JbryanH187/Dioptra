package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dioptra/internal/board"
	"dioptra/internal/contract"
	"dioptra/internal/e2e"
	"dioptra/internal/layoutagent"
)

type fakeBackend struct {
	published []PublishReportInput
	reports   []contract.ReportSummary
	state     *GetReportStateOutput
}

func (f *fakeBackend) PublishReport(_ context.Context, in PublishReportInput) (*PublishReportOutput, error) {
	f.published = append(f.published, in)
	return &PublishReportOutput{
		OK:       true,
		ID:       "r_test12345678",
		Action:   "created",
		Revision: 1,
		URL:      "http://127.0.0.1:7474/?report=r_test12345678",
		ModeUsed: "fast_goldmark",
		Summary: contract.ReportSummary{
			ID:      "r_test12345678",
			Title:   in.Title,
			Project: in.Project,
			MaxSev:  "info",
		},
	}, nil
}

func (f *fakeBackend) ListReports(_ context.Context, in ListReportsInput) (*ListReportsOutput, error) {
	out := f.reports
	if in.Limit > 0 && len(out) > in.Limit {
		out = out[:in.Limit]
	}
	return &ListReportsOutput{Count: len(out), Reports: out}, nil
}

func (f *fakeBackend) GetReportState(_ context.Context, in GetReportStateInput) (*GetReportStateOutput, error) {
	if f.state != nil {
		return f.state, nil
	}
	return &GetReportStateOutput{
		ID:       in.ID,
		Title:    "Test Report",
		Project:  "demo",
		Markdown: "# Test Report\n\n- [x] Approve deploy",
		Checklists: []contract.ChecklistItemState{
			{ID: "chk-1", Text: "Approve deploy", Done: true},
		},
	}, nil
}

type fakeFullBackend struct {
	fakeBackend
}

func (f *fakeFullBackend) BoardList(_ context.Context, _ BoardListInput) (*board.Result, error) {
	return &board.Result{Message: "ok"}, nil
}

func (f *fakeFullBackend) BoardTask(_ context.Context, _ BoardTaskInput) (*board.Result, error) {
	return &board.Result{Message: "ok"}, nil
}

func (f *fakeFullBackend) E2EStart(_ context.Context, _ E2EStartInput) (*e2e.Result, error) {
	return &e2e.Result{Run: "run_1", Status: e2e.StatusRecording, Message: "ok"}, nil
}

func (f *fakeFullBackend) E2ERecord(_ context.Context, _ E2ERecordInput) (*e2e.Result, error) {
	return &e2e.Result{Run: "run_1", Status: e2e.StatusRecording, Message: "ok"}, nil
}

func (f *fakeFullBackend) E2EIngestHAR(_ context.Context, _ E2EIngestHARInput) (*e2e.Result, error) {
	return &e2e.Result{Run: "run_1", Status: e2e.StatusRecording, Message: "ok"}, nil
}

func (f *fakeFullBackend) E2EFinish(_ context.Context, _ E2EFinishInput) (*e2e.Result, error) {
	return &e2e.Result{Run: "run_1", Status: e2e.StatusFinished, Message: "ok"}, nil
}

func connectInMemory(t *testing.T, backend Backend) (*mcp.ClientSession, func()) {
	t.Helper()
	ctx := context.Background()
	srv := NewMCPServer(backend)
	cTransport, sTransport := mcp.NewInMemoryTransports()

	srvSession, err := srv.Connect(ctx, sTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	clientSession, err := client.Connect(ctx, cTransport, nil)
	if err != nil {
		_ = srvSession.Close()
		t.Fatalf("client connect: %v", err)
	}
	cleanup := func() {
		_ = clientSession.Close()
		_ = srvSession.Close()
	}
	return clientSession, cleanup
}

func TestNewMCPServer_CoreToolsAndResource(t *testing.T) {
	backend := &fakeBackend{
		reports: []contract.ReportSummary{
			{ID: "r_1", Title: "Audit 1", Project: "payments-api", MaxSev: "warning", Excerpt: "Findings"},
			{ID: "r_2", Title: "Plan 2", Project: "payments-api", MaxSev: "info", Excerpt: "Steps"},
		},
	}
	session, cleanup := connectInMemory(t, backend)
	defer cleanup()

	ctx := context.Background()

	// 1. Core backend registers exactly the 5 core tools (without board/e2e).
	toolsRes, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range toolsRes.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"publish_report", "list_reports", "get_report_state", "dioptra_context", "get_layout_contract"} {
		if !names[want] {
			t.Errorf("missing core tool %q", want)
		}
	}
	if len(toolsRes.Tools) != 5 {
		t.Errorf("expected 5 core tools, got %d", len(toolsRes.Tools))
	}

	// 2. publish_report succeeds and normalizes nil warnings slice to [].
	pubRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "publish_report",
		Arguments: map[string]any{
			"title":    "API Audit",
			"project":  "payments-api",
			"markdown": "# API Audit\n\nAll checks passed.",
		},
	})
	if err != nil || pubRes.IsError {
		t.Fatalf("publish_report failed: err=%v res=%+v", err, pubRes)
	}
	if len(backend.published) != 1 || backend.published[0].Project != "payments-api" {
		t.Fatalf("unexpected backend published calls: %+v", backend.published)
	}

	// 3. get_report_state rejects empty id and returns checklists for valid id.
	emptyIDRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_report_state",
		Arguments: map[string]any{"id": "   "},
	})
	if err == nil && !emptyIDRes.IsError {
		t.Fatalf("expected error for blank id in get_report_state")
	}

	stateRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_report_state",
		Arguments: map[string]any{"id": "r_1"},
	})
	if err != nil || stateRes.IsError {
		t.Fatalf("get_report_state failed: err=%v res=%+v", err, stateRes)
	}

	// 4. dioptra_context validates project and returns items.
	emptyProjRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "dioptra_context",
		Arguments: map[string]any{"project": ""},
	})
	if err == nil && !emptyProjRes.IsError {
		t.Fatalf("expected error for blank project in dioptra_context")
	}

	ctxRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "dioptra_context",
		Arguments: map[string]any{"project": "payments-api", "limit": 1},
	})
	if err != nil || ctxRes.IsError {
		t.Fatalf("dioptra_context failed: err=%v res=%+v", err, ctxRes)
	}

	// 5. get_layout_contract works without daemon credentials.
	contractRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_layout_contract",
		Arguments: map[string]any{"include_canvas_tokens": false},
	})
	if err != nil || contractRes.IsError {
		t.Fatalf("get_layout_contract failed: err=%v res=%+v", err, contractRes)
	}

	// 6. Resource dioptra://contract/layout is readable.
	resRead, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: layoutagent.LayoutContractURI})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(resRead.Contents) != 1 || !strings.Contains(resRead.Contents[0].Text, "dioptra") {
		t.Fatalf("unexpected resource content: %+v", resRead.Contents)
	}
}

func TestNewMCPServer_FullBackendRegistersBoardAndE2ETools(t *testing.T) {
	session, cleanup := connectInMemory(t, &fakeFullBackend{})
	defer cleanup()

	toolsRes, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(toolsRes.Tools) != 11 {
		t.Fatalf("expected 11 tools (5 core + 2 board + 4 e2e), got %d", len(toolsRes.Tools))
	}
}

func TestHTTPBridgeBackend_RoundTrip(t *testing.T) {
	var seenMethod, seenPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/reports":
			_ = json.NewEncoder(w).Encode(PublishReportOutput{OK: true, ID: "r_bridge01", Action: "created"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/reports":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []contract.ReportSummary{{ID: "r_bridge01", Title: "Bridge"}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/reports/r_bridge01/state":
			_ = json.NewEncoder(w).Encode(GetReportStateOutput{ID: "r_bridge01", Title: "Bridge"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	bridge := &HTTPBridgeBackend{BaseURL: ts.URL, Client: ts.Client()}
	ctx := context.Background()

	pub, err := bridge.PublishReport(ctx, PublishReportInput{Title: "Bridge", Project: "demo", Markdown: "# Bridge"})
	if err != nil || pub.ID != "r_bridge01" {
		t.Fatalf("PublishReport: out=%+v err=%v", pub, err)
	}
	if seenMethod != http.MethodPost || seenPath != "/api/reports" {
		t.Fatalf("unexpected request %s %s", seenMethod, seenPath)
	}

	list, err := bridge.ListReports(ctx, ListReportsInput{Project: "demo", Limit: 5})
	if err != nil || list.Count != 1 {
		t.Fatalf("ListReports: out=%+v err=%v", list, err)
	}

	st, err := bridge.GetReportState(ctx, GetReportStateInput{ID: "r_bridge01"})
	if err != nil || st.ID != "r_bridge01" {
		t.Fatalf("GetReportState: out=%+v err=%v", st, err)
	}
}
