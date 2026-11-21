// ==========================================================================
// DioptraEngine Viewer — ui/components/dx-report.js
// <dx-report> (§2.1 Shell — Renderizador recursivo de los 9 bloques dx-*)
// ==========================================================================

import {
  clearNode,
  createSeverityIcon,
  createUiIcon,
  el,
  normalizeReportEnvelope,
  SEV_RANK,
  SEV_SET,
  svgEl
} from './shared.js';

// Texto legible del sello de severidad máxima (claves = SEV_SET de shared.js).
const SEAL_LABELS = Object.freeze({
  error: 'Crítico',
  warning: 'Advertencia',
  info: 'Informativo',
  success: 'Correcto'
});

function createPlateGem() {
  return svgEl(
    'svg',
    {
      class: 'dx-plate-gem',
      viewBox: '0 0 10 10',
      width: '10',
      height: '10',
      'aria-hidden': 'true',
      focusable: 'false'
    },
    svgEl('polygon', { points: '5,0 10,5 5,10 0,5', fill: 'currentColor' })
  );
}

const TAG_BY_BLOCK_TYPE = Object.freeze({
  kpi: 'dx-kpi-grid',
  callout: 'dx-severity-callout',
  table: 'dx-data-table',
  section: 'dx-collapsible-section',
  code: 'dx-code-view',
  diagram: 'dx-diagram-block',
  checklist: 'dx-action-checklist',
  prose: 'dx-prose-block',
  canvas: 'dx-free-canvas'
});

function formatFullTimestamp(isoStr) {
  const d = new Date(isoStr);
  if (Number.isNaN(d.getTime())) return String(isoStr ?? '');
  return d.toLocaleString('es-MX', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  });
}

function formatTimeHHMMSS(isoStr) {
  const d = new Date(isoStr);
  if (Number.isNaN(d.getTime())) return String(isoStr ?? '');
  return d.toLocaleTimeString('es-MX', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  });
}

// Clave de contenido de un bloque normalizado: dos bloques con la misma clave son
// visualmente idénticos y el nodo existente puede reutilizarse.
function blockKey(blockSpec) {
  try {
    return `${blockSpec.t}:${JSON.stringify(blockSpec.data)}`;
  } catch {
    return `${blockSpec.t}:${Math.random()}`;
  }
}

export function instantiateBlockTree(blockSpec) {
  const tagName = TAG_BY_BLOCK_TYPE[blockSpec.t];
  if (!tagName) return null;

  const element = document.createElement(tagName);
  if (blockSpec.t === 'section' && Array.isArray(blockSpec.data?.blocks)) {
    for (const childSpec of blockSpec.data.blocks) {
      const childEl = instantiateBlockTree(childSpec);
      if (childEl) element.appendChild(childEl);
    }
  }
  element.data = blockSpec.data;
  return element;
}

export class DxReport extends HTMLElement {
  constructor() {
    super();
    this._envelope = null;
    this._theme = 'dark';
    this._themeBtn = null;
    this._lastResult = null;
    this._checkState = new Map();
    this._progressEl = null;
    this._progressLabelEl = null;
    // Los checklists emiten `dx-check` (bubbles+composed) al marcar una casilla;
    // se actualiza solo la barra de progreso, sin re-crear la cabecera.
    this.addEventListener('dx-check', (e) => {
      const id = e.detail?.id;
      if (typeof id !== 'string' || !this._checkState.has(id)) return;
      this._checkState.set(id, Boolean(e.detail?.done));
      this._updateProgress();
    });
  }

  _updateProgress() {
    if (!this._progressEl || !this._progressLabelEl) return;
    const total = this._checkState.size;
    let done = 0;
    for (const v of this._checkState.values()) if (v) done += 1;
    this._progressEl.max = total;
    this._progressEl.value = done;
    this._progressLabelEl.textContent = `${done}/${total}`;
  }

  get envelope() {
    return this._envelope;
  }

  set envelope(val) {
    this._envelope = val;
    this.render();
  }

