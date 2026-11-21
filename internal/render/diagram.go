package render

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math"
	"strings"
	"sync"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/lib/geo"
	d2log "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

var (
	rulerOnce sync.Once
	sharedRlr *textmeasure.Ruler
	rulerErr  error
	d2Mu      sync.Mutex
)

func getRuler() (*textmeasure.Ruler, error) {
	rulerOnce.Do(func() {
		sharedRlr, rulerErr = textmeasure.NewRuler()
	})
	return sharedRlr, rulerErr
}

// CompileFlowDiagramSVG compila un bloque `diagram` (`kind: "flow"`) usando el motor
// de grafos dirigidos D2 (Dagre layout) y emite un SVG limpio con clases CSS `dg-*`
// compatibles con el conmutador de temas en vivo sin regenerar el SVG (§4.6 ADR 2).
func CompileFlowDiagramSVG(ctx context.Context, block map[string]any) (string, error) {
	type nodeItem struct {
		id    string
		label string
		sev   string
	}

	var nodes []nodeItem
	sevByID := make(map[string]string)
	labelByID := make(map[string]string)

	extractNode := func(m map[string]any) {
		id, _ := m["id"].(string)
		lbl, _ := m["label"].(string)
		sev, _ := m["sev"].(string)
		if id != "" {
			if lbl == "" {
				lbl = id
			}
			nodes = append(nodes, nodeItem{id: id, label: lbl, sev: sev})
			sevByID[id] = sev
			labelByID[id] = lbl
		}
	}

	switch rawNodes := block["nodes"].(type) {
	case []map[string]any:
		for _, m := range rawNodes {
			extractNode(m)
		}
	case []any:
		for _, x := range rawNodes {
			if m, ok := x.(map[string]any); ok {
				extractNode(m)
			}
		}
	}

	if len(nodes) == 0 {
		return "", fmt.Errorf("diagrama flow sin nodos")
	}

	var edges [][3]string
	switch rawEdges := block["edges"].(type) {
	case [][]string:
		for _, e := range rawEdges {
			if len(e) >= 2 {
				lbl := ""
				if len(e) >= 3 {
					lbl = e[2]
				}
				edges = append(edges, [3]string{e[0], e[1], lbl})
			}
		}
	case []any:
		for _, x := range rawEdges {
			if arr, ok := x.([]any); ok && len(arr) >= 2 {
				from, _ := arr[0].(string)
				to, _ := arr[1].(string)
				lbl := ""
				if len(arr) >= 3 {
					lbl, _ = arr[2].(string)
				}
				if from != "" && to != "" {
					edges = append(edges, [3]string{from, to, lbl})
				}
			}
		}
	}

	var d2Buf strings.Builder
	d2Buf.WriteString("direction: right\n")
	safeIDMap := make(map[string]string, len(nodes))
	reverseIDMap := make(map[string]string, len(nodes))
	for i, n := range nodes {
		safeID := fmt.Sprintf("n_%d", i)
		safeIDMap[n.id] = safeID
		reverseIDMap[safeID] = n.id
		// Las etiquetas largas se parten en 2 líneas (no se recortan) y D2 recibe el
		// salto para dimensionar el nodo con el alto correcto.
		lines := wrapNodeLabel(n.label)
		for li := range lines {
			lines[li] = strings.ReplaceAll(strings.ReplaceAll(lines[li], `\`, `\\`), `"`, `\"`)
		}
		fmt.Fprintf(&d2Buf, "%s: \"%s\"\n", safeID, strings.Join(lines, `\n`))
	}
	for _, e := range edges {
		srcSafe, okS := safeIDMap[e[0]]
		dstSafe, okD := safeIDMap[e[1]]
		if !okS || !okD {
			continue
		}
		if strings.TrimSpace(e[2]) != "" {
			escapedEdge := strings.ReplaceAll(strings.ReplaceAll(e[2], `\`, `\\`), `"`, `\"`)
			fmt.Fprintf(&d2Buf, "%s -> %s: \"%s\"\n", srcSafe, dstSafe, escapedEdge)
		} else {
			fmt.Fprintf(&d2Buf, "%s -> %s\n", srcSafe, dstSafe)
		}
	}

	cacheKey := svgCacheKey(d2Buf.String(), labelByID, sevByID)
	if cached, ok := svgCacheGet(cacheKey); ok {
		return cached, nil
	}

	ruler, err := getRuler()
	if err != nil {
		return "", fmt.Errorf("error inicializando ruler D2: %w", err)
	}

	d2Mu.Lock()
	compileOpts := &d2lib.CompileOptions{
		Ruler: ruler,
		LayoutResolver: func(_ string) (d2graph.LayoutGraph, error) {
			return d2dagrelayout.DefaultLayout, nil
		},
	}
	pad := int64(28)
	renderOpts := &d2svg.RenderOpts{
		Pad: &pad,
	}
	d2Ctx := d2log.With(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	diagram, _, err := d2lib.Compile(d2Ctx, d2Buf.String(), compileOpts, renderOpts)
	d2Mu.Unlock()
	if err != nil {
		return "", fmt.Errorf("error compilando grafo D2: %w", err)
	}

	tl, br := diagram.BoundingBox()
	padPx := 28.0
	vbX := float64(tl.X) - padPx
	vbY := float64(tl.Y) - padPx
	vbW := math.Max(320, float64(br.X-tl.X)+padPx*2)
	vbH := math.Max(160, float64(br.Y-tl.Y)+padPx*2)

	var svg strings.Builder
	fmt.Fprintf(&svg, `<svg viewBox="%.1f %.1f %.1f %.1f" xmlns="http://www.w3.org/2000/svg">`, vbX, vbY, vbW, vbH)
	svg.WriteString(`<defs><marker id="d2-flow-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 1.5 L 10 5 L 0 8.5 z" class="dg-arrow"/></marker></defs>`)

	for _, conn := range diagram.Connections {
		if len(conn.Route) < 2 {
			continue
		}
		svg.WriteString(`<g class="dg-edge">`)
		pathData := buildSVGPathFromRoute(conn.Route, conn.IsCurve)
		fmt.Fprintf(&svg, `<path d="%s" marker-end="url(#d2-flow-arrow)"/>`, pathData)

		if lbl := strings.TrimSpace(conn.Label); lbl != "" {
			runes := []rune(lbl)
			shortLbl := lbl
			if len(runes) > 26 {
				shortLbl = string(runes[:25]) + "…"
			}
			lw := float64(len([]rune(shortLbl)))*6.8 + 14.0
			var cx, cy float64
			if lblTL := conn.GetLabelTopLeft(); lblTL != nil {
				cx = lblTL.X + float64(conn.LabelWidth)/2.0
				cy = lblTL.Y + float64(conn.LabelHeight)/2.0
			} else {
				mid := conn.Route[len(conn.Route)/2]
				cx = mid.X
				cy = mid.Y - 10
			}
			fmt.Fprintf(&svg, `<rect x="%.1f" y="%.1f" width="%.1f" height="18" rx="4" class="dg-label-bg"/>`, cx-lw/2, cy-10, lw)
			fmt.Fprintf(&svg, `<text x="%.1f" y="%.1f" text-anchor="middle" class="dg-label">%s</text>`, cx, cy+3, html.EscapeString(shortLbl))
		}
		svg.WriteString(`</g>`)
	}

	for _, sh := range diagram.Shapes {
		origID := reverseIDMap[sh.ID]
		fullLabel := labelByID[origID]
		if fullLabel == "" {
			fullLabel = sh.Label
		}
		sev := sevByID[origID]
		sevCls := ""
		if sev == "info" || sev == "success" || sev == "warning" || sev == "error" {
			sevCls = " dg-sev-" + sev
		}

		x := float64(sh.Pos.X)
		y := float64(sh.Pos.Y)
		w := math.Max(104, float64(sh.Width))
		h := math.Max(40, float64(sh.Height))

		lines := wrapNodeLabel(fullLabel)
		if len(lines) > 1 {
			h = math.Max(h, 52)
		}

		fmt.Fprintf(&svg, `<g class="dg-node%s">`, sevCls)
		fmt.Fprintf(&svg, `<title>%s</title>`, html.EscapeString(fullLabel))
		fmt.Fprintf(&svg, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="8"/>`, x, y, w, h)
		const lineH = 15.0
		firstY := y + h/2 + 4.5 - lineH*float64(len(lines)-1)/2
		fmt.Fprintf(&svg, `<text x="%.1f" y="%.1f" text-anchor="middle" class="dg-node-label">`, x+w/2, firstY)
		for li, line := range lines {
			if li == 0 {
				fmt.Fprintf(&svg, `<tspan x="%.1f">%s</tspan>`, x+w/2, html.EscapeString(line))
			} else {
				fmt.Fprintf(&svg, `<tspan x="%.1f" dy="%.1f">%s</tspan>`, x+w/2, lineH, html.EscapeString(line))
			}
		}
		svg.WriteString(`</text></g>`)
	}

	svg.WriteString(`</svg>`)
	out := svg.String()
	svgCachePut(cacheKey, out)
	return out, nil
}

// wrapNodeLabel parte una etiqueta de nodo en hasta 2 líneas de ≤ 22 caracteres
// por el espacio más cercano al centro; solo recorta (con …) lo que no cabe ni así.
func wrapNodeLabel(label string) []string {
	const maxLine = 22
	runes := []rune(strings.TrimSpace(label))
	if len(runes) <= maxLine {
		return []string{string(runes)}
	}
	best := -1
	for i, r := range runes {
		if r == ' ' && i > 0 && i < len(runes)-1 {
			if best < 0 || absInt(i-len(runes)/2) < absInt(best-len(runes)/2) {
				best = i
			}
		}
	}
	clip := func(s []rune) string {
		if len(s) > maxLine {
			return string(s[:maxLine-1]) + "…"
		}
		return string(s)
	}
	if best < 0 {
		return []string{clip(runes)}
	}
	first := []rune(strings.TrimSpace(string(runes[:best])))
	second := []rune(strings.TrimSpace(string(runes[best+1:])))
	return []string{clip(first), clip(second)}
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func buildSVGPathFromRoute(pts []*geo.Point, isCurve bool) string {
	if len(pts) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "M %.1f %.1f", pts[0].X, pts[0].Y)
	if isCurve && len(pts) >= 4 && (len(pts)-1)%3 == 0 {
		for i := 1; i+2 < len(pts); i += 3 {
			fmt.Fprintf(&sb, " C %.1f %.1f, %.1f %.1f, %.1f %.1f",
				pts[i].X, pts[i].Y,
				pts[i+1].X, pts[i+1].Y,
				pts[i+2].X, pts[i+2].Y,
			)
		}
		return sb.String()
	}
	for i := 1; i < len(pts); i++ {
		fmt.Fprintf(&sb, " L %.1f %.1f", pts[i].X, pts[i].Y)
	}
	return sb.String()
}

var allowedSVGElements = map[string]bool{
	"svg": true, "g": true, "path": true, "rect": true, "circle": true,
	"ellipse": true, "line": true, "polyline": true, "polygon": true,
	"text": true, "tspan": true, "title": true, "defs": true, "marker": true,
}

var strippedSVGAttrs = map[string]bool{
	"style": true, "fill": true, "stroke": true, "font-family": true,
	"font-size": true, "font-weight": true,
}

// SanitizeSVG aplica la lista blanca estricta (§4.6) sobre cualquier SVG externo,
// rechazando script/foreignObject y eliminando estilos/colores inline.
func SanitizeSVG(rawSVG string) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(rawSVG))
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	depth := 0
	hasRoot := false

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("SVG XML inválido: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if name == "script" || name == "foreignobject" {
				return "", fmt.Errorf("elemento SVG prohibido: %s", name)
			}
			if !allowedSVGElements[name] {
				if err := dec.Skip(); err != nil {
					return "", err
				}
				continue
			}
			if depth == 0 {
				if name != "svg" {
					return "", fmt.Errorf("el nodo raíz debe ser svg")
				}
				hasRoot = true
			}
			var cleanAttrs []xml.Attr
			hasViewBox := false
			for _, a := range t.Attr {
				aName := strings.ToLower(a.Name.Local)
				if strings.HasPrefix(aName, "on") || strippedSVGAttrs[aName] {
					continue
				}
				if depth == 0 && (aName == "width" || aName == "height") {
					continue
				}
				if (aName == "href" || aName == "xlink:href") && !strings.HasPrefix(strings.TrimSpace(a.Value), "#") {
					continue
				}
				if aName == "viewbox" {
					hasViewBox = true
				}
				cleanAttrs = append(cleanAttrs, a)
			}
			if depth == 0 && !hasViewBox {
				cleanAttrs = append(cleanAttrs, xml.Attr{Name: xml.Name{Local: "viewBox"}, Value: "0 0 800 360"})
			}
			t.Attr = cleanAttrs
			if err := enc.EncodeToken(t); err != nil {
				return "", err
			}
			depth++

		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			if allowedSVGElements[name] {
				if err := enc.EncodeToken(t); err != nil {
					return "", err
				}
				depth--
			}

		case xml.CharData:
			if depth > 0 {
				if err := enc.EncodeToken(t); err != nil {
					return "", err
				}
			}
		}
	}
	if !hasRoot {
		return "", fmt.Errorf("SVG vacío")
	}
	if err := enc.Flush(); err != nil {
		return "", err
	}
	return out.String(), nil
}
