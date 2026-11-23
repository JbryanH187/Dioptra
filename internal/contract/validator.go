package contract

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

var validSeverities = map[string]int{
	"info":    2,
	"success": 1,
	"warning": 3,
	"error":   4,
}

var allowedEnvelopeKeys = map[string]bool{
	"v":        true,
	"report":   true,
	"warnings": true,
}

var allowedReportKeys = map[string]bool{
	"id":         true,
	"project":    true,
	"title":      true,
	"ts":         true,
	"updatedAt":  true,
	"revision":   true,
	"lastChange": true,
	"maxSev":     true,
	"excerpt":    true,
	"blocks":     true,
}

var allowedBlockKeys = map[string]map[string]bool{
	"kpi":       {"t": true, "items": true},
	"callout":   {"t": true, "sev": true, "title": true, "body": true, "ref": true},
	"table":     {"t": true, "cols": true, "rows": true, "sort": true, "filter": true},
	"section":   {"t": true, "title": true, "sev": true, "open": true, "blocks": true},
	"code":      {"t": true, "lang": true, "src": true, "old": true, "ref": true, "hl": true, "tokens": true, "hunks": true, "degraded": true},
	"diagram":   {"t": true, "kind": true, "nodes": true, "edges": true, "svg": true, "src": true, "direction": true},
	"checklist": {"t": true, "items": true, "readonly": true},
	"prose":     {"t": true, "title": true, "md": true, "degraded": true},
	"canvas":    {"t": true, "html": true, "src": true, "h": true},
}

// EnvelopeError representa un fallo de Paso 0 (sobre raíz inutilizable).
type EnvelopeError struct {
	Path    string
	Message string
}

func (e *EnvelopeError) Error() string {
	return fmt.Sprintf("error de sobre en %s: %s", e.Path, e.Message)
}

// ValidateAndNormalizeRawJSON valida bytes JSON crudos aplicando la política ADR 1.
func ValidateAndNormalizeRawJSON(raw []byte) (*Envelope, []Warning, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, nil, &EnvelopeError{Path: "root", Message: fmt.Sprintf("JSON inválido: %v", err)}
	}
	return ValidateAndNormalizeMap(root)
}

// ValidateAndNormalizeMap ejecuta los Pasos 0, 1, 2 y 3 de §2 ADR 1.
func ValidateAndNormalizeMap(root map[string]any) (*Envelope, []Warning, error) {
	if root == nil {
		return nil, nil, &EnvelopeError{Path: "root", Message: "el sobre raíz debe ser un objeto"}
	}

	if _, ok := root["v"]; !ok || root["v"] == nil {
		return nil, nil, &EnvelopeError{Path: "v", Message: "falta campo obligatorio v"}
	}

	repAny, ok := root["report"]
	if !ok || repAny == nil {
		return nil, nil, &EnvelopeError{Path: "report", Message: "falta objeto obligatorio report"}
	}
	repMap, ok := repAny.(map[string]any)
	if !ok {
		return nil, nil, &EnvelopeError{Path: "report", Message: "report debe ser un objeto JSON"}
	}

	titleStr, ok := asNonEmptyString(repMap["title"])
	if !ok {
		return nil, nil, &EnvelopeError{Path: "report.title", Message: "falta report.title válido"}
	}

	blocksAny, ok := repMap["blocks"]
	if !ok || blocksAny == nil {
		return nil, nil, &EnvelopeError{Path: "report.blocks", Message: "falta arreglo report.blocks[]"}
	}
	blocksSlice, ok := blocksAny.([]any)
	if !ok {
		return nil, nil, &EnvelopeError{Path: "report.blocks", Message: "report.blocks debe ser un arreglo"}
	}

	var warnings []Warning

	// Paso 1: Ignorar claves desconocidas en sobre y reporte (ordenadas para determinismo)
	for _, k := range sortedKeys(root) {
		if !allowedEnvelopeKeys[k] {
			warnings = append(warnings, Warning{
				Path:   k,
				Action: "ignored",
				Reason: fmt.Sprintf("propiedad desconocida %q ignorada", k),
			})
		}
	}
	for _, k := range sortedKeys(repMap) {
		if !allowedReportKeys[k] {
			warnings = append(warnings, Warning{
				Path:   "report." + k,
				Action: "ignored",
				Reason: fmt.Sprintf("propiedad desconocida %q ignorada", k),
			})
		}
	}

	var cleanBlocks []map[string]any
	for i, rawBlk := range blocksSlice {
		normalized := NormalizeBlock(rawBlk, fmt.Sprintf("blocks[%d]", i), 1, &warnings)
		cleanBlocks = append(cleanBlocks, normalized...)
	}

	tsStr, ok := asNonEmptyString(repMap["ts"])
	if !ok {
		tsStr = time.Now().Format(time.RFC3339)
	}
	projStr, _ := asNonEmptyString(repMap["project"])
	if projStr == "" {
		projStr = "dioptra"
	}
	idStr, _ := asNonEmptyString(repMap["id"])
	updatedAtStr, _ := asNonEmptyString(repMap["updatedAt"])
	lastChangeStr, _ := asNonEmptyString(repMap["lastChange"])
	revisionInt := 0
	if revNum, ok := asInt(repMap["revision"]); ok && revNum >= 1 {
		revisionInt = revNum
	}

	env := &Envelope{
		V:        1,
		Warnings: warnings,
		Report: Report{
			ID:         idStr,
			Project:    projStr,
			Title:      titleStr,
			TS:         tsStr,
			UpdatedAt:  updatedAtStr,
			Revision:   revisionInt,
			LastChange: lastChangeStr,
			MaxSev:     ExtractMaxSeverity(cleanBlocks),
			Excerpt:    ExtractExcerpt(cleanBlocks, titleStr),
			Blocks:     cleanBlocks,
		},
	}
	return env, warnings, nil
}

