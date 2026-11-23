// Package board implementa el tablero de equipo: agentes con rol (Dev, Tester, PO)
// coordinan tareas sobre un tablero compartido en vez de conversar entre sí.
// Cada agente ve el tablero, toma una tarea de su rol, la mueve con evidencia y el
// siguiente rol la recoge. Dioptra lo muestra como un reporte vivo por proyecto.
package board

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Estados de una tarea.
const (
	Pending    = "pendiente"
	InProgress = "en_curso"
	Review     = "en_revision"
	Done       = "hecha"
	Rejected   = "rechazada"
)

// Roles.
const (
	RoleDev    = "dev"
	RoleTester = "tester"
	RolePO     = "po"
	RoleUX     = "ux"
	// RoleUser es la persona: la última aprobación en modo manual. Solo actúa desde el visor.
	RoleUser = "usuario"
)

var validRoles = map[string]bool{RoleDev: true, RoleTester: true, RolePO: true, RoleUX: true, RoleUser: true}

// TimeLayout: UTC con microsegundos y ancho fijo, para que el orden por texto sea el
// orden real aunque varios eventos caigan en el mismo segundo.
const TimeLayout = "2006-01-02T15:04:05.000000Z07:00"

// Task es una tarea del tablero.
type Task struct {
	ID        string   `json:"id"`
	Project   string   `json:"project"`
	Title     string   `json:"title"`
	Detail    string   `json:"detail,omitempty"`
	Criteria  []string `json:"criteria,omitempty"` // criterios de aceptación
	Role      string   `json:"role"`               // a qué rol le toca ahora
	Status    string   `json:"status"`
	Owner     string   `json:"owner,omitempty"` // agente/sesión que la tiene
	DependsOn []string `json:"depends_on,omitempty"`
	Evidence  string   `json:"evidence,omitempty"`
	Created   string   `json:"created"`
	Updated   string   `json:"updated"`
	History   []Event  `json:"history"`
}

