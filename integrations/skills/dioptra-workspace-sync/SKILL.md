---
name: dioptra-workspace-sync
description: Auditar, leer y sincronizar proyectos, sub-equipos, alias y raíces locales (roots.json) en la base SQLite de Dioptra sin mover ni borrar reportes Markdown. Activar cuando el usuario pida sincronizar proyectos o workspaces en Dioptra, limpiar duplicados, agrupar sub-equipos o registrar raíces de código.
---

# Dioptra DB & Workspace Sync

Protocolo determinista y 100% aditivo para inspeccionar y sincronizar el índice de proyectos de Dioptra (`.index/dioptra.db` + `roots.json`) sin tocar ni renombrar archivos `.md` en disco.

## Reglas Inquebrantables

1. **Cero SQL crudo sobre `dioptra.db`:** usa únicamente las herramientas MCP (`workspace_audit`, `workspace_organize`) o los subcomandos oficiales (`dioptra db check|audit|alias|group` y `dioptra roots list|set|remove`).
2. **Nunca fusiones sub-equipos con `merge_alias`:** los tableros por capa/rol (`<proyecto>-producto`, `-db`, `-backend`, `-sec`, `-ux`, `-mobile`, `-qa`) deben mantenerse como proyectos propios agrupados con `group_subteam` bajo su `<padre>` para no mezclar sus tableros Kanban.
3. **Confirmación humana ante dudas:** antes de ejecutar `merge_alias` sobre nombres ambiguos o espacios históricos, presenta la tabla de candidatos al usuario y pregunta si desea fusionar (`merge_alias`), agrupar (`group_subteam`) o mantener separados (`ignore_pair`).

## Flujo Obligatorio en 4 Pasos

### Paso 1 · Leer estado e integridad (Solo lectura)
1. Verificar salud del índice: `dioptra db check` (`quick_check=ok`, `esquema v7`, paridad `reportes == filas de búsqueda`).
2. Auditar proyectos y relaciones: llamar MCP `workspace_audit` (o `dioptra db audit --json` + `GET /api/projects`).
3. Revisar raíces registradas: `dioptra roots list`.

### Paso 2 · Clasificar hallazgos con criterio
| Caso | Acción (`workspace_organize` / CLI) | Ejemplo |
|---|---|---|
| Mismo repo con distinta grafía | `merge_alias` (`dioptra db alias set <alias> <canónica>`) | `GeniusShell` → `genius-shell`, `snapshooter` → `snapproof` |
| Tablero de rol/capa de un producto | `group_subteam` (`dioptra db group set <subequipo> <padre> <rol>`) | `tsugi-db`, `tsugi-backend`, `tsugi-sec` → `tsugi` |
| Espacios distintos o handoffs históricos | `ignore_pair` (`workspace_organize {action:"ignore_pair", a, b}`) | `bryan-dev` o handoffs congelados |

### Paso 3 · Sincronizar raíces de código (`roots.json`)
Registra la ruta local real de cada workspace activo con `dioptra roots set <proyecto> <ruta-absoluta>`:
- Habilita inspección clickeable `file:line` (`/api/source`), enlace automático de sesiones en `<dx-sessions>` y detección de duplicados por ruta física.
- En monorepos con sub-equipos (`<padre>-<rol>`), registra también la ruta del monorepo en cada sub-equipo para que las referencias `file:line` dentro de sus tableros abran el código fuente.

### Paso 4 · Verificar y reportar
1. Re-ejecutar `workspace_audit` (o `dioptra db audit` + `dioptra db check`) tras aplicar cambios y raíces.
2. Entregar al usuario una tabla concisa con: **Workspaces raíz**, **Sub-equipos agrupados**, **Alias activos**, **Raíces `roots.json`** y cualquier par ambiguo pendiente de su decisión.
