// ==========================================================================
// DioptraEngine Viewer — ui/components/canvas-annotator.js
// Capa de anotación visual CSP-safe (Pin, Caja y Marcador) en coordenadas
// normalizadas 0..1 sobre pantallas de <dx-design-canvas>.
// ==========================================================================

import { clearNode, el, svgEl } from './shared.js';

let probeSeq = 0;

/**
 * Consulta de solo lectura al iframe (origen opaco sandbox="allow-scripts") qué
 * elemento HTML está bajo el punto normalizado (nx, ny) ∈ [0, 1].
 */
export function probeIframeElement(iframe, nx, ny, timeoutMs = 160) {
  return new Promise((resolve) => {
    if (!iframe?.contentWindow) {
      resolve({ selector: '', tag: '', text: '' });
      return;
    }
    const reqId = `probe-${++probeSeq}-${Date.now()}`;
    let settled = false;
    const finish = (val) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      window.removeEventListener('message', onMsg);
      resolve(val);
    };
    const onMsg = (e) => {
      if (e.source !== iframe.contentWindow) return;
      const hit = e.data?.dxProbeHit;
      if (!hit || hit.reqId !== reqId) return;
      finish({
        selector: typeof hit.selector === 'string' ? hit.selector : '',
        tag: typeof hit.tag === 'string' ? hit.tag : '',
        text: typeof hit.text === 'string' ? hit.text : ''
      });
    };
    const timer = setTimeout(() => finish({ selector: '', tag: '', text: '' }), timeoutMs);
    window.addEventListener('message', onMsg);
    try {
      iframe.contentWindow.postMessage({ dxProbePoint: { reqId, x: nx, y: ny } }, '*');
    } catch {
      finish({ selector: '', tag: '', text: '' });
    }
  });
}

function clamp01(v) {
  return Math.max(0, Math.min(1, Number.isFinite(v) ? v : 0));
}

function round4(v) {
  return Math.round(clamp01(v) * 10000) / 10000;
}

function anchorOf(ann) {
  const g = ann?.geometry || {};
  if (ann.kind === 'box') {
    return { x: clamp01((g.x || 0) + (g.w || 0)), y: clamp01(g.y || 0) };
  }
  if (ann.kind === 'highlight' && Array.isArray(g.points) && g.points.length > 0) {
    const first = g.points[0];
    return { x: clamp01(first[0]), y: clamp01(first[1]) };
  }
  return { x: clamp01(g.x), y: clamp01(g.y) };
}

/**
 * Monta un overlay interactivo encima de un board (en modo Lienzo o Prototipo).
 */
export class BoardAnnotationOverlay {
  constructor({ host, iframe, boardFile, pageId, getTool, onDraftReady, onSelectAnnotation }) {
    this.host = host;
    this.iframe = iframe;
    this.boardFile = boardFile;
    this.pageId = pageId || '';
    this.getTool = getTool;
    this.onDraftReady = onDraftReady;
    this.onSelectAnnotation = onSelectAnnotation;

    this.root = el('div', { className: 'dc-ann-overlay', dataset: { tool: 'interact' } });
    this.svg = svgEl('svg', { class: 'dc-ann-svg', viewBox: '0 0 1000 1000', preserveAspectRatio: 'none', 'aria-hidden': 'true' });
    this.pinsLayer = el('div', { className: 'dc-ann-pins' });
    this.draftSvg = svgEl('svg', { class: 'dc-ann-draft-svg', viewBox: '0 0 1000 1000', preserveAspectRatio: 'none', 'aria-hidden': 'true' });
    this.root.append(this.svg, this.draftSvg, this.pinsLayer);
    this.host.appendChild(this.root);

    this._annotations = [];
    this._selectedId = '';
    this._wireDrawing();
  }

  setTool(tool) {
    this.root.dataset.tool = tool || 'interact';
    clearNode(this.draftSvg);
  }

  setAnnotations(list, indexMap, selectedId = '') {
    this._annotations = Array.isArray(list) ? list : [];
    this._selectedId = selectedId;
    this._renderMarks(indexMap);
  }

  clearDraft() {
    clearNode(this.draftSvg);
  }

  _renderMarks(indexMap) {
    clearNode(this.svg);
    clearNode(this.pinsLayer);

    for (const ann of this._annotations) {
      const num = indexMap?.get(ann.id) ?? '?';
      const status = ann.status || 'open';
      const selected = ann.id === this._selectedId;
      const g = ann.geometry || {};

      if (ann.kind === 'box') {
        const x = clamp01(g.x) * 1000;
        const y = clamp01(g.y) * 1000;
        const w = Math.max(8, clamp01(g.w) * 1000);
        const h = Math.max(8, clamp01(g.h) * 1000);
        this.svg.appendChild(svgEl('rect', {
          x: x.toFixed(1),
          y: y.toFixed(1),
          width: w.toFixed(1),
          height: h.toFixed(1),
          rx: '10',
          class: `dc-mark-box dc-mark-${status}${selected ? ' dc-mark-selected' : ''}`
        }));
      } else if (ann.kind === 'highlight' && Array.isArray(g.points) && g.points.length >= 2) {
        const d = g.points
          .map((pt, i) => `${i === 0 ? 'M' : 'L'} ${(clamp01(pt[0]) * 1000).toFixed(1)} ${(clamp01(pt[1]) * 1000).toFixed(1)}`)
          .join(' ');
        this.svg.appendChild(svgEl('path', {
          d,
          class: `dc-mark-highlight dc-mark-${status}${selected ? ' dc-mark-selected' : ''}`
        }));
      }

      const pos = anchorOf(ann);
      const pin = el('button', {
        type: 'button',
        className: 'dc-pin',
        dataset: { status, selected: selected ? 'true' : 'false', annId: ann.id },
        title: `#${num} (${status}): ${ann.comment}`,
        'aria-label': `Comentario ${num}: ${ann.comment}`
      }, String(num));
      pin.style.setProperty('--px', `${(pos.x * 100).toFixed(2)}%`);
      pin.style.setProperty('--py', `${(pos.y * 100).toFixed(2)}%`);
      pin.addEventListener('click', (e) => {
        e.stopPropagation();
        this.onSelectAnnotation?.(ann);
      });
      this.pinsLayer.appendChild(pin);
    }
  }

