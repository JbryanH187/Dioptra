package server

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dioptra/internal/board"
	"dioptra/internal/mcpserver"
	"dioptra/internal/roots"
	"dioptra/internal/store"
)

// boardPublisher publica el tablero de cada proyecto como un reporte vivo.
type boardPublisher struct{ s *AppServer }

func (p boardPublisher) PublishBoard(ctx context.Context, project, markdown string) (string, string, error) {
	out, err := p.s.PublishReport(ctx, mcpserver.PublishReportInput{
		IdempotencyKey: "board:" + store.ProjectKey(project),
		Title:          "Tablero · " + project,
		Project:        project,
		Tool:           "Dioptra Tablero",
		Model:          "board",
		Markdown:       markdown,
		ChangeNote:     "Tablero actualizado",
	})
	if err != nil {
		return "", "", err
	}
	return out.ID, out.URL, nil
}

// BoardChecks lee las casillas marcadas del tablero del proyecto (modo automático y
// «Aprobar Tn»). Las marca la persona en el visor; ninguna herramienta MCP las cambia.
func (p boardPublisher) BoardChecks(ctx context.Context, project string) map[string]bool {
	out := map[string]bool{}
	list, err := p.s.store.ListReports(ctx, project, "", 200)
	if err != nil {
		return out
	}
	for _, r := range list {
		if !strings.HasPrefix(r.Title, "Tablero · ") { // la lista ya viene filtrada por proyecto
			continue
		}
		st, err := p.s.store.GetReportState(ctx, r.ID)
		if err != nil {
			return out
		}
		for _, c := range st.Checklists {
			if c.Done {
				out[board.NormCheck(c.Text)] = true
			}
		}
		return out
	}
	return out
}

// syncBoardApprovals aplica las casillas del tablero cuando la persona marca una en el visor.
func (s *AppServer) syncBoardApprovals(ctx context.Context, title, project string) {
	if !strings.HasPrefix(title, "Tablero · ") || project == "" {
		return
	}
	_, _ = s.boardService().SyncApprovals(ctx, project)
}

// PublishChat publica la bitácora tipo chat del equipo como su propio reporte.
func (p boardPublisher) PublishChat(ctx context.Context, project, markdown string) (string, string, error) {
	out, err := p.s.PublishReport(ctx, mcpserver.PublishReportInput{
		IdempotencyKey: "chat:" + store.ProjectKey(project),
		Title:          "Chat del equipo · " + project,
		Project:        project,
		Tool:           "Dioptra Tablero",
		Model:          "board",
		Markdown:       markdown,
		ChangeNote:     "Conversación actualizada",
	})
	if err != nil {
		return "", "", err
	}
	return out.ID, out.URL, nil
}

func (s *AppServer) boardService() *board.Service {
	s.boardOnce.Do(func() {
		s.board = &board.Service{Store: s.store, Pub: boardPublisher{s}, KeyOf: store.ProjectKey, Files: s.evidenceFileCheck}
	})
	return s.board
}

// evidenceFileCheck comprueba que un archivo:línea citado como evidencia exista en el
// repo del proyecto (raíz registrada con `dioptra roots`). Sin raíz, con rutas absolutas,
// con «..» o con solo un nombre de archivo no opina (known=false): mejor callar que acusar en falso.
func (s *AppServer) evidenceFileCheck(project, rel string, line int) (known, exists, lineOK bool) {
	reg, err := roots.Load(s.store.BaseDir())
	if err != nil {
		return false, false, false
	}
	root, ok := reg.Resolve(project)
	if !ok || root == "" {
		return false, false, false
	}
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") || !strings.Contains(rel, "/") {
		return false, false, false
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if r, err := filepath.Rel(root, abs); err != nil || strings.HasPrefix(r, "..") {
		return false, false, false
	}
	st, err := os.Stat(abs)
	if err != nil || st.IsDir() {
		return true, false, false
	}
	if st.Size() > 8<<20 {
		return true, true, true // demasiado grande para contar líneas: se da por buena
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return false, false, false
	}
	lines := bytes.Count(data, []byte("\n")) + 1
	return true, true, line >= 1 && line <= lines
}

// BoardTask implementa mcpserver.BoardBackend en memoria.
func (s *AppServer) BoardTask(ctx context.Context, in mcpserver.BoardTaskInput) (*board.Result, error) {
	return s.boardService().Do(ctx, mcpserver.InputToBoard(in))
}

// BoardList implementa mcpserver.BoardBackend en memoria.
func (s *AppServer) BoardList(ctx context.Context, in mcpserver.BoardListInput) (*board.Result, error) {
	return s.boardService().List(ctx, in.Project, in.Role, in.Status)
}

func (s *AppServer) handleBoardList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := s.BoardList(r.Context(), mcpserver.BoardListInput{Project: q.Get("project"), Role: q.Get("role"), Status: q.Get("status")})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *AppServer) registerBoardRoutes() {
	s.mux.HandleFunc("POST /api/board/task", handleE2E(s.BoardTask))
	s.mux.HandleFunc("GET /api/board", s.handleBoardList)
}
