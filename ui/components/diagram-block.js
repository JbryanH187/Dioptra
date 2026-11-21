// ==========================================================================
// DioptraEngine Viewer — ui/components/diagram-block.js
// <dx-diagram-block> (§4.6 Contrato A — Rutas flow SVG, seq y tree + Pan/Zoom)
// ==========================================================================

import {
  adoptSheets,
  baseBlockSheet,
  clearNode,
  createSheet,
  el,
  SEV_SET,
  svgEl,
  createUiIcon,
} from './shared.js';

const SVG_ALLOWED_TAGS = new Set([
  'svg', 'g', 'path', 'rect', 'circle', 'ellipse', 'line',
  'polyline', 'polygon', 'text', 'tspan', 'title', 'defs', 'marker'
]);

const diagramSheet = createSheet(`
  /* Vista en grande: el lienzo ocupa casi toda la pantalla; el texto alternativo se oculta. */
  :host(:popover-open) {
    position: fixed;
    inset: var(--spacing-5);
    width: auto;
    height: auto;
    max-width: none;
    max-height: none;
    margin: 0;
    padding: var(--spacing-5);
    border: 0;
    overflow: hidden;
    color: var(--color-text-primary);
    background: var(--color-surface-base);
    filter: drop-shadow(0 24px 48px rgba(0, 0, 0, 0.45));
  }

  :host(:popover-open)::backdrop {
    background: rgba(4, 6, 9, 0.72);
  }

  :host(:popover-open) [part="figure"] {
    height: 100%;
  }

  :host(:popover-open) [part="viewport"] {
    flex: 1 1 auto;
    height: auto;
    min-height: 0;
  }

  :host(:popover-open) [part="text-alt"] {
    display: none;
  }

  :host(:popover-open) [part="toolbar"] {
    opacity: 1;
  }

  :host {
    padding: var(--spacing-4);
  }

  [part="figure"] {
    position: relative;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: var(--spacing-3);
  }

  [part="toolbar"] {
    position: absolute;
    top: var(--spacing-3);
    right: var(--spacing-3);
    z-index: var(--z-sticky);
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding: 3px;
    border-radius: var(--rounded-sm);
    background: var(--color-surface-elevated);
    border: var(--border-width) solid var(--color-border-translucent);
    box-shadow: var(--shadow-inset), var(--shadow-floating);
    opacity: 0.7;
    transition: opacity var(--motion-duration-normal) var(--motion-easing-standard),
                border-color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  :host(:hover) [part="toolbar"],
  :host(:focus-within) [part="toolbar"],
  [part="toolbar"]:hover {
    opacity: 1;
    border-color: var(--color-border-strong);
  }

  [part="toolbar"] button {
    width: 28px;
    height: 28px;
    padding: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 0;
    border-radius: var(--rounded-sm);
    background: transparent;
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--fs-small);
    font-weight: var(--fw-semibold);
    line-height: 1;
    cursor: pointer;
    outline: none;
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard),
                color var(--motion-duration-fast) var(--motion-easing-standard),
                box-shadow var(--motion-duration-fast) var(--motion-easing-standard),
                transform var(--motion-duration-instant) var(--motion-easing-standard);
  }

  [part="toolbar"] button:hover {
    background: var(--color-surface-hover);
    color: var(--color-accent-fg);
  }

  [part="toolbar"] button:active {
    transform: scale(0.97);
  }

  [part="toolbar"] button:focus-visible {
    box-shadow: var(--focus-ring);
  }

  .zoom-readout {
    min-width: 4.5ch;
    padding: 0 var(--spacing-1);
    text-align: center;
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    letter-spacing: var(--ls-caps);
    font-variant-numeric: tabular-nums;
    font-variant-ligatures: none;
    color: var(--color-text-soft);
    user-select: none;
  }

  /* Mesa de trabajo: pozo hundido con retícula de puntos sutil */
  [part="viewport"] {
    width: 100%;
    overflow: hidden;
    background-color: var(--color-surface-sunken);
    background-image: radial-gradient(circle at 1px 1px, var(--color-border-translucent) 1px, transparent 1.5px);
    background-size: 16px 16px;
    background-position: 0 0;
    border: var(--border-width) solid var(--color-border-translucent);
    border-radius: var(--rounded-sm);
    box-shadow: inset 0 1px 3px rgba(0, 0, 0, 0.18);
    position: relative;
    transition: border-color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  :host(:hover) [part="viewport"] {
    border-color: var(--color-border);
  }

  [part="svg"] {
    display: block;
    width: 100%;
    height: 100%;
    cursor: grab;
    user-select: none;
    touch-action: pan-y;
    outline: none;
    border-radius: var(--rounded-sm);
    transition: box-shadow var(--motion-duration-fast) var(--motion-easing-standard);
  }

  [part="svg"]:focus-visible {
    box-shadow: inset 0 0 0 2px var(--color-accent-fg);
  }

  [part="svg"][data-panning="true"] {
    cursor: grabbing;
  }

  /* Clases temáticas compartidas por rutas (A) flow y (B) seq/tree */
  .dg-node rect,
  rect.dg-node {
  fill: var(--color-surface-raised);
  stroke: var(--color-ink);
  stroke-width: 2px;
  stroke-linejoin: round;
    transition: stroke var(--motion-duration-fast) var(--motion-easing-standard),
                fill var(--motion-duration-fast) var(--motion-easing-standard),
                filter var(--motion-duration-fast) var(--motion-easing-standard);
  }

  .dg-node {
    cursor: default;
  }

  .dg-sev-info rect, rect.dg-sev-info { stroke: var(--color-sev-info); stroke-width: 2px; }
  .dg-sev-success rect, rect.dg-sev-success { stroke: var(--color-sev-success); stroke-width: 2px; }
  .dg-sev-warning rect, rect.dg-sev-warning { stroke: var(--color-sev-warning); stroke-width: 2px; }
  .dg-sev-error rect, rect.dg-sev-error { stroke: var(--color-sev-error); stroke-width: 2px; }

  /* Nodo no interactivo (cursor default): hover sutil sin halo; conserva stroke de severidad */
.dg-node:hover rect {
  fill: var(--color-stone-steel);
  stroke-width: 3px;
  }
.dg-node:not([class*="dg-sev-"]):hover rect {
  stroke: var(--color-ink);
  }

  .dg-node text,
text.dg-node-label {
  fill: var(--color-text-primary);
  font-family: var(--font-display);
    font-size: 13px;
    font-weight: 500;
  }

.dg-lifeline {
  stroke: var(--color-bronze);
    stroke-width: 1.5px;
    stroke-dasharray: 5 5;
  }

  .dg-edge,
  .dg-edge path,
  .dg-edge line {
  fill: none;
  stroke: var(--color-copper);
  stroke-width: 2.4px;
    transition: stroke var(--motion-duration-fast) var(--motion-easing-standard),
                stroke-opacity var(--motion-duration-fast) var(--motion-easing-standard);
  }

  .dg-edge path,
  .dg-edge line {
  stroke-opacity: 0.9;
  stroke-linecap: round;
  }

  .dg-edge[data-active="true"],
  .dg-edge[data-active="true"] path,
  .dg-edge[data-active="true"] line {
  stroke: var(--color-copper-strong);
  stroke-width: 3px;
  stroke-opacity: 1;
  }

  /* Flujo vivo: solo mientras el bloque está en hover (desactivado en reduced-motion) */
  @keyframes dg-flow {
    from { stroke-dashoffset: 18; }
    to { stroke-dashoffset: 0; }
  }

  :host(:hover) .dg-edge path,
  :host(:hover) .dg-edge line {
    stroke-dasharray: 6 3;
    animation: dg-flow var(--motion-duration-deliberate) linear infinite;
  }

.dg-arrow {
  fill: var(--color-gold);
  stroke: var(--color-ink);
  stroke-width: 0.8px;
  stroke-linejoin: round;
  fill-opacity: 1;
  }

.dg-arrow-active {
  fill: var(--color-gold-glow);
  stroke: var(--color-ink);
  stroke-width: 0.8px;
  stroke-linejoin: round;
  }

.dg-label-bg {
  fill: var(--color-surface-sunken);
  stroke: var(--color-ink);
  stroke-width: 1.5px;
  }

  .dg-label,
  text.dg-label {
    fill: var(--color-text-soft);
    stroke: none;
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
  }

.dg-edge[data-active="true"] .dg-label {
  fill: var(--color-gold-text);
  }

  [part="text-alt"] {
    font-size: var(--fs-caption);
    color: var(--color-text-soft);
  }

  [part="text-alt"] summary {
    cursor: pointer;
    user-select: none;
    font-weight: var(--fw-semibold);
    padding: var(--spacing-1) 0;
    border-radius: var(--rounded-xs);
    transition: color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  [part="text-alt"] summary:hover {
    color: var(--color-text-primary);
  }

  [part="text-alt"] summary:focus-visible {
    outline: none;
    box-shadow: var(--focus-ring);
  }

  [part="text-alt"] ol {
    margin: var(--spacing-2) 0 0;
    padding-left: var(--spacing-5);
    font-family: var(--font-mono);
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .svg-error {
    padding: var(--spacing-4);
    color: var(--color-sev-warning-fg);
    font-size: var(--fs-small);
    background: var(--color-sev-warning-bg);
    border: var(--border-width) solid var(--color-sev-warning-border);
    border-radius: var(--rounded-sm);
  }

@media (prefers-reduced-motion: reduce) {
  .dg-node rect, rect.dg-node,
  .dg-edge, .dg-edge path, .dg-edge line { transition: none; }
}
@media (forced-colors: active) {
  .dg-node rect, rect.dg-node { fill: Canvas; stroke: CanvasText; }
  .dg-node:hover rect { fill: Canvas; stroke: Highlight; }
  .dg-node text, text.dg-node-label, .dg-label, text.dg-label { fill: CanvasText; }
  .dg-edge, .dg-edge path, .dg-edge line, .dg-lifeline { stroke: CanvasText; }
  .dg-arrow, .dg-arrow-active { fill: CanvasText; stroke: CanvasText; }
  .dg-label-bg { fill: Canvas; stroke: CanvasText; }
}
@media (prefers-reduced-motion: reduce) {
  :host(:hover) .dg-edge path,
    :host(:hover) .dg-edge line {
      animation: none;
      stroke-dasharray: none;
    }

    [part="toolbar"] button:active {
      transform: none;
    }
  }
`);

