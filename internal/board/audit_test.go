package board

import (
	"strings"
	"testing"
	"time"
)

func kinds(alerts []Alert, id string) []string {
	var out []string
	for _, a := range alerts {
		if a.TaskID == id {
			out = append(out, a.Kind)
		}
	}
	return out
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func submitted(t *testing.T, id, evidence string, at time.Time) *Task {
	t.Helper()
	tk, _ := NewTask(id, "abonos", "Tarea "+id, "", "dev", "po", []string{"criterio"}, nil, at)
	tk.Claim("dev · genius", nil, at)
	if err := tk.Move("dev · genius", Review, "", evidence, "", at); err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestAudit_EvidenciaQueNoSeSostiene(t *testing.T) {
	now := t0
	files := func(_ string, path string, line int) (bool, bool, bool) {
		switch path {
		case "internal/abonos/handler.go":
			return true, true, line <= 120
		case "internal/abonos/inventado.go":
			return true, false, false
		}
		return false, false, false
	}
	vague := submitted(t, "T1", "listo, ya quedó funcionando", now)
	noResult := submitted(t, "T2", "corrí go test ./internal/abonos/...", now)
	ghost := submitted(t, "T3", "go test ok (14 tests) · internal/abonos/inventado.go:57", now)
	badLine := submitted(t, "T4", "go test ok · internal/abonos/handler.go:900", now)
	good := submitted(t, "T5", "go test ./internal/abonos/... ok (14 tests) · internal/abonos/handler.go:57", now)
	unknown := submitted(t, "T6", "npm test ok · otro/repo/x.ts:3", now)

	a := Audit([]*Task{vague, noResult, ghost, badLine, good, unknown}, now, files)
	if !has(kinds(a, "T1"), "evidencia_no_verificable") {
		t.Errorf("T1: %v", kinds(a, "T1"))
	}
	if !has(kinds(a, "T2"), "evidencia_sin_resultado") {
		t.Errorf("T2: %v", kinds(a, "T2"))
	}
	if !has(kinds(a, "T3"), "evidencia_inexistente") || !has(kinds(a, "T4"), "evidencia_inexistente") {
		t.Errorf("T3/T4 deben marcar evidencia inexistente: %v %v", kinds(a, "T3"), kinds(a, "T4"))
	}
	if len(kinds(a, "T5")) != 0 || len(kinds(a, "T6")) != 0 {
		t.Errorf("evidencia correcta o no verificable por falta de raíz no alerta: %v %v", kinds(a, "T5"), kinds(a, "T6"))
	}
	if a[0].Level != "error" {
		t.Error("los errores van primero")
	}
}

func TestAudit_ConversacionQueNoConverge(t *testing.T) {
	now := t0.Add(5 * time.Hour)
	tk, _ := NewTask("T1", "p", "API", "", "dev", "po", []string{"c"}, nil, t0)
	tk.Comment("po · genius", "decision", "NSF incluye saldo 0", t0)
	tk.Comment("dev · genius", "decision", "NSF solo si saldo > 0", t0)
	tk.Comment("dev · genius", "pregunta", "¿y la cuenta inactiva?", t0)
	tk.Comment("dev · genius", "respuesta", "la trato como NSF", t0)
	tk.Comment("tester · genius", "pregunta", "¿qué mensaje muestra?", t0)
	for i := 0; i < 3; i++ {
		tk.Claim("dev", nil, t0)
		tk.Move("dev", Review, "", "go test ok", "", t0)
		tk.Claim("tester", nil, t0)
		tk.Move("tester", Rejected, "", "", "falla", t0)
	}
	tk.Claim("dev", nil, t0)
	// Otra tarea sí avanzó hace poco: el equipo sigue trabajando, T1 se quedó atrás.
	other, _ := NewTask("T2", "p", "UI", "", "tester", "po", []string{"c"}, nil, t0)
	other.Comment("tester · genius", "nota", "avance en la UI", now.Add(-30*time.Minute))
	k := kinds(Audit([]*Task{tk, other}, now, nil), "T1")
	for _, want := range []string{"decisiones_en_conflicto", "autorrespuesta", "pregunta_sin_respuesta", "rebote", "estancada"} {
		if !has(k, want) {
			t.Errorf("falta %s en %v", want, k)
		}
	}
	tk.Comment("po · genius", "acuerdo", "NSF incluye saldo 0; cuenta inactiva es otro error", now)
	if has(kinds(Audit([]*Task{tk}, now, nil), "T1"), "decisiones_en_conflicto") {
		t.Error("un acuerdo posterior concilia las decisiones")
	}
}

func TestNadieApruebaSuPropioTrabajo(t *testing.T) {
	tk := submitted(t, "T1", "go test ok", t0)
	if err := tk.Move("dev · genius", Done, "", "lo probé", "", t0); err == nil || !strings.Contains(err.Error(), "propio trabajo") {
		t.Errorf("el dev no cierra su propia tarea: %v", err)
	}
	if err := tk.Move("DEV  ·  genius", Review, RolePO, "ok", "", t0); err == nil {
		t.Error("tampoco con otra grafía del mismo actor")
	}
	if err := tk.Move("tester · genius", Review, RolePO, "criterios verificados: go test ok (14 tests)", "", t0); err != nil || tk.Role != RolePO {
		t.Fatalf("el tester la aprueba y pasa al PO: %v %+v", err, tk)
	}
	if err := tk.Move("po · genius", Done, "", "validado en QA con usuario de prueba", "", t0); err != nil {
		t.Errorf("el PO la cierra: %v", err)
	}
}

func TestMarkdown_SaludDelTablero(t *testing.T) {
	tk := submitted(t, "T1", "listo", t0)
	md := Markdown("p", []*Task{tk}, Audit([]*Task{tk}, t0, nil)...)
	for _, want := range []string{"Alertas del moderador | 1 | | | warning", "## Salud del tablero", "> [!WARNING] T1 · evidencia no verificable"} {
		if !strings.Contains(md, want) {
			t.Errorf("falta %q en:\n%s", want, md)
		}
	}
	clean := submitted(t, "T2", "go test ok (3 tests)", t0)
	if md := Markdown("p", []*Task{clean}, Audit([]*Task{clean}, t0, nil)...); !strings.Contains(md, "> [!TIP] Sin alertas") {
		t.Error("sin alertas lo dice")
	}
}

func TestCadaQuienConLoSuyo(t *testing.T) {
	tk, _ := NewTask("T1", "p", "API", "", "dev", "po", []string{"c"}, nil, t0)
	if err := tk.Claim("tester · genius", nil, t0); err == nil || !strings.Contains(err.Error(), "le toca a Dev y tú eres Tester") {
		t.Errorf("el tester no toma tareas de Dev: %v", err)
	}
	if err := tk.Claim("moderador · genius", nil, t0); err == nil || !strings.Contains(err.Error(), "no toma tareas") {
		t.Errorf("el moderador no toma tareas: %v", err)
	}
	if err := tk.Claim("dev · genius", nil, t0); err != nil {
		t.Fatal(err)
	}
	if err := tk.Move("tester · claude", Review, "", "go test ok", "", t0); err == nil || !strings.Contains(err.Error(), "solo su dueño") {
		t.Errorf("nadie mueve la tarea en curso de otro: %v", err)
	}
	if err := tk.Move("moderador · genius", Review, "", "go test ok", "", t0); err == nil {
		t.Error("el moderador no entrega trabajo")
	}
	if err := tk.Move("moderador · genius", Pending, "", "", "", t0); err != nil || tk.Owner != "" {
		t.Fatalf("el moderador sí libera una tarea en curso: %v", err)
	}
	tk.Claim("dev · genius", nil, t0)
	tk.Move("dev · genius", Review, "", "go test ok (3 tests)", "", t0)
	if err := tk.Move("po · genius", Review, RolePO, "ok", "", t0); err == nil || !strings.Contains(err.Error(), "le toca a Tester") {
		t.Errorf("el PO no salta al Tester: %v", err)
	}
	if err := tk.Move("moderador · genius", Rejected, "", "", "la evidencia cita un archivo que no existe", t0); err != nil {
		t.Errorf("el moderador puede rechazar lo que no se sostiene: %v", err)
	}
	if RoleOf("QA-bot") != RoleTester || RoleOf("Moderator: genius") != RoleModerator || RoleOf("genius") != "" {
		t.Error("RoleOf")
	}
}

func TestAudit_RolEquivocadoSinRolYAcaparamiento(t *testing.T) {
	mk := func(id, role, owner string) *Task {
		tk, _ := NewTask(id, "p", "x", "", role, "po", []string{"c"}, nil, t0)
		tk.Status, tk.Owner = InProgress, owner // datos heredados o editados a mano
		return tk
	}
	a := Audit([]*Task{mk("T1", "tester", "dev · genius"), mk("T2", "dev", "genius"), mk("T3", "dev", "dev · claude"), mk("T4", "dev", "dev · claude")}, t0, nil)
	if !has(kinds(a, "T1"), "rol_equivocado") || !has(kinds(a, "T2"), "sin_rol") || !has(kinds(a, "T3"), "acaparamiento") {
		t.Errorf("alertas: %+v", a)
	}
}

func TestSoloElModeradorVerifica(t *testing.T) {
	tk, _ := NewTask("T9", "SnapProof", "Edición de video", "", "dev", "po", []string{"c"}, nil, t0)
	if err := tk.Comment("dev · genius", "verificado", "E4 lista", t0); err == nil || !strings.Contains(err.Error(), "solo el moderador") {
		t.Errorf("el Dev no se verifica a sí mismo: %v", err)
	}
	if err := tk.Comment("tester · genius", "alerta", "x", t0); err == nil {
		t.Error("el Tester tampoco pone alertas de moderador")
	}
	if err := tk.Comment("moderador · genius", "verificado", "corrí jest: 128 passed", t0); err != nil {
		t.Errorf("el moderador sí: %v", err)
	}
}

func TestChat_DatosHeredadosNoSeVuelvenVerificaciones(t *testing.T) {
	tk, _ := NewTask("T9", "p", "Edición", "", "dev", "po", []string{"c"}, nil, t0)
	tk.Claim("dev · genius", nil, t0)
	// Como en datos reales anteriores a la regla: el Dev se puso «verificado» a sí mismo.
	tk.History = append(tk.History, Event{At: t0.Format(TimeLayout), By: "dev · genius", Tag: "verificado", Text: "E4 lista"})
	other, _ := NewTask("T5", "p", "Pendiente", "", "dev", "po", nil, nil, t0)
	md := ChatMarkdown("p", []*Task{tk, other}, nil, "")
	if strings.Contains(md, "verificada por el moderador") || strings.Contains(md, "> [!TIP] dev · genius · verificó") {
		t.Errorf("un «verificado» del propio Dev no cuenta como verificación:\n%s", md)
	}
	if !strings.Contains(md, "se marcó «Verificado» a sí mismo") || !strings.Contains(md, "| T5 | Pendiente | 0 | 0 | 0 | 0 | 0 | sin empezar |") {
		t.Errorf("lectura:\n%s", md)
	}
}

func TestAudit_SinRuido(t *testing.T) {
	now := t0.Add(12 * time.Hour)
	// De noche todo el equipo paró: nadie se quedó atrás, no hay "estancada".
	a, _ := NewTask("T1", "p", "API", "", "dev", "po", []string{"c"}, nil, t0)
	a.Claim("dev · genius", nil, t0)
	b, _ := NewTask("T2", "p", "UI", "", "dev", "po", []string{"c"}, nil, t0)
	b.Claim("dev · genius", nil, t0.Add(time.Hour))
	if has(kinds(Audit([]*Task{a, b}, now, nil), "T1"), "estancada") {
		t.Error("con todo el tablero quieto no es estancamiento")
	}
	// Una pregunta abierta en una tarea ya cerrada no alerta.
	done := submitted(t, "T3", "go test ok", t0)
	done.Comment("tester · genius", "pregunta", "¿cubre la cuenta inactiva?", t0)
	if err := done.Move("tester · genius", Review, RolePO, "criterios verificados: go test ok (14 tests)", "", t0); err != nil {
		t.Fatal(err)
	}
	if err := done.Move("po · genius", Done, "", "validado en QA con usuario de prueba", "", t0); err != nil {
		t.Fatal(err)
	}
	if has(kinds(Audit([]*Task{done}, now, nil), "T3"), "pregunta_sin_respuesta") {
		t.Error("una tarea cerrada no arrastra preguntas")
	}
}
