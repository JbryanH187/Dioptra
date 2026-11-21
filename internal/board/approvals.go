package board

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Aprobaciones del usuario. En modo manual (por defecto) ningún agente cierra una tarea:
// la aprobación del PO la deja «le toca a Usuario» y la persona la cierra marcando
// «Aprobar Tn» en el tablero de Dioptra. El modo automático también es una casilla que
// solo la persona marca. Ningún agente puede firmar como usuario.

// AutoModeText es el texto de la casilla que activa el modo automático.
const AutoModeText = "Modo automático: los agentes cierran las tareas sin esperar mi aprobación"

// UserActor firma las aprobaciones hechas en el visor.
const UserActor = "usuario · visor de Dioptra"

// UserApprovalEvidence es la evidencia que queda al aprobar desde el visor.
const UserApprovalEvidence = "Aprobada por el usuario en Dioptra (casilla del tablero)"

// CheckReader lee las casillas marcadas del tablero publicado (texto normalizado → true).
type CheckReader interface {
	BoardChecks(ctx context.Context, project string) map[string]bool
}

var trailingSev = regexp.MustCompile(`\s*\[(?:info|warning|error|success|ok)\]\s*$`)

// NormCheck reduce el texto de una casilla a una clave comparable.
func NormCheck(s string) string {
	s = trailingSev.ReplaceAllString(s, "")
	s = strings.NewReplacer("*", "", "`", "", "_", "").Replace(s)
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func approveText(t *Task) string { return fmt.Sprintf("Aprobar %s · %s", t.ID, one(t.Title)) }

func (s *Service) checks(ctx context.Context, project string) map[string]bool {
	if cr, ok := s.Pub.(CheckReader); ok && s.Pub != nil {
		if m := cr.BoardChecks(ctx, project); m != nil {
			return m
		}
	}
	return map[string]bool{}
}

func autoMode(checks map[string]bool) bool { return checks[NormCheck(AutoModeText)] }

// approvalsSection es la sección «Tus aprobaciones» del tablero.
func approvalsSection(tasks []*Task, auto bool) string {
	var waiting []*Task
	for _, t := range tasks {
		if t.Status == Review && t.Role == RoleUser {
			waiting = append(waiting, t)
		}
	}
	var b strings.Builder
	b.WriteString("## Tus aprobaciones\n\n")
	if auto {
		b.WriteString("Modo **automático**: la aprobación del PO cierra la tarea. Desmarca la casilla para volver a aprobar tú cada una.\n\n")
	} else {
		b.WriteString("Modo **manual**: ninguna tarea se cierra sin ti. Cuando el PO aprueba, la tarea llega aquí; márcala para cerrarla.\n\n")
	}
	mark := " "
	if auto {
		mark = "x"
	}
	fmt.Fprintf(&b, "- [%s] %s\n", mark, AutoModeText)
	for _, t := range waiting {
		fmt.Fprintf(&b, "- [ ] %s\n", approveText(t))
	}
	b.WriteString("\n")
	if len(waiting) == 0 {
		b.WriteString("_No hay tareas esperando tu aprobación._\n\n")
		return b.String()
	}
	b.WriteString("| Tarea | Lo que verificaron | Criterios |\n|---|---|---|\n")
	for _, t := range waiting {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", t.ID, cell(short(t.Evidence, 200)), cell(strings.Join(t.Criteria, "; ")))
	}
	b.WriteString("\n")
	return b.String()
}

// withApprovals inserta la sección de aprobaciones justo después de los KPI.
func withApprovals(md string, tasks []*Task, auto bool) string {
	start := strings.Index(md, "```kpi")
	if start < 0 {
		return md
	}
	end := strings.Index(md[start+6:], "```\n\n")
	if end < 0 {
		return md
	}
	cut := start + 6 + end + len("```\n\n")
	return md[:cut] + approvalsSection(tasks, auto) + md[cut:]
}

// SyncApprovals cierra las tareas cuya casilla «Aprobar Tn» marcó la persona y vuelve a
// publicar el tablero (también refleja si cambió el modo). Devuelve cuántas cerró.
func (s *Service) SyncApprovals(ctx context.Context, project string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, _, err := s.load(ctx, project)
	if err != nil {
		return 0, err
	}
	checks := s.checks(ctx, project)
	closed := 0
	for _, t := range list {
		if t.Status != Review || t.Role != RoleUser || !checks[NormCheck(approveText(t))] {
			continue
		}
		if err := t.Move(UserActor, Done, "", UserApprovalEvidence, "", s.now()); err != nil {
			return closed, err
		}
		if err := s.save(ctx, t); err != nil {
			return closed, err
		}
		closed++
	}
	if len(list) > 0 {
		s.publishWith(ctx, project, list, autoMode(checks))
	}
	return closed, nil
}
