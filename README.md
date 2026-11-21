# DioptraEngine (`dioptra`)

> *Tired of reading raw Markdown walls, ehh 😏? A fast single-binary Go MCP viewer that turns AI agent outputs into slick local UI reports & clickable approvals.*

**A fast, local, single-binary technical report viewer and MCP server for AI coding agents — written in pure Go, with zero API keys and near-zero token overhead.**

AI coding agents (Claude Code, Cursor, Codex, OpenCode, Antigravity, Gentle Shell, etc.) generate rich audits, architecture plans, code reviews, diffs, and postmortems that get buried in terminal scrollback. **Dioptra** moves those long deliverables out of the terminal into an interactive local web workspace (`http://127.0.0.1:7474`) backed by SQLite FTS5, live SSE updates, bidirectional human-in-the-loop checklists, a multi-role agent board, and an E2E test observer.

---

## Why Dioptra?

- **Deterministic Markdown → Rich UI (Zero LLM calls by default):** Plain GitHub Markdown converts in milliseconds (`fast_goldmark`, ~1% extra token overhead) into sortable tables, KPI cards, severity callouts, syntax-highlighted code/diffs (Chroma), D2/Mermaid SVG diagrams, collapsible sections, and interactive checklists. **No API keys or external layout LLM required.**
- **Human-in-the-loop approvals:** Check boxes right in your browser (`PATCH /api/reports/{id}/check`). Your agent reads exact human approvals via `get_report_state` before executing risky changes.
- **Single Go binary (`CGO_ENABLED=0`), zero external runtime:** Embedded Shadow DOM Web Components (`ui/embed.go`), pure-Go SQLite (`modernc.org/sqlite`) with WAL + FTS5/BM25 search, self-hosted OFL fonts, zero external CDNs.
- **Multi-role Agent Board + Anti-hallucination Moderator:** Coordinate `dev`, `tester`, `po`, and `moderador` agents through `board_list` / `board_task` with manual user approval gates, evidence validation (`file:line`, commands, links), and live team chat logs.
- **E2E Observer (`e2e_start` / `e2e_record` / `e2e_ingest_har` / `e2e_finish`):** Document manual end-to-end test runs directly from browser HAR files and service logs with automatic server-side PII/secret redaction.
- **Idempotent in-place updates & cost transparency:** Republishing with the same `id`, `idempotency_key`, or `(project, title)` updates the existing report in place via SSE with a change note — never duplicates. `dioptra stats` tracks exact token/USD delta.
- **Local-first security:** Loopback-only (`127.0.0.1`), Host allowlist, same-origin + `Sec-Fetch-Site` + `Content-Type` guards on writes/MCP (CSRF & DNS-rebinding defense), strict CSP, and isolated iframe CSP for interactive `free-canvas` blocks.

---

## Quick Start (2 Minutes)

### 1. Install or Build from Source

Download the latest single binary for Windows, macOS, or Linux from **Releases**, or build locally with Go (`CGO_ENABLED=0`):

```bash
git clone https://github.com/JbryanH187/Dioptra.git
cd Dioptra
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dioptra ./cmd/dioptra
```

Verify health:

```bash
./dioptra doctor
```

### 2. Connect to Any MCP Client

`dioptra mcp` runs over standard **stdio** and automatically starts the background local viewer daemon (`http://127.0.0.1:7474`) when needed.

#### Gentle Shell / `gentle-ai`
```bash
gentle-shell mcp add dioptra -- dioptra mcp
```
*(Optional ODD Step 7 auto-close extension and skills included under [`integrations/`](./integrations/).)*

#### Claude Code
```bash
claude mcp add dioptra -- dioptra mcp
```

#### Cursor / VS Code / Antigravity / Generic `mcp.json`
```json
{
  "mcpServers": {
    "dioptra": {
      "command": "dioptra",
      "args": ["mcp"]
    }
  }
}
```

#### Streamable HTTP MCP (when running `dioptra serve` or `dioptra start`)
Point any HTTP MCP client to `http://127.0.0.1:7474/mcp`.

---

## Markdown Conventions (Zero JSON Needed)

Send regular GitHub Markdown to `publish_report` (or run `dioptra open report.md --project my-app`). Dioptra deterministically upgrades it into interactive UI blocks:

