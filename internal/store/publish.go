package store

// Publicación de reportes: identidad, hash, delta entre revisiones, layout, costo y persistencia.
// Movido ÍNTEGRO desde store.go (F7: partir archivos gigantes).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"dioptra/internal/contract"
	"dioptra/internal/cost"
	"dioptra/internal/layoutagent"
	"dioptra/internal/render"
)

// PublishParams define los parámetros de entrada para publicar o actualizar un reporte.
type PublishParams struct {
	ID              string
	IdempotencyKey  string
	ChangeNote      string
	ForceNew        bool
	Title           string
	Project         string
	Tool            string
	Model           string
	Markdown        string
	InteractiveMode bool
	BlocksOverride  []any
	LayoutCfg       layoutagent.Config
	Now             time.Time
}

// PublishResult contiene la salida persistida lista para responder por MCP o HTTP.
type PublishResult struct {
	ID           string                 `json:"id"`
	Action       string                 `json:"action"` // "created" | "updated" | "unchanged"
	Revision     int                    `json:"revision"`
	ChangeNote   string                 `json:"changeNote,omitempty"`
	Summary      contract.ReportSummary `json:"summary"`
	Envelope     *contract.Envelope     `json:"envelope"`
	Warnings     []contract.Warning     `json:"warnings"`
	MarkdownPath string                 `json:"markdownPath"`
	ModeUsed     string                 `json:"modeUsed"` // "fast_goldmark" | "blocks_override" | "layout_agent"
	Layout       *contract.LayoutMeta   `json:"layout,omitempty"`
	Fidelity     *contract.FidelityMeta `json:"fidelity,omitempty"`
	Cost         *contract.CostReport   `json:"cost,omitempty"`
}

// findExistingReportLocked resuelve un reporte previo por id, idempotency_key o (project_key + slug).
// La identidad por defecto es proyecto+título; la herramienta (tool) NO participa en la clave.
func (s *Store) findExistingReportLocked(ctx context.Context, id, idempotencyKey, projectKey, baseSlug string) (*existingReportMeta, bool) {
	queryRow := func(where string, args ...any) (*existingReportMeta, bool) {
		var m existingReportMeta
		err := s.db.QueryRowContext(ctx, `
			SELECT id, slug, md_rel_path, ts, updated_at, revision, content_hash, last_change,
			       idempotency_key, envelope_json, warnings_count, title, project, max_sev, excerpt, tool, model
			FROM reports `+where, args...).Scan(
			&m.ID, &m.Slug, &m.RelPath, &m.CreatedTS, &m.UpdatedAt, &m.Revision, &m.ContentHash,
			&m.LastChange, &m.IdempotencyKey, &m.EnvelopeJSON, &m.WarningsCount,
			&m.Title, &m.Project, &m.MaxSev, &m.Excerpt, &m.Tool, &m.Model,
		)
		if err != nil {
			return nil, false
		}
		if m.Revision < 1 {
			m.Revision = 1
		}
		return &m, true
	}

	if id != "" {
		if m, ok := queryRow(`WHERE id = ? LIMIT 1`, id); ok {
			return m, true
		}
	}
	if idempotencyKey != "" {
		if m, ok := queryRow(`WHERE idempotency_key = ? ORDER BY updated_at DESC LIMIT 1`, idempotencyKey); ok {
			return m, true
		}
	}
	if projectKey != "" && baseSlug != "" {
		// La identidad cubre la clave canónica y todos sus alias (filas históricas sin reescribir).
		group := s.projectGroupKeys(ctx, projectKey)
		args := []any{baseSlug}
		marks := make([]string, len(group))
		for i, k := range group {
			marks[i] = "?"
			args = append(args, k)
		}
		if m, ok := queryRow(`WHERE slug = ? AND project_key IN (`+strings.Join(marks, ",")+`) ORDER BY updated_at DESC LIMIT 1`, args...); ok {
			return m, true
		}
	}
	return nil, false
}

func computeMarkdownHash(md string) string {
	sum := sha256.Sum256([]byte(strings.ReplaceAll(md, "\r\n", "\n")))
	return hex.EncodeToString(sum[:])
}

