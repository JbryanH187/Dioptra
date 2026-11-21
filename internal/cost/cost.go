// Package cost mide la diferencia de tokens y el costo en USD entre imprimir un
// reporte técnico en consola (baseline) y renderizarlo a través de DioptraEngine
// (`fast_goldmark`, `blocks_override` o `layout_agent`).
package cost

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode"

	"dioptra/internal/contract"
)

const (
	// Sobrecarga fija estimada por turno en el agente anfitrión al usar `publish_report`:
	// wrapper de argumentos JSON + respuesta resumida de 3-5 líneas en consola + lectura del tool_result.
	goldmarkWrapperTokens = 24
	blocksWrapperTokens   = 28
	consoleSummaryTokens  = 55
	toolResultInputTokens = 95
	// ContractReadEstTokens: tamaño (~chars/4) de get_layout_contract sin tokens CSS;
	// lo verifica TestHostContractTokenBudget en layoutagent.
	ContractReadEstTokens = 1180
	contractReadEstTokens = ContractReadEstTokens
)

// Pricing define tarifas en USD por cada 1,000,000 de tokens.
type Pricing struct {
	Label              string
	InputPer1M         float64
	OutputPer1M        float64
	CacheReadPer1M     float64
	CacheWritePer1M    float64
	IsReferenceDefault bool
	// Source indica la procedencia de la tarifa para no presentar una referencia como dato real:
	// SourceVerified (contrastada contra usage.cost del Gateway), SourceEnv (definida por el usuario)
	// o SourceReference (tarifa pública/genérica, no contrastada).
	Source string
}

const (
	SourceVerified  = "verified"
	SourceEnv       = "env"
	SourceReference = "reference"
)

var defaultReferencePricing = Pricing{
	Label:              "tarifa ref. estándar ($3/$15 M)",
	InputPer1M:         3.00,
	OutputPer1M:        15.00,
	CacheReadPer1M:     0.30,
	CacheWritePer1M:    3.75,
	IsReferenceDefault: true,
}

type modelRule struct {
	match   func(m string) bool
	pricing Pricing
}

