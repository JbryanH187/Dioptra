package board

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Alert es una señal del moderador automático: algo en el tablero que no se sostiene
// con evidencia o que indica que los agentes no se están entendiendo.
type Alert struct {
	Level   string `json:"level"` // error | warning | info
	TaskID  string `json:"task"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// FileCheck verifica una referencia archivo:línea contra el repo del proyecto.
// known=false si no hay raíz registrada (no se puede comprobar); exists/lineOK si sí.
type FileCheck func(project, path string, line int) (known, exists, lineOK bool)

var (
	fileLineRe   = regexp.MustCompile(`(?:^|[\s(\x60])([\w./\\-]+\.[A-Za-z0-9]{1,8}):(\d+)\b`)
	commandRe    = regexp.MustCompile(`(?i)\b(go (test|vet|build)|npm (test|run)|node --test|pytest|mvnw?|gradlew?|dotnet (test|build)|cargo (test|build)|jest|vitest|curl|newman|playwright|check\.ps1)\b`)
	outcomeRe    = regexp.MustCompile(`(?i)\b(ok|pass(ed)?|verde|green|exit 0|0 (fail(ures|ed)?|errores)|\d+\s*(tests?|pruebas?)|✓|✔|fail(ed|s)?|rojo|HTTP \d{3}|\b[1-5]\d\d\b)`)
	reportLinkRe = regexp.MustCompile(`(?i)https?://|\br_[0-9a-f]{6,}\b|\be2e_[0-9a-f]{6,}\b`)
)

// Umbrales.
const (
	maxRejects       = 3
	questionAgeLimit = 2 * time.Hour
	staleLimit       = 4 * time.Hour
)

func parseTS(ts string) time.Time {
	t, _ := time.Parse(time.RFC3339, ts)
	return t
}

func sameActor(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}

// checkEvidence revisa que una evidencia se pueda verificar y que lo que cita exista.
func checkEvidence(t *Task, evidence string, files FileCheck) []Alert {
	ev := strings.TrimSpace(evidence)
	if ev == "" {
		return nil
	}
	var out []Alert
	refs := fileLineRe.FindAllStringSubmatch(ev, -1)
	hasCmd := commandRe.MatchString(ev)
	if !hasCmd && len(refs) == 0 && !reportLinkRe.MatchString(ev) {
		out = append(out, Alert{"warning", t.ID, "evidencia_no_verificable",
			fmt.Sprintf("La evidencia «%s» no trae un comando, un archivo:línea ni un enlace: nadie puede comprobarla.", short(ev, 90))})
	}
	if hasCmd && !outcomeRe.MatchString(ev) {
		out = append(out, Alert{"warning", t.ID, "evidencia_sin_resultado",
			"La evidencia menciona un comando pero no su resultado (ok/falló, cuántas pruebas, código HTTP)."})
	}
	if files != nil {
		for _, m := range refs {
			line, _ := strconv.Atoi(m[2])
			known, exists, lineOK := files(t.Project, m[1], line)
			switch {
			case !known:
			case !exists:
				out = append(out, Alert{"error", t.ID, "evidencia_inexistente",
					fmt.Sprintf("La evidencia cita %s:%s, pero ese archivo no existe en el repo. Posible alucinación.", m[1], m[2])})
			case !lineOK:
				out = append(out, Alert{"error", t.ID, "evidencia_inexistente",
					fmt.Sprintf("La evidencia cita %s:%s, pero el archivo no tiene esa línea. Posible alucinación.", m[1], m[2])})
			}
		}
	}
	return out
}

// Audit revisa el tablero completo y devuelve alertas ordenadas por gravedad.
func Audit(tasks []*Task, now time.Time, files FileCheck) []Alert {
	var out []Alert
	// Última actividad de todo el tablero: una tarea solo está "estancada" si el resto
	// del equipo siguió avanzando sin ella (no cuando todos pararon, p. ej. de noche).
	var boardLast time.Time
	for _, t := range tasks {
		for _, e := range t.History {
			if at := parseTS(e.At); at.After(boardLast) {
				boardLast = at
			}
		}
	}
	for _, t := range tasks {
		// 1. Evidencia de la última entrega (o de la tarea hecha).
		if t.Status == Review || t.Status == Done {
			out = append(out, checkEvidence(t, t.Evidence, files)...)
		}
		// 2. Hecha sin criterios: no había contra qué validarla.
		if t.Status == Done && len(t.Criteria) == 0 {
			out = append(out, Alert{"info", t.ID, "sin_criterios", "Se cerró sin criterios de aceptación: no hay contra qué comprobar que está bien."})
		}

		rejects := 0
		var questions []Event
		decisions := map[string]string{} // autor → última decisión desde el último acuerdo
		var lastMove time.Time
		for _, e := range t.History {
			if !e.IsComment() {
				lastMove = parseTS(e.At)
				if e.To == Rejected {
					rejects++
				}
			}
			switch e.Tag {
			case "pregunta":
				questions = append(questions, e)
			case "respuesta":
				if len(questions) > 0 {
					q := questions[0]
					questions = questions[1:]
					if sameActor(q.By, e.By) {
						out = append(out, Alert{"warning", t.ID, "autorrespuesta",
							fmt.Sprintf("%s respondió su propia pregunta («%s»). La respuesta debe venir del rol al que se le preguntó.", one(e.By), short(q.Text, 70))})
					}
				}
			case "decision":
				decisions[strings.ToLower(one(e.By))] = e.Text
			case "acuerdo":
				decisions = map[string]string{}
			}
		}
		// 3. Rebota: los roles no se están entendiendo sobre los criterios.
		if rejects >= maxRejects {
			out = append(out, Alert{"error", t.ID, "rebote",
				fmt.Sprintf("Se ha rechazado %d veces. Antes de otro intento, el PO debe aclarar los criterios y dejar un acuerdo.", rejects)})
		}
		// 4. Decisiones de distintos roles sin un acuerdo que las cierre.
		if len(decisions) >= 2 {
			var who []string
			for k := range decisions {
				who = append(who, k)
			}
			sort.Strings(who)
			out = append(out, Alert{"warning", t.ID, "decisiones_en_conflicto",
				fmt.Sprintf("Hay decisiones de %s sin un acuerdo que las concilie.", strings.Join(who, " y "))})
		}
		// 5. Preguntas sin respuesta demasiado tiempo (en una tarea cerrada ya no aplican).
		if t.Status != Done {
			for _, q := range questions {
				if age := now.Sub(parseTS(q.At)); age > questionAgeLimit {
					out = append(out, Alert{"warning", t.ID, "pregunta_sin_respuesta",
						fmt.Sprintf("«%s» lleva %s sin respuesta.", short(q.Text, 70), age.Round(time.Minute))})
				}
			}
		}
		// 6. En curso sin movimiento mientras el resto del tablero sí avanzó.
		if t.Status == InProgress && !lastMove.IsZero() && now.Sub(lastMove) > staleLimit && boardLast.Sub(lastMove) > staleLimit {
			out = append(out, Alert{"info", t.ID, "estancada",
				fmt.Sprintf("Lleva %s sin movimiento mientras el resto del tablero avanzó (%s).", boardLast.Sub(lastMove).Round(time.Minute), one(t.Owner))})
		}
	}
	// 7. Cada quien con lo suyo: dueño del rol equivocado, sin rol declarado, o acaparando.
	held := map[string][]string{}
	for _, t := range tasks {
		if t.Owner == "" || (t.Status != InProgress && t.Status != Review) {
			continue
		}
		switch r := RoleOf(t.Owner); {
		case r == "":
			out = append(out, Alert{"info", t.ID, "sin_rol",
				fmt.Sprintf("La tiene «%s», que no dice su rol: no se puede comprobar que le corresponde (firma como «<rol> · agente»).", one(t.Owner))})
		case r != t.Role:
			out = append(out, Alert{"error", t.ID, "rol_equivocado",
				fmt.Sprintf("La tiene %s pero le toca a %s. El moderador debe liberarla.", one(t.Owner), roleLabel(t.Role))})
		}
		if t.Status == InProgress {
			key := strings.ToLower(one(t.Owner))
			held[key] = append(held[key], t.ID)
		}
	}
	owners := make([]string, 0, len(held))
	for o := range held {
		owners = append(owners, o)
	}
	sort.Strings(owners)
	for _, o := range owners {
		if ids := held[o]; len(ids) > 1 {
			out = append(out, Alert{"warning", ids[0], "acaparamiento",
				fmt.Sprintf("%s tiene %d tareas en curso a la vez (%s). Una a la vez: que libere las que no está trabajando.", o, len(ids), strings.Join(ids, ", "))})
		}
	}

	rank := map[string]int{"error": 0, "warning": 1, "info": 2}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Level] < rank[out[j].Level] })
	return out
}

// submitter es quien entregó la tarea a revisión por última vez (el implementador).
func (t *Task) submitter() string {
	for i := len(t.History) - 1; i >= 0; i-- {
		e := t.History[i]
		if e.To == Review && e.From == InProgress {
			return e.By
		}
	}
	return ""
}
