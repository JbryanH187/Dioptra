# RFC / Issue Proposal for `Gentleman-Programming/gentle-ai`

> **Note on `gentle-ai` workflow:** `Gentleman-Programming/gentle-ai` enforces an **Issue-First CI gate** (PRs must reference an issue labeled `status:approved`). Open this issue first, align on scope with the maintainers, and open the PR once approved.

---

## Suggested Issue Title

`feat(mcp): add optional DioptraEngine MCP server integration (local report viewer, human-in-the-loop checklists & team board)`

---

## Issue Body (Ready to Copy & Paste)

### Problem / Motivation

When working with ODD (Outcome-Driven Development), Judgment Day reviews, architecture specs, or E2E verification across the agents supported by `gentle-ai`, long deliverables (>40 lines, tables, diffs, Mermaid diagrams, and multi-item approval plans) get buried in terminal scrollback:

- Humans struggle to review dense plans/audits or approve specific items selectively in a terminal.
- Agents have no structured way to read back which checklist items the user approved before touching code.
- Adding LLM-powered renderers wastes tokens and requires extra API keys.

### Proposed Solution

Integrate **[DioptraEngine (`dioptra`)](https://github.com/JbryanH187/Dioptra)** as an **optional MCP component** in `gentle-ai` (similar in spirit to how `engram` integrates as a companion pure-Go single binary):

- **Pure Go single binary (`CGO_ENABLED=0`)**, MIT licensed, zero Node/CDN runtime, pure-Go SQLite (`WAL` + `FTS5/BM25`), cross-platform releases (Windows, macOS, Linux).
- **Zero-credential / deterministic Markdown rendering (`fast_goldmark`):** converts standard GitHub Markdown (`##` sections, tables, `kpi` fences, `mermaid` → D2 SVG, `diff` syntax highlighting, `[!WARNING]` callouts, `- [ ]` checklists) in milliseconds (~1% token delta, 0 LLM calls).
- **Bidirectional human-in-the-loop approvals:** Users check/uncheck items in the local viewer (`http://127.0.0.1:7474`), and the agent reads exact approvals via `get_report_state` before acting.
- **Idempotent updates + Live SSE:** Updating a report with the same `id` / `idempotency_key` updates it in place instead of creating duplicates.
- **Multi-role board & E2E observer (optional):** Built-in `board_list` / `board_task` (with anti-hallucination evidence audit) and `e2e_*` tools (sanitized HAR ingestion).
- **Local-first security:** Loopback-only (`127.0.0.1`), Host allowlist, Origin/`Sec-Fetch-Site`/`Content-Type` write guards, strict CSP.

### Proposed Scope for the Follow-up PR (after `status:approved`)

1. **Optional MCP entry (`dioptra`):** Register `dioptra mcp` (stdio) when the user selects Dioptra in `gentle-ai`.
2. **Optional companion skill:** Install `dioptra-report-writer` (instructs agents to publish deliverables >40 lines at ODD `Close` without dumping full walls of text to the terminal).
3. **`gentle-ai doctor` read-only check:** Verify `dioptra` binary availability / `dioptra doctor` health when enabled.
4. *(Optional, separate issue for `gentle-shell`)*: Include the lightweight `dioptra-goal-close.ts` Pi extension (skips worker subagents via `GENTLE_PI_AGENTS_CHILD === "1"` and activates only when `publish_report` is present).

### Verification Plan

- Unit tests in `gentle-ai` covering MCP config generation (idempotent merge, snapshot rollback) and `doctor` check.
- End-to-end manual test across at least 2 agents (e.g., Claude Code + Gentle Shell) verifying `publish_report` → browser checklist approval → `get_report_state`.

### Maintainer Commitment

I am the author of DioptraEngine and am happy to maintain the `dioptra` integration, keep cross-platform releases up to date via GoReleaser, and adjust scope to fit `gentle-ai`'s conventions.

---

## Checklist Before Opening the Issue on GitHub

1. Push this clean standalone repository to a public GitHub repo (`https://github.com/JbryanH187/Dioptra`).
2. Tag `v1.1.0` so `.github/workflows/release.yml` builds and publishes the Windows/macOS/Linux binaries + `checksums.txt`.
3. Verify `.github/workflows/ci.yml` passes green on `ubuntu-latest`, `macos-latest`, and `windows-latest` (`go test -race ./...`).
4. Record a 20-second GIF showing: agent calls `publish_report` → browser opens `http://127.0.0.1:7474` → user checks 2 items → agent reads them via `get_report_state`, and embed it in `README.md` + the GitHub Issue.
5. Open the Issue in `Gentleman-Programming/gentle-ai` using the body above and wait for `status:approved` before opening the PR.