  setThemeIndicator(theme) {
    this._theme = theme === 'light' ? 'light' : 'dark';
    if (this._themeBtn) {
      clearNode(this._themeBtn);
      const isDark = this._theme === 'dark';
      this._themeBtn.appendChild(createUiIcon(isDark ? 'sun' : 'moon', 15));
      this._themeBtn.title = isDark ? 'Tema claro' : 'Tema oscuro';
      this._themeBtn.setAttribute(
        'aria-label',
        isDark ? 'Cambiar a tema claro' : 'Cambiar a tema oscuro'
      );
    }
  }

  get warnings() {
    return this._lastResult?.warnings || [];
  }

  render() {
    // Render con claves: si se vuelve a pintar el MISMO reporte (SSE, revisión
    // nueva), los bloques cuyo contenido no cambió se reutilizan tal cual, y así
    // conservan su estado (sección abierta, orden/filtro de tabla, zoom del
    // diagrama). También se conserva la posición de lectura.
    const prevId = this._renderedId;
    const prevScroll = this.scrollTop;
    const reusable = new Map();
    for (const { key, node } of this._renderedBlocks || []) {
      if (!reusable.has(key)) reusable.set(key, []);
      reusable.get(key).push(node);
    }

    clearNode(this);
    this._renderedBlocks = [];
    if (!this._envelope) return;

    const normalized = normalizeReportEnvelope(this._envelope);
    this._lastResult = normalized;

    if (!normalized.ok) {
      this.appendChild(el('div', { className: 'dx-report-error', role: 'alert' }, normalized.error));
      return;
    }

    const { report, warnings } = normalized;
    this._theme = document.documentElement.dataset.theme === 'light' ? 'light' : 'dark';

    this._themeBtn = el('button', { type: 'button', className: 'dx-theme-btn' });
    this.setThemeIndicator(this._theme);
    this._themeBtn.addEventListener('click', () => {
      this.dispatchEvent(new CustomEvent('dx-request-theme-toggle', {
        bubbles: true,
        composed: true
      }));
    });

    const actions = el('div', { className: 'dx-report-actions' });
    if (warnings.length > 0) {
      const summaryLines = warnings
        .map((w) => `[${w.action}] ${w.path}: ${w.reason}`)
        .join('\n');
      actions.appendChild(
        el(
          'span',
          { className: 'dx-warnings-pill', title: summaryLines },
          createSeverityIcon('warning', 15),
          `${warnings.length} ${warnings.length === 1 ? 'aviso' : 'avisos'}`
        )
      );
    }
    actions.appendChild(this._themeBtn);

    const isUpdated = Number(report.revision) > 1 && Boolean(report.updatedAt);

    // normalizeReportEnvelope no propaga `maxSev` ni `model`: se leen del sobre crudo
    // y se validan; nunca se inventan.
    const rawReport = this._envelope?.report || {};
    const rawModel = typeof rawReport.model === 'string' ? rawReport.model.trim() : '';
    // Severidad sobre bloques crudos: sin `sev` explícito → null → sin sello.
    const maxSev = SEV_SET.has(rawReport.maxSev)
      ? rawReport.maxSev
      : computeMaxSeverity(rawReport.blocks);

    // Ruta de contexto: workspace › fecha · rev · modelo. Una línea ligera sobre
    // el título; el workspace es lo que agrupa los reportes en la barra lateral.
    const sep = () => el('span', { className: 'dx-crumb-sep', 'aria-hidden': 'true' }, '·');
    const plate = el(
      'div',
      { className: 'dx-report-plate dx-report-crumbs' },
      createPlateGem(),
      el('span', { className: 'dx-plate-project', title: 'Workspace' }, report.project),
      el('span', { className: 'dx-crumb-arrow', 'aria-hidden': 'true' }, '›'),
      el('time', { className: 'dx-plate-date', datetime: report.ts }, formatFullTimestamp(report.ts)),
      isUpdated ? sep() : null,
      isUpdated
        ? el('span', { className: 'dx-report-rev-inline dx-plate-rev', title: `Actualizado ${formatFullTimestamp(report.updatedAt)}` }, `rev ${report.revision}`)
        : null,
      rawModel ? sep() : null,
      rawModel ? el('span', { className: 'dx-plate-model', title: 'Modelo' }, rawModel) : null
    );

    const seal = maxSev && Object.prototype.hasOwnProperty.call(SEAL_LABELS, maxSev)
      ? el(
        'div',
        {
          className: 'dx-report-seal',
          dataset: { sev: maxSev },
          role: 'img',
          'aria-label': `Severidad máxima: ${SEAL_LABELS[maxSev]}`
        },
        createSeverityIcon(maxSev, 16),
        el('span', { className: 'dx-report-seal-label' }, SEAL_LABELS[maxSev])
      )
      : null;

    const progress = collectChecklistProgress(report.blocks);
    this._checkState = new Map();
    this._progressEl = null;
    this._progressLabelEl = null;
    let progressWrap = null;
    if (progress.total > 0) {
      progress.items.forEach((it, idx) => {
        this._checkState.set(it.id || `__chk_${idx}`, it.done);
      });
      this._progressEl = el('progress', {
        className: 'dx-progress',
        max: String(progress.total),
        value: String(progress.done),
        'aria-label': 'Progreso del checklist'
      });
      this._progressLabelEl = el('span', { className: 'dx-progress-label' }, `${progress.done}/${progress.total}`);
      progressWrap = el('div', { className: 'dx-report-progress' }, this._progressEl, this._progressLabelEl);
      this._updateProgress();
    }

    const status = el('div', { className: 'dx-report-status' }, seal, progressWrap);
    if (status.childNodes.length) actions.prepend(status);
    const header = el(
      'header',
      { className: 'dx-report-header' },
      el(
        'div',
        { className: 'dx-report-meta-group' },
        plate,
        el('h1', { className: 'dx-report-title dx-display', title: report.title }, report.title)
      ),
      actions
    );

    const body = el('main', { className: 'dx-report-body' });
    if (isUpdated) {
      const updateBarChildren = [
        el('span', { className: 'dx-update-icon' }, createUiIcon('history', 14)),
        el('span', { className: 'dx-update-label' }, 'Última actualización'),
        el('time', {
          className: 'dx-update-time',
          datetime: report.updatedAt,
          title: formatFullTimestamp(report.updatedAt)
        }, formatTimeHHMMSS(report.updatedAt)),
        el('span', { className: 'dx-update-rev' }, `rev ${report.revision}`)
      ];
      if (report.lastChange) {
        updateBarChildren.push(
          el('span', { className: 'dx-update-sep', 'aria-hidden': 'true' }, '—'),
          el('span', { className: 'dx-update-note', title: report.lastChange }, report.lastChange)
        );
      }
      body.appendChild(el('aside', {
        className: 'dx-update-bar',
        role: 'status',
        'aria-label': 'Última actualización del reporte'
      }, ...updateBarChildren));
    }

    const sameReport = prevId !== undefined && prevId === report.id;
    for (const blockSpec of report.blocks) {
      const key = blockKey(blockSpec);
      const pool = sameReport ? reusable.get(key) : null;
      const node = pool && pool.length ? pool.shift() : instantiateBlockTree(blockSpec);
      if (node) {
        body.appendChild(node);
        this._renderedBlocks.push({ key, node });
      }
    }

    // Índice del reporte: con 3+ secciones de primer nivel, una fila de enlaces
    // que abre la sección elegida y la lleva a la vista.
    const sections = this._renderedBlocks
      .map(({ node }) => node)
      .filter((n) => n.tagName === 'DX-COLLAPSIBLE-SECTION' && n.data?.title);
    if (sections.length >= 3) {
      const toc = el('nav', { className: 'dx-report-toc', 'aria-label': 'Índice del reporte' });
      toc.appendChild(el('span', { className: 'dx-report-toc-label' }, 'Índice'));
      const list = el('ol', { className: 'dx-report-toc-list' });
      sections.forEach((sec, i) => {
        const btn = el('button', { type: 'button', className: 'dx-report-toc-link' },
          el('span', { className: 'dx-report-toc-num' }, String(i + 1)),
          String(sec.data.title));
        btn.addEventListener('click', () => {
          sec.setAttribute('open', '');
          sec.scrollIntoView({ block: 'start' });
        });
        list.appendChild(el('li', null, btn));
      });
      toc.appendChild(list);
      const firstBlock = this._renderedBlocks[0]?.node;
      body.insertBefore(toc, firstBlock && firstBlock.parentNode === body ? firstBlock : null);
    }

    this.appendChild(header);
    this.appendChild(body);
    this._renderedId = report.id;
    if (sameReport) {
      // Dos pasadas: inmediata y tras el layout (los SVG de diagramas cambian el alto).
      this.scrollTop = prevScroll;
      requestAnimationFrame(() => { this.scrollTop = prevScroll; });
    } else {
      this.scrollTop = 0;
    }
  }
}

