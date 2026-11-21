package render

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"dioptra/internal/contract"
)

var (
	calloutTagRe = regexp.MustCompile(`(?i)^\[!(NOTE|INFO|TIP|SUCCESS|WARNING|CAUTION|IMPORTANT|ERROR|DANGER)\]\s*(.*)$`)
	ownerTokenRe = regexp.MustCompile(`(?:^|\s)(@[A-Za-z0-9_.-]+)`)
	sevTagRe     = regexp.MustCompile(`(?i)\[(info|success|warning|error)\]`)
	refInlineRe  = regexp.MustCompile(`([A-Za-z0-9_./-]+\.[A-Za-z0-9_]+:\d+(?::\d+)?)`)
	edgeLineRe   = regexp.MustCompile(`^\s*([A-Za-z0-9_.-]+)\s*(?:-->|->|=>)\s*([A-Za-z0-9_.-]+)(?:\s*:\s*(.+))?\s*$`)
	nodeDeclRe   = regexp.MustCompile(`^\s*([A-Za-z0-9_.-]+)\s*:\s*"?([^"\n]+)"?\s*$`)
	taskLineRe   = regexp.MustCompile(`^(\s*[-*+]\s+\[)([ xX])(\]\s+.*)$`)
	fenceLineRe  = regexp.MustCompile(`^\s*(` + "```" + `|~~~)`)
	numericValRe = regexp.MustCompile(`^[-+]?\d+(?:\.\d+)?%?$`)
)

// FrontmatterMeta contiene metadatos opcionales extraídos del encabezado de un .md.
type FrontmatterMeta struct {
	Title   string
	Project string
	TS      string
	Body    string
}

// ParseFrontmatter extrae metadatos simples YAML (title, project, ts) si existen,
// sin alterar el archivo original guardado en disco.
func ParseFrontmatter(md string) FrontmatterMeta {
	normalized := strings.ReplaceAll(md, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return FrontmatterMeta{Body: md}
	}
	endIdx := strings.Index(normalized[4:], "\n---\n")
	if endIdx < 0 {
		return FrontmatterMeta{Body: md}
	}
	header := normalized[4 : 4+endIdx]
	body := normalized[4+endIdx+5:]
	meta := FrontmatterMeta{Body: body}
	for _, line := range strings.Split(header, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		switch k {
		case "title":
			meta.Title = v
		case "project":
			meta.Project = v
		case "ts", "date":
			meta.TS = v
		}
	}
	return meta
}

// ConvertMarkdownToEnvelope transforma determinísticamente un documento Markdown
// al sobre JSON del Contrato B usando Goldmark (GFM) + Chroma + D2 en milisegundos
// y con 0 consumo de tokens de LLM.
func ConvertMarkdownToEnvelope(ctx context.Context, title, project, ts, mdContent string) (*contract.Envelope, []contract.Warning, error) {
	fm := ParseFrontmatter(mdContent)
	if strings.TrimSpace(title) == "" {
		title = fm.Title
	}
	if strings.TrimSpace(project) == "" {
		project = fm.Project
	}
	if strings.TrimSpace(ts) == "" {
		ts = fm.TS
	}
	if strings.TrimSpace(ts) == "" {
		ts = time.Now().Format(time.RFC3339)
	}
	if strings.TrimSpace(project) == "" {
		project = "dioptra"
	}

	srcBytes := []byte(strings.ReplaceAll(fm.Body, "\r\n", "\n"))
	doc := mdParser.Parser().Parse(text.NewReader(srcBytes))
	title, rawBlocks := newMDConverter(srcBytes, title).convert(doc)

	if strings.TrimSpace(title) == "" {
		title = "Reporte Técnico"
	}
	if len(rawBlocks) == 0 {
		rawBlocks = append(rawBlocks, map[string]any{
			"t":  "prose",
			"md": "(Documento sin contenido)",
		})
	}

	rootMap := map[string]any{
		"v": 1,
		"report": map[string]any{
			"project": project,
			"title":   title,
			"ts":      ts,
			"blocks":  rawBlocks,
		},
	}

	env, warnings, err := contract.ValidateAndNormalizeMap(rootMap)
	if err != nil {
		return nil, nil, err
	}
	EnrichBlocks(ctx, env.Report.Blocks)
	return env, warnings, nil
}

