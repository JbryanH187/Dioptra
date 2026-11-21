// ==========================================================================
// DioptraEngine Viewer — ui/components/dx-app.js
// <dx-app> (§2.1 Shell — Estado global de tema, sidebar redimensionable y HUD)
// ==========================================================================

import { clearNode, createSheet, el } from './shared.js';
import './dx-timeline.js';
// Helper único de severidad (L6); dx-report.js no importa dx-app.js → sin ciclo.
import { computeMaxSeverity } from './dx-report.js';
// R6/R5: visor de evidencia y gestor singleton de tooltips.
import './dx-evidence.js';
import './dx-tooltip.js';

const STORAGE_THEME_KEY = 'dx-theme';
const STORAGE_SIDEBAR_KEY = 'dx-sidebar-width';
const STORAGE_COLLAPSED_KEY = 'dx-sidebar-collapsed';
const DRAWER_QUERY = '(max-width: 1023.98px)';

function extractExcerpt(blocks) {
  if (!Array.isArray(blocks)) return '';
  for (const b of blocks) {
    if (!b || typeof b !== 'object') continue;
    if (typeof b.md === 'string' && b.md.trim()) {
      return b.md.replace(/[*`[\]()]/g, '').trim().slice(0, 96);
    }
    if (typeof b.body === 'string' && b.body.trim()) {
      return b.body.replace(/[*`[\]()]/g, '').trim().slice(0, 96);
    }
    if (typeof b.title === 'string' && b.title.trim()) {
      return b.title.trim();
    }
  }
  return 'Reporte técnico de inspección';
}

export class DxApp extends HTMLElement {
  constructor() {
    super();
    this._reportsById = new Map();
    this._timelineEl = null;
    this._reportEl = null;
    this._toastEl = null;
    this._toastTimer = null;
    this._sidebarSheet = createSheet('');
    this._revealSheet = createSheet('');
    this._sidebarWidth = 288;
    this._collapsed = false;
    this._shellEl = null;
    this._scrimEl = null;
    this._mounted = false;
    this._mql = null;
    this._onKeyDown = (e) => {
      if (e.key === 'Escape' && this._shellEl?.dataset.drawerOpen === 'true') {
        this._setDrawerOpen(false);
      }
    };
    // R4/R6: único consumidor de `dx-ref` (bubbles+composed desde createRefButton).
    this._onRef = (e) => {
      const d = e.detail || {};
      if (!d.path) return;
      const evidence = this._ensureEvidence();
      if (!evidence || typeof evidence.open !== 'function') return;
      const path = typeof e.composedPath === 'function' ? e.composedPath() : [];
      evidence.open({
        project: this._activeProject(),
        path: d.path,
        line: d.line,
        column: d.column ?? null,
        origin: path[0] || e.target
      });
    };
    this._onMediaChange = () => {
      if (!this._isDrawerMode()) this._setDrawerOpen(false);
      // Reaplica el colapso al cruzar el umbral en cualquier sentido (L2).
      this._setCollapsed(this._collapsed, false);
      this._syncInert();
    };
  }

  connectedCallback() {
    if (!this._mounted) {
      this._initPreferences();
      this._buildLayout();
      this._mounted = true;
    }
    this._attachGlobalListeners();
    this._ensureTooltip();
    this._ensureEvidence();
    this.removeEventListener('dx-ref', this._onRef);
    this.addEventListener('dx-ref', this._onRef);
    this._syncInert();
  }

  disconnectedCallback() {
    this.removeEventListener('dx-ref', this._onRef);
    document.removeEventListener('keydown', this._onKeyDown);
    if (this._mql) {
      if (typeof this._mql.removeEventListener === 'function') this._mql.removeEventListener('change', this._onMediaChange);
      else if (typeof this._mql.removeListener === 'function') this._mql.removeListener(this._onMediaChange);
      this._mql = null;
    }
  }

