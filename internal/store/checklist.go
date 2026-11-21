package store

// Aprobación de casillas (checklist) en SQLite y en el .md.
// Movido ÍNTEGRO desde store.go (F7: partir archivos gigantes).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"dioptra/internal/contract"
	"dioptra/internal/render"
)

// UpdateChecklistItem actualiza la casilla tanto en SQLite como en el archivo .md original (`- [ ]` <-> `- [x]`).
func (s *Store) UpdateChecklistItem(ctx context.Context, reportID, checkID string, done bool) (*contract.ReportSummary, []contract.ChecklistItemState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var envJSON, relPath, slug, prevUpdatedAt, lastChange string
	var warningsCount, revision int
	err := s.db.QueryRowContext(ctx, `
		SELECT slug, md_rel_path, envelope_json, warnings_count, updated_at, revision, last_change
		FROM reports WHERE id = ?`, reportID).Scan(&slug, &relPath, &envJSON, &warningsCount, &prevUpdatedAt, &revision, &lastChange)
	if err != nil {
		return nil, nil, err
	}
	if revision < 1 {
		revision = 1
	}

	var env contract.Envelope
	if err := json.Unmarshal([]byte(envJSON), &env); err != nil {
		return nil, nil, err
	}

	matchedText, foundInBlocks := mutateChecklistInBlocks(env.Report.Blocks, checkID, done)
	if !foundInBlocks {
		return nil, nil, fmt.Errorf("casilla %q no encontrada en el reporte %q", checkID, reportID)
	}

	// Sincronizar el archivo .md fuente de verdad en disco
	absPath := filepath.Join(s.baseDir, filepath.FromSlash(relPath))
	var (
		writtenMD    string
		wroteMD      bool
		updatedMDHsh string
	)
	if mdBytes, err := os.ReadFile(absPath); err == nil {
		updatedMD, ok := render.ToggleMarkdownCheckbox(string(mdBytes), checkID, matchedText, done)
		if ok {
			if err := writeFileAtomic(absPath, []byte(updatedMD)); err != nil {
				return nil, nil, fmt.Errorf("actualizando casilla en archivo .md: %w", err)
			}
			writtenMD = updatedMD
			wroteMD = true
			updatedMDHsh = computeMarkdownHash(updatedMD)
		}
	}

	checks := contract.CollectChecklists(env.Report.Blocks)
	newEnvBytes, err := json.Marshal(&env)
	if err != nil {
		return nil, nil, err
	}
	newChecksBytes, err := json.Marshal(checks)
	if err != nil {
		return nil, nil, err
	}

	cacheFile := filepath.Join(s.baseDir, ".cache", reportID+".json")
	_ = os.WriteFile(cacheFile, newEnvBytes, 0o644)

	nowStr := time.Now().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
		UPDATE reports
		SET envelope_json = ?, checks_json = ?, updated_at = ?, content_hash = '',
		    md_hash = CASE WHEN ? <> '' THEN ? ELSE md_hash END
		WHERE id = ?`, string(newEnvBytes), string(newChecksBytes), nowStr, updatedMDHsh, updatedMDHsh, reportID)
	if err != nil {
		return nil, nil, err
	}
	if wroteMD {
		s.recordDiskWrite(filepath.ToSlash(relPath), absPath, writtenMD)
	}

	summary := &contract.ReportSummary{
		ID:            reportID,
		Slug:          slug,
		Project:       env.Report.Project,
		Title:         env.Report.Title,
		TS:            env.Report.TS,
		Revision:      revision,
		MaxSev:        env.Report.MaxSev,
		Excerpt:       env.Report.Excerpt,
		MarkdownPath:  absPath,
		WarningsCount: warningsCount,
	}
	if revision > 1 {
		summary.UpdatedAt = prevUpdatedAt
		summary.LastChange = lastChange
	}
	return summary, checks, nil
}

func mutateChecklistInBlocks(blocks []map[string]any, checkID string, done bool) (string, bool) {
	for _, b := range blocks {
		t, _ := b["t"].(string)
		if t == "checklist" {
			if itemsAny, ok := b["items"].([]any); ok {
				for _, rawIt := range itemsAny {
					if it, ok := rawIt.(map[string]any); ok {
						if id, _ := it["id"].(string); id == checkID {
							it["done"] = done
							txt, _ := it["text"].(string)
							return txt, true
						}
					}
				}
			} else if itemsMap, ok := b["items"].([]map[string]any); ok {
				for _, it := range itemsMap {
					if id, _ := it["id"].(string); id == checkID {
						it["done"] = done
						txt, _ := it["text"].(string)
						return txt, true
					}
				}
			}
		}
		if sub, ok := b["blocks"].([]any); ok {
			var subMaps []map[string]any
			for _, x := range sub {
				if m, ok := x.(map[string]any); ok {
					subMaps = append(subMaps, m)
				}
			}
			if txt, found := mutateChecklistInBlocks(subMaps, checkID, done); found {
				return txt, true
			}
		} else if subMaps, ok := b["blocks"].([]map[string]any); ok {
			if txt, found := mutateChecklistInBlocks(subMaps, checkID, done); found {
				return txt, true
			}
		}
	}
	return "", false
}

func ensureChecklistInMarkdown(absPath, mdContent string, blocks []map[string]any) string {
	checks := contract.CollectChecklists(blocks)
	if len(checks) == 0 {
		return mdContent
	}
	// Si el .md ya contiene listas de tareas "- [ ]" o "- [x]", se respeta intacto
	if strings.Contains(mdContent, "- [ ]") || strings.Contains(mdContent, "- [x]") || strings.Contains(mdContent, "- [X]") {
		return mdContent
	}
	var sb strings.Builder
	sb.WriteString(strings.TrimRight(mdContent, "\r\n"))
	sb.WriteString("\n\n## Action Checklist\n\n")
	for _, c := range checks {
		mark := " "
		if c.Done {
			mark = "x"
		}
		fmt.Fprintf(&sb, "- [%s] %s <!-- %s -->\n", mark, c.Text, c.ID)
	}
	updated := sb.String()
	_ = writeFileAtomic(absPath, []byte(updated))
	return updated
}