// Event es una entrada del historial de la tarea: un movimiento o un comentario clave.
type Event struct {
	At     string `json:"at"`
	By     string `json:"by"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Role   string `json:"role,omitempty"`
	Note   string `json:"note,omitempty"`
	Reason string `json:"reason,omitempty"`
	Tag    string `json:"tag,omitempty"`  // comentario: decision | pregunta | respuesta | riesgo | acuerdo | nota
	Text   string `json:"text,omitempty"` // comentario: el mensaje
}

// Tipos de comentario clave. No es un chat: cada comentario deja constancia de algo
// que otro rol (o una persona) necesita saber.
var commentTags = map[string]string{
	"decision":  "Decisión",
	"pregunta":  "Pregunta",
	"respuesta": "Respuesta",
	"riesgo":    "Riesgo",
	"acuerdo":   "Acuerdo",
	"nota":      "Nota",
	// Del moderador: lo comprobó por su cuenta, o algo no se sostiene.
	"verificado": "Verificado",
	"alerta":     "Alerta",
}

// NormalizeTag valida el tipo de comentario.
func NormalizeTag(tag string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(tag))
	switch t {
	case "", "comentario", "comment":
		t = "nota"
	case "decisión", "decision", "decided":
		t = "decision"
	case "question":
		t = "pregunta"
	case "answer":
		t = "respuesta"
	case "risk":
		t = "riesgo"
	case "agreement", "consenso":
		t = "acuerdo"
	case "verified", "verificada", "comprobado":
		t = "verificado"
	case "alert", "warning":
		t = "alerta"
	}
	if _, ok := commentTags[t]; !ok {
		return "", fmt.Errorf("tag %q no válido (decision, pregunta, respuesta, riesgo, acuerdo, nota, verificado o alerta)", tag)
	}
	return t, nil
}

// TagLabel nombre visible del tipo de comentario.
func TagLabel(tag string) string { return commentTags[tag] }

// MaxCommentLen limita cada comentario: un comentario clave cabe en pocas líneas.
const MaxCommentLen = 600

// Comment agrega un comentario clave a la tarea (no cambia su estado ni su dueño).
func (t *Task) Comment(by, tag, text string, now time.Time) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("comment requiere text")
	}
	if r := []rune(text); len(r) > MaxCommentLen {
		return fmt.Errorf("comentario de %d caracteres (máximo %d): resume la conclusión", len(r), MaxCommentLen)
	}
	tg, err := NormalizeTag(tag)
	if err != nil {
		return err
	}
	// «verificado» y «alerta» son del moderador: si quien hizo el trabajo se los pone a sí
	// mismo, dejan de significar algo (visto en uso real: un Dev se marcaba «verificado»).
	if (tg == "verificado" || tg == "alerta") && RoleOf(by) != RoleModerator {
		return fmt.Errorf("solo el moderador usa «%s»; firma como «moderador · …» o deja una nota con tu evidencia", TagLabel(tg))
	}
	t.touch(now, Event{By: by, Tag: tg, Text: text, Role: t.Role})
	return nil
}

// IsComment indica si el evento es un comentario (no un movimiento).
func (e Event) IsComment() bool { return e.Text != "" }

// Transiciones permitidas. "en_revision" la toma el siguiente rol (tester o PO);
// "rechazada" vuelve a quien corresponda con el motivo.
var transitions = map[string][]string{
	Pending:    {InProgress},
	InProgress: {Review, Pending},
	Review:     {Done, Rejected, InProgress, Review}, // Review→Review: Tester la aprueba y la pasa al PO
	Rejected:   {InProgress},
	Done:       {},
}

// NormalizeRole valida y normaliza un rol.
func NormalizeRole(r string) (string, error) {
	r = strings.ToLower(strings.TrimSpace(r))
	switch r {
	case "developer", "desarrollo", "desarrollador":
		r = RoleDev
	case "qa", "test", "testing", "pruebas":
		r = RoleTester
	case "product owner", "product-owner", "productowner":
		r = RolePO
	case "ux", "ui", "ui/ux", "ui-ux", "design", "diseno", "diseño", "designer", "diseñador":
		r = RoleUX
	case "user", "humano", "persona":
		r = RoleUser
	}
	if !validRoles[r] {
		return "", fmt.Errorf("rol %q no válido (usa dev, tester, po o ux)", r)
	}
	return r, nil
}

// NewTask crea una tarea pendiente.
func NewTask(id, project, title, detail, role, by string, criteria, deps []string, now time.Time) (*Task, error) {
	if strings.TrimSpace(title) == "" {
		return nil, errors.New("falta title")
	}
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("falta project")
	}
	if role == "" {
		role = RoleDev
	}
	r, err := NormalizeRole(role)
	if err != nil {
		return nil, err
	}
	ts := now.UTC().Format(TimeLayout)
	return &Task{
		ID: id, Project: strings.TrimSpace(project), Title: strings.TrimSpace(title), Detail: strings.TrimSpace(detail),
		Criteria: clean(criteria), Role: r, Status: Pending, DependsOn: clean(deps),
		Created: ts, Updated: ts, History: []Event{{At: ts, By: by, To: Pending, Role: r, Note: "creada"}},
	}, nil
}

func clean(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// RoleModerator es el rol del moderador: vigila el tablero, no toma tareas.
const RoleModerator = "moderador"

// RoleOf saca el rol del actor de su firma ("tester · genius" → tester). "" si no lo dice.
func RoleOf(by string) string {
	first := strings.ToLower(strings.TrimSpace(by))
	if i := strings.IndexAny(first, "·-:/ "); i >= 0 {
		first = strings.TrimSpace(first[:i])
	}
	switch first {
	case "moderador", "moderator", "mod", "sm":
		return RoleModerator
	}
	if r, err := NormalizeRole(first); err == nil {
		return r
	}
	return ""
}

func roleName(r string) string {
	if r == RoleModerator {
		return "Moderador"
	}
	return roleLabel(r)
}

// Claim asigna la tarea a un agente y la pone en curso. Exige que sus dependencias estén
// hechas y que la tarea le toque al rol de quien la toma.
func (t *Task) Claim(by string, deps map[string]*Task, now time.Time) error {
	if strings.TrimSpace(by) == "" {
		return errors.New("falta owner (quién la toma)")
	}
	switch actor := RoleOf(by); {
	case actor == RoleModerator:
		return fmt.Errorf("el moderador no toma tareas: vigila que cada rol tome la suya (%s le toca a %s)", t.ID, roleLabel(t.Role))
	case actor != "" && actor != t.Role:
		return fmt.Errorf("%s le toca a %s y tú eres %s: toma una tarea de tu rol (board_list con role)", t.ID, roleLabel(t.Role), roleName(actor))
	}
	if t.Status != Pending && t.Status != Rejected && t.Status != Review {
		return fmt.Errorf("la tarea %s está %s; solo se toman tareas pendientes, rechazadas o en revisión", t.ID, t.Status)
	}
	for _, d := range t.DependsOn {
		if dep, ok := deps[d]; !ok || dep.Status != Done {
			return fmt.Errorf("la tarea %s depende de %s, que aún no está hecha", t.ID, d)
		}
	}
	from := t.Status
	t.Owner = by
	if from == Review {
		// El revisor (tester o PO) la toma para revisarla: sigue en revisión, cambia el dueño.
		t.touch(now, Event{By: by, From: from, To: Review, Role: t.Role, Note: "tomada para revisar"})
		return nil
	}
	t.Status = InProgress
	t.touch(now, Event{By: by, From: from, To: InProgress, Role: t.Role, Note: "tomada"})
	return nil
}

// Move cambia el estado. Pasar a revisión, hecha o rechazada exige evidencia o motivo.
// nextRole indica a qué rol le toca después (p. ej. dev → tester al pasar a revisión).
func (t *Task) Move(by, to, nextRole, evidence, reason string, now time.Time) error {
	to = strings.ToLower(strings.TrimSpace(to))
	if !contains(transitions[t.Status], to) {
		return fmt.Errorf("no se puede pasar de %s a %s (permitido: %s)", t.Status, to, strings.Join(transitions[t.Status], ", "))
	}
	if (to == Review || to == Done) && strings.TrimSpace(evidence) == "" {
		return fmt.Errorf("pasar a %s exige evidencia (prueba corrida, enlace al reporte, archivo:línea)", to)
	}
	// Nadie aprueba su propio trabajo: quien lo entregó a revisión no puede pasarlo al
	// siguiente rol ni cerrarlo (el agente que afirma algo no es quien lo confirma).
	if t.Status == Review && (to == Done || to == Review) {
		if sub := t.submitter(); sub != "" && sameActor(sub, by) {
			return fmt.Errorf("%s no puede aprobar su propio trabajo en %s: lo revisa otro rol", by, t.ID)
		}
	}
	// Cada quien mueve lo suyo.
	actor := RoleOf(by)
	switch {
	case actor == RoleModerator:
		// El moderador solo corrige: libera una tarea mal tomada o rechaza lo que no se sostiene.
		if !(t.Status == InProgress && to == Pending) && !(t.Status == Review && to == Rejected) {
			return fmt.Errorf("el moderador solo libera tareas en curso (→ pendiente) o rechaza las que están en revisión; %s → %s le toca a %s", ColumnLabel(t.Status), ColumnLabel(to), roleLabel(t.Role))
		}
	case t.Status == InProgress && t.Owner != "":
		if !sameActor(by, t.Owner) && (actor == "" || actor != RoleOf(t.Owner)) {
			return fmt.Errorf("%s la tiene %s: solo su dueño la mueve (o el moderador la libera)", t.ID, t.Owner)
		}
	case actor != "" && actor != t.Role:
		return fmt.Errorf("%s le toca a %s y tú eres %s: no puedes moverla", t.ID, roleLabel(t.Role), roleName(actor))
	}
	if to == Rejected && strings.TrimSpace(reason) == "" {
		return errors.New("rechazar exige el motivo (qué criterio no se cumple y cómo reproducirlo)")
	}
	if nextRole != "" {
		r, err := NormalizeRole(nextRole)
		if err != nil {
			return err
		}
		t.Role = r
	} else if to == Review && (t.Role == RoleDev || t.Role == RoleUX) {
		t.Role = RoleTester // por defecto, lo que entrega Dev o UX lo revisa Tester
	} else if to == Rejected {
		if RoleOf(t.submitter()) == RoleUX {
			t.Role = RoleUX
		} else {
			t.Role = RoleDev // por defecto, lo rechazado vuelve a Dev
		}
	}
	if evidence != "" {
		t.Evidence = strings.TrimSpace(evidence)
	}
	from := t.Status
	t.Status = to
	if to == Pending || to == Rejected || to == Review {
		t.Owner = "" // queda libre para que la tome el siguiente rol
	}
	t.touch(now, Event{By: by, From: from, To: to, Role: t.Role, Note: strings.TrimSpace(evidence), Reason: strings.TrimSpace(reason)})
	return nil
}

func (t *Task) touch(now time.Time, ev Event) {
	ts := now.UTC().Format(TimeLayout)
	ev.At = ts
	t.Updated = ts
	t.History = append(t.History, ev)
	if len(t.History) > 100 {
		t.History = t.History[len(t.History)-100:]
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Columns es el orden de columnas del tablero.
var Columns = []string{Pending, InProgress, Review, Rejected, Done}

// ColumnLabel nombre visible de cada estado.
func ColumnLabel(s string) string {
	switch s {
	case Pending:
		return "Pendiente"
	case InProgress:
		return "En curso"
	case Review:
		return "En revisión"
	case Rejected:
		return "Rechazada"
	case Done:
		return "Hecha"
	}
	return s
}

// SortTasks ordena por última actualización (más reciente primero) dentro de cada columna.
func SortTasks(ts []*Task) {
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].Updated > ts[j].Updated })
}
