package roots_test

import (
	"os"
	"path/filepath"
	"testing"

	"dioptra/internal/roots"
)

func TestRoots_FileLivesInParentOfReportsDir(t *testing.T) {
	base := t.TempDir()
	reports := filepath.Join(base, "reports")
	got := roots.FilePath(reports)
	want := filepath.Join(base, "roots.json")
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("FilePath = %q, esperaba %q", got, want)
	}
}

func TestRoots_LoadMissingIsEmpty(t *testing.T) {
	reg, err := roots.Load(filepath.Join(t.TempDir(), "reports"))
	if err != nil {
		t.Fatalf("Load sin archivo no debe fallar: %v", err)
	}
	if len(reg.List()) != 0 {
		t.Fatalf("esperaba registro vacío")
	}
	if _, ok := reg.Resolve("x"); ok {
		t.Fatalf("Resolve en vacío debe dar false")
	}
}

func TestRoots_SetSaveLoadResolveRemove(t *testing.T) {
	base := t.TempDir()
	reports := filepath.Join(base, "reports")
	src := filepath.Join(base, "src", "proj")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	reg, err := roots.Load(reports)
	if err != nil {
		t.Fatal(err)
	}
	key, abs, err := reg.Set("Dioptra Engine", src+string(filepath.Separator)+".")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if key != "dioptra-engine" {
		t.Fatalf("clave canónica = %q", key)
	}
	if abs != filepath.Clean(src) {
		t.Fatalf("ruta guardada no limpia: %q", abs)
	}
	if err := reg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(roots.FilePath(reports)); err != nil {
		t.Fatalf("roots.json no creado: %v", err)
	}
	// Sin residuos .tmp tras escritura atómica.
	matches, _ := filepath.Glob(filepath.Join(base, "roots.json.*"))
	if len(matches) != 0 {
		t.Fatalf("quedaron temporales: %v", matches)
	}

	reg2, err := roots.Load(reports)
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"DIOPTRA_ENGINE", "dioptra-engine", "Dióptra Engine"} {
		got, ok := reg2.Resolve(alias)
		if !ok || got != filepath.Clean(src) {
			t.Fatalf("Resolve(%q) = %q,%v", alias, got, ok)
		}
	}
	if !reg2.Remove("dioptra engine") {
		t.Fatalf("Remove debía devolver true")
	}
	if reg2.Remove("dioptra engine") {
		t.Fatalf("segundo Remove debía devolver false")
	}
	if err := reg2.Save(); err != nil {
		t.Fatal(err)
	}
	reg3, _ := roots.Load(reports)
	if _, ok := reg3.Resolve("dioptra-engine"); ok {
		t.Fatalf("la raíz debía haberse retirado")
	}
}

func TestRoots_SetValidatesDirectory(t *testing.T) {
	base := t.TempDir()
	reg, _ := roots.Load(filepath.Join(base, "reports"))
	if _, _, err := reg.Set("p", filepath.Join(base, "no-existe")); err == nil {
		t.Fatalf("esperaba error para ruta inexistente")
	}
	file := filepath.Join(base, "f.txt")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	if _, _, err := reg.Set("p", file); err == nil {
		t.Fatalf("esperaba error para archivo (no directorio)")
	}
	if _, _, err := reg.Set("  !!  ", base); err == nil {
		t.Fatalf("esperaba error para proyecto vacío tras canonizar")
	}
}

func TestRoots_LoadCorruptFails(t *testing.T) {
	base := t.TempDir()
	reports := filepath.Join(base, "reports")
	_ = os.WriteFile(roots.FilePath(reports), []byte("{no json"), 0o644)
	if _, err := roots.Load(reports); err == nil {
		t.Fatalf("esperaba error con JSON corrupto")
	}
}