  _attachGlobalListeners() {
    document.removeEventListener('keydown', this._onKeyDown);
    document.addEventListener('keydown', this._onKeyDown);
    if (!this._mql && typeof window.matchMedia === 'function') {
      this._mql = window.matchMedia(DRAWER_QUERY);
      if (typeof this._mql.addEventListener === 'function') this._mql.addEventListener('change', this._onMediaChange);
      else if (typeof this._mql.addListener === 'function') this._mql.addListener(this._onMediaChange);
    }
  }

  _initPreferences() {
    if (!document.adoptedStyleSheets.includes(this._sidebarSheet)) {
      document.adoptedStyleSheets = [...document.adoptedStyleSheets, this._sidebarSheet];
    }

    try {
      // theme-init.js ya aplicó la preferencia guardada o la del sistema antes
      // del primer pintado; aquí solo se respeta ese valor.
      const savedTheme = localStorage.getItem(STORAGE_THEME_KEY);
      const initialTheme = savedTheme === 'light' || savedTheme === 'dark'
        ? savedTheme
        : (document.documentElement.dataset.theme === 'light' ? 'light' : 'dark');
      document.documentElement.dataset.theme = initialTheme;

      const savedWidth = Number.parseInt(localStorage.getItem(STORAGE_SIDEBAR_KEY) || '', 10);
      if (Number.isFinite(savedWidth) && savedWidth >= 240 && savedWidth <= 480) {
        this._setSidebarWidth(savedWidth, false);
      }
    } catch {
      document.documentElement.dataset.theme = 'dark';
    }

    try {
      this._collapsed = localStorage.getItem(STORAGE_COLLAPSED_KEY) === '1';
    } catch {
      this._collapsed = false;
    }
  }

  _isDrawerMode() {
    return Boolean(window.matchMedia?.(DRAWER_QUERY).matches);
  }

  _setCollapsed(value, persist = true) {
    this._collapsed = Boolean(value);
    if (this._shellEl) {
      if (this._collapsed) this._shellEl.dataset.collapsed = 'true';
      else delete this._shellEl.dataset.collapsed;
    }
    if (this._timelineEl && typeof this._timelineEl.setCollapsed === 'function') {
      // En modo drawer el rail no aplica (< 1024px): timeline siempre expandido.
      this._timelineEl.setCollapsed(this._isDrawerMode() ? false : this._collapsed);
    }
    if (persist) {
      try {
        localStorage.setItem(STORAGE_COLLAPSED_KEY, this._collapsed ? '1' : '0');
      } catch {
        // Ignorado si localStorage está deshabilitado
      }
    }
  }

  _setDrawerOpen(open) {
    if (!this._shellEl) return;
    const wasOpen = this._shellEl.dataset.drawerOpen === 'true';
    if (open) this._shellEl.dataset.drawerOpen = 'true';
    else delete this._shellEl.dataset.drawerOpen;
    if (this._drawerBtn) this._drawerBtn.setAttribute('aria-expanded', open ? 'true' : 'false');
    // Quitar inert ANTES de mover el foco (un nodo inert no es enfocable).
    this._syncInert();
    if (open && !wasOpen) {
      const search = this._timelineEl?.querySelector('.dx-search-input');
      if (search && typeof search.focus === 'function') search.focus();
    } else if (!open && wasOpen && this._drawerBtn) {
      this._drawerBtn.focus();
    }
  }

  // Drawer (<1024 px): cerrado → timeline inert; abierto → dx-report inert
  // (su .dx-report-header es descendiente, hereda inert). Escritorio → nada inert.
  _syncInert() {
    const drawer = this._isDrawerMode();
    const open = this._shellEl?.dataset.drawerOpen === 'true';
    if (this._timelineEl) this._timelineEl.inert = drawer && !open;
    if (this._reportEl) this._reportEl.inert = drawer && open;
  }

  // API pública (C — Ronda 3): delega en <dx-timeline>.setState.
  setTimelineState(kind, opts) {
    this._timelineEl?.setState?.(kind, opts);
  }