func extractCodeLines(n ast.Node, source []byte) string {
	var buf bytes.Buffer
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(seg.Value(source))
	}
	return strings.TrimRight(buf.String(), "\r\n")
}

func extractInlineMarkdown(n ast.Node, source []byte) string {
	var buf strings.Builder
	var walk func(cur ast.Node)
	walk = func(cur ast.Node) {
		for c := cur.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *east.TaskCheckBox:
				// Omitido: se procesa explícitamente en convertTaskListToChecklist
			case *ast.Text:
				buf.Write(v.Segment.Value(source))
				if v.SoftLineBreak() || v.HardLineBreak() {
					buf.WriteByte(' ')
				}
			case *ast.String:
				buf.Write(v.Value)
			case *ast.CodeSpan:
				buf.WriteByte('`')
				for gc := v.FirstChild(); gc != nil; gc = gc.NextSibling() {
					if t, ok := gc.(*ast.Text); ok {
						buf.Write(t.Segment.Value(source))
					}
				}
				buf.WriteByte('`')
			case *ast.Emphasis:
				mark := "*"
				if v.Level >= 2 {
					mark = "**"
				}
				buf.WriteString(mark)
				walk(v)
				buf.WriteString(mark)
			case *east.Strikethrough:
				buf.WriteString("~~")
				walk(v)
				buf.WriteString("~~")
			case *ast.Image:
				buf.WriteString("![")
				walk(v)
				buf.WriteString("](")
				buf.Write(v.Destination)
				buf.WriteByte(')')
			case *ast.RawHTML:
				for i := 0; i < v.Segments.Len(); i++ {
					seg := v.Segments.At(i)
					buf.Write(seg.Value(source))
				}
			case *ast.Link:
				buf.WriteByte('[')
				walk(v)
				buf.WriteString("](")
				buf.Write(v.Destination)
				buf.WriteByte(')')
			case *ast.AutoLink:
				urlBytes := v.URL(source)
				buf.WriteByte('[')
				buf.Write(urlBytes)
				buf.WriteString("](")
				buf.Write(urlBytes)
				buf.WriteByte(')')
			default:
				walk(c)
			}
		}
	}
	walk(n)
	return buf.String()
}

func extractListItemText(li ast.Node, source []byte) string {
	var parts []string
	for c := li.FirstChild(); c != nil; c = c.NextSibling() {
		if _, isSubList := c.(*ast.List); isSubList {
			continue
		}
		txt := strings.TrimSpace(extractInlineMarkdown(c, source))
		if txt != "" {
			parts = append(parts, txt)
		}
	}
	return strings.Join(parts, " ")
}

func isTaskList(list *ast.List) bool {
	for li := list.FirstChild(); li != nil; li = li.NextSibling() {
		firstBlock := li.FirstChild()
		if firstBlock != nil && firstBlock.FirstChild() != nil {
			if _, ok := firstBlock.FirstChild().(*east.TaskCheckBox); ok {
				return true
			}
		}
	}
	return false
}

