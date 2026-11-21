package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
)

// verifyBackup abre la copia en solo lectura y exige quick_check = ok.
func verifyBackup(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var res string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&res); err != nil {
		return fmt.Errorf("verificando respaldo: %w", err)
	}
	if res != "ok" {
		return fmt.Errorf("el respaldo no pasó quick_check: %s", res)
	}
	return nil
}
