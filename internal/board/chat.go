package board

import (
	"fmt"
	"sort"
	"strings"
)

// ChatMarkdown es la bitácora de la conversación del equipo, como un chat: un hilo por
// tarea con cada mensaje firmado por su rol, los movimientos con su evidencia (grounding)
// y los puntos donde el equipo se atoró. Sirve para ver si hubo lluvia de ideas, cómo
// llegaron a cada acuerdo y en qué se quedaron pegados.
func ChatMarkdown(project string, tasks []*Task, alerts []Alert, boardURL string) string {
	ordered := append([]*Task(nil), tasks...)
	sort.SliceStable(ordered, func(i, j int) bool { return taskNum(ordered[i].ID) < taskNum(ordered[j].ID) })

	type stats struct{ msgs, questions, decisions, agreements, rejects, verified, flagged int }
	per := map[string]*stats{}
	people := map[string]bool{}
	total := stats{}
	for _, t := range ordered {
		s := &stats{}
		for _, e := range t.History {
			if e.IsComment() {
				s.msgs++
				people[strings.ToLower(one(e.By))] = true
				switch e.Tag {
				case "pregunta":
					s.questions++
				case "decision":
					s.decisions++
				case "acuerdo":
					s.agreements++
				case "verificado", "alerta":
					// Solo cuenta si lo hizo el moderador (datos anteriores a la regla pueden traer
					// «verificado» puesto por el mismo Dev: eso no es una verificación).
					if RoleOf(e.By) == RoleModerator {
						if e.Tag == "verificado" {
							s.verified++
						} else {
							s.flagged++
						}
					}
				}
			} else if e.To == Rejected {
				s.rejects++
			}
		}
		per[t.ID] = s
		total.msgs += s.msgs
		total.questions += s.questions
		total.decisions += s.decisions
		total.agreements += s.agreements
		total.rejects += s.rejects
	}
	alertsBy := map[string][]Alert{}
	for _, a := range alerts {
		alertsBy[a.TaskID] = append(alertsBy[a.TaskID], a)
	}
	open := map[string]int{}
	for _, q := range openQuestions(tasks) {
		open[q.task.ID]++
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Chat del equipo · %s\n\n", one(project))
	b.WriteString("La conversación entre los agentes, tarea por tarea: quién dijo qué, con qué evidencia y dónde se atoraron.")
	if boardURL != "" {
		fmt.Fprintf(&b, " El estado de las tareas está en el [tablero](%s).", boardURL)
	}
	b.WriteString("\n\n```kpi\n")
	fmt.Fprintf(&b, "Mensajes | %d | | |\n", total.msgs)
	fmt.Fprintf(&b, "Participantes | %d | | |\n", len(people))
	fmt.Fprintf(&b, "Preguntas | %d | | |\n", total.questions)
	fmt.Fprintf(&b, "Decisiones | %d | | |\n", total.decisions)
	fmt.Fprintf(&b, "Acuerdos | %d | | | %s\n", total.agreements, iff(total.agreements > 0, "success", "info"))
	fmt.Fprintf(&b, "Rechazos | %d | | | %s\n", total.rejects, iff(total.rejects > 0, "warning", "success"))
	b.WriteString("```\n\n")

	if len(ordered) == 0 {
		b.WriteString("_Todavía no hay conversación._\n")
		return b.String()
	}

	b.WriteString("## Cómo fue cada tarea\n\n| Tarea | Estado | Mensajes | Preguntas | Decisiones | Acuerdos | Rechazos | Lectura |\n|---|---|---|---|---|---|---|---|\n")
	for _, t := range ordered {
		s := per[t.ID]
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d | %d | %s |\n", t.ID, ColumnLabel(t.Status), s.msgs, s.questions, s.decisions, s.agreements, s.rejects, cell(reading(t, *s, open[t.ID], alertsBy[t.ID])))
	}
	b.WriteString("\n")

	for _, t := range ordered {
		fmt.Fprintf(&b, "## %s · %s\n\n", t.ID, one(t.Title))
		meta := []string{"Estado: **" + ColumnLabel(t.Status) + "**", "Le toca a " + roleLabel(t.Role)}
		if t.Owner != "" {
			meta = append(meta, "La tiene "+one(t.Owner))
		}
		b.WriteString(strings.Join(meta, " · ") + "\n\n")
		for _, a := range alertsBy[t.ID] {
			if a.Level != "info" {
				fmt.Fprintf(&b, "> [!%s] Se atoró: %s\n> %s\n\n", map[string]string{"error": "ERROR", "warning": "WARNING"}[a.Level], kindLabel(a.Kind), one(a.Message))
			}
		}
		for _, e := range t.History {
			writeChatLine(&b, e)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// writeChatLine dibuja un mensaje (comentario) o un movimiento (con su evidencia).
func writeChatLine(b *strings.Builder, e Event) {
	who := one(e.By)
	if who == "" {
		who = "agente"
	}
	head := fmt.Sprintf("**%s** · %s", who, timeOnly(e.At))
	if e.IsComment() {
		moderator := RoleOf(e.By) == RoleModerator
		switch {
		case e.Tag == "alerta" && moderator:
			fmt.Fprintf(b, "> [!ERROR] %s · moderador marcó una alerta\n> %s\n\n", who, one(e.Text))
		case e.Tag == "verificado" && moderator:
			fmt.Fprintf(b, "> [!TIP] %s · verificó\n> %s\n\n", who, one(e.Text))
		case e.Tag == "verificado" || e.Tag == "alerta":
			// Puesto por quien no es moderador (datos anteriores a la regla): se muestra como nota.
			fmt.Fprintf(b, "%s · *Nota (se marcó «%s» a sí mismo):* %s\n\n", head, TagLabel(e.Tag), one(e.Text))
		case e.Tag == "acuerdo" || e.Tag == "decision":
			fmt.Fprintf(b, "> [!NOTE] %s · %s\n> %s\n\n", who, TagLabel(e.Tag), one(e.Text))
		default:
			fmt.Fprintf(b, "%s · *%s:* %s\n\n", head, TagLabel(e.Tag), one(e.Text))
		}
		return
	}
	switch {
	case e.Reason != "":
		fmt.Fprintf(b, "%s · rechazó *(→ %s)*: %s\n\n", head, ColumnLabel(e.To), one(e.Reason))
	case e.Note != "" && e.Note != "tomada para revisar" && (e.To == Review || e.To == Done):
		fmt.Fprintf(b, "%s · *→ %s* con evidencia: `%s`\n\n", head, ColumnLabel(e.To), strings.ReplaceAll(one(e.Note), "`", "'"))
	default:
		fmt.Fprintf(b, "%s · *→ %s* · %s\n\n", head, ColumnLabel(e.To), one(e.Note))
	}
}

// reading resume en una frase cómo fue la conversación de la tarea.
func reading(t *Task, s struct{ msgs, questions, decisions, agreements, rejects, verified, flagged int }, openQ int, alerts []Alert) string {
	var parts []string
	switch {
	case s.msgs-s.verified-s.flagged >= 3: // discusión entre roles (sin contar al moderador)
		parts = append(parts, "hubo lluvia de ideas")
	case s.msgs == 0 && t.Status != Pending:
		parts = append(parts, "sin discusión")
	case s.msgs == 0 && t.Status == Pending:
		return "sin empezar"
	}
	if s.rejects >= maxRejects {
		parts = append(parts, fmt.Sprintf("se atoró (%d rechazos)", s.rejects))
	} else if s.rejects > 0 {
		parts = append(parts, fmt.Sprintf("%d rechazo(s)", s.rejects))
	}
	if openQ > 0 {
		parts = append(parts, fmt.Sprintf("%d pregunta(s) esperando", openQ))
	}
	if s.agreements > 0 {
		parts = append(parts, "llegaron a acuerdo")
	}
	for _, a := range alerts {
		if a.Kind == "decisiones_en_conflicto" {
			parts = append(parts, "decisiones sin conciliar")
		}
		switch a.Kind {
		case "evidencia_inexistente":
			parts = append(parts, "evidencia que no existe")
		case "acaparamiento":
			parts = append(parts, "su dueño tiene otra tarea en curso")
		case "rol_equivocado":
			parts = append(parts, "la tiene otro rol")
		case "evidencia_no_verificable":
			parts = append(parts, "evidencia no verificable")
		}
	}
	if s.verified > 0 {
		parts = append(parts, "verificada por el moderador")
	}
	if len(parts) == 0 {
		return "fluida"
	}
	return strings.Join(parts, "; ")
}

func taskNum(id string) int {
	n := 0
	for _, r := range id {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		}
	}
	return n
}
