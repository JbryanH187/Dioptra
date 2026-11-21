// ==========================================================================
// DioptraEngine Viewer — ui/components/data-table.js
// <dx-data-table> (§4.3 Contrato A — Ordenable, filtrable y paginada >500)
// ==========================================================================

import {
  adoptSheets,
  baseBlockSheet,
  clearNode,
  createRefButton,
  createSeverityIcon,
  createSheet,
  el,
  renderHighlightedText,
  SEV_LABELS,
  SEV_RANK,
  SEV_SET,
  svgEl,
  createUiIcon,
} from './shared.js';

const PAGE_SIZE = 200;
const PAGINATION_THRESHOLD = 500;

const tableSheet = createSheet(`
  :host {
    padding: var(--spacing-4);
    filter: none;
  }

  /* El hijo ya es la losa: sin marco base duplicado */
  :host::before {
    display: none;
  }

  [part="toolbar"] {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--spacing-3);
    margin-bottom: var(--spacing-3);
  }

  .filter-field {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--spacing-2);
    flex: 1 1 280px;
    max-width: 420px;
    height: var(--row-height);
    padding: 0 var(--spacing-3);
    background: var(--color-surface-sunken);
    border: 2px solid var(--color-ink);
    border-radius: var(--rounded-sm);
    /* Buscador hundido: bisel invertido (sombra interior arriba, luz abajo) */
    box-shadow: inset 0 2px 0 var(--color-ink-shadow), inset 0 -1px 0 var(--color-stone-steel);
    color: var(--color-text-soft);
    transition: border-color var(--motion-duration-fast) var(--motion-easing-standard),
                box-shadow var(--motion-duration-fast) var(--motion-easing-standard),
                background-color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  .filter-field:hover {
    border-color: var(--color-ink);
    background: var(--color-stone-abyss);
  }

  .filter-field > svg {
    flex-shrink: 0;
    color: var(--color-text-soft);
    transition: color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  .filter-field:focus-within {
    border-color: var(--color-ink);
    box-shadow: inset 0 2px 0 var(--color-ink-shadow), var(--focus-ring);
  }

  .filter-field:focus-within > svg {
    color: var(--color-accent-fg);
  }

  [part="filter"] {
    flex: 1 1 auto;
    min-width: 0;
    height: 100%;
    padding: 0;
    background: transparent;
    border: 0;
    outline: none;
    color: var(--color-text-primary);
    font-size: var(--fs-small);
  }

  [part="filter"]::placeholder {
    color: var(--color-text-soft);
  }

  /* El anillo lo pinta .filter-field:focus-within (bisel hundido + var(--focus-ring)) */
  [part="filter"]:focus-visible {
    box-shadow: none;
  }

  [part="count"] {
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    font-variant-numeric: tabular-nums;
    font-variant-ligatures: none;
    color: var(--color-text-soft);
    white-space: nowrap;
  }

  [part="scroller"] {
    overflow: auto;
    max-height: 70vh;
    /* Losa Reliquia: contorno de tinta + sombra dura desplazada (sin blur) */
    border: 2px solid var(--color-ink);
    border-radius: var(--rounded-sm);
    background: var(--color-surface-base);
    filter: drop-shadow(3px 4px 0 var(--color-ink-shadow));
    /* overflow:auto + border-radius ya recorta thead sticky y hover a los radios */
    overscroll-behavior-x: contain;
  }

  [part="table"] {
    width: 100%;
    border-collapse: separate;
    border-spacing: 0;
    text-align: left;
  }

  thead {
    position: sticky;
    top: 0;
    z-index: var(--z-sticky);
    background: var(--color-stone-slab);
  }

  /* Cabecera tallada: gradiente acero→losa, bisel superior, filete de tinta */
  th {
    height: var(--row-height);
    padding: 0;
    background: linear-gradient(180deg, var(--color-stone-steel), var(--color-stone-slab));
    border-bottom: 2px solid var(--color-ink);
    box-shadow: inset 0 1px 0 var(--color-bronze-light);
    font-family: var(--font-display);
    font-size: var(--fs-caption);
    font-weight: var(--fw-semibold);
    font-variant: small-caps;
    color: var(--color-accent-fg);
    text-transform: none;
    letter-spacing: .08em;
    white-space: nowrap;
  }

  th[data-type="num"] {
    text-align: right;
  }

  th > button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    height: var(--row-height);
    padding: 0 var(--spacing-3);
    background: transparent;
    border: 0;
    color: inherit;
    font: inherit;
    text-transform: inherit;
    letter-spacing: inherit;
    cursor: pointer;
    text-align: left;
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard),
                color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  th[data-type="num"] > button {
    justify-content: flex-end;
  }

  th > button:hover {
    background-color: var(--color-row-hover-bg);
    color: var(--color-accent-fg);
  }

  th > button:focus-visible {
    outline: none;
    box-shadow: var(--focus-ring);
  }

  th[aria-sort="ascending"] > button,
  th[aria-sort="descending"] > button {
    color: var(--color-accent-fg);
  }

  /* Indicador de orden: un único glifo que rota (asc 180°, desc 0°) */
  .sort-ind {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 12px;
    height: 12px;
    flex-shrink: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    line-height: 1;
    letter-spacing: 0;
    opacity: 0;
    transform: rotate(0deg);
    transition: transform var(--motion-duration-normal) var(--motion-easing-emphasized),
                opacity var(--motion-duration-fast) var(--motion-easing-standard);
  }

  th > button:hover .sort-ind {
    opacity: 0.6;
  }

  th[aria-sort="descending"] .sort-ind,
  th[aria-sort="ascending"] .sort-ind {
    opacity: 1;
  }

  th[aria-sort="ascending"] .sort-ind {
    transform: rotate(180deg);
  }

  tbody tr {
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  tbody tr:hover > td {
    background-color: var(--color-row-hover-bg);
  }

  /* Fila seleccionada (solo estilo; no altera lógica): tinte + filete cobre interior */
  tbody tr[aria-selected="true"] > td,
  tbody tr[data-selected="true"] > td {
    background-color: var(--color-row-selected-bg);
    box-shadow: inset 0 1px 0 var(--color-copper), inset 0 -1px 0 var(--color-copper);
  }
  tbody tr[aria-selected="true"] > td:first-child,
  tbody tr[data-selected="true"] > td:first-child {
    box-shadow: inset 0 1px 0 var(--color-copper), inset 0 -1px 0 var(--color-copper),
                inset 1px 0 0 var(--color-copper);
  }
  tbody tr[aria-selected="true"] > td:last-child,
  tbody tr[data-selected="true"] > td:last-child {
    box-shadow: inset 0 1px 0 var(--color-copper), inset 0 -1px 0 var(--color-copper),
                inset -1px 0 0 var(--color-copper);
  }
  tbody tr[aria-selected="true"] > td:only-child,
  tbody tr[data-selected="true"] > td:only-child {
    box-shadow: inset 0 0 0 1px var(--color-copper);
  }

  td {
    height: var(--row-height);
    padding: var(--spacing-1) var(--spacing-3);
    background: var(--color-surface-base);
    border-bottom: 1px solid var(--color-stone-abyss);
    font-size: var(--fs-small);
    font-variant-numeric: tabular-nums;
    vertical-align: middle;
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  tbody tr:last-child td {
    border-bottom: 0;
  }

  td[data-type="num"] {
    text-align: right;
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    font-variant-numeric: tabular-nums;
    font-variant-ligatures: none;
    color: var(--color-text-primary);
  }

  mark {
    background-color: var(--color-match-bg);
    color: inherit;
    border-radius: var(--rounded-xs);
    padding: 0 2px;
    box-decoration-break: clone;
    -webkit-box-decoration-break: clone;
  }

  /* Pastilla de severidad (spec §1): tinte + borde + icono + texto (nunca solo color) */
  [part="badge"] {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 1px 10px 1px 8px;
    border: var(--border-width) solid var(--color-border-translucent);
    border-radius: var(--rounded-sm);
    font-size: var(--fs-caption);
    font-weight: var(--fw-semibold);
    line-height: 1.4;
    white-space: nowrap;
  }

  /* Icono de severidad 13px; hereda el color sev-fg de la pastilla vía currentColor */
  [part="badge"] > svg {
    display: block;
    width: 13px;
    height: 13px;
    flex-shrink: 0;
    color: inherit;
  }

  [part="badge"][data-sev="info"] {
    background: var(--color-sev-info-bg);
    border-color: var(--color-sev-info-border);
    color: var(--color-sev-info-fg);
  }
  [part="badge"][data-sev="success"] {
    background: var(--color-sev-success-bg);
    border-color: var(--color-sev-success-border);
    color: var(--color-sev-success-fg);
  }
  [part="badge"][data-sev="warning"] {
    background: var(--color-sev-warning-bg);
    border-color: var(--color-sev-warning-border);
    color: var(--color-sev-warning-fg);
  }
  [part="badge"][data-sev="error"] {
    background: var(--color-sev-error-bg);
    border-color: var(--color-sev-error-border);
    color: var(--color-sev-error-fg);
  }

  tbody tr:has(> .empty-cell):hover > td {
    background-color: var(--color-surface-base);
  }

  .empty-cell {
    height: auto;
    text-align: center;
    padding: var(--spacing-6) var(--spacing-3);
    color: var(--color-text-soft);
    font-size: var(--fs-small);
  }

  .empty-cell > svg {
    display: inline-block;
    vertical-align: -3px;
    margin-right: var(--spacing-2);
    color: var(--color-text-soft);
  }

  [part="pagination"] {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--spacing-2);
    margin-top: var(--spacing-3);
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    font-variant-numeric: tabular-nums;
    color: var(--color-text-soft);
  }

  /* Sellos de piedra: contorno tinta, bisel, sombra dura 2px; al pulsar se hunden */
  [part="pagination"] button {
    height: var(--row-height);
    padding: 0 var(--spacing-3);
    border-radius: var(--rounded-sm);
    border: 2px solid var(--color-ink);
    background: linear-gradient(180deg, var(--color-stone-steel), var(--color-stone-slab));
    box-shadow: inset 0 1px 0 var(--color-bronze-light), 2px 2px 0 var(--color-ink-shadow);
    color: var(--color-text-primary);
    font-family: var(--font-display);
    font-variant: small-caps;
    letter-spacing: .06em;
    cursor: pointer;
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard),
                transform var(--motion-duration-instant) var(--motion-easing-standard),
                box-shadow var(--motion-duration-instant) var(--motion-easing-standard);
  }

  [part="pagination"] button:hover:not([disabled]) {
    background: var(--color-stone-steel);
    color: var(--color-accent-fg);
  }

  [part="pagination"] button:active:not([disabled]) {
    transform: translate(1px, 1px);
    box-shadow: inset 0 1px 0 var(--color-bronze-light), 1px 1px 0 var(--color-ink-shadow);
  }

  [part="pagination"] button:focus-visible {
    outline: none;
    box-shadow: inset 0 1px 0 var(--color-bronze-light), var(--focus-ring);
  }

  [part="pagination"] button[disabled] {
    color: var(--color-text-disabled);
    background: var(--color-stone-obsidian);
    box-shadow: none;
    cursor: not-allowed;
  }

  @media (prefers-reduced-motion: reduce) {
    .filter-field,
    .filter-field > svg,
    th > button,
    .sort-ind,
    tbody tr,
    td,
    [part="pagination"] button {
      transition: none;
    }
    [part="pagination"] button:active:not([disabled]) {
      transform: none;
    }
  }

  @media (forced-colors: active) {
    [part="scroller"] {
      filter: none;
      border-color: CanvasText;
    }
    th {
      background: Canvas;
      color: CanvasText;
      border-bottom-color: CanvasText;
      box-shadow: none;
    }
    td {
      background: Canvas;
      border-bottom-color: GrayText;
    }
    tbody tr[aria-selected="true"] > td,
    tbody tr[data-selected="true"] > td {
      background: Highlight;
      color: HighlightText;
      box-shadow: none;
      forced-color-adjust: none;
    }
    .filter-field,
    [part="pagination"] button {
      border-color: ButtonText;
      box-shadow: none;
    }
    .filter-field:focus-within,
    th > button:focus-visible,
    [part="pagination"] button:focus-visible {
      outline: 2px solid Highlight;
      outline-offset: 2px;
    }
    [part="pagination"] button[disabled] {
      color: GrayText;
      border-color: GrayText;
    }
  }
`);

