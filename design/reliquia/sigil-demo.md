# Sigilo de Dioptra (`dx-sigil`)

## Registro y presentación

El módulo `ui/components/index.js` registra el elemento; también puede importarse
`ui/components/dioptra-sigil.js` directamente. No requiere paquetes ni recursos externos.

```html
<dx-sigil id="sigilo" size="full" state="awake"></dx-sigil>
<dx-sigil size="mark" state="asleep"></dx-sigil>
<dx-sigil size="glyph" decorative></dx-sigil>
<button id="despertar" type="button">Despertar Dioptra</button>
```

| Atributo | Valores / comportamiento |
| --- | --- |
| `size` | `full` (predeterminado): criatura completa, 160 px, viewBox 260×250. `mark`: caparazón, ojo y eje, 36 px, viewBox propio. `glyph`: ojo romboidal, 18 px. |
| `state` | `awake` (predeterminado) o `asleep`. Cambiarlo ejecuta la misma transición que `wake()` / `sleep()`. |
| `decorative` | Booleano por presencia: `aria-hidden="true"`, sin nombre ni rol. Sin él: `role="img"`, `aria-label="Dioptra"`. |
| `hidden` | Oculta y suspende el movimiento y los temporizadores de la instancia. |

Valores desconocidos de `size` / `state` usan el comportamiento `full` / `awake`.
El ancho puede ajustarse desde una hoja CSS externa (`dx-sigil { width: 170px; }`).
La anatomía se construye con DOM SVG, sin inserción de HTML. Todos los colores
usan tokens de Reliquia con fallback oscuro dentro de `var()`. La hoja se adopta
mediante `createSheet` / `adoptSheets`; no se necesitan atributos `style` ni CSP más permisiva.

## API pública

Todos los métodos son síncronos, no devuelven una promesa y pueden invocarse repetidamente.

| Método | Efecto |
| --- | --- |
| `blink(withFlash = false)` | Parpadeo de 140 ms; destello oro de 180 ms si se pasa `true`. El clic invoca `blink(true)`. |
| `attend(x, y)` | Mira coordenadas **clientX/clientY**, en píxeles CSS del viewport; pupila a escala 0.85. Bloquea el seguimiento del puntero hasta `release()`. Ignora coordenadas no finitas. |
| `release()` | Quita la atención, centra suavemente el ojo y permite seguir el siguiente movimiento del puntero. |
| `celebrate()` | Ojo muy abierto durante 600 ms y destello breve. No despierta una instancia dormida. |
| `sleep()` | Establece `state="asleep"`, cierra los párpados en 200 ms y suspende el movimiento. Hover muestra una rendija dorada. |
| `wake()` | Establece `state="awake"`; desde sueño, apertura con pequeño sobrepaso y parpadeo en 420 ms. |

### Integración desde un módulo externo

Este ejemplo es código de un archivo `.js` servido por la aplicación, **no** un
script inline ni un atributo `onclick`. El sigilo sigue siendo una imagen, no un
botón: despertar por teclado debe exponerse mediante un `<button>` real.

```js
import './components/index.js'; // Ajustar respecto al módulo integrador.

const sigilo = document.querySelector('#sigilo');
const despertar = document.querySelector('#despertar');
const onWake = () => sigilo.wake();
despertar.addEventListener('click', onWake);

// Conectar en el componente que posee las filas, también para foco de teclado.
const fila = document.querySelector('.dx-timeline-item[data-report-id]');
const onAttend = () => {
  const rect = fila.getBoundingClientRect();
  sigilo.attend(rect.left + rect.width / 2, rect.top + rect.height / 2);
};
const onRelease = () => sigilo.release();
if (fila) {
  fila.addEventListener('pointerenter', onAttend);
  fila.addEventListener('pointerleave', onRelease);
  fila.addEventListener('focusin', onAttend);
  fila.addEventListener('focusout', onRelease);
}

// Al desmontar esta integración:
function dispose() {
  despertar.removeEventListener('click', onWake);
  fila?.removeEventListener('pointerenter', onAttend);
  fila?.removeEventListener('pointerleave', onRelease);
  fila?.removeEventListener('focusin', onAttend);
  fila?.removeEventListener('focusout', onRelease);
}
```

El evento existente de nuevos reportes es `report_published` del `EventSource`
creado en `ui/app.js`; no es un evento DOM global. Añadir `sigilo.celebrate()`
en ese manejador, después de validar/actualizar el reporte. Este cambio no conecta
el shell ni abre otra conexión SSE: dichas integraciones están fuera de los archivos
permitidos. Al colapsar el panel, el propietario debe establecer `size="mark"` y
llamar `sleep()`; al expandir, restaurar `full` y llamar `wake()`.

## Movimiento, preferencias y ciclo de vida

- La matriz SVG inversa compensa −18°, escala y posición. Lerp del iris 0.14,
  límite elíptico y paralaje de pupila ×1.2; tres ojos secundarios de amplitud menor.
- rAF funciona únicamente hasta converger. Microsacadas discretas tras >2.5 s
  de reposo, separadas 1.2–3 s y con saltos ≤2 unidades del viewBox, sin rAF continuo.
- Parpadeo automático cada 3.5–8 s, con probabilidad de doble del 30%.
  Los cinco tentáculos respiran ±2.5°, con duraciones 3.8–4.6 s desfasadas.
- Documento oculto, `hidden`, sueño y desconexión cancelan rAF, temporizadores
  y animaciones transitorias; la respiración se pausa. Volver a conectar es seguro.
- `prefers-reduced-motion: reduce` se consulta y escucha en vivo: ojo centrado,
  sin seguimiento, microsacadas, parpadeos, destellos, transiciones ni respiración.
  El estado sigue cambiando instantáneamente; la rendija hover no se anima.
- Cada instancia posee IDs de recorte propios y limpia todos sus listeners al desconectarse.

## Comprobación manual recomendada

1. Revisar los tres tamaños en temas oscuro/claro y sin los nuevos tokens declarados.
2. Mover el puntero hacia las esquinas, invocar atención/liberación y comprobar
   que el ojo no sale de su cuenca. Probar scroll, zoom y varias instancias.
3. Dormir/despertar, alternar tamaño, celebrar y desmontar durante una transición.
4. Observar que rAF cesa tras converger; ocultar la pestaña y activar movimiento
   reducido en vivo. Reanudar y confirmar que no se duplican timers/listeners.
5. Revisar accesibilidad y despertar por teclado mediante el botón integrador;
   verificar ausencia de violaciones CSP en la consola.