function truncateLabel(str, maxChars = 22) {
  const s = String(str ?? '');
  return s.length > maxChars ? `${s.slice(0, maxChars - 1)}…` : s;
}

function estimateBoxWidth(str) {
  const len = Math.min(22, String(str ?? '').length);
  return Math.min(200, Math.max(108, len * 8 + 28));
}

function createArrowDefs(idPrefix) {
  return svgEl(
    'defs',
    null,
    svgEl(
      'marker',
      {
        id: `${idPrefix}-arrow`,
        viewBox: '0 0 10 10',
        refX: '9',
        refY: '5',
        markerWidth: '7',
        markerHeight: '7',
        orient: 'auto-start-reverse'
      },
      svgEl('path', { d: 'M 0 1.5 L 10 5 L 0 8.5 z', class: 'dg-arrow' })
    ),
    svgEl(
      'marker',
      {
        id: `${idPrefix}-arrow-hl`,
        viewBox: '0 0 10 10',
        refX: '9',
        refY: '5',
        markerWidth: '7',
        markerHeight: '7',
        orient: 'auto-start-reverse'
      },
      svgEl('path', { d: 'M 0 1.5 L 10 5 L 0 8.5 z', class: 'dg-arrow-active' })
    )
  );
}

// Sanitización estricta en defensa en profundidad para SVG precompilado (Ruta A)
function parseAndSanitizePrecompiledSvg(svgString) {
  const parser = new DOMParser();
  const doc = parser.parseFromString(svgString, 'image/svg+xml');
  if (doc.querySelector('parsererror') || doc.querySelector('script') || doc.querySelector('foreignObject')) {
    return null;
  }
  const root = doc.documentElement;
  if (!root || root.tagName.toLowerCase() !== 'svg') return null;

  const sanitizeTree = (node) => {
    const children = Array.from(node.children);
    for (const child of children) {
      const tag = child.tagName.toLowerCase();
      if (!SVG_ALLOWED_TAGS.has(tag)) {
        child.remove();
        continue;
      }
      for (const attr of Array.from(child.attributes)) {
        const name = attr.name.toLowerCase();
        const val = attr.value.trim();
        if (name.startsWith('on')) {
          child.removeAttribute(attr.name);
        } else if ((name === 'href' || name === 'xlink:href') && !val.startsWith('#')) {
          child.removeAttribute(attr.name);
        } else if (name === 'style') {
          child.removeAttribute(attr.name);
        }
      }
      sanitizeTree(child);
    }
  };

  for (const attr of Array.from(root.attributes)) {
    const name = attr.name.toLowerCase();
    if (name.startsWith('on') || name === 'width' || name === 'height' || name === 'style') {
      root.removeAttribute(attr.name);
    }
  }
  sanitizeTree(root);

  if (!root.getAttribute('viewBox')) {
    root.setAttribute('viewBox', '0 0 800 360');
  }
  return document.importNode(root, true);
}