let tableInstanceSeq = 0;

export class DxDataTable extends HTMLElement {
  constructor() {
    super();
    this._data = null;
    this._query = '';
    this._sortKey = '';
    this._sortDir = 'desc';
    this._page = 0;
    this._debounceTimer = null;
    this._tableId = `dx-tbl-${++tableInstanceSeq}`;
    this._tbody = null;
    this._theadRow = null;
    this._countOut = null;
    this._pagerWrap = null;
    this.attachShadow({ mode: 'open' });
    adoptSheets(this.shadowRoot, baseBlockSheet, tableSheet);
  }

  get data() {
    return this._data;
  }

  set data(val) {
    this._data = val;
    this._query = '';
    this._page = 0;
    const cols = Array.isArray(val?.cols) ? val.cols : [];
    const initialSort = typeof val?.sort === 'string' && cols.some((c) => c.key === val.sort) ? val.sort : '';
    this._sortKey = initialSort;
    const sortCol = cols.find((c) => c.key === initialSort);
    this._sortDir = sortCol && (sortCol.type === 'sev' || sortCol.type === 'num') ? 'desc' : 'asc';

    if (val?.filter) {
      this.setAttribute('filterable', '');
    } else {
      this.removeAttribute('filterable');
    }
    this._syncSortAttrs();
    this.render();
  }

