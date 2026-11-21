# Contributing to DioptraEngine

Thank you for your interest in contributing to **DioptraEngine**!

## Principles

1. **Single binary, zero runtime dependencies:** Keep the Go build `CGO_ENABLED=0` and the frontend 100% native Web Components with zero external CDNs or Node build steps.
2. **Markdown-first, zero-credential default:** Plain GitHub Markdown must always render deterministically and cleanly without requiring API keys or an external LLM.
3. **Local-first security:** Every write endpoint and MCP route must remain loopback-only (`127.0.0.1`) with strict Host, Origin, and CSP checks.
4. **No silent content loss:** Changes to Markdown conversion (`internal/render`) must keep the golden corpus tests passing (`TestCorpus_NoSilentContentLoss`).

## Workflow

1. **Open an issue first** for new features or architectural changes so we can align on scope before writing code.
2. **Create a focused branch** (`feat/<topic>` or `fix/<topic>`).
3. **Run the full verification suite** before opening a PR:

```bash
gofmt -l cmd internal ui
go vet ./...
go test -race ./...
node --experimental-vm-modules scripts/check-ui.mjs ui/components/*.js ui/app.js ui/theme-init.js
```

On Windows PowerShell:

```powershell
.\scripts\check.ps1
```

## AI-Assisted Contributions

AI coding assistants are welcome, provided you:
- Review and understand every line you submit.
- Include real tests verifying the invariant or fix.
- Keep diffs scoped strictly to the issue with zero unrelated edits or placeholder code.
