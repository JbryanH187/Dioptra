package store

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"

	_ "modernc.org/sqlite"
)

var nonSlugRe = regexp.MustCompile(`[^a-z0-9-]+`)

// Store administra los archivos .md fuente de verdad y el índice SQLite puro.
type Store struct {
	baseDir string
	db      *sql.DB
	hasFTS  bool
	mu      sync.Mutex
	disk    diskState
}

// DefaultReportsDir devuelve ~/.dioptra/reports expandido de forma segura.
func DefaultReportsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".dioptra", "reports")
	}
	return filepath.Join(home, ".dioptra", "reports")
}

// Open inicializa el directorio raíz de reportes .md y el índice SQLite (modernc.org/sqlite).
func Open(baseDir string) (*Store, error) {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = DefaultReportsDir()
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	indexDir := filepath.Join(absBase, ".index")
	cacheDir := filepath.Join(absBase, ".cache")
	for _, d := range []string{absBase, indexDir, cacheDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("creando directorio %s: %w", d, err)
		}
	}

	dbPath := filepath.Join(indexDir, "dioptra.db")
	dsn := "file:" + filepath.ToSlash(dbPath) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abriendo SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{
		baseDir: absBase,
		db:      db,
	}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// BaseDir devuelve la ruta absoluta donde residen los .md fuente de verdad.
func (s *Store) BaseDir() string {
	return s.baseDir
}

// Close cierra la conexión SQLite limpia y ordenadamente.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func makeDocID(relSlash string) string {
	sum := sha1.Sum([]byte(relSlash))
	return "r_" + hex.EncodeToString(sum[:6])
}

func inferFirstHeading(md string) string {
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}

func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch r {
		case 'á', 'à', 'ä', 'â':
			b.WriteByte('a')
		case 'é', 'è', 'ë', 'ê':
			b.WriteByte('e')
		case 'í', 'ì', 'ï', 'î':
			b.WriteByte('i')
		case 'ó', 'ò', 'ö', 'ô':
			b.WriteByte('o')
		case 'ú', 'ù', 'ü', 'û':
			b.WriteByte('u')
		case 'ñ':
			b.WriteByte('n')
		default:
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			} else {
				b.WriteByte('-')
			}
		}
	}
	res := strings.Trim(nonSlugRe.ReplaceAllString(b.String(), "-"), "-")
	if res == "" {
		return "reporte"
	}
	if len(res) > 64 {
		res = strings.Trim(res[:64], "-")
	}
	return res
}

func writeFileAtomic(targetPath string, data []byte) error {
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, targetPath)
}
