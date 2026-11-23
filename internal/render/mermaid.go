package render

import (
	"fmt"
	"regexp"
	"strings"
)

// Conversión de diagramas de texto (Mermaid flowchart, sequenceDiagram,
// stateDiagram, erDiagram, classDiagram, DSL estilo D2 y cajas ASCII) al bloque
// `diagram`. Si un diagrama Mermaid usa sintaxis exótica fuera del compilador
// nativo D2 (gantt, pie, mindmap, etc.), emite `kind: "mermaid"` con `src` para
// que el visor lo renderice en pantalla con el motor local lazy (nunca queda en
// texto plano).

var (
	// Operadores de arista Mermaid, con etiqueta opcional: -->|txt|, -- txt -->, -.->, ==>, --x, <-->...
	mmEdgeRe = regexp.MustCompile(`\s*(?:--\s+([^-|>][^|>]*?)\s+-->|-\.\s+([^.|>][^|>]*?)\s+\.->|==\s+([^=|>][^|>]*?)\s+==>|(<-->|-->|---|-\.->|-\.-|==>|===|--x|--o))\s*(?:\|([^|]*)\|)?\s*`)
	// Nodo Mermaid con forma opcional: A, A[txt], A(txt), A{txt}, A[(txt)], A((txt)), A([txt]), A>txt], A{{txt}}, A[/txt/]
	mmNodeRe      = regexp.MustCompile(`^([A-Za-z0-9_\-\.]+)\s*(?:(\[\[|\[\(|\(\(|\(\[|\[/|\[\\|\{\{|\[|\(|\{|>)(.*?)(\]\]|\)\]|\)\)|\]\)|/\]|\\\]|\}\}|\]|\)|\}))?(?::::[A-Za-z0-9_\-]+)?$`)
	mmSubgraphRe  = regexp.MustCompile(`(?i)^subgraph\s+(.+)$`)
	mmSubTitleRe  = regexp.MustCompile(`^([A-Za-z0-9_\-]+)\s*\[(.*)\]$`)
	mmFlowDirRe   = regexp.MustCompile(`(?i)^(?:flowchart|graph)\s+(TB|TD|BT|RL|LR)\b`)
	mmDirOnlyRe   = regexp.MustCompile(`(?i)^direction\s+(TB|TD|BT|RL|LR)\b`)
	mmParticipant = regexp.MustCompile(`(?i)^(?:participant|actor)\s+([A-Za-z0-9_\-\.]+)(?:\s+as\s+(.+))?$`)
	mmMessageRe   = regexp.MustCompile(`^([A-Za-z0-9_\-\.]+)\s*(-->>|->>|-->|->|--x|-x|--\)|-\))\s*[+-]?\s*([A-Za-z0-9_\-\.]+)\s*:\s*(.*)$`)
	mmSeqNoteRe   = regexp.MustCompile(`(?i)^note\s+(?:left of|right of|over)\s+([A-Za-z0-9_\-\.]+)(?:\s*,\s*([A-Za-z0-9_\-\.]+))?\s*:\s*(.*)$`)
	mmSeqBlockRe  = regexp.MustCompile(`(?i)^(?:loop|alt|else|opt|par|and|critical|option|break|rect)\b(.*)$`)

	// stateDiagram / stateDiagram-v2
	mmStateEdgeRe  = regexp.MustCompile(`^(\[\*\]|[A-Za-z0-9_\-\.]+)\s*-->\s*(\[\*\]|[A-Za-z0-9_\-\.]+)(?:\s*:\s*(.*))?$`)
	mmStateAliasRe = regexp.MustCompile(`(?i)^state\s+"([^"]+)"\s+as\s+([A-Za-z0-9_\-\.]+)$`)
	mmStateDescRe  = regexp.MustCompile(`^([A-Za-z0-9_\-\.]+)\s*:\s*(.+)$`)
	mmStateOpenRe  = regexp.MustCompile(`(?i)^state\s+(?:"([^"]+)"\s+as\s+)?([A-Za-z0-9_\-\.]+)\s*\{$`)

	// erDiagram
	mmERRelRe = regexp.MustCompile(`^([A-Za-z0-9_\-\.]+)\s+([|}o][|o](?:--|\.\.)[o|][{|o]|[|}o]{1,2}(?:--|\.\.)[{|o]{1,2})\s+([A-Za-z0-9_\-\.]+)\s*:\s*(.*)$`)
	mmEREntRe = regexp.MustCompile(`^([A-Za-z0-9_\-\.]+)\s*\{$`)

	// classDiagram
	mmClassRelRe  = regexp.MustCompile(`^([A-Za-z0-9_\-\.]+)\s*(?:"[^"]*"\s*)?(<\|--|--\|>|\*--|--\*|o--|--o|-->|<--|\.\.>|\<\.\.|\.\.\|>|<\|\.\.|--|\.\.)\s*(?:"[^"]*"\s*)?([A-Za-z0-9_\-\.]+)(?:\s*:\s*(.*))?$`)
	mmClassDeclRe = regexp.MustCompile(`(?i)^class\s+([A-Za-z0-9_\-\.]+)(?:\s*\{)?$`)

	// Diagramas ASCII simples en fences de texto
	asciiBoxTopRe   = regexp.MustCompile(`^[+\x{250c}\x{2554}][-\x{2500}\x{2550}=]{2,}[+\x{2510}\x{2557}]$`)
	asciiBoxRowRe   = regexp.MustCompile(`[|\x{2502}\x{2551}]\s*([^|\x{2502}\x{2551}]+?)\s*[|\x{2502}\x{2551}]`)
	asciiArrowSegRe = regexp.MustCompile(`(?:-->|->|=>|\x{2500}+>|\x{2500}+\x{25b6}|\x{2192})`)
)

