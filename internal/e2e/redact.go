package e2e

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxBodyBytes es el tope por request/response/log guardado en un paso.
const MaxBodyBytes = 16 * 1024

// Cabeceras que nunca se guardan con su valor.
var sensitiveHeaders = map[string]bool{
	"authorization": true, "proxy-authorization": true, "cookie": true, "set-cookie": true,
	"x-api-key": true, "x-auth-token": true, "x-access-token": true, "x-csrf-token": true, "x-xsrf-token": true,
}

// Campos JSON / de formulario cuyo valor se oculta completo.
var sensitiveKey = regexp.MustCompile(`(?i)(pass(word|wd)?|pwd|secret|token|api[_-]?key|authorization|cookie|session|pin|cvv|cvc|nip|tarjeta|card[_-]?number|pan|otp)`)

var (
	digitRun   = regexp.MustCompile(`\b\d(?:[ -]?\d){12,18}\b`)
	emailRe    = regexp.MustCompile(`\b([A-Za-z0-9._%+-])[A-Za-z0-9._%+-]*@([A-Za-z0-9.-]+\.[A-Za-z]{2,})\b`)
	curpRe     = regexp.MustCompile(`\b[A-Z][AEIOUX][A-Z]{2}\d{6}[HM][A-Z]{5}[A-Z0-9]\d\b`)
	rfcRe      = regexp.MustCompile(`\b[A-ZÑ&]{3,4}\d{6}[A-Z0-9]{3}\b`)
	phoneRe    = regexp.MustCompile(`(?:\+?52[ -]?)?\b\d{2,3}[ -]?\d{3,4}[ -]?\d{4}\b`)
	bearerRe   = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`)
	jwtRe      = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)
	kvSecretRe = regexp.MustCompile(`(?i)\b((?:pass(?:word|wd)?|pwd|secret|token|api[_-]?key|pin|cvv|nip)\s*[=:]\s*)("?)([^\s&"',;]{3,})`)
)

// Redactor cuenta cuántos datos ocultó.
type Redactor struct {
	Count int
}

func luhnOK(digits string) bool {
	sum, alt := 0, false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// Text oculta secretos y datos personales en texto libre (logs, bodies no JSON, URLs).
func (r *Redactor) Text(s string) string {
	if s == "" {
		return s
	}
	s = jwtRe.ReplaceAllStringFunc(s, func(string) string { r.Count++; return "[JWT oculto]" })
	s = bearerRe.ReplaceAllStringFunc(s, func(m string) string {
		r.Count++
		return strings.Fields(m)[0] + " [oculto]"
	})
	s = kvSecretRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := kvSecretRe.FindStringSubmatch(m)
		r.Count++
		return sub[1] + sub[2] + "[oculto]"
	})
	s = digitRun.ReplaceAllStringFunc(s, func(m string) string {
		digits := strings.NewReplacer(" ", "", "-", "").Replace(m)
		if len(digits) >= 13 && len(digits) <= 19 && luhnOK(digits) {
			r.Count++
			return "****" + lastN(digits, 4)
		}
		return m
	})
	s = curpRe.ReplaceAllStringFunc(s, func(m string) string { r.Count++; return "[CURP ****" + lastN(m, 3) + "]" })
	s = rfcRe.ReplaceAllStringFunc(s, func(m string) string { r.Count++; return "[RFC ****" + lastN(m, 3) + "]" })
	s = emailRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := emailRe.FindStringSubmatch(m)
		r.Count++
		return sub[1] + "***@" + sub[2]
	})
	s = phoneRe.ReplaceAllStringFunc(s, func(m string) string {
		digits := strings.NewReplacer(" ", "", "-", "", "+", "").Replace(m)
		if len(digits) != 10 && !(len(digits) == 12 && strings.HasPrefix(digits, "52")) {
			return m // un teléfono de México tiene 10 dígitos (12 con lada 52)
		}
		r.Count++
		return "***" + lastN(digits, 4)
	})
	return s
}

// Llaves con datos personales de la persona (nombre, domicilio): se deja la inicial.
var personalKey = regexp.MustCompile(`(?i)^(nombres?|apellidos?|apellido_?(paterno|materno)|primer_?nombre|segundo_?nombre|nombre_?completo|full_?name|first_?name|last_?name|surname|direcci[oó]n|domicilio|address|calle|colonia)$`)

// value oculta recursivamente los valores de llaves sensibles en JSON decodificado.
func (r *Redactor) value(key string, v any) any {
	if s, ok := v.(string); ok && key != "" && personalKey.MatchString(key) && s != "" {
		r.Count++
		first, _ := utf8.DecodeRuneInString(s)
		return string(first) + "***"
	}
	if key != "" && sensitiveKey.MatchString(key) {
		switch v.(type) {
		case map[string]any, []any:
			// Objetos anidados bajo una llave sensible: se revisan adentro.
		case nil:
			return nil
		default:
			r.Count++
			return "[oculto]"
		}
	}
	switch t := v.(type) {
	case map[string]any:
		for k, inner := range t {
			t[k] = r.value(k, inner)
		}
		return t
	case []any:
		for i, inner := range t {
			t[i] = r.value("", inner)
		}
		return t
	case string:
		return r.Text(t)
	default:
		return v
	}
}

// Body oculta datos sensibles en un body: si es JSON, por llave y por contenido;
// si no, como texto. Luego recorta a MaxBodyBytes.
func (r *Redactor) Body(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	if (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) && json.Valid([]byte(trimmed)) {
		var v any
		if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
			out, err := json.MarshalIndent(r.value("", v), "", "  ")
			if err == nil {
				return Truncate(string(out), MaxBodyBytes)
			}
		}
	}
	if strings.Contains(trimmed, "=") && !strings.ContainsAny(trimmed, " \n<{") {
		// Formulario urlencoded: a=1&password=x
		parts := strings.Split(trimmed, "&")
		for i, p := range parts {
			if k, _, ok := strings.Cut(p, "="); ok && sensitiveKey.MatchString(k) {
				parts[i] = k + "=[oculto]"
				r.Count++
			}
		}
		trimmed = strings.Join(parts, "&")
	}
	return Truncate(r.Text(trimmed), MaxBodyBytes)
}

// Header devuelve el valor a guardar para una cabecera.
func (r *Redactor) Header(name, value string) string {
	if sensitiveHeaders[strings.ToLower(name)] {
		r.Count++
		return "[oculto]"
	}
	return r.Text(value)
}

// Truncate recorta a n bytes sin partir un carácter UTF-8.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "\n… [recortado]"
}

// Step enmascara todos los campos de texto de un paso.
func (r *Redactor) Step(st Step) Step {
	st.URL = r.Text(st.URL)
	st.Request = r.Body(st.Request)
	st.Response = r.Body(st.Response)
	st.Log = Truncate(r.Text(st.Log), MaxBodyBytes)
	st.Detail = r.Text(st.Detail)
	st.Title = r.Text(st.Title)
	return st
}
