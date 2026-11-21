package board

import (
	"context"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)

func TestTask_FlujoYReglas(t *testing.T) {
	tk, err := NewTask("T1", "abonos", "Validar NSF en abonos", "", "", "po", []string{"Rechaza con 422 si saldo < monto"}, nil, t0)
	if err != nil || tk.Role != RoleDev || tk.Status != Pending {
		t.Fatalf("NewTask = %+v, %v", tk, err)
	}
	if err := tk.Move("dev", Review, "", "npm test ok", "", t0); err == nil {
		t.Error("no se puede saltar de pendiente a revisión")
	}
	if err := tk.Claim("dev · genius", nil, t0); err != nil || tk.Status != InProgress || tk.Owner != "dev · genius" {
		t.Fatalf("Claim: %v %+v", err, tk)
	}
	if err := tk.Move("dev", Review, "", "", "", t0); err == nil || !strings.Contains(err.Error(), "evidencia") {
		t.Errorf("revisión sin evidencia: %v", err)
	}
	if err := tk.Move("dev", Review, "", "go test ./... ok (12 tests)", "", t0); err != nil || tk.Role != RoleTester || tk.Owner != "" {
		t.Fatalf("a revisión pasa a Tester y queda libre: %v %+v", err, tk)
	}
	if err := tk.Claim("tester · genius", nil, t0); err != nil || tk.Status != Review || tk.Owner != "tester · genius" {
		t.Fatalf("el tester la toma sin sacarla de revisión: %v %+v", err, tk)
	}
	if err := tk.Move("tester", Rejected, "", "", "", t0); err == nil {
		t.Error("rechazar exige motivo")
	}
	if err := tk.Move("tester", Rejected, "", "", "Con saldo 0 responde 500, no 422", t0); err != nil || tk.Role != RoleDev {
		t.Fatalf("rechazada vuelve a Dev: %v %+v", err, tk)
	}
	if err := tk.Claim("dev", nil, t0); err != nil {
		t.Fatal(err)
	}
	tk.Move("dev", Review, RolePO, "arreglado: 422 con saldo 0 (test agregado)", "", t0)
	if tk.Role != RolePO {
		t.Errorf("next_role manda: %s", tk.Role)
	}
	if err := tk.Move("po", Done, "", "criterio validado en QA con usuario de prueba", "", t0); err != nil || tk.Status != Done {
		t.Fatalf("hecha: %v", err)
	}
	if err := tk.Move("po", InProgress, "", "", "", t0); err == nil {
		t.Error("una tarea hecha no se reabre")
	}
	if len(tk.History) != 8 { // los movimientos rechazados no dejan rastro
		t.Errorf("historial = %d eventos", len(tk.History))
	}
}

func TestTask_DependenciasYRoles(t *testing.T) {
	a, _ := NewTask("T1", "p", "API", "", "dev", "po", nil, nil, t0)
	b, _ := NewTask("T2", "p", "Pantalla", "", "desarrollador", "po", nil, []string{"T1"}, t0)
	if err := b.Claim("dev", map[string]*Task{"T1": a}, t0); err == nil || !strings.Contains(err.Error(), "depende de T1") {
		t.Errorf("no se toma con dependencias pendientes: %v", err)
	}
	a.Status = Done
	if err := b.Claim("dev", map[string]*Task{"T1": a}, t0); err != nil {
		t.Error(err)
	}
	if r, err := NormalizeRole("QA"); err != nil || r != RoleTester {
		t.Errorf("QA → tester: %s %v", r, err)
	}
	if _, err := NormalizeRole("arquitecto"); err == nil {
		t.Error("rol inválido")
	}
	if _, err := NewTask("T3", "p", " ", "", "", "x", nil, nil, t0); err == nil {
		t.Error("sin título")
	}
}

