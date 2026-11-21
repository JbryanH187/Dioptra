package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dioptra/internal/layoutagent"
	"dioptra/internal/roots"
	"dioptra/internal/server"
	"dioptra/internal/store"
)

const srcTestHost = "127.0.0.1:7474"

type sourceFixture struct {
	srv     *server.AppServer
	base    string
	reports string
	root    string
}

// newSourceFixture crea <tmp>/reports (store) y <tmp>/proj (raíz de código) con roots.json en <tmp>.
func newSourceFixture(t *testing.T, register bool) *sourceFixture {
	t.Helper()
	base := t.TempDir()
	reports := filepath.Join(base, "reports")
	root := filepath.Join(base, "proj")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(reports)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &sourceFixture{
		srv:     server.New(server.Options{Store: st, BaseURL: "http://" + srcTestHost, LayoutCfg: layoutagent.Config{}}),
		base:    base,
		reports: reports,
		root:    root,
	}
	if register {
		f.register(t, "Mi Proyecto", root)
	}
	return f
}

func (f *sourceFixture) register(t *testing.T, project, path string) {
	t.Helper()
	reg, err := roots.Load(f.reports)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := reg.Set(project, path); err != nil {
		t.Fatal(err)
	}
	if err := reg.Save(); err != nil {
		t.Fatal(err)
	}
}

