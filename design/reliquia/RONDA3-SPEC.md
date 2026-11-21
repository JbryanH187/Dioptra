# Ronda 3: icono Bellum del usuario + pase Impeccable

La fuente de verdad del dibujo es `design/reliquia/bellum-reference.svg`, aprobado por el usuario: "tiene que ser así el dibujo". Solo se permiten las adecuaciones obvias para que quede limpio y homogéneo con la UI. No se rediseña nada.

Siguen vigentes las restricciones de las rondas 1 y 2:
- CSP `default-src 'self'`, sin `style=`/`onclick=`/`innerHTML` con datos (CSSOM sí está permitido).
- No renombrar ni borrar tokens.
- No dejar ninguna línea que empiece por `}` dentro del bloque oscuro de `tokens.css` antes de su cierre.
- Cumplir el contrato de `SHELL-CONTRACT.md`.

## A. Sigilo (`ui/components/dioptra-sigil.js`)

### Fidelidad
El nivel `full` reproduce la referencia pieza por pieza, con las mismas coordenadas, paths, viewBox `0 0 500 540`, capas y orden de pintado:
1. Cinco tentáculos: 2 internos, 2 guadañas externas con poros y miniojo, y la lanza central con miniojo romboidal.
2. Alas escalonadas de bronce con espiral grabada.
3. Cuello y vientre en espiral.
4. Cabeza a −20° con el eje mecánico y sus perillas, la cuenca con estrías, el globo de doble anillo (iris r47, núcleo r29.5, pupila r16.8 y brillo), los párpados de cobre, la bóveda superior y la copa inferior tallada en zigzag.

### Adecuaciones permitidas (y solo estas)
1. **Colores a tokens.** Cada color se declara en la hoja del shadow root como `fill: var(--sigil-…, <hex de la referencia>)` o `stop-color: var(…)`. Los stops de los gradientes llevan clase y su color va por CSS (los atributos de presentación no aceptan `var()`). Los tokens globales `--sigil-*` los define `tokens.css` (sección C). El arte conserva su paleta en ambos temas: un ojo de oro y un cobre se ven igual de día. Por tema solo cambian:
   - `--sigil-ink`: el contorno; en claro es el marrón tinta.
   - `--sigil-rim`: un halo de silueta para que los tentáculos abisales no desaparezcan sobre el fondo oscuro. Se implementa como una capa de los mismos paths de silueta, por detrás, con `stroke: var(--sigil-rim)`, un trazo 4 unidades mayor que el contorno, `fill: none` y `vector-effect` normal.
2. **IDs únicos por instancia.** Prefijo `sg<n>-` en gradientes y clipPaths; los `url(#…)` se construyen con ese prefijo. No puede quedar ningún `id` fijo.
3. **Tamaños.** El trazo de la referencia está pensado para unos 380 px, así que cada tamaño lleva su propio dibujo:
   - `mark`, para la cabecera y el rail de unos 36 a 56 px: solo la cabeza (eje con perillas, cuenca, globo de doble anillo, párpados, bóveda y copa). El viewBox se recorta a la cabeza. Se quitan el zigzag fino, los destellos pequeños y las estrías interiores (queda solo la exterior). Trazo mínimo de 2 px renderizados, así que hay que escalar los `stroke-width` de forma proporcional.
   - `glyph`, para 16 a 24 px: almendra de la cuenca, iris con pupila, bóveda superior y copa inferior como dos masas de cobre. No lleva eje, estrías, zigzag ni brillos, y como máximo tiene unas 7 formas. Debe leerse como ojo de Bellum a 16 px.
   - Los tres niveles comparten silueta y paleta.
4. **Motor.** Se conservan la API pública (`size`, `state`, `decorative`; `blink`, `attend`, `release`, `celebrate`, `wake`, `sleep`) y las garantías que comprueba el harness: no hay rAF en reposo, se limpian timers y rAF al dormir o desconectar, y se respeta `prefers-reduced-motion`. Comportamiento de la referencia adaptado:
   - **Seguimiento del puntero.** Rotación del vector por +20°. El globo se desplaza ±33 en X y ±11 en Y, el núcleo tiene un paralaje de ±6.8/±3.2 y las estrías se mueven al 0.14. Las perillas se empujan `miradaX*2.8*0.7` y la cabeza se balancea `-20 + miradaX*2.6`. Lerp de 0.16 con el puntero y de 0.075 en autónomo.
   - **Escaneo autónomo** izquierda↔derecha con pausas de 1150 a 2600 ms. Se programa con `setTimeout`. El rAF solo corre mientras la mirada converge y se detiene al llegar (|Δ|<0.002).
   - **Parpadeo.** Tres curvas: `rapido` 190 ms, `doble` 390 ms y `siniestro` 560 ms. Los párpados quedan en ±55 en reposo y se juntan en 0. Hay seguimiento vertical de `miradaY*4.5`. Los parpadeos se programan con timers a intervalos de 2400 a 5800 ms.
   - **Miniojos.** Sus pupilas siguen la mirada. Cada uno parpadea en su propio ciclo con desfase de 4700, 5600 y 5100 ms, mediante una animación CSS sobre el rect del párpado con `transform`.
   - **Partes que se mueven solas.** La flotación del cuerpo, el aleteo de las alas y la ondulación de los 5 tentáculos (ángulos y pivotes de la referencia) se hacen con animaciones CSS `@keyframes` sobre `transform` con `transform-box: view-box` y su `transform-origin`, no con JS. Se pausan con `asleep` (`animation-play-state: paused`) y se desactivan con reduced-motion. La respuesta a `celebrate`/clic, que es el impulso de los tentáculos, se hace con una clase temporal.
   - **Estado dormido.** Párpados cerrados (cierre = 1), mirada en (0, 0.1) y animaciones CSS al 35 % de velocidad.
   - No se escucha en `window` (solo `pointermove` en `document`, con passive y limitado por el bucle). No se crea ningún global y no se llama a `getBoundingClientRect` en cada cuadro: se cachea y se invalida con resize/scroll vía ResizeObserver.