let diagramSeq = 0;

export class DxDiagramBlock extends HTMLElement {
  static get observedAttributes() {
    return ['kind'];
  }

  constructor() {
    super();
    this._data = null;
    this._uid = `dg-${++diagramSeq}`;
    this._baseBox = { x: 0, y: 0, w: 800, h: 360 };
    this._curBox = { x: 0, y: 0, w: 800, h: 360 };
    this._svgEl = null;
    this._zoomReadout = null;
    this.addEventListener('toggle', () => this._onFullscreenChange());
    this._viewportSheet = createSheet('[part="viewport"] { height: 360px; }');
    this.attachShadow({ mode: 'open' });
    adoptSheets(this.shadowRoot, baseBlockSheet, diagramSheet, this._viewportSheet);
  }

  get data() {
    return this._data;
  }

  set data(val) {
    this._data = val;
    const kind = val?.kind === 'seq' || val?.kind === 'tree' || val?.kind === 'flow' ? val.kind : 'flow';
    if (this.getAttribute('kind') !== kind) {
      this.setAttribute('kind', kind);
    }
    this.render();
  }

  attributeChangedCallback(name, oldVal, newVal) {
    if (name === 'kind' && oldVal !== newVal && this._data) {
      this.render();
    }
  }

  _buildSequenceSvg(nodes, edges) {
    const colGap = 196;
    const padX = 36;
    const topY = 24;
    const nodeH = 40;
    const rowStep = 40;

    const colCenters = new Map();
    nodes.forEach((n, idx) => {
      colCenters.set(n.id, padX + 90 + idx * colGap);
    });

    const totalW = Math.max(420, padX * 2 + Math.max(1, nodes.length) * colGap);
    const totalH = Math.max(200, topY + nodeH + 44 + edges.length * rowStep + 28);

    const svg = svgEl('svg', { viewBox: `0 0 ${totalW} ${totalH}` }, createArrowDefs(this._uid));
    const lifelinesGroup = svgEl('g');
    const edgesGroup = svgEl('g');
    const nodesGroup = svgEl('g');
    const edgeEls = [];

    nodes.forEach((n) => {
      const cx = colCenters.get(n.id);
      const bw = estimateBoxWidth(n.label);
      lifelinesGroup.appendChild(svgEl('line', {
        x1: cx,
        y1: topY + nodeH,
        x2: cx,
        y2: totalH - 20,
        class: 'dg-lifeline'
      }));

      const sevCls = SEV_SET.has(n.sev) ? ` dg-sev-${n.sev}` : '';
      const g = svgEl(
        'g',
        { class: `dg-node${sevCls}`, 'data-node-id': n.id },
        svgEl('title', null, n.label),
        svgEl('rect', { x: cx - bw / 2, y: topY, width: bw, height: nodeH, rx: '3' }),
        svgEl('text', {
          x: cx,
          y: topY + 25,
          'text-anchor': 'middle',
          class: 'dg-node-label'
        }, truncateLabel(n.label))
      );
      nodesGroup.appendChild(g);
    });

    edges.forEach((e, idx) => {
      const [from, to, label] = e;
      const x1 = colCenters.get(from);
      const x2 = colCenters.get(to);
      if (x1 === undefined || x2 === undefined) return;
      const y = topY + nodeH + 40 + idx * rowStep;

      const eg = svgEl('g', { class: 'dg-edge', 'data-from': from, 'data-to': to });
      let pathEl;
      let lx;
      let ly;

      if (from === to) {
        pathEl = svgEl('path', {
          d: `M ${x1} ${y - 8} H ${x1 + 48} V ${y + 12} H ${x1 + 4}`,
          'marker-end': `url(#${this._uid}-arrow)`
        });
        lx = x1 + 54;
        ly = y + 4;
      } else {
        const dir = x2 > x1 ? 1 : -1;
        pathEl = svgEl('line', {
          x1,
          y1: y,
          x2: x2 - dir * 4,
          y2: y,
          'marker-end': `url(#${this._uid}-arrow)`
        });
        lx = (x1 + x2) / 2;
        ly = y - 8;
      }

      eg.appendChild(pathEl);
      if (label) {
        const textStr = truncateLabel(label, 28);
        const lw = textStr.length * 6.8 + 14;
        const anchorX = from === to ? lx + lw / 2 : lx;
        eg.appendChild(svgEl('rect', {
          x: anchorX - lw / 2,
          y: ly - 12,
          width: lw,
          height: 18,
          rx: '4',
          class: 'dg-label-bg'
        }));
        eg.appendChild(svgEl('text', {
          x: anchorX,
          y: ly + 1,
          'text-anchor': 'middle',
          class: 'dg-label'
        }, textStr));
      }

      edgesGroup.appendChild(eg);
      edgeEls.push({ eg, pathEl, from, to });
    });

    svg.appendChild(lifelinesGroup);
    svg.appendChild(edgesGroup);
    svg.appendChild(nodesGroup);
    this._wireHoverHighlight(nodesGroup, edgeEls);
    return { svg, w: totalW, h: totalH };
  }

