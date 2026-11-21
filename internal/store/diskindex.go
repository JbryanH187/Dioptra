package store

// Indexación de los .md que ya existen en disco.
// Movido ÍNTEGRO desde store.go (F7: partir archivos gigantes).

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"dioptra/internal/render"
)

// SyncMarkdownFromDisk escanea recursivamente archivos .md en baseDir que aún no estén indexados.
func (s *Store) SyncMarkdownFromDisk(ctx context.Context) error {
	_, err := s.SyncDiskChanges(ctx)
	return err
}

func (s *Store) indexExistingMarkdownFile(ctx context.Context, relSlash, title, project string, ts time.Time, mdContent string) (*PublishResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tsISO := ts.Format(time.RFC3339)
	env, warnings, err := render.ConvertMarkdownToEnvelope(ctx, title, project, tsISO, mdContent)
	if err != nil {
		return nil, err
	}

	slug := strings.TrimSuffix(filepath.Base(relSlash), filepath.Ext(relSlash))
	docID := makeDocID(relSlash)
	env.Report.ID = docID
	env.Report.Revision = 1

	return s.persistEnvelopeLocked(ctx, docID, slug, relSlash, mdContent, env, warnings, "fast_goldmark", persistMeta{
		ContentHash:  computePublishHash(title, project, mdContent, nil, false),
		Revision:     1,
		SQLUpdatedAt: tsISO,
		Action:       "created",
	})
}

type existingReportMeta struct {
	ID             string
	Slug           string
	RelPath        string
	CreatedTS      string
	UpdatedAt      string
	Revision       int
	ContentHash    string
	LastChange     string
	IdempotencyKey string
	EnvelopeJSON   string
	WarningsCount  int
	Title          string
	Project        string
	MaxSev         string
	Excerpt        string
	Tool           string
	Model          string
}

type persistMeta struct {
	Tool           string
	Model          string
	IdempotencyKey string
	ContentHash    string
	Revision       int
	UpdatedAt      string
	SQLUpdatedAt   string
	LastChange     string
	Action         string
}
