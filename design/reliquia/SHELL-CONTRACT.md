# Contrato compartido del Shell Reliquia (fuente de verdad entre workers)

Complementa `design/reliquia/SPEC.md` §2 y §4. Ningún worker inventa nombres fuera de esta tabla.

## Estado y atributos
| Elemento | Atributo | Valores | Dueño que lo escribe |
|---|---|---|---|
| `.dx-shell` (dx-app.js ~332) | `data-collapsed` | `"true"` / ausente | dx-app.js |
| `.dx-shell` | `data-drawer-open` | `"true"` / ausente (solo < 1024px) | dx-app.js |
| `dx-timeline` | `id="dx-sidebar"` | fijo | dx-timeline.js |

## Eventos (CustomEvent, `bubbles: true, composed: true`)
| Evento | Emite | Escucha | detail |
|---|---|---|---|
| `dx-toggle-sidebar` | botón colapsar en dx-timeline.js | dx-app.js | `{}` |
| `dx-sidebar-collapsed-changed` | **no se usa evento**: dx-app.js llama directamente `timeline.setCollapsed(bool)` | — | — |

## Métodos públicos de `dx-timeline` (dx-timeline.js)
- `setCollapsed(bool)`: alterna la vista rail; `false` → `sigilFull.wake()`, `true` → `sigilFull.sleep()`; actualiza `aria-expanded` del botón.
- `celebrate()`: `sigilFull.celebrate()` si existe.
Todas las llamadas al sigilo se protegen con `typeof s.m === 'function'`.

## DOM nuevo en la sidebar (clases que estiliza shell.css)
```
dx-timeline#dx-sidebar
  .dx-sidebar-brand                 (existente)
    .dx-codex-head                  (nuevo) cabecera del códice
      .dx-codex-title               "DIOPTRA" (texto real)
      dx-sigil.dx-codex-sigil[size=full]
    .dx-brand-identity              (existente)
      .dx-brand-mark > dx-sigil[size=mark][decorative]
      h2.dx-brand-title             (existente)
      .dx-brand-badge               (existente)
    button.dx-collapse-btn[aria-expanded][aria-controls=dx-sidebar][aria-label="Contraer panel"|"Expandir panel"]
  .dx-rail                          (nuevo) visible solo con data-collapsed
    button.dx-rail-wake[aria-label="Expandir panel"] > dx-sigil[size=mark][state=asleep]
    nav.dx-rail-list[aria-label="Reportes recientes"]
      button.dx-rail-item[data-report-id][aria-label=<título>][aria-current="true" si activo]  texto I…X
```
`dx-app.js` añade `button.dx-scrim[aria-label="Cerrar panel"]` hermano de la sidebar dentro de `.dx-shell` (visible solo con `data-drawer-open` en < 1024px). Esc cierra el drawer.

## Cabecera del reporte (dx-report.js)
```
.dx-report-plate        placa: [modelo si existe] + svg.dx-plate-gem (rombo) + proyecto
h1.dx-report-title      (existente) + clase dx-display
.dx-report-seal[data-sev]  sello de severidad máxima (report.maxSev), con texto
.dx-report-progress     contenedor; dentro <progress class="dx-progress" max=N value=M> + span.dx-progress-label "M/N"
```
Solo se renderiza `.dx-report-progress` si hay ≥1 ítem de checklist (recorrer bloques `checklist` incluyendo anidados en `section`). Modelo: solo si el objeto reporte/summary lo trae; nunca inventarlo.

## Persistencia
`localStorage['dx-sidebar-collapsed'] = '1' | '0'` (dx-app.js, try/catch como STORAGE_SIDEBAR_KEY).

## Tokens a usar (con fallback)
`var(--color-ink,#06080C)`, `var(--color-ink-shadow,#040609)`, `var(--shadow-ink-sm|md|lg, …)`, `var(--clip-chamfer-sm|md, polygon(…))`, `var(--font-display, Georgia, serif)`, `var(--color-copper|copper-strong|copper-deep)`, `var(--color-bronze|bronze-light)`, `var(--color-gold|gold-glow)`, `var(--color-stone-abyss|obsidian|slab|steel)`, `var(--bevel-stone)`, `var(--bevel-copper)`.
