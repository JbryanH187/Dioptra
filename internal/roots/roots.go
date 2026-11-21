// Package roots mantiene el registro explícito proyecto → raíz de código fuente
// (roots.json) que usa el visor de evidencia. El servidor nunca adivina rutas:
// solo sirve archivos bajo raíces registradas aquí con `dioptra roots set`.
package roots

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"dioptra/internal/store"
)

// FileName es el nombre del registro dentro del directorio padre de reportsDir.
const FileName = "roots.json"

// Entry es una raíz registrada.
type Entry struct {
	Project string `json:"project"`
	Path    string `json:"path"`
}

// Registry es el contenido en memoria de roots.json ({"<proyecto-canónico>": "<ruta absoluta>"}).
type Registry struct {
	path  string
	roots map[string]string
}

// FilePath devuelve la ubicación de roots.json: directorio padre de reportsDir
// (con el valor por defecto ~/.dioptra/reports → ~/.dioptra/roots.json).
func FilePath(reportsDir string) string {
	abs, err := filepath.Abs(reportsDir)
	if err != nil {
		abs = filepath.Clean(reportsDir)
	}
	return filepath.Join(filepath.Dir(abs), FileName)
}

// Key canoniza el nombre de proyecto con la misma función que usa el store.
func Key(project string) string {
	return store.ProjectKey(project)
}

// Load lee roots.json; si no existe devuelve un registro vacío. JSON corrupto es error.
func Load(reportsDir string) (*Registry, error) {
	return LoadFile(FilePath(reportsDir))
}

// LoadFile lee un roots.json concreto.
func LoadFile(path string) (*Registry, error) {
	reg := &Registry{path: path, roots: map[string]string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return reg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leyendo %s: %w", path, err)
	}
	raw := map[string]string{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("roots.json inválido (%s): %w", path, err)
	}
	for k, v := range raw {
		key := Key(k)
		if key == "" || v == "" {
			continue
		}
		reg.roots[key] = filepath.Clean(v)
	}
	return reg, nil
}

// Path devuelve la ruta del archivo roots.json asociado.
func (r *Registry) Path() string { return r.path }

// List devuelve las raíces ordenadas por proyecto.
func (r *Registry) List() []Entry {
	out := make([]Entry, 0, len(r.roots))
	for k, v := range r.roots {
		out = append(out, Entry{Project: k, Path: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Project < out[j].Project })
	return out
}

// Resolve devuelve la raíz registrada para el proyecto (canonizado).
func (r *Registry) Resolve(project string) (string, bool) {
	key := Key(project)
	if key == "" {
		return "", false
	}
	p, ok := r.roots[key]
	return p, ok
}

// Set registra (o reemplaza) la raíz de un proyecto. Valida que la ruta exista y sea
// un directorio; devuelve la clave canónica y la ruta absoluta limpia guardada.
func (r *Registry) Set(project, dir string) (string, string, error) {
	key := Key(project)
	if key == "" {
		return "", "", fmt.Errorf("nombre de proyecto inválido: %q", project)
	}
	if dir == "" {
		return "", "", errors.New("ruta vacía")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", fmt.Errorf("ruta inválida %q: %w", dir, err)
	}
	abs = filepath.Clean(abs)
	fi, err := os.Stat(abs)
	if err != nil {
		return "", "", fmt.Errorf("la ruta no existe: %s", abs)
	}
	if !fi.IsDir() {
		return "", "", fmt.Errorf("la ruta no es un directorio: %s", abs)
	}
	r.roots[key] = abs
	return key, abs, nil
}

// Remove retira la raíz del proyecto; devuelve true si existía.
func (r *Registry) Remove(project string) bool {
	key := Key(project)
	if _, ok := r.roots[key]; !ok {
		return false
	}
	delete(r.roots, key)
	return true
}

// Save escribe roots.json de forma atómica (temporal en el mismo directorio + rename).
func (r *Registry) Save() error {
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creando %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(r.roots, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, FileName+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, r.path); err != nil {
		cleanup()
		return fmt.Errorf("guardando %s: %w", r.path, err)
	}
	return nil
}