// NormalizeBlock valida, repara o degrada un bloque individual según ADR 1.
func NormalizeBlock(rawAny any, path string, depth int, warnings *[]Warning) []map[string]any {
	rawBlock, ok := rawAny.(map[string]any)
	if !ok || rawBlock == nil {
		return []map[string]any{degradeBlock(rawAny, path, "invalid", "el bloque no es un objeto JSON", warnings)}
	}

	tStr, _ := rawBlock["t"].(string)
	allowed, knownType := allowedBlockKeys[tStr]
	if !knownType {
		from := tStr
		if from == "" {
			from = "unknown"
		}
		return []map[string]any{degradeBlock(rawBlock, path, from, fmt.Sprintf("tipo de bloque desconocido %q", from), warnings)}
	}

	// Paso 1: Ignorar propiedades desconocidas adicionales
	for _, k := range sortedKeys(rawBlock) {
		if !allowed[k] {
			*warnings = append(*warnings, Warning{
				Path:   fmt.Sprintf("%s.%s", path, k),
				Action: "ignored",
				Reason: fmt.Sprintf("propiedad desconocida %q ignorada", k),
			})
		}
	}

	switch tStr {
	case "kpi":
		itemsAny, ok := rawBlock["items"].([]any)
		if !ok || len(itemsAny) == 0 {
			return []map[string]any{degradeBlock(rawBlock, path, "kpi", "falta items[] válido", warnings)}
		}
		if len(itemsAny) > 12 {
			*warnings = append(*warnings, Warning{
				Path:   path + ".items",
				Action: "repaired",
				Reason: fmt.Sprintf("KPIs truncados de %d a 12", len(itemsAny)),
			})
			itemsAny = itemsAny[:12]
		}
		cleanItems := make([]map[string]any, 0, len(itemsAny))
		for i, itAny := range itemsAny {
			itMap, ok := itAny.(map[string]any)
			if !ok {
				return []map[string]any{degradeBlock(rawBlock, path, "kpi", fmt.Sprintf("items[%d] no es un objeto", i), warnings)}
			}
			label, okL := asNonEmptyString(itMap["label"])
			val, okV := normalizeScalarValue(itMap["value"])
			if !okL || !okV {
				return []map[string]any{degradeBlock(rawBlock, path, "kpi", fmt.Sprintf("items[%d] carece de label o value", i), warnings)}
			}
			itemOut := map[string]any{
				"label": label,
				"value": val,
			}
			if u, ok := asString(itMap["unit"]); ok && u != "" {
				itemOut["unit"] = u
			}
			if d, ok := asString(itMap["delta"]); ok && d != "" {
				itemOut["delta"] = d
			}
			if sev := sanitizeSev(itMap["sev"], fmt.Sprintf("%s.items[%d].sev", path, i), "", warnings); sev != "" {
				itemOut["sev"] = sev
			}
			cleanItems = append(cleanItems, itemOut)
		}
		return []map[string]any{{"t": "kpi", "items": cleanItems}}

	case "callout":
		title, okT := asString(rawBlock["title"])
		body, okB := asString(rawBlock["body"])
		if !okT || !okB || strings.TrimSpace(title) == "" {
			return []map[string]any{degradeBlock(rawBlock, path, "callout", "falta title o body", warnings)}
		}
		sev := sanitizeSev(rawBlock["sev"], path+".sev", "info", warnings)
		out := map[string]any{
			"t":     "callout",
			"sev":   sev,
			"title": title,
			"body":  body,
		}
		if ref, ok := asNonEmptyString(rawBlock["ref"]); ok {
			out["ref"] = ref
		}
		return []map[string]any{out}

	case "table":
		colsAny, okC := rawBlock["cols"].([]any)
		if !okC || len(colsAny) == 0 {
			return []map[string]any{degradeBlock(rawBlock, path, "table", "falta cols", warnings)}
		}
		rowsAny, okR := rawBlock["rows"].([]any)
		if !okR {
			return []map[string]any{degradeBlock(rawBlock, path, "table", "falta rows[]", warnings)}
		}
		validColTypes := map[string]bool{"text": true, "num": true, "sev": true, "ref": true}
		cleanCols := make([]map[string]any, 0, len(colsAny))
		for i, cAny := range colsAny {
			cMap, ok := cAny.(map[string]any)
			if !ok {
				return []map[string]any{degradeBlock(rawBlock, path, "table", fmt.Sprintf("cols[%d] inválido", i), warnings)}
			}
			key, okK := asNonEmptyString(cMap["key"])
			label, okL := asNonEmptyString(cMap["label"])
			if !okK || !okL {
				return []map[string]any{degradeBlock(rawBlock, path, "table", fmt.Sprintf("cols[%d] sin key o label", i), warnings)}
			}
			cType, _ := asString(cMap["type"])
			if !validColTypes[cType] {
				cType = "text"
			}
			cleanCols = append(cleanCols, map[string]any{"key": key, "label": label, "type": cType})
		}
		cleanRows := make([]map[string]any, 0, len(rowsAny))
		for _, rAny := range rowsAny {
			if rMap, ok := rAny.(map[string]any); ok {
				cleanRows = append(cleanRows, rMap)
			}
		}
		out := map[string]any{
			"t":      "table",
			"cols":   cleanCols,
			"rows":   cleanRows,
			"filter": asBool(rawBlock["filter"]),
		}
		if sortKey, ok := asNonEmptyString(rawBlock["sort"]); ok {
			out["sort"] = sortKey
		}
		return []map[string]any{out}

	case "section":
		subAny, ok := rawBlock["blocks"].([]any)
		if !ok {
			return []map[string]any{degradeBlock(rawBlock, path, "section", "falta blocks[]", warnings)}
		}
		title, okT := asNonEmptyString(rawBlock["title"])
		if !okT {
			title = "(sin título)"
			*warnings = append(*warnings, Warning{
				Path:   path + ".title",
				Action: "repaired",
				Reason: `section sin título reparada a "(sin título)"`,
			})
		}
		sev := sanitizeSev(rawBlock["sev"], path+".sev", "", warnings)
		var children []map[string]any
		for i, ch := range subAny {
			children = append(children, NormalizeBlock(ch, fmt.Sprintf("%s.blocks[%d]", path, i), depth+1, warnings)...)
		}
		if depth > 3 {
			*warnings = append(*warnings, Warning{
				Path:   path,
				Action: "repaired",
				Reason: "anidación > 3 aplanada en el nivel 3",
			})
			return children
		}
		out := map[string]any{
			"t":      "section",
			"title":  title,
			"open":   asBool(rawBlock["open"]),
			"blocks": children,
		}
		if sev != "" {
			out["sev"] = sev
		}
		return []map[string]any{out}

	case "code":
		src, ok := asString(rawBlock["src"])
		if !ok {
			return []map[string]any{degradeBlock(rawBlock, path, "code", "falta src", warnings)}
		}
		lang, _ := asNonEmptyString(rawBlock["lang"])
		if lang == "" {
			lang = "text"
		}
		out := map[string]any{
			"t":    "code",
			"lang": lang,
			"src":  truncateLines(src, 2000, path+".src", warnings),
		}
		if oldStr, ok := asString(rawBlock["old"]); ok {
			out["old"] = truncateLines(oldStr, 2000, path+".old", warnings)
		}
		if ref, ok := asNonEmptyString(rawBlock["ref"]); ok {
			out["ref"] = ref
		}
		if hlSlice, ok := rawBlock["hl"].([]any); ok {
			var hl []int
			for _, v := range hlSlice {
				if n, ok := asInt(v); ok && n > 0 {
					hl = append(hl, n)
				}
			}
			if len(hl) > 0 {
				out["hl"] = hl
			}
		}
		if rawBlock["tokens"] != nil {
			out["tokens"] = rawBlock["tokens"]
		}
		if rawBlock["hunks"] != nil {
			out["hunks"] = rawBlock["hunks"]
		}
		if rawBlock["degraded"] != nil {
			out["degraded"] = rawBlock["degraded"]
		}
		return []map[string]any{out}

	case "diagram":
		kind, _ := asString(rawBlock["kind"])
		if kind == "mermaid" {
			srcStr, okS := asNonEmptyString(rawBlock["src"])
			if !okS {
				return []map[string]any{degradeBlock(rawBlock, path, "diagram", "diagrama mermaid sin src", warnings)}
			}
			if len(srcStr) > 64*1024 {
				return []map[string]any{degradeBlock(rawBlock, path, "diagram", "diagrama mermaid excede 64 KB", warnings)}
			}
			return []map[string]any{{"t": "diagram", "kind": "mermaid", "src": srcStr}}
		}
		if kind != "flow" && kind != "seq" && kind != "tree" {
			return []map[string]any{degradeBlock(rawBlock, path, "diagram", fmt.Sprintf("kind inválido %q", kind), warnings)}
		}
		nodesAny, _ := rawBlock["nodes"].([]any)
		edgesAny, _ := rawBlock["edges"].([]any)
		svgStr, _ := asNonEmptyString(rawBlock["svg"])

		if len(nodesAny) == 0 && svgStr == "" {
			return []map[string]any{degradeBlock(rawBlock, path, "diagram", "diagrama sin nodes[]", warnings)}
		}
		if kind == "seq" && (len(nodesAny) > 12 || len(edgesAny) > 40) {
			return []map[string]any{degradeBlock(rawBlock, path, "diagram", fmt.Sprintf("excede límites seq (%d/12 nodos, %d/40 mensajes)", len(nodesAny), len(edgesAny)), warnings)}
		}
		if kind == "tree" && len(nodesAny) > 60 {
			return []map[string]any{degradeBlock(rawBlock, path, "diagram", fmt.Sprintf("excede límite tree (%d/60 nodos)", len(nodesAny)), warnings)}
		}
		if kind == "flow" && len(nodesAny) > 80 {
			return []map[string]any{degradeBlock(rawBlock, path, "diagram", fmt.Sprintf("excede límite flow (%d/80 nodos)", len(nodesAny)), warnings)}
		}

		type dNode struct {
			id    string
			label string
			sev   string
		}
		var parsedNodes []dNode
		cleanNodes := make([]map[string]any, 0, len(nodesAny))
		for i, nAny := range nodesAny {
			nMap, ok := nAny.(map[string]any)
			if !ok {
				return []map[string]any{degradeBlock(rawBlock, path, "diagram", fmt.Sprintf("nodes[%d] inválido", i), warnings)}
			}
			id, okI := asNonEmptyString(nMap["id"])
			lbl, okL := asNonEmptyString(nMap["label"])
			if !okI || !okL {
				return []map[string]any{degradeBlock(rawBlock, path, "diagram", fmt.Sprintf("nodes[%d] sin id o label", i), warnings)}
			}
			sev := sanitizeSev(nMap["sev"], fmt.Sprintf("%s.nodes[%d].sev", path, i), "", warnings)
			parsedNodes = append(parsedNodes, dNode{id: id, label: lbl, sev: sev})
			nm := map[string]any{"id": id, "label": lbl}
			if sev != "" {
				nm["sev"] = sev
			}
			cleanNodes = append(cleanNodes, nm)
		}

		var parsedEdges [][3]string
		cleanEdges := make([][]string, 0, len(edgesAny))
		for i, eAny := range edgesAny {
			eTuple, ok := expandEdgeTuple(eAny)
			if !ok {
				return []map[string]any{degradeBlock(rawBlock, path, "diagram", fmt.Sprintf("edges[%d] inválido", i), warnings)}
			}
			parsedEdges = append(parsedEdges, eTuple)
			if eTuple[2] != "" {
				cleanEdges = append(cleanEdges, []string{eTuple[0], eTuple[1], eTuple[2]})
			} else {
				cleanEdges = append(cleanEdges, []string{eTuple[0], eTuple[1]})
			}
		}

		nodeIDs := make([]string, len(parsedNodes))
		for i, n := range parsedNodes {
			nodeIDs[i] = n.id
		}
		if kind == "tree" && !IsStrictTree(nodeIDs, parsedEdges) {
			kind = "flow"
			*warnings = append(*warnings, Warning{
				Path:   path + ".kind",
				Action: "repaired",
				Reason: `estructura no arbórea reclasificada de "tree" a "flow"`,
			})
		}

		out := map[string]any{
			"t":     "diagram",
			"kind":  kind,
			"nodes": cleanNodes,
			"edges": cleanEdges,
		}
		if dirStr, ok := asNonEmptyString(rawBlock["direction"]); ok {
			if dirStr == "right" || dirStr == "down" || dirStr == "left" || dirStr == "up" {
				out["direction"] = dirStr
			}
		}
		if svgStr != "" {
			out["svg"] = svgStr
		}
		return []map[string]any{out}

	case "checklist":
		itemsAny, ok := rawBlock["items"].([]any)
		if !ok || len(itemsAny) == 0 {
			return []map[string]any{degradeBlock(rawBlock, path, "checklist", "falta items[]", warnings)}
		}
		cleanItems := make([]map[string]any, 0, len(itemsAny))
		for i, itAny := range itemsAny {
			itMap, ok := itAny.(map[string]any)
			if !ok {
				return []map[string]any{degradeBlock(rawBlock, path, "checklist", fmt.Sprintf("items[%d] inválido", i), warnings)}
			}
			id, okI := asNonEmptyString(itMap["id"])
			txt, okT := asNonEmptyString(itMap["text"])
			doneVal, okD := itMap["done"].(bool)
			if !okI || !okT || !okD {
				return []map[string]any{degradeBlock(rawBlock, path, "checklist", fmt.Sprintf("items[%d] carece de id, text o done(bool)", i), warnings)}
			}
			itemOut := map[string]any{
				"id":   id,
				"text": txt,
				"done": doneVal,
			}
			if owner, ok := asNonEmptyString(itMap["owner"]); ok {
				itemOut["owner"] = owner
			}
			if sev := sanitizeSev(itMap["sev"], fmt.Sprintf("%s.items[%d].sev", path, i), "", warnings); sev != "" {
				itemOut["sev"] = sev
			}
			cleanItems = append(cleanItems, itemOut)
		}
		out := map[string]any{
			"t":     "checklist",
			"items": cleanItems,
		}
		if asBool(rawBlock["readonly"]) {
			out["readonly"] = true
		}
		return []map[string]any{out}

	case "prose":
		md, ok := asString(rawBlock["md"])
		if !ok {
			return []map[string]any{degradeBlock(rawBlock, path, "prose", "falta md", warnings)}
		}
		out := map[string]any{
			"t":  "prose",
			"md": md,
		}
		if title, ok := asNonEmptyString(rawBlock["title"]); ok {
			out["title"] = title
		}
		if rawBlock["degraded"] != nil {
			out["degraded"] = rawBlock["degraded"]
		}
		return []map[string]any{out}

	case "canvas":
		srcStr, hasSrc := asNonEmptyString(rawBlock["src"])
		htmlStr, hasHTML := asString(rawBlock["html"])
		if !hasSrc && !hasHTML {
			return []map[string]any{degradeBlock(rawBlock, path, "canvas", "falta html o src", warnings)}
		}
		if hasHTML && len(htmlStr) > 200*1024 {
			return []map[string]any{degradeBlock(rawBlock, path, "canvas", "html excede el límite de 200 KB", warnings)}
		}
		h := 320
		if hNum, ok := asInt(rawBlock["h"]); ok {
			if hNum < 120 {
				h = 120
			} else if hNum > 1200 {
				h = 1200
			} else {
				h = hNum
			}
		}
		out := map[string]any{
			"t": "canvas",
			"h": h,
		}
		if hasSrc {
			out["src"] = srcStr
		}
		if hasHTML {
			out["html"] = htmlStr
		}
		return []map[string]any{out}
	}

	return []map[string]any{degradeBlock(rawBlock, path, tStr, "tipo no soportado", warnings)}
}

