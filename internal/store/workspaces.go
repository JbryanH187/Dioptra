package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// WorkspaceGroup declara que un proyecto (sub-equipo) pertenece a un workspace padre
// sin fusionar sus reportes ni su tablero (p. ej. tsugi-backend → padre tsugi, etiqueta backend).
type WorkspaceGroup struct {
	ProjectKey string `json:"projectKey"`
	ParentKey  string `json:"parentKey"`
	Label      string `json:"label"`
	CreatedAt  string `json:"createdAt"`
}

// DuplicateCandidate representa dos proyectos que probablemente son el mismo workspace
// (p. ej. SnapProof vs snapshooter, genius-shell vs GeniusShell).
type DuplicateCandidate struct {
	A          string  `json:"a"`
	AName      string  `json:"aName"`
	AReports   int     `json:"aReports"`
	B          string  `json:"b"`
	BName      string  `json:"bName"`
	BReports   int     `json:"bReports"`
	Score      float64 `json:"score"`
	Reason     string  `json:"reason"`
	Suggestion string  `json:"suggestion"` // clave canónica sugerida (la de más reportes o más corta)
}

// SubteamCandidate sugiere agrupar un proyecto bajo un workspace padre conservando su tablero aparte.
type SubteamCandidate struct {
	Project    string `json:"project"`
	Name       string `json:"name"`
	Reports    int    `json:"reports"`
	Parent     string `json:"parent"`
	ParentName string `json:"parentName"`
	Label      string `json:"label"`
	Reason     string `json:"reason"`
}

// WorkspaceAuditResult resume las sugerencias deterministas de deduplicación y jerarquía.
type WorkspaceAuditResult struct {
	Projects   int                  `json:"projects"`
	Duplicates []DuplicateCandidate `json:"duplicates"`
	Subteams   []SubteamCandidate   `json:"subteams"`
	Aliases    []ProjectAlias       `json:"aliases"`
	Groups     []WorkspaceGroup     `json:"groups"`
	Ignored    []string             `json:"ignored"`
}

// ResolveProjectKey devuelve la clave canónica vigente de un nombre o clave de proyecto.
func (s *Store) ResolveProjectKey(ctx context.Context, project string) string {
	return s.resolveProjectKey(ctx, ProjectKey(project))
}

// ParentWorkspaceOf devuelve el workspace padre (clave canónica) de un proyecto, o "" si es raíz.
func (s *Store) ParentWorkspaceOf(ctx context.Context, project string) string {
	key := s.resolveProjectKey(ctx, ProjectKey(project))
	if key == "" {
		return ""
	}
	var parent string
	if err := s.db.QueryRowContext(ctx, `SELECT parent_key FROM workspace_groups WHERE project_key = ?`, key).Scan(&parent); err == nil && parent != "" {
		return s.resolveProjectKey(ctx, parent)
	}
	return ""
}

// SetWorkspaceGroup agrupa project bajo el workspace parent (aditivo y reversible).
// Rechaza ciclos y cadenas (un padre no puede ser a su vez hijo de otro grupo).
func (s *Store) SetWorkspaceGroup(ctx context.Context, project, parent, label string) (*WorkspaceGroup, error) {
	pKey := s.resolveProjectKey(ctx, ProjectKey(project))
	parentKey := s.resolveProjectKey(ctx, ProjectKey(parent))
	switch {
	case pKey == "" || parentKey == "":
		return nil, errors.New("proyecto y workspace padre no pueden quedar vacíos")
	case pKey == parentKey:
		return nil, errors.New("un proyecto no puede ser sub-equipo de sí mismo")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_groups WHERE project_key = ?`, parentKey).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("el workspace %q ya es sub-equipo de otro; usa el workspace raíz", parentKey)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_groups WHERE parent_key = ?`, pKey).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("%q ya tiene sub-equipos propios; desagrúpalos primero para evitar cadenas", pKey)
	}

	lbl := strings.TrimSpace(label)
	if lbl == "" {
		lbl = deriveSubteamLabel(pKey, parentKey)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO workspace_groups (project_key, parent_key, label, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(project_key) DO UPDATE SET parent_key = excluded.parent_key, label = excluded.label`,
		pKey, parentKey, lbl, now); err != nil {
		return nil, fmt.Errorf("guardando grupo de workspace: %w", err)
	}
	return &WorkspaceGroup{ProjectKey: pKey, ParentKey: parentKey, Label: lbl, CreatedAt: now}, nil
}

// RemoveWorkspaceGroup desagrupa un sub-equipo y lo devuelve a la raíz.
func (s *Store) RemoveWorkspaceGroup(ctx context.Context, project string) (bool, error) {
	pKey := s.resolveProjectKey(ctx, ProjectKey(project))
	if pKey == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `DELETE FROM workspace_groups WHERE project_key = ?`, pKey)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListWorkspaceGroups devuelve todas las relaciones sub-equipo → workspace padre.
func (s *Store) ListWorkspaceGroups(ctx context.Context) ([]WorkspaceGroup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT project_key, parent_key, label, created_at FROM workspace_groups ORDER BY parent_key, project_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkspaceGroup{}
	for rows.Next() {
		var g WorkspaceGroup
		if err := rows.Scan(&g.ProjectKey, &g.ParentKey, &g.Label, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func canonicalPairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

// IgnoreWorkspacePair registra que el usuario decidió mantener separados a y b,
// para que futuras auditorías no vuelvan a sugerir unirlos.
func (s *Store) IgnoreWorkspacePair(ctx context.Context, a, b string) (string, error) {
	ka := s.resolveProjectKey(ctx, ProjectKey(a))
	kb := s.resolveProjectKey(ctx, ProjectKey(b))
	if ka == "" || kb == "" || ka == kb {
		return "", errors.New("se requieren dos proyectos distintos para ignorar el par")
	}
	pair := canonicalPairKey(ka, kb)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO workspace_audit_ignores (pair_key, created_at) VALUES (?, ?)
		ON CONFLICT(pair_key) DO NOTHING`, pair, now); err != nil {
		return "", fmt.Errorf("guardando par ignorado: %w", err)
	}
	return pair, nil
}

