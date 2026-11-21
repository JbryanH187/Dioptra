import './dx-sessions.js';
// ==========================================================================
// DioptraEngine Viewer — ui/components/dx-timeline.js
// <dx-timeline> (§2.1 Shell — Historial cronológico estilo mensajería)
// ==========================================================================

import { clearNode, createUiIcon, el, renderHighlightedText, SEV_SET } from './shared.js';
import './dioptra-sigil.js';

const RAIL_MAX = 10;
const ROMAN = ['I', 'II', 'III', 'IV', 'V', 'VI', 'VII', 'VIII', 'IX', 'X'];

// Llamada protegida al sigilo: tolera ausencia del elemento o de la API.
function callSigil(s, method, ...args) {
  if (s && typeof s[method] === 'function') s[method](...args);
}

function formatTimeHHMM(isoStr) {
  const d = new Date(isoStr);
  if (Number.isNaN(d.getTime())) return '--:--';
  return d.toLocaleTimeString('es-MX', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  });
}

function getDayKey(dateObj) {
  return `${dateObj.getFullYear()}-${String(dateObj.getMonth() + 1).padStart(2, '0')}-${String(dateObj.getDate()).padStart(2, '0')}`;
}

function computeGroupLabel(isoStr, referenceDate) {
  const d = new Date(isoStr);
  if (Number.isNaN(d.getTime())) return 'Sin fecha';

  const todayKey = getDayKey(referenceDate);
  const yesterday = new Date(referenceDate);
  yesterday.setDate(yesterday.getDate() - 1);
  const yesterdayKey = getDayKey(yesterday);
  const itemKey = getDayKey(d);

  if (itemKey === todayKey) return 'Hoy';
  if (itemKey === yesterdayKey) return 'Ayer';

  return d.toLocaleDateString('es-MX', {
    day: '2-digit',
    month: 'short',
    year: 'numeric'
  });
}

export class DxTimeline extends HTMLElement {
  constructor() {
    super();
    this._items = [];
    this._activeId = '';
    this._query = '';
    this._filters = { project: '', tool: '', model: '' };
    this._listEl = null;
    this._badgeEl = null;
    this._facetsEl = null;
    this._railListEl = null;
    this._collapseBtn = null;
    this._sigilFull = null;
    this._collapsed = false;
    this._built = false;
    this._railWakeBtn = null;
    this._searchInput = null;
    this._focusIntent = false;
    this._focusRaf = 0;
    // Estado de carga del timeline: 'loading' | 'error' | 'ready' (C — Ronda 3).
    this._state = 'ready';
    this._stateOpts = {};
  }

  connectedCallback() {
    if (!this.id) this.id = 'dx-sidebar';
    if (!this._built) {
      this._buildShell();
      this._built = true;
    }
    this._renderList();
    if (!this._onSigilBlink) {
      this._onSigilBlink = (e) => callSigil(this._sigilFull, 'blink', e.detail?.flash !== false);
    }
    document.addEventListener('dx-sigil-blink', this._onSigilBlink);
  }

  disconnectedCallback() {
    if (this._onSigilBlink) document.removeEventListener('dx-sigil-blink', this._onSigilBlink);
    if (this._focusRaf) {
      cancelAnimationFrame(this._focusRaf);
      this._focusRaf = 0;
    }
  }

  get items() {
    return this._items;
  }

  set items(val) {
    this._items = Array.isArray(val) ? [...val] : [];
    this._items.sort((a, b) => new Date(b.updatedAt || b.ts).getTime() - new Date(a.updatedAt || a.ts).getTime());
    if (!this._activeId && this._items.length > 0) {
      this._activeId = this._items[0].id;
    }
    this._scheduleRender();
  }

  get activeId() {
    return this._activeId;
  }

  set activeId(id) {
    this._activeId = String(id ?? '');
    this._scheduleRender();
  }

  // Agrupa en un solo pintado los cambios hechos en el mismo ciclo (items + activeId).
  _scheduleRender() {
    if (!this._built || this._renderQueued) return;
    this._renderQueued = true;
    queueMicrotask(() => {
      this._renderQueued = false;
      this._renderList();
    });
  }

