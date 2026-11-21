package layoutagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Dialectos soportados por DIOPTRA_LAYOUT_PROVIDER.
const (
	ProviderCLI       = "cli"
	ProviderAnthropic = "anthropic"
	ProviderGemini    = "gemini"
	ProviderOpenAI    = "openai-compatible"
)

const (
	anthropicVersion     = "2023-06-01"
	defaultAnthropicURL  = "https://api.anthropic.com/v1/messages"
	defaultGeminiBase    = "https://generativelanguage.googleapis.com/v1beta"
	defaultOpenAIURL     = "https://api.openai.com/v1/chat/completions"
	legacyOpenAIModel    = "gpt-4o-mini"
	envelopeToolName     = "emit_dioptra_envelope"
	maxProviderAttempts  = 2
	maxRetryAfter        = 3 * time.Second
	maxErrorSnippet      = 300
	maxProviderRespBytes = 8 * 1024 * 1024
	anthropicModeTool    = "tool"
	anthropicModePrefill = "prefill"
)

// ProviderError representa un fallo HTTP o semántico devuelto por el proveedor.
type ProviderError struct {
	Provider string
	Status   int
	Message  string
}

func (e *ProviderError) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("%s respondió HTTP %d: %s", e.Provider, e.Status, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Provider, e.Message)
}

// ErrTruncated indica que el modelo agotó su presupuesto de salida (JSON incompleto).
var ErrTruncated = errors.New("respuesta truncada por límite de tokens de salida")

// Resolved es la configuración efectiva tras normalizar proveedor, endpoint y modelo.
type Resolved struct {
	Provider       string
	ProviderSource string // "explicit" | "auto:cli" | "auto:url" | "auto:model"
	Endpoint       string
	Model          string
	Command        []string // solo proveedor cli: argv ya resuelto
}

type usage struct {
	input, output, cacheRead, cacheWrite int
}

type providerReply struct {
	text  string
	usage usage
}

// NormalizeProvider traduce alias comunes al dialecto canónico.
func NormalizeProvider(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "anthropic", "claude":
		return ProviderAnthropic, true
	case "gemini", "google", "vertex-gemini":
		return ProviderGemini, true
	case "openai-compatible", "openai", "openai_compatible", "compatible", "azure-openai", "litellm", "ollama", "vllm":
		return ProviderOpenAI, true
	case "cli", "local-cli", "exec":
		return ProviderCLI, true
	}
	return "", false
}

// DetectProvider infiere el dialecto por la URL y, en su defecto, por el nombre del modelo.
func DetectProvider(apiURL, model string) (provider, source string) {
	u := strings.ToLower(apiURL)
	switch {
	case strings.Contains(u, "anthropic.com") || strings.HasSuffix(strings.TrimRight(u, "/"), "/v1/messages"):
		return ProviderAnthropic, "auto:url"
	case strings.Contains(u, "generativelanguage.googleapis.com") || strings.Contains(u, ":generatecontent"):
		return ProviderGemini, "auto:url"
	case strings.Contains(u, "/chat/completions") || strings.Contains(u, "openai"):
		return ProviderOpenAI, "auto:url"
	}
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "claude") || strings.Contains(m, "anthropic"):
		return ProviderAnthropic, "auto:model"
	case strings.HasPrefix(m, "gemini") || strings.HasPrefix(m, "models/gemini"):
		return ProviderGemini, "auto:model"
	case m != "" || u != "":
		return ProviderOpenAI, "auto:model"
	}
	return "", ""
}

