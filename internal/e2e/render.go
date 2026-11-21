package e2e

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Markdown convierte la corrida en un reporte de Dioptra (KPI, mapa de servicios en
// Mermaid, una sección por caso con tabla de pasos y detalle colapsable, y la
// checklist de calidad de logs). `done` conserva las mejoras ya aprobadas en el visor
// (texto de la casilla → marcada) para que volver a publicar no las desmarque.
func Markdown(r *Run, done map[string]bool) string {
	var b strings.Builder
	failures, warnings := 0, 0
	for _, c := range r.Cases {
		for _, s := range c.Steps {
			switch s.Verdict {
			case "error":
				failures++
			case "warn":
				warnings++
			}
		}
	}

	fmt.Fprintf(&b, "# E2E · %s\n\n", oneLine(r.Title))
	meta := []string{"Estado: **" + r.Status + "**"}
	if r.Environment != "" {
		meta = append(meta, "Ambiente: **"+oneLine(r.Environment)+"**")
	}
	if r.Agent != "" {
		meta = append(meta, "Observó: "+oneLine(r.Agent))
	}
	meta = append(meta, "Inicio: "+humanTime(r.Started))
	b.WriteString(strings.Join(meta, " · ") + "\n\n")

	b.WriteString("```kpi\n")
	fmt.Fprintf(&b, "Casos | %d | | |\n", len(r.Cases))
	fmt.Fprintf(&b, "Pasos | %d | | |\n", r.TotalSteps())
	fmt.Fprintf(&b, "Fallos | %d | | | %s\n", failures, sevIf(failures > 0, "error", "success"))
	fmt.Fprintf(&b, "Advertencias | %d | | | %s\n", warnings, sevIf(warnings > 0, "warning", "success"))
	fmt.Fprintf(&b, "Servicios | %d | | |\n", len(r.Services))
	fmt.Fprintf(&b, "Mejoras de logs | %d | | | %s\n", len(r.Findings), sevIf(len(r.Findings) > 0, "warning", "success"))
	b.WriteString("```\n\n")

	if r.Summary != "" {
		b.WriteString("## Resumen\n\n" + strings.TrimSpace(r.Summary) + "\n\n")
	}
	if r.Redactions > 0 {
		fmt.Fprintf(&b, "> [!NOTE] Datos protegidos\n> Se ocultaron %d datos sensibles (tokens, contraseñas, tarjetas, correos, teléfonos, CURP/RFC) antes de guardar.\n\n", r.Redactions)
	}

	if m := serviceMap(r); m != "" {
		b.WriteString("## Mapa de servicios\n\n```mermaid\n" + m + "```\n\n")
	}

	if len(r.Services) > 0 {
		b.WriteString("## Servicios\n\n| Servicio | URL | Ambiente | Puerto | Logs |\n|---|---|---|---|---|\n")
		for _, s := range r.Services {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", cell(s.Name), codeCell(s.BaseURL), cell(s.Environment), cell(s.Port), cell(s.LogSource))
		}
		b.WriteString("\n")
	}

	for _, c := range r.Cases {
		writeCase(&b, c)
	}

	if len(r.Findings) > 0 {
		b.WriteString("## Calidad de logs\n\nMejoras detectadas durante la prueba. Márcalas cuando el equipo las atienda.\n\n")
		for _, f := range r.Findings {
			text := findingText(f)
			mark := " "
			if done[NormalizeCheck(text)] {
				mark = "x"
			}
			fmt.Fprintf(&b, "- [%s] %s [%s]\n", mark, text, f.Severity)
		}
		b.WriteString("\n")
	}

	if len(r.Notes) > 0 {
		b.WriteString("## Notas\n\n")
		for _, n := range r.Notes {
			b.WriteString("- " + oneLine(n) + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// NormalizeCheck reduce el texto de una casilla a una clave comparable: sin énfasis
// Markdown, sin la etiqueta [severidad], minúsculas y espacios colapsados. Así coincide
// el texto que se publicó con el que devuelve el estado del reporte.
func NormalizeCheck(s string) string {
	s = sevTagRe.ReplaceAllString(s, "")
	s = strings.NewReplacer("*", "", "`", "", "_", "").Replace(s)
	return strings.ToLower(oneLine(s))
}

var sevTagRe = regexp.MustCompile(`\s*\[(?:info|warning|error|ok)\]\s*$`)

// findingText es el texto de la casilla de un hallazgo.
func findingText(f Finding) string {
	parts := []string{}
	if f.Service != "" {
		parts = append(parts, "**"+oneLine(f.Service)+"**:")
	}
	parts = append(parts, oneLine(f.Issue))
	if f.Where != "" {
		parts = append(parts, "("+oneLine(f.Where)+")")
	}
	if f.Suggestion != "" {
		parts = append(parts, "→ "+oneLine(f.Suggestion))
	}
	return strings.Join(parts, " ")
}

func writeCase(b *strings.Builder, c Case) {
	fmt.Fprintf(b, "## Caso %d · %s\n\n", c.N, oneLine(c.Title))
	if c.Expected != "" {
		b.WriteString("**Esperado:** " + oneLine(c.Expected) + "\n\n")
	}
	errs := 0
	for _, s := range c.Steps {
		if s.Verdict == "error" {
			errs++
		}
	}
	switch {
	case c.Result != "" && errs > 0:
		fmt.Fprintf(b, "> [!ERROR] Resultado\n> %s\n\n", oneLine(c.Result))
	case c.Result != "":
		fmt.Fprintf(b, "> [!TIP] Resultado\n> %s\n\n", oneLine(c.Result))
	case errs > 0:
		fmt.Fprintf(b, "> [!ERROR] %d paso(s) con error\n> Revisa el detalle de cada paso abajo.\n\n", errs)
	}
	if len(c.Steps) == 0 {
		b.WriteString("_Sin pasos registrados todavía._\n\n")
		return
	}
	b.WriteString("| # | Hora | Tipo | Servicio | Operación | Estado | ms | Resultado |\n|---|---|---|---|---|---|---|---|\n")
	for _, s := range c.Steps {
		op := strings.TrimSpace(s.Method + " " + shortURL(s.URL))
		if op == "" {
			op = s.Title
		}
		status := ""
		if s.Status > 0 {
			status = fmt.Sprint(s.Status)
		}
		ms := ""
		if s.DurationMs > 0 {
			ms = fmt.Sprint(s.DurationMs)
		}
		fmt.Fprintf(b, "| %d | %s | %s | %s | %s | %s | %s | %s |\n",
			s.Seq, clock(s.Time), cell(s.Kind), cell(s.Service), cell(op), status, ms, verdictLabel(s.Verdict))
	}
	b.WriteString("\n")
	for _, s := range c.Steps {
		if s.Request == "" && s.Response == "" && s.Log == "" && s.Detail == "" {
			continue
		}
		title := strings.TrimSpace(fmt.Sprintf("Paso %d · %s %s", s.Seq, s.Method, shortURL(s.URL)))
		if s.Method == "" && s.Title != "" {
			title = fmt.Sprintf("Paso %d · %s", s.Seq, oneLine(s.Title))
		}
		fmt.Fprintf(b, "<details><summary>%s</summary>\n\n", htmlEscape(title))
		if s.Detail != "" {
			b.WriteString(oneLine(s.Detail) + "\n\n")
		}
		if s.Request != "" {
			b.WriteString("**Request**\n\n" + fence(s.Request) + "\n")
		}
		if s.Response != "" {
			b.WriteString("**Response**\n\n" + fence(s.Response) + "\n")
		}
		if s.Log != "" {
			b.WriteString("**Log**\n\n" + fence(s.Log) + "\n")
		}
		b.WriteString("</details>\n\n")
	}
}

var nodeIDRe = regexp.MustCompile(`[^A-Za-z0-9]+`)

// serviceMap genera un flowchart con las llamadas observadas (quién → servicio).
func serviceMap(r *Run) string {
	type edge struct{ from, to string }
	counts := map[edge]int{}
	nodes := map[string]bool{}
	for _, c := range r.Cases {
		for _, s := range c.Steps {
			if s.Service == "" || (s.Kind != "request" && s.Kind != "query") {
				continue
			}
			from := s.From
			if from == "" {
				from = "cliente"
			}
			counts[edge{from, s.Service}]++
			nodes[from], nodes[s.Service] = true, true
		}
	}
	if len(counts) == 0 {
		return ""
	}
	id := func(name string) string { return "n_" + strings.Trim(nodeIDRe.ReplaceAllString(name, "_"), "_") }
	label := func(name string) string {
		return strings.NewReplacer("[", "(", "]", ")", "|", "/", `"`, "'").Replace(name)
	}
	names := make([]string, 0, len(nodes))
	for n := range nodes {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	for _, n := range names {
		fmt.Fprintf(&b, "  %s[%s]\n", id(n), label(n))
	}
	edges := make([]edge, 0, len(counts))
	for e := range counts {
		edges = append(edges, e)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].from != edges[j].from {
			return edges[i].from < edges[j].from
		}
		return edges[i].to < edges[j].to
	})
	for _, e := range edges {
		fmt.Fprintf(&b, "  %s -->|%d| %s\n", id(e.from), counts[e], id(e.to))
	}
	return b.String()
}

func sevIf(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

func verdictLabel(v string) string {
	switch v {
	case "ok":
		return "ok"
	case "warn":
		return "warning"
	case "error":
		return "error"
	default:
		return "info"
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func cell(s string) string {
	s = oneLine(s)
	if s == "" {
		return "-"
	}
	return strings.ReplaceAll(s, "|", "\\|")
}

// codeCell muestra un valor técnico (URL) como código: el visor no lo convierte en enlace.
func codeCell(s string) string {
	s = oneLine(s)
	if s == "" {
		return "-"
	}
	return "`" + strings.NewReplacer("`", "'", "|", "\\|").Replace(s) + "`"
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// shortURL quita esquema y host para la tabla (el host ya está en la columna Servicio).
func shortURL(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		rest := u[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			u = rest[j:]
		} else {
			u = "/"
		}
	}
	if len(u) > 90 {
		u = u[:87] + "…"
	}
	return u
}

func clock(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Local().Format("15:04:05")
	}
	return ts
}

func humanTime(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return ts
}

// fence envuelve contenido en un bloque de código con un delimitador que no choca
// con backticks del propio contenido; detecta JSON para resaltarlo.
func fence(s string) string {
	ticks := "```"
	for strings.Contains(s, ticks) {
		ticks += "`"
	}
	lang := ""
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
		lang = "json"
	}
	return ticks + lang + "\n" + strings.TrimRight(s, "\n") + "\n" + ticks + "\n"
}