  _syncSortAttrs() {
    if (this._sortKey) {
      this.setAttribute('sort-key', this._sortKey);
      this.setAttribute('sort-dir', this._sortDir);
    } else {
      this.removeAttribute('sort-key');
      this.removeAttribute('sort-dir');
    }
  }

  render() {
    clearNode(this.shadowRoot);
    if (!this._data) return;

    const cols = Array.isArray(this._data.cols) ? this._data.cols : [];
    const totalRows = Array.isArray(this._data.rows) ? this._data.rows.length : 0;

    if (this._data.filter) {
      const input = el('input', {
        part: 'filter',
        type: 'search',
        placeholder: 'Filtrar filas…',
        'aria-label': 'Filtrar filas',
        'aria-controls': this._tableId
      });
      input.addEventListener('input', () => {
        if (this._debounceTimer) clearTimeout(this._debounceTimer);
        this._debounceTimer = setTimeout(() => {
          this._query = input.value.trim();
          this._page = 0;
          const count = this._renderRows();
          this.dispatchEvent(new CustomEvent('dx-filter', {
            bubbles: true,
            composed: true,
            detail: { query: this._query, count }
          }));
        }, 80);
      });

      this._countOut = el('output', { part: 'count' }, `${totalRows} de ${totalRows}`);
      // [v2] Envoltura visual: icono de lupa que se tiñe en :focus-within (sin cambios de API)
      const searchIcon = svgEl('svg', {
        width: 16,
        height: 16,
        viewBox: '0 0 24 24',
        fill: 'none',
        stroke: 'currentColor',
        'stroke-width': '2',
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
        'aria-hidden': 'true'
      }, svgEl('circle', { cx: '11', cy: '11', r: '7' }), svgEl('path', { d: 'M20 20l-3.5-3.5' }));
      const field = el('label', { className: 'filter-field' }, searchIcon, input);
      this.shadowRoot.appendChild(el('div', { part: 'toolbar' }, field, this._countOut));
    } else {
      this._countOut = null;
    }

    this._theadRow = el('tr');
    this._tbody = el('tbody');
    const table = el(
      'table',
      { part: 'table', id: this._tableId },
      el('caption', { className: 'sr-only' }, 'Tabla técnica de datos'),
      el('thead', null, this._theadRow),
      this._tbody
    );

    this.shadowRoot.appendChild(el('div', { part: 'scroller' }, table));
    this._pagerWrap = el('div', { part: 'pagination', hidden: true });
    this.shadowRoot.appendChild(this._pagerWrap);

    this._renderHeaders(cols);
    this._renderRows();
  }

