package board

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Markdown dibuja el tablero de un proyecto: KPI por columna, carga por rol, una sección
// por columna y el detalle colapsable de cada tarea (criterios e historial).
func Markdown(project string, tasks []*Task, alerts ...Alert) string {
	by := map[string][]*Task{}
	owners := map[string]bool{}
	for _, t := range tasks {
		by[t.Status] = append(by[t.Status], t)
		if t.Owner != "" && (t.Status == InProgress || t.Status == Review) {
			owners[t.Owner] = true
		}
	}
	for _, c := range Columns {
		SortTasks(by[c])
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Tablero · %s\n\n", one(project))
	b.WriteString(boardIntro + "\n\n")
	b.WriteString("```kpi\n")
	fmt.Fprintf(&b, "Pendientes | %d | | |\n", len(by[Pending]))
	fmt.Fprintf(&b, "En curso | %d | | |\n", len(by[InProgress]))
	fmt.Fprintf(&b, "En revisión | %d | | | %s\n", len(by[Review]), iff(len(by[Review]) > 0, "warning", "success"))
	fmt.Fprintf(&b, "Rechazadas | %d | | | %s\n", len(by[Rejected]), iff(len(by[Rejected]) > 0, "error", "success"))
	fmt.Fprintf(&b, "Hechas | %d | | | success\n", len(by[Done]))
	fmt.Fprintf(&b, "Agentes trabajando | %d | | |\n", len(owners))
	open := openQuestions(tasks)
	fmt.Fprintf(&b, "Preguntas abiertas | %d | | | %s\n", len(open), iff(len(open) > 0, "warning", "success"))
	serious := 0
	for _, a := range alerts {
		if a.Level != "info" {
			serious++
		}
	}
	fmt.Fprintf(&b, "Alertas del moderador | %d | | | %s\n", serious, iff(serious > 0, iff(hasError(alerts), "error", "warning"), "success"))
	b.WriteString("```\n\n")

	if len(tasks) == 0 {
		b.WriteString("_El tablero está vacío. El PO o el Dev crean tareas con `board_task` (action: create)._\n")
		return b.String()
	}

	writeHealth(&b, alerts)
	writeConversation(&b, tasks, open)

	b.WriteString("## Carga por rol\n\n| Rol | Pendientes | En curso | En revisión | Rechazadas |\n|---|---|---|---|---|\n")
	rolesToShow := []string{RoleDev, RoleTester, RolePO}
	for _, t := range tasks {
		if t.Role == RoleUX {
			rolesToShow = append(rolesToShow, RoleUX)
			break
		}
	}
	for _, r := range rolesToShow {
		count := func(s string) int {
			n := 0
			for _, t := range by[s] {
				if t.Role == r {
					n++
				}
			}
			return n
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d |\n", roleLabel(r), count(Pending), count(InProgress), count(Review), count(Rejected))
	}
	b.WriteString("\n")

	if m := depGraph(tasks); m != "" {
		b.WriteString("## Dependencias\n\n```mermaid\n" + m + "```\n\n")
	}

	for _, c := range Columns {
		list := by[c]
		if len(list) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n| Tarea | Título | Le toca a | Dueño | Actualizada | %s |\n|---|---|---|---|---|---|\n", ColumnLabel(c), lastCol(c))
		for _, t := range list {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", t.ID, cell(t.Title), roleLabel(t.Role), cell(t.Owner), clock(t.Updated), cell(lastNote(t)))
		}
		if c == Rejected {
			for _, t := range list {
				fmt.Fprintf(&b, "\n> [!ERROR] %s rechazada\n> %s\n", t.ID, one(lastReason(t)))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## Detalle de tareas\n\n")
	for _, c := range Columns {
		for _, t := range by[c] {
			fmt.Fprintf(&b, "<details><summary>%s · %s (%s)</summary>\n\n", t.ID, html(t.Title), ColumnLabel(t.Status))
			if t.Detail != "" {
				b.WriteString(one(t.Detail) + "\n\n")
			}
			if len(t.Criteria) > 0 {
				b.WriteString("**Criterios de aceptación**\n\n")
				for _, cr := range t.Criteria {
					b.WriteString("- " + one(cr) + "\n")
				}
				b.WriteString("\n")
			}
			if len(t.DependsOn) > 0 {
				b.WriteString("**Depende de:** " + strings.Join(t.DependsOn, ", ") + "\n\n")
			}
			if t.Evidence != "" {
				b.WriteString("**Evidencia:** " + one(t.Evidence) + "\n\n")
			}
			b.WriteString("| Hora | Quién | De | A | Nota |\n|---|---|---|---|---|\n")
			for _, h := range t.History {
				note := h.Note
				if h.Reason != "" {
					note = "Motivo: " + h.Reason
				}
				if h.IsComment() {
					note = TagLabel(h.Tag) + ": " + h.Text
				}
				fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", clock(h.At), cell(h.By), ColumnLabel(h.From), ColumnLabel(h.To), cell(note))
			}
			b.WriteString("\n</details>\n\n")
		}
	}
	return b.String()
}

func hasError(alerts []Alert) bool {
	for _, a := range alerts {
		if a.Level == "error" {
			return true
		}
	}
	return false
}

// writeHealth muestra lo que el moderador automático encontró: evidencia que no se
// sostiene, decisiones en conflicto, tareas que rebotan, preguntas sin respuesta.
func writeHealth(b *strings.Builder, alerts []Alert) {
	if len(alerts) == 0 {
		b.WriteString("## Salud del tablero\n\n> [!TIP] Sin alertas\n> La evidencia citada se puede verificar y no hay decisiones en conflicto ni preguntas olvidadas.\n\n")
		return
	}
	b.WriteString("## Salud del tablero\n\nLo que el moderador encontró. Un error suele ser una alucinación (evidencia que no existe) o roles que no se están entendiendo.\n\n")
	tag := map[string]string{"error": "ERROR", "warning": "WARNING", "info": "NOTE"}
	for _, a := range alerts {
		fmt.Fprintf(b, "> [!%s] %s · %s\n> %s\n\n", tag[a.Level], a.TaskID, kindLabel(a.Kind), one(a.Message))
	}
}

func kindLabel(kind string) string {
	switch kind {
	case "evidencia_inexistente":
		return "evidencia que no existe"
	case "evidencia_no_verificable":
		return "evidencia no verificable"
	case "evidencia_sin_resultado":
		return "evidencia sin resultado"
	case "decisiones_en_conflicto":
		return "decisiones en conflicto"
	case "rebote":
		return "rebota entre roles"
	case "pregunta_sin_respuesta":
		return "pregunta sin respuesta"
	case "autorrespuesta":
		return "se respondió a sí mismo"
	case "sin_criterios":
		return "sin criterios"
	case "estancada":
		return "estancada"
	case "rol_equivocado":
		return "la tiene otro rol"
	case "sin_rol":
		return "dueño sin rol"
	case "acaparamiento":
		return "acapara tareas"
	}
	return kind
}

type feedItem struct {
	task *Task
	ev   Event
}

// feed reúne movimientos y comentarios de todas las tareas en orden cronológico.
func feed(tasks []*Task) []feedItem {
	var out []feedItem
	for _, t := range tasks {
		for _, e := range t.History {
			out = append(out, feedItem{t, e})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ev.At < out[j].ev.At })
	return out
}

// openQuestions: preguntas sin una respuesta posterior en la misma tarea.
func openQuestions(tasks []*Task) []feedItem {
	var out []feedItem
	for _, t := range tasks {
		var pending []Event
		for _, e := range t.History {
			switch e.Tag {
			case "pregunta":
				pending = append(pending, e)
			case "respuesta":
				if len(pending) > 0 {
					pending = pending[1:] // responde la pregunta más antigua de la tarea
				}
			}
		}
		for _, e := range pending {
			out = append(out, feedItem{t, e})
		}
	}
	return out
}

// writeConversation muestra cómo se pone de acuerdo el equipo: conclusiones, lo que
// espera respuesta y la conversación completa (movimientos + comentarios clave).
func writeConversation(b *strings.Builder, tasks []*Task, open []feedItem) {
	items := feed(tasks)
	var agreements []feedItem
	for i := len(items) - 1; i >= 0; i-- {
		if t := items[i].ev.Tag; t == "decision" || t == "acuerdo" {
			agreements = append(agreements, items[i])
		}
	}
	if len(agreements) > 0 {
		b.WriteString("## Decisiones y acuerdos\n\nA qué conclusiones llegó el equipo (lo más reciente primero).\n\n")
		for _, it := range agreements {
			fmt.Fprintf(b, "- **%s** · %s · %s (%s): %s\n", TagLabel(it.ev.Tag), it.task.ID, one(it.ev.By), clock(it.ev.At), one(it.ev.Text))
		}
		b.WriteString("\n")
	}
	if len(open) > 0 {
		b.WriteString("## Preguntas abiertas\n\n")
		for _, it := range open {
			fmt.Fprintf(b, "> [!WARNING] %s · %s pregunta\n> %s\n\n", it.task.ID, one(it.ev.By), one(it.ev.Text))
		}
	}
	if len(items) == 0 {
		return
	}
	const maxRows = 80
	shown := items
	if len(shown) > maxRows {
		shown = shown[len(shown)-maxRows:]
	}
	b.WriteString("## Conversación del equipo\n\n")
	if len(items) > maxRows {
		fmt.Fprintf(b, "Últimos %d de %d eventos; el resto está en el detalle de cada tarea.\n\n", maxRows, len(items))
	}
	b.WriteString("| Hora | Quién | Tarea | Qué | Mensaje |\n|---|---|---|---|---|\n")
	for _, it := range shown {
		what, msg := "", ""
		if it.ev.IsComment() {
			what, msg = TagLabel(it.ev.Tag), it.ev.Text
		} else {
			what = "→ " + ColumnLabel(it.ev.To)
			msg = it.ev.Note
			if it.ev.Reason != "" {
				msg = "Motivo: " + it.ev.Reason
			}
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n", timeOnly(it.ev.At), cell(it.ev.By), it.task.ID, what, cell(short(msg, 220)))
	}
	b.WriteString("\n")
}

// boardIntro abre el tablero (el servicio le agrega el enlace al chat del equipo).
const boardIntro = "Dev, Tester y PO coordinan aquí: cada agente toma tareas de su rol y las mueve con evidencia. Se actualiza en vivo."

var nodeRe = regexp.MustCompile(`[^A-Za-z0-9]+`)

func depGraph(tasks []*Task) string {
	var edges []string
	for _, t := range tasks {
		for _, d := range t.DependsOn {
			edges = append(edges, fmt.Sprintf("  %s --> %s", nodeID(d), nodeID(t.ID)))
		}
	}
	if len(edges) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	for _, t := range tasks {
		fmt.Fprintf(&b, "  %s[%s %s]\n", nodeID(t.ID), t.ID, strings.NewReplacer("[", "(", "]", ")", "|", "/", `"`, "'").Replace(short(t.Title, 32)))
	}
	b.WriteString(strings.Join(edges, "\n") + "\n")
	return b.String()
}

func nodeID(id string) string { return "t_" + nodeRe.ReplaceAllString(id, "_") }

func roleLabel(r string) string {
	switch r {
	case RoleUser:
		return "Usuario"
	case RoleDev:
		return "Dev"
	case RoleTester:
		return "Tester"
	case RolePO:
		return "PO"
	case RoleUX:
		return "UX"
	}
	return r
}

func lastCol(status string) string {
	if status == Rejected {
		return "Motivo"
	}
	return "Última nota"
}

func lastNote(t *Task) string {
	if t.Status == Rejected {
		return lastReason(t)
	}
	for i := len(t.History) - 1; i >= 0; i-- {
		if t.History[i].Note != "" {
			return short(t.History[i].Note, 80)
		}
	}
	return ""
}

func lastReason(t *Task) string {
	for i := len(t.History) - 1; i >= 0; i-- {
		if t.History[i].Reason != "" {
			return t.History[i].Reason
		}
	}
	return ""
}

func iff(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}

func one(s string) string { return strings.Join(strings.Fields(s), " ") }

func short(s string, n int) string {
	s = one(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func cell(s string) string {
	s = one(s)
	if s == "" {
		return "-"
	}
	return strings.ReplaceAll(s, "|", "\\|")
}

func html(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(one(s))
}

// timeOnly hora local con segundos (la conversación ocurre en un mismo día de trabajo).
func timeOnly(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Local().Format("15:04:05")
	}
	return ts
}

func clock(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Local().Format("01-02 15:04")
	}
	return ts
}