func degradeBlock(rawAny any, path, fromType, reason string, warnings *[]Warning) map[string]any {
	*warnings = append(*warnings, Warning{
		Path:   path,
		Action: "degraded",
		Reason: fmt.Sprintf("%s: %s", fromType, reason),
	})
	meta := DegradedMeta{From: fromType, Reason: reason}

	if rawMap, ok := rawAny.(map[string]any); ok && rawMap != nil {
		// (a) Si tiene src o html string -> code
		if srcStr, okS := asString(rawMap["src"]); okS {
			lang, _ := asNonEmptyString(rawMap["lang"])
			if lang == "" {
				lang = "text"
			}
			return map[string]any{
				"t":        "code",
				"lang":     lang,
				"src":      truncateLines(srcStr, 2000, path+".src", warnings),
				"degraded": meta,
			}
		}
		if htmlStr, okH := asString(rawMap["html"]); okH {
			lang, _ := asNonEmptyString(rawMap["lang"])
			if lang == "" {
				lang = "html"
			}
			return map[string]any{
				"t":        "code",
				"lang":     lang,
				"src":      truncateLines(htmlStr, 2000, path+".html", warnings),
				"degraded": meta,
			}
		}

		// (b) Si tiene title, body, md o text -> prose
		var bodyCandidate string
		for _, k := range []string{"body", "md", "text"} {
			if s, ok := asNonEmptyString(rawMap[k]); ok {
				bodyCandidate = s
				break
			}
		}
		titleCandidate, hasTitle := asNonEmptyString(rawMap["title"])
		if bodyCandidate != "" || hasTitle {
			mdVal := bodyCandidate
			if mdVal == "" {
				mdVal = titleCandidate
			}
			out := map[string]any{
				"t":        "prose",
				"md":       mdVal,
				"degraded": meta,
			}
			if hasTitle && bodyCandidate != "" {
				out["title"] = titleCandidate
			}
			return out
		}
	}

	// (c) En cualquier otro caso -> code json con el bloque original formateado
	formatted, err := json.MarshalIndent(rawAny, "", "  ")
	if err != nil {
		formatted = []byte(fmt.Sprintf("%v", rawAny))
	}
	return map[string]any{
		"t":        "code",
		"lang":     "json",
		"src":      truncateLines(string(formatted), 2000, path, warnings),
		"degraded": meta,
	}
}

