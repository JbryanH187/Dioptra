# SPEC — Rediseño "Reliquia" de DioptraEngine

Fuente de dirección: HTML base entregado por el usuario (códice estilo Zelda: piedra abisal, cobre, bronce, oro, contorno de tinta cel-shaded, criatura-ojo con 5 tentáculos). Este SPEC **mejora** esa base; no la copia.

## 0. Restricciones duras (no negociables)
- CSP principal `default-src 'self'; script-src 'self'; style-src 'self'` (`internal/server/http.go:24`, verificada por `server_test.go:59`): **sin CDNs, sin Tailwind, sin Google Fonts, sin `onclick=` inline, sin atributos `style=` en markup**. Se permite CSSOM (`el.style.prop = …`), `setAttribute('transform', …)` en SVG y `adoptedStyleSheets`.
- **No renombrar ni borrar tokens** de `ui/styles/tokens.css`; cambiar valores y **añadir** nuevos dentro de `:root, [data-theme="dark"]` (el prompt del sintetizador los extrae de ese bloque; `internal/layoutagent/client_test.go:540-549,688`). Mantener `--color-surface-base`, `--color-text-primary`, `--color-sev-error-fg`, `--font-mono`.
- Conservar nombres/atributos/roles que usa el JS: `.dx-timeline-item[data-report-id]`, `[role=option]`, `.dx-splitter`, `.dx-theme-btn`, `.dx-toast[data-visible]`, `.dx-chip[aria-pressed]`, `.dx-sev-dot[data-sev]`, `.dx-brand-mark`, `.dx-brand-badge`, `.dx-brand-title`, `.dx-search-input`.
- `free-canvas.js:69-122` tiene copia de tokens para modo preview: sincronizarla con los nuevos valores.
- Ambos temas: `dark` = **Reliquia abisal** (principal); `light` = **Pergamino de piedra** (mismos nombres de token).
- `prefers-reduced-motion: reduce` desactiva toda animación no esencial (ojo centrado y quieto, sin parpadeo automático, sin respiración de tentáculos).
- Tests verdes: `go test ./... -count=1`.

## 1. Paleta (tokens nuevos, valores de referencia)
| Token nuevo | Oscuro | Claro | Uso |
|---|---|---|---|
| `--color-ink` | `#06080C` | `#1A1410` | Contorno cel-shaded de toda losa/botón/chip |
| `--color-ink-shadow` | `#040609` | `rgba(26,20,16,.55)` | Sombra dura desplazada |
| `--color-copper` / `-strong` / `-deep` | `#D96B27` / `#EE823A` / `#9E3F12` | `#B4531A` / `#C9621F` / `#7A2E0B` | Acento primario, botón primario, activo |
| `--color-bronze` / `-light` | `#C88A36` / `#E5B25C` | `#8E5E1E` / `#A8722A` | Ornamento, filetes, etiquetas de sección |
| `--color-gold` / `-glow` | `#F4C430` / `#FFF29C` | `#9A7400` / `#C79A10` | Iris, sellos encendidos, cifras KPI |
| `--color-stone-*` | abismo `#0B1017`, obsidiana `#121A24`, losa `#172230`, acero `#1E2C3D` | `#EDE7D5`, `#E4DEC9`, `#D8D0B8`, `#C9C0AA` | Planos |
| `--shadow-ink-sm` / `-md` / `-lg` | `2px 2px 0 var(--color-ink-shadow)` / `3px 4px 0 …` / `4px 5px 0 …` | idem | Elevación sin blur |
| `--bevel-stone` / `--bevel-copper` | insets del HTML base | equivalentes claros | Bisel tallado |
| `--clip-chamfer-sm` / `-md` | `polygon(…7px…)` / `polygon(…10px…)` | idem | Esquinas achaflanadas |
| `--font-display` | `"Cinzel","Trajan Pro","Cormorant SC","Palatino Linotype",Georgia,serif` | idem | Títulos/rótulos (solo si está instalada; sin descarga) |

Los tokens existentes se reasignan: `--color-surface-base/raised/elevated/sunken` → piedras; `--color-accent*` → cobre; `--color-border` → tinta; `--shadow-*` → sombras duras de tinta; `--rounded-*` → 2-3px (casi rectos, el chaflán da la forma); `--focus-ring` → `0 0 0 2px var(--color-ink), 0 0 0 4px var(--color-gold)`.
Severidades: error carmesí `#D94A38`/fg `#FF8A7A`, warning oro, success jade `#3BA676`/fg `#6FE3AE`, info reliquia azul-acero `#A8B3C2`. **Revalidar contraste** ≥ 4.5:1 de cada `*-fg` sobre su `*-bg` y sobre `--color-surface-base` en ambos temas.

## 2. Mejoras obligatorias sobre la base (pase Impeccable incorporado)
1. Callouts: **prohibida la franja lateral de color** (la base usa `inset 4px 0 0`). Usar fondo teñido + borde de tinta + sello romboidal de severidad + etiqueta textual.
2. Sin `animate-ping`, sin halos `blur-xl` decorativos, sin brillos permanentes. Un solo momento coreografiado: **despertar del ojo** al abrir el códice.
3. Sin toasts por acciones triviales (abrir/cerrar panel). Toasts solo para copiar, error o nuevo reporte recibido.
4. Sin kickers decorativos encima de títulos ni rótulos inventados ("Cartografía de Conductos de Cobre", "Letargo Abisal"): los textos de UI son funcionales y cortos. La temática vive en forma, material y motion, **no en copy rebuscado**.
5. Sin tarjetas dentro de tarjetas. Losas de un solo nivel; los bloques del reporte no llevan doble marco.
6. Sin emojis/caracteres como iconos (✕, ◆, ❖): SVG propios con trazo uniforme.
7. Texto seleccionable siempre (la base pone `select-none` en body).
8. Copiar con `navigator.clipboard.writeText` (fallback silencioso), nunca `execCommand`.
9. Bucle de animación **no infinito**: rAF solo mientras el puntero se mueve o la interpolación no converge; se detiene en reposo, con pestaña oculta o panel colapsado.
10. Texturas: grano SVG ≤ 3% opacidad, solo en el fondo de la app, nunca sobre texto de lectura.

