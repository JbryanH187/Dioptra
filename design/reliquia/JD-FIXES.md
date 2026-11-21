# Judgment Day — correcciones confirmadas (Juez A + Juez B)

## Grupo L — Lógica JS (dx-app.js, dx-timeline.js, dx-report.js)
- L1 (A#1) `dx-app.js`: listener `matchMedia(DRAWER_QUERY)` `change`: al pasar a escritorio `_setDrawerOpen(false)`; al cruzar en cualquier sentido reaplicar estado colapsado (L2). Remover en `disconnectedCallback`.
- L2 (A#2) En modo drawer llamar `timeline.setCollapsed(false)` (el rail no aplica < 1024px); en escritorio, el valor persistido.
- L3 (A#3) `dx-timeline.js`: tras selección desde el rail, devolver el foco al `[data-report-id]` correspondiente dentro de `_railListEl` (no a la opción oculta de la lista).
- L4 (A#4) Seleccionar reporte en modo drawer cierra el drawer.
- L5 (A#5) `dx-select-report` renderiza dos veces (dx-app caché + app.js fetch). Mantener un solo render: el de app.js (fuente fresca); dx-app solo renderiza desde caché si no hay manejador externo (documentar cómo se decide, sin romper preview).
- L6 (A#6) Sello de severidad: calcular sobre bloques **crudos** (`sev` ausente = sin severidad) para no mostrar "Informativo" por el default de normalización; unificar `extractMaxSeverity` (dx-app) y `computeMaxSeverity` (dx-report) en un helper; sin severidad explícita → sin sello.
- L7 (A#7) Guardar handler `keydown` de document y quitarlo en `disconnectedCallback` de DxApp.
- L8 (B#12) Quitar el toast "Referencia invocada:" (dx-app.js ~434); conservar la acción de resaltar/navegar.
- L9 (B#14) Quitar `.dx-report-eyebrow` en dx-report.js (~259): la placa `.dx-report-plate` ya muestra proyecto, fecha y revisión; mover fecha/rev a la placa si no están.

## Grupo S — shell.css + tokens.css + free-canvas.js
- S1 (B#1 CRÍTICO) z-index: drawer `var(--z-overlay)`, scrim `calc(var(--z-overlay) - 1)`; sin números fijos.
- S2 (B#2) `.dx-chip` y `.dx-rail-item`: chaflán a `::before` (patrón de `.dx-collapse-btn`), sin `clip-path` en el elemento; foco visible.
- S3 (B#4) `.dx-codex-title` → `var(--color-gold-text)`.
- S4 (B#5) Nuevo token `--color-bronze-text` (oscuro `#E5B25C`, claro `#6B4512`; verificar ≥4.5:1 sobre base y raised y anotar ratio) en ambos bloques de tokens.css y en free-canvas; sustituir `--color-bronze-light`/`--color-gold` usados como TEXTO en shell.css (~1221, 1313, 1392, 1468).
- S5 (B#6) `--backdrop-blur-surface: none` en ambos temas; eliminar `backdrop-filter` de shell.css (~630, ~898).
- S6 (B#7) Eliminar halo `dx-report::before` (~600-611).
- S7 (B#11) Restos slate/navy en tokens.css (23-24, 30, 69, 202, 240) → piedra/tinta; `--gradient-sheen: none` ambos temas. Sincronizar free-canvas (y quitar backdrop/sheen azul si hay).
- S8 (B#10) Pastillas `--rounded-full` en shell.css (223, 751, 796, 814) → `--rounded-sm` (los sev-dot pueden seguir redondos).
- S9 (B#15) `.dx-brand-mark` sin degradado/brillo v2; quitar barra de vida del toast; unificar bloques `forced-colors` duplicados; borrar comentarios "v2 Instrumento Óptico"/"Blur"; añadir forced-colors para `.dx-drawer-open` (filter:none, ::before clip-path:none + border CanvasText).
- S10 Quitar estilos de `.dx-report-eyebrow` (L9).
- S11 `--lh-relaxed: 1.7` como token nuevo en ambos bloques (prose-block lo usa).

## Grupo C — CSS de componentes
- C1 (B#3) `collapsible-section.js`: quitar `:host{overflow:hidden}` o usar foco inset; foco visible.
- C2 (B#5) `diagram-block.js:259` texto oro → `var(--color-gold-text)`; cualquier `--color-bronze-light` como texto en componentes → `var(--color-bronze-text, var(--color-accent-fg))`.
- C3 (B#6) `diagram-block.js:45-46` quitar `backdrop-filter`.
- C4 (B#8) Doble marco: en `shared.js` añadir `:host([data-flat])::before{display:none}` y `:host([data-flat]){filter:none}`; los componentes cuyo hijo ya es la losa (data-table, code-view, severity-callout, kpi-grid, collapsible-section) ponen `this.setAttribute('data-flat','')` en `connectedCallback` **o** (preferible, sin JS) anulan en su propia hoja `:host::before{display:none} :host{filter:none}`. Usar la vía CSS. Ítems del checklist: separador 1px en lugar de contorno+sombra por ítem.
- C5 (B#9) Borrar CSS muerto de action-checklist.js (`.check`, `.check::after`, `@keyframes check-pop` y sus reglas reduced-motion/forced-colors).
- C6 (B#10) Pastillas `--rounded-full` en diagram-block.js 43/69, data-table.js 292, action-checklist.js 53/63/270, collapsible-section.js 126 → `--rounded-sm`.