// ListIgnoredWorkspacePairs devuelve los pares marcados como "mantener separados".
func (s *Store) ListIgnoredWorkspacePairs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pair_key FROM workspace_audit_ignores ORDER BY pair_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AuditWorkspaces inspecciona todos los proyectos vigentes (determinista, 0 LLM) y
// clasifica candidatos en sub-equipos (<base>-<área>) y posibles duplicados
// (similitud de nombre, variantes sin guion o misma ruta física en rootsByProject).
func (s *Store) AuditWorkspaces(ctx context.Context, rootsByProject map[string]string) (*WorkspaceAuditResult, error) {
	projects, err := s.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	aliases, err := s.ListProjectAliases(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := s.ListWorkspaceGroups(ctx)
	if err != nil {
		return nil, err
	}
	ignoredList, err := s.ListIgnoredWorkspacePairs(ctx)
	if err != nil {
		return nil, err
	}

	ignored := make(map[string]bool, len(ignoredList))
	for _, p := range ignoredList {
		ignored[p] = true
	}
	grouped := make(map[string]string, len(groups))
	for _, g := range groups {
		grouped[g.ProjectKey] = g.ParentKey
	}
	byKey := make(map[string]ProjectSummary, len(projects))
	for _, p := range projects {
		byKey[p.Key] = p
	}

	// 1. Detectar sub-equipos con patrón <base>-<sufijo> donde <base> existe
	//    o hay >= 2 hermanos compartiendo el mismo prefijo <base>-.
	prefixCounts := map[string]int{}
	for _, p := range projects {
		if idx := strings.IndexByte(p.Key, '-'); idx > 1 {
			prefixCounts[p.Key[:idx]]++
		}
	}

	subteamKeys := map[string]bool{}
	var subteams []SubteamCandidate
	for _, p := range projects {
		if _, already := grouped[p.Key]; already {
			subteamKeys[p.Key] = true
			continue
		}
		idx := strings.IndexByte(p.Key, '-')
		if idx <= 1 || idx >= len(p.Key)-1 {
			continue
		}
		base := p.Key[:idx]
		rest := p.Key[idx+1:]
		// Evita clasificar variantes sin guion (p. ej. genius-shell vs geniusshell) como sub-equipos.
		if _, hasNoDash := byKey[base+strings.ReplaceAll(rest, "-", "")]; hasNoDash {
			continue
		}
		parentProj, hasBase := byKey[base]
		if !hasBase && prefixCounts[base] < 2 {
			continue
		}
		// Solo si el sufijo parece un sub-equipo/área (no un slug gigante de handoff de 3+ guiones).
		if strings.Count(rest, "-") >= 2 && !isKnownSubteamSuffix(rest) {
			continue
		}
		if ignored[canonicalPairKey(p.Key, base)] {
			continue
		}
		parentName := base
		reason := fmt.Sprintf("comparte prefijo «%s-» con %d proyectos", base, prefixCounts[base])
		if hasBase {
			parentName = parentProj.Name
			reason = fmt.Sprintf("prefijo «%s-» del workspace existente «%s»", base, parentProj.Name)
		}
		subteams = append(subteams, SubteamCandidate{
			Project:    p.Key,
			Name:       p.Name,
			Reports:    p.Reports,
			Parent:     base,
			ParentName: parentName,
			Label:      rest,
			Reason:     reason,
		})
		subteamKeys[p.Key] = true
	}
	sort.Slice(subteams, func(i, j int) bool {
		if subteams[i].Parent != subteams[j].Parent {
			return subteams[i].Parent < subteams[j].Parent
		}
		return subteams[i].Project < subteams[j].Project
	})

	// 2. Detectar posibles duplicados entre proyectos no marcados como sub-equipo hermano.
	cleanRoot := func(k string) string {
		if rootsByProject == nil {
			return ""
		}
		r := strings.TrimSpace(rootsByProject[k])
		if r == "" {
			return ""
		}
		return strings.ToLower(filepath.Clean(r))
	}

	var duplicates []DuplicateCandidate
	for i := 0; i < len(projects); i++ {
		for j := i + 1; j < len(projects); j++ {
			a, b := projects[i], projects[j]
			if ignored[canonicalPairKey(a.Key, b.Key)] {
				continue
			}
			// No sugerir como duplicado un par padre↔sub-equipo ni dos sub-equipos del mismo padre.
			if isSubteamRelation(a.Key, b.Key, grouped, subteams) {
				continue
			}

			score, reason := compareProjectSimilarity(a.Key, b.Key, cleanRoot(a.Key), cleanRoot(b.Key))
			if score < 0.58 {
				continue
			}
			suggestion := pickCanonicalSuggestion(a, b)
			duplicates = append(duplicates, DuplicateCandidate{
				A:          a.Key,
				AName:      a.Name,
				AReports:   a.Reports,
				B:          b.Key,
				BName:      b.Name,
				BReports:   b.Reports,
				Score:      float64(int(score*100+0.5)) / 100,
				Reason:     reason,
				Suggestion: suggestion,
			})
		}
	}
	sort.Slice(duplicates, func(i, j int) bool {
		if duplicates[i].Score != duplicates[j].Score {
			return duplicates[i].Score > duplicates[j].Score
		}
		return duplicates[i].A < duplicates[j].A
	})

	if duplicates == nil {
		duplicates = []DuplicateCandidate{}
	}
	if subteams == nil {
		subteams = []SubteamCandidate{}
	}

	return &WorkspaceAuditResult{
		Projects:   len(projects),
		Duplicates: duplicates,
		Subteams:   subteams,
		Aliases:    aliases,
		Groups:     groups,
		Ignored:    ignoredList,
	}, nil
}

func deriveSubteamLabel(projectKey, parentKey string) string {
	prefix := parentKey + "-"
	if strings.HasPrefix(projectKey, prefix) && len(projectKey) > len(prefix) {
		return projectKey[len(prefix):]
	}
	return projectKey
}

func isKnownSubteamSuffix(s string) bool {
	switch s {
	case "backend", "back", "api", "frontend", "front", "web", "mobile", "app", "ios", "android",
		"db", "database", "infra", "devops", "qa", "test", "ux", "ui", "design", "producto", "product", "docs", "cli", "core":
		return true
	}
	return false
}

func isSubteamRelation(aKey, bKey string, grouped map[string]string, subteams []SubteamCandidate) bool {
	parentOf := func(k string) string {
		if p, ok := grouped[k]; ok {
			return p
		}
		for _, s := range subteams {
			if s.Project == k {
				return s.Parent
			}
		}
		return ""
	}
	pa, pb := parentOf(aKey), parentOf(bKey)
	if pa != "" && (pa == bKey || pa == pb) {
		return true
	}
	if pb != "" && pb == aKey {
		return true
	}
	return false
}

func stripDashes(s string) string {
	return strings.ReplaceAll(s, "-", "")
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			cur[j] = m
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func compareProjectSimilarity(aKey, bKey, aRoot, bRoot string) (float64, string) {
	if aRoot != "" && bRoot != "" && aRoot == bRoot {
		return 1.0, "apuntan a la misma raíz de código en roots.json"
	}

	af, bf := stripDashes(aKey), stripDashes(bKey)
	if af == bf {
		return 0.98, "mismo nombre con diferente separación (guiones/espacios)"
	}

	// Uno contiene exactamente al otro (p. ej. dioptra ⊂ dioptraengine, geniusshell ⊂ dioptraengine-geniusshell-v1-handoff).
	shorter, longer := af, bf
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len(shorter) >= 5 && strings.Contains(longer, shorter) {
		if strings.HasPrefix(longer, shorter) {
			return 0.86, fmt.Sprintf("«%s» es prefijo directo de «%s»", aKey, bKey)
		}
		return 0.78, fmt.Sprintf("«%s» aparece contenido dentro de «%s»", shorter, longer)
	}

	// Similitud por Levenshtein + prefijo común (captura SnapProof vs snapshooter).
	maxLen := len(af)
	if len(bf) > maxLen {
		maxLen = len(bf)
	}
	dist := levenshtein(af, bf)
	levSim := 1.0 - float64(dist)/float64(maxLen)
	pref := commonPrefixLen(af, bf)
	if pref >= 4 {
		prefBonus := float64(pref) / float64(maxLen) * 0.22
		combined := levSim + prefBonus
		if combined > 0.95 {
			combined = 0.95
		}
		if combined >= 0.58 {
			return combined, fmt.Sprintf("prefijo común «%s» y alta similitud ortográfica", af[:pref])
		}
	}
	if levSim >= 0.65 {
		return levSim, "alta similitud ortográfica entre nombres"
	}
	return 0, ""
}

func pickCanonicalSuggestion(a, b ProjectSummary) string {
	// Si uno es mucho más corto y ya tiene reports (p. ej. dioptra vs dioptraengine), prefiere el más corto/limpio o el de más reportes.
	if a.Reports != b.Reports {
		if a.Reports > b.Reports {
			return a.Key
		}
		return b.Key
	}
	if len(a.Key) <= len(b.Key) {
		return a.Key
	}
	return b.Key
}
