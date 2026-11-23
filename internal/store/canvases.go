package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DesignCanvas representa un lienzo multipantalla local registrado opt-in para un proyecto
// (p. ej. Tsugi en C:\Dev\tsugi\design\canvas con project/canvas.json y pantallas .dc.html).
type DesignCanvas struct {
	ProjectKey      string `json:"projectKey"`
	CanvasID        string `json:"canvasId"`
	Title           string `json:"title"`
	DirPath         string `json:"dirPath"`
	EntryJSON       string `json:"entryJson"`
	UpdatedAt       string `json:"updatedAt"`
	OpenAnnotations int    `json:"openAnnotations"`
}

// CanvasAnnotation representa un comentario visual (Pin, Caja o Marcador) dejado sobre
// una pantalla del lienzo con coordenadas normalizadas 0..1 y contexto DOM opcional.
type CanvasAnnotation struct {
	ID             string          `json:"id"`
	ProjectKey     string          `json:"projectKey"`
	CanvasID       string          `json:"canvasId"`
	PageID         string          `json:"pageId,omitempty"`
	BoardFile      string          `json:"boardFile"`
	Kind           string          `json:"kind"` // pin | box | highlight
	Geometry       json.RawMessage `json:"geometry"`
	TargetSelector string          `json:"targetSelector,omitempty"`
	TargetText     string          `json:"targetText,omitempty"`
	Comment        string          `json:"comment"`
	Author         string          `json:"author"`
	Status         string          `json:"status"` // open | applied | resolved
	ResolutionNote string          `json:"resolutionNote,omitempty"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
}

var (
	validAnnotationKinds    = map[string]bool{"pin": true, "box": true, "highlight": true}
	validAnnotationStatuses = map[string]bool{"open": true, "applied": true, "resolved": true}
)

func sanitizeCanvasID(raw string) string {
	return ProjectKey(raw)
}

func randomHexID(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

// UpsertDesignCanvas registra o actualiza un lienzo local opt-in validando que dir_path
// exista y contenga entry_json dentro del propio directorio.
func (s *Store) UpsertDesignCanvas(ctx context.Context, project, canvasID, title, dirPath, entryJSON string) (*DesignCanvas, error) {
	pKey := s.resolveProjectKey(ctx, ProjectKey(project))
	if pKey == "" {
		return nil, errors.New("falta project válido")
	}
	cID := sanitizeCanvasID(canvasID)
	if cID == "" {
		cID = "default"
	}
	t := strings.TrimSpace(title)
	if t == "" {
		t = cID
	}
	rawDir := strings.TrimSpace(dirPath)
	if rawDir == "" {
		return nil, errors.New("falta dir_path del lienzo")
	}
	absDir, err := filepath.Abs(rawDir)
	if err != nil {
		return nil, fmt.Errorf("dir_path inválido: %w", err)
	}
	absDir = filepath.Clean(absDir)
	fi, err := os.Stat(absDir)
	if err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("dir_path no existe o no es un directorio: %s", absDir)
	}

	entry := strings.TrimSpace(strings.ReplaceAll(entryJSON, `\`, "/"))
	if entry == "" {
		// Detecta automáticamente project/canvas.json o canvas.json en la raíz.
		if _, err := os.Stat(filepath.Join(absDir, "project", "canvas.json")); err == nil {
			entry = "project/canvas.json"
		} else {
			entry = "canvas.json"
		}
	}
	if strings.HasPrefix(entry, "/") || strings.Contains(entry, "..") || strings.Contains(entry, ":") {
		return nil, fmt.Errorf("entry_json inválido: %q", entryJSON)
	}
	realDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		return nil, fmt.Errorf("resolviendo dir_path: %w", err)
	}
	entryAbs := filepath.Join(realDir, filepath.FromSlash(entry))
	realEntry, err := filepath.EvalSymlinks(entryAbs)
	if err != nil {
		return nil, fmt.Errorf("no se encontró manifiesto %s dentro de %s", entry, absDir)
	}
	relCheck, err := filepath.Rel(realDir, realEntry)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) || filepath.IsAbs(relCheck) {
		return nil, errors.New("entry_json escapa del directorio del lienzo")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO design_canvases (project_key, canvas_id, title, dir_path, entry_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_key, canvas_id) DO UPDATE SET
			title = excluded.title,
			dir_path = excluded.dir_path,
			entry_json = excluded.entry_json,
			updated_at = excluded.updated_at`,
		pKey, cID, t, absDir, entry, now); err != nil {
		return nil, fmt.Errorf("guardando lienzo: %w", err)
	}
	return &DesignCanvas{
		ProjectKey: pKey,
		CanvasID:   cID,
		Title:      t,
		DirPath:    absDir,
		EntryJSON:  entry,
		UpdatedAt:  now,
	}, nil
}

