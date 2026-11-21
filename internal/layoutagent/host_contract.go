package layoutagent

import "dioptra/internal/contract"

// Contrato para agentes ANFITRIONES (Claude Code, Genius Shell, Codex…). Está en
// inglés y es deliberadamente corto (~1.5k tokens): el anfitrión ya tiene el
// Markdown y solo necesita las reglas para construir blocks_override. El prompt
// largo del Sintetizador interno (SystemPrompt) sigue existiendo para el
// adaptador HTTP del demonio, pero ya no se envía a los anfitriones.

// HostUsage explica cuándo conviene blocks_override y cómo se audita.
const HostUsage = `blocks_override is OPTIONAL. Plain Markdown already renders as rich blocks (sections, typed tables, callouts, checklists, mermaid diagrams, kpi fences, diffs, collapsible <details>). Use blocks_override only for layouts Markdown cannot express (custom KPI selection, typed table columns, a free-canvas simulation).
How: publish_report { title, project, tool, model, markdown: <full original Markdown>, blocks_override: [ ...blocks... ] }. The markdown stays the source of truth on disk; the server validates the blocks (unknown keys ignored, invalid blocks degraded) and audits them against the markdown. Read "fidelity" (verdict, coverage, hint) and "warnings" in the response and fix them in your next publish.`

// HostRules son las reglas mínimas para construir blocks fieles y válidos.
const HostRules = `Fidelity (server-audited; violations are repaired, or the whole override is discarded and the markdown is published as-is):
F1 Code and diffs verbatim, never abbreviated ("...", "rest of code"). Long code goes inside section {open:false}, complete.
F2 Numbers literal from the markdown (value, unit, sign, precision). KPIs whose value is not in the markdown are removed.
F3 Keep every path:line reference (in "ref", md text or a ref column).
F4 Each "- [ ]"/"- [x]" becomes exactly one checklist item, in document order, id "chk-N" (N = ordinal of that task in the whole document), done=true only for [x], text literal without the marker. Never merge, split or reorder.
F5 Coverage: every paragraph, table, list and section must appear in some block. Fold, never drop. Under 45% word coverage the override is discarded.
F6 Keep the source language and terminology.

Limits: kpi 1-12 items. section nesting <= 3, blocks never empty. table: unique keys [a-z0-9_], every row uses exactly those keys; "num" columns hold pure numbers (unit goes in the label), "sev" columns only S values, "ref" columns only path:line; "filter": true from 6 rows. code: real lang; diffs as lang "diff" with the unified diff in src (or "old" + "src"). diagram: flow <= 80 nodes; seq <= 12 participants and <= 40 messages in time order; tree <= 60 nodes, single root, no cycles; node ids [A-Za-z0-9_-]; edges [from,to,label?] reference existing ids. canvas: max 1 per report, html <= 200 KB, h 120-1200.

Order: kpi (3+ key figures) -> critical callouts (error > warning > info > success) -> body in source order -> folded appendices (logs, traces, long code: section open:false).
Block choice: comparisons with 2+ attributes -> table; risks/blockers/approvals -> callout with ref; flows/architecture -> diagram flow; request/response between actors -> seq; strict hierarchies -> tree; actions -> checklist; narrative -> prose.
md syntax (prose md, callout body): **bold** *italic* ~~strike~~ ` + "`code`" + ` [text](https://url) path:line, lists "- " or "1." nested with 2 spaces, "> " quotes, ` + "```" + ` fences, "###" subheadings. No raw HTML.

free-canvas: one self-contained fragment (<style> + markup + inline <script>). Sandbox CSP default-src 'none': no external scripts, fonts, images, fetch, WebSocket or storage. Colors, spacing, radii and type ONLY via var(--token) (call get_layout_contract with include_canvas_tokens:true for the list); no literal colors. Canvas/JS-drawn SVG reads colors with getComputedStyle and redraws on window event "dx-canvas-theme". Animate only transform/opacity, respect prefers-reduced-motion, label every control, keep the original figures also in kpi/table/prose.`

// BuildHostContract devuelve el contrato compacto para anfitriones. Los tokens
// CSS (~2.3k caracteres) solo se incluyen cuando se van a diseñar free-canvas.
func BuildHostContract(includeCanvasTokens bool) LayoutContract {
	c := LayoutContract{
		Version: PromptVersion,
		Usage:   HostUsage,
		Rules:   HostRules,
		Schema:  contract.CompactSchemaB,
	}
	if includeCanvasTokens {
		c.CSSTokens = CanvasTokens()
	}
	return c
}