// Resolve valida la configuración y calcula proveedor, endpoint y modelo efectivos.
// Prioridad: DIOPTRA_LAYOUT_PROVIDER explícito → DIOPTRA_LAYOUT_CLI (CLI ya autenticada,
// sin API keys) → adaptador HTTP secundario si existen DIOPTRA_LAYOUT_API_KEY/URL.
func (c Config) Resolve() (Resolved, error) {
	apiURL := strings.TrimSpace(c.APIURL)
	model := strings.TrimSpace(c.Model)
	key := strings.TrimSpace(c.APIKey)
	cliCmd := strings.TrimSpace(c.CLICommand)

	r := Resolved{}
	explicit := strings.TrimSpace(c.Provider) != ""
	if explicit {
		p, ok := NormalizeProvider(c.Provider)
		if !ok {
			return Resolved{}, fmt.Errorf("DIOPTRA_LAYOUT_PROVIDER=%q no soportado (usa cli, anthropic, gemini u openai-compatible)", c.Provider)
		}
		r.Provider, r.ProviderSource = p, "explicit"
	}

	if r.Provider == ProviderCLI || (!explicit && cliCmd != "") {
		return resolveCLI(cliCmd, explicit)
	}

	if !explicit {
		if apiURL == "" && key == "" {
			return Resolved{}, errors.New("sin sintetizador local configurado (opcional): define DIOPTRA_LAYOUT_CLI (ej. \"claude -p\") o DIOPTRA_LAYOUT_API_KEY")
		}
		r.Provider, r.ProviderSource = DetectProvider(apiURL, model)
		if r.Provider == "" {
			return Resolved{}, errors.New("no se pudo inferir el proveedor: define DIOPTRA_LAYOUT_PROVIDER o DIOPTRA_LAYOUT_MODEL")
		}
	} else if apiURL == "" && key == "" {
		return Resolved{}, fmt.Errorf("DIOPTRA_LAYOUT_PROVIDER=%s requiere DIOPTRA_LAYOUT_API_KEY o DIOPTRA_LAYOUT_API_URL", r.Provider)
	}

	switch r.Provider {
	case ProviderAnthropic:
		if model == "" {
			return Resolved{}, errors.New("anthropic requiere DIOPTRA_LAYOUT_MODEL (ej. el id exacto del modelo Claude habilitado en tu cuenta)")
		}
		r.Model = model
		r.Endpoint = resolveSuffixEndpoint(apiURL, defaultAnthropicURL, "/v1/messages", "/messages")
		if key == "" && strings.Contains(r.Endpoint, "api.anthropic.com") {
			return Resolved{}, errors.New("anthropic requiere DIOPTRA_LAYOUT_API_KEY")
		}
	case ProviderGemini:
		if model == "" {
			model = modelFromGeminiURL(apiURL)
		}
		model = strings.TrimPrefix(model, "models/")
		if model == "" {
			return Resolved{}, errors.New("gemini requiere DIOPTRA_LAYOUT_MODEL (o una URL .../models/{modelo}:generateContent)")
		}
		r.Model = model
		r.Endpoint = resolveGeminiEndpoint(apiURL, model)
		if key == "" && strings.Contains(r.Endpoint, "generativelanguage.googleapis.com") {
			return Resolved{}, errors.New("gemini requiere DIOPTRA_LAYOUT_API_KEY")
		}
	case ProviderOpenAI:
		if model == "" {
			model = legacyOpenAIModel
		}
		r.Model = model
		r.Endpoint = resolveSuffixEndpoint(apiURL, defaultOpenAIURL, "/v1/chat/completions", "/chat/completions")
		if key == "" && strings.Contains(r.Endpoint, "api.openai.com") {
			return Resolved{}, errors.New("openai-compatible contra api.openai.com requiere DIOPTRA_LAYOUT_API_KEY")
		}
	}

	if _, err := url.ParseRequestURI(r.Endpoint); err != nil {
		return Resolved{}, fmt.Errorf("endpoint inválido %q: %w", r.Endpoint, err)
	}
	return r, nil
}

// resolveSuffixEndpoint respeta URLs completas de pasarelas y solo completa bases desnudas.
func resolveSuffixEndpoint(apiURL, def, fullSuffix, v1Suffix string) string {
	if apiURL == "" {
		return def
	}
	u, err := url.Parse(apiURL)
	if err != nil {
		return apiURL
	}
	p := strings.TrimRight(u.Path, "/")
	switch {
	case p == "":
		u.Path = fullSuffix
	case strings.HasSuffix(p, "/v1"):
		u.Path = p + v1Suffix
	default:
		return apiURL
	}
	return u.String()
}

func modelFromGeminiURL(apiURL string) string {
	idx := strings.Index(apiURL, "/models/")
	if idx < 0 {
		return ""
	}
	rest := apiURL[idx+len("/models/"):]
	if end := strings.IndexAny(rest, ":/?"); end >= 0 {
		rest = rest[:end]
	}
	if rest == "{model}" {
		return ""
	}
	return rest
}

