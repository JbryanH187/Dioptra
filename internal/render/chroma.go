package render

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"dioptra/internal/contract"
)

// TokenizeCode colorea el código en el servidor con Chroma v2 devolviendo líneas
// de spans compatibles con las clases CSS .syn-* de <dx-code-view> (§2 ADR 1 y §4.5).
func TokenizeCode(lang, src string) [][]contract.CodeSpan {
	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	rawLines := strings.Split(normalized, "\n")

	var lexer chroma.Lexer
	if strings.TrimSpace(lang) != "" {
		lexer = lexers.Get(lang)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	iter, err := lexer.Tokenise(nil, normalized)
	if err != nil {
		out := make([][]contract.CodeSpan, len(rawLines))
		for i, line := range rawLines {
			out[i] = []contract.CodeSpan{{Text: line}}
		}
		return out
	}

	lines := [][]contract.CodeSpan{{}}
	for _, tok := range iter.Tokens() {
		cls := mapChromaTokenClass(tok.Type)
		val := strings.ReplaceAll(tok.Value, "\r\n", "\n")
		parts := strings.Split(val, "\n")
		for idx, part := range parts {
			if idx > 0 {
				lines = append(lines, []contract.CodeSpan{})
			}
			if part == "" {
				continue
			}
			curLine := &lines[len(lines)-1]
			if len(*curLine) > 0 && (*curLine)[len(*curLine)-1].Class == cls {
				(*curLine)[len(*curLine)-1].Text += part
			} else {
				*curLine = append(*curLine, contract.CodeSpan{
					Text:  part,
					Class: cls,
				})
			}
		}
	}

	// Si el texto original no terminaba en \n, ajustamos longitud exacta
	if len(lines) > len(rawLines) && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(rawLines)]
	}
	for len(lines) < len(rawLines) {
		lines = append(lines, []contract.CodeSpan{})
	}
	return lines
}

func mapChromaTokenClass(tt chroma.TokenType) string {
	switch {
	case tt == chroma.KeywordType || tt.InCategory(chroma.Name) && (tt == chroma.NameClass || tt == chroma.NameNamespace):
		return "syn-type"
	case tt.InCategory(chroma.Keyword):
		return "syn-keyword"
	case tt.InCategory(chroma.LiteralString):
		return "syn-string"
	case tt.InCategory(chroma.LiteralNumber):
		return "syn-number"
	case tt.InSubCategory(chroma.NameFunction) || tt == chroma.NameBuiltin || tt.InSubCategory(chroma.NameBuiltin):
		return "syn-function"
	case tt.InCategory(chroma.Comment):
		return "syn-comment"
	default:
		return ""
	}
}