  _buildTreeOrLayeredSvg(nodes, edges) {
    const nodeMap = new Map(nodes.map((n) => [n.id, n]));
    const childrenMap = new Map(nodes.map((n) => [n.id, []]));
    const indegree = new Map(nodes.map((n) => [n.id, 0]));

    for (const [p, c] of edges) {
      if (!nodeMap.has(p) || !nodeMap.has(c) || p === c) continue;
      if ((indegree.get(c) || 0) === 0) {
        childrenMap.get(p).push(c);
        indegree.set(c, 1);
      }
    }

    const roots = nodes.filter((n) => (indegree.get(n.id) || 0) === 0).map((n) => n.id);
    if (roots.length === 0 && nodes.length > 0) {
      roots.push(nodes[0].id);
    }

    const posX = new Map();
    const posDepth = new Map();
    const visited = new Set();
    let leafSlot = 0;
    const slotSpacing = 210;
    let maxDepth = 0;

    const dfs = (id, depth) => {
      visited.add(id);
      posDepth.set(id, depth);
      if (depth > maxDepth) maxDepth = depth;

      const kids = (childrenMap.get(id) || []).filter((k) => !visited.has(k));
      if (kids.length === 0) {
        const x = 120 + leafSlot * slotSpacing;
        leafSlot++;
        posX.set(id, x);
        return x;
      }

      let sum = 0;
      for (const k of kids) {
        sum += dfs(k, depth + 1);
      }
      const x = sum / kids.length;
      posX.set(id, x);
      return x;
    };

    for (const r of roots) {
      if (!visited.has(r)) dfs(r, 0);
    }
    for (const n of nodes) {
      if (!visited.has(n.id)) dfs(n.id, 0);
    }

    const levelGap = 72;
    const nodeH = 40;
    const topPad = 28;
    const totalW = Math.max(420, Math.max(1, leafSlot) * slotSpacing + 40);
    const totalH = Math.max(180, topPad * 2 + (maxDepth + 1) * levelGap);

    const svg = svgEl('svg', { viewBox: `0 0 ${totalW} ${totalH}` }, createArrowDefs(this._uid));
    const edgesGroup = svgEl('g');
    const nodesGroup = svgEl('g');
    const edgeEls = [];

    for (const [from, to, label] of edges) {
      const x1 = posX.get(from);
      const x2 = posX.get(to);
      const d1 = posDepth.get(from);
      const d2 = posDepth.get(to);
      if (x1 === undefined || x2 === undefined) continue;

      const y1 = topPad + d1 * levelGap + nodeH;
      const y2 = topPad + d2 * levelGap - 4;
      const midY = (y1 + y2) / 2;

      const eg = svgEl('g', { class: 'dg-edge', 'data-from': from, 'data-to': to });
      const pathEl = svgEl('path', {
        d: `M ${x1} ${y1} C ${x1} ${midY}, ${x2} ${midY}, ${x2} ${y2}`,
        'marker-end': `url(#${this._uid}-arrow)`
      });
      eg.appendChild(pathEl);

      if (label) {
        const textStr = truncateLabel(label, 22);
        const lw = textStr.length * 6.8 + 12;
        const lx = (x1 + x2) / 2;
        const ly = midY;
        eg.appendChild(svgEl('rect', {
          x: lx - lw / 2,
          y: ly - 9,
          width: lw,
          height: 18,
          rx: '4',
          class: 'dg-label-bg'
        }));
        eg.appendChild(svgEl('text', {
          x: lx,
          y: ly + 4,
          'text-anchor': 'middle',
          class: 'dg-label'
        }, textStr));
      }

      edgesGroup.appendChild(eg);
      edgeEls.push({ eg, pathEl, from, to });
    }

    for (const n of nodes) {
      const cx = posX.get(n.id) ?? 120;
      const y = topPad + (posDepth.get(n.id) ?? 0) * levelGap;
      const bw = estimateBoxWidth(n.label);
      const sevCls = SEV_SET.has(n.sev) ? ` dg-sev-${n.sev}` : '';

      const g = svgEl(
        'g',
        { class: `dg-node${sevCls}`, 'data-node-id': n.id },
        svgEl('title', null, n.label),
        svgEl('rect', { x: cx - bw / 2, y, width: bw, height: nodeH, rx: '3' }),
        svgEl('text', {
          x: cx,
          y: y + 25,
          'text-anchor': 'middle',
          class: 'dg-node-label'
        }, truncateLabel(n.label))
      );
      nodesGroup.appendChild(g);
    }

    svg.appendChild(edgesGroup);
    svg.appendChild(nodesGroup);
    this._wireHoverHighlight(nodesGroup, edgeEls);
    return { svg, w: totalW, h: totalH };
  }