func resolveGeminiEndpoint(apiURL, model string) string {
	if apiURL == "" {
		return defaultGeminiBase + "/models/" + url.PathEscape(model) + ":generateContent"
	}
	if strings.Contains(apiURL, "{model}") {
		return strings.ReplaceAll(apiURL, "{model}", url.PathEscape(model))
	}
	if strings.Contains(apiURL, ":generateContent") {
		return apiURL
	}
	u, err := url.Parse(apiURL)
	if err != nil {
		return apiURL
	}
	p := strings.TrimRight(u.Path, "/")
	switch {
	case p == "":
		u.Path = "/v1beta/models/" + model + ":generateContent"
	case strings.HasSuffix(p, "/v1beta") || strings.HasSuffix(p, "/v1"):
		u.Path = p + "/models/" + model + ":generateContent"
	default:
		return apiURL
	}
	return u.String()
}

// callProvider ejecuta la llamada con un reintento acotado ante 429/5xx o fallos de red.
func callProvider(ctx context.Context, cfg Config, r Resolved, system, user string) (providerReply, int, error) {
	if r.Provider == ProviderCLI {
		reply, err := callCLI(ctx, r, system, user)
		return reply, 1, err
	}

	var body any
	headers := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}
	key := strings.TrimSpace(cfg.APIKey)

	switch r.Provider {
	case ProviderAnthropic:
		body = buildAnthropicBody(cfg, r, system, user)
		headers["anthropic-version"] = anthropicVersion
		if key != "" {
			headers["x-api-key"] = key
		}
	case ProviderGemini:
		body = buildGeminiBody(cfg, system, user)
		if key != "" {
			headers["x-goog-api-key"] = key
		}
	default:
		body = buildOpenAIBody(cfg, r, system, user)
		if key != "" {
			headers["Authorization"] = "Bearer " + key
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return providerReply{}, 0, err
	}

	attempts := 0
	var lastErr error
	for attempts < maxProviderAttempts {
		attempts++
		status, respBody, retryAfter, err := postJSON(ctx, cfg.httpClient(), r.Endpoint, headers, payload)
		if err != nil {
			lastErr = &ProviderError{Provider: r.Provider, Message: redact(err.Error(), key)}
			// Solo se reintenta si la petición nunca llegó (fallo al conectar). Un error
			// de lectura o timeout tras enviarla puede significar que el proveedor ya
			// generó (y cobró) la respuesta: reintentar duplicaba costo y espera.
			if ctx.Err() != nil || attempts >= maxProviderAttempts || status != 0 || !isDialError(err) {
				break
			}
			if !sleepCtx(ctx, 400*time.Millisecond) {
				break
			}
			continue
		}
		if status < 200 || status >= 300 {
			lastErr = &ProviderError{Provider: r.Provider, Status: status, Message: redact(snippet(respBody), key)}
			if !isRetryableStatus(status) || attempts >= maxProviderAttempts {
				break
			}
			wait := 600 * time.Millisecond
			if retryAfter > 0 {
				wait = retryAfter
			}
			if !sleepCtx(ctx, wait) {
				break
			}
			continue
		}

		var reply providerReply
		switch r.Provider {
		case ProviderAnthropic:
			reply, err = parseAnthropic(respBody, cfg.anthropicMode())
		case ProviderGemini:
			reply, err = parseGemini(respBody)
		default:
			reply, err = parseOpenAI(respBody)
		}
		if err != nil {
			return providerReply{}, attempts, err
		}
		return reply, attempts, nil
	}
	return providerReply{}, attempts, lastErr
}

func postJSON(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, payload []byte) (int, []byte, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderRespBytes))
	if err != nil {
		return resp.StatusCode, nil, 0, err
	}
	var retryAfter time.Duration
	if ra := strings.TrimSpace(resp.Header.Get("Retry-After")); ra != "" {
		if secs, convErr := strconv.Atoi(ra); convErr == nil && secs >= 0 {
			retryAfter = time.Duration(secs) * time.Second
			if retryAfter > maxRetryAfter {
				retryAfter = maxRetryAfter
			}
		}
	}
	return resp.StatusCode, respBody, retryAfter, nil
}