  _renderHeaders(cols) {
    clearNode(this._theadRow);
    for (const col of cols) {
      const isSorted = this._sortKey === col.key;
      const ariaSort = isSorted ? (this._sortDir === 'asc' ? 'ascending' : 'descending') : 'none';
      // [v2] Indicador único que rota por CSS según aria-sort (asc = 180°); aria-sort sigue informando el estado
      const btn = el(
        'button',
        { type: 'button' },
        el('span', null, col.label),
        el('span', { className: 'sort-ind', 'aria-hidden': 'true' }, createUiIcon('trend-down', 12))
      );
      btn.addEventListener('click', () => {
        if (this._sortKey === col.key) {
          this._sortDir = this._sortDir === 'asc' ? 'desc' : 'asc';
        } else {
          this._sortKey = col.key;
          this._sortDir = col.type === 'sev' || col.type === 'num' ? 'desc' : 'asc';
        }
        this._syncSortAttrs();
        this._renderHeaders(cols);
        this._renderRows();
        this.dispatchEvent(new CustomEvent('dx-sort', {
          bubbles: true,
          composed: true,
          detail: { key: this._sortKey, dir: this._sortDir }
        }));
      });

      this._theadRow.appendChild(el('th', {
        scope: 'col',
        'aria-sort': ariaSort,
        'data-type': col.type
      }, btn));
    }
  }

