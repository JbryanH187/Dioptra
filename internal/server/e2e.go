package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"dioptra/internal/e2e"
	"dioptra/internal/mcpserver"
)

// e2ePublisher publica las corridas E2E como reportes normales (visor, SSE, checklist
// aprobable y búsqueda se reutilizan sin cambios).
type e2ePublisher struct{ s *AppServer }

func (p e2ePublisher) PublishE2E(ctx context.Context, run *e2e.Run, markdown string) (string, string, error) {
	model := run.Agent
	if model == "" {
		model = "e2e"
	}
	out, err := p.s.PublishReport(ctx, mcpserver.PublishReportInput{
		IdempotencyKey: "e2e:" + run.ID,
		Title:          "E2E · " + run.Title,
		Project:        run.Project,
		Tool:           "Dioptra E2E",
		Model:          model,
		Markdown:       markdown,
		ChangeNote:     "Observador E2E: " + run.Status,
	})
	if err != nil {
		return "", "", err
	}
	return out.ID, out.URL, nil
}

func (p e2ePublisher) CheckedItems(ctx context.Context, reportID string) map[string]bool {
	done := map[string]bool{}
	st, err := p.s.store.GetReportState(ctx, reportID)
	if err != nil {
		return done
	}
	for _, c := range st.Checklists {
		if c.Done {
			done[e2e.NormalizeCheck(c.Text)] = true
		}
	}
	return done
}

func (s *AppServer) e2eManager() *e2e.Manager {
	s.e2eOnce.Do(func() {
		s.e2e = &e2e.Manager{Store: s.store, Pub: e2ePublisher{s}}
	})
	return s.e2e
}

// E2EStart implementa mcpserver.E2EBackend en memoria.
func (s *AppServer) E2EStart(ctx context.Context, in mcpserver.E2EStartInput) (*e2e.Result, error) {
	return s.e2eManager().Start(ctx, in.Project, in.Title, in.Environment, in.Agent)
}

// E2ERecord implementa mcpserver.E2EBackend en memoria.
func (s *AppServer) E2ERecord(ctx context.Context, in mcpserver.E2ERecordInput) (*e2e.Result, error) {
	return s.e2eManager().Record(ctx, in.Run, mcpserver.EventFromInput(in))
}

// E2EIngestHAR implementa mcpserver.E2EBackend en memoria.
func (s *AppServer) E2EIngestHAR(ctx context.Context, in mcpserver.E2EIngestHARInput) (*e2e.Result, error) {
	return s.e2eManager().IngestHAR(ctx, in.Run, in.Case, in.Path)
}

// E2EFinish implementa mcpserver.E2EBackend en memoria.
func (s *AppServer) E2EFinish(ctx context.Context, in mcpserver.E2EFinishInput) (*e2e.Result, error) {
	return s.e2eManager().Finish(ctx, in.Run, in.Summary)
}

// handleE2E arma endpoints POST JSON (/api/e2e/*, /api/board/task) que usa el puente `dioptra mcp`.
func handleE2E[T any, R any](call func(context.Context, T) (R, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in T
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
		if err := dec.Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON inválido: " + err.Error()})
			return
		}
		res, err := call(r.Context(), in)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "no pude publicar") {
				status = http.StatusInternalServerError
			}
			writeJSON(w, status, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func (s *AppServer) handleListE2E(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListE2ERuns(r.Context(), 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs, "count": len(runs)})
}

func (s *AppServer) registerE2ERoutes() {
	s.mux.HandleFunc("POST /api/e2e/start", handleE2E(s.E2EStart))
	s.mux.HandleFunc("POST /api/e2e/record", handleE2E(s.E2ERecord))
	s.mux.HandleFunc("POST /api/e2e/har", handleE2E(s.E2EIngestHAR))
	s.mux.HandleFunc("POST /api/e2e/finish", handleE2E(s.E2EFinish))
	s.mux.HandleFunc("GET /api/e2e", s.handleListE2E)
}