5. **Accesibilidad.** Se mantienen `decorative`, `role="img"` y `aria-label` como hoy, y `forced-colors` (contornos en `CanvasText`).

### Verificación
- `node design/reliquia/sigil-harness.mjs` en verde. Las aserciones de la alidada (T10) se sustituyen por equivalentes del eje: existe en `full` y `mark`, no en `glyph`; las perillas se desplazan con el puntero; quedan en 0 con reduced-motion y en asleep; no hay rAF en reposo. Se añaden aserciones de IDs únicos con 2 instancias y de ausencia de literales de color fuera de `var()` en la hoja.
- Se regenera `design/reliquia/sigil-preview.svg` en los 3 niveles, lado a lado y en ambos temas.

## B. Visor y tooltip (`dx-evidence.js`, `dx-tooltip.js`)
Hallazgos de la auditoría de Impeccable de la ronda 2:
- Foco: `outline:2px solid transparent; box-shadow: inset 0 0 0 2px var(--color-ink), var(--focus-ring)` en todos los botones.
- Quitar `aria-live` del cuerpo del código. Usar un `p.sr-only[role=status]` con «Evidencia cargada: N líneas, línea L resaltada» y «Ruta copiada».
- Fondo del tooltip `--color-surface-elevated`. Contorno `--color-bronze` en oscuro y `--color-ink` en claro.
- Borde del diálogo en oscuro: `inset 0 0 0 2px var(--color-ink), 0 0 0 1px var(--color-bronze)`.
- Ruta y línea del título en `--font-mono` (Cinzel pierde la distinción entre mayúsculas y minúsculas).
- `aria-description` en disparadores dentro de shadow roots, que se quita al ocultar.
- Espaciados y animación con tokens de escala (`--spacing-*`, `--motion-duration-fast`, `--motion-easing-*`).
- «Copiado» vuelve a su texto a los 1400 ms.
- «Reintentar» también en `read_error` y `roots_unreadable`.
- En forced-colors, `button:disabled{color:GrayText;border-color:GrayText}`.
- Chaflán coherente: en el tooltip, `--clip-chamfer-xs`, que se define en tokens (C); en el diálogo, sobre un envoltorio interior para no recortar la sombra.

## C. Shell + modo claro (`shell.css`, `tokens.css`, `dx-timeline.js`, `dx-app.js`, `free-canvas.js`)
- Tokens nuevos en ambos temas:
  - `--sigil-ink`: oscuro `#0E1116`; claro `#2A1E14`.
  - `--sigil-rim`: oscuro `rgba(229,178,92,.28)`; claro `transparent`.
  - `--clip-chamfer-xs`: `polygon(4px 0,calc(100% - 4px) 0,100% 4px,100% calc(100% - 4px),calc(100% - 4px) 100%,4px 100%,0 calc(100% - 4px),0 4px)`.
- Foco con `forced-colors`: `:focus-visible{outline:2px solid Highlight;outline-offset:2px}`.
- Drawer (<1024 px): `inert` en el timeline cuando está cerrado y en `dx-report` + cabecera cuando está abierto (`_setDrawerOpen`).
- Al colapsar o expandir, el foco pasa a `.dx-rail-wake` o `.dx-collapse-btn`.
- El botón del drawer va por encima de la cabecera fija y no tapa el título.
- Timeline con estados `loading` (esqueleto), `error` (con reintento) y `empty`. «Sin reportes» se distingue de «Sin coincidencias», que lleva el botón «Limpiar filtros».
- Modo claro, accesibilidad (el usuario percibe que aún le falta):
  - Contador de chips con opacidad 0.7, que da 3.3: usar color sólido `--text-secondary` hasta ≥4.5.
  - Anillo de foco en claro (2.4–2.8): darle un token propio en claro con ≥3:1 frente a la cabecera y las losas, por ejemplo cobre profundo.
  - Separadores con ≥1.5:1 si son estructurales.
  - Contorno del botón de tema en oscuro: 1.41.
  - Todos los pares medidos con node (no a mano) y la tabla final actualizada en `tokens.css`.
- Detector de Impeccable sin hallazgos sobre los archivos tocados.