func computePublishHash(title, project, md string, blocksOverride []any, interactiveMode bool) string {
	h := sha256.New()
	_, _ = h.Write([]byte("v1\n"))
	_, _ = h.Write([]byte(title))
	_, _ = h.Write([]byte("\x00"))
	_, _ = h.Write([]byte(project))
	_, _ = h.Write([]byte("\x00"))
	_, _ = h.Write([]byte(strings.ReplaceAll(md, "\r\n", "\n")))
	_, _ = h.Write([]byte("\x00"))
	if len(blocksOverride) > 0 {
		if raw, err := json.Marshal(blocksOverride); err == nil {
			_, _ = h.Write([]byte("blocks:"))
			_, _ = h.Write(raw)
		}
	} else if interactiveMode {
		_, _ = h.Write([]byte("interactive:1"))
	} else {
		_, _ = h.Write([]byte("fast:1"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func truncateRunes(s string, max int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

// summarizeMarkdownDelta genera un resumen conciso de qué cambió entre dos versiones
// del mismo documento cuando el cliente no envió `change_note` explícita.
func summarizeMarkdownDelta(prevMD, newMD, modeUsed string) string {
	normPrev := strings.ReplaceAll(strings.TrimSpace(prevMD), "\r\n", "\n")
	normNew := strings.ReplaceAll(strings.TrimSpace(newMD), "\r\n", "\n")
	if normPrev == normNew {
		if modeUsed == "blocks_override" || modeUsed == "layout_agent" {
			return fmt.Sprintf("Actualización visual (%s) sin cambios en texto", modeUsed)
		}
		return "Reindexado del reporte sin cambios en Markdown"
	}

	prevLines := strings.Split(normPrev, "\n")
	newLines := strings.Split(normNew, "\n")
	prevCounts := make(map[string]int, len(prevLines))
	for _, l := range prevLines {
		if t := strings.TrimSpace(l); t != "" {
			prevCounts[t]++
		}
	}
	added := 0
	for _, l := range newLines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if prevCounts[t] > 0 {
			prevCounts[t]--
		} else {
			added++
		}
	}
	removed := 0
	for _, rem := range prevCounts {
		removed += rem
	}

	// Detectar encabezados nuevos o secciones cuyo contenido cambió
	extractSections := func(lines []string) ([]string, map[string]string) {
		var order []string
		bodyBySec := make(map[string]string)
		current := ""
		var buf strings.Builder
		flush := func() {
			if current != "" {
				bodyBySec[current] = buf.String()
			}
			buf.Reset()
		}
		for _, l := range lines {
			t := strings.TrimSpace(l)
			if strings.HasPrefix(t, "#") {
				heading := strings.TrimSpace(strings.TrimLeft(t, "#"))
				if heading != "" {
					flush()
					current = heading
					if _, exists := bodyBySec[current]; !exists {
						order = append(order, current)
					}
					continue
				}
			}
			buf.WriteString(t)
			buf.WriteByte('\n')
		}
		flush()
		return order, bodyBySec
	}

	_, prevSecs := extractSections(prevLines)
	newOrder, newSecs := extractSections(newLines)
	var touched []string
	for _, sec := range newOrder {
		if prevBody, existed := prevSecs[sec]; !existed || prevBody != newSecs[sec] {
			touched = append(touched, truncateRunes(sec, 36))
			if len(touched) == 2 {
				break
			}
		}
	}

	var deltaPart string
	switch {
	case added > 0 && removed > 0:
		deltaPart = fmt.Sprintf("+%d / -%d líneas", added, removed)
	case added > 0:
		deltaPart = fmt.Sprintf("+%d líneas", added)
	case removed > 0:
		deltaPart = fmt.Sprintf("-%d líneas", removed)
	default:
		deltaPart = "Ajuste de formato/espaciado"
	}

	if len(touched) > 0 {
		return truncateRunes(fmt.Sprintf("%s en %s", deltaPart, strings.Join(touched, ", ")), 160)
	}
	return deltaPart
}

// Publish guarda SIEMPRE primero el .md limpio en {baseDir}/{YYYY-MM}/{slug}.md,
// aplica idempotencia (actualiza in-place si coincide id, idempotency_key o project+slug),
// genera la vista JSON por ruta rápida (Goldmark) o Agente Maquetador / blocks_override,
// extrae el HTML de cualquier free-canvas hacia /canvas/{docId}/{blockId} e indexa en SQLite.
//
// Concurrencia: la síntesis con el Agente Maquetador (interactive_mode) es una llamada
// HTTP a un LLM que puede tardar hasta 25 s. Se ejecuta FUERA de s.mu (presynthesizeLayout)
// para no congelar otras publicaciones ni los PATCH de casillas; luego se toma el candado
// solo para persistir.
func (s *Store) Publish(ctx context.Context, p PublishParams) (*PublishResult, error) {
	if p.Now.IsZero() {
		p.Now = time.Now()
	}
	pre := s.presynthesizeLayout(ctx, p)

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publishLocked(ctx, p, pre)
}

// layoutPresynth es el resultado de layoutagent.Synthesize calculado sin candado.
type layoutPresynth struct {
	res *layoutagent.Result
	err error
}

// resolvePublishIdentity calcula título y proyecto (sin canonizar) a partir de los
// parámetros y el frontmatter. Es pura: no toca la base.
func resolvePublishIdentity(p PublishParams) (title, project string) {
	fm := render.ParseFrontmatter(p.Markdown)
	title = strings.TrimSpace(p.Title)
	if title == "" {
		title = fm.Title
	}
	if title == "" {
		title = inferFirstHeading(fm.Body)
	}
	if title == "" {
		title = "Reporte Técnico"
	}
	project = strings.TrimSpace(p.Project)
	if project == "" {
		project = fm.Project
	}
	if project == "" {
		project = "dioptra"
	}
	return title, project
}

// daemonLayoutConfig devuelve la configuración del adaptador HTTP del demonio
// (jamás lanza la CLI local).
func daemonLayoutConfig(p PublishParams) layoutagent.Config {
	cfg := p.LayoutCfg.ForDaemon()
	if !cfg.IsConfigured() {
		cfg = layoutagent.ConfigFromEnv().ForDaemon()
	}
	return cfg
}

// presynthesizeLayout ejecuta la síntesis del LLM sin mantener s.mu. Devuelve nil
// cuando no aplica (sin interactive_mode, con blocks_override, adaptador no
// configurado o contenido idéntico al ya publicado); en ese caso publishLocked
// sigue su camino habitual.
func (s *Store) presynthesizeLayout(ctx context.Context, p PublishParams) *layoutPresynth {
	if !p.InteractiveMode || len(p.BlocksOverride) > 0 {
		return nil
	}
	cfg := daemonLayoutConfig(p)
	if !cfg.IsConfigured() {
		return nil
	}
	title, project := resolvePublishIdentity(p)
	mdContent := p.Markdown
	if strings.TrimSpace(mdContent) == "" {
		mdContent = fmt.Sprintf("# %s\n", title)
	}

	// Lectura corta bajo candado: proyecto canónico e idempotencia, para no gastar
	// una llamada al LLM si el contenido no cambió.
	s.mu.Lock()
	project, projectKey := s.canonicalProjectLocked(ctx, project)
	unchanged := false
	if !p.ForceNew {
		hash := computePublishHash(title, project, mdContent, p.BlocksOverride, p.InteractiveMode)
		if ex, ok := s.findExistingReportLocked(ctx, strings.TrimSpace(p.ID), strings.TrimSpace(p.IdempotencyKey), projectKey, slugify(title)); ok {
			unchanged = ex.ContentHash != "" && ex.ContentHash == hash
		}
	}
	s.mu.Unlock()
	if unchanged {
		return nil
	}

	res, err := layoutagent.Synthesize(ctx, cfg, layoutagent.Input{
		Title: title, Project: project, TS: p.Now.Format(time.RFC3339), Markdown: mdContent,
	})
	return &layoutPresynth{res: res, err: err}
}

// publishLocked es el cuerpo de Publish; requiere s.mu tomado. Si pre != nil usa
// esa síntesis en lugar de llamar al LLM bajo candado.
func (s *Store) publishLocked(ctx context.Context, p PublishParams, pre *layoutPresynth) (*PublishResult, error) {
	now := p.Now
	if now.IsZero() {
		now = time.Now()
	}
	tsISO := now.Format(time.RFC3339)

	title, project := resolvePublishIdentity(p)
	// Anti-duplicados: si ya existe el mismo proyecto bajo otra grafía, se reutiliza el display original.
	project, projectKey := s.canonicalProjectLocked(ctx, project)
	tool := strings.TrimSpace(p.Tool)
	model := strings.TrimSpace(p.Model)

	mdContent := p.Markdown
	if strings.TrimSpace(mdContent) == "" {
		mdContent = fmt.Sprintf("# %s\n", title)
	}

	baseSlug := slugify(title)
	effKey := strings.TrimSpace(p.IdempotencyKey)
	contentHash := computePublishHash(title, project, mdContent, p.BlocksOverride, p.InteractiveMode)

	var existing *existingReportMeta
	if !p.ForceNew {
		if found, ok := s.findExistingReportLocked(ctx, strings.TrimSpace(p.ID), effKey, projectKey, baseSlug); ok {
			existing = found
		}
	}

	// Idempotencia pura: si el contenido y la vista coinciden exactamente con el último estado persistido,
	// no reescribimos disco ni incrementamos revisión ni duplicamos entradas.
	if existing != nil && existing.ContentHash != "" && existing.ContentHash == contentHash {
		var cachedEnv contract.Envelope
		if err := json.Unmarshal([]byte(existing.EnvelopeJSON), &cachedEnv); err == nil && cachedEnv.Report.Title != "" {
			absExisting := filepath.Join(s.baseDir, filepath.FromSlash(existing.RelPath))
			var blocksRaw []byte
			unchangedMode := "fast_goldmark"
			if len(p.BlocksOverride) > 0 {
				unchangedMode = "blocks_override"
				blocksRaw, _ = json.Marshal(p.BlocksOverride)
			}
			costReport := cost.Compute(cost.ComputeParams{
				Mode:              unchangedMode,
				Model:             p.Model,
				Markdown:          mdContent,
				BlocksOverrideRaw: blocksRaw,
			})
			sum := contract.ReportSummary{
				ID:            existing.ID,
				Slug:          existing.Slug,
				Project:       existing.Project,
				Tool:          existing.Tool,
				Model:         existing.Model,
				Title:         existing.Title,
				TS:            existing.CreatedTS,
				Revision:      existing.Revision,
				MaxSev:        existing.MaxSev,
				Excerpt:       existing.Excerpt,
				MarkdownPath:  absExisting,
				WarningsCount: existing.WarningsCount,
			}
			if existing.Revision > 1 {
				sum.UpdatedAt = existing.UpdatedAt
				sum.LastChange = existing.LastChange
			}
			warns := cachedEnv.Warnings
			if warns == nil {
				warns = []contract.Warning{}
			}
			return &PublishResult{
				ID:           existing.ID,
				Action:       "unchanged",
				Revision:     existing.Revision,
				ChangeNote:   existing.LastChange,
				Summary:      sum,
				Envelope:     &cachedEnv,
				Warnings:     warns,
				MarkdownPath: absExisting,
				ModeUsed:     unchangedMode,
				Cost:         &costReport,
			}, nil
		}
	}

	// 1. Resolver archivo fuente .md destino (reutilizar in-place si es actualización)
	var (
		docID     string
		slug      string
		relSlash  string
		absMDPath string
		prevMD    string
		createdTS = tsISO
		revision  = 1
		updatedAt string
		action    = "created"
	)

	if existing != nil {
		action = "updated"
		docID = existing.ID
		slug = existing.Slug
		relSlash = existing.RelPath
		absMDPath = filepath.Join(s.baseDir, filepath.FromSlash(relSlash))
		if existing.CreatedTS != "" {
			createdTS = existing.CreatedTS
		}
		revision = existing.Revision + 1
		updatedAt = tsISO
		if effKey == "" {
			effKey = existing.IdempotencyKey
		}
		if prevBytes, err := os.ReadFile(absMDPath); err == nil {
			prevMD = string(prevBytes)
		}
		if err := os.MkdirAll(filepath.Dir(absMDPath), 0o755); err != nil {
			return nil, fmt.Errorf("creando carpeta del reporte existente: %w", err)
		}
	} else {
		// HU-7: {base}/{project_key}/YYYY-MM/. Sin clave válida se cae a la raíz mensual legacy.
		monthFolder := now.Format("2006-01")
		if projectKey != "" {
			monthFolder = filepath.Join(projectKey, monthFolder)
		}
		monthDir := filepath.Join(s.baseDir, monthFolder)
		if err := os.MkdirAll(monthDir, 0o755); err != nil {
			return nil, fmt.Errorf("creando carpeta mensual %s: %w", monthDir, err)
		}

		slug = baseSlug
		absMDPath = filepath.Join(monthDir, slug+".md")
		if strings.TrimSpace(p.ID) == "" {
			// Evitar sobrescribir un archivo de otro proyecto distinto o cuando force_new=true
			suffix := 2
			for {
				if _, err := os.Stat(absMDPath); os.IsNotExist(err) {
					break
				}
				slug = fmt.Sprintf("%s-%d", baseSlug, suffix)
				absMDPath = filepath.Join(monthDir, slug+".md")
				suffix++
			}
		}

		relPath, err := filepath.Rel(s.baseDir, absMDPath)
		if err != nil {
			relPath = filepath.Join(monthFolder, slug+".md")
		}
		relSlash = filepath.ToSlash(relPath)

		docID = strings.TrimSpace(p.ID)
		if docID == "" {
			docID = makeDocID(relSlash)
		}
	}

	if err := writeFileAtomic(absMDPath, []byte(mdContent)); err != nil {
		return nil, fmt.Errorf("escribiendo archivo fuente .md: %w", err)
	}

	// 2. Construir sobre JSON según modo solicitado con tolerancia ADR 1
	var (
		env          *contract.Envelope
		warnings     []contract.Warning
		modeUsed     = "fast_goldmark"
		layoutMeta   *contract.LayoutMeta
		fidelityMeta *contract.FidelityMeta
	)

	if len(p.BlocksOverride) > 0 {
		rootMap := map[string]any{
			"v": 1,
			"report": map[string]any{
				"id":      docID,
				"project": project,
				"title":   title,
				"ts":      tsISO,
				"blocks":  p.BlocksOverride,
			},
		}
		candEnv, candWarns, err := contract.ValidateAndNormalizeMap(rootMap)
		if err == nil {
			// Guardián de Fidelidad SIEMPRE activo sobre blocks_override de agentes externos
			guardRep, fidMeta, guardErr := layoutagent.GuardExternalEnvelope(ctx, candEnv, layoutagent.Input{
				Title: title, Project: project, TS: tsISO, Markdown: mdContent,
			})
			fidelityMeta = &fidMeta
			if guardErr != nil {
				warnings = append(warnings, candWarns...)
				warnings = append(warnings, contract.Warning{
					Path:   "blocks_override",
					Action: "degraded",
					Reason: fmt.Sprintf("Guardián de Fidelidad rechazó blocks_override (%v); publicado por ruta rápida goldmark", guardErr),
				})
			} else {
				candEnv.Report.ID = docID
				render.EnrichBlocks(ctx, candEnv.Report.Blocks)
				env = candEnv
				warnings = append(warnings, candWarns...)
				warnings = append(warnings, guardRep.Warnings...)
				modeUsed = "blocks_override"
				// Si el .md no tenía casillas pero blocks_override trae checklist, sincronizamos sección de tareas al .md
				mdContent = ensureChecklistInMarkdown(absMDPath, mdContent, env.Report.Blocks)
			}
		} else {
			warnings = append(warnings, contract.Warning{
				Path:   "blocks_override",
				Action: "degraded",
				Reason: fmt.Sprintf("blocks_override inválido (%v); degradado a ruta rápida goldmark", err),
			})
		}
	} else if p.InteractiveMode {
		// El demonio solo usa el adaptador HTTP secundario; jamás lanza la CLI local.
		cfg := daemonLayoutConfig(p)
		if cfg.IsConfigured() {
			var (
				res *layoutagent.Result
				err error
			)
			if pre != nil {
				// Ruta normal: síntesis ya calculada fuera del candado.
				res, err = pre.res, pre.err
			} else {
				// Respaldo poco frecuente (p. ej. el contenido cambió entre la comprobación
				// de idempotencia y la publicación): síntesis bajo candado como antes.
				res, err = layoutagent.Synthesize(ctx, cfg, layoutagent.Input{
					Title: title, Project: project, TS: tsISO, Markdown: mdContent,
				})
			}
			if res != nil && res.Meta.Provider != "" {
				meta := res.Meta
				layoutMeta = &meta
			}
			if err == nil {
				candEnv := res.Envelope
				candEnv.Report.ID = docID
				candEnv.Report.Project = project
				candEnv.Report.Title = title
				candEnv.Report.TS = tsISO
				render.EnrichBlocks(ctx, candEnv.Report.Blocks)
				env = candEnv
				warnings = append(warnings, res.Warnings...)
				modeUsed = "layout_agent"
				mdContent = ensureChecklistInMarkdown(absMDPath, mdContent, env.Report.Blocks)
			} else {
				warnings = append(warnings, contract.Warning{
					Path:   "interactive_mode",
					Action: "degraded",
					Reason: fmt.Sprintf("Agente Único falló (%v); degradado automáticamente a goldmark", err),
				})
			}
		} else {
			reason := "interactive_mode no usa credenciales en el flujo MCP: llama get_layout_contract y envía blocks_override junto con el markdown; publicado por ruta rápida goldmark"
			if cfg.HasHTTPSettings() {
				_, resolveErr := cfg.Resolve()
				reason = fmt.Sprintf("adaptador HTTP opcional mal configurado (%v); publicado por ruta rápida goldmark", resolveErr)
			}
			warnings = append(warnings, contract.Warning{
				Path:   "interactive_mode",
				Action: "degraded",
				Reason: reason,
			})
		}
	}

	if env == nil {
		fastEnv, fastWarns, err := render.ConvertMarkdownToEnvelope(ctx, title, project, createdTS, mdContent)
		if err != nil {
			return nil, fmt.Errorf("convirtiendo markdown por ruta rápida: %w", err)
		}
		fastEnv.Report.ID = docID
		env = fastEnv
		warnings = append(warnings, fastWarns...)
	}

	changeNote := ""
	if action == "updated" {
		changeNote = truncateRunes(p.ChangeNote, 180)
		if changeNote == "" {
			changeNote = summarizeMarkdownDelta(prevMD, mdContent, modeUsed)
		}
	}

	env.Report.TS = createdTS
	env.Report.Revision = revision
	if revision > 1 {
		env.Report.UpdatedAt = updatedAt
		env.Report.LastChange = changeNote
	} else {
		env.Report.UpdatedAt = ""
		env.Report.LastChange = ""
	}
	env.Warnings = warnings

	res, err := s.persistEnvelopeLocked(ctx, docID, slug, relSlash, mdContent, env, warnings, modeUsed, persistMeta{
		Tool:           tool,
		Model:          model,
		IdempotencyKey: effKey,
		ContentHash:    contentHash,
		Revision:       revision,
		UpdatedAt:      updatedAt,
		SQLUpdatedAt:   tsISO,
		LastChange:     changeNote,
		Action:         action,
	})
	if err != nil {
		return nil, err
	}
	res.Layout = layoutMeta
	res.Fidelity = fidelityMeta

	var blocksOverrideRaw []byte
	if len(p.BlocksOverride) > 0 && modeUsed == "blocks_override" {
		blocksOverrideRaw, _ = json.Marshal(p.BlocksOverride)
	}
	costReport := cost.Compute(cost.ComputeParams{
		Mode:              modeUsed,
		Model:             p.Model,
		Markdown:          mdContent,
		BlocksOverrideRaw: blocksOverrideRaw,
		LayoutMeta:        layoutMeta,
	})
	res.Cost = &costReport
	s.recordCostMetricLocked(ctx, docID, costReport, now.Format(time.RFC3339))

	return res, nil
}

func (s *Store) recordCostMetricLocked(ctx context.Context, reportID string, c contract.CostReport, ts string) {
	exactInt := 0
	if c.IsExact {
		exactInt = 1
	}
	// Una métrica fallida nunca debe romper la publicación, pero tampoco pasar en silencio.
	// Se registra SIEMPRE en stderr: stdout lo usa el transporte MCP para JSON-RPC.
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO publish_metrics (
			report_id, mode, model, is_exact,
			baseline_out_tokens, dioptra_in_tokens, dioptra_out_tokens, extra_out_tokens, delta_pct,
			baseline_usd, dioptra_usd, delta_usd, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		reportID, c.Mode, c.Model, exactInt,
		c.BaselineOutputTokens, c.DioptraInputTokens, c.DioptraOutputTokens, c.ExtraOutputTokens, c.DeltaPct,
		c.BaselineUSD, c.DioptraUSD, c.DeltaUSD, ts,
	); err != nil {
		fmt.Fprintf(os.Stderr, "dioptra: no se pudo registrar la métrica de costo (report=%s modo=%s modelo=%s): %v\n",
			reportID, c.Mode, c.Model, err)
	}
}

// CostModeStats agrupa el impacto acumulado en tokens y USD por modo de renderizado.
type CostModeStats struct {
	Mode              string  `json:"mode"`
	Reports           int     `json:"reports"`
	BaselineOutTokens int     `json:"baselineOutTokens"`
	DioptraInTokens   int     `json:"dioptraInTokens"`
	DioptraOutTokens  int     `json:"dioptraOutTokens"`
	ExtraOutTokens    int     `json:"extraOutTokens"`
	AvgDeltaPct       float64 `json:"avgDeltaPct"`
	BaselineUSD       float64 `json:"baselineUsd"`
	DioptraUSD        float64 `json:"dioptraUsd"`
	DeltaUSD          float64 `json:"deltaUsd"`
}

// CostSummary contiene el acumulado histórico de mediciones para `dioptra stats`.
type CostSummary struct {
	TotalReports      int             `json:"totalReports"`
	BaselineOutTokens int             `json:"baselineOutTokens"`
	DioptraInTokens   int             `json:"dioptraInTokens"`
	DioptraOutTokens  int             `json:"dioptraOutTokens"`
	ExtraOutTokens    int             `json:"extraOutTokens"`
	BaselineUSD       float64         `json:"baselineUsd"`
	DioptraUSD        float64         `json:"dioptraUsd"`
	DeltaUSD          float64         `json:"deltaUsd"`
	ByMode            []CostModeStats `json:"byMode"`
}

// CostSummary devuelve el acumulado histórico registrado en `publish_metrics`.
func (s *Store) CostSummary(ctx context.Context) (*CostSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			mode,
			COUNT(*),
			COALESCE(SUM(baseline_out_tokens), 0),
			COALESCE(SUM(dioptra_in_tokens), 0),
			COALESCE(SUM(dioptra_out_tokens), 0),
			COALESCE(SUM(extra_out_tokens), 0),
			COALESCE(AVG(delta_pct), 0),
			COALESCE(SUM(baseline_usd), 0),
			COALESCE(SUM(dioptra_usd), 0),
			COALESCE(SUM(delta_usd), 0)
		FROM publish_metrics
		GROUP BY mode
		ORDER BY COUNT(*) DESC, mode ASC`)
	if err != nil {
		return nil, fmt.Errorf("consultando publish_metrics: %w", err)
	}
	defer rows.Close()

	summary := &CostSummary{ByMode: []CostModeStats{}}
	for rows.Next() {
		var m CostModeStats
		if err := rows.Scan(
			&m.Mode, &m.Reports, &m.BaselineOutTokens, &m.DioptraInTokens, &m.DioptraOutTokens,
			&m.ExtraOutTokens, &m.AvgDeltaPct, &m.BaselineUSD, &m.DioptraUSD, &m.DeltaUSD,
		); err != nil {
			return nil, err
		}
		summary.ByMode = append(summary.ByMode, m)
		summary.TotalReports += m.Reports
		summary.BaselineOutTokens += m.BaselineOutTokens
		summary.DioptraInTokens += m.DioptraInTokens
		summary.DioptraOutTokens += m.DioptraOutTokens
		summary.ExtraOutTokens += m.ExtraOutTokens
		summary.BaselineUSD += m.BaselineUSD
		summary.DioptraUSD += m.DioptraUSD
		summary.DeltaUSD += m.DeltaUSD
	}
	return summary, rows.Err()
}

func (s *Store) persistEnvelopeLocked(
	ctx context.Context,
	docID, slug, relSlash, mdContent string,
	env *contract.Envelope,
	warnings []contract.Warning,
	modeUsed string,
	meta persistMeta,
) (*PublishResult, error) {
	// Extraer bloques free-canvas y reemplazar `html` por `src: "/canvas/{docId}/{blockId}"` (§4.9)
	canvasMap := make(map[string]string)
	canvasCounter := 0
	extractCanvasHTML(docID, env.Report.Blocks, canvasMap, &canvasCounter)

	if meta.Revision < 1 {
		meta.Revision = 1
	}
	if meta.SQLUpdatedAt == "" {
		meta.SQLUpdatedAt = time.Now().Format(time.RFC3339)
	}
	if meta.Action == "" {
		meta.Action = "created"
	}

	env.Report.ID = docID
	env.Report.Revision = meta.Revision
	if meta.Revision > 1 {
		env.Report.UpdatedAt = meta.UpdatedAt
		env.Report.LastChange = meta.LastChange
	}
	env.Report.MaxSev = contract.ExtractMaxSeverity(env.Report.Blocks)
	env.Report.Excerpt = contract.ExtractExcerpt(env.Report.Blocks, env.Report.Title)
	env.Warnings = warnings

	checks := contract.CollectChecklists(env.Report.Blocks)

	envBytes, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	checksBytes, err := json.Marshal(checks)
	if err != nil {
		return nil, err
	}

	// Caché descartable en disco (.cache/{id}.json)
	cacheFile := filepath.Join(s.baseDir, ".cache", docID+".json")
	_ = os.WriteFile(cacheFile, envBytes, 0o644)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	mdHash := computeMarkdownHash(mdContent)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO reports (
			id, slug, project, title, ts, max_sev, excerpt, md_rel_path,
			envelope_json, checks_json, warnings_count, updated_at,
			idempotency_key, content_hash, md_hash, revision, last_change,
			tool, model, project_key
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			slug            = excluded.slug,
			project         = excluded.project,
			title           = excluded.title,
			ts              = excluded.ts,
			max_sev         = excluded.max_sev,
			excerpt         = excluded.excerpt,
			md_rel_path     = excluded.md_rel_path,
			envelope_json   = excluded.envelope_json,
			checks_json     = excluded.checks_json,
			warnings_count  = excluded.warnings_count,
			updated_at      = excluded.updated_at,
			idempotency_key = CASE WHEN excluded.idempotency_key <> '' THEN excluded.idempotency_key ELSE reports.idempotency_key END,
			content_hash    = excluded.content_hash,
			md_hash         = excluded.md_hash,
			revision        = excluded.revision,
			last_change     = excluded.last_change,
			tool            = CASE WHEN excluded.tool  <> '' THEN excluded.tool  ELSE reports.tool  END,
			model           = CASE WHEN excluded.model <> '' THEN excluded.model ELSE reports.model END,
			project_key     = excluded.project_key
	`, docID, slug, env.Report.Project, env.Report.Title, env.Report.TS, env.Report.MaxSev,
		env.Report.Excerpt, relSlash, string(envBytes), string(checksBytes), len(warnings), meta.SQLUpdatedAt,
		meta.IdempotencyKey, meta.ContentHash, mdHash, meta.Revision, meta.LastChange,
		meta.Tool, meta.Model, ProjectKey(env.Report.Project))
	if err != nil {
		return nil, fmt.Errorf("indexando reporte en SQLite: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM canvas_blocks WHERE doc_id = ?`, docID); err != nil {
		return nil, err
	}
	for blockID, rawHTML := range canvasMap {
		if _, err := tx.ExecContext(ctx, `INSERT INTO canvas_blocks (doc_id, block_id, html) VALUES (?, ?, ?)`, docID, blockID, rawHTML); err != nil {
			return nil, err
		}
	}

	if s.hasFTS {
		_, _ = tx.ExecContext(ctx, `DELETE FROM reports_search WHERE id = ?`, docID)
		_, _ = tx.ExecContext(ctx, `INSERT INTO reports_search (id, title, project, tool, model, body) VALUES (?, ?, ?, ?, ?, ?)`,
			docID, env.Report.Title, env.Report.Project, meta.Tool, meta.Model, mdContent)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	absMDPath := filepath.Join(s.baseDir, filepath.FromSlash(relSlash))
	s.recordDiskWrite(relSlash, absMDPath, mdContent)
	summary := contract.ReportSummary{
		ID:            docID,
		Slug:          slug,
		Project:       env.Report.Project,
		Tool:          meta.Tool,
		Model:         meta.Model,
		Title:         env.Report.Title,
		TS:            env.Report.TS,
		Revision:      meta.Revision,
		MaxSev:        env.Report.MaxSev,
		Excerpt:       env.Report.Excerpt,
		MarkdownPath:  absMDPath,
		WarningsCount: len(warnings),
	}
	if meta.Revision > 1 {
		summary.UpdatedAt = meta.UpdatedAt
		summary.LastChange = meta.LastChange
	}

	return &PublishResult{
		ID:           docID,
		Action:       meta.Action,
		Revision:     meta.Revision,
		ChangeNote:   meta.LastChange,
		Summary:      summary,
		Envelope:     env,
		Warnings:     warnings,
		MarkdownPath: absMDPath,
		ModeUsed:     modeUsed,
	}, nil
}