  _buildShell() {
    clearNode(this);

    // Marca compacta: el sigilo completo (con sus animaciones) a la izquierda y
    // el nombre a la derecha, en una sola fila. Conserva la presencia del sigilo
    // sin empujar la lista hacia abajo.
    this._sigilFull = el('dx-sigil', { className: 'dx-codex-sigil', size: 'full' });
    this._badgeEl = el('span', { className: 'dx-brand-badge' }, '0');
    this._brandMetaEl = el('span', { className: 'dx-brand-meta' });
    const codexHead = el(
      'div',
      { className: 'dx-codex-head' },
      this._sigilFull,
      el(
        'div',
        { className: 'dx-codex-text' },
        el('span', { className: 'dx-codex-title' }, 'DIOPTRA'),
        el('span', { className: 'dx-codex-sub' }, this._badgeEl, this._brandMetaEl)
      )
    );

    this._collapseBtn = el('button', {
      type: 'button',
      className: 'dx-collapse-btn',
      'aria-controls': 'dx-sidebar',
      'aria-expanded': this._collapsed ? 'false' : 'true',
      'aria-label': this._collapsed ? 'Expandir panel' : 'Contraer panel'
    }, createUiIcon('chevron', 16));
    this._collapseBtn.addEventListener('click', () => this._emitToggle());

    const header = el(
      'header',
      { className: 'dx-sidebar-brand' },
      codexHead,
      this._collapseBtn
    );

    const railWake = el(
      'button',
      { type: 'button', className: 'dx-rail-wake', 'aria-label': 'Expandir panel' },
      el('dx-sigil', { size: 'mark', state: 'asleep', decorative: true })
    );
    railWake.addEventListener('click', () => this._emitToggle());
    this._railWakeBtn = railWake;
    this._railListEl = el('nav', { className: 'dx-rail-list', 'aria-label': 'Reportes recientes' });
    const rail = el('div', { className: 'dx-rail' }, railWake, this._railListEl);

    const searchIcon = createUiIcon('search', 15);
    searchIcon.setAttribute('class', 'dx-search-icon');
    const searchInput = el('input', {
      type: 'search',
      className: 'dx-search-input',
      placeholder: 'Buscar por título o proyecto…',
      'aria-label': 'Buscar reportes por título o proyecto'
    });
    this._searchInput = searchInput;
    searchInput.addEventListener('input', () => {
      clearTimeout(this._searchTimer);
      this._searchTimer = setTimeout(() => {
        this._query = searchInput.value.trim();
        this._renderList();
        this._searchServer(this._query);
      }, 120);
    });

    const searchBar = el(
      'div',
      { className: 'dx-sidebar-search' },
      el('div', { className: 'dx-search-wrap' }, searchIcon, searchInput)
    );

    this._facetsEl = el('div', { className: 'dx-facets', role: 'group', 'aria-label': 'Filtrar por herramienta y modelo' });
    this._filtersSummaryEl = el('span', { className: 'dx-filters-active' });
    this._filtersEl = el(
      'details',
      { className: 'dx-filters' },
      el('summary', { className: 'dx-filters-summary' }, createUiIcon('chevron', 12), 'Filtros', this._filtersSummaryEl),
      this._facetsEl
    );
    this._closedGroups = DxTimeline.loadClosedGroups();

    this._listEl = el('div', {
      className: 'dx-timeline-list',
      role: 'listbox',
      'aria-label': 'Historial cronológico de reportes'
    });

    this._listEl.addEventListener('keydown', (e) => this._handleKeyDown(e));
    this._listEl.addEventListener('mouseleave', () => callSigil(this._sigilFull, 'release'));

    this.appendChild(header);
    this.appendChild(rail);
    this.appendChild(searchBar);
    this.appendChild(this._filtersEl);
    // Sesiones abiertas de agentes (se oculta solo si no hay ninguna).
    this.appendChild(document.createElement("dx-sessions"));
    this.appendChild(this._listEl);
  }

  _emitToggle() {
    // El disparo vino de un botón propio: autoriza mover el foco tras el render.
    this._focusIntent = true;
    this.dispatchEvent(new CustomEvent('dx-toggle-sidebar', { bubbles: true, composed: true, detail: {} }));
    // Si nadie llamó a setCollapsed (p. ej. modo drawer), la intención caduca.
    this._focusIntent = false;
  }