// ComputeUnifiedDiff calcula en servidor el diff unificado (LCS) y colorea cada línea con Chroma.
func ComputeUnifiedDiff(lang, oldSrc, newSrc string) []contract.DiffHunkLine {
	a := strings.Split(strings.ReplaceAll(oldSrc, "\r\n", "\n"), "\n")
	b := strings.Split(strings.ReplaceAll(newSrc, "\r\n", "\n"), "\n")
	oldTokens := TokenizeCode(lang, oldSrc)
	newTokens := TokenizeCode(lang, newSrc)

	m, n := len(a), len(b)
	if m*n > 250000 {
		out := make([]contract.DiffHunkLine, 0, m+n)
		for i, line := range a {
			out = append(out, contract.DiffHunkLine{
				Kind:   "del",
				LineNo: i + 1,
				Text:   line,
				Tokens: safeLineTokens(oldTokens, i),
			})
		}
		for j, line := range b {
			out = append(out, contract.DiffHunkLine{
				Kind:   "add",
				LineNo: j + 1,
				Text:   line,
				Tokens: safeLineTokens(newTokens, j),
			})
		}
		return out
	}

	dp := make([][]uint16, m+1)
	for i := range dp {
		dp[i] = make([]uint16, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var rows []contract.DiffHunkLine
	i, j := 0, 0
	for i < m && j < n {
		if a[i] == b[j] {
			rows = append(rows, contract.DiffHunkLine{
				Kind:   "ctx",
				LineNo: j + 1,
				Text:   b[j],
				Tokens: safeLineTokens(newTokens, j),
			})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			rows = append(rows, contract.DiffHunkLine{
				Kind:   "del",
				LineNo: i + 1,
				Text:   a[i],
				Tokens: safeLineTokens(oldTokens, i),
			})
			i++
		} else {
			rows = append(rows, contract.DiffHunkLine{
				Kind:   "add",
				LineNo: j + 1,
				Text:   b[j],
				Tokens: safeLineTokens(newTokens, j),
			})
			j++
		}
	}
	for i < m {
		rows = append(rows, contract.DiffHunkLine{
			Kind:   "del",
			LineNo: i + 1,
			Text:   a[i],
			Tokens: safeLineTokens(oldTokens, i),
		})
		i++
	}
	for j < n {
		rows = append(rows, contract.DiffHunkLine{
			Kind:   "add",
			LineNo: j + 1,
			Text:   b[j],
			Tokens: safeLineTokens(newTokens, j),
		})
		j++
	}
	return rows
}

var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// ParseUnifiedDiff convierte un diff unificado (git diff / patch) en filas
// coloreables: "hdr" (cabeceras de archivo), "hunk" (@@), "add", "del" y "ctx".
// El lenguaje para el resaltado se deduce de la ruta en "+++ b/archivo.ext";
// los números de línea salen de las cabeceras @@ (nuevo para add/ctx, viejo para del).
func ParseUnifiedDiff(src string) []contract.DiffHunkLine {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	rows := make([]contract.DiffHunkLine, 0, len(lines))
	var lexer chroma.Lexer
	oldLn, newLn := 0, 0
	tokenize := func(text string) []contract.CodeSpan {
		if lexer == nil || text == "" {
			return nil
		}
		it, err := lexer.Tokenise(nil, text)
		if err != nil {
			return nil
		}
		var spans []contract.CodeSpan
		for _, tok := range it.Tokens() {
			if v := strings.TrimRight(tok.Value, "\n"); v != "" {
				spans = append(spans, contract.CodeSpan{Text: v, Class: mapChromaTokenClass(tok.Type)})
			}
		}
		return spans
	}
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, "--- "),
			strings.HasPrefix(line, "diff --git"), strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "new file mode"), strings.HasPrefix(line, "deleted file mode"):
			if strings.HasPrefix(line, "+++ ") {
				path := strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")), "b/")
				if l := lexers.Match(path); l != nil {
					lexer = chroma.Coalesce(l)
				}
			}
			rows = append(rows, contract.DiffHunkLine{Kind: "hdr", Text: line})
		case strings.HasPrefix(line, "@@"):
			if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
				oldLn, _ = strconv.Atoi(m[1])
				newLn, _ = strconv.Atoi(m[2])
			}
			rows = append(rows, contract.DiffHunkLine{Kind: "hunk", Text: line})
		case strings.HasPrefix(line, "+"):
			rows = append(rows, contract.DiffHunkLine{Kind: "add", LineNo: newLn, Text: line[1:], Tokens: tokenize(line[1:])})
			if newLn > 0 {
				newLn++
			}
		case strings.HasPrefix(line, "-"):
			rows = append(rows, contract.DiffHunkLine{Kind: "del", LineNo: oldLn, Text: line[1:], Tokens: tokenize(line[1:])})
			if oldLn > 0 {
				oldLn++
			}
		default:
			text := strings.TrimPrefix(line, " ")
			rows = append(rows, contract.DiffHunkLine{Kind: "ctx", LineNo: newLn, Text: text, Tokens: tokenize(text)})
			if newLn > 0 {
				newLn++
				oldLn++
			}
		}
	}
	for len(rows) > 0 && rows[len(rows)-1].Kind == "ctx" && rows[len(rows)-1].Text == "" {
		rows = rows[:len(rows)-1]
	}
	return rows
}

func safeLineTokens(lines [][]contract.CodeSpan, idx int) []contract.CodeSpan {
	if idx >= 0 && idx < len(lines) {
		return lines[idx]
	}
	return nil
}

// EnrichBlocks recorre todos los bloques validados y adjunta tokens Chroma,
// hunks de diff y SVGs precompilados con D2 para diagramas `flow`.
func EnrichBlocks(ctx context.Context, blocks []map[string]any) {
	for _, b := range blocks {
		tStr, _ := b["t"].(string)
		switch tStr {
		case "code":
			lang, _ := b["lang"].(string)
			src, _ := b["src"].(string)
			if oldSrc, hasOld := b["old"].(string); hasOld {
				if b["hunks"] == nil {
					b["hunks"] = ComputeUnifiedDiff(lang, oldSrc, src)
				}
			} else if (lang == "diff" || lang == "patch") && b["hunks"] == nil {
				b["hunks"] = ParseUnifiedDiff(src)
			} else if b["tokens"] == nil {
				b["tokens"] = TokenizeCode(lang, src)
			}

		case "diagram":
			kind, _ := b["kind"].(string)
			if kind == "flow" {
				if existingSVG, _ := b["svg"].(string); strings.TrimSpace(existingSVG) != "" {
					if clean, err := SanitizeSVG(existingSVG); err == nil {
						b["svg"] = clean
					}
				} else {
					if compiledSVG, err := CompileFlowDiagramSVG(ctx, b); err == nil && compiledSVG != "" {
						b["svg"] = compiledSVG
					}
				}
			}

		case "section":
			if sub, ok := b["blocks"].([]map[string]any); ok {
				EnrichBlocks(ctx, sub)
			} else if subAny, ok := b["blocks"].([]any); ok {
				var conv []map[string]any
				for _, x := range subAny {
					if m, ok := x.(map[string]any); ok {
						conv = append(conv, m)
					}
				}
				EnrichBlocks(ctx, conv)
			}
		}
	}
}