// IsStrictTree verifica si los nodos y aristas forman un árbol estricto con raíz única.
func IsStrictTree(nodeIDs []string, edges [][3]string) bool {
	if len(nodeIDs) <= 1 {
		return len(edges) == 0
	}
	idSet := make(map[string]bool, len(nodeIDs))
	indegree := make(map[string]int, len(nodeIDs))
	adj := make(map[string][]string, len(nodeIDs))
	for _, id := range nodeIDs {
		idSet[id] = true
	}
	for _, e := range edges {
		u, v := e[0], e[1]
		if !idSet[u] || !idSet[v] || u == v {
			return false
		}
		indegree[v]++
		if indegree[v] > 1 {
			return false
		}
		adj[u] = append(adj[u], v)
	}
	var roots []string
	for _, id := range nodeIDs {
		if indegree[id] == 0 {
			roots = append(roots, id)
		}
	}
	if len(roots) != 1 {
		return false
	}
	visited := make(map[string]bool, len(nodeIDs))
	stack := []string{roots[0]}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[cur] {
			return false
		}
		visited[cur] = true
		stack = append(stack, adj[cur]...)
	}
	return len(visited) == len(nodeIDs)
}

// ExtractMaxSeverity recorre recursivamente los bloques para hallar la severidad dominante.
func ExtractMaxSeverity(blocks []map[string]any) string {
	best := "info"
	bestRank := 0

	var walk func(list []map[string]any)
	walk = func(list []map[string]any) {
		for _, b := range list {
			if sev, ok := b["sev"].(string); ok {
				if r := validSeverities[sev]; r > bestRank {
					bestRank = r
					best = sev
				}
			}
			if items, ok := b["items"].([]map[string]any); ok {
				for _, it := range items {
					if sev, ok := it["sev"].(string); ok {
						if r := validSeverities[sev]; r > bestRank {
							bestRank = r
							best = sev
						}
					}
				}
			} else if itemsAny, ok := b["items"].([]any); ok {
				for _, itAny := range itemsAny {
					if it, ok := itAny.(map[string]any); ok {
						if sev, ok := it["sev"].(string); ok {
							if r := validSeverities[sev]; r > bestRank {
								bestRank = r
								best = sev
							}
						}
					}
				}
			}
			if sub, ok := b["blocks"].([]map[string]any); ok {
				walk(sub)
			} else if subAny, ok := b["blocks"].([]any); ok {
				var conv []map[string]any
				for _, x := range subAny {
					if m, ok := x.(map[string]any); ok {
						conv = append(conv, m)
					}
				}
				walk(conv)
			}
		}
	}
	walk(blocks)
	return best
}

