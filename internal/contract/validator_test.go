package contract

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateAndNormalize_EnvelopeRejectionStep0(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantPath string
	}{
		{"invalid_json", `{not-json`, "root"},
		{"missing_v", `{"report":{"title":"Hola","blocks":[]}}`, "v"},
		{"missing_report", `{"v":1}`, "report"},
		{"empty_title", `{"v":1,"report":{"title":"   ","blocks":[]}}`, "report.title"},
		{"missing_blocks", `{"v":1,"report":{"title":"Ok"}}`, "report.blocks"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ValidateAndNormalizeRawJSON([]byte(tc.raw))
			if err == nil {
				t.Fatalf("esperaba error de sobre para %s, obtuvo nil", tc.name)
			}
			envErr, ok := err.(*EnvelopeError)
			if !ok {
				t.Fatalf("esperaba *EnvelopeError, obtuvo %T (%v)", err, err)
			}
			if envErr.Path != tc.wantPath {
				t.Fatalf("ruta esperada %q, obtuvo %q", tc.wantPath, envErr.Path)
			}
		})
	}
}

func TestValidateAndNormalize_ControlledToleranceSteps1To3(t *testing.T) {
	kpiItems := make([]any, 15)
	for i := 0; i < 15; i++ {
		sev := "info"
		if i == 0 {
			sev = "critical_unknown"
		}
		kpiItems[i] = map[string]any{
			"label": fmt.Sprintf("M-%d", i),
			"value": i,
			"sev":   sev,
		}
	}

	manyLines := strings.Repeat("fmt.Println(1)\n", 2005)

	root := map[string]any{
		"v":          1,
		"_extraRoot": true,
		"report": map[string]any{
			"title":      "Prueba Tolerancia ADR 1",
			"project":    "dioptra-test",
			"_extraMeta": "ignorar",
			"blocks": []any{
				// 0: KPI > 12 + sev inválida -> reparado
				map[string]any{
					"t":          "kpi",
					"_extraProp": 99,
					"items":      kpiItems,
				},
				// 1: Tabla rota sin cols -> degradado a code(json) con degraded:{from:"table", reason:"falta cols"}
				map[string]any{
					"t":    "table",
					"rows": []any{map[string]any{"a": 1}},
				},
				// 2: Tipo desconocido con title+body -> degradado a prose
				map[string]any{
					"t":     "custom_banner",
					"title": "Aviso heredado",
					"body":  "Detalle preservado en **prosa**",
				},
				// 3: Tipo desconocido con html -> degradado a code(html)
				map[string]any{
					"t":    "legacy_widget",
					"html": "<div>widget</div>",
				},
				// 4: Código > 2000 líneas -> truncado
				map[string]any{
					"t":    "code",
					"lang": "go",
					"src":  manyLines,
				},
				// 5: Diagrama tree con 2 raíces -> reparado a flow
				map[string]any{
					"t":    "diagram",
					"kind": "tree",
					"nodes": []any{
						map[string]any{"id": "a", "label": "Raíz A"},
						map[string]any{"id": "b", "label": "Raíz B"},
					},
					"edges": []any{},
				},
				// 6: Sección anidada nivel > 3 -> aplanada en nivel 3
				map[string]any{
					"t":     "section",
					"title": "", // también repara título vacío a "(sin título)"
					"blocks": []any{
						map[string]any{
							"t":     "section",
							"title": "Nivel 2",
							"blocks": []any{
								map[string]any{
									"t":     "section",
									"title": "Nivel 3",
									"blocks": []any{
										map[string]any{
											"t":     "section",
											"title": "Nivel 4 aplanado",
											"blocks": []any{
												map[string]any{"t": "prose", "md": "Contenido profundo"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	env, warnings, err := ValidateAndNormalizeMap(root)
	if err != nil {
		t.Fatalf("no debía fallar el documento completo: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("esperaba advertencias registradas")
	}

	blocks := env.Report.Blocks
	if len(blocks) != 7 {
		t.Fatalf("esperaba 7 bloques de primer nivel, obtuvo %d", len(blocks))
	}

	// Bloque 0: KPI truncado a 12 y sev inválida omitida
	kpis := blocks[0]["items"].([]map[string]any)
	if len(kpis) != 12 {
		t.Fatalf("esperaba 12 KPIs tras truncado, obtuvo %d", len(kpis))
	}
	if _, hasSev := kpis[0]["sev"]; hasSev {
		t.Fatalf("esperaba sev inválida omitida en kpis[0], obtuvo %v", kpis[0]["sev"])
	}

	// Bloque 1: Tabla sin cols degradada a code(json)
	if blocks[1]["t"] != "code" || blocks[1]["lang"] != "json" {
		t.Fatalf("esperaba degradación a code/json, obtuvo %+v", blocks[1])
	}
	deg1, ok := blocks[1]["degraded"].(DegradedMeta)
	if !ok || deg1.From != "table" || deg1.Reason != "falta cols" {
		t.Fatalf("metadato degraded inesperado: %+v", blocks[1]["degraded"])
	}

	// Bloque 2: Degradado a prose
	if blocks[2]["t"] != "prose" {
		t.Fatalf("esperaba degradación a prose, obtuvo %+v", blocks[2])
	}

	// Bloque 3: Degradado a code(html)
	if blocks[3]["t"] != "code" || blocks[3]["lang"] != "html" {
		t.Fatalf("esperaba degradación a code/html, obtuvo %+v", blocks[3])
	}

	// Bloque 4: Código truncado
	codeLines := strings.Split(blocks[4]["src"].(string), "\n")
	if len(codeLines) != 2001 { // 2000 líneas + línea de aviso de truncado
		t.Fatalf("esperaba 2001 líneas tras truncado, obtuvo %d", len(codeLines))
	}

	// Bloque 5: Tree inválido reclasificado a flow
	if blocks[5]["kind"] != "flow" {
		t.Fatalf("esperaba reclasificación a flow, obtuvo %v", blocks[5]["kind"])
	}

	// Bloque 6: Título reparado y nivel 4 aplanado en nivel 3
	if blocks[6]["title"] != "(sin título)" {
		t.Fatalf("esperaba '(sin título)', obtuvo %v", blocks[6]["title"])
	}
	lvl2 := blocks[6]["blocks"].([]map[string]any)[0]
	lvl3 := lvl2["blocks"].([]map[string]any)[0]
	lvl3Children := lvl3["blocks"].([]map[string]any)
	if len(lvl3Children) != 1 || lvl3Children[0]["t"] != "prose" {
		t.Fatalf("esperaba hijo aplanado 'prose' en nivel 3, obtuvo %+v", lvl3Children)
	}
}
