package layoutagent_test

import (
	"context"
	"strings"
	"testing"

	"dioptra/internal/contract"
	"dioptra/internal/layoutagent"
)

func envFromBlocks(t *testing.T, blocks []any) *contract.Envelope {
	t.Helper()
	env, _, err := contract.ValidateAndNormalizeMap(map[string]any{
		"v":      1,
		"report": map[string]any{"title": "T", "ts": "2026-11-20T00:00:00Z", "blocks": blocks},
	})
	if err != nil {
		t.Fatalf("envelope: %v", err)
	}
	return env
}

// Convertir un mermaid a nodos no debe contar como pérdida de contenido (F5).
func TestGuard_MermaidAsDiagramIsNotLowCoverage(t *testing.T) {
	md := "# Flujo\n\nEl router recibe eventos del gateway y los reparte a los workers de correo.\n\n" +
		"```mermaid\nflowchart LR\n    subgraph Infraestructura\n      A[API Gateway] -->|evento| B(Router de eventos)\n    end\n    B --> C[Worker SMTP]\n    classDef caliente fill:#f00\n    class A caliente\n```\n"
	env := envFromBlocks(t, []any{
		map[string]any{"t": "prose", "md": "El router recibe eventos del gateway y los reparte a los workers de correo."},
		map[string]any{"t": "diagram", "kind": "flow",
			"nodes": []any{
				map[string]any{"id": "A", "label": "API Gateway"},
				map[string]any{"id": "B", "label": "Router de eventos"},
				map[string]any{"id": "C", "label": "Worker SMTP"},
			},
			"edges": []any{[]any{"A", "B", "evento"}, []any{"B", "C"}}},
	})
	_, meta, err := layoutagent.GuardExternalEnvelope(context.Background(), env, layoutagent.Input{Title: "Flujo", Markdown: md})
	if err != nil || meta.Verdict == "rejected" {
		t.Fatalf("un diagrama fiel no debe rechazarse: verdict=%s err=%v hint=%s", meta.Verdict, err, meta.Hint)
	}
}

// F2: un KPI "3/5" exige que existan ambas cifras en la fuente.
func TestGuard_KPIRequiresAllNumbers(t *testing.T) {
	md := "# Estado\n\nPasaron 3 de las pruebas de carga planeadas para hoy en el clúster de staging del equipo de pagos."
	env := envFromBlocks(t, []any{
		map[string]any{"t": "kpi", "items": []any{
			map[string]any{"label": "Pruebas", "value": "3/5"},
			map[string]any{"label": "Pasaron", "value": "3"},
		}},
		map[string]any{"t": "prose", "md": "Pasaron 3 de las pruebas de carga planeadas para hoy en el clúster de staging del equipo de pagos."},
	})
	rep, _, err := layoutagent.GuardExternalEnvelope(context.Background(), env, layoutagent.Input{Title: "Estado", Markdown: md})
	if err != nil {
		t.Fatal(err)
	}
	items, _ := env.Report.Blocks[0]["items"].([]map[string]any)
	if len(items) != 1 || items[0]["label"] != "Pasaron" {
		t.Fatalf("el KPI 3/5 (5 no está en la fuente) debía eliminarse: %+v (warnings %+v)", items, rep.Warnings)
	}
}

// Al rechazar, la pista nombra las secciones "##" que quedaron fuera.
func TestGuard_RejectHintNamesWeakSections(t *testing.T) {
	riesgos := "El clúster primario opera cerca del límite de memoria; los nodos réplica tardan demasiado en sincronizar " +
		"cuando aumenta el tráfico nocturno, y la cola de mensajes acumula retrasos visibles para clientes corporativos durante cierres contables mensuales."
	rollback := "Revertir requiere congelar escrituras, restaurar instantáneas verificadas, invalidar cachés distribuidas, " +
		"reapuntar balanceadores hacia la versión anterior y confirmar con soporte que ningún pedido quedó huérfano tras completar la reversión."
	md := "# Reporte\n\n## Resumen\n\nTodo estable en producción durante la semana.\n\n## Riesgos de capacidad\n\n" + riesgos + "\n\n## Plan de rollback\n\n" + rollback
	env := envFromBlocks(t, []any{
		map[string]any{"t": "prose", "md": "Todo estable en producción durante la semana."},
	})
	_, meta, err := layoutagent.GuardExternalEnvelope(context.Background(), env, layoutagent.Input{Title: "Reporte", Markdown: md})
	if err == nil || meta.Verdict != "rejected" {
		t.Fatalf("se esperaba rechazo por baja cobertura, verdict=%s", meta.Verdict)
	}
	if !strings.Contains(meta.Hint, "Riesgos de capacidad") || !strings.Contains(meta.Hint, "Plan de rollback") {
		t.Fatalf("la pista debe nombrar las secciones débiles: %s", meta.Hint)
	}
}