// ExtractExcerpt extrae un resumen corto de 1 línea para la barra lateral.
func ExtractExcerpt(blocks []map[string]any, fallback string) string {
	if s := firstExcerptText(blocks); s != "" {
		return s
	}
	return fallback
}

var excerptLineMarkRe = regexp.MustCompile(`(?m)^\s*(?:#{1,6}\s+|>\s?|[-*+]\s+|\d+[.)]\s+)`)

// firstExcerptText busca en profundidad el primer texto legible: entra en las
// secciones antes de usar su título y limpia marcas de Markdown de inicio de línea.
func firstExcerptText(blocks []map[string]any) string {
	replacer := strings.NewReplacer("*", "", "`", "", "[", "", "]", "", "~~", "", "\n", " ", "\r", " ")
	for _, b := range blocks {
		if t, _ := b["t"].(string); t == "section" {
			var children []map[string]any
			switch cb := b["blocks"].(type) {
			case []map[string]any:
				children = cb
			case []any:
				for _, c := range cb {
					if m, ok := c.(map[string]any); ok {
						children = append(children, m)
					}
				}
			}
			if s := firstExcerptText(children); s != "" {
				return s
			}
		}
		for _, key := range []string{"md", "body", "title"} {
			if s, ok := asNonEmptyString(b[key]); ok {
				clean := excerptLineMarkRe.ReplaceAllString(s, "")
				clean = strings.TrimSpace(replacer.Replace(clean))
				clean = strings.Join(strings.Fields(clean), " ")
				runes := []rune(clean)
				if len(runes) > 96 {
					return string(runes[:95]) + "…"
				}
				if clean != "" {
					return clean
				}
			}
		}
	}
	return ""
}