if (!customElements.get('dx-report')) {
  customElements.define('dx-report', DxReport);
}

/**
 * Función pura: recorre recursivamente los bloques (forma normalizada `{ t, data }`
 * y forma cruda `{ t, items, blocks }`) y devuelve los ítems de checklist
 * con su estado `done`, incluyendo los anidados en `section`.
 * @param {Array} blocks
 * @returns {{done:number,total:number,items:Array<{id:string,done:boolean}>}}
 */
export function collectChecklistProgress(blocks) {
  let done = 0;
  let total = 0;
  const items = [];
  const seenIds = new Set();
  const walk = (list) => {
    if (!Array.isArray(list)) return;
    for (const b of list) {
      if (!b || typeof b !== 'object') continue;
      for (const payload of blockPayloads(b)) {
        if (b.t === 'checklist' && Array.isArray(payload.items)) {
          for (const it of payload.items) {
            if (!it || typeof it !== 'object') continue;
            const id = typeof it.id === 'string' && it.id ? it.id : '';
            // Deduplica solo si hay id; sin id cuenta por posición.
            if (id) {
              if (seenIds.has(id)) continue;
              seenIds.add(id);
            }
            const isDone = it.done === true;
            total += 1;
            if (isDone) done += 1;
            items.push({ id, done: isDone });
          }
        }
        for (const child of childLists(payload)) walk(child);
      }
    }
  };
  walk(blocks);
  return { done, total, items };
}