// Tarifario de referencia por familia de modelo (USD / 1M tokens).
// Se puede sobrescribir en cualquier momento con DIOPTRA_COST_INPUT_PER_1M y
// DIOPTRA_COST_OUTPUT_PER_1M (útil para proxies corporativos o tarifas privadas).
var modelCatalog = []modelRule{
	{
		match: func(m string) bool {
			return m == "cli" || m == "local" || strings.HasPrefix(m, "ollama/") || strings.Contains(m, "local-llm")
		},
		pricing: Pricing{Label: "local ($0/$0)", InputPer1M: 0, OutputPer1M: 0, CacheReadPer1M: 0, CacheWritePer1M: 0},
	},
	// Tarifas VERIFICADAS contra facturación de referencia (2026-11-18, mínimos cuadrados sin
	// caché, error relativo máx. 0). Deben ir ANTES de las reglas genéricas de familia.
	// CacheRead es un supuesto (10% de la entrada): sin muestras suficientes.
	{
		match:   func(m string) bool { return strings.Contains(m, "claude") && strings.Contains(m, "opus-5") },
		pricing: Pricing{Label: "claude-opus-5 ($4/$20 M)", InputPer1M: 4.00, OutputPer1M: 20.00, CacheReadPer1M: 0.40, CacheWritePer1M: 5.00, Source: SourceVerified},
	},
	{
		match:   func(m string) bool { return strings.Contains(m, "claude") && strings.Contains(m, "sonnet-5") },
		pricing: Pricing{Label: "claude-sonnet-5 ($2/$10 M)", InputPer1M: 2.00, OutputPer1M: 10.00, CacheReadPer1M: 0.20, CacheWritePer1M: 2.50, Source: SourceVerified},
	},
	{
		match:   func(m string) bool { return strings.Contains(m, "claude") && strings.Contains(m, "opus") },
		pricing: Pricing{Label: "claude-opus ($15/$75 M)", InputPer1M: 15.00, OutputPer1M: 75.00, CacheReadPer1M: 1.50, CacheWritePer1M: 18.75},
	},
	{
		match:   func(m string) bool { return strings.Contains(m, "claude") && strings.Contains(m, "haiku") },
		pricing: Pricing{Label: "claude-haiku ($0.80/$4 M)", InputPer1M: 0.80, OutputPer1M: 4.00, CacheReadPer1M: 0.08, CacheWritePer1M: 1.00},
	},
	{
		match:   func(m string) bool { return strings.Contains(m, "claude") || strings.Contains(m, "sonnet") },
		pricing: Pricing{Label: "claude-sonnet ($3/$15 M)", InputPer1M: 3.00, OutputPer1M: 15.00, CacheReadPer1M: 0.30, CacheWritePer1M: 3.75},
	},
	{
		match:   func(m string) bool { return strings.Contains(m, "gemini") && strings.Contains(m, "flash") },
		pricing: Pricing{Label: "gemini-flash ($0.15/$0.60 M)", InputPer1M: 0.15, OutputPer1M: 0.60, CacheReadPer1M: 0.0375, CacheWritePer1M: 0.15},
	},
	{
		match:   func(m string) bool { return strings.Contains(m, "gemini") },
		pricing: Pricing{Label: "gemini-pro ($1.25/$10 M)", InputPer1M: 1.25, OutputPer1M: 10.00, CacheReadPer1M: 0.3125, CacheWritePer1M: 1.25},
	},
	{
		match: func(m string) bool {
			return strings.Contains(m, "gpt-4o-mini") || strings.Contains(m, "gpt-5-mini") || strings.Contains(m, "o4-mini")
		},
		pricing: Pricing{Label: "gpt-mini ($0.15/$0.60 M)", InputPer1M: 0.15, OutputPer1M: 0.60, CacheReadPer1M: 0.075, CacheWritePer1M: 0.15},
	},
	{
		match:   func(m string) bool { return strings.Contains(m, "gpt-4o") },
		pricing: Pricing{Label: "gpt-4o ($2.50/$10 M)", InputPer1M: 2.50, OutputPer1M: 10.00, CacheReadPer1M: 1.25, CacheWritePer1M: 2.50},
	},
	{
		match:   func(m string) bool { return strings.Contains(m, "gpt-5") || strings.Contains(m, "codex") },
		pricing: Pricing{Label: "gpt-5 ($1.25/$10 M)", InputPer1M: 1.25, OutputPer1M: 10.00, CacheReadPer1M: 0.125, CacheWritePer1M: 1.25},
	},
}

// ResolvePricing determina las tarifas aplicables:
// 1. Variables DIOPTRA_COST_INPUT_PER_1M / DIOPTRA_COST_OUTPUT_PER_1M si están presentes.
// 2. Catálogo por nombre de modelo (parámetro `model` o DIOPTRA_HOST_MODEL).
// 3. Tarifa de referencia estándar explícita cuando el modelo es desconocido.
func ResolvePricing(model string) (string, Pricing) {
	effectiveModel := strings.TrimSpace(model)
	if effectiveModel == "" {
		effectiveModel = strings.TrimSpace(os.Getenv("DIOPTRA_HOST_MODEL"))
	}

	p := defaultReferencePricing
	if effectiveModel != "" {
		low := strings.ToLower(effectiveModel)
		for _, rule := range modelCatalog {
			if rule.match(low) {
				p = rule.pricing
				break
			}
		}
	} else {
		effectiveModel = "host-default"
	}

	inEnv, hasIn := parseFloatEnv("DIOPTRA_COST_INPUT_PER_1M")
	outEnv, hasOut := parseFloatEnv("DIOPTRA_COST_OUTPUT_PER_1M")
	if hasIn || hasOut {
		if hasIn {
			p.InputPer1M = inEnv
			p.CacheReadPer1M = inEnv * 0.10
			p.CacheWritePer1M = inEnv * 1.25
		}
		if hasOut {
			p.OutputPer1M = outEnv
		}
		if crEnv, hasCR := parseFloatEnv("DIOPTRA_COST_CACHE_READ_PER_1M"); hasCR {
			p.CacheReadPer1M = crEnv
		}
		p.IsReferenceDefault = false
		p.Source = SourceEnv
		p.Label = fmt.Sprintf("custom ($%.2f/$%.2f M)", p.InputPer1M, p.OutputPer1M)
	}
	if p.Source == "" {
		p.Source = SourceReference
	}

	return effectiveModel, p
}

