package render

import (
	"context"
	"strings"
	"testing"
)

func TestConvertMarkdownToEnvelope_GoldmarkChromaAndD2(t *testing.T) {
	md := `---
title: "Auditoría Motor Pagos"
project: "pagos-core"
---

# Auditoría Motor Pagos

Resumen inicial revisando internal/payments/engine.go:88 con soporte **atómico**.

> [!ERROR] Fuga de goroutines en reintentos
> El canal de respuesta no se cierra en internal/payments/retry.go:44.

| Severidad | Servicio | Latencia | Ubicación |
| :--- | :--- | :---: | :--- |
| error | Validador | 42.5 | internal/payments/retry.go:44 |
| success | Liquidador | 3.1 | internal/payments/settle.go:19 |

` + "```go\npackage payments\n\nfunc Process(amount int) error {\n\treturn nil\n}\n```" + `

` + "```d2\ngateway: \"API Gateway\"\nworker: \"Worker Pagos\"\ngateway -> worker: \"gRPC\"\n```" + `

- [x] Aislar pool de conexiones @dba [success]
- [ ] Cerrar canal en timeout @backend [error]
`

	ctx := context.Background()
	env, warnings, err := ConvertMarkdownToEnvelope(ctx, "", "", "2026-11-18T00:15:00Z", md)
	if err != nil {
		t.Fatalf("ConvertMarkdownToEnvelope falló: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("no se esperaban advertencias en markdown limpio, obtuvo: %+v", warnings)
	}
	if env.Report.Title != "Auditoría Motor Pagos" || env.Report.Project != "pagos-core" {
		t.Fatalf("metadatos inesperados: title=%q project=%q", env.Report.Title, env.Report.Project)
	}

	blocks := env.Report.Blocks
	if len(blocks) != 6 {
		t.Fatalf("esperaba 6 bloques (prose, callout, table, code, diagram, checklist), obtuvo %d", len(blocks))
	}

	// 1. Prose
	if blocks[0]["t"] != "prose" {
		t.Fatalf("bloque 0 esperado prose, obtuvo %v", blocks[0]["t"])
	}

	// 2. Callout
	if blocks[1]["t"] != "callout" || blocks[1]["sev"] != "error" || blocks[1]["ref"] != "internal/payments/retry.go:44" {
		t.Fatalf("callout inesperado: %+v", blocks[1])
	}

	// 3. Table con inferencia de tipos sev, text, num, ref
	if blocks[2]["t"] != "table" {
		t.Fatalf("bloque 2 esperado table, obtuvo %v", blocks[2]["t"])
	}
	cols := blocks[2]["cols"].([]map[string]any)
	if cols[0]["type"] != "sev" || cols[2]["type"] != "num" || cols[3]["type"] != "ref" {
		t.Fatalf("inferencia de columnas incorrecta: %+v", cols)
	}

	// 4. Code coloreado con Chroma v2
	if blocks[3]["t"] != "code" || blocks[3]["tokens"] == nil {
		t.Fatalf("bloque code sin tokens Chroma: %+v", blocks[3])
	}

	// 5. Diagram flow precompilado a SVG limpio con D2 y clases dg-*
	if blocks[4]["t"] != "diagram" || blocks[4]["kind"] != "flow" {
		t.Fatalf("bloque 4 esperado diagram/flow, obtuvo %+v", blocks[4])
	}
	svgOut, _ := blocks[4]["svg"].(string)
	if !strings.Contains(svgOut, "<svg") || !strings.Contains(svgOut, "dg-node") || !strings.Contains(svgOut, "dg-edge") {
		t.Fatalf("SVG precompilado con D2 no contiene estructura temática dg-*: %s", svgOut)
	}
	if strings.Contains(svgOut, "<script") || strings.Contains(svgOut, "foreignObject") {
		t.Fatalf("SVG contiene etiquetas peligrosas: %s", svgOut)
	}

	// 6. Checklist
	if blocks[5]["t"] != "checklist" {
		t.Fatalf("bloque 5 esperado checklist, obtuvo %v", blocks[5]["t"])
	}
}

func TestSanitizeSVG_BlocksHostilePayloads(t *testing.T) {
	if _, err := SanitizeSVG(`<svg><script>alert(1)</script></svg>`); err == nil {
		t.Fatal("esperaba rechazo de <script> en SVG")
	}
	if _, err := SanitizeSVG(`<svg><foreignObject><div>xss</div></foreignObject></svg>`); err == nil {
		t.Fatal("esperaba rechazo de <foreignObject> en SVG")
	}
	clean, err := SanitizeSVG(`<svg width="500" height="300" onload="evil()"><rect x="0" y="0" width="50" height="40" fill="#ff0000" style="opacity:0.5" class="dg-node"/></svg>`)
	if err != nil {
		t.Fatalf("SVG legítimo debía sanitizarse sin error: %v", err)
	}
	if strings.Contains(clean, "onload") || strings.Contains(clean, "fill=") || strings.Contains(clean, "style=") {
		t.Fatalf("atributos peligrosos o inline no fueron limpiados: %s", clean)
	}
	if !strings.Contains(clean, "viewBox=") {
		t.Fatalf("faltó preservar/inyectar viewBox: %s", clean)
	}
}

func TestToggleMarkdownCheckbox_IgnoresCodeFences(t *testing.T) {
	original := "# Plan\r\n\r\n```md\r\n- [ ] Tarea dentro de código (NO tocar)\r\n```\r\n\r\n- [ ] Primera tarea real\r\n- [x] Segunda tarea real\r\n"

	updated1, ok := ToggleMarkdownCheckbox(original, "chk-1", "", true)
	if !ok {
		t.Fatal("debía encontrar y actualizar chk-1")
	}
	if !strings.Contains(updated1, "- [ ] Tarea dentro de código (NO tocar)\r\n") {
		t.Fatalf("modificó por error una casilla dentro de un bloque de código:\n%s", updated1)
	}
	if !strings.Contains(updated1, "- [x] Primera tarea real\r\n") {
		t.Fatalf("no marcó chk-1 como [x]:\n%s", updated1)
	}

	updated2, ok := ToggleMarkdownCheckbox(updated1, "chk-2", "", false)
	if !ok {
		t.Fatal("debía encontrar y desmarcar chk-2")
	}
	if !strings.Contains(updated2, "- [ ] Segunda tarea real\r\n") {
		t.Fatalf("no desmarcó chk-2 a [ ]:\n%s", updated2)
	}
}

func TestParseUnifiedDiff_KindsLineNumbersAndHighlight(t *testing.T) {
	src := "--- a/internal/cache/lru.go\n+++ b/internal/cache/lru.go\n@@ -40,3 +40,4 @@ func (c *LRU) Get(k string) (V, bool) {\n-\te, ok := c.items[k]\n+\tc.mu.Lock()\n+\te, ok := c.items[k]\n \tif !ok {\n"
	rows := ParseUnifiedDiff(src)
	kinds := make([]string, len(rows))
	for i, r := range rows {
		kinds[i] = r.Kind
	}
	want := "hdr,hdr,hunk,del,add,add,ctx"
	if got := strings.Join(kinds, ","); got != want {
		t.Fatalf("kinds = %s, want %s", got, want)
	}
	if rows[3].LineNo != 40 || rows[4].LineNo != 40 || rows[5].LineNo != 41 || rows[6].LineNo != 42 {
		t.Fatalf("números de línea incorrectos: del=%d add=%d,%d ctx=%d", rows[3].LineNo, rows[4].LineNo, rows[5].LineNo, rows[6].LineNo)
	}
	if rows[2].Text != "@@ -40,3 +40,4 @@ func (c *LRU) Get(k string) (V, bool) {" {
		t.Fatalf("la cabecera @@ debe conservarse íntegra: %q", rows[2].Text)
	}
	highlighted := false
	for _, sp := range rows[4].Tokens {
		if sp.Class != "" {
			highlighted = true
		}
	}
	if !highlighted {
		t.Fatalf("las líneas de un .go deben resaltarse con el lexer de Go: %+v", rows[4].Tokens)
	}
}