  _getProcessedRows() {
    const cols = Array.isArray(this._data?.cols) ? this._data.cols : [];
    const allRows = Array.isArray(this._data?.rows) ? this._data.rows : [];
    const q = this._query.toLowerCase();

    let filtered = allRows;
    if (q) {
      filtered = allRows.filter((row) => cols.some((c) => {
        const raw = row[c.key];
        if (c.type === 'sev') {
          const sevStr = SEV_SET.has(raw) ? `${raw} ${SEV_LABELS[raw]}` : String(raw ?? '');
          return sevStr.toLowerCase().includes(q);
        }
        return String(raw ?? '').toLowerCase().includes(q);
      }));
    }

    if (this._sortKey) {
      const sortCol = cols.find((c) => c.key === this._sortKey);
      const mul = this._sortDir === 'asc' ? 1 : -1;
      if (sortCol) {
        filtered = [...filtered].sort((a, b) => {
          const va = a[sortCol.key];
          const vb = b[sortCol.key];
          if (sortCol.type === 'num') {
            const na = Number(String(va ?? '').replace(/[^0-9.-]/g, '')) || 0;
            const nb = Number(String(vb ?? '').replace(/[^0-9.-]/g, '')) || 0;
            return (na - nb) * mul;
          }
          if (sortCol.type === 'sev') {
            const ra = SEV_RANK[va] || 0;
            const rb = SEV_RANK[vb] || 0;
            return (ra - rb) * mul;
          }
          return String(va ?? '').localeCompare(String(vb ?? ''), undefined, {
            numeric: true,
            sensitivity: 'base'
          }) * mul;
        });
      }
    }

    return filtered;
  }

  _renderRows() {
    const cols = Array.isArray(this._data?.cols) ? this._data.cols : [];
    const totalRows = Array.isArray(this._data?.rows) ? this._data.rows.length : 0;
    const processed = this._getProcessedRows();

    if (this._countOut) {
      this._countOut.textContent = `${processed.length} de ${totalRows}`;
    }

    clearNode(this._tbody);
    if (processed.length === 0) {
      this._tbody.appendChild(
        el('tr', null, el('td', { className: 'empty-cell', colspan: Math.max(1, cols.length) }, createSeverityIcon('info', 16), 'Sin coincidencias'))
      );
      this._pagerWrap.hidden = true;
      return 0;
    }

    let visibleRows = processed;
    const paginate = processed.length > PAGINATION_THRESHOLD;
    if (paginate) {
      const totalPages = Math.ceil(processed.length / PAGE_SIZE);
      if (this._page >= totalPages) this._page = totalPages - 1;
      const start = this._page * PAGE_SIZE;
      visibleRows = processed.slice(start, start + PAGE_SIZE);
      this._renderPager(processed.length, totalPages);
    } else {
      this._pagerWrap.hidden = true;
    }

    for (const row of visibleRows) {
      const tr = el('tr');
      for (const col of cols) {
        const val = row[col.key];
        const td = el('td', { 'data-type': col.type });

        if (col.type === 'sev') {
          const sev = SEV_SET.has(val) ? val : 'info';
          const badge = el(
            'span',
            { part: 'badge', 'data-sev': sev },
            createSeverityIcon(sev, 16),
            renderHighlightedText(SEV_LABELS[sev], this._query)
          );
          td.appendChild(badge);
        } else if (col.type === 'ref') {
          const refStr = String(val ?? '');
          if (refStr) {
            const btn = createRefButton(refStr, 'ref');
            if (btn) {
              if (this._query) {
                clearNode(btn);
                btn.appendChild(renderHighlightedText(refStr, this._query));
              }
              td.appendChild(btn);
            }
          }
        } else {
          td.appendChild(renderHighlightedText(val ?? '', this._query));
        }

        tr.appendChild(td);
      }
      this._tbody.appendChild(tr);
    }

    return processed.length;
  }

  _renderPager(totalFiltered, totalPages) {
    clearNode(this._pagerWrap);
    this._pagerWrap.hidden = false;
    const start = this._page * PAGE_SIZE + 1;
    const end = Math.min(totalFiltered, (this._page + 1) * PAGE_SIZE);

    const prevBtn = el('button', {
      type: 'button',
      disabled: this._page === 0,
      'aria-disabled': this._page === 0 ? 'true' : 'false'
    }, 'Anterior');
    prevBtn.addEventListener('click', () => {
      if (this._page > 0) {
        this._page--;
        this._renderRows();
      }
    });

    const nextBtn = el('button', {
      type: 'button',
      disabled: this._page >= totalPages - 1,
      'aria-disabled': this._page >= totalPages - 1 ? 'true' : 'false'
    }, 'Siguiente');
    nextBtn.addEventListener('click', () => {
      if (this._page < totalPages - 1) {
        this._page++;
        this._renderRows();
      }
    });

    this._pagerWrap.appendChild(el('span', null, `Filas ${start}–${end} de ${totalFiltered}`));
    this._pagerWrap.appendChild(prevBtn);
    this._pagerWrap.appendChild(nextBtn);
  }
}

if (!customElements.get('dx-data-table')) {
  customElements.define('dx-data-table', DxDataTable);
}
