# Ronda 2: correcciones de feedback (usuario, 19-nov)

Base: `SPEC.md`, `SHELL-CONTRACT.md`. Restricciones de la ronda 1 vigentes: CSP `default-src 'self'`, sin `style=`/`onclick=` en markup, no renombrar tokens, selectores protegidos intactos, `prompt.go` corta el bloque oscuro de tokens en el primer `\n}`.

## R2. Menú contraído: el reporte "no se ve" al hacer clic (causa confirmada)
`shell.css:1199` pone el grid en `72px 0 minmax(0,1fr)` y `shell.css:1214` oculta `.dx-splitter` con `display:none`. Sin el splitter, la auto-colocación manda `dx-report` a la **columna 2 (0 px)**. Corrección: dentro de `@media (min-width:1024px)` usar `grid-template-columns: 72px minmax(0,1fr)` y colocar explícitamente `dx-timeline{grid-column:1}` y `dx-report{grid-column:2}` en colapsado. En el estado normal fijar también `dx-timeline{grid-column:1} .dx-splitter{grid-column:2} dx-report{grid-column:3}` para que ningún hijo oculto vuelva a desplazar columnas.

## R4. La evidencia "se queda cargando" (causa confirmada)
`createRefButton` (`shared.js:363-381`) emite `dx-ref` con `{path,line}`, pero **nadie lo escucha** (no hay `addEventListener('dx-ref')` en `ui/`). El botón solo hace `:active scale(.96)`. `parseRefValue` (`shared.js:346-361`) descarta la columna.

## R6. Visor de evidencia: modal con código + abrir en VS Code
### Backend Go
- Registro explícito de raíces: `~/.dioptra/roots.json` = `{"<proyecto-canónico>": "<ruta absoluta>"}`; ubicación = directorio padre de `--dir`. El servidor **nunca** adivina rutas.
- CLI: `dioptra roots list|set <proyecto> <ruta>|remove <proyecto> [--dir …]` con el mismo patrón que `dioptra db alias` (`cmd/dioptra/db_cmd.go`). `set` valida que la ruta exista y sea un directorio, guarda la ruta absoluta limpia y canoniza el proyecto con la misma función que usa el store.
- Endpoint `GET /api/source?project=&path=&line=&context=`:
  - Hay que validar `Host` ∈ {`127.0.0.1:<port>`, `localhost:<port>`} (contra DNS rebinding) y rechazar con 403 si viene `Origin` y no es el mismo origen.
  - Sin raíz registrada para el proyecto, responde 404 `{"error":"root_not_configured","project":…,"hint":"dioptra roots set <proyecto> <ruta>"}`.
  - Se rechazan rutas absolutas, `..`, NUL y rutas vacías. Se resuelve con `filepath.EvalSymlinks` sobre la raíz y el candidato, y se exige que el resultado quede dentro de la raíz.
  - Lista de bloqueo por nombre, sin distinguir mayúsculas: `.env*`, `*.pem`, `*.key`, `*.pfx`, `*.p12`, `id_rsa*`, `*.kdbx`, `secrets.*`, y cualquier segmento `.git`. Archivos > 2 MiB o binarios (NUL en los primeros 8 KiB) dan 415.
  - `context` va por defecto en 40 y como máximo en 120. Se devuelven las líneas `[line-context, line+context]` recortadas a los límites del archivo.
  - Respuesta JSON: `{project, path, absPath, line, startLine, endLine, totalLines, language, lines:[...]}`. El lenguaje se infiere por extensión.
  - Cabeceras: `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`.
- Tests: casos `traversal`, `absoluta`, `symlink fuera de la raíz`, `bloqueado .env`, `binario`, `host inválido`, `sin raíz`, `rango recortado` y `ok`.

### UI
- **`<dx-evidence>`** (`ui/components/dx-evidence.js`) es un modal sobre `<dialog>` nativo con `showModal()`, en el top layer, con Shadow DOM y lenguaje Reliquia.
  - **Cabecera:** ruta, `:línea` y un botón "Abrir en VS Code" (`vscode://file/<absPath>:<line>[:col]`; se activa navegando un `<a>` creado al vuelo). Botones "Copiar ruta" (`navigator.clipboard`) y "Cerrar".
  - **Cuerpo:** líneas numeradas con la línea objetivo resaltada (`--code-line-hl-bg` + marca rombo) y scroll automático hasta ella. El texto siempre va con `textContent`.
  - **Estados:** cargando con `aria-busy`, esqueleto y **timeout de 8 s con AbortController**. Error legible con acción. `root_not_configured` muestra el comando exacto para copiar.
  - **Comportamiento:** Esc y clic fuera cierran. El foco vuelve al botón de origen y el foco queda atrapado mientras está abierto, como hace `<dialog>`.