/**
 * Payloads de un bloque: normalizado (`b.data`, ver shared.js normalizeBlock
 * → section `{ t, data: { blocks } }`) y crudo (`b` mismo). Si ambos existen
 * se recorren los dos (sin duplicar objeto).
 */
function blockPayloads(b) {
  const out = [];
  if (b.data && typeof b.data === 'object' && !Array.isArray(b.data)) out.push(b.data);
  out.push(b);
  return out;
}

function childLists(payload) {
  const out = [];
  if (Array.isArray(payload.blocks)) out.push(payload.blocks);
  if (Array.isArray(payload.children)) out.push(payload.children);
  return out;
}

/**
 * Severidad máxima derivada de los bloques (fallback cuando `report.maxSev`
 * no viene en el sobre). Considera `sev` de bloques/sections/callouts y de
 * ítems (kpi/checklist), validado contra SEV_SET y ordenado por SEV_RANK.
 * @param {Array} blocks
 * Debe recibir bloques CRUDOS (antes de normalizar): `sev` ausente = sin
 * severidad, así el default de normalización no fabrica un "Informativo".
 * Helper único compartido con dx-app.js (timeline).
 * @returns {string|null} sev o null si ningún bloque/ítem trae sev explícito
 */
export function computeMaxSeverity(blocks) {
  let best = null;
  const consider = (sev) => {
    if (typeof sev !== 'string' || !SEV_SET.has(sev)) return;
    if (!best || (SEV_RANK[sev] || 0) > (SEV_RANK[best] || 0)) best = sev;
  };
  const walk = (list) => {
    if (!Array.isArray(list)) return;
    for (const b of list) {
      if (!b || typeof b !== 'object') continue;
      for (const payload of blockPayloads(b)) {
        consider(payload.sev);
        if (Array.isArray(payload.items)) {
          for (const it of payload.items) {
            if (it && typeof it === 'object') consider(it.sev);
          }
        }
        for (const child of childLists(payload)) walk(child);
      }
    }
  };
  walk(blocks);
  return best;
}