func parseFloatEnv(name string) (float64, bool) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// EstimateTokens estima de forma determinista el conteo de tokens BPE de un texto
// técnico/Markdown/JSON en español/inglés (~3.5-4.0 caracteres por token),
// ponderando identidades cortas, puntuación JSON y saltos de línea.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	tokens := 0
	run := 0
	flushRun := func() {
		if run == 0 {
			return
		}
		// Bloques alfanuméricos de hasta 4 caracteres suelen caer en 1 token; los más largos se fragmentan.
		tokens += (run + 3) / 4
		run = 0
	}

	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			run++
		case r == ' ' || r == '\t':
			flushRun()
		case r == '\n':
			flushRun()
			tokens++
		default:
			flushRun()
			tokens++
		}
	}
	flushRun()

	// Ajuste conservador mínimo para caracteres multi-byte si el texto era muy compacto.
	byBytes := (len(s) + 3) / 4
	if byBytes > tokens {
		return byBytes
	}
	return tokens
}

// ComputeParams describe los datos de una publicación para calcular su impacto en tokens y USD.
type ComputeParams struct {
	Mode              string // "fast_goldmark" | "blocks_override" | "layout_agent"
	Model             string
	Markdown          string
	BlocksOverrideRaw []byte
	LayoutMeta        *contract.LayoutMeta
}

