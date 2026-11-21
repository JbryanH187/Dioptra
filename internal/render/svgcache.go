package render

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
)

// Caché LRU de SVG compilados con D2. El layout de D2 cuesta ~50 ms por diagrama
// y además se ejecuta bajo un mutex global (el Ruler no es thread-safe), así que
// republicar un reporte sin cambios en sus diagramas no debe recompilarlos.
const svgCacheCap = 256

type svgCacheEntry struct {
	key string
	svg string
}

var (
	svgCacheMu    sync.Mutex
	svgCacheList  = list.New()
	svgCacheIndex = map[string]*list.Element{}
)

// svgCacheKey identifica la salida: fuente D2 + etiquetas completas + severidades.
func svgCacheKey(d2Source string, labels, sevs map[string]string) string {
	h := sha256.New()
	h.Write([]byte(d2Source))
	for _, m := range []map[string]string{labels, sevs} {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		h.Write([]byte{0})
		h.Write([]byte(strings.Join(keys, "\x1f")))
		for _, k := range keys {
			h.Write([]byte{0x1e})
			h.Write([]byte(m[k]))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func svgCacheGet(key string) (string, bool) {
	svgCacheMu.Lock()
	defer svgCacheMu.Unlock()
	if el, ok := svgCacheIndex[key]; ok {
		svgCacheList.MoveToFront(el)
		return el.Value.(*svgCacheEntry).svg, true
	}
	return "", false
}

func svgCachePut(key, svg string) {
	svgCacheMu.Lock()
	defer svgCacheMu.Unlock()
	if el, ok := svgCacheIndex[key]; ok {
		el.Value.(*svgCacheEntry).svg = svg
		svgCacheList.MoveToFront(el)
		return
	}
	svgCacheIndex[key] = svgCacheList.PushFront(&svgCacheEntry{key: key, svg: svg})
	for svgCacheList.Len() > svgCacheCap {
		oldest := svgCacheList.Back()
		svgCacheList.Remove(oldest)
		delete(svgCacheIndex, oldest.Value.(*svgCacheEntry).key)
	}
}
