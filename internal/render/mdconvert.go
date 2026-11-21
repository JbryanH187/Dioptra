package render

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
)

// Parser goldmark reutilizable: goldmark.Markdown es seguro para uso concurrente
// y construirlo en cada conversión costaba asignaciones innecesarias.
var mdParser = goldmark.New(goldmark.WithExtensions(extension.GFM))

var (
	detailsOpenRe  = regexp.MustCompile(`(?is)^\s*<details\b[^>]*>`)
	detailsCloseRe = regexp.MustCompile(`(?i)</details\s*>`)
	summaryRe      = regexp.MustCompile(`(?is)<summary\b[^>]*>(.*?)</summary\s*>`)
	htmlTagRe      = regexp.MustCompile(`(?s)<[^>]+>`)
)

// container es un destino de bloques: la raíz del reporte o una sección abierta.
type container struct {
	blk    map[string]any // nil en la raíz
	kind   string         // "root" | "section" | "details"
	blocks []any
}

// mdConverter recorre el AST de goldmark y produce bloques del Contrato B sin
// descartar contenido en silencio:
//   - los encabezados del nivel principal (normalmente H2) abren secciones;
//   - los niveles inferiores quedan como encabezados dentro de la prosa;
//   - listas ordenadas/anidadas, citas, imágenes, <details> y HTML se preservan.
type mdConverter struct {
	src      []byte
	title    string
	secLevel int
	stack    []*container
	prose    []string
	checks   int
}

func newMDConverter(src []byte, title string) *mdConverter {
	return &mdConverter{src: src, title: title, stack: []*container{{kind: "root"}}}
}

func (c *mdConverter) top() *container { return c.stack[len(c.stack)-1] }

func (c *mdConverter) flushProse() {
	joined := strings.TrimSpace(strings.Join(c.prose, "\n\n"))
	c.prose = nil
	if joined == "" {
		return
	}
	t := c.top()
	t.blocks = append(t.blocks, map[string]any{"t": "prose", "md": joined})
}

func (c *mdConverter) addBlock(b map[string]any) {
	c.flushProse()
	t := c.top()
	t.blocks = append(t.blocks, b)
}

func (c *mdConverter) push(kind, title string, open bool) {
	c.flushProse()
	c.stack = append(c.stack, &container{
		kind: kind,
		blk:  map[string]any{"t": "section", "title": title, "open": open},
	})
}

func (c *mdConverter) pop() {
	if len(c.stack) <= 1 {
		return
	}
	c.flushProse()
	ct := c.top()
	c.stack = c.stack[:len(c.stack)-1]
	blocks := ct.blocks
	if blocks == nil {
		blocks = []any{}
	}
	ct.blk["blocks"] = blocks
	parent := c.top()
	parent.blocks = append(parent.blocks, ct.blk)
}

// popTo cierra contenedores hasta dejar en la cima uno del tipo indicado (o la raíz).
func (c *mdConverter) popWhile(kinds ...string) {
	for len(c.stack) > 1 {
		k := c.top().kind
		match := false
		for _, want := range kinds {
			if k == want {
				match = true
			}
		}
		if !match {
			return
		}
		c.pop()
	}
}

// computeSectionLevel elige el nivel de encabezado que abre secciones: el menor
// nivel presente, excluyendo el H1 usado como título del reporte.
func computeSectionLevel(doc ast.Node, src []byte, title string) int {
	level := 0
	skippedTitle := false
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if !ok {
			continue
		}
		if h.Level == 1 && !skippedTitle {
			txt := strings.TrimSpace(extractInlineMarkdown(h, src))
			if title == "" || strings.EqualFold(txt, strings.TrimSpace(title)) {
				skippedTitle = true
				continue
			}
		}
		if level == 0 || h.Level < level {
			level = h.Level
		}
	}
	return level
}

