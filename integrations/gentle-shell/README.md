# Integrating DioptraEngine with Gentle Shell (`gentle-shell` / Pi)

> **Clean first run:** If `~/.gentle-shell` has not been initialized yet, launch `gentle-shell` once before registering MCP servers so its first-run bootstrap creates `settings.json` cleanly.

## 1. Register Dioptra as an MCP server (one-time)

Make sure `dioptra` (or `dioptra.exe` on Windows) is in your `PATH`, then run:

```bash
# Registers Dioptra over stdio (automatically starts `dioptra serve` if stopped)
gentle-shell mcp add dioptra -- dioptra mcp
```

*(If you run `gentle-shell` linked to an existing Pi home, add `--link` before `mcp`: `gentle-shell --link mcp add dioptra -- dioptra mcp`.)*

## 2. Optional automatic rule for ODD Step 7 (`Close`)

Copy the self-contained extension `dioptra-goal-close.ts` into your active extensions folder and restart Gentle Shell:

```bash
# macOS / Linux (isolated mode: ~/.gentle-shell/agent):
mkdir -p ~/.gentle-shell/agent/extensions
cp ./integrations/gentle-shell/dioptra-goal-close.ts ~/.gentle-shell/agent/extensions/dioptra-goal-close.ts
```

```powershell
# Windows (PowerShell):
New-Item -ItemType Directory -Force "$env:USERPROFILE\.gentle-shell\agent\extensions" | Out-Null
Copy-Item ".\integrations\gentle-shell\dioptra-goal-close.ts" "$env:USERPROFILE\.gentle-shell\agent\extensions\dioptra-goal-close.ts"
```

The extension:
- Skips worker subagents (`GENTLE_PI_AGENTS_CHILD === "1"`).
- Activates only when `dioptra` is registered in the active MCP tools.
- Instructs the agent to publish only at task close (`Close`), with zero extra credentials.