  setCollapsed(collapsed) {
    const next = Boolean(collapsed);
    const changed = next !== this._collapsed;
    // Solo se mueve el foco si estaba dentro del timeline o el cambio lo disparó un botón propio.
    const active = document.activeElement;
    const focusInside = Boolean(active && active !== document.body && this.contains(active));
    const shouldMoveFocus = changed && (focusInside || this._focusIntent);
    this._focusIntent = false;

    this._collapsed = next;
    if (this._collapseBtn) {
      this._collapseBtn.setAttribute('aria-expanded', this._collapsed ? 'false' : 'true');
      this._collapseBtn.setAttribute('aria-label', this._collapsed ? 'Expandir panel' : 'Contraer panel');
    }
    callSigil(this._sigilFull, this._collapsed ? 'sleep' : 'wake');

    if (shouldMoveFocus) {
      if (this._focusRaf) cancelAnimationFrame(this._focusRaf);
      // Tras el render: el botón destino deja de estar oculto por CSS en el siguiente frame.
      this._focusRaf = requestAnimationFrame(() => {
        this._focusRaf = 0;
        const target = this._collapsed ? this._railWakeBtn : this._collapseBtn;
        if (target && target.isConnected && typeof target.focus === 'function') target.focus();
      });
    }
  }

  // API pública (C — Ronda 3): kind 'loading' | 'error' | 'ready'.
  setState(kind, { message, onRetry } = {}) {
    const safe = kind === 'loading' || kind === 'error' ? kind : 'ready';
    this._state = safe;
    this._stateOpts = {
      message: typeof message === 'string' && message.trim() ? message.trim() : '',
      onRetry: typeof onRetry === 'function' ? onRetry : null
    };
    if (this._built) this._renderList();
  }

  get state() {
    return this._state;
  }

  // Resetea búsqueda y chips con el estado existente y notifica como _setFilter.
  _clearFilters() {
    this._query = '';
    if (this._searchInput) this._searchInput.value = '';
    this._filters = { project: '', tool: '', model: '' };
    this._renderList();
    this.dispatchEvent(new CustomEvent('dx-filter-change', {
      bubbles: true,
      composed: true,
      detail: { ...this._filters }
    }));
    // El botón «Limpiar filtros» desaparece: el foco va a la búsqueda.
    if (this._searchInput && typeof this._searchInput.focus === 'function') this._searchInput.focus();
  }

  // Alterna la semántica del área de lista: listbox solo cuando contiene opciones.
  _setListSemantics(isListbox) {
    if (!this._listEl) return;
    if (isListbox) {
      this._listEl.setAttribute('role', 'listbox');
      this._listEl.setAttribute('aria-label', 'Historial cronológico de reportes');
    } else {
      this._listEl.removeAttribute('role');
      this._listEl.removeAttribute('aria-label');
    }
  }

  _renderStateView(kind) {
    if (kind === 'loading') {
      const skels = [];
      for (let i = 0; i < 5; i++) skels.push(el('div', { className: 'dx-skel', 'aria-hidden': 'true' }));
      return el(
        'div',
        { className: 'dx-timeline-state', 'data-kind': 'loading', 'aria-busy': 'true' },
        el('span', { className: 'sr-only', role: 'status' }, 'Cargando reportes…'),
        skels
      );
    }
    if (kind === 'error') {
      const retry = el('button', { type: 'button', className: 'dx-timeline-retry' }, 'Reintentar');
      const onRetry = this._stateOpts.onRetry;
      if (onRetry) {
        retry.addEventListener('click', () => onRetry());
      } else {
        retry.disabled = true;
      }
      return el(
        'div',
        { className: 'dx-timeline-state', 'data-kind': 'error', role: 'alert' },
        el('p', { className: 'dx-timeline-state-msg' }, this._stateOpts.message || 'No se pudieron cargar los reportes.'),
        retry
      );
    }
    return null;
  }

  _renderEmptyView() {
    if (this._items.length === 0) {
      return el(
        'div',
        { className: 'dx-timeline-state', 'data-kind': 'empty' },
        el('p', { className: 'dx-timeline-state-msg' }, 'Aún no hay reportes')
      );
    }
    const clearBtn = el('button', { type: 'button', className: 'dx-timeline-clear' }, 'Limpiar filtros');
    clearBtn.addEventListener('click', () => this._clearFilters());
    return el(
      'div',
      { className: 'dx-timeline-state', 'data-kind': 'empty' },
      el('p', { className: 'dx-timeline-state-msg' }, 'Sin coincidencias'),
      clearBtn
    );
  }

