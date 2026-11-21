package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ProjectAlias une una clave de proyecto equivalente (alias) con la clave canónica que la agrupa.
type ProjectAlias struct {
	AliasKey     string `json:"aliasKey"`
	CanonicalKey string `json:"canonicalKey"`
	CreatedAt    string `json:"createdAt"`
}

// resolveProjectKey devuelve la clave canónica de key (o key si no tiene alias).
// Los alias nunca forman cadenas (SetProjectAlias lo impide), así que basta una consulta.
func (s *Store) resolveProjectKey(ctx context.Context, key string) string {
	if key == "" {
		return key
	}
	var canonical string
	if err := s.db.QueryRowContext(ctx, `SELECT canonical_key FROM project_aliases WHERE alias_key = ?`, key).Scan(&canonical); err == nil && canonical != "" {
		return canonical
	}
	return key
}

// projectGroupKeys devuelve todas las claves equivalentes al proyecto: la canónica más sus alias.
// Se usa para filtrar sin reescribir filas históricas.
func (s *Store) projectGroupKeys(ctx context.Context, project string) []string {
	key := s.resolveProjectKey(ctx, ProjectKey(project))
	if key == "" {
		return nil
	}
	keys := []string{key}
	rows, err := s.db.QueryContext(ctx, `SELECT alias_key FROM project_aliases WHERE canonical_key = ?`, key)
	if err != nil {
		return keys
	}
	defer rows.Close()
	for rows.Next() {
		var a string
		if rows.Scan(&a) == nil && a != "" {
			keys = append(keys, a)
		}
	}
	return keys
}

// SetProjectAlias declara que alias (nombre o clave) es el mismo proyecto que canonical.
// Aditivo y reversible: no reescribe reportes ni mueve archivos .md. Rechaza ciclos y cadenas.
func (s *Store) SetProjectAlias(ctx context.Context, alias, canonical string) (*ProjectAlias, error) {
	a, c := ProjectKey(alias), ProjectKey(canonical)
	switch {
	case a == "" || c == "":
		return nil, errors.New("alias y clave canónica no pueden quedar vacíos tras normalizar")
	case a == c:
		return nil, errors.New("el alias es igual a la clave canónica: no hay nada que unir")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_aliases WHERE alias_key = ?`, c).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("la clave canónica %q ya es alias de otro proyecto; usa la canónica final", c)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_aliases WHERE canonical_key = ?`, a).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("%q ya es canónica de otros alias; retíralos o únelos primero para evitar cadenas", a)
	}

	// Colisión (clave canónica, slug) entre ambos grupos: no se unen automáticamente.
	rows, err := s.db.QueryContext(ctx, `
		SELECT x.slug FROM reports x JOIN reports y ON y.slug = x.slug
		WHERE x.project_key = ? AND y.project_key = ? LIMIT 5`, a, c)
	if err != nil {
		return nil, err
	}
	var clashes []string
	for rows.Next() {
		var sl string
		if rows.Scan(&sl) == nil {
			clashes = append(clashes, sl)
		}
	}
	_ = rows.Close()
	if len(clashes) > 0 {
		return nil, fmt.Errorf("colisión de slug entre %q y %q (%s): resuelve los duplicados antes de unirlos", a, c, strings.Join(clashes, ", "))
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO project_aliases (alias_key, canonical_key, created_at) VALUES (?, ?, ?)
		ON CONFLICT(alias_key) DO UPDATE SET canonical_key = excluded.canonical_key`, a, c, now); err != nil {
		return nil, fmt.Errorf("guardando alias: %w", err)
	}
	return &ProjectAlias{AliasKey: a, CanonicalKey: c, CreatedAt: now}, nil
}

// RemoveProjectAlias retira el alias; revierte la agrupación (las filas nunca se tocaron).
func (s *Store) RemoveProjectAlias(ctx context.Context, alias string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `DELETE FROM project_aliases WHERE alias_key = ?`, ProjectKey(alias))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListProjectAliases devuelve los alias vigentes ordenados por clave canónica.
func (s *Store) ListProjectAliases(ctx context.Context) ([]ProjectAlias, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT alias_key, canonical_key, created_at FROM project_aliases ORDER BY canonical_key, alias_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectAlias{}
	for rows.Next() {
		var p ProjectAlias
		if err := rows.Scan(&p.AliasKey, &p.CanonicalKey, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