func convertBlockquoteToCallout(bq *ast.Blockquote, source []byte) map[string]any {
	// Cada hijo se serializa como Markdown (listas y código incluidos) para no
	// aplanar ni perder contenido dentro del callout.
	var paras []string
	for c := bq.FirstChild(); c != nil; c = c.NextSibling() {
		var t string
		switch v := c.(type) {
		case *ast.List:
			t = listMarkdown(v, source, 0)
		case *ast.FencedCodeBlock:
			t = "```" + strings.TrimSpace(string(v.Language(source))) + "\n" + extractCodeLines(v, source) + "\n```"
		default:
			t = strings.TrimSpace(extractInlineMarkdown(c, source))
		}
		if strings.TrimSpace(t) != "" {
			paras = append(paras, t)
		}
	}
	if len(paras) == 0 {
		return map[string]any{"t": "callout", "sev": "info", "title": "Nota", "body": ""}
	}

	first := paras[0]
	sev := "info"
	title := "Hallazgo técnico"
	body := strings.Join(paras, "\n\n")

	if m := calloutTagRe.FindStringSubmatch(first); len(m) == 3 {
		tag := strings.ToUpper(m[1])
		switch tag {
		case "ERROR", "DANGER", "CAUTION":
			sev = "error"
		case "WARNING", "IMPORTANT":
			sev = "warning"
		case "SUCCESS", "TIP":
			sev = "success"
		default:
			sev = "info"
		}
		restTitle := strings.TrimSpace(m[2])
		// "> [!WARNING] Título\n> detalle" llega como UN párrafo (salto suave):
		// el título es solo la primera línea y el resto del párrafo es cuerpo.
		var firstParaRest string
		if p, ok := bq.FirstChild().(*ast.Paragraph); ok && p.Lines().Len() > 1 {
			lines := p.Lines()
			seg0 := lines.At(0)
			line0 := strings.TrimSpace(string(seg0.Value(source)))
			if lm := calloutTagRe.FindStringSubmatch(line0); len(lm) == 3 {
				restTitle = strings.TrimSpace(lm[2])
				var rest []string
				for i := 1; i < lines.Len(); i++ {
					segI := lines.At(i)
					if l := strings.TrimSpace(string(segI.Value(source))); l != "" {
						rest = append(rest, l)
					}
				}
				firstParaRest = strings.Join(rest, " ")
			}
		}
		if restTitle != "" {
			title = restTitle
		} else {
			title = tag
		}
		var bodyParts []string
		if firstParaRest != "" {
			bodyParts = append(bodyParts, firstParaRest)
		}
		bodyParts = append(bodyParts, paras[1:]...)
		if len(bodyParts) > 0 {
			body = strings.Join(bodyParts, "\n\n")
		} else {
			body = title
		}
	} else if len(paras) > 1 {
		title = strings.Trim(first, "*")
		body = strings.Join(paras[1:], "\n\n")
	} else {
		// Callout de un solo párrafo sin etiqueta: el texto es el cuerpo.
		title = "Nota"
	}

	out := map[string]any{
		"t":     "callout",
		"sev":   sev,
		"title": title,
		"body":  body,
	}
	if m := refInlineRe.FindStringSubmatch(body); len(m) == 2 {
		out["ref"] = m[1]
	}
	return out
}

func convertTableNode(tbl *east.Table, source []byte) map[string]any {
	var headers []string
	var rows [][]string

	for rowNode := tbl.FirstChild(); rowNode != nil; rowNode = rowNode.NextSibling() {
		var cells []string
		for cell := rowNode.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, strings.TrimSpace(extractInlineMarkdown(cell, source)))
		}
		if _, isHeader := rowNode.(*east.TableHeader); isHeader {
			headers = cells
		} else {
			rows = append(rows, cells)
		}
	}

	if len(headers) == 0 {
		return nil
	}

	cols := make([]any, len(headers))
	colKeys := make([]string, len(headers))
	for i, h := range headers {
		key := fmt.Sprintf("c%d", i)
		colKeys[i] = key
		colType := inferColumnType(rows, i)
		cols[i] = map[string]any{
			"key":   key,
			"label": h,
			"type":  colType,
		}
	}

	rowMaps := make([]any, 0, len(rows))
	for _, r := range rows {
		m := make(map[string]any, len(headers))
		for i := range headers {
			val := ""
			if i < len(r) {
				val = r[i]
			}
			m[colKeys[i]] = val
		}
		rowMaps = append(rowMaps, m)
	}

	return map[string]any{
		"t":      "table",
		"cols":   cols,
		"rows":   rowMaps,
		"filter": len(rowMaps) >= 4,
	}
}

