package store

// Consultas: listar reportes, sobres, estado y canvas.
// Movido ÍNTEGRO desde store.go (F7: partir archivos gigantes).

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"dioptra/internal/contract"
	"dioptra/internal/render"
)

// ReportState expone el estado vivo de un reporte para get_report_state.
type ReportState struct {
	Summary    contract.ReportSummary        `json:"summary"`
	Checklists []contract.ChecklistItemState `json:"checklists"`
	Markdown   string                        `json:"markdown"`
}

// ListReports devuelve el historial ordenado por última actividad descendente (con filtro opcional por proyecto o búsqueda).
func (s *Store) ListReports(ctx context.Context, project, query string, limit int) ([]contract.ReportSummary, error) {
	return s.ListReportsFiltered(ctx, project, "", "", query, limit)
}

// ListReportsFiltered añade filtros exactos (sin distinguir mayúsculas) por herramienta y modelo.
// Todos los filtros se combinan con AND; el proyecto acepta cualquier grafía equivalente.
func (s *Store) ListReportsFiltered(ctx context.Context, project, tool, model, query string, limit int) ([]contract.ReportSummary, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var (
		rows *sql.Rows
		err  error
	)

	project = strings.TrimSpace(project)
	query = strings.TrimSpace(query)

	const selectCols = `SELECT r.id, r.slug, r.project, r.title, r.ts, r.updated_at, r.revision, r.last_change, r.max_sev, r.excerpt, r.md_rel_path, r.warnings_count, r.tool, r.model FROM reports r`
	const orderClause = `ORDER BY COALESCE(NULLIF(r.updated_at, ''), r.ts) DESC, r.ts DESC LIMIT ?`

	var (
		conds []string
		args  []any
	)
	if project != "" {
		group := s.projectGroupKeys(ctx, project)
		if len(group) == 0 {
			return []contract.ReportSummary{}, nil
		}
		marks := make([]string, len(group))
		for i, k := range group {
			marks[i] = "?"
			args = append(args, k)
		}
		conds = append(conds, `r.project_key IN (`+strings.Join(marks, ",")+`)`)
	}
	if tool = strings.TrimSpace(tool); tool != "" {
		conds = append(conds, `LOWER(r.tool) = LOWER(?)`)
		args = append(args, tool)
	}
	if model = strings.TrimSpace(model); model != "" {
		conds = append(conds, `LOWER(r.model) = LOWER(?)`)
		args = append(args, model)
	}
	// Búsqueda por relevancia: MATCH + filtros + BM25 + LIMIT en UNA consulta. Antes
	// se tomaban los 500 mejores por FTS y se filtraba después, lo que podía perder
	// resultados al combinar texto con proyecto/herramienta/modelo.
	if expr := ftsMatchExpr(query); query != "" && s.hasFTS && expr != "" {
		conds = append(conds, `reports_search MATCH ?`)
		args = append(args, expr, limit)
		rows, err = s.db.QueryContext(ctx, selectCols+
			` JOIN reports_search ON reports_search.id = r.id WHERE `+strings.Join(conds, ` AND `)+
			` ORDER BY bm25(reports_search, 0.0, 10.0, 2.0, 1.0, 1.0, 1.0) LIMIT ?`, args...)
	} else {
		if query != "" {
			like := "%" + query + "%"
			conds = append(conds, `(r.title LIKE ? OR r.project LIKE ? OR r.excerpt LIKE ? OR r.tool LIKE ? OR r.model LIKE ?)`)
			args = append(args, like, like, like, like, like)
		}
		where := ""
		if len(conds) > 0 {
			where = ` WHERE ` + strings.Join(conds, ` AND `)
		}
		args = append(args, limit)
		rows, err = s.db.QueryContext(ctx, selectCols+where+` `+orderClause, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []contract.ReportSummary
	for rows.Next() {
		var it contract.ReportSummary
		var relPath, updatedAt, lastChange string
		var revision int
		if err := rows.Scan(
			&it.ID, &it.Slug, &it.Project, &it.Title, &it.TS,
			&updatedAt, &revision, &lastChange,
			&it.MaxSev, &it.Excerpt, &relPath, &it.WarningsCount,
			&it.Tool, &it.Model,
		); err != nil {
			return nil, err
		}
		if revision < 1 {
			revision = 1
		}
		it.Revision = revision
		if revision > 1 {
			it.UpdatedAt = updatedAt
			it.LastChange = lastChange
		}
		it.MarkdownPath = filepath.Join(s.baseDir, filepath.FromSlash(relPath))
		out = append(out, it)
	}
	if out == nil {
		out = []contract.ReportSummary{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetEnvelope devuelve el sobre JSON precompilado (y lo regenera desde el .md si la caché faltara).
func (s *Store) GetEnvelope(ctx context.Context, id string) (*contract.Envelope, error) {
	var envJSON, relPath, title, project, ts, updatedAt, lastChange string
	var revision int
	err := s.db.QueryRowContext(ctx, `
		SELECT envelope_json, md_rel_path, title, project, ts, updated_at, revision, last_change
		FROM reports WHERE id = ?`, id).Scan(&envJSON, &relPath, &title, &project, &ts, &updatedAt, &revision, &lastChange)
	if err != nil {
		return nil, err
	}
	if revision < 1 {
		revision = 1
	}

	var env contract.Envelope
	if err := json.Unmarshal([]byte(envJSON), &env); err == nil && env.Report.Title != "" {
		env.Report.Revision = revision
		if revision > 1 {
			if env.Report.UpdatedAt == "" {
				env.Report.UpdatedAt = updatedAt
			}
			if env.Report.LastChange == "" {
				env.Report.LastChange = lastChange
			}
		}
		return &env, nil
	}

	// Regeneración limpia desde la fuente de verdad .md si la caché estuviera corrupta
	absPath := filepath.Join(s.baseDir, filepath.FromSlash(relPath))
	mdBytes, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	rebuilt, warns, err := render.ConvertMarkdownToEnvelope(ctx, title, project, ts, string(mdBytes))
	if err != nil {
		return nil, err
	}
	rebuilt.Report.ID = id
	rebuilt.Report.Revision = revision
	if revision > 1 {
		rebuilt.Report.UpdatedAt = updatedAt
		rebuilt.Report.LastChange = lastChange
	}
	rebuilt.Warnings = warns
	return rebuilt, nil
}

// GetReportState devuelve los metadatos, el Markdown vivo y las casillas aprobadas/pendientes.
func (s *Store) GetReportState(ctx context.Context, id string) (*ReportState, error) {
	var summary contract.ReportSummary
	var relPath, checksJSON, updatedAt, lastChange string
	var revision int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, slug, project, title, ts, updated_at, revision, last_change, max_sev, excerpt, md_rel_path, warnings_count, checks_json
		FROM reports WHERE id = ?`, id).Scan(
		&summary.ID, &summary.Slug, &summary.Project, &summary.Title, &summary.TS,
		&updatedAt, &revision, &lastChange,
		&summary.MaxSev, &summary.Excerpt, &relPath, &summary.WarningsCount, &checksJSON,
	)
	if err != nil {
		return nil, err
	}
	if revision < 1 {
		revision = 1
	}
	summary.Revision = revision
	if revision > 1 {
		summary.UpdatedAt = updatedAt
		summary.LastChange = lastChange
	}

	absPath := filepath.Join(s.baseDir, filepath.FromSlash(relPath))
	summary.MarkdownPath = absPath

	var checks []contract.ChecklistItemState
	_ = json.Unmarshal([]byte(checksJSON), &checks)
	if checks == nil {
		checks = []contract.ChecklistItemState{}
	}

	mdBytes, _ := os.ReadFile(absPath)
	return &ReportState{
		Summary:    summary,
		Checklists: checks,
		Markdown:   string(mdBytes),
	}, nil
}

// LookupCanvasHTML recupera el HTML aislado para GET /canvas/{docId}/{blockId}.
func (s *Store) LookupCanvasHTML(ctx context.Context, docID, blockID string) (string, bool) {
	var rawHTML string
	err := s.db.QueryRowContext(ctx, `
		SELECT html FROM canvas_blocks WHERE doc_id = ? AND block_id = ?`, docID, blockID).Scan(&rawHTML)
	if err != nil {
		return "", false
	}
	return rawHTML, true
}

func extractCanvasHTML(docID string, blocks []map[string]any, out map[string]string, seq *int) {
	for _, b := range blocks {
		t, _ := b["t"].(string)
		if t == "canvas" {
			if rawHTML, ok := b["html"].(string); ok && rawHTML != "" {
				*seq++
				blockID := fmt.Sprintf("b_%d", *seq)
				out[blockID] = rawHTML
				delete(b, "html")
				b["src"] = fmt.Sprintf("/canvas/%s/%s", docID, blockID)
			}
		}
		if subMaps, ok := b["blocks"].([]map[string]any); ok {
			extractCanvasHTML(docID, subMaps, out, seq)
		} else if subAny, ok := b["blocks"].([]any); ok {
			var conv []map[string]any
			for _, x := range subAny {
				if m, ok := x.(map[string]any); ok {
					conv = append(conv, m)
				}
			}
			extractCanvasHTML(docID, conv, out, seq)
		}
	}
}