// convert recorre el documento y devuelve el título efectivo y los bloques crudos.
func (c *mdConverter) convert(doc ast.Node) (string, []any) {
	c.secLevel = computeSectionLevel(doc, c.src, c.title)
	emittedAnything := false

	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		switch v := n.(type) {
		case *ast.Heading:
			txt := strings.TrimSpace(extractInlineMarkdown(v, c.src))
			if v.Level == 1 && c.title == "" {
				c.title = txt
				continue
			}
			if v.Level == 1 && strings.EqualFold(txt, strings.TrimSpace(c.title)) && !emittedAnything {
				continue
			}
			if c.secLevel > 0 && v.Level <= c.secLevel {
				c.popWhile("details", "section")
				c.push("section", txt, true)
			} else {
				depth := v.Level - c.secLevel + 2
				if c.secLevel == 0 {
					depth = 3
				}
				if depth > 6 {
					depth = 6
				}
				c.prose = append(c.prose, strings.Repeat("#", depth)+" "+txt)
			}

		case *ast.Paragraph, *ast.TextBlock:
			if txt := strings.TrimSpace(extractInlineMarkdown(n, c.src)); txt != "" {
				c.prose = append(c.prose, txt)
			}

		case *ast.Blockquote:
			c.convertBlockquote(v)

		case *ast.List:
			if isTaskList(v) {
				c.convertTaskList(v)
			} else {
				c.convertList(v)
			}

		case *east.Table:
			if tbl := convertTableNode(v, c.src); tbl != nil {
				c.addBlock(tbl)
			}

		case *ast.FencedCodeBlock:
			lang := strings.ToLower(strings.TrimSpace(string(v.Language(c.src))))
			c.addBlock(convertCodeFence(lang, extractCodeLines(v, c.src)))

		case *ast.CodeBlock:
			c.addBlock(map[string]any{"t": "code", "lang": "text", "src": extractCodeLines(v, c.src)})

		case *ast.HTMLBlock:
			c.convertHTMLBlock(v)

		case *ast.ThematicBreak:
			c.prose = append(c.prose, "---")

		default:
			// Cualquier nodo no contemplado conserva su texto (nunca se descarta).
			if txt := strings.TrimSpace(extractInlineMarkdown(n, c.src)); txt != "" {
				c.prose = append(c.prose, txt)
			}
		}
		emittedAnything = true
	}
	c.popWhile("details", "section")
	c.flushProse()
	return c.title, c.stack[0].blocks
}

// convertBlockquote: `> [!TIPO] título` y `> **Título**\n> cuerpo` son callouts;
// cualquier otra cita se preserva como cita dentro de la prosa.
func (c *mdConverter) convertBlockquote(bq *ast.Blockquote) {
	first := bq.FirstChild()
	firstTxt := ""
	if first != nil {
		firstTxt = strings.TrimSpace(extractInlineMarkdown(first, c.src))
	}
	isBoldTitle := strings.HasPrefix(firstTxt, "**") && strings.HasSuffix(firstTxt, "**") &&
		strings.Count(firstTxt, "**") == 2 && first.NextSibling() != nil
	if calloutTagRe.MatchString(firstTxt) || isBoldTitle {
		c.addBlock(convertBlockquoteToCallout(bq, c.src))
		return
	}
	body := blockquoteMarkdown(bq, c.src)
	if body != "" {
		c.prose = append(c.prose, body)
	}
}