  celebrate() {
    callSigil(this._sigilFull, 'celebrate');
  }

  _attendTo(node) {
    const r = node?.getBoundingClientRect?.();
    if (!r) return;
    callSigil(this._sigilFull, 'attend', r.left + r.width / 2, r.top + r.height / 2);
  }

  // Ruta única de selección por clic (lista y rail).
  _activateFromClick(id, { fromRail = false } = {}) {
    callSigil(this._sigilFull, 'blink', false);
    // Desde el rail la lista está oculta: no enfocar su opción, devolver el foco al botón del rail.
    this._selectId(id, !fromRail);
    if (fromRail) {
      this._railListEl?.querySelector('[data-report-id="' + CSS.escape(id) + '"]')?.focus();
    }
  }

  _renderRail(filtered) {
    if (!this._railListEl) return;
    clearNode(this._railListEl);
    filtered.slice(0, RAIL_MAX).forEach((item, i) => {
      const btn = el('button', {
        type: 'button',
        className: 'dx-rail-item',
        'aria-label': item.title || item.id,
        'aria-current': item.id === this._activeId ? 'true' : null,
        dataset: { reportId: item.id }
      }, ROMAN[i]);
      btn.addEventListener('click', () => this._activateFromClick(item.id, { fromRail: true }));
      this._railListEl.appendChild(btn);
    });
  }