  _wireHoverHighlight(nodesGroup, edgeEls) {
    nodesGroup.querySelectorAll('.dg-node').forEach((nodeEl) => {
      const nodeId = nodeEl.getAttribute('data-node-id');
      nodeEl.addEventListener('pointerenter', () => {
        for (const item of edgeEls) {
          const active = item.from === nodeId || item.to === nodeId;
          item.eg.setAttribute('data-active', active ? 'true' : 'false');
          item.pathEl.setAttribute(
            'marker-end',
            active ? `url(#${this._uid}-arrow-hl)` : `url(#${this._uid}-arrow)`
          );
        }
      });
      nodeEl.addEventListener('pointerleave', () => {
        for (const item of edgeEls) {
          item.eg.setAttribute('data-active', 'false');
          item.pathEl.setAttribute('marker-end', `url(#${this._uid}-arrow)`);
        }
      });
    });
  }

  _applyViewBox() {
    if (!this._svgEl) return;
    const { x, y, w, h } = this._curBox;
    this._svgEl.setAttribute('viewBox', `${x.toFixed(2)} ${y.toFixed(2)} ${w.toFixed(2)} ${h.toFixed(2)}`);
    if (this._zoomReadout) {
      const scale = this._baseBox.w / w;
      this._zoomReadout.textContent = `${Math.round(scale * 100)}%`;
    }
  }