- **Listener:** `dx-app.js` escucha `dx-ref` y abre `<dx-evidence>` con `{project: proyecto del reporte activo, path, line, column, origin: e.composedPath()[0]}`. El proyecto del reporte activo se obtiene del estado que ya maneja dx-app/app.js; no hay que inventarlo.
- `parseRefValue` conserva la columna (`column`) y el detalle de `dx-ref` la incluye.

## R5. Componente de tooltip
- **`ui/components/dx-tooltip.js`** es un gestor singleton que se monta una vez desde `dx-app`. Es un elemento `<dx-tooltip>` con `popover="manual"`, para que viva en el top layer y no lo tapen `filter`/`isolation`.
- **Detección:** escucha `pointerover`/`pointerout`/`focusin`/`focusout` en `document` y usa `e.composedPath()` para encontrar el primer elemento con `data-tip` o `title`, también dentro de Shadow DOM. Si tiene `title`, lo mueve a `data-tip` para suprimir el tooltip nativo, y si el elemento no tenía nombre accesible añade `aria-label` con ese texto.
- **Tiempos:** aparece tras 450 ms con puntero y al instante con foco de teclado. Se oculta con Esc, scroll, `pointerdown` y blur.
- **Posición:** arriba por defecto; cambia de lado si no cabe y se ajusta al viewport con 8 px de margen. Lleva una flecha romboidal.
- **Estilo Reliquia:** losa obsidiana (en claro, pergamino), contorno de tinta, sombra dura de 2 px, texto `--fs-xs` y máximo 32ch. `role="tooltip"` y `aria-describedby` dinámico en el disparador.
- **Movimiento:** fade + 4 px en 120 ms; con reduced-motion no hay transición.

## R1. Estándares de diseño, sobre todo en modo claro
- Scrim con literal oscuro (`shell.css:1346 rgba(4,6,9,.72)`): pasa a `color-mix(in srgb, var(--color-ink) 45%, transparent)` y un token nuevo `--color-scrim` en ambos temas.
- Sombras duras en claro: `--color-ink-shadow` en claro como sombra parda **suave** (`rgba(60,40,20,.28)`), y `--shadow-ink-*` en claro con desplazamientos 1-2 px.
- `text-shadow` en títulos (`shell.css:1019`, `:1380`) va a `none` en claro, o a una sombra de 1 px muy tenue.
- El grano y `--bevel-stone` en claro se reducen (ver valores en tokens).
- Hay que recorrer todos los literales oscuros fuera de `tokens.css` (`#0…`, `#1…`, `rgba(4,6,9`, `rgba(6,8,12`) en shell.css y componentes, y cambiarlos por tokens.
- Contraste ≥ 4.5:1 en texto y ≥ 3:1 en bordes o controles funcionales en claro, incluidos el rail, los chips, el tooltip y el modal.
- Jerarquía en claro: la base `#E4DEC9` y las losas `#D8D0B8` deben distinguirse; el contorno de tinta en claro debe ser marrón profundo `#2A1E14`, no negro puro.

## R3. Sigilo = Bellum + dioptra (instrumento)
Hay que redibujar el SVG de `dioptra-sigil.js` sin tocar la API ni el motor de movimiento.

**La dioptra** (instrumento topográfico griego) aporta:
- un **disco graduado** circular con marcas cada 10° y números romanos en N/E/S/O, que funciona como cuenca del ojo;
- una **alidada**, la barra de mira diametral que cruza el disco, con **pínulas** (placas verticales perforadas) en ambos extremos; sustituye al eje con topes;
- un **cuello de columna** con tornillo sinfín o engranaje y una **base de trípode**.

**Bellum** (Phantom Hourglass) aporta:
- el **ojo amarillo** con pupila vertical en el centro del disco;
- el **caparazón o capucha** de cobre que envuelve la mitad superior del disco como un párpado;
- **tentáculos** que nacen bajo el disco y se entrelazan con las patas del trípode, cinco en total, tres de ellos con ojo.

Reglas del redibujo:
- Se mantienen los campos `_head`, `_aperture`, `_iris`, `_pupil`, `_top`, `_bottom`, `_flash`, `_smallEyes` y `_svg`. El origen de `_head` sigue siendo el centro del ojo. Los tentáculos son `<g class="tentacle">` hijos consecutivos de un mismo `<g>`.
- **Si cambia el tamaño del ojo**, hay que reescalar a la vez los límites del iris, los párpados y el `transform-origin`.
- **Niveles:**
  - `full`: todo.
  - `mark`: disco + alidada + capucha + ojo.
  - `glyph`: disco simplificado con ojo, sin marcas finas.
- **Estilo:** trazo jerárquico (contorno 2.6-3, detalles 1.2-1.6, grabados 1). Colores solo con `var(--token)`, así que funciona en claro y oscuro.
- **Movimiento:** la alidada gira ±6° siguiendo la horizontal del puntero, como si apuntara, con lerp y respetando reduced-motion.
- **Verificación:** el harness sigue en verde, ampliándolo si cambian nombres internos que no forman parte de la API.
