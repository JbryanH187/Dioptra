package board

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Store persiste las tareas como documento JSON por (proyecto, id).
type Store interface {
	SaveBoardTask(ctx context.Context, projectKey, id, status string, doc []byte) error
	LoadBoardTasks(ctx context.Context, projectKey string) ([][]byte, error)
}

// Publisher publica el tablero de un proyecto como reporte de Dioptra.
type Publisher interface {
	PublishBoard(ctx context.Context, project, markdown string) (reportID, url string, err error)
}

// Input es lo que envía un agente con `board_task`.
type Input struct {
	Action    string   `json:"action"` // create | claim | move | comment
	Project   string   `json:"project"`
	ID        string   `json:"id,omitempty"`
	Title     string   `json:"title,omitempty"`
	Detail    string   `json:"detail,omitempty"`
	Criteria  []string `json:"criteria,omitempty"`
	Role      string   `json:"role,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
	By        string   `json:"by,omitempty"`
	To        string   `json:"to,omitempty"`
	NextRole  string   `json:"next_role,omitempty"`
	Evidence  string   `json:"evidence,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Tag       string   `json:"tag,omitempty"`
	Text      string   `json:"text,omitempty"`
	AsRole    string   `json:"as_role,omitempty"`
}

// Result es la respuesta al agente.
type Result struct {
	Task     *Task   `json:"task,omitempty"`
	Tasks    []*Task `json:"tasks,omitempty"`
	ReportID string  `json:"report_id,omitempty"`
	URL      string  `json:"url,omitempty"`
	Alerts   []Alert `json:"alerts,omitempty"` // del moderador automático (board_list)
	Message  string  `json:"message"`
}

// Service coordina el tablero.
type Service struct {
	Store   Store
	Pub     Publisher
	Now     func() time.Time
	KeyOf   func(project string) string // clave canónica del proyecto
	Files   FileCheck                   // verifica archivo:línea de la evidencia (opcional)
	mu      sync.Mutex
	reports map[string][2]string // proyecto → (reportID, url)
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) key(project string) string {
	if s.KeyOf != nil {
		return s.KeyOf(project)
	}
	return strings.ToLower(strings.TrimSpace(project))
}

func (s *Service) load(ctx context.Context, project string) ([]*Task, map[string]*Task, error) {
	docs, err := s.Store.LoadBoardTasks(ctx, s.key(project))
	if err != nil {
		return nil, nil, err
	}
	list := make([]*Task, 0, len(docs))
	byID := map[string]*Task{}
	for _, d := range docs {
		var t Task
		if json.Unmarshal(d, &t) == nil {
			list = append(list, &t)
			byID[t.ID] = &t
		}
	}
	return list, byID, nil
}

func (s *Service) save(ctx context.Context, t *Task) error {
	doc, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return s.Store.SaveBoardTask(ctx, s.key(t.Project), t.ID, t.Status, doc)
}

func (s *Service) publish(ctx context.Context, project string, list []*Task) (string, string) {
	return s.publishWith(ctx, project, list, autoMode(s.checks(ctx, project)))
}

func (s *Service) publishWith(ctx context.Context, project string, list []*Task, auto bool) (string, string) {
	if s.Pub == nil {
		return "", ""
	}
	if s.reports == nil {
		s.reports = map[string][2]string{}
	}
	alerts := Audit(list, s.now(), s.Files)
	md := withApprovals(Markdown(project, list, alerts...), list, auto)
	// El tablero enlaza al chat (si ya se publicó antes) y el chat enlaza al tablero.
	if chat, ok := s.reports["chat:"+s.key(project)]; ok && chat[1] != "" {
		md = strings.Replace(md, boardIntro, boardIntro+" La conversación completa está en el [chat del equipo]("+chat[1]+").", 1)
	}
	id, url, err := s.Pub.PublishBoard(ctx, project, md)
	if err != nil {
		return "", ""
	}
	s.reports[s.key(project)] = [2]string{id, url}
	if cp, ok := s.Pub.(ChatPublisher); ok {
		if cid, curl, err := cp.PublishChat(ctx, project, ChatMarkdown(project, list, alerts, url)); err == nil {
			s.reports["chat:"+s.key(project)] = [2]string{cid, curl}
		}
	}
	return id, url
}

// ChatPublisher es opcional: publica la bitácora tipo chat del equipo.
type ChatPublisher interface {
	PublishChat(ctx context.Context, project, markdown string) (reportID, url string, err error)
}

