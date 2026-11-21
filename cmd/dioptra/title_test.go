package main

import "testing"

func TestTitleFromMarkdown(t *testing.T) {
	cases := []struct{ md, fm, file, want string }{
		{"# Hola mundo\n\ntexto", "", "x/archivo.md", "Hola mundo"},
		{"# Hola", "Del frontmatter", "a.md", "Del frontmatter"},
		{"```\n# no es titulo\n```\n## H2\n", "", "dir/REPORTE_X.md", "REPORTE_X"},
		{"intro\n\n# Título tardío ##\n", "", "a.md", "Título tardío"},
	}
	for _, c := range cases {
		if got := titleFromMarkdown(c.md, c.fm, c.file); got != c.want {
			t.Errorf("titleFromMarkdown(%q) = %q, quiero %q", c.md, got, c.want)
		}
	}
}