// CollectChecklists extrae todas las casillas actuales del documento para get_report_state.
func CollectChecklists(blocks []map[string]any) []ChecklistItemState {
	var out []ChecklistItemState
	var walk func(list []map[string]any)
	walk = func(list []map[string]any) {
		for _, b := range list {
			t, _ := b["t"].(string)
			if t == "checklist" {
				switch items := b["items"].(type) {
				case []map[string]any:
					for _, it := range items {
						out = append(out, mapToCheckState(it))
					}
				case []any:
					for _, rawIt := range items {
						if it, ok := rawIt.(map[string]any); ok {
							out = append(out, mapToCheckState(it))
						}
					}
				}
			}
			if sub, ok := b["blocks"].([]map[string]any); ok {
				walk(sub)
			} else if subAny, ok := b["blocks"].([]any); ok {
				var conv []map[string]any
				for _, x := range subAny {
					if m, ok := x.(map[string]any); ok {
						conv = append(conv, m)
					}
				}
				walk(conv)
			}
		}
	}
	walk(blocks)
	return out
}

func mapToCheckState(it map[string]any) ChecklistItemState {
	id, _ := it["id"].(string)
	txt, _ := it["text"].(string)
	done, _ := it["done"].(bool)
	owner, _ := it["owner"].(string)
	sev, _ := it["sev"].(string)
	return ChecklistItemState{
		ID:    id,
		Text:  txt,
		Done:  done,
		Owner: owner,
		Sev:   sev,
	}
}

