package layoutagent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"dioptra/internal/contract"
	"dioptra/internal/render"
)

// LayoutContractURI es el URI del recurso MCP que expone el contrato de maquetación.
const LayoutContractURI = "dioptra://contract/layout"

// LayoutContract es el conocimiento visual que el modelo anfitrión necesita para maquetar.
type LayoutContract struct {
	Version   string   `json:"version"`
	Usage     string   `json:"usage"`
	Rules     string   `json:"rules"`
	Schema    string   `json:"schema"`
	CSSTokens []string `json:"css_tokens,omitempty"`
}

// BuildLayoutContract devuelve el contrato completo para anfitriones (con tokens
// CSS de free-canvas). Ver BuildHostContract para la variante sin tokens.
func BuildLayoutContract() LayoutContract {
	return BuildHostContract(true)
}

// LayoutContractMarkdown serializa el contrato compacto como Markdown para el
// recurso MCP (sin la lista de tokens; se pide aparte para free-canvas).
func LayoutContractMarkdown() string {
	c := BuildHostContract(false)
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Dioptra layout contract (%s)\n\n", c.Version)
	sb.WriteString("## Usage\n\n")
	sb.WriteString(c.Usage)
	sb.WriteString("\n\n## Rules\n\n")
	sb.WriteString(c.Rules)
	sb.WriteString("\n\n## Contract B compact schema\n\n")
	sb.WriteString(c.Schema)
	sb.WriteString("\n\nCSS tokens for free-canvas: call get_layout_contract with include_canvas_tokens=true.\n")
	return sb.String()
}

// GuardExternalEnvelope audita un sobre construido fuera de Dioptra (blocks_override de un
// agente anfitrión) contra su Markdown fuente y devuelve la métrica para autocorrección.
// Si la cobertura es insuficiente devuelve ErrLowFidelity y el llamador debe degradar.
func GuardExternalEnvelope(ctx context.Context, env *contract.Envelope, in Input) (FidelityReport, contract.FidelityMeta, error) {
	baseline, _, err := render.ConvertMarkdownToEnvelope(ctx, in.Title, in.Project, in.TS, in.Markdown)
	if err != nil {
		baseline = nil
	}
	sourceTasks := 0
	if baseline != nil {
		sourceTasks = len(contract.CollectChecklists(baseline.Report.Blocks))
	}

	rep, guardErr := ApplyFidelityGuard(env, baseline, in.Markdown)
	meta := contract.FidelityMeta{
		Coverage:    rep.Coverage,
		Repairs:     rep.Repairs,
		SourceTasks: sourceTasks,
	}
	switch {
	case errors.Is(guardErr, ErrLowFidelity):
		meta.Verdict = "rejected"
		meta.Hint = fmt.Sprintf("Coverage %.0f%% < %.0f%%: blocks_override dropped too much of the markdown, so the markdown was published as-is. Include all content (fold it in section open:false) and retry.", rep.Coverage*100, minCoverage*100)
		if len(rep.WeakSections) > 0 {
			meta.Hint += " Missing mostly in: " + strings.Join(rep.WeakSections, "; ") + "."
		}
	case guardErr != nil:
		meta.Verdict = "rejected"
		meta.Hint = guardErr.Error()
	case rep.Repairs > 0:
		meta.Verdict = "repaired"
		meta.Hint = "El Guardián corrigió la salida (ver warnings). En el próximo reporte: ids chk-N en orden de documento, código íntegro, rutas archivo:línea preservadas y solo cifras literales del markdown."
	default:
		meta.Verdict = "clean"
	}
	return rep, meta, guardErr
}
