package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckDB_DetectsMissingMarkdownAndReportsHealthy(t *testing.T) {
	st, dir := openStore(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	r := pub(t, st, "Sano", "p", "", "", "# Sano\n\nok", now)

	rep, err := st.CheckDB(ctx)
	if err != nil || rep.QuickCheck != "ok" || len(rep.MissingMD) != 0 || rep.SchemaVersion != rep.LatestVersion {
		t.Fatalf("estado sano esperado: %+v err=%v", rep, err)
	}

	if err := os.Remove(r.Summary.MarkdownPath); err != nil {
		t.Fatal(err)
	}
	_ = dir
	rep, _ = st.CheckDB(ctx)
	if len(rep.MissingMD) != 1 {
		t.Fatalf("debía detectar el .md faltante: %+v", rep.MissingMD)
	}
}

func TestBackupTo_ConsistentCopyAndNoOverwrite(t *testing.T) {
	st, dir := openStore(t)
	ctx := context.Background()
	pub(t, st, "Uno", "p", "", "", "# Uno\n\na", time.Now())
	dest := filepath.Join(dir, ".backups", "b.db")
	if err := st.BackupTo(ctx, dest); err != nil {
		t.Fatalf("respaldo: %v", err)
	}
	if fi, err := os.Stat(dest); err != nil || fi.Size() == 0 {
		t.Fatalf("respaldo vacío o ausente: %v", err)
	}
	if err := st.BackupTo(ctx, dest); err == nil {
		t.Fatal("no debe sobrescribir un respaldo existente")
	}
}

func TestRebuildSearchIndex_RestoresSearchFromMarkdown(t *testing.T) {
	st, _ := openStore(t)
	ctx := context.Background()
	pub(t, st, "Cuerpo raro", "p", "", "", "# Cuerpo\n\npalabraunica123", time.Now())
	if _, err := st.RebuildSearchIndex(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := st.ListReportsFiltered(ctx, "", "", "", "palabraunica123", 10)
	if err != nil || len(got) != 1 {
		t.Fatalf("la búsqueda por cuerpo debía funcionar tras reindexar: %d err=%v", len(got), err)
	}
}