| Markdown Syntax | Rendered Dioptra Block |
|---|---|
| `## Heading` | Collapsible/navigable section (`<dx-collapsible-section>`) |
| Markdown table | Sortable, filterable table (`<dx-data-table>`) with auto-typed numbers, severities & `file:line` refs |
| `` ```kpi `` (`Label \| value \| unit \| delta \| sev`) | KPI metric cards (`<dx-kpi-grid>`) |
| `> [!WARNING] Title` (`NOTE`, `TIP`, `SUCCESS`, `IMPORTANT`, `CAUTION`, `ERROR`) | Typed severity callout (`<dx-severity-callout>`) |
| `- [ ] Task description @owner [warning]` | Interactive approval checklist (`<dx-action-checklist>`) synced back to `get_report_state` |
| `` ```mermaid `` (`flowchart` / `sequenceDiagram`) | Native D2/SVG diagram (`<dx-diagram-block>`) |
| `` ```diff `` | Colorized unified diff with line numbers (`<dx-code-view>`) |
| `<details><summary>Title</summary> ... </details>` | Collapsed section |

> **Optional custom layouts:** For bespoke dashboards or interactive HTML/SVG/Canvas sandboxes, agents can call `get_layout_contract` and pass `blocks_override` (validated against the source Markdown by the Fidelity Guardian).

---

## MCP Tools & Resources

| Tool | Purpose |
|---|---|
| `publish_report` | Create or update a report in place (idempotent); returns `id`, `url`, and `cost.consoleLine` |
| `list_reports` | Search and filter reports by `project`, `tool`, `model`, or FTS5 `query` |
| `get_report_state` | Read current Markdown and which checklist items the human checked (`done: true`) |
| `dioptra_context` | Lightweight session-start summary of recent reports for a project |
| `get_layout_contract` | Compact schema (`dioptra-synth/1.1`) for optional `blocks_override` / `free-canvas` |
| `board_list` / `board_task` | Multi-role agent task board (`dev`, `tester`, `po`, `moderador`) with anti-hallucination audit |
| `e2e_start` / `e2e_record` / `e2e_ingest_har` / `e2e_finish` | Live E2E test observation & sanitized HAR ingestion |

Resource: `dioptra://contract/layout`

---

## CLI Commands

```bash
dioptra serve [--port 7474] [--dir ~/.dioptra/reports]   # Run foreground local viewer + /mcp
dioptra start [--port 7474] [--dir ~/.dioptra/reports]   # Start detached background daemon
dioptra mcp   [--port 7474] [--dir ~/.dioptra/reports]   # Run stdio MCP bridge (auto-starts daemon)
dioptra open  [file.md] [--project name] [--no-browser]  # Import/open a Markdown report
dioptra autostart on|off|status                          # Login autostart (Windows)
dioptra doctor [--probe]                                 # Check SQLite WAL/FTS5, MCP contract & daemon
dioptra stats                                            # Cumulative token & USD overhead metrics
dioptra db check|backup|reindex|alias ...                # SQLite maintenance & project aliases
dioptra roots list|set <project> <path>|remove <project> # Authorized source roots for code viewer
```

---

## Bundled Agent Skills & Integrations

- [`integrations/skills/dioptra-report-writer/SKILL.md`](./integrations/skills/dioptra-report-writer/SKILL.md) — Concise publishing conventions and examples.
- [`integrations/skills/trabajo-en-equipo/SKILL.md`](./integrations/skills/trabajo-en-equipo/SKILL.md) — Multi-role `dev` / `tester` / `po` / `moderador` coordination protocol.
- [`integrations/skills/observador-e2e/SKILL.md`](./integrations/skills/observador-e2e/SKILL.md) — Manual E2E test observation & log-quality audit rubric.
- [`integrations/gentle-shell/`](./integrations/gentle-shell/) — Ready-to-drop extension (`dioptra-goal-close.ts`) for Gentle Shell / Pi.

---

## Development & Verification

```bash
go vet ./...
go test -race ./...
node --experimental-vm-modules scripts/check-ui.mjs ui/components/*.js ui/app.js ui/theme-init.js
```

On Windows PowerShell:

```powershell
.\scripts\check.ps1 -Version v1.1.0
```

---

## License

[MIT](./LICENSE) © 2026 Bryan Higuera
