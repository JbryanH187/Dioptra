package render

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Corpus de reportes reales de agentes (testdata/*.md). Mide que la ruta rápida
// goldmark NO pierda contenido en silencio: toda palabra significativa del .md
// debe llegar a los bloques renderizados.

var corpusWordRe = regexp.MustCompile(`\p{L}[\p{L}\p{N}_]{3,}`)

// Palabras de sintaxis (Mermaid, HTML, fences) que no son contenido del autor.
var corpusSyntaxWords = map[string]bool{
	"flowchart": true, "graph": true, "subgraph": true, "participant": true,
	"sequencediagram": true, "details": true, "summary": true, "mermaid": true,
	// etiquetas de callout GitHub ([!NOTE], [!TIP]...)
	"note": true, "tip": true, "important": true, "caution": true, "danger": true,
}

func corpusFiles(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("corpus vacío: %v", err)
	}
	sort.Strings(files)
	return files
}

func sourceWords(md string) map[string]bool {
	words := map[string]bool{}
	for _, w := range corpusWordRe.FindAllString(md, -1) {
		lw := strings.ToLower(w)
		if !corpusSyntaxWords[lw] {
			words[lw] = true
		}
	}
	return words
}

// renderedText concatena todo el texto visible de los bloques (sin SVG ni tokens
// de resaltado, que duplican el código y ocultarían pérdidas reales).
func renderedText(blocks []map[string]any) string {
	var sb strings.Builder
	var walk func(v any, key string)
	walk = func(v any, key string) {
		switch x := v.(type) {
		case string:
			if key != "svg" {
				sb.WriteString(x)
				sb.WriteByte(' ')
			}
		case []any:
			for _, it := range x {
				walk(it, key)
			}
		case []map[string]any:
			for _, it := range x {
				walk(it, key)
			}
		case [][]string:
			for _, it := range x {
				for _, s := range it {
					sb.WriteString(s)
					sb.WriteByte(' ')
				}
			}
		case []string:
			for _, s := range x {
				sb.WriteString(s)
				sb.WriteByte(' ')
			}
		case map[string]any:
			for k, it := range x {
				if k == "tokens" || k == "hunks" {
					continue
				}
				walk(it, k)
			}
		}
	}
	for _, b := range blocks {
		walk(b, "")
	}
	return strings.ToLower(sb.String())
}

func TestCorpus_NoSilentContentLoss(t *testing.T) {
	ctx := context.Background()
	for _, f := range corpusFiles(t) {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			env, _, err := ConvertMarkdownToEnvelope(ctx, "", "corpus", "2026-11-20T00:00:00Z", string(raw))
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			out := renderedText(env.Report.Blocks) + " " + strings.ToLower(env.Report.Title)
			outWords := sourceWords(out)
			var missing []string
			for w := range sourceWords(string(raw)) {
				if !outWords[w] {
					missing = append(missing, w)
				}
			}
			sort.Strings(missing)
			if len(missing) > 0 {
				t.Errorf("contenido perdido (%d palabras): %s", len(missing), strings.Join(missing, ", "))
			}
		})
	}
}

// Los ejemplos de la skill dioptra-report-writer son copias del corpus: si alguien
// edita uno sin el otro, la skill dejaría de mostrar algo probado.
func TestSkillExamplesMatchCorpus(t *testing.T) {
	pairs := map[string]string{
		"auditoria.md":    "01-auditoria.md",
		"arquitectura.md": "02-arquitectura-mermaid.md",
		"postmortem.md":   "06-postmortem.md",
	}
	dir := filepath.Join("..", "..", "integrations", "skills", "dioptra-report-writer", "examples")
	for ex, corpus := range pairs {
		a, errA := os.ReadFile(filepath.Join(dir, ex))
		b, errB := os.ReadFile(filepath.Join("testdata", corpus))
		if errA != nil || errB != nil {
			t.Fatalf("leyendo %s / %s: %v %v", ex, corpus, errA, errB)
		}
		if string(a) != string(b) {
			t.Errorf("examples/%s difiere de testdata/%s", ex, corpus)
		}
	}
}

func BenchmarkConvertCorpus(b *testing.B) {
	ctx := context.Background()
	var docs []string
	for _, f := range corpusFiles(b) {
		raw, err := os.ReadFile(f)
		if err != nil {
			b.Fatal(err)
		}
		docs = append(docs, string(raw))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, d := range docs {
			if _, _, err := ConvertMarkdownToEnvelope(ctx, "", "bench", "2026-11-20T00:00:00Z", d); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkConvertCorpusFile mide cada reporte por separado para ubicar el costo.
func BenchmarkConvertCorpusFile(b *testing.B) {
	ctx := context.Background()
	for _, f := range corpusFiles(b) {
		raw, err := os.ReadFile(f)
		if err != nil {
			b.Fatal(err)
		}
		doc := string(raw)
		b.Run(filepath.Base(f), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, _, err := ConvertMarkdownToEnvelope(ctx, "", "bench", "2026-11-20T00:00:00Z", doc); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

var updateGolden = flag.Bool("update", false, "reescribe testdata/*.golden.json con la salida actual")

// stripVolatile quita del árbol los campos derivados y voluminosos (SVG de D2,
// tokens de Chroma, hunks) para que el golden capture solo la estructura.
func stripVolatile(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, it := range x {
			if k == "svg" || k == "tokens" || k == "hunks" {
				continue
			}
			out[k] = stripVolatile(it)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, it := range x {
			out[i] = stripVolatile(it)
		}
		return out
	}
	return v
}

// TestCorpus_Golden congela la estructura de bloques del corpus. Tras un cambio
// intencional: go test ./internal/render -run Golden -update
func TestCorpus_Golden(t *testing.T) {
	ctx := context.Background()
	for _, f := range corpusFiles(t) {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			env, warnings, err := ConvertMarkdownToEnvelope(ctx, "", "corpus", "2026-11-20T00:00:00Z", string(raw))
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			if len(warnings) != 0 {
				t.Errorf("Markdown limpio no debe producir warnings: %+v", warnings)
			}
			// Normalizar vía JSON para comparar tipos homogéneos.
			b, _ := json.Marshal(map[string]any{"title": env.Report.Title, "blocks": env.Report.Blocks})
			var generic any
			_ = json.Unmarshal(b, &generic)
			got, _ := json.MarshalIndent(stripVolatile(generic), "", "  ")
			got = append(got, '\n')

			golden := strings.TrimSuffix(f, ".md") + ".golden.json"
			if *updateGolden {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("falta %s (genera con -update): %v", golden, err)
			}
			if string(want) != strings.ReplaceAll(string(got), "\r\n", "\n") && strings.ReplaceAll(string(want), "\r\n", "\n") != string(got) {
				t.Errorf("la salida cambió respecto a %s; revisa el diff y regenera con -update si es intencional", filepath.Base(golden))
			}
		})
	}
}