// isRetryableStatus: solo estados en los que el proveedor NO procesó la petición
// (límite de tasa, no disponible, sobrecargado). 500/502/504/408 pueden llegar
// después de generar la respuesta, así que reintentarlos podía cobrar dos veces.
func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable, 529:
		return true
	}
	return false
}

// isDialError indica que la conexión ni siquiera se estableció.
func isDialError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > maxErrorSnippet {
		s = s[:maxErrorSnippet] + "…"
	}
	return s
}

func redact(s, key string) string {
	if key != "" && len(key) >= 6 {
		s = strings.ReplaceAll(s, key, "[REDACTED]")
	}
	return s
}

// ---------------------------------------------------------------------------
// Anthropic /v1/messages
// ---------------------------------------------------------------------------

func buildAnthropicBody(cfg Config, r Resolved, system, user string) map[string]any {
	body := map[string]any{
		"model":      r.Model,
		"max_tokens": cfg.maxTokens(),
		// Caché de prompt efímero: el bloque de sistema (y las herramientas que lo preceden)
		// es idéntico entre llamadas, por lo que las lecturas posteriores se cobran con descuento.
		"system": []map[string]any{{
			"type":          "text",
			"text":          system,
			"cache_control": map[string]any{"type": "ephemeral"},
		}},
	}
	if !cfg.OmitTemperature {
		body["temperature"] = cfg.Temperature
	}
	messages := []map[string]any{{"role": "user", "content": user}}
	if cfg.anthropicMode() == anthropicModePrefill {
		messages = append(messages, map[string]any{"role": "assistant", "content": "{"})
	} else {
		body["tools"] = []map[string]any{{
			"name":         envelopeToolName,
			"description":  "Entrega el sobre JSON completo del Contrato B v1 de DioptraEngine.",
			"input_schema": envelopeToolSchema(),
		}}
		body["tool_choice"] = map[string]any{"type": "tool", "name": envelopeToolName}
	}
	body["messages"] = messages
	return body
}

func envelopeToolSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"v": map[string]any{"type": "integer", "enum": []int{1}},
			"report": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":  map[string]any{"type": "string"},
					"ts":     map[string]any{"type": "string"},
					"blocks": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				},
				"required": []string{"title", "blocks"},
			},
		},
		"required": []string{"v", "report"},
	}
}

func parseAnthropic(body []byte, mode string) (providerReply, error) {
	var resp struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return providerReply{}, &ProviderError{Provider: ProviderAnthropic, Message: "respuesta no es JSON: " + snippet(body)}
	}
	if resp.StopReason == "max_tokens" {
		return providerReply{}, fmt.Errorf("%s: %w", ProviderAnthropic, ErrTruncated)
	}
	u := usage{
		input:      resp.Usage.InputTokens,
		output:     resp.Usage.OutputTokens,
		cacheRead:  resp.Usage.CacheReadInputTokens,
		cacheWrite: resp.Usage.CacheCreationInputTokens,
	}
	var text strings.Builder
	for _, c := range resp.Content {
		if c.Type == "tool_use" && len(c.Input) > 0 {
			return providerReply{text: string(c.Input), usage: u}, nil
		}
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	out := text.String()
	if mode == anthropicModePrefill && !strings.HasPrefix(strings.TrimSpace(out), "{") {
		out = "{" + out
	}
	if strings.TrimSpace(out) == "" {
		return providerReply{}, &ProviderError{Provider: ProviderAnthropic, Message: "respuesta sin contenido utilizable (stop_reason=" + resp.StopReason + ")"}
	}
	return providerReply{text: out, usage: u}, nil
}

// ---------------------------------------------------------------------------
// Gemini generateContent
// ---------------------------------------------------------------------------

func buildGeminiBody(cfg Config, system, user string) map[string]any {
	gen := map[string]any{
		"responseMimeType": "application/json",
		"maxOutputTokens":  cfg.maxTokens(),
	}
	if !cfg.OmitTemperature {
		gen["temperature"] = cfg.Temperature
	}
	return map[string]any{
		"systemInstruction": map[string]any{"parts": []map[string]any{{"text": system}}},
		"contents":          []map[string]any{{"role": "user", "parts": []map[string]any{{"text": user}}}},
		"generationConfig":  gen,
	}
}