  _toggleSidebar() {
    if (this._isDrawerMode()) {
      this._setDrawerOpen(this._shellEl?.dataset.drawerOpen !== 'true');
    } else {
      this._setCollapsed(!this._collapsed, true);
    }
  }

  _setSidebarWidth(px, persist = true) {
    const clamped = Math.min(480, Math.max(240, Math.round(px)));
    this._sidebarWidth = clamped;
    this._sidebarSheet.replaceSync(`:root { --sidebar-width: ${clamped}px; }`);
    if (persist) {
      try {
        localStorage.setItem(STORAGE_SIDEBAR_KEY, String(clamped));
      } catch {
        // Ignorado si localStorage está deshabilitado
      }
    }
  }

  setTheme(nextTheme) {
    const safeTheme = nextTheme === 'light' ? 'light' : 'dark';
    document.documentElement.dataset.theme = safeTheme;
    try {
      localStorage.setItem(STORAGE_THEME_KEY, safeTheme);
    } catch {
      // Ignorado
    }
    if (this._reportEl) {
      this._reportEl.setThemeIndicator(safeTheme);
    }
    window.dispatchEvent(new CustomEvent('dx-theme', {
      detail: { theme: safeTheme }
    }));
  }

  toggleTheme(origin = null) {
    const current = document.documentElement.dataset.theme === 'light' ? 'light' : 'dark';
    const next = current === 'dark' ? 'light' : 'dark';
    const reduceMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;

    // Revelado circular (View Transitions API). Fallback instantáneo si no hay soporte
    // o el usuario pidió movimiento reducido. Variables vía CSSStyleSheet adoptada (CSP-safe).
    if (typeof document.startViewTransition !== 'function' || reduceMotion) {
      this.setTheme(next);
      return;
    }

    const rect = origin?.getBoundingClientRect?.();
    const x = rect ? rect.left + rect.width / 2 : window.innerWidth - 48;
    const y = rect ? rect.top + rect.height / 2 : 28;
    const r = Math.hypot(Math.max(x, window.innerWidth - x), Math.max(y, window.innerHeight - y));
    this._revealSheet.replaceSync(
      `:root { --dx-reveal-x: ${Math.round(x)}px; --dx-reveal-y: ${Math.round(y)}px; --dx-reveal-r: ${Math.ceil(r)}px; }`
    );
    if (!document.adoptedStyleSheets.includes(this._revealSheet)) {
      document.adoptedStyleSheets = [...document.adoptedStyleSheets, this._revealSheet];
    }

    document.startViewTransition(() => this.setTheme(next));
  }

  loadReports(envelopes, activeReportId = '') {
    if (!this._mounted) {
      this._initPreferences();
      this._buildLayout();
      this._mounted = true;
    }

    this._reportsById.clear();
    const timelineItems = [];

    for (let i = 0; i < envelopes.length; i++) {
      const env = envelopes[i];
      const rep = env?.report || {};
      const id = rep.id || `rep_${i + 1}`;
      this._reportsById.set(id, env);

      timelineItems.push({
        id,
        project: rep.project || 'dioptra-core',
        title: rep.title || `Reporte ${i + 1}`,
        ts: rep.ts || new Date().toISOString(),
        maxSev: rep.maxSev || computeMaxSeverity(rep.blocks),
        excerpt: rep.excerpt || extractExcerpt(rep.blocks)
      });
    }

    this._timelineEl.items = timelineItems;
    const targetId = activeReportId && this._reportsById.has(activeReportId)
      ? activeReportId
      : timelineItems[0]?.id;

    if (targetId) {
      this._timelineEl.activeId = targetId;
      this._reportEl.envelope = this._reportsById.get(targetId);
    }
  }

  get activeReportId() {
    return this._timelineEl?.activeId || '';
  }

