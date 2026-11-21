# DioptraEngine Viewer — Rediseño v2 "Instrumento Óptico"

Spec-by-Reference para todos los workers. Solo UI/UX/Motion. **Prohibido** cambiar contratos de datos (Contrato B), APIs públicas de los componentes, nombres de eventos o atributos que consume `preview.js`.

## 1. Dirección de arte
Una dioptra es un instrumento óptico de precisión, y la UI tiene que sentirse como uno: **precisa, calmada y de lectura técnica densa**.
- Superficies escalonadas por luminancia (sunken < body < base < raised). La jerarquía la marcan la luz y el borde, no las sombras pesadas.
- Un único acento luminoso, azul técnico (`--color-accent-fg`), que se usa para foco, selección e "ingeniería viva".
- Bordes de 1px con "hairline highlight" superior (`--shadow-inset`) para dar sensación de vidrio mecanizado.
- Las severidades son señales, no decoración: tinte suave + borde/acento + icono, y siempre texto. Nunca solo color.
- Los datos (números, tiempos, rutas) van en mono tabular.

## 2. Restricciones selladas (NO romper)
- CSP `script-src 'self'; style-src 'self'`: cero `style=""`, cero `<style>` inline, cero `innerHTML`. Estilos solo en `ui/styles/*.css` o en `createSheet()` (constructable stylesheets). Variables dinámicas mediante `CSSStyleSheet.replaceSync` adoptada (patrón `_sidebarSheet` de dx-app) o `el.style.setProperty` (CSSOM, permitido por CSP).
- Sin librerías ni fuentes externas. Sin assets nuevos de red.
- Contraste WCAG AA mínimo (4.5:1 texto, 3:1 UI/iconos). No bajar los `*-fg` auditados.
- Toda animación respeta `prefers-reduced-motion: reduce`: en ese modo los tokens de duración valen 0ms y las animaciones decorativas (`animation`) se desactivan.
- Animar solo `transform`, `opacity`, `clip-path`, `filter`, `background-color`, `border-color`, `box-shadow` y `color`. Nunca `width`/`height`/`top`/`left` (excepto `grid-template-rows 0fr→1fr` para plegar contenido).
- Mantener presupuesto ≤ 60 KB gzip en total.

## 3. Tokens nuevos v2 (definidos en ui/styles/tokens.css, disponibles en Shadow DOM)
Superficies: `--color-surface-sunken` (fondos de código y de inputs), `--color-surface-raised` (hover de tarjeta, headers de tabla, toolbars), `--color-surface-hover`, `--color-border-strong` (hover de borde), `--color-accent-soft` (tinte de acento al 12%), `--color-accent-ring` (borde de acento al 40%).
Gradientes/luz: `--gradient-accent` (marca/progreso), `--gradient-sheen` (brillo superior sutil en tarjetas), `--glow-accent` (box-shadow de halo para foco/selección activa).
Severidad: `--color-sev-{info|success|warning|error}-border` (borde tintado al ~32%).
Radios: `--rounded-xs: 4px`, `--rounded-lg: 20px` (los existentes `sm 8 / md 16 / full` se mantienen; tarjetas usan `--rounded-md`).
Tipografía: `--fs-micro: 11px` (solo kbd/badges en mayúsculas con tracking), `--ls-tight: -0.015em` (títulos), `--ls-caps: 0.06em` (eyebrows en mayúsculas).
Motion:
- Duraciones: `--motion-duration-instant: 80ms`, `--motion-duration-fast: 140ms`, `--motion-duration-normal: 220ms`, `--motion-duration-slow: 360ms`, `--motion-duration-deliberate: 560ms`.
- Easings: `--motion-easing-standard`, `--motion-easing-snappy` (salida enfática), `--motion-easing-decelerate` (entradas), `--motion-easing-accelerate` (salidas), `--motion-easing-emphasized` (overshoot suave con `linear()`, con fallback).
- Stagger: `--motion-stagger: 40ms`.
- Distancias: `--motion-distance-sm: 4px`, `--motion-distance-md: 8px`.
- Keyframes globales en `tokens.css` para Light DOM: `dx-fade-in`, `dx-rise-in`, `dx-scale-in`, `dx-shimmer`. **En Shadow DOM los keyframes del documento NO aplican**: cada componente que anime algo interno debe declarar sus propios `@keyframes` dentro de su `createSheet` (prefijo del componente, p. ej. `kpi-count-in`).

## 4. Patrones de interacción por estado (todos los componentes)
| Estado | Tratamiento |
|---|---|
| Reposo | `surface-base`, borde `--color-border`, `--shadow-inset, --shadow-default` |
| Hover (elementos interactivos) | fondo `--color-surface-hover` o `--color-row-hover-bg`, borde `--color-border-strong`, en `--motion-duration-fast` |
| Press | `transform: scale(0.97)` en `--motion-duration-instant` (solo botones y chips) |
| Foco | `--focus-ring` (no cambiar) |
| Seleccionado/activo | `--color-accent-soft` + borde `--color-accent-ring` o indicador de acento |
| Deshabilitado | `--color-text-disabled`, `cursor: not-allowed`, sin hover |
| Vacío | mensaje centrado en `text-soft` con icono de 16px, nunca un hueco en blanco |

Las tarjetas (`:host` en baseBlockSheet) no se elevan en hover (son contenido, no botones). Solo reciben un highlight de borde muy sutil vía `:host(:hover)` con `--color-border-strong`.

## 5. Coreografía
- Entrada de reporte: el Shell (shell.css) anima los hijos directos de `.dx-report-body` con `dx-rise-in` (opacity 0→1, translateY 8px→0), `--motion-duration-slow`, `--motion-easing-decelerate`, y stagger con `:nth-child` hasta 8 (máx. 320 ms acumulados). **Los componentes NO deben animar su propio `:host` al conectarse**, para no duplicar la animación.
- Micro-animaciones internas permitidas: barra de progreso del checklist (transform scaleX), check de checkbox (stroke-dashoffset o scale), chevron del acordeón (rotate), apertura del acordeón (`::details-content` / grid-rows), filas de tabla en hover, botones de toolbar (press), copiar código (feedback "Copiado" con fade), zoom del diagrama (transform con transición), número KPI (fade + rise corto, una sola vez).
- Cambio de tema: View Transition con revelado circular desde el botón (dx-app). Hay fallback instantáneo.
- Scroll del reporte: el header se compacta y aparece una barra de progreso de lectura (scroll-driven animations, mejora progresiva bajo `@supports (animation-timeline: scroll())`).

## 6. Criterios de aceptación
1. `node --check` pasa en todos los `.js` modificados.
2. Cero `innerHTML`, `style=` o `<style` nuevos (verificable con grep).
3. Las APIs públicas, `part=`, eventos `dx-*` y estructura de props no cambian.
4. Cada transición usa tokens `--motion-*` (sin ms hardcodeados, salvo dentro de `@keyframes`).
5. Hay bloque `@media (prefers-reduced-motion: reduce)` en cada sheet que declare `animation`.
6. Contraste: el texto usa solo tokens `--color-text-*`, `--color-*-fg` o `--color-on-primary`.
