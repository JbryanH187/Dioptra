package store

import (
	"context"
	"strings"
	"unicode"
)

// accentFold reemplaza letras acentuadas comunes (es/pt/fr) por su base ASCII sin depender de x/text.
var accentFold = strings.NewReplacer(
	"á", "a", "à", "a", "ä", "a", "â", "a", "ã", "a",
	"é", "e", "è", "e", "ë", "e", "ê", "e",
	"í", "i", "ì", "i", "ï", "i", "î", "i",
	"ó", "o", "ò", "o", "ö", "o", "ô", "o", "õ", "o",
	"ú", "u", "ù", "u", "ü", "u", "û", "u",
	"ñ", "n", "ç", "c",
)

// ProjectKey devuelve la clave canónica anti-duplicados de un proyecto:
// minúsculas, sin acentos y con cualquier separador (espacio, _, ., /) colapsado a "-".
// "DioptraEngine", "dioptra engine" y "Dioptra_Engine" NO colapsan entre sí por diseño
// (no se adivina dónde cortar palabras); sí colapsan "Dioptra Engine", "dioptra-engine" y "DIOPTRA_ENGINE".
func ProjectKey(project string) string {
	s := accentFold.Replace(strings.ToLower(strings.TrimSpace(project)))
	var b strings.Builder
	lastDash := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// canonicalProjectLocked devuelve el nombre de display ya registrado para la misma clave canónica
// (el primero publicado gana) o el nombre recibido si el proyecto es nuevo.
func (s *Store) canonicalProjectLocked(ctx context.Context, project string) (display, key string) {
	key = s.resolveProjectKey(ctx, ProjectKey(project))
	if key == "" {
		return project, key
	}
	group := s.projectGroupKeys(ctx, key)
	args := make([]any, len(group))
	marks := make([]string, len(group))
	for i, k := range group {
		args[i], marks[i] = k, "?"
	}
	var existing string
	err := s.db.QueryRowContext(ctx,
		`SELECT project FROM reports WHERE project_key IN (`+strings.Join(marks, ",")+`) ORDER BY ts ASC LIMIT 1`, args...).Scan(&existing)
	if err == nil && strings.TrimSpace(existing) != "" {
		return existing, key
	}
	return project, key
}