  _normPoint(e) {
    const rect = this.root.getBoundingClientRect();
    const x = rect.width > 0 ? (e.clientX - rect.left) / rect.width : 0.5;
    const y = rect.height > 0 ? (e.clientY - rect.top) / rect.height : 0.5;
    return { x: round4(x), y: round4(y) };
  }

  _wireDrawing() {
    let active = false;
    let tool = 'interact';
    let start = null;
    let points = [];

    this.root.addEventListener('pointerdown', (e) => {
      tool = this.getTool?.() || 'interact';
      if (tool === 'interact' || e.button !== 0) return;
      if (e.target.closest?.('.dc-pin')) return;
      e.preventDefault();
      e.stopPropagation();
      active = true;
      start = this._normPoint(e);
      points = [[start.x, start.y]];
      this.root.setPointerCapture(e.pointerId);
      clearNode(this.draftSvg);

      if (tool === 'pin') {
        this.draftSvg.appendChild(svgEl('circle', {
          cx: (start.x * 1000).toFixed(1),
          cy: (start.y * 1000).toFixed(1),
          r: '16',
          class: 'dc-draft-pin'
        }));
      }
    });

    this.root.addEventListener('pointermove', (e) => {
      if (!active) return;
      const cur = this._normPoint(e);
      clearNode(this.draftSvg);

      if (tool === 'box') {
        const x = Math.min(start.x, cur.x) * 1000;
        const y = Math.min(start.y, cur.y) * 1000;
        const w = Math.max(4, Math.abs(cur.x - start.x) * 1000);
        const h = Math.max(4, Math.abs(cur.y - start.y) * 1000);
        this.draftSvg.appendChild(svgEl('rect', {
          x: x.toFixed(1),
          y: y.toFixed(1),
          width: w.toFixed(1),
          height: h.toFixed(1),
          rx: '10',
          class: 'dc-draft-box'
        }));
      } else if (tool === 'highlight') {
        const last = points[points.length - 1];
        if (!last || Math.hypot(cur.x - last[0], cur.y - last[1]) >= 0.008) {
          if (points.length < 140) points.push([cur.x, cur.y]);
        }
        const d = points
          .map((pt, i) => `${i === 0 ? 'M' : 'L'} ${(pt[0] * 1000).toFixed(1)} ${(pt[1] * 1000).toFixed(1)}`)
          .join(' ');
        this.draftSvg.appendChild(svgEl('path', { d, class: 'dc-draft-highlight' }));
      }
    });

    const finish = async (e) => {
      if (!active) return;
      active = false;
      try {
        if (this.root.hasPointerCapture(e.pointerId)) this.root.releasePointerCapture(e.pointerId);
      } catch {
        // noop
      }
      const end = this._normPoint(e);
      let geometry = null;
      let probeX = end.x;
      let probeY = end.y;
      let kind = tool;

      if (tool === 'pin') {
        geometry = { x: start.x, y: start.y };
        probeX = start.x;
        probeY = start.y;
      } else if (tool === 'box') {
        const x = round4(Math.min(start.x, end.x));
        const y = round4(Math.min(start.y, end.y));
        const w = round4(Math.abs(end.x - start.x));
        const h = round4(Math.abs(end.y - start.y));
        if (w < 0.015 && h < 0.015) {
          // Clic breve en modo caja → tratar como pin puntual.
          kind = 'pin';
          geometry = { x: start.x, y: start.y };
          probeX = start.x;
          probeY = start.y;
        } else {
          geometry = { x, y, w, h };
          probeX = round4(x + w / 2);
          probeY = round4(y + h / 2);
        }
      } else if (tool === 'highlight') {
        if (points.length < 2) {
          points.push([round4(Math.min(1, start.x + 0.03)), start.y]);
        }
        geometry = { points, strokeWidth: 0.026 };
        const mid = points[Math.floor(points.length / 2)];
        probeX = mid[0];
        probeY = mid[1];
      } else {
        return;
      }

      const hit = await probeIframeElement(this.iframe, probeX, probeY);
      this.onDraftReady?.({
        boardFile: this.boardFile,
        pageId: this.pageId,
        kind,
        geometry,
        targetSelector: hit.selector,
        targetText: hit.text,
        clearDraft: () => this.clearDraft()
      });
    };

    this.root.addEventListener('pointerup', finish);
    this.root.addEventListener('pointercancel', () => {
      active = false;
      clearNode(this.draftSvg);
    });
  }
}