// líneas Mermaid sin contenido de autor (estilo/estructura) que pueden ignorarse.
func mermaidIgnorable(line string) bool {
	low := strings.ToLower(line)
	for _, p := range []string{"%%", "classdef ", "class ", "style ", "linkstyle ", "click ", "autonumber", "activate ", "deactivate ", "accTitle", "accDescr"} {
		if strings.HasPrefix(low, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func stripInlineMermaidComment(line string) string {
	if idx := strings.Index(line, "%%"); idx >= 0 {
		return strings.TrimSpace(line[:idx])
	}
	return strings.TrimSpace(line)
}

func mermaidDirToD2(dir string) string {
	switch strings.ToUpper(strings.TrimSpace(dir)) {
	case "TB", "TD":
		return "down"
	case "BT":
		return "up"
	case "RL":
		return "left"
	default:
		return "right"
	}
}

type diagramBuilder struct {
	order     []string
	labels    map[string]string
	edges     []any
	groups    []string
	direction string
}

func newDiagramBuilder() *diagramBuilder {
	return &diagramBuilder{labels: map[string]string{}}
}

func (d *diagramBuilder) currentGroup() string {
	if len(d.groups) == 0 {
		return ""
	}
	return d.groups[len(d.groups)-1]
}

func (d *diagramBuilder) node(id, label string) {
	label = strings.TrimSpace(strings.Trim(strings.TrimSpace(label), `"`))
	grp := d.currentGroup()
	if _, ok := d.labels[id]; !ok {
		d.order = append(d.order, id)
		d.labels[id] = id
		if grp != "" && label == "" {
			d.labels[id] = id + " · " + grp
		}
	}
	if label != "" {
		if grp != "" && !strings.Contains(label, " · "+grp) {
			label += " · " + grp
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
	blk := map[string]any{"t": "diagram", "kind": kind, "nodes": nodes, "edges": edges}
	if d.direction != "" && d.direction != "right" && kind == "flow" {
		blk["direction"] = d.direction
	}
	return blk
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

// splitAmpOutsideQuotes parte "A[x & y] & B" solo por `&` fuera de comillas/corchetes.
func splitAmpOutsideQuotes(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	bracketDepth := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '"':
			inQuote = !inQuote
			cur.WriteByte(ch)
		case '[', '(', '{':
			if !inQuote {
				bracketDepth++
			}
			cur.WriteByte(ch)
		case ']', ')', '}':
			if !inQuote && bracketDepth > 0 {
				bracketDepth--
			}
			cur.WriteByte(ch)
		case '&':
			if !inQuote && bracketDepth == 0 {
				parts = append(parts, strings.TrimSpace(cur.String()))
				cur.Reset()
				continue
			}
			cur.WriteByte(ch)
		default:
			cur.WriteByte(ch)
		}
	}
	if rem := strings.TrimSpace(cur.String()); rem != "" {
		parts = append(parts, rem)
	}
	return parts
}

func parseMermaidNodeGroup(seg string, d *diagramBuilder) ([]string, bool) {
	parts := splitAmpOutsideQuotes(seg)
	if len(parts) == 0 {
		return nil, false
	}
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		id, lbl, ok := parseMermaidNode(p)
		if !ok {
			return nil, false
		}
		d.node(id, lbl)
		ids = append(ids, id)
	}
	return ids, true
}

func parseMermaidFlowchart(code string) map[string]any {
	d := newDiagramBuilder()
	for _, raw := range strings.Split(code, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(stripInlineMermaidComment(raw), ";"))
		low := strings.ToLower(line)
		switch {
		case line == "" || mermaidIgnorable(line):
			continue
		case strings.HasPrefix(low, "flowchart") || strings.HasPrefix(low, "graph"):
			if m := mmFlowDirRe.FindStringSubmatch(line); m != nil {
				d.direction = mermaidDirToD2(m[1])
			}
			continue
		case low == "end":
			if len(d.groups) > 0 {
				d.groups = d.groups[:len(d.groups)-1]
			}
			continue
		}
		if m := mmDirOnlyRe.FindStringSubmatch(line); m != nil {
			if len(d.groups) == 0 {
				d.direction = mermaidDirToD2(m[1])
			}
			continue
		}
		if m := mmSubgraphRe.FindStringSubmatch(line); m != nil {
			title := strings.TrimSpace(m[1])
			if t := mmSubTitleRe.FindStringSubmatch(title); t != nil {
				title = t[2]
			}
			d.groups = append(d.groups, strings.Trim(title, `"`))
			continue
		}

		locs := mmEdgeRe.FindAllStringSubmatchIndex(line, -1)
		if len(locs) == 0 {
			if _, ok := parseMermaidNodeGroup(line, d); !ok {
				return nil
			}
			continue
		}
		// Cadena con soporte de &: (A & B) op (C & D) op E ...
		var groups [][]string
		var labels []string
		prev := 0
		for _, loc := range locs {
			ids, ok := parseMermaidNodeGroup(line[prev:loc[0]], d)
			if !ok {
				return nil
			}
			groups = append(groups, ids)
			edgeLbl := ""
			for _, g := range []int{2, 4, 6, 10} { // -- txt -->, -. txt .->, == txt ==>, |txt|
				if loc[g] >= 0 {
					edgeLbl = line[loc[g]:loc[g+1]]
				}
			}
			labels = append(labels, edgeLbl)
			prev = loc[1]
		}
		ids, ok := parseMermaidNodeGroup(line[prev:], d)
		if !ok {
			return nil
		}
		groups = append(groups, ids)
		for i := 0; i+1 < len(groups); i++ {
			for _, from := range groups[i] {
				for _, to := range groups[i+1] {
					d.edge(from, to, labels[i])
				}
			}
		}
	}
	return d.block("flow")
}

func parseMermaidSequence(code string) map[string]any {
	d := newDiagramBuilder()
	var scopeStack []string
	for _, raw := range strings.Split(code, "\n") {
		line := stripInlineMermaidComment(raw)
		low := strings.ToLower(line)
		if line == "" || low == "sequencediagram" || mermaidIgnorable(line) {
			continue
		}
		if low == "end" {
			if len(scopeStack) > 0 {
				scopeStack = scopeStack[:len(scopeStack)-1]
			}
			continue
		}
		if m := mmParticipant.FindStringSubmatch(line); m != nil {
			d.node(m[1], m[2])
			continue
		}
		if m := mmSeqBlockRe.FindStringSubmatch(line); m != nil {
			tag := strings.TrimSpace(line)
			if strings.HasPrefix(low, "else") || strings.HasPrefix(low, "and") || strings.HasPrefix(low, "option") {
				if len(scopeStack) > 0 {
					scopeStack[len(scopeStack)-1] = tag
				} else {
					scopeStack = append(scopeStack, tag)
				}
			} else {
				scopeStack = append(scopeStack, tag)
			}
			continue
		}
		if m := mmSeqNoteRe.FindStringSubmatch(line); m != nil {
			a := m[1]
			b := m[2]
			if b == "" {
				b = a
			}
			d.node(a, "")
			d.node(b, "")
			d.edge(a, b, "Nota: "+strings.TrimSpace(m[3]))
			continue
		}
		if m := mmMessageRe.FindStringSubmatch(line); m != nil {
			d.node(m[1], "")
			d.node(m[3], "")
			msg := strings.TrimSpace(m[4])
			if len(scopeStack) > 0 {
				msg = "[" + scopeStack[len(scopeStack)-1] + "] " + msg
			}
			d.edge(m[1], m[3], msg)
			continue
		}
		return nil
	}
	blk := d.block("seq")
	if blk != nil && (len(d.order) > 12 || len(d.edges) > 40) {
		return nil
	}
	return blk
}

func parseMermaidState(code string) map[string]any {
	d := newDiagramBuilder()
	d.direction = "down"
	startUsed := false
	endUsed := false
	mapStateID := func(raw string, isSource bool) (string, string) {
		if raw == "[*]" {
			if isSource {
				startUsed = true
				return "__start", "Inicio"
			}
			endUsed = true
			return "__end", "Fin"
		}
		return raw, ""
	}
	for _, raw := range strings.Split(code, "\n") {
		line := stripInlineMermaidComment(raw)
		low := strings.ToLower(line)
		if line == "" || strings.HasPrefix(low, "statediagram") || mermaidIgnorable(line) {
			continue
		}
		if m := mmDirOnlyRe.FindStringSubmatch(line); m != nil {
			d.direction = mermaidDirToD2(m[1])
			continue
		}
		if line == "}" {
			if len(d.groups) > 0 {
				d.groups = d.groups[:len(d.groups)-1]
			}
			continue
		}
		if m := mmStateOpenRe.FindStringSubmatch(line); m != nil {
			lbl := strings.TrimSpace(m[1])
			if lbl == "" {
				lbl = m[2]
			}
			d.groups = append(d.groups, lbl)
			continue
		}
		if m := mmStateAliasRe.FindStringSubmatch(line); m != nil {
			d.node(m[2], m[1])
			continue
		}
		if m := mmStateEdgeRe.FindStringSubmatch(line); m != nil {
			fromID, fromLbl := mapStateID(m[1], true)
			toID, toLbl := mapStateID(m[2], false)
			d.node(fromID, fromLbl)
			d.node(toID, toLbl)
			d.edge(fromID, toID, m[3])
			continue
		}
		if m := mmStateDescRe.FindStringSubmatch(line); m != nil {
			d.node(m[1], m[1]+": "+strings.TrimSpace(m[2]))
			continue
		}
		return nil
	}
	_, _ = startUsed, endUsed
	return d.block("flow")
}

func parseMermaidER(code string) map[string]any {
	d := newDiagramBuilder()
	inEntity := false
	for _, raw := range strings.Split(code, "\n") {
		line := stripInlineMermaidComment(raw)
		low := strings.ToLower(line)
		if line == "" || low == "erdiagram" || mermaidIgnorable(line) {
			continue
		}
		if inEntity {
			if line == "}" {
				inEntity = false
			}
			continue
		}
		if m := mmEREntRe.FindStringSubmatch(line); m != nil {
			d.node(m[1], m[1])
			inEntity = true
			continue
		}
		if m := mmERRelRe.FindStringSubmatch(line); m != nil {
			d.node(m[1], m[1])
			d.node(m[3], m[3])
			lbl := strings.TrimSpace(strings.Trim(m[4], `"`))
			if lbl == "" {
				lbl = m[2]
			}
			d.edge(m[1], m[3], lbl)
			continue
		}
		return nil
	}
	return d.block("flow")
}

func parseMermaidClass(code string) map[string]any {
	d := newDiagramBuilder()
	d.direction = "down"
	inClassBody := false
	for _, raw := range strings.Split(code, "\n") {
		line := stripInlineMermaidComment(raw)
		low := strings.ToLower(line)
		if line == "" || low == "classdiagram" {
			continue
		}
		if m := mmDirOnlyRe.FindStringSubmatch(line); m != nil {
			d.direction = mermaidDirToD2(m[1])
			continue
		}
		if inClassBody {
			if line == "}" {
				inClassBody = false
			}
			continue
		}
		if m := mmClassDeclRe.FindStringSubmatch(line); m != nil {
			d.node(m[1], m[1])
			if strings.HasSuffix(line, "{") {
				inClassBody = true
			}
			continue
		}
		if mermaidIgnorable(line) {
			continue
		}
		if m := mmClassRelRe.FindStringSubmatch(line); m != nil {
			d.node(m[1], m[1])
			d.node(m[3], m[3])
			lbl := strings.TrimSpace(m[4])
			d.edge(m[1], m[3], lbl)
			continue
		}
		if strings.Contains(line, ":") {
			// Miembro suelto `Clase : +metodo()`
			parts := strings.SplitN(line, ":", 2)
			cls := strings.TrimSpace(parts[0])
			if cls != "" && !strings.ContainsAny(cls, " \t") {
				d.node(cls, cls)
				continue
			}
		}
		return nil
	}
	return d.block("flow")
}

// parseASCIIDiagram reconoce diagramas de flujo ASCII simples con cajas (`┌──┐`/`+--+`)
// conectadas por flechas horizontales o verticales y los convierte en bloques `flow`.
func parseASCIIDiagram(code string) map[string]any {
	lines := strings.Split(strings.ReplaceAll(code, "\r\n", "\n"), "\n")
	if len(lines) < 3 || len(lines) > 80 {
		return nil
	}
	hasBoxBorder := false
	hasArrow := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if asciiBoxTopRe.MatchString(t) || strings.Contains(t, "┌─") || strings.Contains(t, "+--") {
			hasBoxBorder = true
		}
		if asciiArrowSegRe.MatchString(t) || strings.ContainsAny(t, "▼↓→▶") {
			hasArrow = true
		}
	}
	if !hasBoxBorder || !hasArrow {
		return nil
	}

	d := newDiagramBuilder()
	var prevRowIDs []string
	seq := 0
	for _, l := range lines {
		matches := asciiBoxRowRe.FindAllStringSubmatch(l, -1)
		if len(matches) == 0 {
			continue
		}
		var rowIDs []string
		for _, m := range matches {
			lbl := strings.TrimSpace(m[1])
			if lbl == "" || strings.Trim(lbl, "-─═=+|┌┐└┘") == "" {
				continue
			}
			seq++
			id := fmt.Sprintf("b%d", seq)
			d.node(id, lbl)
			rowIDs = append(rowIDs, id)
		}
		if len(rowIDs) == 0 {
			continue
		}
		// Flechas horizontales entre cajas en la misma fila.
		if len(rowIDs) > 1 && asciiArrowSegRe.MatchString(l) {
			for i := 0; i+1 < len(rowIDs); i++ {
				d.edge(rowIDs[i], rowIDs[i+1], "")
			}
		}
		// Conexión vertical entre filas consecutivas si hay flujo descendente.
		if len(prevRowIDs) > 0 && len(rowIDs) == 1 && len(prevRowIDs) == 1 {
			d.edge(prevRowIDs[0], rowIDs[0], "")
		} else if len(prevRowIDs) > 0 && len(rowIDs) == len(prevRowIDs) && !asciiArrowSegRe.MatchString(l) {
			for i := range rowIDs {
				d.edge(prevRowIDs[i], rowIDs[i], "")
			}
		}
		prevRowIDs = rowIDs
	}
	if len(d.order) < 2 || len(d.edges) == 0 {
		return nil
	}
	return d.block("flow")
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
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "direction:") {
			dir := strings.TrimSpace(line[len("direction:"):])
			if dir == "right" || dir == "down" || dir == "left" || dir == "up" {
				d.direction = dir
			}
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
		if trimmed == "" {
			return nil
		}
		low := strings.ToLower(trimmed)
		var parsed map[string]any
		switch {
		case strings.HasPrefix(low, "sequencediagram"):
			parsed = parseMermaidSequence(codeStr)
		case strings.HasPrefix(low, "flowchart") || strings.HasPrefix(low, "graph"):
			parsed = parseMermaidFlowchart(codeStr)
		case strings.HasPrefix(low, "statediagram"):
			parsed = parseMermaidState(codeStr)
		case strings.HasPrefix(low, "erdiagram"):
			parsed = parseMermaidER(codeStr)
		case strings.HasPrefix(low, "classdiagram"):
			parsed = parseMermaidClass(codeStr)
		}
		if parsed != nil {
			return parsed
		}
		// Fallback renderizable en el visor local (lazy Mermaid JS): nunca se queda como texto plano.
		if len(trimmed) <= 64*1024 {
			return map[string]any{
				"t":    "diagram",
				"kind": "mermaid",
				"src":  trimmed,
			}
		}
		return nil
	}
	return parseSimpleDiagram(lang, codeStr)
}
