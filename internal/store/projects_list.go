package store

import (
	"context"
	"fmt"
	"strings"
)

// ProjectSummary describe un "space" (proyecto) con su actividad agregada para la barra lateral.
type ProjectSummary struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Reports      int      `json:"reports"`
	LastActivity string   `json:"lastActivity"`
	Tools        []string `json:"tools"`
	Models       []string `json:"models"`
	Aliases      []string `json:"aliases"`
}

// ListProjects agrupa por clave canónica (alias resueltos), ordenado por última actividad descendente.
// Name es la grafía del primer reporte publicado del grupo.
func (s *Store) ListProjects(ctx context.Context) ([]ProjectSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.gkey,
		       (SELECT r2.project FROM reports r2
		          WHERE COALESCE((SELECT canonical_key FROM project_aliases a2 WHERE a2.alias_key = r2.project_key), r2.project_key) = g.gkey
		          ORDER BY r2.ts ASC LIMIT 1),
		       COUNT(*),
		       MAX(g.act),
		       COALESCE(GROUP_CONCAT(DISTINCT NULLIF(g.tool, '')), ''),
		       COALESCE(GROUP_CONCAT(DISTINCT NULLIF(g.model, '')), '')
		FROM (
		  SELECT COALESCE(a.canonical_key, r.project_key) AS gkey,
		         COALESCE(NULLIF(r.updated_at, ''), r.ts) AS act, r.tool AS tool, r.model AS model
		  FROM reports r LEFT JOIN project_aliases a ON a.alias_key = r.project_key
		) g
		GROUP BY g.gkey
		ORDER BY 4 DESC`)
	if err != nil {
		return nil, fmt.Errorf("listando proyectos: %w", err)
	}
	defer rows.Close()

	out := []ProjectSummary{}
	for rows.Next() {
		var p ProjectSummary
		var tools, models string
		if err := rows.Scan(&p.Key, &p.Name, &p.Reports, &p.LastActivity, &tools, &models); err != nil {
			return nil, err
		}
		p.Tools = splitCSV(tools)
		p.Models = splitCSV(models)
		p.Aliases = []string{}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	aliases, err := s.ListProjectAliases(ctx)
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	for i, p := range out {
		idx[p.Key] = i
	}
	for _, a := range aliases {
		if i, ok := idx[a.CanonicalKey]; ok {
			out[i].Aliases = append(out[i].Aliases, a.AliasKey)
		}
	}
	return out, nil
}

func splitCSV(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