func parseGemini(body []byte) (providerReply, error) {
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
		UsageMetadata struct {
			PromptTokenCount        int `json:"promptTokenCount"`
			CandidatesTokenCount    int `json:"candidatesTokenCount"`
			CachedContentTokenCount int `json:"cachedContentTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return providerReply{}, &ProviderError{Provider: ProviderGemini, Message: "respuesta no es JSON: " + snippet(body)}
	}
	if resp.PromptFeedback.BlockReason != "" {
		return providerReply{}, &ProviderError{Provider: ProviderGemini, Message: "prompt bloqueado: " + resp.PromptFeedback.BlockReason}
	}
	if len(resp.Candidates) == 0 {
		return providerReply{}, &ProviderError{Provider: ProviderGemini, Message: "respuesta sin candidatos"}
	}
	cand := resp.Candidates[0]
	switch cand.FinishReason {
	case "MAX_TOKENS":
		return providerReply{}, fmt.Errorf("%s: %w", ProviderGemini, ErrTruncated)
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return providerReply{}, &ProviderError{Provider: ProviderGemini, Message: "generación detenida: " + cand.FinishReason}
	}
	var text strings.Builder
	for _, p := range cand.Content.Parts {
		if p.Thought {
			continue
		}
		text.WriteString(p.Text)
	}
	if strings.TrimSpace(text.String()) == "" {
		return providerReply{}, &ProviderError{Provider: ProviderGemini, Message: "candidato sin texto (finishReason=" + cand.FinishReason + ")"}
	}
	return providerReply{
		text: text.String(),
		usage: usage{
			input:     resp.UsageMetadata.PromptTokenCount,
			output:    resp.UsageMetadata.CandidatesTokenCount,
			cacheRead: resp.UsageMetadata.CachedContentTokenCount,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// OpenAI-compatible /v1/chat/completions
// ---------------------------------------------------------------------------

func buildOpenAIBody(cfg Config, r Resolved, system, user string) map[string]any {
	body := map[string]any{
		"model":           r.Model,
		"response_format": map[string]any{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	if !cfg.OmitTemperature {
		body["temperature"] = cfg.Temperature
	}
	if cfg.MaxTokensExplicit {
		body["max_tokens"] = cfg.maxTokens()
	}
	return body
}

func parseOpenAI(body []byte) (providerReply, error) {
	var resp struct {
		V       any `json:"v"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		// Pasarelas que devuelven texto plano con el sobre (posiblemente con cercas)
		return providerReply{text: string(body)}, nil
	}
	if resp.V != nil {
		// Pasarela personalizada que devuelve el sobre directamente
		return providerReply{text: string(body)}, nil
	}
	if len(resp.Choices) == 0 {
		return providerReply{}, &ProviderError{Provider: ProviderOpenAI, Message: "respuesta sin choices: " + snippet(body)}
	}
	ch := resp.Choices[0]
	if ch.FinishReason == "length" {
		return providerReply{}, fmt.Errorf("%s: %w", ProviderOpenAI, ErrTruncated)
	}
	if strings.TrimSpace(ch.Message.Content) == "" {
		return providerReply{}, &ProviderError{Provider: ProviderOpenAI, Message: "choice sin contenido (finish_reason=" + ch.FinishReason + ")"}
	}
	return providerReply{
		text: ch.Message.Content,
		usage: usage{
			input:     resp.Usage.PromptTokens,
			output:    resp.Usage.CompletionTokens,
			cacheRead: resp.Usage.PromptTokensDetails.CachedTokens,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Limpieza defensiva de la salida
// ---------------------------------------------------------------------------

// ExtractJSONObject elimina cercas ```json y texto accesorio, devolviendo el primer
// objeto JSON balanceado (consciente de cadenas y escapes).
func ExtractJSONObject(raw string) []byte {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "\ufeff")
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		}
		if end := strings.LastIndex(s, "```"); end >= 0 {
			s = s[:end]
		}
		s = strings.TrimSpace(s)
	}
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return []byte(s)
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case ch == '\\':
				esc = true
			case ch == '"':
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return []byte(s[start : i+1])
			}
		}
	}
	return []byte(s[start:])
}