// List devuelve las tareas del proyecto (opcionalmente de un rol y/o estado).
func (s *Service) List(ctx context.Context, project, role, status string) (*Result, error) {
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("falta project")
	}
	list, _, err := s.load(ctx, project)
	if err != nil {
		return nil, err
	}
	if role != "" {
		if role, err = NormalizeRole(role); err != nil {
			return nil, err
		}
	}
	out := []*Task{}
	for _, t := range list {
		if (role == "" || t.Role == role) && (status == "" || t.Status == status) {
			out = append(out, t)
		}
	}
	SortTasks(out)
	res := &Result{Tasks: out, Alerts: Audit(list, s.now(), s.Files), Message: fmt.Sprintf("%d tarea(s)", len(out))}
	s.mu.Lock()
	if r, ok := s.reports[s.key(project)]; ok {
		res.ReportID, res.URL = r[0], r[1]
	}
	s.mu.Unlock()
	return res, nil
}

// Do aplica una acción (create | claim | move) y vuelve a publicar el tablero.
func (s *Service) Do(ctx context.Context, in Input) (*Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, byID, err := s.load(ctx, in.Project)
	if err != nil {
		return nil, err
	}
	now := s.now()
	by := strings.TrimSpace(in.By)
	if by == "" {
		by = "agente"
	}
	// as_role: el rol de quien actúa. Si su firma no lo dice, se antepone ("tester · genius").
	if r := strings.TrimSpace(in.AsRole); r != "" {
		role := RoleOf(r)
		if role == "" {
			return nil, fmt.Errorf("as_role %q no válido (dev, tester, po o moderador)", r)
		}
		switch signed := RoleOf(by); {
		case signed == "":
			by = role + " · " + by
		case signed != role:
			return nil, fmt.Errorf("by dice %s pero as_role dice %s: usa un solo rol", roleName(signed), roleName(role))
		}
	}
	// Nadie firma como la persona: sus aprobaciones solo se hacen en el tablero de Dioptra.
	if RoleOf(by) == RoleUser {
		return nil, errors.New("ningún agente actúa como usuario: el usuario aprueba marcando «Aprobar Tn» en el tablero de Dioptra")
	}
	var t *Task
	var msg string
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "create":
		if r, _ := NormalizeRole(in.Role); r == RoleUser {
			return nil, errors.New("no se crean tareas para el usuario: llegan a él cuando el PO las aprueba")
		}
		for _, d := range in.DependsOn {
			if _, ok := byID[strings.TrimSpace(d)]; !ok {
				return nil, fmt.Errorf("depends_on: no existe la tarea %s en %s", d, in.Project)
			}
		}
		t, err = NewTask(fmt.Sprintf("T%d", len(list)+1), in.Project, in.Title, in.Detail, in.Role, by, in.Criteria, in.DependsOn, now)
		if err != nil {
			return nil, err
		}
		list = append(list, t)
		msg = fmt.Sprintf("Tarea %s creada para %s.", t.ID, roleLabel(t.Role))
	case "comment":
		if t = byID[strings.TrimSpace(in.ID)]; t == nil {
			return nil, fmt.Errorf("no existe la tarea %q en %s (usa board_list)", in.ID, in.Project)
		}
		if err = t.Comment(by, in.Tag, in.Text, now); err != nil {
			return nil, err
		}
		tag, _ := NormalizeTag(in.Tag)
		msg = fmt.Sprintf("%s registrado en %s.", TagLabel(tag), t.ID)
	case "claim", "move":
		if t = byID[strings.TrimSpace(in.ID)]; t == nil {
			return nil, fmt.Errorf("no existe la tarea %q en %s (usa board_list)", in.ID, in.Project)
		}
		if strings.EqualFold(in.Action, "claim") {
			err = t.Claim(by, byID, now)
			msg = fmt.Sprintf("%s es tuya (%s).", t.ID, ColumnLabel(t.Status))
		} else {
			to, next := in.To, in.NextRole
			if r, _ := NormalizeRole(next); r == RoleUser && strings.EqualFold(strings.TrimSpace(to), Done) {
				next = ""
			}
			manual := !autoMode(s.checks(ctx, in.Project))
			if manual && strings.EqualFold(strings.TrimSpace(to), Done) {
				// Modo manual: el cierre lo hace la persona. La aprobación del PO deja la tarea
				// esperando su casilla «Aprobar Tn» en el tablero.
				to, next = Review, RoleUser
			}
			err = t.Move(by, to, next, in.Evidence, in.Reason, now)
			if err == nil && manual && t.Role == RoleUser && t.Status == Review {
				msg = fmt.Sprintf("%s aprobada por %s; queda esperando la aprobación del usuario en Dioptra (modo manual).", t.ID, roleName(RoleOf(by)))
			} else {
				msg = fmt.Sprintf("%s → %s; ahora le toca a %s.", t.ID, ColumnLabel(t.Status), roleLabel(t.Role))
			}
		}
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("action %q no válida (create, claim, move o comment)", in.Action)
	}
	if err := s.save(ctx, t); err != nil {
		return nil, err
	}
	id, url := s.publish(ctx, t.Project, list)
	return &Result{Task: t, ReportID: id, URL: url, Message: msg}, nil
}