func inferColumnType(rows [][]string, colIdx int) string {
	if len(rows) == 0 {
		return "text"
	}
	allSev := true
	allNum := true
	allRef := true
	nonEmpty := 0

	for _, r := range rows {
		if colIdx >= len(r) {
			continue
		}
		v := strings.TrimSpace(strings.Trim(r[colIdx], "`"))
		if v == "" {
			continue
		}
		nonEmpty++
		low := strings.ToLower(v)
		if low != "info" && low != "success" && low != "warning" && low != "error" {
			allSev = false
		}
		if !numericValRe.MatchString(strings.ReplaceAll(v, ",", "")) {
			allNum = false
		}
		if !refInlineRe.MatchString(v) {
			allRef = false
		}
	}

	if nonEmpty == 0 {
		return "text"
	}
	if allSev {
		return "sev"
	}
	if allRef {
		return "ref"
	}
	if allNum {
		return "num"
	}
	return "text"
}

func convertCodeFence(lang, codeStr string) map[string]any {
	switch lang {
	case "kpi":
		var items []any
		for _, line := range strings.Split(codeStr, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) >= 2 {
				it := map[string]any{
					"label": strings.TrimSpace(parts[0]),
					"value": strings.TrimSpace(parts[1]),
				}
				if len(parts) >= 3 && strings.TrimSpace(parts[2]) != "" {
					it["unit"] = strings.TrimSpace(parts[2])
				}
				if len(parts) >= 4 && strings.TrimSpace(parts[3]) != "" {
					it["delta"] = strings.TrimSpace(parts[3])
				}
				if len(parts) >= 5 && strings.TrimSpace(parts[4]) != "" {
					it["sev"] = strings.ToLower(strings.TrimSpace(parts[4]))
				}
				items = append(items, it)
			}
		}
		if len(items) > 0 {
			return map[string]any{"t": "kpi", "items": items}
		}

	case "d2", "flow", "seq", "tree", "mermaid":
		if diag := parseDiagramFence(lang, codeStr); diag != nil {
			return diag
		}

	case "diff", "patch":
		// Un diff unificado (varios archivos/hunks, cabeceras @@ con contexto) se
		// conserva íntegro y se resalta como diff; partirlo en antes/después perdía
		// las cabeceras y el archivo de cada hunk.
		return map[string]any{
			"t":    "code",
			"lang": "diff",
			"src":  codeStr,
		}

	case "html-canvas", "free-canvas":
		return map[string]any{
			"t":    "canvas",
			"html": codeStr,
			"h":    340,
		}
	}

	if lang == "" {
		lang = "text"
	}
	return map[string]any{
		"t":    "code",
		"lang": lang,
		"src":  codeStr,
	}
}

// ToggleMarkdownCheckbox sincroniza bidireccionalmente el estado de una casilla
// (`- [ ]` <-> `- [x]`) en el archivo .md fuente de verdad, respetando bloques de código.
func ToggleMarkdownCheckbox(mdContent, checkID, matchText string, done bool) (string, bool) {
	useCRLF := strings.Contains(mdContent, "\r\n")
	lines := strings.Split(strings.ReplaceAll(mdContent, "\r\n", "\n"), "\n")

	targetIndex := 0
	if strings.HasPrefix(checkID, "chk-") {
		if n, err := strconv.Atoi(strings.TrimPrefix(checkID, "chk-")); err == nil && n > 0 {
			targetIndex = n
		}
	}

	mark := " "
	if done {
		mark = "x"
	}

	inFence := false
	seenTasks := 0
	updated := false
	cleanNeedle := strings.ToLower(strings.TrimSpace(matchText))

	for i, line := range lines {
		if fenceLineRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := taskLineRe.FindStringSubmatch(line)
		if len(m) != 4 {
			continue
		}
		seenTasks++
		matched := false
		if targetIndex > 0 && seenTasks == targetIndex {
			matched = true
		} else if targetIndex == 0 && checkID != "" && strings.Contains(line, checkID) {
			matched = true
		} else if targetIndex == 0 && cleanNeedle != "" && strings.Contains(strings.ToLower(m[3]), cleanNeedle) {
			matched = true
		}
		if matched {
			lines[i] = m[1] + mark + m[3]
			updated = true
			break
		}
	}

	sep := "\n"
	if useCRLF {
		sep = "\r\n"
	}
	return strings.Join(lines, sep), updated
}
