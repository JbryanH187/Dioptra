---
name: dioptra-design-canvas
description: Publish and iterate local multi-screen HTML design canvases in Dioptra (Lienzo + Prototipo) and resolve Pin/Box/Marker comments via MCP (canvas_register, canvas_list, canvas_get_feedback, canvas_resolve_feedback).
---

# Dioptra Design Canvas (MCP + Visor)

Dioptra sirve lienzos HTML multipantalla **opt-in** en modal 90% (`Lienzo` + `Prototipo`) y expone comentarios visuales (Pin/Caja/Marcador) vía MCP. El lienzo vive en Git (`design/canvas/`). Combina con **`arquitectura-lienzo-ux`** para bandas, fundamentos, coordenadas y `v1`/`v2`/`v3`.

## 1. Estructura (`design/canvas/project/`)

- `canvas.json` (manifiesto), `support.js` (shim `<helmet>` + navegación Prototipo), `*.dc.html` (una pantalla autocontenida por archivo).

### `project/canvas.json`
```json
{
  "v": 1, "title": "Proyecto · Lienzo", "launch": {"view":"canvas","page":"v1"},
  "pages": [{"id":"v1","name":"v1 · Flujo base"}],
  "boards": {
    "Tipografia.dc.html": {"x":0,"y":0,"w":1280,"h":900,"title":"Tipografía","page":"v1"},
    "Inicio.dc.html": {"x":0,"y":1320,"w":390,"h":844,"title":"1 · Inicio","page":"v1","is_interactive":true,"radius":32}
  },
  "order": ["Tipografia.dc.html","Inicio.dc.html"],
  "notes": {
    "t1": {"x":0,"y":-300,"text":"Lineamientos","kind":"title1","maxW":1280,"page":"v1"},
    "n1": {"x":470,"y":1320,"w":340,"text":"Decisiones…","fill":"green","page":"v1"}
  }
}
```

### `project/support.js` (copiar tal cual)
```js
window.DCLogic = window.DCLogic || class { renderVals() { return {}; } };
document.addEventListener('DOMContentLoaded', () => {
  const s = document.createElement('style'); s.textContent = 'html,body{margin:0;padding:0;overflow:hidden}x-dc{display:block}';
  document.head.appendChild(s);
  const h = document.querySelector('helmet');
  if (h) { for (const n of [...h.querySelectorAll('link,style')]) document.head.appendChild(n); h.remove(); }
  if (window.parent !== window) try { window.parent.postMessage({ type: 'tsugi-screen', file: location.pathname.split('/').pop(), title: document.title }, '*'); } catch (_) {}
});
```

## 2. `DesignCanvasCSP`

- `<iframe sandbox="allow-scripts">` (origen `null`); raíz del `.dc.html` = `w × h` exacto de `canvas.json`.
- **OK:** CSS/JS inline, `./support.js`, Google Fonts (`fonts.googleapis.com`/`fonts.gstatic.com`), imágenes locales/`https:`/`data:`/`blob:`, `<a href="Otra.dc.html">`.
- **Bloqueado:** `fetch`/`XHR` externos (`connect-src 'none'`), `<script src="https://...">`, rutas fuera de `dir_path`.

## 3. Flujo MCP

1. `canvas_register { project, canvas_id, title, dir_path }` → URL `http://127.0.0.1:7474/?project=<p>&canvas=<id>`.
2. `canvas_get_feedback { project, canvas_id?, status: "open" }` → `id`, `boardFile`, `pageId`, `kind` (`pin`|`box`|`highlight`), `geometry` (`0..1`), `targetSelector`, `targetText`, `comment`.
3. Edita `project/<boardFile>` ubicando el nodo por `targetSelector` + `targetText` + `geometry` con los tokens del proyecto.
4. `canvas_resolve_feedback { project, canvas_id, annotation_id, status: "applied", note: "<archivo>:<línea> <resumen>" }` → recarga SSE. Usa `"applied"` (el usuario marca `"resolved"`).
