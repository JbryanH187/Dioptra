package layoutagent

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"dioptra/internal/contract"
	"dioptra/internal/render"
)

// Umbral de cobertura léxica por debajo del cual la salida se considera un resumen
// encubierto (viola F5) y se degrada completa a la ruta rápida goldmark.
const (
	minCoverage         = 0.45
	minWordsForCoverage = 40
	maxCanvasPerReport  = 1
	preservedTitle      = "Contenido preservado por fidelidad"
)

// ErrLowFidelity indica que el modelo omitió demasiado contenido de la fuente.
var ErrLowFidelity = errors.New("fidelidad insuficiente: la síntesis omite demasiado contenido de la fuente")

var (
	refRe          = regexp.MustCompile(`[A-Za-z0-9_./-]+\.[A-Za-z0-9_]+:\d+(?::\d+)?`)
	ipLikeRe       = regexp.MustCompile(`^[\d.]+:\d+`)
	numTokenRe     = regexp.MustCompile(`[-+]?\d[\d,]*(?:\.\d+)?`)
	digitCommaRe   = regexp.MustCompile(`(\d),(\d)`)
	ownerStripRe   = regexp.MustCompile(`@[A-Za-z0-9_.-]+`)
	sevStripRe     = regexp.MustCompile(`(?i)\[(info|success|warning|error)\]`)
	htmlCommentRe  = regexp.MustCompile(`<!--.*?-->`)
	canvasExternRe = regexp.MustCompile(`(?i)(https?://|fetch\s*\(|XMLHttpRequest|WebSocket|import\s*\(|<script[^>]+src=|<link\b|localStorage|document\.cookie)`)
	canvasColorRe  = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\brgba?\s*\(|\bhsla?\s*\(`)
)

// FidelityReport resume la auditoría del Guardián de Fidelidad.
type FidelityReport struct {
	Coverage     float64
	Repairs      int
	Warnings     []contract.Warning
	WeakSections []string // títulos "## " de la fuente con cobertura insuficiente
}

func (r *FidelityReport) warn(path, action, reason string) {
	r.Warnings = append(r.Warnings, contract.Warning{Path: path, Action: action, Reason: reason})
	if action == "repaired" || action == "ignored" {
		r.Repairs++
	}
}

// ApplyFidelityGuard audita y repara determinísticamente la salida del LLM contra la
// fuente: F4 tareas chk-N canónicas, F2 KPIs verificables, F1 código íntegro,
// F3 referencias archivo:línea, F5 cobertura y reglas de aislamiento de free-canvas.
func ApplyFidelityGuard(env *contract.Envelope, baseline *contract.Envelope, sourceMD string) (FidelityReport, error) {
	rep := FidelityReport{}
	if env == nil {
		return rep, errors.New("sobre vacío")
	}
	body := render.ParseFrontmatter(sourceMD).Body

	// F5 — Cobertura léxica (antes de cualquier reparación)
	var outText strings.Builder
	collectStrings(env.Report.Blocks, &outText)
	coverage, srcWords := wordCoverage(body, outText.String())
	rep.Coverage = coverage
	rep.WeakSections = weakSections(body, outText.String())
	if srcWords >= minWordsForCoverage && coverage < minCoverage {
		return rep, fmt.Errorf("%w (cobertura %.0f%% < %.0f%%)", ErrLowFidelity, coverage*100, minCoverage*100)
	}

	// F4 — Tareas canónicas chk-N
	if baseline != nil {
		reconcileChecklists(env, contract.CollectChecklists(baseline.Report.Blocks), &rep)
	}

	// F2 — KPIs verificables contra la fuente
	verifyKPIs(env, body, &rep)

	// free-canvas — límite por reporte y auditoría de aislamiento/tema
	auditCanvas(env, &rep)

	// F1 — Código íntegro y F3 — referencias
	var preserved []map[string]any
	if baseline != nil {
		preserved = append(preserved, missingCodeBlocks(env, baseline, &rep)...)
	}
	outText.Reset()
	collectStrings(env.Report.Blocks, &outText)
	if refsBlock := missingRefs(body, outText.String(), &rep); refsBlock != nil {
		preserved = append(preserved, refsBlock)
	}
	if len(preserved) > 0 {
		env.Report.Blocks = append(env.Report.Blocks, map[string]any{
			"t":      "section",
			"title":  preservedTitle,
			"open":   false,
			"blocks": preserved,
		})
	}
	return rep, nil
}

// ---------------------------------------------------------------------------
// Recorrido genérico de bloques
// ---------------------------------------------------------------------------

func asBlockList(v any) []map[string]any {
	switch x := v.(type) {
	case []map[string]any:
		return x
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, it := range x {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// filterBlocks recorre bloques (incluyendo secciones anidadas) y conserva los que fn acepta.
// Las secciones que quedan vacías se eliminan para no violar el Contrato B.
func filterBlocks(list []map[string]any, path string, fn func(b map[string]any, path string) bool) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for i, b := range list {
		p := fmt.Sprintf("%s[%d]", path, i)
		if !fn(b, p) {
			continue
		}
		if t, _ := b["t"].(string); t == "section" {
			children := filterBlocks(asBlockList(b["blocks"]), p+".blocks", fn)
			if len(children) == 0 {
				continue
			}
			b["blocks"] = children
		}
		out = append(out, b)
	}
	return out
}

func collectStrings(v any, sb *strings.Builder) {
	switch x := v.(type) {
	case string:
		sb.WriteString(x)
		sb.WriteByte('\n')
	case []map[string]any:
		for _, m := range x {
			collectStrings(m, sb)
		}
	case []any:
		for _, it := range x {
			collectStrings(it, sb)
		}
	case [][]string:
		for _, row := range x {
			for _, s := range row {
				sb.WriteString(s)
				sb.WriteByte('\n')
			}
		}
	case []string:
		for _, s := range x {
			sb.WriteString(s)
			sb.WriteByte('\n')
		}
	case map[string]any:
		for k, val := range x {
			if k == "t" || k == "id" || k == "key" || k == "type" || k == "kind" {
				continue
			}
			collectStrings(val, sb)
		}
	case float64:
		sb.WriteString(strconv.FormatFloat(x, 'f', -1, 64))
		sb.WriteByte('\n')
	case int:
		sb.WriteString(strconv.Itoa(x))
		sb.WriteByte('\n')
	}
}

func tokenizeWords(s string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(w)) >= 4 {
			set[w] = true
		}
	}
	return set
}

// stripFencedCode quita el contenido de los bloques ``` / ~~~ de la fuente. El
// código lo protege F1 por separado y un diagrama mermaid/d2 convertido a nodos no
// repite sus palabras clave: contarlos en F5 producía rechazos falsos.
func stripFencedCode(src string) string {
	var out strings.Builder
	fence := ""
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence == "" {
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				fence = trimmed[:3]
				continue
			}
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, fence[:1]) == "" {
			fence = ""
		}
	}
	return out.String()
}

// weakSections devuelve los títulos "## " de la fuente cuya cobertura en la salida
// es menor a minCoverage, para que el anfitrión corrija solo esa parte.
func weakSections(source, output string) []string {
	out := tokenizeWords(output)
	var weak []string
	title, body := "", []string{}
	check := func() {
		if title == "" {
			return
		}
		words := tokenizeWords(stripFencedCode(strings.Join(body, "\n")))
		if len(words) < 15 {
			return
		}
		hit := 0
		for w := range words {
			if out[w] {
				hit++
			}
		}
		if float64(hit)/float64(len(words)) < minCoverage {
			weak = append(weak, title)
		}
	}
	for _, line := range strings.Split(source, "\n") {
		if strings.HasPrefix(line, "## ") {
			check()
			title, body = strings.TrimSpace(strings.TrimPrefix(line, "## ")), nil
			continue
		}
		body = append(body, line)
	}
	check()
	return weak
}

func wordCoverage(source, output string) (float64, int) {
	src := tokenizeWords(htmlCommentRe.ReplaceAllString(stripFencedCode(source), " "))
	if len(src) == 0 {
		return 1, 0
	}
	out := tokenizeWords(output)
	hit := 0
	for w := range src {
		if out[w] {
			hit++
		}
	}
	return float64(hit) / float64(len(src)), len(src)
}

// ---------------------------------------------------------------------------
// F4 — Reconciliación de tareas (sincronización bidireccional por ordinal chk-N)
// ---------------------------------------------------------------------------

func normalizeTask(s string) string {
	s = htmlCommentRe.ReplaceAllString(s, " ")
	s = ownerStripRe.ReplaceAllString(s, " ")
	s = sevStripRe.ReplaceAllString(s, " ")
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		switch r {
		case '*', '`', '_', '~':
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.Trim(s, " .:;,")
}

func taskJaccard(a, b string) float64 {
	wa, wb := map[string]bool{}, map[string]bool{}
	for _, w := range strings.Fields(a) {
		if len([]rune(w)) >= 3 {
			wa[w] = true
		}
	}
	for _, w := range strings.Fields(b) {
		if len([]rune(w)) >= 3 {
			wb[w] = true
		}
	}
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	inter := 0
	for w := range wa {
		if wb[w] {
			inter++
		}
	}
	return float64(inter) / float64(len(wa)+len(wb)-inter)
}

func reconcileChecklists(env *contract.Envelope, canon []contract.ChecklistItemState, rep *FidelityReport) {
	used := make([]bool, len(canon))
	canonNorm := make([]string, len(canon))
	for i, c := range canon {
		canonNorm[i] = normalizeTask(c.Text)
	}

	match := func(text string) int {
		n := normalizeTask(text)
		if n == "" {
			return -1
		}
		for i := range canon {
			if !used[i] && canonNorm[i] == n {
				return i
			}
		}
		for i := range canon {
			if !used[i] && len(n) >= 6 && len(canonNorm[i]) >= 6 &&
				(strings.Contains(canonNorm[i], n) || strings.Contains(n, canonNorm[i])) {
				return i
			}
		}
		best, bestScore := -1, 0.7
		for i := range canon {
			if used[i] {
				continue
			}
			if s := taskJaccard(canonNorm[i], n); s >= bestScore {
				best, bestScore = i, s
			}
		}
		return best
	}

	env.Report.Blocks = filterBlocks(env.Report.Blocks, "report.blocks", func(b map[string]any, path string) bool {
		if t, _ := b["t"].(string); t != "checklist" {
			return true
		}
		items := asBlockList(b["items"])
		kept := make([]map[string]any, 0, len(items))
		for j, it := range items {
			itPath := fmt.Sprintf("%s.items[%d]", path, j)
			txt, _ := it["text"].(string)
			idx := match(txt)
			if idx < 0 {
				rep.warn(itPath, "ignored", fmt.Sprintf("tarea %q inexistente en la fuente: eliminada para no desfasar la sincronización con el .md", truncate(txt, 60)))
				continue
			}
			used[idx] = true
			c := canon[idx]
			if oldID, _ := it["id"].(string); oldID != c.ID {
				rep.warn(itPath+".id", "repaired", fmt.Sprintf("id %q reasignado al ordinal canónico %q", oldID, c.ID))
			}
			fixed := map[string]any{"id": c.ID, "text": c.Text, "done": c.Done}
			if c.Owner != "" {
				fixed["owner"] = c.Owner
			} else if o, ok := it["owner"].(string); ok && o != "" {
				fixed["owner"] = o
			}
			if c.Sev != "" {
				fixed["sev"] = c.Sev
			} else if s, ok := it["sev"].(string); ok && s != "" {
				fixed["sev"] = s
			}
			kept = append(kept, fixed)
		}
		if len(kept) == 0 {
			return false
		}
		b["items"] = kept
		return true
	})

	var missing []map[string]any
	for i, c := range canon {
		if used[i] {
			continue
		}
		it := map[string]any{"id": c.ID, "text": c.Text, "done": c.Done}
		if c.Owner != "" {
			it["owner"] = c.Owner
		}
		if c.Sev != "" {
			it["sev"] = c.Sev
		}
		missing = append(missing, it)
	}
	if len(missing) > 0 {
		rep.warn("report.blocks", "repaired", fmt.Sprintf("%d tarea(s) omitida(s) por el modelo reinyectadas en orden canónico", len(missing)))
		env.Report.Blocks = append(env.Report.Blocks, map[string]any{"t": "checklist", "items": missing})
	}
}

// ---------------------------------------------------------------------------
// F2 — KPIs verificables
// ---------------------------------------------------------------------------

func normalizeNumericText(s string) string {
	s = strings.ToLower(s)
	for digitCommaRe.MatchString(s) {
		s = digitCommaRe.ReplaceAllString(s, "$1$2")
	}
	return s
}

func containsNumberToken(hay, tok string) bool {
	tok = strings.TrimPrefix(tok, "+")
	if tok == "" {
		return false
	}
	for start := 0; ; {
		idx := strings.Index(hay[start:], tok)
		if idx < 0 {
			return false
		}
		abs := start + idx
		end := abs + len(tok)
		beforeOK := abs == 0 || !isDigitOrDot(hay[abs-1])
		afterOK := end >= len(hay) || !unicode.IsDigit(rune(hay[end]))
		if beforeOK && afterOK {
			return true
		}
		start = abs + 1
	}
}

func isDigitOrDot(b byte) bool { return (b >= '0' && b <= '9') || b == '.' }

func kpiValueString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return fmt.Sprint(v)
}

func kpiValuePresent(value any, normSource string) bool {
	raw := strings.TrimSpace(kpiValueString(value))
	if raw == "" {
		return false
	}
	norm := normalizeNumericText(raw)
	// Todas las cifras del valor deben existir en la fuente ("3/5" exige 3 y 5).
	if nums := numTokenRe.FindAllString(norm, -1); len(nums) > 0 {
		for _, m := range nums {
			if !containsNumberToken(normSource, strings.ReplaceAll(m, ",", "")) {
				return false
			}
		}
		return true
	}
	return strings.Contains(normSource, norm)
}

func verifyKPIs(env *contract.Envelope, source string, rep *FidelityReport) {
	normSource := normalizeNumericText(source)
	env.Report.Blocks = filterBlocks(env.Report.Blocks, "report.blocks", func(b map[string]any, path string) bool {
		if t, _ := b["t"].(string); t != "kpi" {
			return true
		}
		items := asBlockList(b["items"])
		kept := make([]map[string]any, 0, len(items))
		for j, it := range items {
			if kpiValuePresent(it["value"], normSource) {
				kept = append(kept, it)
				continue
			}
			label, _ := it["label"].(string)
			rep.warn(fmt.Sprintf("%s.items[%d].value", path, j), "ignored",
				fmt.Sprintf("KPI %q con valor %q no aparece en la fuente (posible cifra inventada): eliminado", truncate(label, 40), kpiValueString(it["value"])))
		}
		if len(kept) == 0 {
			return false
		}
		b["items"] = kept
		return true
	})
}

// ---------------------------------------------------------------------------
// free-canvas — límite y auditoría
// ---------------------------------------------------------------------------

func auditCanvas(env *contract.Envelope, rep *FidelityReport) {
	seen := 0
	env.Report.Blocks = filterBlocks(env.Report.Blocks, "report.blocks", func(b map[string]any, path string) bool {
		if t, _ := b["t"].(string); t != "canvas" {
			return true
		}
		seen++
		if seen > maxCanvasPerReport {
			rep.warn(path, "ignored", fmt.Sprintf("free-canvas excedente eliminado (máximo %d por reporte)", maxCanvasPerReport))
			return false
		}
		html, _ := b["html"].(string)
		if m := canvasExternRe.FindString(html); m != "" {
			rep.Warnings = append(rep.Warnings, contract.Warning{Path: path + ".html", Action: "ignored",
				Reason: fmt.Sprintf("canvas referencia recurso externo o almacenamiento (%q): la CSP aislada lo bloqueará", m)})
		}
		if m := canvasColorRe.FindString(html); m != "" {
			rep.Warnings = append(rep.Warnings, contract.Warning{Path: path + ".html", Action: "ignored",
				Reason: fmt.Sprintf("canvas usa color literal (%q): no responderá al cambio de tema; usa var(--token)", m)})
		}
		return true
	})
}

// ---------------------------------------------------------------------------
// F1 — Código íntegro y F3 — referencias archivo:línea
// ---------------------------------------------------------------------------

func normalizeCode(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func missingCodeBlocks(env, baseline *contract.Envelope, rep *FidelityReport) []map[string]any {
	var outCode []string
	var walk func([]map[string]any)
	walk = func(list []map[string]any) {
		for _, b := range list {
			if t, _ := b["t"].(string); t == "code" {
				for _, k := range []string{"src", "old"} {
					if s, ok := b[k].(string); ok && s != "" {
						outCode = append(outCode, normalizeCode(s))
					}
				}
			}
			walk(asBlockList(b["blocks"]))
		}
	}
	walk(env.Report.Blocks)

	present := func(fragment string) bool {
		n := normalizeCode(fragment)
		if n == "" {
			return true
		}
		for _, oc := range outCode {
			if strings.Contains(oc, n) {
				return true
			}
		}
		return false
	}

	var missing []map[string]any
	var walkBase func([]map[string]any)
	walkBase = func(list []map[string]any) {
		for _, b := range list {
			if t, _ := b["t"].(string); t == "code" {
				src, _ := b["src"].(string)
				old, _ := b["old"].(string)
				if !present(src) || !present(old) {
					clean := map[string]any{"t": "code", "src": src}
					for _, k := range []string{"lang", "old", "ref", "hl"} {
						if v, ok := b[k]; ok && v != nil && v != "" {
							clean[k] = v
						}
					}
					missing = append(missing, clean)
				}
			}
			walkBase(asBlockList(b["blocks"]))
		}
	}
	walkBase(baseline.Report.Blocks)

	if len(missing) > 0 {
		rep.warn("report.blocks", "repaired", fmt.Sprintf("%d bloque(s) de código omitido(s) o alterado(s) por el modelo preservados íntegros desde la fuente", len(missing)))
	}
	return missing
}

func missingRefs(source, outText string, rep *FidelityReport) map[string]any {
	seen := map[string]bool{}
	var lost []string
	for _, ref := range refRe.FindAllString(source, -1) {
		ref = strings.Trim(ref, "./")
		if seen[ref] || ipLikeRe.MatchString(ref) {
			continue
		}
		seen[ref] = true
		if !strings.Contains(outText, ref) {
			lost = append(lost, ref)
		}
	}
	if len(lost) == 0 {
		return nil
	}
	rep.warn("report.blocks", "repaired", fmt.Sprintf("%d referencia(s) archivo:línea omitida(s) por el modelo preservadas", len(lost)))
	var sb strings.Builder
	for _, r := range lost {
		sb.WriteString("- ")
		sb.WriteString(r)
		sb.WriteByte('\n')
	}
	return map[string]any{"t": "prose", "title": "Referencias preservadas", "md": strings.TrimRight(sb.String(), "\n")}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