// convertList serializa listas (ordenadas y anidadas) como Markdown dentro de la
// prosa. Los bloques de código dentro de un ítem se emiten como bloques `code`
// reales (con resaltado) sin perder el orden.
func (c *mdConverter) convertList(list *ast.List) {
	var buf []string
	flush := func() {
		if len(buf) > 0 {
			c.prose = append(c.prose, strings.Join(buf, "\n"))
			buf = nil
		}
	}
	var walk func(l *ast.List, depth int)
	walk = func(l *ast.List, depth int) {
		indent := strings.Repeat("  ", depth)
		idx := l.Start
		if idx == 0 {
			idx = 1
		}
		for li := l.FirstChild(); li != nil; li = li.NextSibling() {
			marker := "- "
			if l.IsOrdered() {
				marker = fmt.Sprintf("%d. ", idx)
				idx++
			}
			wroteLead := false
			for ch := li.FirstChild(); ch != nil; ch = ch.NextSibling() {
				switch cv := ch.(type) {
				case *ast.List:
					walk(cv, depth+1)
				case *ast.FencedCodeBlock:
					flush()
					lang := strings.ToLower(strings.TrimSpace(string(cv.Language(c.src))))
					c.addBlock(convertCodeFence(lang, extractCodeLines(cv, c.src)))
				case *ast.CodeBlock:
					flush()
					c.addBlock(map[string]any{"t": "code", "lang": "text", "src": extractCodeLines(cv, c.src)})
				case *ast.Blockquote:
					if q := blockquoteMarkdown(cv, c.src); q != "" {
						for _, line := range strings.Split(q, "\n") {
							buf = append(buf, indent+"  "+line)
						}
					}
				default:
					txt := strings.TrimSpace(extractInlineMarkdown(ch, c.src))
					if txt == "" {
						continue
					}
					if !wroteLead {
						buf = append(buf, indent+marker+txt)
						wroteLead = true
					} else {
						buf = append(buf, indent+"  "+txt)
					}
				}
			}
			if !wroteLead && li.FirstChild() == nil {
				buf = append(buf, indent+marker)
			}
		}
	}
	walk(list, 0)
	flush()
}

// convertTaskList crea un checklist con las casillas en orden de documento
// (incluidas las anidadas), de modo que chk-N coincida con la N-ésima casilla
// del .md que actualiza ToggleMarkdownCheckbox. Los ítems sin casilla se
// conservan como prosa.
func (c *mdConverter) convertTaskList(list *ast.List) {
	var items []any
	var plain []string
	var walk func(l *ast.List)
	walk = func(l *ast.List) {
		for li := l.FirstChild(); li != nil; li = li.NextSibling() {
			isTask, done := false, false
			if fb := li.FirstChild(); fb != nil && fb.FirstChild() != nil {
				if cb, ok := fb.FirstChild().(*east.TaskCheckBox); ok {
					isTask, done = true, cb.IsChecked
				}
			}
			txt := strings.TrimSpace(extractListItemText(li, c.src))
			if isTask {
				c.checks++
				items = append(items, buildChecklistItem(c.checks, txt, done))
			} else if txt != "" {
				plain = append(plain, "- "+txt)
			}
			for ch := li.FirstChild(); ch != nil; ch = ch.NextSibling() {
				if sub, ok := ch.(*ast.List); ok {
					walk(sub)
				}
			}
		}
	}
	walk(list)
	if len(items) > 0 {
		c.addBlock(map[string]any{"t": "checklist", "items": items})
	}
	if len(plain) > 0 {
		c.prose = append(c.prose, strings.Join(plain, "\n"))
	}
}

// convertHTMLBlock: <details><summary>T</summary> abre una sección colapsada que
// se cierra en </details>; cualquier otro HTML se conserva como código.
func (c *mdConverter) convertHTMLBlock(n *ast.HTMLBlock) {
	var sb strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		sb.Write(seg.Value(c.src))
	}
	if n.HasClosure() {
		sb.Write(n.ClosureLine.Value(c.src))
	}
	raw := strings.TrimSpace(sb.String())
	if raw == "" {
		return
	}

	if detailsOpenRe.MatchString(raw) {
		title := "Detalles"
		rest := detailsOpenRe.ReplaceAllString(raw, "")
		if m := summaryRe.FindStringSubmatch(rest); len(m) == 2 {
			if s := strings.TrimSpace(htmlTagRe.ReplaceAllString(m[1], "")); s != "" {
				title = s
			}
			rest = strings.Replace(rest, m[0], "", 1)
		}
		closes := detailsCloseRe.MatchString(rest)
		rest = strings.TrimSpace(detailsCloseRe.ReplaceAllString(rest, ""))
		c.push("details", title, false)
		if txt := strings.TrimSpace(htmlTagRe.ReplaceAllString(rest, "")); txt != "" {
			c.prose = append(c.prose, txt)
		}
		if closes {
			c.popWhile("details")
		}
		return
	}
	if detailsCloseRe.MatchString(raw) && strings.TrimSpace(detailsCloseRe.ReplaceAllString(raw, "")) == "" {
		c.popWhile("details")
		return
	}
	c.addBlock(map[string]any{"t": "code", "lang": "html", "src": raw})
}

