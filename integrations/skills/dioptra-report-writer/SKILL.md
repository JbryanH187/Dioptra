---
name: dioptra-report-writer
description: Write technical reports that DioptraEngine renders as rich, navigable documents (sections, typed tables, callouts, approvable checklists, diagrams, KPIs). Use when publishing to Dioptra with publish_report, or when a deliverable is longer than ~40 lines (audits, plans, architecture, code reviews, postmortems, benchmarks).
---

# Writing Dioptra reports

Dioptra converts plain GitHub Markdown into interactive blocks. Write good Markdown; do not hand-build JSON unless the layout truly needs it.

## Shape of a great report

1. `# Title`: specific, in the user's language ("API de pagos: auditoría de seguridad", not "Reporte").
2. **Opening paragraph (2–3 sentences)**: the verdict first. What you found, how bad it is, what you recommend.
3. Optional `kpi` fence when there are 3+ key figures:
   ````
   ```kpi
   p95 | 140 | ms | -83% | success
   Errores | 0.2 | % | | info
   ```
   ````
   Every value must appear literally elsewhere in the report (the server removes invented figures).
4. `## Sections` in reading order. Each `##` becomes a collapsible section; use `###` inside for sub-topics.
5. Critical items as callouts, right after the opening:
   `> [!WARNING] Reembolso sin validar monto` followed by `> detail with internal/refunds/handler.go:57`.
   Tags: NOTE, TIP, IMPORTANT (warning), WARNING, CAUTION/ERROR (error).
6. Comparisons as tables. Put units in headers (`Latencia p95 (ms)`), plain numbers in cells, `file:line` in its own column. These become sortable typed columns.
7. Flows as mermaid (`flowchart LR` or `sequenceDiagram`). Use simple nodes `A[Label]`, edges `-->|label|`, `subgraph`. Unsupported syntax (`Note`, `loop`, `classDiagram`) is kept as code, never half-drawn.
8. Code and diffs complete in fences with the right language. Use `diff` for patches.
9. Long logs, traces and appendices inside `<details><summary>Title</summary> ... </details>`.
10. `## Próximos pasos` as a task list: `- [ ] Acción concreta @dueño [warning]`. The user approves items in the browser; call `get_report_state` before acting on them.

## Quality checklist (before publishing)

- [ ] The first paragraph answers "so what?" on its own.
- [ ] Every claim about code cites `path/file.ext:line` that you actually read.
- [ ] Figures are copied exactly (value, unit, sign); nothing estimated is presented as measured.
- [ ] Nothing important is only in the chat reply; the report stands alone.
- [ ] No filler sections ("Introducción", "Conclusión" restating the summary).
- [ ] Tasks are concrete, owned and verifiable.
- [ ] Same `project` value as previous reports of this repo; updates reuse `id` + `change_note`.

## After publishing

Reply in chat with 3–5 lines: verdict, the most important action, the `url`, and the literal `cost.consoleLine`. Do not paste the report into the chat.

## Examples

See `examples/` for three reference reports (audit, architecture with mermaid, postmortem with KPIs). They are part of the renderer's golden test corpus, so they always render correctly.