// Compute calcula la comparativa Baseline (imprimir en consola) vs DioptraEngine.
func Compute(p ComputeParams) contract.CostReport {
	modelHint := strings.TrimSpace(p.Model)
	if modelHint == "" && p.LayoutMeta != nil && p.LayoutMeta.Model != "" {
		if p.LayoutMeta.Provider == "cli" {
			modelHint = "cli"
		} else {
			modelHint = p.LayoutMeta.Model
		}
	}

	resolvedModel, pricing := ResolvePricing(modelHint)
	baselineOut := EstimateTokens(p.Markdown)
	baselineUSD := tokensCost(baselineOut, pricing.OutputPer1M)

	rep := contract.CostReport{
		Mode:                 p.Mode,
		Model:                resolvedModel,
		PricingLabel:         pricing.Label,
		PricingSource:        pricing.Source,
		BaselineOutputTokens: baselineOut,
		BaselineUSD:          roundUSD(baselineUSD),
	}

	var extraIn int
	var dioptraOut int
	var dioptraUSD float64

	switch p.Mode {
	case "blocks_override":
		blocksTok := EstimateTokens(string(p.BlocksOverrideRaw))
		dioptraOut = baselineOut + blocksTok + blocksWrapperTokens + consoleSummaryTokens
		extraIn = toolResultInputTokens
		dioptraUSD = tokensCost(extraIn, pricing.InputPer1M) + tokensCost(dioptraOut, pricing.OutputPer1M)
		rep.ContractNote = fmt.Sprintf("+~%s in si llamaste get_layout_contract por 1ra vez en la sesión (~$%.4f USD amortizable)",
			formatInt(contractReadEstTokens), tokensCost(contractReadEstTokens, pricing.InputPer1M))

	case "layout_agent":
		// Si hay LayoutMeta con conteo real del proveedor, usamos los tokens exactos de esa llamada.
		hostOut := baselineOut + goldmarkWrapperTokens + consoleSummaryTokens
		if p.LayoutMeta != nil && (p.LayoutMeta.InputTokens > 0 || p.LayoutMeta.OutputTokens > 0) {
			rep.IsExact = true
			extraIn = p.LayoutMeta.InputTokens + toolResultInputTokens
			dioptraOut = hostOut + p.LayoutMeta.OutputTokens
			agentCost := tokensCost(p.LayoutMeta.InputTokens, pricing.InputPer1M) +
				tokensCost(p.LayoutMeta.OutputTokens, pricing.OutputPer1M) +
				tokensCost(p.LayoutMeta.CacheReadTokens, pricing.CacheReadPer1M) +
				tokensCost(p.LayoutMeta.CacheWriteTokens, pricing.CacheWritePer1M)
			hostCost := tokensCost(toolResultInputTokens, pricing.InputPer1M) + tokensCost(hostOut, pricing.OutputPer1M)
			dioptraUSD = agentCost + hostCost
		} else {
			// Proveedor `cli` local o sin usage reportado
			dioptraOut = hostOut
			extraIn = toolResultInputTokens
			dioptraUSD = tokensCost(extraIn, pricing.InputPer1M) + tokensCost(dioptraOut, pricing.OutputPer1M)
		}

	default:
		// Ruta rápida determinista goldmark (0 gasto extra de razonamiento)
		dioptraOut = baselineOut + goldmarkWrapperTokens + consoleSummaryTokens
		extraIn = toolResultInputTokens
		dioptraUSD = tokensCost(extraIn, pricing.InputPer1M) + tokensCost(dioptraOut, pricing.OutputPer1M)
	}

	rep.DioptraInputTokens = extraIn
	rep.DioptraOutputTokens = dioptraOut
	rep.ExtraOutputTokens = dioptraOut - baselineOut
	if baselineOut > 0 {
		rep.DeltaPct = math.Round((float64(rep.ExtraOutputTokens)/float64(baselineOut))*1000) / 10
	}
	rep.DioptraUSD = roundUSD(dioptraUSD)
	rep.DeltaUSD = roundUSD(rep.DioptraUSD - rep.BaselineUSD)

	sourceTag := "est."
	if rep.IsExact {
		sourceTag = "real"
	}
	signTok := "+"
	if rep.ExtraOutputTokens < 0 {
		signTok = ""
	}
	signUSD := "+"
	if rep.DeltaUSD < 0 {
		signUSD = "-"
	}
	absDeltaUSD := math.Abs(rep.DeltaUSD)

	rep.ConsoleLine = fmt.Sprintf(
		"Costo Dioptra [%s · %s · %s · tarifa %s]: %s%s tok salida (%s%.1f%% vs consola) | Consola: $%.4f -> Dioptra: $%.4f (%s$%.4f USD)",
		rep.Mode,
		sourceTag,
		pricing.Label,
		pricing.Source,
		signTok,
		formatInt(rep.ExtraOutputTokens),
		signTok,
		rep.DeltaPct,
		rep.BaselineUSD,
		rep.DioptraUSD,
		signUSD,
		absDeltaUSD,
	)

	return rep
}

func tokensCost(tokens int, usdPer1M float64) float64 {
	if tokens <= 0 || usdPer1M <= 0 {
		return 0
	}
	return (float64(tokens) / 1_000_000.0) * usdPer1M
}

func roundUSD(v float64) float64 {
	return math.Round(v*1_000_000) / 1_000_000
}

func formatInt(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var out []byte
	rem := len(s) % 3
	if rem > 0 {
		out = append(out, s[:rem]...)
	}
	for i := rem; i < len(s); i += 3 {
		if len(out) > 0 {
			out = append(out, ',')
		}
		out = append(out, s[i:i+3]...)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