  // Proyecto del reporte ACTIVO: envelope cacheado por id (loadReports/displayEnvelope
  // guardan {report:{project}}), luego el envelope pintado en <dx-report>, luego el
  // summary del timeline (setTimelineSummaries trae `project`).
  _activeProject() {
    const id = this.activeReportId;
    const fromCache = id ? this._reportsById.get(id)?.report?.project : '';
    if (typeof fromCache === 'string' && fromCache) return fromCache;
    const fromView = this._reportEl?.envelope?.report?.project;
    if (typeof fromView === 'string' && fromView) return fromView;
    const items = Array.isArray(this._timelineEl?.items) ? this._timelineEl.items : [];
    const summary = id ? items.find((it) => it && it.id === id) : null;
    return typeof summary?.project === 'string' ? summary.project : '';
  }

  _ensureEvidence() {
    if (this._evidenceEl?.isConnected) return this._evidenceEl;
    let ev = document.body.querySelector(':scope > dx-evidence');
    if (!ev) {
      ev = document.createElement('dx-evidence');
      document.body.appendChild(ev);
    }
    this._evidenceEl = ev;
    return ev;
  }

  _ensureTooltip() {
    if (!document.body.querySelector(':scope > dx-tooltip')) {
      document.body.appendChild(document.createElement('dx-tooltip'));
    }
  }

  setTimelineSummaries(items, activeReportId = '') {
    if (!this._mounted) {
      this._initPreferences();
      this._buildLayout();
      this._mounted = true;
    }
    const list = Array.isArray(items) ? items : [];
    this._timelineEl.items = list;
    if (activeReportId) {
      this._timelineEl.activeId = activeReportId;
    }
  }

  upsertTimelineSummary(summary, makeActive = false) {
    if (!summary || typeof summary !== 'object' || !summary.id) return;
    if (!this._mounted) {
      this._initPreferences();
      this._buildLayout();
      this._mounted = true;
    }
    const existing = Array.isArray(this._timelineEl.items) ? [...this._timelineEl.items] : [];
    const idx = existing.findIndex((it) => it.id === summary.id);
    if (idx >= 0) {
      existing[idx] = { ...existing[idx], ...summary };
    } else {
      existing.unshift(summary);
    }
    this._timelineEl.items = existing;
    if (makeActive) {
      this._timelineEl.activeId = summary.id;
    }
  }

  displayEnvelope(id, envelope, { preserveScroll = false } = {}) {
    if (!this._mounted) {
      this._initPreferences();
      this._buildLayout();
      this._mounted = true;
    }
    if (id) {
      this._reportsById.set(id, envelope);
      if (this._timelineEl.activeId !== id) {
        this._timelineEl.activeId = id;
      }
    }
    const prevScroll = preserveScroll && this._reportEl ? this._reportEl.scrollTop : 0;
    this._reportEl.envelope = envelope;
    if (preserveScroll && this._reportEl) {
      this._reportEl.scrollTop = prevScroll;
    }
  }

  notifyToast(prefix, accentText) {
    this._showToast(prefix, accentText);
  }

  _showToast(prefix, accentText) {
    if (!this._toastEl) return;
    clearNode(this._toastEl);
    this._toastEl.appendChild(el('span', null, prefix));
    this._toastEl.appendChild(el('span', { className: 'dx-toast-accent' }, accentText));
    this._toastEl.dataset.visible = 'true';
    if (this._toastTimer) clearTimeout(this._toastTimer);
    this._toastTimer = setTimeout(() => {
      this._toastEl.dataset.visible = 'false';
    }, 2600);
  }

