---
name: arquitectura-lienzo-ux
description: Style-agnostic structural blueprint for multi-screen product design canvases (v1/v2/v3 pages, bands Foundations->Mobile->Staff->Tablet/Desktop, spacing math, semantic notes). Styling derives 100% from the target project.
---

# Arquitectura de Lienzo UX (Agnóstica al Estilo)

Define **orden, bandas, coordenadas y versionado** de un lienzo de producto (compatible con Dioptra vía `dioptra-design-canvas`).

> **Regla de oro:** fija **estructura**, nunca estética. Paleta, fuentes, voz, radios y densidad nacen **100% del proyecto actual** (`PRODUCT.md`, tokens, brief). Jamás copies estilos ni textos de otro proyecto.

## 1. Orden de trabajo

1. **Identidad:** lee `PRODUCT.md`, tokens, ADRs e historias (o acuérdalos antes).
2. **Banda 0 primero:** tipografía, color medido y carga cognitiva antes de pantallas funcionales.
3. **Banda 1 (móvil principal)** izquierda→derecha con `<a href="Siguiente.dc.html">` para Prototipo.
4. **Roles secundarios y pantallas grandes** (Tablet/iPad/Desktop) tras validar el flujo principal.
5. **Versionado no destructivo:** nunca sobrescribas páginas previas; agrega `v1`→`v2`→`v3` y actualiza `launch.page`.

## 2. Bandas horizontales (arriba → abajo)

Cada página organiza tableros en bandas separadas, con título arriba (`kind: "title1"`) y nota a la derecha:

| Banda | Contenido | Tamaño | Int. | Radio | Paso X |
|---|---|---|---|---|---|
| **0 · Fundamentos** | `Tipografia.dc.html` (combos+escala), `Color.dc.html` (60-30-10+WCAG+sí/no), `Carga.dc.html` (tocables, palabras, tamaños, colores, CTA, bloques) | `1280×900` | `false` | — | `1360` |
| **1 · Principal (Móvil)** | Flujo usuario final (entrada→selección→acción→acceso→confirmación→seguimiento) | `390×844` | `true` | `32` | `470` |
| **2 · Operación (Móvil)** | Segundo rol (staff/admin/dueño): agenda/cola→detalle→métricas→ajustes | `390×844` | `true` | `32` | `470` |
| **3 · Tablet/Desktop** | Layout real grande (rail fijo, multi-columna, maestro-detalle; no móvil estirado) | `1194×834`/`1440×900` | `true` | `24`/`16` | `1274`/`1520` |
| **4 · Opcional** | Directivas, modo oscuro, estados vacíos/error críticos | `1280×844`/`390×844` | caso | `0`/`32` | `w+80` |

*(Omite bandas no aplicables conservando orden y fórmulas.)*

## 3. Fórmulas de coordenadas (cero encimes)

- **Horizontal:** `gapX = 80` → `x_i = i * (w + 80)`.
- **Vertical:** `bandY_next = bandY_prev + bandH_prev + 420` (ref: `0`→`1320`→`2564`→`3808`).
- **Título (`kind: "title1"`):** `x = 0`, `y = bandY - 300`, `maxW = N * (w + 80) - 80`, `"<Rol/tema> · <alcance>"`.
- **Nota derecha (`w: 320..340`):** `x = lastBoard.x + lastBoard.w + 80`, `y = bandY`. `fill`: `"green"` (aprobado/metas), `"orange"` (brechas/pendientes), `"teal"` (implementado), `"gray"` (principios).

## 4. Checklist antes de registrar en Dioptra

- [ ] `pages` conserva versiones previas; `launch.page` apunta a la actual.
- [ ] Pantallas interactivas con `is_interactive: true` y enlaces relativos.
- [ ] `order[]` sigue el orden visual banda por banda.
- [ ] Fundamentos con métricas reales (correr `design/canvas/tools/` si existe).
- [ ] Registrar con `canvas_register` (`dioptra-design-canvas`) y devolver URL directa.