## 3. Identidad: el Sigilo de Dioptra (icono vivo) — `ui/components/dioptra-sigil.js` (nuevo)
Custom element `<dx-sigil size="full|mark|glyph" state="awake|asleep">`, SVG inline construido con DOM (sin innerHTML con datos externos), Shadow DOM + `adoptedStyleSheets`, colores **solo** por `var(--token)`.
- **Tres niveles de detalle** (el icono actual se ve tosco porque usa trazos de 4-4.5 a 178px y no escala):
  - `full` (≈150-170 px, cabecera del códice): criatura completa — caparazón de cobre con ojo, eje con topes de bronce inclinado −18°, alas de bronce con espiral, cuello retorcido, 5 tentáculos con 3 ojos romboidales.
  - `mark` (28-40 px, cabecera compacta / panel colapsado): solo caparazón + ojo + eje. Sin tentáculos.
  - `glyph` (14-20 px, favicon/insignias): ojo romboidal (rombo de tinta, iris oro, pupila).
- **Jerarquía de trazo fina**: contorno exterior 2.6-3 (viewBox 260), detalles 1.2-1.6, grabados 1; `stroke-linejoin: round`; `vector-effect: non-scaling-stroke` solo en `mark`/`glyph`.
- **Comportamiento (motion con intención):**
  - Seguimiento: iris y pupila siguen el puntero con lerp (iris 0.14, pupila con paralaje ×1.2), compensando la rotación de −18°; límite elíptico dentro de la cuenca. Los 3 ojos de tentáculo siguen con amplitud menor.
  - Microsacadas: en reposo > 2.5 s, saltos pequeños aleatorios de la pupila cada 1.2-3 s (≤ 2 px), sin bucle continuo de rAF.
  - Parpadeo: párpados de cobre por `transform: translateY` (≤ 140 ms), automático cada 3.5-8 s con 30% de doble parpadeo; clic = parpadeo + destello oro breve.
  - Atención: al hacer hover sobre un reporte de la lista, la pupila se contrae ligeramente (scale 0.85) y mira hacia él; al llegar un reporte nuevo (evento existente de la app), ojo muy abierto 600 ms + destello.
  - Respiración: tentáculos ±2.5° con duraciones desfasadas (3.8-4.6 s), **pausados** con panel colapsado, pestaña oculta o reduced-motion.
  - Sueño (`asleep`): caparazón cerrado; al pasar el puntero, rendija dorada se entreabre (200 ms, `cubic-bezier(.22,1,.36,1)`).
  - Despertar (único momento coreografiado): al expandir el panel, rendija → apertura completa con overshoot leve + un parpadeo, 420 ms total.
- Accesible: `role="img"`, `aria-label="Dioptra"`; decorativo en lista (`aria-hidden`). Botón de despertar con `<button>` real y etiqueta visible o `aria-label`.

## 4. Shell
- **Sidebar = Códice**: cabecera con rótulo "DIOPTRA" (texto real con `--font-display`, sombra de tinta dura, sin SVG de texto) + `<dx-sigil size="full">`; buscador tallado; chips de proyecto/tool/model como sellos achaflanados; lista de reportes como tablillas (activa: borde interior cobre + tinte, sin glow).
- **Panel colapsable** (nuevo): botón para colapsar a columna de ~72 px con `<dx-sigil size="mark" state="asleep">` arriba y accesos numerados (I, II, III…) a los reportes recientes; preferencia en `localStorage` como el resto. En < 1024 px: panel deslizante con velo.
- **Cabecera del reporte**: placa `modelo ◆ proyecto` (rombo SVG), título en `--font-display`, fecha, severidad máxima como sello, progreso de checklist como barra de sellos (cobre→oro).
- **Lectura**: ancho de lectura ≤ 72ch para prosa, `--font-sans` para cuerpo, `--font-display` solo en h1/h2 y rótulos cortos.

## 5. Componentes (fase 2, mismo lenguaje)
kpi → losas achaflanadas con cifra en oro tabular; callout → §2.1; table → cabecera bronce `--font-display` versalitas, filas piedra, hover tinte oro sutil; code → tótem de obsidiana con cabecera de lenguaje y botón copiar; checklist → sello romboidal que enciende un ojo oro al marcar (y el sigilo reacciona); section → bisagra de bronce; diagram → nodos losa + conductos cobre con flecha oro; prose → reglas §4; canvas → solo hereda tokens.

## 6. Criterios de aceptación
- [ ] CSP intacta y sin recursos externos; `go test ./... -count=1` verde.
- [ ] Ningún token renombrado/borrado; tokens nuevos declarados en el bloque oscuro y en el claro.
- [ ] Contraste ≥ 4.5:1 en texto y ≥ 3:1 en elementos grandes, ambos temas.
- [ ] Sigilo en 3 niveles; seguimiento, parpadeo, sueño/despertar, atención y reduced-motion funcionando; rAF se detiene en reposo.
- [ ] Ninguna franja lateral de color > 1px, ningún emoji-icono, ningún toast trivial, texto seleccionable.
- [ ] Selectores/atributos de §0 intactos; tema claro y oscuro funcionales.