func (f *sourceFixture) write(t *testing.T, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

type srcReq struct {
	project, path, line, context string
	host, origin                 string
}

func (f *sourceFixture) do(t *testing.T, q srcReq) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	v := url.Values{}
	if q.project != "-" {
		p := q.project
		if p == "" {
			p = "mi-proyecto"
		}
		v.Set("project", p)
	}
	v.Set("path", q.path)
	if q.line != "" {
		v.Set("line", q.line)
	}
	if q.context != "" {
		v.Set("context", q.context)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/source?"+v.Encode(), nil)
	req.Host = srcTestHost
	if q.host != "" {
		req.Host = q.host
	}
	if q.origin != "" {
		req.Header.Set("Origin", q.origin)
	}
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func expectErr(t *testing.T, rec *httptest.ResponseRecorder, body map[string]any, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, esperaba %d; cuerpo=%s", rec.Code, status, rec.Body.String())
	}
	if body["error"] != code {
		t.Fatalf("error = %v, esperaba %q; cuerpo=%s", body["error"], code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("faltan cabeceras no-store/nosniff en error")
	}
}

func numberedLines(n int) []byte {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return []byte(b.String())
}

func TestSource_OK(t *testing.T) {
	f := newSourceFixture(t, true)
	f.write(t, "src/main.go", numberedLines(200))
	rec, body := f.do(t, srcReq{path: "src/main.go", line: "100", context: "5", origin: "http://" + srcTestHost})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("cabeceras de seguridad ausentes")
	}
	if body["project"] != "mi-proyecto" || body["path"] != "src/main.go" || body["language"] != "go" {
		t.Fatalf("campos inesperados: %v", body)
	}
	if body["line"].(float64) != 100 || body["startLine"].(float64) != 95 || body["endLine"].(float64) != 105 || body["totalLines"].(float64) != 200 {
		t.Fatalf("rango inesperado: %v", body)
	}
	lines := body["lines"].([]any)
	if len(lines) != 11 || lines[0] != "line 95" || lines[10] != "line 105" {
		t.Fatalf("líneas inesperadas: %v", lines)
	}
	abs, _ := body["absPath"].(string)
	realRoot, _ := filepath.EvalSymlinks(f.root)
	if !strings.HasPrefix(abs, realRoot) || !filepath.IsAbs(abs) {
		t.Fatalf("absPath %q fuera de raíz %q", abs, realRoot)
	}
}

func TestSource_DefaultAndMaxContext(t *testing.T) {
	f := newSourceFixture(t, true)
	f.write(t, "a.py", numberedLines(500))
	_, body := f.do(t, srcReq{path: "a.py", line: "250"})
	if body["startLine"].(float64) != 210 || body["endLine"].(float64) != 290 || body["language"] != "python" {
		t.Fatalf("context por defecto (40) no aplicado: %v %v", body["startLine"], body["endLine"])
	}
	_, body = f.do(t, srcReq{path: "a.py", line: "250", context: "999"})
	if body["startLine"].(float64) != 130 || body["endLine"].(float64) != 370 {
		t.Fatalf("context máximo (120) no aplicado: %v %v", body["startLine"], body["endLine"])
	}
}

func TestSource_RangeClampedAtStartAndEnd(t *testing.T) {
	f := newSourceFixture(t, true)
	f.write(t, "x.ts", numberedLines(10))
	_, body := f.do(t, srcReq{path: "x.ts", line: "2", context: "5"})
	if body["startLine"].(float64) != 1 || body["endLine"].(float64) != 7 || len(body["lines"].([]any)) != 7 {
		t.Fatalf("recorte al inicio: %v", body)
	}
	_, body = f.do(t, srcReq{path: "x.ts", line: "9", context: "5"})
	if body["startLine"].(float64) != 4 || body["endLine"].(float64) != 10 || body["totalLines"].(float64) != 10 {
		t.Fatalf("recorte al final: %v", body)
	}
	// Línea más allá del final: se recorta a la última.
	_, body = f.do(t, srcReq{path: "x.ts", line: "99", context: "2"})
	if body["line"].(float64) != 10 || body["startLine"].(float64) != 8 || body["endLine"].(float64) != 10 {
		t.Fatalf("línea fuera de rango no recortada: %v", body)
	}
}

func TestSource_Traversal(t *testing.T) {
	f := newSourceFixture(t, true)
	_ = os.WriteFile(filepath.Join(f.base, "secret.txt"), []byte("x"), 0o644)
	for _, p := range []string{"../secret.txt", "src/../../secret.txt", `src\..\..\secret.txt`, ".."} {
		rec, body := f.do(t, srcReq{path: p})
		expectErr(t, rec, body, http.StatusBadRequest, "invalid_path")
	}
}

func TestSource_AbsoluteEmptyNul(t *testing.T) {
	f := newSourceFixture(t, true)
	abs := filepath.Join(f.root, "src", "main.go")
	for _, p := range []string{abs, "/etc/passwd", `C:\Windows\win.ini`, `\\server\share\x`, "", "a\x00b"} {
		rec, body := f.do(t, srcReq{path: p})
		expectErr(t, rec, body, http.StatusBadRequest, "invalid_path")
	}
}

func TestSource_SymlinkOutsideRoot(t *testing.T) {
	f := newSourceFixture(t, true)
	outside := filepath.Join(f.base, "outside.txt")
	_ = os.WriteFile(outside, []byte("fuera"), 0o644)
	if err := os.Symlink(outside, filepath.Join(f.root, "link.txt")); err != nil {
		t.Skipf("el SO no permite crear symlinks: %v", err)
	}
	rec, body := f.do(t, srcReq{path: "link.txt"})
	expectErr(t, rec, body, http.StatusForbidden, "outside_root")
}

func TestSource_BlockedNames(t *testing.T) {
	f := newSourceFixture(t, true)
	for _, p := range []string{".env", ".ENV.local", "certs/server.pem", "k.KEY", "a.pfx", "b.p12", "id_rsa", "id_rsa.pub", "db.kdbx", "Secrets.json", ".git/config", "sub/.Git/HEAD"} {
		f.write(t, p, []byte("TOKEN=1\n"))
		rec, body := f.do(t, srcReq{path: p})
		expectErr(t, rec, body, http.StatusForbidden, "blocked_path")
	}
	// .gitignore no es un segmento .git: permitido.
	f.write(t, ".gitignore", []byte("bin/\n"))
	if rec, _ := f.do(t, srcReq{path: ".gitignore"}); rec.Code != http.StatusOK {
		t.Fatalf(".gitignore debía permitirse: %d", rec.Code)
	}
}

func TestSource_BinaryAndTooLarge(t *testing.T) {
	f := newSourceFixture(t, true)
	f.write(t, "bin.dat", []byte{'a', 0, 'b', '\n'})
	rec, body := f.do(t, srcReq{path: "bin.dat"})
	expectErr(t, rec, body, http.StatusUnsupportedMediaType, "binary_file")

	f.write(t, "big.txt", bytes.Repeat([]byte("a\n"), (2<<20)/2+1))
	rec, body = f.do(t, srcReq{path: "big.txt"})
	expectErr(t, rec, body, http.StatusUnsupportedMediaType, "file_too_large")
}

func TestSource_InvalidHost(t *testing.T) {
	f := newSourceFixture(t, true)
	f.write(t, "a.go", numberedLines(3))
	for _, h := range []string{"evil.example:7474", "127.0.0.1:9999", "localhost", "127.0.0.1"} {
		rec, body := f.do(t, srcReq{path: "a.go", host: h})
		expectErr(t, rec, body, http.StatusForbidden, "invalid_host")
	}
	if rec, _ := f.do(t, srcReq{path: "a.go", host: "localhost:7474", origin: "http://localhost:7474"}); rec.Code != http.StatusOK {
		t.Fatalf("localhost:<port> debía aceptarse: %d", rec.Code)
	}
}

func TestSource_ForeignOrigin(t *testing.T) {
	f := newSourceFixture(t, true)
	f.write(t, "a.go", numberedLines(3))
	for _, o := range []string{"http://evil.example", "http://localhost:7474", "https://127.0.0.1:7474", "null"} {
		rec, body := f.do(t, srcReq{path: "a.go", origin: o})
		expectErr(t, rec, body, http.StatusForbidden, "invalid_origin")
	}
}

func TestSource_NoRootConfiguredThenHotReload(t *testing.T) {
	f := newSourceFixture(t, false)
	f.write(t, "a.go", numberedLines(3))
	rec, body := f.do(t, srcReq{project: "Mi Proyecto", path: "a.go"})
	expectErr(t, rec, body, http.StatusNotFound, "root_not_configured")
	if body["project"] != "mi-proyecto" || body["hint"] != "dioptra roots set mi-proyecto <ruta>" {
		t.Fatalf("hint/proyecto inesperados: %v", body)
	}
	// `dioptra roots set` sin reiniciar el servidor.
	f.register(t, "mi-proyecto", f.root)
	if rec, _ := f.do(t, srcReq{path: "a.go"}); rec.Code != http.StatusOK {
		t.Fatalf("recarga en caliente de roots.json falló: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSource_MissingParamsAndNotFound(t *testing.T) {
	f := newSourceFixture(t, true)
	rec, body := f.do(t, srcReq{project: "-", path: "a.go"})
	expectErr(t, rec, body, http.StatusBadRequest, "missing_project")
	rec, body = f.do(t, srcReq{path: "nope.go"})
	expectErr(t, rec, body, http.StatusNotFound, "file_not_found")
	rec, body = f.do(t, srcReq{path: "src"})
	expectErr(t, rec, body, http.StatusBadRequest, "not_a_file")
	f.write(t, "a.go", numberedLines(3))
	rec, body = f.do(t, srcReq{path: "a.go", line: "abc"})
	expectErr(t, rec, body, http.StatusBadRequest, "invalid_line")
	rec, body = f.do(t, srcReq{path: "a.go", context: "-1"})
	expectErr(t, rec, body, http.StatusBadRequest, "invalid_context")
}
