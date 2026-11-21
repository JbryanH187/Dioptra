package render

import (
	"regexp"
	"strings"
)

// Conversión de diagramas de texto (Mermaid flowchart/sequence y el DSL simple
// estilo D2) al bloque `diagram`. Regla de fidelidad: si UNA sola línea con
// contenido no se entiende, se devuelve nil y el llamador conserva el fence como
// bloque de código. Nunca se publica un diagrama parcial.

var (
	// Operadores de arista Mermaid, con etiqueta opcional: -->|txt|, -- txt -->, -.->, ==>, --x, <-->...
	mmEdgeRe = regexp.MustCompile(`\s*(?:--\s+([^-|>][^|>]*?)\s+-->|-\.\s+([^.|>][^|>]*?)\s+\.->|==\s+([^=|>][^|>]*?)\s+==>|(<-->|-->|---|-\.->|-\.-|==>|===|--x|--o))\s*(?:\|([^|]*)\|)?\s*`)
	// Nodo Mermaid con forma opcional: A, A[txt], A(txt), A{txt}, A[(txt)], A((txt)), A>txt], A{{txt}}, A[/txt/]
	mmNodeRe     = regexp.MustCompile(`^([A-Za-z0-9_\-]+)\s*(?:(\[\[|\[\(|\(\(|\(\[|\[/|\[\\|\{\{|\[|\(|\{|>)(.*?)(\]\]|\)\]|\)\)|\]\)|/\]|\\\]|\}\}|\]|\)|\}))?(?::::[A-Za-z0-9_\-]+)?$`)
	mmSubgraphRe = regexp.MustCompile(`^subgraph\s+(.+)$`)
	mmSubTitleRe = regexp.MustCompile(`^([A-Za-z0-9_\-]+)\s*\[(.*)\]$`)

	mmParticipantRe = regexp.MustCompile(`^(?:participant|actor)\s+([A-Za-z0-9_\-]+)(?:\s+as\s+(.+))?$`)
	mmMessageRe     = regexp.MustCompile(`^([A-Za-z0-9_\-]+)\s*(-->>|->>|-->|->|--x|-x|--\)|-\))\s*[+-]?\s*([A-Za-z0-9_\-]+)\s*:\s*(.*)$`)
)

// líneas Mermaid sin contenido de autor (estilo/estructura) que pueden ignorarse.
func mermaidIgnorable(line string) bool {
	low := strings.ToLower(line)
	for _, p := range []string{"%%", "classdef ", "class ", "style ", "linkstyle ", "click ", "direction ", "autonumber", "activate ", "deactivate "} {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	return false
}

type diagramBuilder struct {
	order  []string
	labels map[string]string
	edges  []any
	group  string
}

func newDiagramBuilder() *diagramBuilder {
	return &diagramBuilder{labels: map[string]string{}}
}

func (d *diagramBuilder) node(id, label string) {
	label = strings.TrimSpace(strings.Trim(strings.TrimSpace(label), `"`))
	if _, ok := d.labels[id]; !ok {
		d.order = append(d.order, id)
		d.labels[id] = id
		if d.group != "" && label == "" {
			d.labels[id] = id + " · " + d.group
		}
	}
	if label != "" {
		if d.group != "" {
			label += " · " + d.group
		}
		d.labels[id] = label
	}
}

func (d *diagramBuilder) edge(from, to, label string) {
	label = strings.TrimSpace(strings.Trim(strings.TrimSpace(label), `"`))
	if label != "" {
		d.edges = append(d.edges, []any{from, to, label})
	} else {
		d.edges = append(d.edges, []any{from, to})
	}
}

func (d *diagramBuilder) block(kind string) map[string]any {
	if len(d.order) == 0 {
		return nil
	}
	nodes := make([]any, 0, len(d.order))
	for _, id := range d.order {
		nodes = append(nodes, map[string]any{"id": id, "label": d.labels[id]})
	}
	edges := d.edges
	if edges == nil {
		edges = []any{}
	}
	return map[string]any{"t": "diagram", "kind": kind, "nodes": nodes, "edges": edges}
}

// parseMermaidNode interpreta "A[Etiqueta]" y devuelve id y etiqueta.
func parseMermaidNode(s string) (id, label string, ok bool) {
	m := mmNodeRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", "", false
	}
	if m[2] != "" && m[4] == "" {
		return "", "", false // forma abierta sin cerrar
	}
	return m[1], m[3], true
}