func TestMarkdown_Tablero(t *testing.T) {
	a, _ := NewTask("T1", "abonos", "API de NSF", "", "dev", "po", []string{"422 si saldo < monto"}, nil, t0)
	b, _ := NewTask("T2", "abonos", "Pantalla de rechazo", "", "dev", "po", nil, []string{"T1"}, t0)
	a.Claim("dev · genius", nil, t0)
	a.Move("dev", Review, "", "go test ok", "", t0)
	a.Claim("tester", nil, t0)
	a.Move("tester", Rejected, "", "", "saldo 0 da 500", t0)
	md := Markdown("abonos", []*Task{a, b})
	for _, want := range []string{
		"# Tablero · abonos", "```kpi", "Rechazadas | 1 | | | error", "## Carga por rol", "| Dev | 1 | 0 | 0 | 1 |",
		"```mermaid\nflowchart LR", "t_T1 --> t_T2", "## Rechazada", "> [!ERROR] T1 rechazada\n> saldo 0 da 500",
		"<details><summary>T1 · API de NSF (Rechazada)</summary>", "- 422 si saldo &lt; monto",
	} {
		if !strings.Contains(md, strings.ReplaceAll(want, "&lt;", "<")) && !strings.Contains(md, want) {
			t.Errorf("falta %q en:\n%s", want, md)
		}
	}
	if empty := Markdown("x", nil); !strings.Contains(empty, "El tablero está vacío") {
		t.Error("tablero vacío con indicación")
	}
}

type memStore map[string][][]byte

func (m memStore) SaveBoardTask(_ context.Context, key, id, _ string, doc []byte) error {
	list := m[key]
	for i, d := range list {
		if strings.Contains(string(d), `"id":"`+id+`"`) {
			list[i] = doc
			return nil
		}
	}
	m[key] = append(list, doc)
	return nil
}

func (m memStore) LoadBoardTasks(_ context.Context, key string) ([][]byte, error) { return m[key], nil }

type memPub struct{ n int }

func (p *memPub) PublishBoard(context.Context, string, string) (string, string, error) {
	p.n++
	return "r_board", "http://127.0.0.1:7474/?report=r_board", nil
}

func TestService_CreaTomaMueveYPublica(t *testing.T) {
	ctx := context.Background()
	pub := &memPub{}
	s := &Service{Store: memStore{}, Pub: pub, Now: func() time.Time { return t0 }}
	r, err := s.Do(ctx, Input{Action: "create", Project: "Abonos", Title: "API NSF", Criteria: []string{"422"}, By: "po"})
	if err != nil || r.Task.ID != "T1" || r.URL == "" {
		t.Fatalf("create: %+v %v", r, err)
	}
	if _, err := s.Do(ctx, Input{Action: "create", Project: "abonos", Title: "UI", DependsOn: []string{"T9"}}); err == nil {
		t.Error("dependencia inexistente")
	}
	s.Do(ctx, Input{Action: "create", Project: "abonos", Title: "UI", DependsOn: []string{"T1"}})
	if _, err := s.Do(ctx, Input{Action: "claim", Project: "abonos", ID: "T2", By: "dev"}); err == nil {
		t.Error("T2 depende de T1")
	}
	if _, err := s.Do(ctx, Input{Action: "claim", Project: "abonos", ID: "T1", By: "dev"}); err != nil {
		t.Fatal(err)
	}
	mv, err := s.Do(ctx, Input{Action: "move", Project: "abonos", ID: "T1", To: Review, Evidence: "tests ok", By: "dev"})
	if err != nil || !strings.Contains(mv.Message, "le toca a Tester") {
		t.Fatalf("move: %+v %v", mv, err)
	}
	list, _ := s.List(ctx, "abonos", "tester", "")
	if len(list.Tasks) != 1 || list.Tasks[0].ID != "T1" || list.URL == "" {
		t.Errorf("lista del tester: %+v", list)
	}
	if _, err := s.Do(ctx, Input{Action: "borrar", Project: "abonos"}); err == nil {
		t.Error("acción inválida")
	}
	if pub.n != 4 {
		t.Errorf("publicaciones = %d (una por cambio válido)", pub.n)
	}
}

