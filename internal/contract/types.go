package contract

import "encoding/json"

// CompactSchemaB es el esquema ultracompacto del Contrato B (§5, 857 caracteres)
// que se envía como única guía estructural al Agente Maquetador Interno.
const CompactSchemaB = `{"v":1,"report":{"title":"s","ts":"iso8601","blocks":"B[]"},"B":"{t:<key>,...props}; ?=optional; extra keys ignored","S":"info|success|warning|error","md":"**b** ` + "`c`" + ` [t](https://u) path:line; prose adds - lists","kpi":{"items":[{"label":"s","value":"s|n","unit":"s?","delta":"s?","sev":"S?"}]},"callout":{"sev":"S","title":"s","body":"md","ref":"s?"},"table":{"cols":[{"key":"s","label":"s","type":"text|num|sev|ref"}],"rows":"{key:val}[]","sort":"key?","filter":"bool?"},"section":{"title":"s","sev":"S?","open":"bool?","blocks":"B[]"},"code":{"lang":"s","src":"s","old":"s?","ref":"s?","hl":"n[]?"},"diagram":{"kind":"flow|seq|tree","nodes":[{"id":"s","label":"s","sev":"S?"}],"edges":"[from,to,label?][]"},"checklist":{"items":[{"id":"s","text":"s","done":"bool","owner":"s?","sev":"S?"}]},"prose":{"title":"s?","md":"md"},"canvas":{"html":"s","h":"n?"}}`

// Warning representa un aviso de tolerancia controlada según §2 ADR 1.
type Warning struct {
	Path   string `json:"path"`
	Action string `json:"action"` // "ignored" | "repaired" | "degraded"
	Reason string `json:"reason"`
}

// LayoutMeta describe la ejecución del Agente Único de Dioptra (Fase 5) para
// diagnóstico de rendimiento: proveedor, modelo, latencia, consumo y fidelidad.
type LayoutMeta struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	Endpoint         string  `json:"endpoint"`
	PromptVersion    string  `json:"promptVersion"`
	LatencyMs        int64   `json:"latencyMs"`
	Attempts         int     `json:"attempts"`
	InputTokens      int     `json:"inputTokens"`
	OutputTokens     int     `json:"outputTokens"`
	CacheReadTokens  int     `json:"cacheReadTokens"`
	CacheWriteTokens int     `json:"cacheWriteTokens"`
	Coverage         float64 `json:"coverage"`
	FidelityRepairs  int     `json:"fidelityRepairs"`
}

// FidelityMeta resume la auditoría del Guardián de Fidelidad sobre un blocks_override
// enviado por un agente anfitrión MCP, para que pueda autocorregirse.
type FidelityMeta struct {
	Verdict     string  `json:"verdict"` // "clean" | "repaired" | "rejected"
	Coverage    float64 `json:"coverage"`
	Repairs     int     `json:"repairs"`
	SourceTasks int     `json:"sourceTasks"`
	Hint        string  `json:"hint,omitempty"`
}

// CostReport cuantifica la diferencia de tokens y el costo estimado/real en USD de usar
// Dioptra frente a imprimir el mismo Markdown directamente en la consola del agente.
type CostReport struct {
	Mode                 string  `json:"mode"`
	Model                string  `json:"model"`
	PricingLabel         string  `json:"pricingLabel"`
	PricingSource        string  `json:"pricingSource,omitempty"` // verified | env | reference
	IsExact              bool    `json:"isExact"`
	BaselineOutputTokens int     `json:"baselineOutputTokens"`
	BaselineUSD          float64 `json:"baselineUsd"`
	DioptraInputTokens   int     `json:"dioptraInputTokens"`
	DioptraOutputTokens  int     `json:"dioptraOutputTokens"`
	DioptraUSD           float64 `json:"dioptraUsd"`
	ExtraOutputTokens    int     `json:"extraOutputTokens"`
	DeltaPct             float64 `json:"deltaPct"`
	DeltaUSD             float64 `json:"deltaUsd"`
	ContractNote         string  `json:"contractNote,omitempty"`
	ConsoleLine          string  `json:"consoleLine"`
}

// DegradedMeta identifica la procedencia de un bloque degradado (§2 ADR 1).
type DegradedMeta struct {
	From   string `json:"from"`
	Reason string `json:"reason"`
}

// CodeSpan representa un fragmento sintáctico coloreado por Chroma en servidor.
type CodeSpan struct {
	Text  string `json:"t"`
	Class string `json:"c,omitempty"` // syn-keyword | syn-string | syn-number | syn-function | syn-type | syn-comment
}

// DiffHunkLine representa una línea precalculada de diff unificado en servidor.
type DiffHunkLine struct {
	Kind   string     `json:"kind"` // "ctx" | "add" | "del"
	LineNo int        `json:"ln"`
	Text   string     `json:"text"`
	Tokens []CodeSpan `json:"tokens,omitempty"`
}

// Envelope es el sobre JSON canónico servido al visor (<dx-report>).
type Envelope struct {
	V        int       `json:"v"`
	Warnings []Warning `json:"warnings,omitempty"`
	Report   Report    `json:"report"`
}

// Report contiene los metadatos y bloques validados del documento.
type Report struct {
	ID         string           `json:"id,omitempty"`
	Project    string           `json:"project,omitempty"`
	Title      string           `json:"title"`
	TS         string           `json:"ts"`
	UpdatedAt  string           `json:"updatedAt,omitempty"`
	Revision   int              `json:"revision,omitempty"`
	LastChange string           `json:"lastChange,omitempty"`
	MaxSev     string           `json:"maxSev,omitempty"`
	Excerpt    string           `json:"excerpt,omitempty"`
	Blocks     []map[string]any `json:"blocks"`
}

// ReportSummary alimenta la barra lateral cronológica (<dx-timeline>).
type ReportSummary struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Project       string `json:"project"`
	Tool          string `json:"tool,omitempty"`  // herramienta/CLI emisora (Genius Shell, Agy, ClaudeCode, Codex)
	Model         string `json:"model,omitempty"` // modelo usado para el trabajo publicado
	Title         string `json:"title"`
	TS            string `json:"ts"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
	Revision      int    `json:"revision,omitempty"`
	LastChange    string `json:"lastChange,omitempty"`
	MaxSev        string `json:"maxSev"`
	Excerpt       string `json:"excerpt"`
	MarkdownPath  string `json:"markdownPath"`
	WarningsCount int    `json:"warningsCount"`
}

// ChecklistItemState expone el estado actual de una casilla para get_report_state.
type ChecklistItemState struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Done  bool   `json:"done"`
	Owner string `json:"owner,omitempty"`
	Sev   string `json:"sev,omitempty"`
}

// CloneEnvelope realiza una copia profunda vía JSON para transformar bloques sin mutar caché.
func CloneEnvelope(env *Envelope) (*Envelope, error) {
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	var out Envelope
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
