package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dioptra/internal/contract"
	"dioptra/internal/render"
)

// DiskChange es un reporte que cambió por una edición directa en disco.
type DiskChange struct {
	Action  string // "published" (archivo nuevo) | "updated" (archivo existente editado)
	Summary contract.ReportSummary
}

// diskState recuerda la última fecha de modificación y el hash puro del markdown
// vistos o escritos por Dioptra, para detectar ediciones hechas a mano (o por otra
// herramienta) sin confundir escrituras propias (Publish / UpdateChecklistItem)
// ni degradar reportes con blocks_override / free-canvas a fast_goldmark.
type diskState struct {
	mu     sync.Mutex
	mtimes map[string]time.Time
	hashes map[string]string
	primed bool
}

type knownReport struct {
	id, title, project, mdHash string
}

// recordDiskWrite registra el mtime y el hash puro del .md recién escrito por el
// propio Dioptra para que SyncDiskChanges no lo re-publique como edición manual.
func (s *Store) recordDiskWrite(relSlash, absPath, mdContent string) {
	info, err := os.Stat(absPath)
	if err != nil {
		return
	}
	key := filepath.ToSlash(relSlash)
	mdHash := computeMarkdownHash(mdContent)
	s.disk.mu.Lock()
	if s.disk.mtimes == nil {
		s.disk.mtimes = map[string]time.Time{}
	}
	if s.disk.hashes == nil {
		s.disk.hashes = map[string]string{}
	}
	s.disk.mtimes[key] = info.ModTime()
	s.disk.hashes[key] = mdHash
	s.disk.mu.Unlock()
}

// SyncDiskChanges recorre el directorio de reportes UNA vez, con una sola consulta
// a la base (antes: una consulta por archivo en cada tick), indexa los .md nuevos
// y republica los existentes cuyo mtime y contenido .md realmente cambiaron. La
// primera pasada solo registra los mtimes. No toma s.mu: Publish e
// indexExistingMarkdownFile lo toman solos.
func (s *Store) SyncDiskChanges(ctx context.Context) ([]DiskChange, error) {
	known := map[string]knownReport{}
	rows, err := s.db.QueryContext(ctx, `SELECT id, md_rel_path, title, project, md_hash FROM reports`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r knownReport
		var rel string
		if err := rows.Scan(&r.id, &rel, &r.title, &r.project, &r.mdHash); err == nil {
			known[rel] = r
		}
	}
	rows.Close()

	s.disk.mu.Lock()
	if s.disk.mtimes == nil {
		s.disk.mtimes = map[string]time.Time{}
	}
	if s.disk.hashes == nil {
		s.disk.hashes = map[string]string{}
	}
	primed := s.disk.primed
	s.disk.mu.Unlock()

	var changes []DiskChange
	walkErr := filepath.WalkDir(s.baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(s.baseDir, path)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mtime := info.ModTime()

		s.disk.mu.Lock()
		prev, seen := s.disk.mtimes[relSlash]
		prevHash := s.disk.hashes[relSlash]
		s.disk.mtimes[relSlash] = mtime
		s.disk.mu.Unlock()

		rep, isKnown := known[relSlash]
		if !isKnown {
			rawMD, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			fm := render.ParseFrontmatter(string(rawMD))
			title := fm.Title
			if title == "" {
				title = strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			}
			project := fm.Project
			if project == "" {
				project = "dioptra"
			}
			if res, err := s.indexExistingMarkdownFile(ctx, relSlash, title, project, mtime, string(rawMD)); err == nil && res != nil {
				changes = append(changes, DiskChange{Action: "published", Summary: res.Summary})
			}
			return nil
		}

		if !primed || !seen || mtime.Equal(prev) {
			return nil
		}
		rawMD, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		diskHash := computeMarkdownHash(string(rawMD))
		s.disk.mu.Lock()
		s.disk.hashes[relSlash] = diskHash
		s.disk.mu.Unlock()
		// Si el contenido puro del .md no cambió respecto al último estado guardado
		// en SQLite o en memoria (p. ej. Publish actualizó blocks_override, otro
		// proceso reescribió el mismo .md o se tocó el mtime), NO re-publicar por
		// fast_goldmark: preservaríamos intactos los bloques blocks_override/canvas.
		if (rep.mdHash != "" && rep.mdHash == diskHash) || (prevHash != "" && prevHash == diskHash) {
			return nil
		}
		res, err := s.Publish(ctx, PublishParams{
			ID:         rep.id,
			Title:      rep.title,
			Project:    rep.project,
			Markdown:   string(rawMD),
			ChangeNote: "Editado directamente en disco",
		})
		if err == nil && res != nil && res.Action == "updated" {
			changes = append(changes, DiskChange{Action: "updated", Summary: res.Summary})
		}
		return nil
	})

	s.disk.mu.Lock()
	s.disk.primed = true
	s.disk.mu.Unlock()
	return changes, walkErr
}
