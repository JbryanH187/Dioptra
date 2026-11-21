// Package layoutagent implementa el Agente Único de Dioptra (Fase 5): un sintetizador
// visual determinista que convierte Markdown técnico en el sobre del Contrato B usando
// un LLM configurable (anthropic, gemini u openai-compatible) y lo somete al validador
// tolerante ADR 1 y a un Guardián de Fidelidad antes de entregarlo al almacén.
package layoutagent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"dioptra/internal/contract"
	"dioptra/internal/render"
)

const (
	defaultTimeout       = 25 * time.Second
	defaultMaxTokens     = 16384
	defaultMaxInputBytes = 256 * 1024
)

// Config agrupa las variables de entorno DIOPTRA_LAYOUT_*. Todas son opcionales: el flujo
// principal por MCP no requiere credenciales (el agente anfitrión envía blocks_override).
type Config struct {
	Provider          string        // DIOPTRA_LAYOUT_PROVIDER (vacío = autodetección)
	CLICommand        string        // DIOPTRA_LAYOUT_CLI, p. ej. "claude -p" (solo `dioptra render`)
	APIURL            string        // DIOPTRA_LAYOUT_API_URL (adaptador HTTP secundario opcional)
	APIKey            string        // DIOPTRA_LAYOUT_API_KEY
	Model             string        // DIOPTRA_LAYOUT_MODEL
	Timeout           time.Duration // DIOPTRA_LAYOUT_TIMEOUT (por defecto 25s)
	MaxTokens         int           // DIOPTRA_LAYOUT_MAX_TOKENS (por defecto 16384)
	MaxTokensExplicit bool          // true si el usuario fijó MAX_TOKENS (openai-compatible solo lo envía entonces)
	MaxInputBytes     int           // DIOPTRA_LAYOUT_MAX_INPUT_BYTES (por defecto 256 KiB)
	AnthropicMode     string        // DIOPTRA_LAYOUT_ANTHROPIC_MODE: tool (defecto) | prefill
	Temperature       float64       // DIOPTRA_LAYOUT_TEMPERATURE (por defecto 0)
	OmitTemperature   bool          // DIOPTRA_LAYOUT_TEMPERATURE=none para modelos que no la aceptan
	Client            *http.Client
}

// ConfigFromEnv carga la configuración desde las variables DIOPTRA_LAYOUT_*.
func ConfigFromEnv() Config {
	cfg := Config{
		Provider:      strings.TrimSpace(os.Getenv("DIOPTRA_LAYOUT_PROVIDER")),
		CLICommand:    strings.TrimSpace(os.Getenv("DIOPTRA_LAYOUT_CLI")),
		APIURL:        strings.TrimSpace(os.Getenv("DIOPTRA_LAYOUT_API_URL")),
		APIKey:        strings.TrimSpace(os.Getenv("DIOPTRA_LAYOUT_API_KEY")),
		Model:         strings.TrimSpace(os.Getenv("DIOPTRA_LAYOUT_MODEL")),
		AnthropicMode: strings.ToLower(strings.TrimSpace(os.Getenv("DIOPTRA_LAYOUT_ANTHROPIC_MODE"))),
	}
	if d, ok := parseDurationEnv(os.Getenv("DIOPTRA_LAYOUT_TIMEOUT")); ok {
		cfg.Timeout = d
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("DIOPTRA_LAYOUT_MAX_TOKENS"))); err == nil && n > 0 {
		cfg.MaxTokens = n
		cfg.MaxTokensExplicit = true
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("DIOPTRA_LAYOUT_MAX_INPUT_BYTES"))); err == nil && n > 0 {
		cfg.MaxInputBytes = n
	}
	switch t := strings.ToLower(strings.TrimSpace(os.Getenv("DIOPTRA_LAYOUT_TEMPERATURE"))); t {
	case "":
	case "none", "omit", "off":
		cfg.OmitTemperature = true
	default:
		if f, err := strconv.ParseFloat(t, 64); err == nil && f >= 0 && f <= 2 {
			cfg.Temperature = f
		}
	}
	return cfg
}

func parseDurationEnv(raw string) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d, true
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second, true
	}
	return 0, false
}

// IsConfigured indica si el Agente Único tiene una configuración resoluble.
func (c Config) IsConfigured() bool {
	_, err := c.Resolve()
	return err == nil
}

// EffectiveTimeout devuelve el tiempo límite total por síntesis
// (25 s para HTTP, 180 s para CLI locales que razonan más lento).
func (c Config) EffectiveTimeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	if r, err := c.Resolve(); err == nil && r.Provider == ProviderCLI {
		return defaultCLITimeout
	}
	return defaultTimeout
}

// ForDaemon devuelve la configuración apta para el demonio `dioptra serve`: el proveedor
// cli nunca se lanza desde el servidor (solo desde `dioptra render` en una terminal).
func (c Config) ForDaemon() Config {
	c.CLICommand = ""
	if p, ok := NormalizeProvider(c.Provider); ok && p == ProviderCLI {
		c.Provider = ""
	}
	return c
}

// HasHTTPSettings indica si el usuario definió variables del adaptador HTTP opcional.
func (c Config) HasHTTPSettings() bool {
	return strings.TrimSpace(c.APIKey) != "" || strings.TrimSpace(c.APIURL) != ""
}

