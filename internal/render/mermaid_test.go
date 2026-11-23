package render

import (
	"context"
	"strings"
	"testing"
)

func TestMermaidAmpersandAndComplexDiagrams(t *testing.T) {
	ctx := context.Background()

	// Caso real que antes caía a código en SnapProof (r_0c0cd033abf7): operador &.
	mdFlow := "# Plan\n\n```mermaid\nflowchart LR\n  F0[Fase 0: T4 y T9 en revisión] --> T11 --> T13\n  F0 --> T12 & T14\n  T13 & T14 --> Done[Listo]\n```\n"
	env, _, err := ConvertMarkdownToEnvelope(ctx, "", "snapproof", "2026-11-23T00:00:00Z", mdFlow)
	if err != nil {
		t.Fatalf("flowchart &: %v", err)
	}
	blk := env.Report.Blocks[0]
	if blk["t"] != "diagram" || blk["kind"] != "flow" {
		t.Fatalf("esperaba diagram/flow, obtuvo %+v", blk)
	}
	svg, _ := blk["svg"].(string)
	if !strings.Contains(svg, "<svg") {
		t.Fatalf("esperaba SVG D2 compilado para flowchart con &, obtuvo: %+v", blk)
	}
	edges := blk["edges"].([][]string)
	if len(edges) != 6 {
		t.Fatalf("esperaba 6 aristas expandidas desde &, obtuvo %d: %+v", len(edges), edges)
	}

	// stateDiagram-v2
	mdState := "# Estados\n\n```mermaid\nstateDiagram-v2\n  [*] --> Pendiente\n  Pendiente --> EnCurso : tomar\n  EnCurso --> [*]\n```\n"
	envState, _, err := ConvertMarkdownToEnvelope(ctx, "", "test", "2026-11-23T00:00:00Z", mdState)
	if err != nil {
		t.Fatalf("stateDiagram: %v", err)
	}
	if envState.Report.Blocks[0]["t"] != "diagram" || envState.Report.Blocks[0]["kind"] != "flow" {
		t.Fatalf("stateDiagram debía compilarse a diagram/flow, obtuvo %+v", envState.Report.Blocks[0])
	}

	// erDiagram
	mdER := "# Datos\n\n```mermaid\nerDiagram\n  BUSINESS ||--o{ SERVICE : ofrece\n  BUSINESS {\n    uuid id\n  }\n```\n"
	envER, _, err := ConvertMarkdownToEnvelope(ctx, "", "test", "2026-11-23T00:00:00Z", mdER)
	if err != nil {
		t.Fatalf("erDiagram: %v", err)
	}
	if envER.Report.Blocks[0]["t"] != "diagram" || envER.Report.Blocks[0]["kind"] != "flow" {
		t.Fatalf("erDiagram debía compilarse a diagram/flow, obtuvo %+v", envER.Report.Blocks[0])
	}

	// classDiagram
	mdClass := "# Clases\n\n```mermaid\nclassDiagram\n  class BusinessDirectory {\n    +findBySlug()\n  }\n  ServiceCatalog --> BusinessDirectory : consulta\n```\n"
	envClass, _, err := ConvertMarkdownToEnvelope(ctx, "", "test", "2026-11-23T00:00:00Z", mdClass)
	if err != nil {
		t.Fatalf("classDiagram: %v", err)
	}
	if envClass.Report.Blocks[0]["t"] != "diagram" || envClass.Report.Blocks[0]["kind"] != "flow" {
		t.Fatalf("classDiagram debía compilarse a diagram/flow, obtuvo %+v", envClass.Report.Blocks[0])
	}

	// sequenceDiagram con Note y alt/else
	mdSeq := "# Secuencia\n\n```mermaid\nsequenceDiagram\n  participant C as Cliente\n  participant A as API\n  Note over C: Inicia flujo\n  alt Token válido\n    C->>A: GET /services\n    A-->>C: 200 OK\n  else Expirado\n    A-->>C: 401\n  end\n```\n"
	envSeq, _, err := ConvertMarkdownToEnvelope(ctx, "", "test", "2026-11-23T00:00:00Z", mdSeq)
	if err != nil {
		t.Fatalf("sequenceDiagram con Note/alt: %v", err)
	}
	if envSeq.Report.Blocks[0]["t"] != "diagram" || envSeq.Report.Blocks[0]["kind"] != "seq" {
		t.Fatalf("sequenceDiagram con Note/alt debía compilarse a diagram/seq, obtuvo %+v", envSeq.Report.Blocks[0])
	}

	// Fallback lazy para tipos exóticos (gantt / pie): nunca queda en code texto plano.
	mdGantt := "# Cronograma\n\n```mermaid\ngantt\n  title Release\n  section Fase 1\n  Workspaces :a1, 2026-11-23, 2d\n```\n"
	envGantt, _, err := ConvertMarkdownToEnvelope(ctx, "", "test", "2026-11-23T00:00:00Z", mdGantt)
	if err != nil {
		t.Fatalf("gantt fallback: %v", err)
	}
	if envGantt.Report.Blocks[0]["t"] != "diagram" || envGantt.Report.Blocks[0]["kind"] != "mermaid" {
		t.Fatalf("gantt debía emitir diagram/mermaid para fallback lazy, obtuvo %+v", envGantt.Report.Blocks[0])
	}

	// Diagrama ASCII en fence de texto plano
	mdASCII := "# Flujo ASCII\n\n```text\n┌─────────┐    ┌─────────┐\n│ Cliente │ -> │ Backend │\n└─────────┘    └─────────┘\n```\n"
	envASCII, _, err := ConvertMarkdownToEnvelope(ctx, "", "test", "2026-11-23T00:00:00Z", mdASCII)
	if err != nil {
		t.Fatalf("ASCII diagram: %v", err)
	}
	if envASCII.Report.Blocks[0]["t"] != "diagram" || envASCII.Report.Blocks[0]["kind"] != "flow" {
		t.Fatalf("diagrama ASCII debía convertirse a diagram/flow, obtuvo %+v", envASCII.Report.Blocks[0])
	}
}
