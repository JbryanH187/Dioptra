package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// ensureSearchIndex crea la tabla FTS5 `reports_search` (tabla nueva: la antigua reports_fts no se toca ni se borra)
// y la rellena desde los .md/BD solo si está vacía. Si FTS5 no está disponible, hasFTS queda en false
// y la búsqueda cae al LIKE de siempre.
func (s *Store) ensureSearchIndex(ctx context.Context) {
	_, err := s.db.ExecContext(ctx, `CREATE VIRTUAL TABLE IF NOT EXISTS reports_search USING fts5(
		id UNINDEXED, title, project, tool, model, body,
		tokenize = 'unicode61 remove_diacritics 2'
	)`)
	if err != nil {
		s.hasFTS = false
		return
	}
	s.hasFTS = true

	var n int
	if s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reports_search`).Scan(&n) != nil || n > 0 {
		return
	}
	// Backfill inicial con el CUERPO completo de cada .md (antes solo el extracto, y
	// los reportes viejos no se encontraban por su contenido hasta republicarlos).
	if _, err := s.RebuildSearchIndex(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "dioptra: no se pudo poblar el índice de búsqueda: %v\n", err)
	}
}

// searchIDs devuelve los ids que coinciden con el texto, ordenados por relevancia BM25
// (pesos: título 10, proyecto 2, herramienta 1, modelo 1, cuerpo 1). Si FTS no está disponible
// o el texto no genera términos, devuelve ok=false y el llamador usa LIKE.
func (s *Store) searchIDs(ctx context.Context, q string, limit int) (ids []string, ok bool) {
	expr := ftsMatchExpr(q)
	if !s.hasFTS || expr == "" {
		return nil, false
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM reports_search WHERE reports_search MATCH ?
		ORDER BY bm25(reports_search, 0.0, 10.0, 2.0, 1.0, 1.0, 1.0) LIMIT ?`, expr, limit)
	if err != nil {
		return nil, false
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err() == nil
}

// ftsMatchExpr convierte texto libre en una consulta FTS5 segura: cada término entre comillas
// (sin operadores ni sintaxis del usuario) y el último con prefijo, unidos por AND implícito.
func ftsMatchExpr(q string) string {
	var terms []string
	for _, f := range strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if f != "" {
			terms = append(terms, `"`+strings.ReplaceAll(f, `"`, ``)+`"`)
		}
	}
	if len(terms) == 0 {
		return ""
	}
	terms[len(terms)-1] += "*"
	return strings.Join(terms, " ")
}
