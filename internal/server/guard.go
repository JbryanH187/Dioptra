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

func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
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
func guardRequest(r *http.Request) (int, string, string) {
	if !isLoopbackHost(r.Host) {
		return http.StatusForbidden, "invalid_host", "Host no permitido: Dioptra solo atiende 127.0.0.1/localhost"
	}
	if isSafeMethod(r.Method) {
		return 0, "", ""
	}
	if o := r.Header.Get("Origin"); o != "" {
		scheme := "http"
		if r.TLS != nil {
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