func (c Config) maxTokens() int {
	if c.MaxTokens > 0 {
		return c.MaxTokens
	}
	return defaultMaxTokens
}

func (c Config) maxInputBytes() int {
	if c.MaxInputBytes > 0 {
		return c.MaxInputBytes
	}
	return defaultMaxInputBytes
}

func (c Config) anthropicMode() string {
	if c.AnthropicMode == anthropicModePrefill {
		return anthropicModePrefill
	}
	return anthropicModeTool
}

func (c Config) httpClient() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	// Sin Timeout propio: el contexto de Synthesize gobierna el presupuesto total.
	return &http.Client{}
}

// Input es el documento a sintetizar.
type Input struct {
	Title    string
	Project  string
	TS       string
	Markdown string
}

// Result es la salida validada y auditada del Agente Único.
type Result struct {
	Envelope *contract.Envelope
	Warnings []contract.Warning
	Meta     contract.LayoutMeta
}

// ErrInputTooLarge indica que el documento excede DIOPTRA_LAYOUT_MAX_INPUT_BYTES.
var ErrInputTooLarge = errors.New("documento excede DIOPTRA_LAYOUT_MAX_INPUT_BYTES")

// Synthesize ejecuta el pipeline completo del Agente Único:
// línea base goldmark → LLM → limpieza de cercas → validador ADR 1 → Guardián de Fidelidad.
// Ante cualquier error el llamador debe degradar a la ruta rápida goldmark.
func Synthesize(ctx context.Context, cfg Config, in Input) (*Result, error) {
	r, err := cfg.Resolve()
	if err != nil {
		return nil, err
	}
	meta := contract.LayoutMeta{
		Provider:      r.Provider,
		Model:         r.Model,
		Endpoint:      endpointLabel(r.Endpoint),
		PromptVersion: PromptVersion,
	}
	if len(in.Markdown) > cfg.maxInputBytes() {
		return &Result{Meta: meta}, fmt.Errorf("%w (%d > %d bytes)", ErrInputTooLarge, len(in.Markdown), cfg.maxInputBytes())
	}
	if strings.TrimSpace(in.TS) == "" {
		in.TS = time.Now().Format(time.RFC3339)
	}

	// Línea base determinista (0 tokens): fuente canónica de tareas chk-N, código y referencias.
	baseline, _, baseErr := render.ConvertMarkdownToEnvelope(ctx, in.Title, in.Project, in.TS, in.Markdown)
	if baseErr != nil {
		baseline = nil
	}
	taskCount := 0
	if baseline != nil {
		taskCount = len(contract.CollectChecklists(baseline.Report.Blocks))
	}

	system := SystemPrompt()
	toolMode := r.Provider == ProviderAnthropic && cfg.anthropicMode() == anthropicModeTool
	user := BuildUserPrompt(in, taskCount, toolMode)

	callCtx, cancel := context.WithTimeout(ctx, cfg.EffectiveTimeout())
	defer cancel()

	started := time.Now()
	reply, attempts, err := callProvider(callCtx, cfg, r, system, user)
	meta.LatencyMs = time.Since(started).Milliseconds()
	meta.Attempts = attempts
	meta.InputTokens = reply.usage.input
	meta.OutputTokens = reply.usage.output
	meta.CacheReadTokens = reply.usage.cacheRead
	meta.CacheWriteTokens = reply.usage.cacheWrite
	if err != nil {
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return &Result{Meta: meta}, fmt.Errorf("tiempo límite %s agotado invocando %s: %w", cfg.EffectiveTimeout(), r.Provider, err)
		}
		return &Result{Meta: meta}, err
	}

	env, warnings, err := contract.ValidateAndNormalizeRawJSON(ExtractJSONObject(reply.text))
	if err != nil {
		return &Result{Meta: meta}, fmt.Errorf("salida inválida del Agente Único: %w", err)
	}
	env.Report.Title = firstNonEmpty(in.Title, env.Report.Title)
	env.Report.Project = firstNonEmpty(in.Project, env.Report.Project)
	env.Report.TS = in.TS

	report, err := ApplyFidelityGuard(env, baseline, in.Markdown)
	meta.Coverage = report.Coverage
	meta.FidelityRepairs = report.Repairs
	if err != nil {
		return &Result{Meta: meta}, err
	}
	warnings = append(warnings, report.Warnings...)
	env.Warnings = warnings

	return &Result{Envelope: env, Warnings: warnings, Meta: meta}, nil
}

// GenerateLayout conserva la firma histórica del cliente básico (compatibilidad).
func GenerateLayout(ctx context.Context, cfg Config, title, project, ts, markdown string) (*contract.Envelope, []contract.Warning, error) {
	res, err := Synthesize(ctx, cfg, Input{Title: title, Project: project, TS: ts, Markdown: markdown})
	if err != nil {
		return nil, nil, err
	}
	return res.Envelope, res.Warnings, nil
}

func endpointLabel(endpoint string) string {
	if i := strings.IndexByte(endpoint, '?'); i >= 0 {
		return endpoint[:i]
	}
	return endpoint
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