// GetDesignCanvas obtiene un lienzo por proyecto y canvas_id. Si el proyecto es un sub-equipo
// sin lienzo propio, busca en su workspace padre para permitir compartir el lienzo.
func (s *Store) GetDesignCanvas(ctx context.Context, project, canvasID string) (*DesignCanvas, error) {
	pKey := s.resolveProjectKey(ctx, ProjectKey(project))
	cID := sanitizeCanvasID(canvasID)
	if pKey == "" || cID == "" {
		return nil, sql.ErrNoRows
	}
	keys := []string{pKey}
	if parent := s.ParentWorkspaceOf(ctx, pKey); parent != "" && parent != pKey {
		keys = append(keys, parent)
	}
	for _, k := range keys {
		var c DesignCanvas
		err := s.db.QueryRowContext(ctx, `
			SELECT d.project_key, d.canvas_id, d.title, d.dir_path, d.entry_json, d.updated_at,
			       (SELECT COUNT(*) FROM canvas_annotations a
			         WHERE a.project_key = d.project_key AND a.canvas_id = d.canvas_id AND a.status = 'open')
			FROM design_canvases d
			WHERE d.project_key = ? AND d.canvas_id = ?`, k, cID).
			Scan(&c.ProjectKey, &c.CanvasID, &c.Title, &c.DirPath, &c.EntryJSON, &c.UpdatedAt, &c.OpenAnnotations)
		if err == nil {
			return &c, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return nil, sql.ErrNoRows
}

// ListDesignCanvases lista los lienzos registrados (todos si project == "", o los del proyecto + su padre).
func (s *Store) ListDesignCanvases(ctx context.Context, project string) ([]DesignCanvas, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if strings.TrimSpace(project) == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT d.project_key, d.canvas_id, d.title, d.dir_path, d.entry_json, d.updated_at,
			       (SELECT COUNT(*) FROM canvas_annotations a
			         WHERE a.project_key = d.project_key AND a.canvas_id = d.canvas_id AND a.status = 'open')
			FROM design_canvases d
			ORDER BY d.updated_at DESC, d.project_key, d.canvas_id`)
	} else {
		pKey := s.resolveProjectKey(ctx, ProjectKey(project))
		parent := s.ParentWorkspaceOf(ctx, pKey)
		if parent != "" && parent != pKey {
			rows, err = s.db.QueryContext(ctx, `
				SELECT d.project_key, d.canvas_id, d.title, d.dir_path, d.entry_json, d.updated_at,
				       (SELECT COUNT(*) FROM canvas_annotations a
				         WHERE a.project_key = d.project_key AND a.canvas_id = d.canvas_id AND a.status = 'open')
				FROM design_canvases d
				WHERE d.project_key IN (?, ?)
				ORDER BY d.updated_at DESC, d.canvas_id`, pKey, parent)
		} else {
			rows, err = s.db.QueryContext(ctx, `
				SELECT d.project_key, d.canvas_id, d.title, d.dir_path, d.entry_json, d.updated_at,
				       (SELECT COUNT(*) FROM canvas_annotations a
				         WHERE a.project_key = d.project_key AND a.canvas_id = d.canvas_id AND a.status = 'open')
				FROM design_canvases d
				WHERE d.project_key = ?
				ORDER BY d.updated_at DESC, d.canvas_id`, pKey)
		}
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DesignCanvas{}
	for rows.Next() {
		var c DesignCanvas
		if err := rows.Scan(&c.ProjectKey, &c.CanvasID, &c.Title, &c.DirPath, &c.EntryJSON, &c.UpdatedAt, &c.OpenAnnotations); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateCanvasAnnotation guarda una nueva anotación visual sobre un board del lienzo.
func (s *Store) CreateCanvasAnnotation(ctx context.Context, ann CanvasAnnotation) (*CanvasAnnotation, error) {
	c, err := s.GetDesignCanvas(ctx, ann.ProjectKey, ann.CanvasID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("lienzo no encontrado (%s/%s)", ann.ProjectKey, ann.CanvasID)
		}
		return nil, err
	}
	kind := strings.ToLower(strings.TrimSpace(ann.Kind))
	if !validAnnotationKinds[kind] {
		return nil, fmt.Errorf("tipo de anotación inválido %q (usa pin, box o highlight)", ann.Kind)
	}
	boardFile := strings.TrimSpace(ann.BoardFile)
	if boardFile == "" || strings.Contains(boardFile, "..") {
		return nil, errors.New("falta boardFile válido")
	}
	comment := strings.TrimSpace(ann.Comment)
	if comment == "" {
		return nil, errors.New("el comentario no puede estar vacío")
	}
	if len([]rune(comment)) > 2000 {
		return nil, errors.New("el comentario excede 2000 caracteres")
	}
	geomStr := strings.TrimSpace(string(ann.Geometry))
	if geomStr == "" {
		geomStr = "{}"
	}
	if !json.Valid([]byte(geomStr)) || len(geomStr) > 64*1024 {
		return nil, errors.New("geometry JSON inválido o demasiado grande")
	}
	author := strings.TrimSpace(ann.Author)
	if author == "" {
		author = "usuario"
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := randomHexID("ann_")
	out := CanvasAnnotation{
		ID:             id,
		ProjectKey:     c.ProjectKey,
		CanvasID:       c.CanvasID,
		PageID:         strings.TrimSpace(ann.PageID),
		BoardFile:      boardFile,
		Kind:           kind,
		Geometry:       json.RawMessage(geomStr),
		TargetSelector: strings.TrimSpace(ann.TargetSelector),
		TargetText:     strings.TrimSpace(ann.TargetText),
		Comment:        comment,
		Author:         author,
		Status:         "open",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO canvas_annotations
			(id, project_key, canvas_id, page_id, board_file, kind, geometry_json,
			 target_selector, target_text, comment, author, status, resolution_note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		out.ID, out.ProjectKey, out.CanvasID, out.PageID, out.BoardFile, out.Kind, geomStr,
		out.TargetSelector, out.TargetText, out.Comment, out.Author, out.Status, out.CreatedAt, out.UpdatedAt); err != nil {
		return nil, fmt.Errorf("guardando anotación: %w", err)
	}
	return &out, nil
}

// ListCanvasAnnotations devuelve las anotaciones de un lienzo filtradas opcionalmente por estado y pantalla.
func (s *Store) ListCanvasAnnotations(ctx context.Context, project, canvasID, status, boardFile string) ([]CanvasAnnotation, error) {
	pKey := s.resolveProjectKey(ctx, ProjectKey(project))
	if pKey == "" {
		return []CanvasAnnotation{}, nil
	}
	cID := sanitizeCanvasID(canvasID)
	if cID != "" {
		if c, err := s.GetDesignCanvas(ctx, pKey, cID); err == nil {
			pKey = c.ProjectKey
			cID = c.CanvasID
		}
	} else if parent := s.ParentWorkspaceOf(ctx, pKey); parent != "" {
		// Si piden feedback para un sub-equipo sin especificar canvasId, incluir los del padre si el sub-equipo no tiene lienzos propios.
		var own int
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM design_canvases WHERE project_key = ?`, pKey).Scan(&own)
		if own == 0 {
			pKey = parent
		}
	}

	var conds []string
	var args []any
	conds = append(conds, "project_key = ?")
	args = append(args, pKey)
	if cID != "" {
		conds = append(conds, "canvas_id = ?")
		args = append(args, cID)
	}
	stFilter := strings.ToLower(strings.TrimSpace(status))
	if stFilter != "" && stFilter != "all" {
		conds = append(conds, "status = ?")
		args = append(args, stFilter)
	}
	if bf := strings.TrimSpace(boardFile); bf != "" {
		conds = append(conds, "board_file = ?")
		args = append(args, bf)
	}

	q := `SELECT id, project_key, canvas_id, page_id, board_file, kind, geometry_json,
	             target_selector, target_text, comment, author, status, resolution_note, created_at, updated_at
	      FROM canvas_annotations
	      WHERE ` + strings.Join(conds, " AND ") + `
	      ORDER BY CASE status WHEN 'open' THEN 0 WHEN 'applied' THEN 1 ELSE 2 END, created_at ASC`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CanvasAnnotation{}
	for rows.Next() {
		var a CanvasAnnotation
		var geom string
		if err := rows.Scan(&a.ID, &a.ProjectKey, &a.CanvasID, &a.PageID, &a.BoardFile, &a.Kind, &geom,
			&a.TargetSelector, &a.TargetText, &a.Comment, &a.Author, &a.Status, &a.ResolutionNote, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.Geometry = json.RawMessage(geom)
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateCanvasAnnotation actualiza el estado, comentario o nota de resolución de una anotación.
func (s *Store) UpdateCanvasAnnotation(ctx context.Context, project, canvasID, annID, status, comment, resolutionNote string) (*CanvasAnnotation, error) {
	c, err := s.GetDesignCanvas(ctx, project, canvasID)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(annID)
	if id == "" {
		return nil, errors.New("falta id de anotación")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var a CanvasAnnotation
	var geom string
	err = s.db.QueryRowContext(ctx, `
		SELECT id, project_key, canvas_id, page_id, board_file, kind, geometry_json,
		       target_selector, target_text, comment, author, status, resolution_note, created_at, updated_at
		FROM canvas_annotations
		WHERE id = ? AND project_key = ? AND canvas_id = ?`, id, c.ProjectKey, c.CanvasID).
		Scan(&a.ID, &a.ProjectKey, &a.CanvasID, &a.PageID, &a.BoardFile, &a.Kind, &geom,
			&a.TargetSelector, &a.TargetText, &a.Comment, &a.Author, &a.Status, &a.ResolutionNote, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.Geometry = json.RawMessage(geom)

	if st := strings.ToLower(strings.TrimSpace(status)); st != "" {
		if !validAnnotationStatuses[st] {
			return nil, fmt.Errorf("estado inválido %q (usa open, applied o resolved)", status)
		}
		a.Status = st
	}
	if cm := strings.TrimSpace(comment); cm != "" {
		if len([]rune(cm)) > 2000 {
			return nil, errors.New("el comentario excede 2000 caracteres")
		}
		a.Comment = cm
	}
	if note := strings.TrimSpace(resolutionNote); note != "" {
		a.ResolutionNote = note
	}
	a.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if _, err := s.db.ExecContext(ctx, `
		UPDATE canvas_annotations
		SET status = ?, comment = ?, resolution_note = ?, updated_at = ?
		WHERE id = ? AND project_key = ? AND canvas_id = ?`,
		a.Status, a.Comment, a.ResolutionNote, a.UpdatedAt, a.ID, a.ProjectKey, a.CanvasID); err != nil {
		return nil, err
	}
	return &a, nil
}

// DeleteCanvasAnnotation elimina una anotación del lienzo.
func (s *Store) DeleteCanvasAnnotation(ctx context.Context, project, canvasID, annID string) (bool, error) {
	c, err := s.GetDesignCanvas(ctx, project, canvasID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM canvas_annotations WHERE id = ? AND project_key = ? AND canvas_id = ?`,
		strings.TrimSpace(annID), c.ProjectKey, c.CanvasID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