  _zoomBy(factor, focalRatioX = 0.5, focalRatioY = 0.5) {
    const currentScale = this._baseBox.w / this._curBox.w;
    const nextScale = Math.min(4, Math.max(0.25, currentScale * factor));
    const nextW = this._baseBox.w / nextScale;
    const nextH = this._baseBox.h / nextScale;

    const anchorX = this._curBox.x + this._curBox.w * focalRatioX;
    const anchorY = this._curBox.y + this._curBox.h * focalRatioY;

    this._curBox = {
      x: anchorX - nextW * focalRatioX,
      y: anchorY - nextH * focalRatioY,
      w: nextW,
      h: nextH
    };
    this._applyViewBox();
  }

  _toggleExpanded() {
    if (!this.hasAttribute('popover')) this.setAttribute('popover', 'auto');
    try {
      if (this.matches(':popover-open')) this.hidePopover();
      else this.showPopover();
    } catch {
      // Sin soporte de popover: el diagrama sigue en su tamaño normal.
    }
  }

  _onFullscreenChange() {
    const on = this.matches(':popover-open');
    if (!on) this.removeAttribute('popover');
    if (this._expandBtn) {
      this._expandBtn.replaceChildren(createUiIcon(on ? 'minimize' : 'maximize', 14));
      this._expandBtn.setAttribute('aria-label', on ? 'Cerrar vista en grande' : 'Ver en grande');
      this._expandBtn.title = on ? 'Cerrar (Esc)' : 'Ver en grande (F) · Esc para cerrar';
    }
    // El encuadre se recalcula con el nuevo tamaño del lienzo.
    if (this._baseBox) requestAnimationFrame(() => this._resetViewBox());
  }