func TestComentarios_DecisionesPreguntasYConversacion(t *testing.T) {
	a, _ := NewTask("T1", "abonos", "API NSF", "", "dev", "po", nil, nil, t0)
	b, _ := NewTask("T2", "abonos", "Pantalla", "", "dev", "po", nil, nil, t0)
	at := func(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }
	if err := a.Comment("dev", "pregunta", "", at(1)); err == nil {
		t.Error("comentario vacío")
	}
	if err := a.Comment("dev", "chisme", "x", at(1)); err == nil {
		t.Error("tag inválido")
	}
	if err := a.Comment("dev", "pregunta", strings.Repeat("x", MaxCommentLen+1), at(1)); err == nil {
		t.Error("comentario demasiado largo")
	}
	a.Comment("dev · genius", "pregunta", "¿Saldo 0 cuenta como NSF o como cuenta inactiva?", at(1))
	b.Comment("tester · genius", "question", "¿La pantalla debe permitir reintentar?", at(2))
	a.Comment("po · genius", "respuesta", "Saldo 0 es NSF: responde 422", at(3))
	a.Comment("po · genius", "decisión", "NSF = saldo < monto, incluido saldo 0", at(4))
	a.Claim("dev · genius", nil, at(5))
	a.Comment("tester · genius", "acuerdo", "Dev y Tester: el log de rechazo lleva folio y correlación", at(6))

	if open := openQuestions([]*Task{a, b}); len(open) != 1 || open[0].task.ID != "T2" {
		t.Fatalf("solo queda abierta la de T2: %+v", open)
	}
	md := Markdown("abonos", []*Task{a, b})
	for _, want := range []string{
		"Preguntas abiertas | 1 | | | warning",
		"## Decisiones y acuerdos",
		"- **Acuerdo** · T1 · tester · genius",
		"- **Decisión** · T1 · po · genius",
		"## Preguntas abiertas\n\n> [!WARNING] T2 · tester · genius pregunta\n> ¿La pantalla debe permitir reintentar?",
		"## Conversación del equipo",
		"| T1 | Pregunta | ¿Saldo 0 cuenta como NSF o como cuenta inactiva? |",
		"| T1 | → En curso | tomada |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("falta %q en:\n%s", want, md)
		}
	}
	if strings.Index(md, "**Acuerdo**") > strings.Index(md, "**Decisión**") {
		t.Error("las decisiones y acuerdos van de la más reciente a la más antigua")
	}
	if strings.Index(md, "Saldo 0 cuenta como NSF") > strings.Index(md, "Saldo 0 es NSF") {
		t.Error("la conversación va en orden cronológico")
	}
}

func TestConversacion_OrdenExactoEnElMismoSegundo(t *testing.T) {
	base := time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)
	a, _ := NewTask("T1", "p", "A", "", "dev", "po", nil, nil, base)
	b, _ := NewTask("T2", "p", "B", "", "dev", "po", nil, nil, base.Add(100*time.Millisecond))
	a.Claim("dev", nil, base.Add(200*time.Millisecond))
	items := feed([]*Task{a, b})
	got := []string{}
	for _, it := range items {
		got = append(got, it.task.ID+":"+it.ev.To)
	}
	if strings.Join(got, ",") != "T1:pendiente,T2:pendiente,T1:en_curso" {
		t.Errorf("orden = %v", got)
	}
	if ts := a.History[0].At; ts != "2026-11-20T18:00:00.000000Z" {
		t.Errorf("formato de hora = %s", ts)
	}
}

func TestService_AsRoleFirmaYValida(t *testing.T) {
	ctx := context.Background()
	s := &Service{Store: memStore{}, Now: func() time.Time { return t0 }}
	s.Do(ctx, Input{Action: "create", Project: "p", Title: "API", By: "po"})
	r, err := s.Do(ctx, Input{Action: "claim", Project: "p", ID: "T1", By: "genius", AsRole: "dev"})
	if err != nil || r.Task.Owner != "dev · genius" {
		t.Fatalf("as_role firma la acción: %+v %v", r, err)
	}
	if _, err := s.Do(ctx, Input{Action: "comment", Project: "p", ID: "T1", By: "tester · x", AsRole: "dev", Text: "hola"}); err == nil {
		t.Error("firma y as_role contradictorios")
	}
	if _, err := s.Do(ctx, Input{Action: "claim", Project: "p", ID: "T1", By: "x", AsRole: "arquitecto"}); err == nil {
		t.Error("as_role inválido")
	}
}

