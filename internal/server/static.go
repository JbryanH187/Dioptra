package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"dioptra/ui"
)

// ETags de los recursos embebidos. embed.FS no tiene fecha de modificación, así
// que sin esto el navegador volvía a descargar todos los módulos en cada carga.
// Se calculan una sola vez: el contenido embebido no cambia mientras corre el binario.
var (
	assetETagsOnce sync.Once
	assetETags     map[string]string
)

func assetETag(name string) string {
	assetETagsOnce.Do(func() {
		assetETags = map[string]string{}
		_ = fs.WalkDir(ui.Assets, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if b, rerr := fs.ReadFile(ui.Assets, p); rerr == nil {
				sum := sha256.Sum256(b)
				assetETags[p] = `"` + hex.EncodeToString(sum[:8]) + `"`
			}
			return nil
		})
	})
	return assetETags[name]
}

// writeCacheHeaders fija la política de caché de un recurso estático y responde
// 304 si el navegador ya tiene esa versión. Devuelve true si ya se respondió.
func writeCacheHeaders(w http.ResponseWriter, r *http.Request, name string) bool {
	if strings.HasPrefix(name, "fonts/") {
		// Las fuentes no cambian entre versiones del visor.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		// Revalidar siempre: un binario nuevo debe servir la UI nueva al instante.
		w.Header().Set("Cache-Control", "no-cache")
	}
	etag := assetETag(name)
	if etag == "" {
		return false
	}
	w.Header().Set("ETag", etag)
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}
