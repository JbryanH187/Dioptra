package e2e

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

// MaxHARBytes es el tamaño máximo de un HAR que se acepta.
const MaxHARBytes = 64 << 20

type harFile struct {
	Log struct {
		Entries []harEntry `json:"entries"`
	} `json:"log"`
}

type harEntry struct {
	StartedDateTime string  `json:"startedDateTime"`
	Time            float64 `json:"time"`
	ResourceType    string  `json:"_resourceType"`
	Initiator       *struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"_initiator"`
	Request struct {
		Method   string      `json:"method"`
		URL      string      `json:"url"`
		Headers  []harHeader `json:"headers"`
		PostData *struct {
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"postData"`
	} `json:"request"`
	Response struct {
		Status  int         `json:"status"`
		Headers []harHeader `json:"headers"`
		Content struct {
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
			Encoding string `json:"encoding"`
		} `json:"content"`
	} `json:"response"`
}

type harHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HARStats resume qué se tomó del HAR.
type HARStats struct {
	Entries  int `json:"entries"`
	Steps    int `json:"steps"`
	Skipped  int `json:"skipped"`
	Services int `json:"services"`
}

var staticExt = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".svg": true, ".ico": true, ".webp": true, ".avif": true, ".woff": true, ".woff2": true, ".ttf": true,
	".otf": true, ".eot": true, ".map": true, ".mp4": true, ".webm": true, ".mp3": true,
}

// Cabeceras de request que vale la pena conservar (el resto es ruido del navegador).
var keepRequestHeaders = map[string]bool{
	"content-type": true, "accept": true, "authorization": true, "cookie": true, "x-api-key": true,
	"x-request-id": true, "x-correlation-id": true, "traceparent": true, "x-b3-traceid": true,
}

func isStatic(e harEntry, u *url.URL) bool {
	if strings.EqualFold(e.Request.Method, "OPTIONS") {
		return true // preflight CORS: ruido, no es un paso del flujo
	}
	switch strings.ToLower(e.ResourceType) {
	case "xhr", "fetch", "document", "websocket", "eventsource":
		return false
	case "script", "stylesheet", "image", "font", "media", "manifest", "texttrack", "ping", "preflight":
		return true
	}
	if staticExt[strings.ToLower(path.Ext(u.Path))] {
		return true
	}
	mt := strings.ToLower(e.Response.Content.MimeType)
	return strings.HasPrefix(mt, "image/") || strings.HasPrefix(mt, "font/") || strings.Contains(mt, "javascript") || strings.Contains(mt, "css")
}

// ServiceFromURL nombra un servicio por su host (y puerto si no es el estándar).
func ServiceFromURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Host
}

// ParseHAR convierte un HAR (exportado de DevTools › Network) en pasos enmascarados,
// en orden cronológico. Omite estáticos (scripts, estilos, imágenes, fuentes) y preflights.
func ParseHAR(rd io.Reader, red *Redactor) ([]Step, []Service, HARStats, error) {
	var f harFile
	data, err := io.ReadAll(io.LimitReader(rd, MaxHARBytes+1))
	if err != nil {
		return nil, nil, HARStats{}, err
	}
	if len(data) > MaxHARBytes {
		return nil, nil, HARStats{}, fmt.Errorf("el HAR supera %d MB", MaxHARBytes>>20)
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, nil, HARStats{}, fmt.Errorf("no es un HAR válido: %w", err)
	}
	entries := f.Log.Entries
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].StartedDateTime < entries[j].StartedDateTime })

	stats := HARStats{Entries: len(entries)}
	steps := make([]Step, 0, len(entries))
	seen := map[string]Service{}
	origin := ""

	for _, e := range entries {
		u, err := url.Parse(e.Request.URL)
		if err != nil || u.Host == "" {
			stats.Skipped++
			continue
		}
		if strings.EqualFold(e.ResourceType, "document") && origin == "" {
			origin = u.Host
		}
		if isStatic(e, u) {
			stats.Skipped++
			continue
		}
		svc := ServiceFromURL(u)
		if _, ok := seen[svc]; !ok {
			seen[svc] = Service{Name: svc, BaseURL: u.Scheme + "://" + u.Host, Port: u.Port()}
		}

		var req strings.Builder
		for _, h := range e.Request.Headers {
			if keepRequestHeaders[strings.ToLower(h.Name)] {
				fmt.Fprintf(&req, "%s: %s\n", h.Name, red.Header(h.Name, h.Value))
			}
		}
		if e.Request.PostData != nil && strings.TrimSpace(e.Request.PostData.Text) != "" {
			if req.Len() > 0 {
				req.WriteString("\n")
			}
			req.WriteString(red.Body(e.Request.PostData.Text))
		}

		body := e.Response.Content.Text
		if strings.EqualFold(e.Response.Content.Encoding, "base64") {
			if dec, err := base64.StdEncoding.DecodeString(body); err == nil && isTextual(e.Response.Content.MimeType) {
				body = string(dec)
			} else {
				body = fmt.Sprintf("[contenido binario %s]", e.Response.Content.MimeType)
			}
		}

		from := "navegador"
		if origin != "" && origin != u.Host {
			from = origin
		}
		ts := e.StartedDateTime
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			ts = t.UTC().Format(time.RFC3339)
		}
		st := red.Step(Step{
			Time:       ts,
			Kind:       "request",
			From:       from,
			Service:    svc,
			Method:     strings.ToUpper(e.Request.Method),
			URL:        e.Request.URL,
			Status:     e.Response.Status,
			DurationMs: int64(e.Time + 0.5),
		})
		st.Request = Truncate(req.String(), MaxBodyBytes)
		st.Response = red.Body(body)
		st.Verdict = verdictFromStatus(st.Status)
		if e.Response.Status == 0 {
			st.Verdict = "error"
			st.Detail = "sin respuesta (cancelada, CORS o red)"
		}
		steps = append(steps, st)
	}

	services := make([]Service, 0, len(seen))
	for _, s := range seen {
		services = append(services, s)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	stats.Steps = len(steps)
	stats.Services = len(services)
	return steps, services, stats, nil
}

func isTextual(mime string) bool {
	m := strings.ToLower(mime)
	return strings.HasPrefix(m, "text/") || strings.Contains(m, "json") || strings.Contains(m, "xml") || strings.Contains(m, "x-www-form-urlencoded")
}