  // Clave canónica equivalente a store.ProjectKey: "Dioptra Engine" y "dioptra-engine" son el mismo space.
  static projectKey(name) {
    return String(name ?? '')
      .toLowerCase()
      .normalize('NFD')
      .replace(/[\u0300-\u036f]/g, '')
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '');
  }

  _matches(it, skip = '') {
    const f = this._filters;
    if (skip !== 'project' && f.project && DxTimeline.projectKey(it.project || 'dioptra') !== f.project) return false;
    if (skip !== 'tool' && f.tool && String(it.tool ?? '').toLowerCase() !== f.tool.toLowerCase()) return false;
    if (skip !== 'model' && f.model && String(it.model ?? '').toLowerCase() !== f.model.toLowerCase()) return false;
    return true;
  }

  _getFilteredItems() {
    const base = this._items.filter((it) => this._matches(it));
    if (!this._query) return base;
    const q = this._query.toLowerCase();
    const hits = this._serverQuery === this._query ? this._serverHits : null;
    const local = base.filter((it) => {
      const hay = [it.title, it.project, it.tool, it.model]
        .map((v) => String(v ?? '').toLowerCase());
      return hay.some((s) => s.includes(q)) || Boolean(hits && hits.has(it.id));
    });
    // Coincidencias por CONTENIDO fuera de los reportes cargados (búsqueda BM25 del servidor).
    if (hits) {
      const known = new Set(this._items.map((it) => it.id));
      for (const it of this._serverItems || []) {
        if (!known.has(it.id) && this._matches(it)) local.push(it);
      }
    }
    return local;
  }

  // Búsqueda en el servidor (FTS5/BM25 sobre el cuerpo completo de cada reporte).
  async _searchServer(query) {
    const seq = (this._searchSeq = (this._searchSeq || 0) + 1);
    if (!query || query.length < 2) {
      this._serverQuery = '';
      this._serverHits = null;
      this._serverItems = [];
      return;
    }
    try {
      const resp = await fetch(`/api/reports?q=${encodeURIComponent(query)}&limit=50`, { headers: { Accept: 'application/json' } });
      if (!resp.ok || seq !== this._searchSeq) return;
      const data = await resp.json();
      const items = Array.isArray(data) ? data : (data.reports || data.items || []);
      this._serverQuery = query;
      this._serverHits = new Set(items.map((it) => it.id));
      this._serverItems = items;
      this._renderList();
    } catch {
      // Sin servidor: se queda solo el filtro local.
    }
  }

  _setFilter(name, value) {
    // Pulsar el chip activo lo desactiva.
    this._filters[name] = this._filters[name] === value ? '' : value;
    this._renderList();
    this.dispatchEvent(new CustomEvent('dx-filter-change', {
      bubbles: true,
      composed: true,
      detail: { ...this._filters }
    }));
  }

  _chipButton(label, pressed, onClick, extraClass = '', count = null) {
    const btn = el(
      'button',
      {
        type: 'button',
        className: `dx-chip ${extraClass}`.trim(),
        'aria-pressed': pressed ? 'true' : 'false'
      },
      label,
      count === null ? '' : el('span', { className: 'dx-chip-count' }, String(count))
    );
    btn.addEventListener('click', onClick);
    return btn;
  }

  // Resumen bajo la marca ("6 reportes · 3 workspaces") e indicador de filtros.
  _renderSpaces() {
    const spaces = new Set(this._items.map((it) => DxTimeline.projectKey(it.project || 'dioptra')));
    if (this._brandMetaEl) {
      this._brandMetaEl.textContent = `reportes · ${spaces.size} ${spaces.size === 1 ? 'workspace' : 'workspaces'}`;
    }
    if (this._filtersSummaryEl) {
      const active = [this._filters.tool, this._filters.model].filter(Boolean);
      this._filtersSummaryEl.textContent = active.length ? active.join(' · ') : '';
    }
  }

  static loadClosedGroups() {
    try {
      const raw = localStorage.getItem('dx-closed-workspaces');
      const arr = raw ? JSON.parse(raw) : [];
      return new Set(Array.isArray(arr) ? arr : []);
    } catch {
      return new Set();
    }
  }

  _toggleGroup(key) {
    if (this._closedGroups.has(key)) this._closedGroups.delete(key);
    else this._closedGroups.add(key);
    try {
      localStorage.setItem('dx-closed-workspaces', JSON.stringify([...this._closedGroups]));
    } catch {
      // Sin almacenamiento: el estado vive solo en esta pestaña.
    }
    this._renderList();
  }

  _renderFacets() {
    if (!this._facetsEl) return;
    clearNode(this._facetsEl);
    const collect = (field, skip) => {
      const counts = new Map();
      for (const it of this._items) {
        if (!this._matches(it, skip)) continue;
        const v = String(it[field] ?? '').trim();
        if (v) counts.set(v, (counts.get(v) || 0) + 1);
      }
      return [...counts.entries()].sort((a, b) => b[1] - a[1]);
    };
    for (const [field, label] of [['tool', 'Herramienta'], ['model', 'Modelo']]) {
      const entries = collect(field, field);
      if (entries.length === 0) continue;
      const row = el('div', { className: 'dx-facet-row' }, el('span', { className: 'dx-facet-label' }, label));
      for (const [value, n] of entries) {
        row.appendChild(this._chipButton(value, this._filters[field].toLowerCase() === value.toLowerCase(), () => this._setFilter(field, value), `dx-chip-${field}`, n));
      }
      this._facetsEl.appendChild(row);
    }
  }

  _selectId(id, focusOption = false) {
    if (!id) return;
    this._activeId = id;
    this._renderList();
    if (focusOption && this._listEl) {
      const opt = this._listEl.querySelector(`[data-report-id="${CSS.escape(id)}"]`);
      if (opt) opt.focus();
    }
    this.dispatchEvent(new CustomEvent('dx-select-report', {
      bubbles: true,
      composed: true,
      detail: { id }
    }));
  }

  _handleKeyDown(e) {
    const options = Array.from(this._listEl.querySelectorAll('[role="option"]')).filter((o) => !o.closest('[hidden]'));
    if (options.length === 0) return;

    const currentIndex = options.findIndex((o) => o.dataset.reportId === this._activeId);
    let nextIndex = currentIndex;

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      nextIndex = currentIndex < options.length - 1 ? currentIndex + 1 : 0;
      this._selectId(options[nextIndex].dataset.reportId, true);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      nextIndex = currentIndex > 0 ? currentIndex - 1 : options.length - 1;
      this._selectId(options[nextIndex].dataset.reportId, true);
    } else if (e.key === 'Home') {
      e.preventDefault();
      this._selectId(options[0].dataset.reportId, true);
    } else if (e.key === 'End') {
      e.preventDefault();
      this._selectId(options[options.length - 1].dataset.reportId, true);
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      const focused = document.activeElement;
      if (focused?.dataset?.reportId) {
        this._selectId(focused.dataset.reportId, true);
      }
    }
  }

  _renderList() {
    if (!this._listEl) return;
    clearNode(this._listEl);
    this._renderSpaces();
    this._renderFacets();

    const filtered = this._getFilteredItems();
    if (this._badgeEl) {
      this._badgeEl.textContent = String(filtered.length);
    }
    this._renderRail(filtered);

    if (this._state !== 'ready') {
      this._setListSemantics(false);
      this._listEl.appendChild(this._renderStateView(this._state));
      return;
    }

    if (filtered.length === 0) {
      this._setListSemantics(false);
      this._listEl.appendChild(this._renderEmptyView());
      return;
    }
    this._setListSemantics(true);

    // Referencia temporal anclada al reporte más reciente (o reloj actual)
    const newestDate = this._items.length > 0 && !Number.isNaN(new Date(this._items[0].ts).getTime())
      ? new Date(this._items[0].ts)
      : new Date();

    // Agrupa por workspace (proyecto). filtered ya viene ordenado por actividad
    // reciente, así que el Map conserva ese orden: el workspace más activo va arriba.
    const groups = new Map();
    for (const item of filtered) {
      const key = DxTimeline.projectKey(item.project || 'dioptra');
      const g = groups.get(key) || { key, name: item.project || 'dioptra', items: [] };
      g.items.push(item);
      groups.set(key, g);
    }

    const searching = Boolean(this._query);
    for (const g of groups.values()) {
      const hasActive = g.items.some((it) => it.id === this._activeId);
      // Al buscar se abren todos los grupos para mostrar cada coincidencia.
      const open = searching || !this._closedGroups.has(g.key);
      const bodyId = `dx-ws-${g.key.replace(/[^a-z0-9_-]/gi, '-')}`;
      const head = el(
        'button',
        {
          type: 'button',
          className: 'dx-ws-head',
          'aria-expanded': open ? 'true' : 'false',
          'aria-controls': bodyId,
          title: g.name,
          dataset: { active: hasActive ? 'true' : 'false' }
        },
        createUiIcon('chevron', 12),
        el('span', { className: 'dx-ws-name' }, renderHighlightedText(g.name, this._query, 'dx-timeline-mark')),
        el('span', { className: 'dx-ws-count' }, String(g.items.length))
      );
      head.addEventListener('click', () => this._toggleGroup(g.key));

      const body = el('div', { className: 'dx-ws-body', id: bodyId, role: 'group', 'aria-label': g.name });
      if (!open) body.hidden = true;
      for (const item of g.items) body.appendChild(this._renderItem(item, newestDate));

      this._listEl.appendChild(el('section', { className: 'dx-ws', dataset: { open: open ? 'true' : 'false' } }, head, body));
    }
  }

  _renderItem(item, newestDate) {
    const isSelected = item.id === this._activeId;
    // Sin severidad → 'info' como punto neutro (acero), no implica severidad informativa real.
    const sev = SEV_SET.has(item.maxSev) ? item.maxSev : 'info';
    const day = computeGroupLabel(item.ts, newestDate);
    const when = day === 'Hoy' ? formatTimeHHMM(item.ts) : day === 'Ayer' ? `Ayer ${formatTimeHHMM(item.ts)}` : day;

    const titleH3 = el(
      'h3',
      { className: 'dx-timeline-title', title: item.title || '' },
      el('span', { className: 'dx-sev-dot', 'data-sev': sev, 'aria-hidden': 'true' }),
      el('span', { className: 'dx-timeline-title-text' }, renderHighlightedText(item.title || '', this._query, 'dx-timeline-mark'))
    );

    const meta = el('div', { className: 'dx-timeline-meta' },
      el('time', { className: 'dx-timeline-time', datetime: item.ts, title: new Date(item.ts).toLocaleString('es-MX') }, when));
    if (item.tool) meta.appendChild(el('span', { className: 'dx-badge dx-badge-tool', title: 'Herramienta' }, item.tool));
    if (item.model) meta.appendChild(el('span', { className: 'dx-badge dx-badge-model', title: 'Modelo' }, item.model));

    const optionEl = el(
      'div',
      {
        className: 'dx-timeline-item',
        role: 'option',
        tabindex: isSelected ? '0' : '-1',
        'aria-selected': isSelected ? 'true' : 'false',
        dataset: { reportId: item.id }
      },
      titleH3,
      meta,
      item.excerpt ? el('p', { className: 'dx-timeline-excerpt' }, item.excerpt) : ''
    );

    optionEl.addEventListener('click', () => this._activateFromClick(item.id));
    optionEl.addEventListener('mouseenter', () => this._attendTo(optionEl));
    return optionEl;
  }
}

if (!customElements.get('dx-timeline')) {
  customElements.define('dx-timeline', DxTimeline);
}