func buildChecklistItem(n int, rawText string, done bool) map[string]any {
	var owner, sev string
	if m := ownerTokenRe.FindStringSubmatch(rawText); len(m) == 2 {
		owner = m[1]
		rawText = strings.TrimSpace(strings.Replace(rawText, m[1], "", 1))
	}
	if m := sevTagRe.FindStringSubmatch(rawText); len(m) == 2 {
		sev = strings.ToLower(m[1])
		rawText = strings.TrimSpace(strings.Replace(rawText, m[0], "", 1))
	}
	if rawText == "" {
		rawText = fmt.Sprintf("Tarea %d", n)
	}
	item := map[string]any{"id": fmt.Sprintf("chk-%d", n), "text": rawText, "done": done}
	if owner != "" {
		item["owner"] = owner
	}
	if sev != "" {
		item["sev"] = sev
	}
	return item
}

// blockquoteMarkdown serializa una cita (con listas y código) a Markdown con "> ".
func blockquoteMarkdown(bq ast.Node, src []byte) string {
	inner := nodesMarkdown(bq, src)
	if inner == "" {
		return ""
	}
	lines := strings.Split(inner, "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n")
}

// nodesMarkdown serializa los hijos de bloque de un nodo a Markdown (párrafos,
// listas anidadas, código, encabezados y citas).
func nodesMarkdown(parent ast.Node, src []byte) string {
	var parts []string
	for ch := parent.FirstChild(); ch != nil; ch = ch.NextSibling() {
		switch v := ch.(type) {
		case *ast.List:
			parts = append(parts, listMarkdown(v, src, 0))
		case *ast.FencedCodeBlock:
			lang := strings.TrimSpace(string(v.Language(src)))
			parts = append(parts, "```"+lang+"\n"+extractCodeLines(v, src)+"\n```")
		case *ast.CodeBlock:
			parts = append(parts, "```\n"+extractCodeLines(v, src)+"\n```")
		case *ast.Heading:
			parts = append(parts, "**"+strings.TrimSpace(extractInlineMarkdown(v, src))+"**")
		case *ast.Blockquote:
			parts = append(parts, blockquoteMarkdown(v, src))
		default:
			if txt := strings.TrimSpace(extractInlineMarkdown(ch, src)); txt != "" {
				parts = append(parts, txt)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func listMarkdown(l *ast.List, src []byte, depth int) string {
	var out []string
	indent := strings.Repeat("  ", depth)
	idx := l.Start
	if idx == 0 {
		idx = 1
	}
	for li := l.FirstChild(); li != nil; li = li.NextSibling() {
		marker := "- "
		if l.IsOrdered() {
			marker = fmt.Sprintf("%d. ", idx)
			idx++
		}
		lead := true
		for ch := li.FirstChild(); ch != nil; ch = ch.NextSibling() {
			if sub, ok := ch.(*ast.List); ok {
				out = append(out, listMarkdown(sub, src, depth+1))
				continue
			}
			txt := strings.TrimSpace(nodesMarkdownSingle(ch, src))
			if txt == "" {
				continue
			}
			if lead {
				out = append(out, indent+marker+txt)
				lead = false
			} else {
				out = append(out, indent+"  "+txt)
			}
		}
	}
	return strings.Join(out, "\n")
}

func nodesMarkdownSingle(n ast.Node, src []byte) string {
	switch v := n.(type) {
	case *ast.FencedCodeBlock:
		return "`" + strings.ReplaceAll(extractCodeLines(v, src), "\n", " ⏎ ") + "`"
	case *ast.CodeBlock:
		return "`" + strings.ReplaceAll(extractCodeLines(v, src), "\n", " ⏎ ") + "`"
	default:
		return extractInlineMarkdown(n, src)
	}
}
