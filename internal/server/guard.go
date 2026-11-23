package server

import (
	"mime"
	"net"
	"net/http"
	"strings"
)

// Guardia de solicitudes del demonio local (anti DNS-rebinding y anti CSRF).
//
// Dioptra solo escucha en loopback, pero un navegador puede alcanzarlo desde
// cualquier página web: con un formulario o fetch "simple" (text/plain, sin
// preflight CORS) o resolviendo un dominio del atacante a 127.0.0.1 (DNS
// rebinding). Sin esta guardia, una web externa podía publicar reportes y
// marcar casillas de action-checklist que el agente lee como aprobación humana.
//
// Reglas:
//  1. Host debe ser un nombre de loopback (127.0.0.1, localhost, ::1) en
//     TODAS las rutas: bloquea DNS rebinding también para lecturas.
//  2. En métodos que modifican estado (todo salvo GET/HEAD/OPTIONS):
//     a. Origin, si viene, debe ser exactamente el mismo origen
//        (rechaza también "null", p. ej. el iframe aislado de /canvas).
//     b. Sec-Fetch-Site, si viene, no puede ser "cross-site".
//     c. POST/PUT/PATCH exigen Content-Type application/json, que fuerza
//        preflight CORS en navegadores (que Dioptra nunca concede).
//
// Los clientes locales legítimos (puente MCP, CLI, tests, UI propia) ya
// cumplen estas reglas: usan Host 127.0.0.1 y JSON, y la UI es mismo origen.

func extractNormalizedHost(hostport string) string {
	host := strings.TrimSpace(hostport)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	return strings.TrimSuffix(host, ".")
}

func isLoopbackHost(hostport string) bool {
	switch extractNormalizedHost(hostport) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// normalizeAllowedHosts limpia la lista opt-in (--allow-host / DIOPTRA_ALLOW_HOSTS).
// Rechaza comodines (*), espacios y rutas: solo acepta nombres de host explícitos.
func normalizeAllowedHosts(raw []string) map[string]struct{} {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(raw))
	for _, entry := range raw {
		for _, part := range strings.Split(entry, ",") {
			h := extractNormalizedHost(part)
			if h == "" || strings.ContainsAny(h, "* /\\:@") {
				continue
			}
			out[h] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isAllowedHost(hostport string, extra map[string]struct{}) bool {
	if isLoopbackHost(hostport) {
		return true
	}
	if len(extra) == 0 {
		return false
	}
	_, ok := extra[extractNormalizedHost(hostport)]
	return ok
}

func isLoopbackRemote(remoteAddr string) bool {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func requiresJSONBody(m string) bool {
	return m == http.MethodPost || m == http.MethodPut || m == http.MethodPatch
}

func isJSONContentType(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mt == "application/json" || (strings.HasPrefix(mt, "application/") && strings.HasSuffix(mt, "+json"))
}

// guardRequest devuelve (status, código, mensaje) si la solicitud debe
// rechazarse, o status 0 si puede continuar.
func guardRequest(r *http.Request, extraHosts map[string]struct{}) (int, string, string) {
	if !isAllowedHost(r.Host, extraHosts) {
		return http.StatusForbidden, "invalid_host", "Host no permitido: Dioptra solo atiende 127.0.0.1/localhost (o hosts explícitos en --allow-host)"
	}
	if isSafeMethod(r.Method) {
		return 0, "", ""
	}
	if o := r.Header.Get("Origin"); o != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		} else if len(extraHosts) > 0 && isLoopbackRemote(r.RemoteAddr) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		if !strings.EqualFold(o, scheme+"://"+r.Host) {
			return http.StatusForbidden, "invalid_origin", "Origin no permitido"
		}
	}
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return http.StatusForbidden, "invalid_origin", "Solicitud cross-site no permitida"
	}
	if requiresJSONBody(r.Method) && !isJSONContentType(r.Header.Get("Content-Type")) {
		return http.StatusUnsupportedMediaType, "invalid_content_type", "Content-Type debe ser application/json"
	}
	return 0, "", ""
}
