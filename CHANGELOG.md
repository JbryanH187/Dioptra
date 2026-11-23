# Changelog — DioptraEngine

Formato: una sección por versión publicada (`git tag vX.Y.Z`; el binario se compila con
`./scripts/check.ps1 -Version vX.Y.Z`). Lo más reciente arriba.

## v1.2.0 — 2026-11-23

### Workspaces jerárquicos y Sync interactivo de duplicados
- **Jerarquía `Workspace → Sub-equipos`:** tabla `workspace_groups` (SQLite v6) para anidar sub-equipos (p. ej. `tsugi-backend`, `tsugi-mobile`, `tsugi-db`, `tsugi-ux`) dentro de un workspace padre plegable manteniendo cada tablero independiente.
- **Sync de duplicados bajo demanda:** herramientas MCP `workspace_audit` / `workspace_organize` y CLI `dioptra db audit` / `dioptra db group` que detectan duplicados ortográficos (`SnapProof` ↔ `snapshooter`, `genius-shell` ↔ `GeniusShell`) y sub-equipos, recordando pares marcados como independientes (`workspace_audit_ignores`).
- **Barra lateral unificada:** `<dx-timeline>` resuelve aliases y agrupa sub-equipos en vivo vía `/api/projects` + SSE `workspaces_updated`.

### Sesiones de Genius Shell ubicables (`<dx-sessions>`)
- Cada sesión abierta de Genius Shell ahora es clickeable para filtrar y abrir su workspace en Dioptra, resuelve aliases / `roots.json` / workspace padre y recupera el título real de la sesión cuando está en espera (`idle`).

### Diagramas siempre renderizados
- Parser Mermaid→D2 ampliado: operador `&` (`A --> B & C`), subgrafos anidados, `direction`, `stateDiagram-v2`, `erDiagram`, `classDiagram`, `Note`/`alt`/`loop` en `sequenceDiagram` y cajas ASCII en fences de texto.
- Fallback local *lazy* de Mermaid JS (`/vendor/mermaid.min.js`, 0 KB en carga inicial, CSP `script-src 'self'`, estilos adoptados vía `CSSStyleSheet`) para el resto de diagramas (`gantt`, `pie`, `mindmap`, etc.): ningún diagrama Mermaid vuelve a quedar en texto plano.

### Lienzos multipantalla opt-in + Comentarios y Marcador (`T1`)
- Registro opt-in de lienzos locales (`design_canvases`, SQLite v7) servidos con aislamiento estricto (`DesignCanvasCSP`, `connect-src 'none'`, `EvalSymlinks` contención, `<iframe sandbox="allow-scripts">`).
- Visor modal **88–90% de pantalla** (`<dx-design-canvas>`) con modo **Lienzo** (zoom, pestañas de versión, notas) y modo **Prototipo interactivo** navegable.
- **Capa de anotaciones visuales (`canvas-annotator.js`):** herramientas **Pin**, **Caja** y **Marcador** en coordenadas normalizadas `0..1` + sonda DOM de solo lectura vía `postMessage` (`dxProbePoint`), panel lateral de hilos y herramientas MCP `canvas_register`, `canvas_list`, `canvas_get_feedback`, `canvas_resolve_feedback`.
- **Rol nativo `ux`** en el tablero multiagente (`dev`, `tester`, `po`, `ux`, `moderador`).

## v1.1.0 — 2026-11-21

### Tablero de equipo (Dev · Tester · PO)
- Tablero por proyecto con el rol de cada agente y la tarea que le toca.
- **Moderador contra alucinaciones entre agentes:**
  - revisa evidencias (comando, `archivo:línea`, enlace);
  - detecta decisiones en conflicto, rebotes y acaparamiento;
  - solo él usa «verificado» y «alerta».
- La conversación del equipo es visible: decisiones, preguntas abiertas y el reporte «Chat del equipo».
- **Tus aprobaciones:**
  - En modo manual (por defecto), una tarea aprobada por el PO espera tu casilla en el visor.
  - El modo automático solo lo activas tú.
  - Ningún agente puede firmar como el usuario.
- **Menos ruido del moderador:**
  - las preguntas de tareas ya cerradas no alertan;
  - «estancada» solo se marca si el resto del tablero siguió avanzando sin esa tarea.

### Observador E2E (v1)
- Documenta pruebas de punta a punta a partir de un HAR y publica su reporte en vivo.

### CLI
- `dioptra start`: deja el demonio corriendo en segundo plano y termina.
- `dioptra autostart on | off | status`: arranque al iniciar sesión en Windows, sin ventana y sin permisos de administrador. El `doctor` muestra su estado.

### Mantenimiento
- `store.go` partido por responsabilidad (F7).