func sanitizeSev(val any, path, fallback string, warnings *[]Warning) string {
	if val == nil {
		return fallback
	}
	s, ok := val.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return fallback
	}
	if _, valid := validSeverities[s]; valid {
		return s
	}
	reason := fmt.Sprintf("severidad inválida %q omitida", s)
	if fallback != "" {
		reason = fmt.Sprintf("severidad inválida %q reemplazada por %q", s, fallback)
	}
	*warnings = append(*warnings, Warning{
		Path:   path,
		Action: "repaired",
		Reason: reason,
	})
	return fallback
}

func truncateLines(s string, maxLines int, path string, warnings *[]Warning) string {
	normalized := strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) <= maxLines {
		return normalized
	}
	*warnings = append(*warnings, Warning{
		Path:   path,
		Action: "repaired",
		Reason: fmt.Sprintf("truncado de %d a %d líneas", len(lines), maxLines),
	})
	return strings.Join(lines[:maxLines], "\n") + fmt.Sprintf("\n// … truncado a %d líneas por límite de contrato", maxLines)
}

func expandEdgeTuple(v any) ([3]string, bool) {
	switch arr := v.(type) {
	case []string:
		if len(arr) < 2 || strings.TrimSpace(arr[0]) == "" || strings.TrimSpace(arr[1]) == "" {
			return [3]string{}, false
		}
		lbl := ""
		if len(arr) >= 3 {
			lbl = arr[2]
		}
		return [3]string{arr[0], arr[1], lbl}, true
	case []any:
		if len(arr) < 2 {
			return [3]string{}, false
		}
		from, okF := asNonEmptyString(arr[0])
		to, okT := asNonEmptyString(arr[1])
		if !okF || !okT {
			return [3]string{}, false
		}
		lbl := ""
		if len(arr) >= 3 {
			lbl, _ = asString(arr[2])
		}
		return [3]string{from, to, lbl}, true
	default:
		return [3]string{}, false
	}
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asNonEmptyString(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	return s, s != ""
}

func asBool(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if math.Trunc(n) == n {
			return int(n), true
		}
		return int(math.Round(n)), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

func normalizeScalarValue(v any) (any, bool) {
	switch x := v.(type) {
	case string:
		if strings.TrimSpace(x) == "" {
			return nil, false
		}
		return x, true
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, false
		}
		if math.Trunc(x) == x {
			return int64(x), true
		}
		return x, true
	case int:
		return x, true
	case int64:
		return x, true
	case json.Number:
		return x.String(), true
	default:
		return nil, false
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