func parseMermaidFlowchart(code string) map[string]any {
	d := newDiagramBuilder()
	for _, raw := range strings.Split(code, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
		low := strings.ToLower(line)
		switch {
		case line == "" || mermaidIgnorable(line):
			continue
		case strings.HasPrefix(low, "flowchart") || strings.HasPrefix(low, "graph"):
			continue
		case low == "end":
			d.group = ""
			continue
		}
		if m := mmSubgraphRe.FindStringSubmatch(line); m != nil {
			title := strings.TrimSpace(m[1])
			if t := mmSubTitleRe.FindStringSubmatch(title); t != nil {
				title = t[2]
			}
			d.group = strings.Trim(title, `"`)
			continue
		}

		locs := mmEdgeRe.FindAllStringSubmatchIndex(line, -1)
		if len(locs) == 0 {
			id, lbl, ok := parseMermaidNode(line)
			if !ok {
				return nil
			}
			d.node(id, lbl)
			continue
		}
		// Cadena: N0 op N1 op N2 ...
		var ids []string
		var labels []string
		prev := 0
		for _, loc := range locs {
			id, lbl, ok := parseMermaidNode(line[prev:loc[0]])
			if !ok {
				return nil
			}
			d.node(id, lbl)
			ids = append(ids, id)
			edgeLbl := ""
			for _, g := range []int{2, 4, 6, 10} { // -- txt -->, -. txt .->, == txt ==>, |txt|
				if loc[g] >= 0 {
					edgeLbl = line[loc[g]:loc[g+1]]
				}
			}
			labels = append(labels, edgeLbl)
			prev = loc[1]
		}
		id, lbl, ok := parseMermaidNode(line[prev:])
		if !ok {
			return nil
		}
		d.node(id, lbl)
		ids = append(ids, id)
		for i := 0; i+1 < len(ids); i++ {
			d.edge(ids[i], ids[i+1], labels[i])
		}
	}
	return d.block("flow")
}

func parseMermaidSequence(code string) map[string]any {
	d := newDiagramBuilder()
	for _, raw := range strings.Split(code, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || line == "sequenceDiagram" || mermaidIgnorable(line) {
			continue
		}
		if m := mmParticipantRe.FindStringSubmatch(line); m != nil {
			d.node(m[1], m[2])
			continue
		}
		if m := mmMessageRe.FindStringSubmatch(line); m != nil {
			d.node(m[1], "")
			d.node(m[3], "")
			d.edge(m[1], m[3], m[4])
			continue
		}
		return nil // Note, loop, alt, opt... → se conserva como código
	}
	blk := d.block("seq")
	if blk != nil && (len(d.order) > 12 || len(d.edges) > 40) {
		return nil // supera los límites del bloque seq; mejor código íntegro
	}
	return blk
}

// parseSimpleDiagram cubre el DSL simple (d2/flow/seq/tree): "a -> b: etiqueta" y "a: \"Etiqueta\"".
func parseSimpleDiagram(lang, code string) map[string]any {
	kind := "flow"
	if lang == "seq" {
		kind = "seq"
	} else if lang == "tree" {
		kind = "tree"
	}
	d := newDiagramBuilder()
	for _, raw := range strings.Split(code, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "direction:") {
			continue
		}
		if m := edgeLineRe.FindStringSubmatch(line); len(m) == 4 {
			d.node(m[1], "")
			d.node(m[2], "")
			d.edge(m[1], m[2], m[3])
			continue
		}
		if m := nodeDeclRe.FindStringSubmatch(line); len(m) == 3 && !strings.HasSuffix(strings.TrimSpace(m[2]), "{") {
			d.node(m[1], m[2])
			continue
		}
		return nil
	}
	return d.block(kind)
}

// parseDiagramFence elige el parser según el lenguaje del fence.
func parseDiagramFence(lang, codeStr string) map[string]any {
	if lang == "mermaid" {
		trimmed := strings.TrimSpace(codeStr)
		low := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(low, "sequencediagram"):
			return parseMermaidSequence(codeStr)
		case strings.HasPrefix(low, "flowchart") || strings.HasPrefix(low, "graph"):
			return parseMermaidFlowchart(codeStr)
		default:
			return nil // classDiagram, erDiagram, gantt... → código
		}
	}
	return parseSimpleDiagram(lang, codeStr)
}