func TestChatMarkdown_HilosLecturaYGrounding(t *testing.T) {
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	a, _ := NewTask("T1", "abonos", "API NSF", "", "dev", "po · genius", []string{"422"}, nil, at(0))
	b, _ := NewTask("T2", "abonos", "Pantalla", "", "dev", "po · genius", nil, nil, at(0))
	a.Comment("dev · genius", "pregunta", "¿saldo 0 es NSF?", at(1))
	a.Comment("po · genius", "respuesta", "Sí, 422.", at(2))
	a.Comment("po · genius", "decision", "NSF incluye saldo 0", at(3))
	a.Claim("dev · genius", nil, at(4))
	a.Move("dev · genius", Review, "", "go test ok (14 tests) · internal/abonos/handler.go:57", "", at(5))
	a.Claim("tester · genius", nil, at(6))
	a.Move("tester · genius", Rejected, "", "", "saldo 0 da 500", at(7))
	a.Comment("moderador · genius", "alerta", "La evidencia no cubre saldo 0", at(8))
	a.Comment("dev · genius", "acuerdo", "Se agrega test de saldo 0", at(9))
	b.Comment("tester · genius", "pregunta", "¿se puede reintentar?", at(10))

	md := ChatMarkdown("abonos", []*Task{b, a}, Audit([]*Task{a, b}, at(10), nil), "http://x/?report=r_board")
	for _, want := range []string{
		"# Chat del equipo · abonos",
		"[tablero](http://x/?report=r_board)",
		"Mensajes | 6 |",
		"| T1 | Rechazada | 5 | 1 | 1 | 1 | 1 | hubo lluvia de ideas; 1 rechazo(s); llegaron a acuerdo |",
		"| T2 | Pendiente | 1 | 1 | 0 | 0 | 0 | 1 pregunta(s) esperando |",
		"**dev · genius** · " + timeOnly(at(1).Format(TimeLayout)) + " · *Pregunta:* ¿saldo 0 es NSF?",
		"*→ En revisión* con evidencia: `go test ok (14 tests) · internal/abonos/handler.go:57`",
		"**tester · genius** · " + timeOnly(at(7).Format(TimeLayout)) + " · rechazó *(→ Rechazada)*: saldo 0 da 500",
		"> [!ERROR] moderador · genius · moderador marcó una alerta",
		"> [!NOTE] po · genius · Decisión",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("falta %q en:\n%s", want, md)
		}
	}
	if strings.Index(md, "## T1 ·") > strings.Index(md, "## T2 ·") {
		t.Error("los hilos van en orden de tarea (T1, T2…)")
	}
	if !strings.Contains(ChatMarkdown("p", nil, nil, ""), "Todavía no hay conversación") {
		t.Error("chat vacío")
	}
}

type chatPub struct {
	memPub
	chats []string
}

func (p *chatPub) PublishChat(_ context.Context, _ string, md string) (string, string, error) {
	p.chats = append(p.chats, md)
	return "r_chat", "http://x/?report=r_chat", nil
}

func TestService_PublicaChatYEnlaza(t *testing.T) {
	ctx := context.Background()
	pub := &chatPub{}
	s := &Service{Store: memStore{}, Pub: pub, Now: func() time.Time { return t0 }}
	s.Do(ctx, Input{Action: "create", Project: "p", Title: "A"})
	s.Do(ctx, Input{Action: "comment", Project: "p", ID: "T1", Tag: "pregunta", Text: "¿x?", By: "dev"})
	if len(pub.chats) != 2 || !strings.Contains(pub.chats[1], "*Pregunta:* ¿x?") || strings.Contains(pub.chats[1], "con evidencia: `tomada para revisar`") {
		t.Fatalf("chat publicado en cada cambio: %d", len(pub.chats))
	}
}