  _resetViewBox() {
    this._curBox = { ...this._baseBox };
    this._applyViewBox();
  }

  _attachViewportInteractions(viewport, svg) {
    // Zoom con Ctrl/Meta + rueda; la rueda normal NO se intercepta (§4.6)
    viewport.addEventListener('wheel', (e) => {
      if (!e.ctrlKey && !e.metaKey) return;
      e.preventDefault();
      const rect = svg.getBoundingClientRect();
      const rx = rect.width > 0 ? (e.clientX - rect.left) / rect.width : 0.5;
      const ry = rect.height > 0 ? (e.clientY - rect.top) / rect.height : 0.5;
      const factor = e.deltaY < 0 ? 1.18 : 1 / 1.18;
      this._zoomBy(factor, rx, ry);
    }, { passive: false });

    // Pan por arrastre con captura de puntero
    let dragging = false;
    let startX = 0;
    let startY = 0;
    let startBoxX = 0;
    let startBoxY = 0;

    svg.addEventListener('pointerdown', (e) => {
      if (e.button !== 0) return;
      dragging = true;
      startX = e.clientX;
      startY = e.clientY;
      startBoxX = this._curBox.x;
      startBoxY = this._curBox.y;
      svg.setAttribute('data-panning', 'true');
      svg.setPointerCapture(e.pointerId);
    });

    svg.addEventListener('pointermove', (e) => {
      if (!dragging) return;
      const rect = svg.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) return;
      const dx = (e.clientX - startX) * (this._curBox.w / rect.width);
      const dy = (e.clientY - startY) * (this._curBox.h / rect.height);
      this._curBox.x = startBoxX - dx;
      this._curBox.y = startBoxY - dy;
      this._applyViewBox();
    });

    const endDrag = (e) => {
      if (!dragging) return;
      dragging = false;
      svg.removeAttribute('data-panning');
      try {
        if (svg.hasPointerCapture(e.pointerId)) {
          svg.releasePointerCapture(e.pointerId);
        }
      } catch {
        // Ignorado
      }
    };

    svg.addEventListener('pointerup', endDrag);
    svg.addEventListener('pointercancel', endDrag);