  _buildLayout() {
    clearNode(this);

    this._timelineEl = document.createElement('dx-timeline');
    this._reportEl = document.createElement('dx-report');

    const splitter = el('div', {
      className: 'dx-splitter',
      role: 'separator',
      tabindex: '0',
      'aria-orientation': 'vertical',
      'aria-label': 'Redimensionar barra lateral',
      'aria-valuemin': '240',
      'aria-valuemax': '480',
      'aria-valuenow': String(this._sidebarWidth)
    });

    let dragging = false;
    splitter.addEventListener('pointerdown', (e) => {
      if (e.button !== 0) return;
      dragging = true;
      splitter.dataset.dragging = 'true';
      splitter.setPointerCapture(e.pointerId);
    });

    splitter.addEventListener('pointermove', (e) => {
      if (!dragging) return;
      this._setSidebarWidth(e.clientX, false);
      splitter.setAttribute('aria-valuenow', String(this._sidebarWidth));
    });

    const stopResize = (e) => {
      if (!dragging) return;
      dragging = false;
      delete splitter.dataset.dragging;
      this._setSidebarWidth(this._sidebarWidth, true);
      try {
        if (splitter.hasPointerCapture(e.pointerId)) {
          splitter.releasePointerCapture(e.pointerId);
        }
      } catch {
        // Ignorado
      }
    };

    splitter.addEventListener('pointerup', stopResize);
    splitter.addEventListener('pointercancel', stopResize);

    splitter.addEventListener('keydown', (e) => {
      if (e.key === 'ArrowLeft') {
        e.preventDefault();
        this._setSidebarWidth(this._sidebarWidth - 16, true);
        splitter.setAttribute('aria-valuenow', String(this._sidebarWidth));
      } else if (e.key === 'ArrowRight') {
        e.preventDefault();
        this._setSidebarWidth(this._sidebarWidth + 16, true);
        splitter.setAttribute('aria-valuenow', String(this._sidebarWidth));
      }
    });

    this._toastEl = el('div', {
      className: 'dx-toast',
      role: 'status',
      'aria-live': 'polite',
      dataset: { visible: 'false' }
    });

    this._scrimEl = el('button', { type: 'button', className: 'dx-scrim', 'aria-label': 'Cerrar panel' });
    this._scrimEl.addEventListener('click', () => this._setDrawerOpen(false));

    // Botón para abrir el drawer (< 1024px). Va fuera de dx-report porque
    // dx-report vacía su contenido al re-renderizar.
    this._drawerBtn = el('button', {
      type: 'button',
      className: 'dx-drawer-open',
      'aria-label': 'Abrir panel',
      'aria-controls': 'dx-sidebar',
      'aria-expanded': 'false'
    });
    const drawerSigil = document.createElement('dx-sigil');
    drawerSigil.setAttribute('size', 'glyph');
    drawerSigil.setAttribute('decorative', '');
    this._drawerBtn.appendChild(drawerSigil);
    this._drawerBtn.addEventListener('click', () => this._setDrawerOpen(true));

    const shell = el('div', { className: 'dx-shell' }, this._timelineEl, this._scrimEl, splitter, this._drawerBtn, this._reportEl);
    this._shellEl = shell;
    this.appendChild(shell);
    this.appendChild(this._toastEl);
    // Estado inicial (persistido) sin volver a escribir localStorage.
    this._setCollapsed(this._collapsed, false);

    this.addEventListener('dx-toggle-sidebar', () => this._toggleSidebar());

    // keydown de document: handler guardado en _onKeyDown (L7), se registra en
    // connectedCallback y se retira en disconnectedCallback.

    // L5 — render único. Orden real: este listener se registra en _buildLayout
    // (connectedCallback), ANTES que el de app.js (en el mismo elemento, tras el
    // primer fetch), así que corre primero y no puede ver `detail.handled`.
    // Vía alternativa: solo se renderiza desde caché si el envelope cacheado trae
    // `report.blocks` (preview/loadReports); en producción los summaries no traen
    // blocks y app.js hace el único render con fuente fresca. Igualmente se
    // respeta `detail.handled` si algún manejador previo lo marcó.
    this.addEventListener('dx-select-report', (e) => {
      const id = e.detail?.id;
      if (this._isDrawerMode()) this._setDrawerOpen(false);
      if (!id || e.detail?.handled) return;
      const cached = this._reportsById.get(id);
      if (Array.isArray(cached?.report?.blocks)) {
        this._reportEl.envelope = cached;
      }
    });

    this.addEventListener('dx-request-theme-toggle', () => {
      this.toggleTheme(this.querySelector('.dx-theme-btn'));
    });

  }
}

if (!customElements.get('dx-app')) {
  customElements.define('dx-app', DxApp);
}