    // Controles de teclado sobre el SVG enfocado
    svg.addEventListener('keydown', (e) => {
      const stepX = this._curBox.w * 0.08;
      const stepY = this._curBox.h * 0.08;
      switch (e.key) {
        case 'f':
        case 'F':
          if (this._expandBtn) {
            e.preventDefault();
            this._toggleExpanded();
          }
          break;
        case '+':
        case '=':
          e.preventDefault();
          this._zoomBy(1.2);
          break;
        case '-':
        case '_':
          e.preventDefault();
          this._zoomBy(1 / 1.2);
          break;
        case '0':
          e.preventDefault();
          this._resetViewBox();
          break;
        case 'ArrowLeft':
          e.preventDefault();
          this._curBox.x -= stepX;
          this._applyViewBox();
          break;
        case 'ArrowRight':
          e.preventDefault();
          this._curBox.x += stepX;
          this._applyViewBox();
          break;
        case 'ArrowUp':
          e.preventDefault();
          this._curBox.y -= stepY;
          this._applyViewBox();
          break;
        case 'ArrowDown':
          e.preventDefault();
          this._curBox.y += stepY;
          this._applyViewBox();
          break;
        default:
          break;
      }
    });
  }

  render() {
    clearNode(this.shadowRoot);
    if (!this._data) return;

    const kind = this.getAttribute('kind') || 'flow';
    const nodes = Array.isArray(this._data.nodes) ? this._data.nodes : [];
    const edges = Array.isArray(this._data.edges) ? this._data.edges : [];
    const nodeLabels = new Map(nodes.map((n) => [n.id, n.label]));

    let built = null;

    if (kind === 'flow' && typeof this._data.svg === 'string' && this._data.svg.trim()) {
      const imported = parseAndSanitizePrecompiledSvg(this._data.svg);
      if (imported) {
        const vbParts = (imported.getAttribute('viewBox') || '0 0 800 360')
          .trim()
          .split(/[\s,]+/)
          .map(Number);
        const [bx = 0, by = 0, bw = 800, bh = 360] = vbParts;
        built = { svg: imported, x: bx, y: by, w: bw || 800, h: bh || 360 };
      }
    } else if (kind === 'seq') {
      const res = this._buildSequenceSvg(nodes, edges);
      built = { svg: res.svg, x: 0, y: 0, w: res.w, h: res.h };
    } else {
      const res = this._buildTreeOrLayeredSvg(nodes, edges);
      built = { svg: res.svg, x: 0, y: 0, w: res.w, h: res.h };
    }

    const figure = el('figure', { part: 'figure' });

    if (!built) {
      figure.appendChild(el('div', { className: 'svg-error' }, 'SVG de diagrama rechazado por validación de seguridad.'));
      this.shadowRoot.appendChild(figure);
      return;
    }

    this._baseBox = { x: built.x, y: built.y, w: built.w, h: built.h };
    this._curBox = { ...this._baseBox };
    this._viewportSheet.replaceSync(`[part="viewport"] { height: min(60vh, ${Math.max(220, Math.round(built.h))}px); }`);

    const kindLabel = kind === 'seq' ? 'Secuencia' : kind === 'tree' ? 'Árbol' : 'Flujo';
    const ariaSummary = nodes.slice(0, 4).map((n) => n.label).join(' → ');
    built.svg.setAttribute('part', 'svg');
    built.svg.setAttribute('role', 'img');
    built.svg.setAttribute('tabindex', '0');
    built.svg.setAttribute('aria-label', `${kindLabel}: ${ariaSummary || 'diagrama técnico'}`);
    this._svgEl = built.svg;

    const btnOut = el('button', { type: 'button', 'aria-label': 'Alejar', title: 'Alejar (−)' }, createUiIcon('minus', 14));
    const btnIn = el('button', { type: 'button', 'aria-label': 'Acercar', title: 'Acercar (+)' }, createUiIcon('plus', 14));
    const btnFit = el('button', { type: 'button', 'aria-label': 'Ajustar', title: 'Ajustar encuadre (0)' }, createUiIcon('fit', 14));
    // Vista en grande con la capa superior nativa (popover): escapa de cualquier
    // contenedor con filter/transform —un overlay position:fixed quedaría atrapado
    // dentro de la sección—, cierra con Esc y no pide permisos como fullscreen.
    const canExpand = typeof this.showPopover === 'function';
    const btnExpand = canExpand
      ? el('button', { type: 'button', 'aria-label': 'Ver en grande', title: 'Ver en grande (F) · Esc para cerrar' }, createUiIcon('maximize', 14))
      : null;
    if (btnExpand) {
      btnExpand.addEventListener('click', () => this._toggleExpanded());
      this._expandBtn = btnExpand;
    }
    this._zoomReadout = el('span', { className: 'zoom-readout' }, '100%');

    btnOut.addEventListener('click', () => this._zoomBy(1 / 1.25));
    btnIn.addEventListener('click', () => this._zoomBy(1.25));
    btnFit.addEventListener('click', () => this._resetViewBox());

    const toolbar = el('div', { part: 'toolbar' }, btnOut, this._zoomReadout, btnIn, btnFit, btnExpand);
    const viewport = el('div', { part: 'viewport' }, built.svg);
    this._attachViewportInteractions(viewport, built.svg);
    this._applyViewBox();

    const ol = el('ol');
    if (edges.length > 0) {
      for (const [from, to, label] of edges) {
        const fromTxt = nodeLabels.get(from) || from;
        const toTxt = nodeLabels.get(to) || to;
        ol.appendChild(el('li', null, label ? `${fromTxt} → ${toTxt}: ${label}` : `${fromTxt} → ${toTxt}`));
      }
    } else {
      for (const n of nodes) {
        ol.appendChild(el('li', null, n.label));
      }
    }

    const textAlt = el(
      'details',
      { part: 'text-alt' },
      el('summary', null, 'Ver como texto'),
      ol
    );

    figure.appendChild(toolbar);
    figure.appendChild(viewport);
    figure.appendChild(textAlt);
    this.shadowRoot.appendChild(figure);
  }
}

if (!customElements.get('dx-diagram-block')) {
  customElements.define('dx-diagram-block', DxDiagramBlock);
}
