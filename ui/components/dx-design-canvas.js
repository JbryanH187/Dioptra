// ==========================================================================
// DioptraEngine Viewer — ui/components/dx-design-canvas.js
// <dx-design-canvas> (T1 — Modal 88-90% de Lienzo multipantalla + Prototipo
// interactivo + capa de anotaciones Pin/Caja/Marcador para agentes MCP).
// 100% CSP-safe: construcción DOM pura, estilos en createSheet, variables por CSSOM.
// ==========================================================================

import { adoptSheets, clearNode, createSheet, createUiIcon, el } from './shared.js';
import { BoardAnnotationOverlay } from './canvas-annotator.js';

const designCanvasSheet = createSheet(`
  :host {
    display: contents;
  }

  dialog.dc-modal {
    width: min(90vw, 1680px);
    height: 90vh;
    max-width: none;
    max-height: none;
    margin: auto;
    padding: 0;
    border: var(--border-width) solid var(--color-border-strong);
    border-radius: var(--rounded-md);
    background: var(--color-surface-base);
    color: var(--color-text-primary);
    box-shadow: 0 28px 64px rgba(0, 0, 0, 0.55);
    overflow: hidden;
    font-family: var(--font-sans);
  }

  dialog.dc-modal[open] {
    display: flex;
    flex-direction: column;
  }

  dialog.dc-modal::backdrop {
    background: rgba(4, 6, 9, 0.72);
  }

  @media (max-width: 820px) {
    dialog.dc-modal {
      width: 96vw;
      height: 94vh;
    }
  }

  .dc-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--spacing-3);
    padding: var(--spacing-2) var(--spacing-4);
    background: var(--color-surface-raised);
    border-bottom: var(--border-width) solid var(--color-border-translucent);
    flex-shrink: 0;
    flex-wrap: wrap;
  }

  .dc-brand {
    display: flex;
    align-items: center;
    gap: var(--spacing-2);
    min-width: 0;
  }

  .dc-title {
    font-size: var(--fs-small);
    font-weight: var(--fw-semibold);
    color: var(--color-text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .dc-sub {
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    color: var(--color-text-soft);
    white-space: nowrap;
  }

  .dc-select {
    height: 28px;
    padding: 0 8px;
    border-radius: var(--rounded-sm);
    border: var(--border-width) solid var(--color-border-translucent);
    background: var(--color-surface-sunken);
    color: var(--color-text-primary);
    font: inherit;
    font-size: var(--fs-caption);
  }

  .dc-tabs,
  .dc-tools {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 3px;
    border-radius: var(--rounded-sm);
    background: var(--color-surface-sunken);
    border: var(--border-width) solid var(--color-border-translucent);
  }

  .dc-tab,
  .dc-tool-btn,
  .dc-btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 28px;
    padding: 0 10px;
    border: var(--border-width) solid transparent;
    border-radius: var(--rounded-xs);
    background: transparent;
    color: var(--color-text-soft);
    font-family: var(--font-sans);
    font-size: var(--fs-caption);
    font-weight: var(--fw-semibold);
    cursor: pointer;
    white-space: nowrap;
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard),
                color var(--motion-duration-fast) var(--motion-easing-standard),
                border-color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  .dc-tab:hover,
  .dc-tool-btn:hover,
  .dc-btn:hover {
    background: var(--color-surface-hover);
    color: var(--color-text-primary);
  }

  .dc-tab[aria-pressed="true"],
  .dc-tool-btn[aria-pressed="true"] {
    background: var(--color-surface-elevated);
    color: var(--color-text-primary);
    border-color: var(--color-border-strong);
  }

  .dc-tool-btn[data-tool-active="annotate"] {
    background: var(--color-accent-soft);
    color: var(--color-accent-fg);
    border-color: var(--color-accent-ring);
  }

  .dc-btn {
    border-color: var(--color-border-translucent);
    background: var(--color-surface-base);
    color: var(--color-text-primary);
  }

  .dc-btn.dc-btn-primary {
    background: var(--color-interactive-primary);
    color: var(--color-on-primary);
    border-color: var(--color-interactive-primary);
  }

  .dc-btn.dc-btn-primary:hover {
    background: var(--color-interactive-primary-hover);
  }

  .dc-btn.dc-btn-icon {
    width: 28px;
    padding: 0;
    justify-content: center;
  }

  .dc-btn:focus-visible,
  .dc-tab:focus-visible,
  .dc-tool-btn:focus-visible {
    outline: none;
    box-shadow: var(--focus-ring);
  }

  .dc-controls {
    display: flex;
    align-items: center;
    gap: var(--spacing-2);
    flex-wrap: wrap;
  }

  .dc-zoom-val {
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    color: var(--color-text-soft);
    min-width: 38px;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }

  .dc-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    border-radius: var(--rounded-full);
    background: var(--color-interactive-primary);
    color: var(--color-on-primary);
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 700;
  }

  .dc-badge[data-zero="true"] {
    background: var(--color-surface-elevated);
    color: var(--color-text-soft);
  }

  .dc-body {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    overflow: hidden;
    position: relative;
  }

  .dc-main {
    flex: 1 1 auto;
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    background: var(--color-surface-sunken);
  }

  .dc-tool-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--spacing-2);
    padding: 6px var(--spacing-4);
    background: var(--color-accent-soft);
    border-bottom: var(--border-width) solid var(--color-accent-ring);
    color: var(--color-text-primary);
    font-size: var(--fs-caption);
    flex-shrink: 0;
  }

  .dc-tool-banner[hidden] {
    display: none;
  }

  /* --- Modo Lienzo --- */
  .dc-stage-wrap {
    flex: 1 1 auto;
    overflow: auto;
    position: relative;
    background-color: var(--color-surface-sunken);
    background-image: radial-gradient(circle at 1px 1px, var(--color-border-translucent) 1px, transparent 1.5px);
    background-size: 20px 20px;
  }

  .dc-stage-wrap[hidden] {
    display: none;
  }

  .dc-stage-sizer {
    position: relative;
    width: var(--sw, 1000px);
    height: var(--sh, 1000px);
  }

  .dc-stage-canvas {
    position: relative;
    width: var(--rw, 1000px);
    height: var(--rh, 1000px);
    transform-origin: 0 0;
    transform: scale(var(--zoom, 0.45));
  }

  .dc-board-wrap {
    position: absolute;
    left: var(--x, 0px);
    top: var(--y, 0px);
    width: var(--w, 390px);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .dc-board-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    font-size: 13px;
    color: var(--color-text-soft);
    padding: 0 2px;
  }

  .dc-board-title {
    font-weight: 600;
    color: var(--color-text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .dc-board-actions {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }

  .dc-chip-btn {
    border: 1px solid var(--color-border-translucent);
    background: var(--color-surface-raised);
    color: var(--color-text-primary);
    font-family: inherit;
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 999px;
    cursor: pointer;
    text-decoration: none;
  }

  .dc-chip-btn:hover {
    border-color: var(--color-border-strong);
    color: var(--color-accent-fg);
  }

  .dc-board-frame {
    position: relative;
    width: var(--w, 390px);
    height: var(--h, 844px);
    border-radius: var(--r, 10px);
    border: 1px solid var(--color-border-translucent);
    background: #FFFFFF;
    box-shadow: 0 12px 30px rgba(0, 0, 0, 0.22);
    overflow: hidden;
  }

  .dc-board-frame iframe {
    display: block;
    width: 100%;
    height: 100%;
    border: 0;
    background: #FFFFFF;
  }

  .dc-note {
    position: absolute;
    left: var(--x, 0px);
    top: var(--y, 0px);
    width: var(--w, auto);
    max-width: var(--mw, none);
    white-space: pre-wrap;
  }

  .dc-note.title1 {
    font-size: 28px;
    font-weight: 700;
    color: var(--color-text-primary);
    letter-spacing: -0.02em;
  }

  .dc-note.card {
    padding: 16px 18px;
    border-radius: 12px;
    font-size: 13px;
    line-height: 1.55;
    border: 1px solid var(--color-border-translucent);
    background: var(--color-surface-raised);
    color: var(--color-text-primary);
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.14);
  }

  .dc-note.green { border-color: var(--color-sev-success-border); background: var(--color-sev-success-bg); }
  .dc-note.orange { border-color: var(--color-sev-warning-border); background: var(--color-sev-warning-bg); }
  .dc-note.teal { border-color: var(--color-sev-info-border); background: var(--color-sev-info-bg); }

  /* --- Modo Prototipo --- */
  .dc-proto-wrap {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    align-items: stretch;
    gap: var(--spacing-4);
    padding: var(--spacing-4);
    overflow: hidden;
  }

  .dc-proto-wrap[hidden] {
    display: none;
  }

  .dc-proto-sidebar {
    width: 248px;
    flex-shrink: 0;
    overflow-y: auto;
    background: var(--color-surface-base);
    border: var(--border-width) solid var(--color-border-translucent);
    border-radius: var(--rounded-sm);
    padding: var(--spacing-3);
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .dc-proto-sidebar h3 {
    margin: 2px 4px 8px;
    font-size: var(--fs-caption);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--color-text-soft);
  }

  .dc-screen-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    width: 100%;
    padding: 7px 9px;
    border: 0;
    border-radius: var(--rounded-xs);
    background: transparent;
    color: var(--color-text-primary);
    font: inherit;
    font-size: var(--fs-caption);
    text-align: left;
    cursor: pointer;
  }

  .dc-screen-item:hover {
    background: var(--color-surface-hover);
  }

  .dc-screen-item[aria-current="true"] {
    background: var(--color-accent-soft);
    color: var(--color-accent-fg);
    font-weight: var(--fw-semibold);
  }

  .dc-proto-stage {
    flex: 1 1 auto;
    min-width: 0;
    min-height: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: auto;
  }

  .dc-device-shell {
    padding: 12px;
    border-radius: 40px;
    background: #14181F;
    border: 2px solid var(--color-border-strong);
    box-shadow: 0 22px 54px rgba(0, 0, 0, 0.45);
    transform-origin: center center;
    transform: scale(var(--proto-scale, 1));
    flex-shrink: 0;
  }

  .dc-device-shell[data-tablet="true"] {
    border-radius: 28px;
  }

  .dc-device-screen {
    position: relative;
    width: var(--proto-w, 390px);
    height: var(--proto-h, 844px);
    border-radius: 28px;
    overflow: hidden;
    background: #FFFFFF;
  }

  .dc-device-shell[data-tablet="true"] .dc-device-screen {
    border-radius: 18px;
  }

  .dc-device-screen iframe {
    display: block;
    width: 100%;
    height: 100%;
    border: 0;
  }

  /* --- Capa de Anotaciones (Pin / Caja / Marcador) --- */
  .dc-ann-overlay {
    position: absolute;
    inset: 0;
    z-index: 4;
    pointer-events: none;
  }

  .dc-ann-overlay[data-tool="pin"],
  .dc-ann-overlay[data-tool="box"],
  .dc-ann-overlay[data-tool="highlight"] {
    pointer-events: auto;
    cursor: crosshair;
    touch-action: none;
  }

  .dc-ann-svg,
  .dc-ann-draft-svg {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    pointer-events: none;
  }

  .dc-ann-pins {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }

  .dc-mark-box {
    fill: rgba(217, 107, 39, 0.16);
    stroke: #D96B27;
    stroke-width: 6;
    vector-effect: non-scaling-stroke;
  }

  .dc-mark-highlight {
    fill: none;
    stroke: rgba(244, 196, 48, 0.58);
    stroke-width: 24;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .dc-mark-applied {
    stroke: #3BA676;
    fill: rgba(59, 166, 118, 0.16);
  }

  .dc-mark-highlight.dc-mark-applied {
    fill: none;
    stroke: rgba(59, 166, 118, 0.52);
  }

  .dc-mark-resolved {
    opacity: 0.32;
  }

  .dc-mark-selected {
    stroke: #F4C430;
    stroke-width: 9;
  }

  .dc-draft-box {
    fill: rgba(244, 196, 48, 0.22);
    stroke: #F4C430;
    stroke-width: 5;
    stroke-dasharray: 12 8;
  }

  .dc-draft-highlight {
    fill: none;
    stroke: rgba(244, 196, 48, 0.65);
    stroke-width: 24;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .dc-draft-pin {
    fill: #F4C430;
    stroke: #06080C;
    stroke-width: 4;
  }

  .dc-pin {
    position: absolute;
    left: var(--px, 50%);
    top: var(--py, 50%);
    transform: translate(-50%, -50%);
    pointer-events: auto;
    min-width: 26px;
    height: 26px;
    padding: 0 6px;
    border-radius: 999px;
    border: 2px solid #06080C;
    background: #D96B27;
    color: #FFFFFF;
    font-family: var(--font-mono);
    font-size: 12px;
    font-weight: 700;
    cursor: pointer;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.45);
  }

  .dc-pin[data-status="applied"] {
    background: #3BA676;
    color: #06080C;
  }

  .dc-pin[data-status="resolved"] {
    background: #4E5A6B;
    opacity: 0.65;
  }

  .dc-pin[data-selected="true"] {
    background: #F4C430;
    color: #06080C;
    transform: translate(-50%, -50%) scale(1.18);
    z-index: 6;
  }

  /* --- Panel lateral derecho de comentarios --- */
  .dc-comments-pane {
    width: 340px;
    max-width: 42vw;
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    background: var(--color-surface-base);
    border-left: var(--border-width) solid var(--color-border-translucent);
    min-height: 0;
  }

  .dc-comments-pane[hidden] {
    display: none;
  }

  @media (max-width: 820px) {
    .dc-comments-pane {
      position: absolute;
      inset: 0 0 0 auto;
      width: min(340px, 88vw);
      max-width: none;
      z-index: 12;
      box-shadow: -12px 0 32px rgba(0, 0, 0, 0.45);
    }
    .dc-proto-sidebar {
      display: none;
    }
  }

  .dc-comments-head {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: var(--spacing-3);
    border-bottom: var(--border-width) solid var(--color-border-translucent);
  }

  .dc-comments-title-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .dc-comments-title {
    margin: 0;
    font-size: var(--fs-small);
    font-weight: var(--fw-semibold);
  }

  .dc-comments-filters {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
  }

  .dc-draft-box-form {
    padding: var(--spacing-3);
    background: var(--color-surface-raised);
    border-bottom: var(--border-width) solid var(--color-accent-ring);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .dc-draft-meta {
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    color: var(--color-text-soft);
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .dc-draft-target {
    color: var(--color-accent-fg);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .dc-textarea {
    width: 100%;
    min-height: 76px;
    padding: 8px 10px;
    border-radius: var(--rounded-sm);
    border: var(--border-width) solid var(--color-border-strong);
    background: var(--color-surface-sunken);
    color: var(--color-text-primary);
    font-family: var(--font-sans);
    font-size: var(--fs-caption);
    resize: vertical;
  }

  .dc-textarea:focus-visible {
    outline: none;
    box-shadow: var(--focus-ring);
  }

  .dc-draft-actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
  }

  .dc-comments-list {
    flex: 1 1 auto;
    overflow-y: auto;
    padding: var(--spacing-3);
    display: flex;
    flex-direction: column;
    gap: var(--spacing-2);
  }

  .dc-comment-card {
    padding: var(--spacing-3);
    border-radius: var(--rounded-sm);
    border: var(--border-width) solid var(--color-border-translucent);
    background: var(--color-surface-raised);
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .dc-comment-card[data-selected="true"] {
    border-color: var(--color-accent-ring);
    background: var(--color-surface-elevated);
  }

  .dc-comment-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    color: var(--color-text-soft);
  }

  .dc-comment-status {
    padding: 1px 6px;
    border-radius: var(--rounded-xs);
    font-weight: 600;
  }

  .dc-comment-status[data-status="open"] {
    background: var(--color-sev-warning-bg);
    color: var(--color-sev-warning-fg);
  }

  .dc-comment-status[data-status="applied"] {
    background: var(--color-sev-success-bg);
    color: var(--color-sev-success-fg);
  }

  .dc-comment-status[data-status="resolved"] {
    background: var(--color-surface-sunken);
    color: var(--color-text-soft);
  }

  .dc-comment-body {
    margin: 0;
    font-size: var(--fs-caption);
    line-height: 1.45;
    color: var(--color-text-primary);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .dc-comment-res {
    margin: 0;
    padding: 6px 8px;
    border-radius: var(--rounded-xs);
    background: var(--color-sev-success-bg);
    color: var(--color-sev-success-fg);
    font-size: var(--fs-micro);
    font-family: var(--font-mono);
  }

  .dc-comment-actions {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }

  .dc-empty {
    padding: var(--spacing-6);
    text-align: center;
    color: var(--color-text-soft);
    font-size: var(--fs-caption);
  }
`);

const TOOL_LABELS = {
  interact: 'Interactuar',
  pin: 'Pin',
  box: 'Caja',
  highlight: 'Marcador'
};

const TOOL_HINTS = {
  pin: 'Haz clic sobre cualquier parte de una pantalla para colocar un pin de comentario.',
  box: 'Arrastra sobre el área que quieres cambiar para dibujar una caja y comentar.',
  highlight: 'Arrastra libremente sobre la pantalla para marcar con resaltador y comentar.'
};

const STATUS_LABELS = {
  open: 'Abierto',
  applied: 'Aplicado',
  resolved: 'Resuelto'
};

export class DxDesignCanvas extends HTMLElement {
  constructor() {
    super();
    this._project = '';
    this._canvases = [];
    this._canvas = null;
    this._manifest = null;
    this._baseHref = '';
    this._annotations = [];
    this._currentPage = '';
    this._mode = 'canvas'; // 'canvas' | 'proto'
    this._zoom = 0.45;
    this._tool = 'interact'; // 'interact' | 'pin' | 'box' | 'highlight'
    this._protoFile = '';
    this._commentsOpen = true;
    this._statusFilter = 'open'; // 'open' | 'applied' | 'all'
    this._selectedAnnId = '';
    this._draft = null;
    this._overlaysByBoard = new Map();
    this._boardWraps = new Map();
    this._iframesByBoard = new Map();
    this._protoOverlay = null;
    this._protoIframe = null;
    this._cacheBust = 0;

    this._onWindowMsg = (e) => this._handleIframeNav(e);
    this._onResize = () => {
      if (this._mode === 'proto' && this._dialog?.open) this._fitProtoDevice(this._protoFile);
    };

    this.attachShadow({ mode: 'open' });
    adoptSheets(this.shadowRoot, designCanvasSheet);
    this._buildShell();
  }

  connectedCallback() {
    window.addEventListener('message', this._onWindowMsg);
    window.addEventListener('resize', this._onResize);
  }

  disconnectedCallback() {
    window.removeEventListener('message', this._onWindowMsg);
    window.removeEventListener('resize', this._onResize);
  }

  get isOpen() {
    return Boolean(this._dialog?.open);
  }

  _buildShell() {
    clearNode(this.shadowRoot);
    this._dialog = el('dialog', { className: 'dc-modal', 'aria-label': 'Lienzo de diseño' });
    this._dialog.addEventListener('cancel', () => {
      this._draft = null;
    });

    // Header
    this._titleEl = el('span', { className: 'dc-title' }, 'Lienzo');
    this._subEl = el('span', { className: 'dc-sub' });
    this._canvasSelect = el('select', { className: 'dc-select', 'aria-label': 'Elegir lienzo', hidden: true });
    this._canvasSelect.addEventListener('change', () => {
      if (this._canvasSelect.value) void this._loadManifest(this._project, this._canvasSelect.value);
    });
    const brand = el('div', { className: 'dc-brand' }, createUiIcon('layers', 16), this._titleEl, this._canvasSelect, this._subEl);

    this._pageTabs = el('div', { className: 'dc-tabs', role: 'group', 'aria-label': 'Versiones del lienzo' });

    this._toolButtons = new Map();
    const toolsWrap = el('div', { className: 'dc-tools', role: 'group', 'aria-label': 'Herramienta de anotación' });
    for (const [id, label] of Object.entries(TOOL_LABELS)) {
      const btn = el('button', {
        type: 'button',
        className: 'dc-tool-btn',
        'aria-pressed': id === this._tool ? 'true' : 'false'
      }, label);
      btn.addEventListener('click', () => this._setTool(id));
      this._toolButtons.set(id, btn);
      toolsWrap.appendChild(btn);
    }

    const btnOut = el('button', { type: 'button', className: 'dc-btn dc-btn-icon', title: 'Alejar', 'aria-label': 'Alejar' }, createUiIcon('minus', 14));
    this._zoomValEl = el('span', { className: 'dc-zoom-val' }, '45%');
    const btnIn = el('button', { type: 'button', className: 'dc-btn dc-btn-icon', title: 'Acercar', 'aria-label': 'Acercar' }, createUiIcon('plus', 14));
    const btnFit = el('button', { type: 'button', className: 'dc-btn', title: 'Restablecer zoom' }, 'Ajustar');
    btnOut.addEventListener('click', () => { this._zoom = Math.max(0.2, +(this._zoom - 0.1).toFixed(2)); this._applyZoom(); });
    btnIn.addEventListener('click', () => { this._zoom = Math.min(1.4, +(this._zoom + 0.1).toFixed(2)); this._applyZoom(); });
    btnFit.addEventListener('click', () => { this._zoom = 0.45; this._applyZoom(); });

    this._modeBtn = el('button', { type: 'button', className: 'dc-btn dc-btn-primary' }, 'Prototipo');
    this._modeBtn.addEventListener('click', () => this._toggleMode());

    this._commentsCountBadge = el('span', { className: 'dc-badge', dataset: { zero: 'true' } }, '0');
    this._commentsToggleBtn = el('button', { type: 'button', className: 'dc-btn' }, 'Comentarios', this._commentsCountBadge);
    this._commentsToggleBtn.addEventListener('click', () => {
      this._commentsOpen = !this._commentsOpen;
      this._commentsPane.hidden = !this._commentsOpen;
      if (this._mode === 'proto') requestAnimationFrame(() => this._fitProtoDevice(this._protoFile));
    });

    const reloadBtn = el('button', { type: 'button', className: 'dc-btn dc-btn-icon', title: 'Recargar pantallas', 'aria-label': 'Recargar pantallas' }, createUiIcon('history', 14));
    reloadBtn.addEventListener('click', () => {
      this._cacheBust = Date.now();
      if (this._canvas) void this._loadManifest(this._canvas.projectKey, this._canvas.canvasId);
    });

    const closeBtn = el('button', { type: 'button', className: 'dc-btn dc-btn-icon', title: 'Cerrar (Esc)', 'aria-label': 'Cerrar lienzo' }, createUiIcon('close', 14));
    closeBtn.addEventListener('click', () => this.close());

    const controls = el('div', { className: 'dc-controls' },
      toolsWrap, btnOut, this._zoomValEl, btnIn, btnFit, this._modeBtn, this._commentsToggleBtn, reloadBtn, closeBtn);

    const header = el('header', { className: 'dc-header' }, brand, this._pageTabs, controls);

    // Banner de modo anotación activo
    this._toolBannerText = el('span', null, '');
    const backInteractBtn = el('button', { type: 'button', className: 'dc-btn' }, 'Volver a interactuar');
    backInteractBtn.addEventListener('click', () => this._setTool('interact'));
    this._toolBanner = el('div', { className: 'dc-tool-banner', hidden: true }, this._toolBannerText, backInteractBtn);

    // Lienzo
    this._stageCanvas = el('div', { className: 'dc-stage-canvas' });
    this._stageSizer = el('div', { className: 'dc-stage-sizer' }, this._stageCanvas);
    this._stageWrap = el('div', { className: 'dc-stage-wrap' }, this._stageSizer);

    // Prototipo
    this._protoList = el('aside', { className: 'dc-proto-sidebar' });
    this._protoIframe = el('iframe', {
      sandbox: 'allow-scripts',
      referrerpolicy: 'no-referrer',
      title: 'Prototipo interactivo',
      src: 'about:blank'
    });
    this._protoScreen = el('div', { className: 'dc-device-screen' }, this._protoIframe);
    this._deviceShell = el('div', { className: 'dc-device-shell' }, this._protoScreen);
    this._protoStage = el('div', { className: 'dc-proto-stage' }, this._deviceShell);
    this._protoWrap = el('section', { className: 'dc-proto-wrap', hidden: true }, this._protoList, this._protoStage);

    const main = el('div', { className: 'dc-main' }, this._toolBanner, this._stageWrap, this._protoWrap);

    // Panel derecho de comentarios
    this._draftHost = el('div');
    this._commentsListEl = el('div', { className: 'dc-comments-list' });
    this._filterButtons = new Map();
    const filterRow = el('div', { className: 'dc-comments-filters' });
    for (const [fKey, fLbl] of [['open', 'Abiertos'], ['applied', 'Aplicados'], ['all', 'Todos']]) {
      const fb = el('button', {
        type: 'button',
        className: 'dc-tab',
        'aria-pressed': fKey === this._statusFilter ? 'true' : 'false'
      }, fLbl);
      fb.addEventListener('click', () => {
        this._statusFilter = fKey;
        for (const [k, node] of this._filterButtons) node.setAttribute('aria-pressed', k === fKey ? 'true' : 'false');
        this._renderCommentsPane();
      });
      this._filterButtons.set(fKey, fb);
      filterRow.appendChild(fb);
    }
    const commentsHead = el('div', { className: 'dc-comments-head' },
      el('div', { className: 'dc-comments-title-row' }, el('h2', { className: 'dc-comments-title' }, 'Comentarios para agentes')),
      filterRow);
    this._commentsPane = el('aside', { className: 'dc-comments-pane' }, commentsHead, this._draftHost, this._commentsListEl);

    const body = el('div', { className: 'dc-body' }, main, this._commentsPane);
    this._dialog.append(header, body);
    this.shadowRoot.appendChild(this._dialog);
  }

  async open({ project, canvasId = '' }) {
    this._project = String(project || '').trim();
    if (!this._project) return;
    if (!this._dialog.open) this._dialog.showModal();
    this._titleEl.textContent = 'Cargando lienzo…';
    this._subEl.textContent = '';

    try {
      const resp = await fetch(`/api/canvases?project=${encodeURIComponent(this._project)}`, { cache: 'no-store', headers: { Accept: 'application/json' } });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      const list = Array.isArray(data.canvases) ? data.canvases : [];
      this._canvases = list;
      if (!list.length) {
        this._titleEl.textContent = `Sin lienzo en ${this._project}`;
        clearNode(this._stageCanvas);
        this._stageCanvas.appendChild(el('div', { className: 'dc-empty' }, 'No hay lienzos registrados para este workspace.'));
        return;
      }
      clearNode(this._canvasSelect);
      for (const c of list) {
        this._canvasSelect.appendChild(el('option', { value: c.canvasId }, c.title || c.canvasId));
      }
      this._canvasSelect.hidden = list.length <= 1;
      const chosen = canvasId && list.some((c) => c.canvasId === canvasId) ? canvasId : list[0].canvasId;
      this._canvasSelect.value = chosen;
      await this._loadManifest(list.find((c) => c.canvasId === chosen).projectKey, chosen);
    } catch (err) {
      this._titleEl.textContent = 'Error cargando lienzo';
      clearNode(this._stageCanvas);
      this._stageCanvas.appendChild(el('div', { className: 'dc-empty' }, String(err.message || err)));
    }
  }

  close() {
    this._draft = null;
    if (this._dialog?.open) this._dialog.close();
  }

  async handleSSE(eventName, detail) {
    if (!this.isOpen || !this._canvas) return;
    if (detail?.project && detail.project !== this._canvas.projectKey) return;
    if (detail?.canvasId && detail.canvasId !== this._canvas.canvasId) return;

    if (eventName === 'canvas_updated') {
      this._cacheBust = Date.now();
      await this._loadManifest(this._canvas.projectKey, this._canvas.canvasId);
      return;
    }
    if (eventName === 'canvas_annotations_updated') {
      await this._reloadAnnotationsOnly();
      if (detail?.status === 'applied' && detail?.boardFile) {
        this._cacheBust = Date.now();
        this._reloadBoardIframe(detail.boardFile);
      }
    }
  }

  async _loadManifest(projectKey, canvasId) {
    const url = `/api/canvases/${encodeURIComponent(projectKey)}/${encodeURIComponent(canvasId)}/manifest`;
    const resp = await fetch(url, { cache: 'no-store', headers: { Accept: 'application/json' } });
    if (!resp.ok) throw new Error(`No se pudo abrir manifiesto (HTTP ${resp.status})`);
    const data = await resp.json();
    this._canvas = data.canvas;
    this._manifest = data.manifest || {};
    this._baseHref = data.baseHref || '';
    this._annotations = Array.isArray(data.annotations) ? data.annotations : [];

    this._titleEl.textContent = this._manifest.title || this._canvas.title || this._canvas.canvasId;
    const pages = Array.isArray(this._manifest.pages) ? this._manifest.pages : [];
    const launchPage = this._manifest.launch?.page;
    if (!this._currentPage || !pages.some((p) => p.id === this._currentPage)) {
      this._currentPage = launchPage && pages.some((p) => p.id === launchPage) ? launchPage : (pages[0]?.id || '');
    }

    this._renderPageTabs();
    this._renderPage();
    this._renderCommentsPane();
  }

  async _reloadAnnotationsOnly() {
    if (!this._canvas) return;
    const url = `/api/canvases/${encodeURIComponent(this._canvas.projectKey)}/${encodeURIComponent(this._canvas.canvasId)}/annotations?status=all`;
    try {
      const resp = await fetch(url, { cache: 'no-store', headers: { Accept: 'application/json' } });
      if (!resp.ok) return;
      const data = await resp.json();
      this._annotations = Array.isArray(data.annotations) ? data.annotations : [];
      this._syncAllOverlays();
      this._renderCommentsPane();
    } catch {
      // noop
    }
  }

  _assetSrc(file) {
    const clean = encodeURI(file);
    return this._cacheBust ? `${this._baseHref}${clean}?v=${this._cacheBust}` : `${this._baseHref}${clean}`;
  }

  _reloadBoardIframe(boardFile) {
    const iframe = this._iframesByBoard.get(boardFile);
    if (iframe) iframe.src = this._assetSrc(boardFile);
    if (this._mode === 'proto' && this._protoFile === boardFile && this._protoIframe) {
      this._protoIframe.src = this._assetSrc(boardFile);
    }
  }

  _setTool(tool) {
    this._tool = tool;
    for (const [id, btn] of this._toolButtons) {
      btn.setAttribute('aria-pressed', id === tool ? 'true' : 'false');
      if (id === tool && id !== 'interact') btn.dataset.toolActive = 'annotate';
      else delete btn.dataset.toolActive;
    }
    if (tool === 'interact') {
      this._toolBanner.hidden = true;
    } else {
      this._toolBannerText.textContent = TOOL_HINTS[tool] || '';
      this._toolBanner.hidden = false;
    }
    for (const ov of this._overlaysByBoard.values()) ov.setTool(tool);
    if (this._protoOverlay) this._protoOverlay.setTool(tool);
  }

  _renderPageTabs() {
    clearNode(this._pageTabs);
    const pages = Array.isArray(this._manifest?.pages) ? this._manifest.pages : [];
    for (const p of pages) {
      const btn = el('button', {
        type: 'button',
        className: 'dc-tab',
        'aria-pressed': p.id === this._currentPage ? 'true' : 'false'
      }, p.name || p.id);
      btn.addEventListener('click', () => {
        this._currentPage = p.id;
        this._renderPageTabs();
        this._renderPage();
      });
      this._pageTabs.appendChild(btn);
    }
  }

  _applyZoom() {
    this._zoomValEl.textContent = `${Math.round(this._zoom * 100)}%`;
    this._stageCanvas.style.setProperty('--zoom', String(this._zoom));
    this._stageSizer.style.setProperty('--sw', `${Math.ceil((this._rawW || 1000) * this._zoom)}px`);
    this._stageSizer.style.setProperty('--sh', `${Math.ceil((this._rawH || 1000) * this._zoom)}px`);
  }

  _boardsForCurrentPage() {
    const manifest = this._manifest || {};
    const boardsMap = manifest.boards || {};
    const order = Array.isArray(manifest.order) ? manifest.order : Object.keys(boardsMap);
    return order
      .filter((f) => boardsMap[f] && (!this._currentPage || boardsMap[f].page === this._currentPage))
      .map((f) => ({ file: f, ...boardsMap[f] }));
  }

  _renderPage() {
    const boards = this._boardsForCurrentPage();
    const notes = Object.entries(this._manifest?.notes || {})
      .filter(([, n]) => !this._currentPage || n.page === this._currentPage)
      .map(([id, n]) => ({ id, ...n }));

    this._subEl.textContent = `${boards.length} ${boards.length === 1 ? 'tablero' : 'tableros'}`;
    this._overlaysByBoard.clear();
    this._boardWraps.clear();
    this._iframesByBoard.clear();
    clearNode(this._stageCanvas);

    if (boards.length === 0) {
      this._stageCanvas.appendChild(el('div', { className: 'dc-empty' }, 'Esta página no tiene tableros.'));
      return;
    }

    let minX = Infinity;
    let minY = Infinity;
    let maxX = -Infinity;
    let maxY = -Infinity;
    for (const b of boards) {
      minX = Math.min(minX, b.x || 0);
      minY = Math.min(minY, (b.y || 0) - 40);
      maxX = Math.max(maxX, (b.x || 0) + (b.w || 390));
      maxY = Math.max(maxY, (b.y || 0) + (b.h || 844));
    }
    for (const n of notes) {
      minX = Math.min(minX, n.x || 0);
      minY = Math.min(minY, n.y || 0);
      maxX = Math.max(maxX, (n.x || 0) + (n.w || 400));
      maxY = Math.max(maxY, (n.y || 0) + 200);
    }
    const pad = 80;
    const ox = -minX + pad;
    const oy = -minY + pad;
    this._rawW = maxX - minX + pad * 2;
    this._rawH = maxY - minY + pad * 2;
    this._stageCanvas.style.setProperty('--rw', `${this._rawW}px`);
    this._stageCanvas.style.setProperty('--rh', `${this._rawH}px`);

    for (const n of notes) {
      const cls = n.kind === 'title1' ? 'dc-note title1' : `dc-note card ${n.fill || ''}`.trim();
      const noteEl = el('div', { className: cls }, n.text || '');
      noteEl.style.setProperty('--x', `${(n.x || 0) + ox}px`);
      noteEl.style.setProperty('--y', `${(n.y || 0) + oy}px`);
      if (n.w) noteEl.style.setProperty('--w', `${n.w}px`);
      if (n.maxW) noteEl.style.setProperty('--mw', `${n.maxW}px`);
      this._stageCanvas.appendChild(noteEl);
    }

    for (const b of boards) {
      const w = b.w || 390;
      const h = b.h || 844;
      const wrap = el('div', { className: 'dc-board-wrap', dataset: { board: b.file } });
      wrap.style.setProperty('--x', `${(b.x || 0) + ox}px`);
      wrap.style.setProperty('--y', `${(b.y || 0) + oy - 34}px`);
      wrap.style.setProperty('--w', `${w}px`);

      const actions = el('div', { className: 'dc-board-actions' });
      if (b.is_interactive) {
        const tryBtn = el('button', { type: 'button', className: 'dc-chip-btn' }, 'Probar');
        tryBtn.addEventListener('click', () => this._openProto(b.file));
        actions.appendChild(tryBtn);
      }
      const openLink = el('a', {
        className: 'dc-chip-btn',
        href: this._assetSrc(b.file),
        target: '_blank',
        rel: 'noopener'
      }, 'Abrir');
      actions.appendChild(openLink);

      const head = el('div', { className: 'dc-board-head' },
        el('span', { className: 'dc-board-title', title: b.title || b.file }, b.title || b.file),
        actions);

      const frameBox = el('div', { className: 'dc-board-frame' });
      frameBox.style.setProperty('--w', `${w}px`);
      frameBox.style.setProperty('--h', `${h}px`);
      if (b.radius) frameBox.style.setProperty('--r', `${b.radius}px`);

      const iframe = el('iframe', {
        sandbox: 'allow-scripts',
        referrerpolicy: 'no-referrer',
        loading: 'lazy',
        title: b.title || b.file,
        src: this._assetSrc(b.file)
      });
      frameBox.appendChild(iframe);
      this._iframesByBoard.set(b.file, iframe);

      const overlay = new BoardAnnotationOverlay({
        host: frameBox,
        iframe,
        boardFile: b.file,
        pageId: this._currentPage,
        getTool: () => this._tool,
        onDraftReady: (draft) => this._startDraft(draft),
        onSelectAnnotation: (ann) => this._selectAnnotation(ann, false)
      });
      overlay.setTool(this._tool);
      this._overlaysByBoard.set(b.file, overlay);
      this._boardWraps.set(b.file, wrap);

      wrap.append(head, frameBox);
      this._stageCanvas.appendChild(wrap);
    }

    // Lista lateral para modo Prototipo
    const interactiveBoards = boards.filter((b) => b.is_interactive);
    const protoBoards = interactiveBoards.length ? interactiveBoards : boards;
    clearNode(this._protoList);
    this._protoList.appendChild(el('h3', null, `Pantallas (${this._currentPage || 'todas'})`));
    for (const b of protoBoards) {
      const btn = el('button', {
        type: 'button',
        className: 'dc-screen-item',
        dataset: { file: b.file },
        'aria-current': b.file === this._protoFile ? 'true' : null
      }, el('span', null, b.title || b.file));
      btn.addEventListener('click', () => this._openProto(b.file));
      this._protoList.appendChild(btn);
    }
    if (protoBoards.length && !protoBoards.some((b) => b.file === this._protoFile)) {
      this._protoFile = protoBoards[0].file;
    }
    if (this._mode === 'proto' && this._protoFile) {
      this._openProto(this._protoFile);
    }

    this._applyZoom();
    this._syncAllOverlays();
  }

  _toggleMode() {
    if (this._mode === 'canvas') {
      this._openProto(this._protoFile || this._boardsForCurrentPage()[0]?.file || '');
    } else {
      this._mode = 'canvas';
      this._protoWrap.hidden = true;
      this._stageWrap.hidden = false;
      this._modeBtn.textContent = 'Prototipo';
    }
  }

  _openProto(file) {
    if (!file) return;
    this._protoFile = file;
    this._mode = 'proto';
    this._stageWrap.hidden = true;
    this._protoWrap.hidden = false;
    this._modeBtn.textContent = 'Ver lienzo';
    this._protoIframe.src = this._assetSrc(file);
    this._fitProtoDevice(file);
    for (const item of this._protoList.querySelectorAll('.dc-screen-item')) {
      if (item.dataset.file === file) item.setAttribute('aria-current', 'true');
      else item.removeAttribute('aria-current');
    }
    // Re-anclar overlay de anotaciones al dispositivo actual
    if (this._protoOverlay) {
      this._protoOverlay.root.remove();
    }
    this._protoOverlay = new BoardAnnotationOverlay({
      host: this._protoScreen,
      iframe: this._protoIframe,
      boardFile: file,
      pageId: this._currentPage,
      getTool: () => this._tool,
      onDraftReady: (draft) => this._startDraft(draft),
      onSelectAnnotation: (ann) => this._selectAnnotation(ann, false)
    });
    this._protoOverlay.setTool(this._tool);
    this._syncAllOverlays();
  }

  _fitProtoDevice(file) {
    const b = this._manifest?.boards?.[file] || { w: 390, h: 844 };
    const w = b.w || 390;
    const h = b.h || 844;
    this._deviceShell.dataset.tablet = w > 600 ? 'true' : 'false';
    this._protoScreen.style.setProperty('--proto-w', `${w}px`);
    this._protoScreen.style.setProperty('--proto-h', `${h}px`);
    const availW = Math.max(280, this._protoStage.clientWidth - 40);
    const availH = Math.max(280, this._protoStage.clientHeight - 40);
    const scale = Math.min(1, availW / (w + 28), availH / (h + 28));
    this._deviceShell.style.setProperty('--proto-scale', scale < 0.99 ? scale.toFixed(3) : '1');
  }

  _handleIframeNav(e) {
    if (!this.isOpen || !this._protoIframe || e.source !== this._protoIframe.contentWindow) return;
    const nav = e.data?.dxCanvasNav || (e.data?.type === 'tsugi-screen' ? e.data : null);
    if (!nav?.file || nav.file === this._protoFile) return;
    if (this._manifest?.boards?.[nav.file]) {
      this._protoFile = nav.file;
      this._fitProtoDevice(nav.file);
      for (const item of this._protoList.querySelectorAll('.dc-screen-item')) {
        if (item.dataset.file === nav.file) item.setAttribute('aria-current', 'true');
        else item.removeAttribute('aria-current');
      }
      if (this._protoOverlay) {
        this._protoOverlay.boardFile = nav.file;
        this._syncAllOverlays();
      }
    }
  }

  _annotationIndexMap() {
    const map = new Map();
    this._annotations.forEach((a, i) => map.set(a.id, i + 1));
    return map;
  }

  _syncAllOverlays() {
    const idxMap = this._annotationIndexMap();
    const byBoard = new Map();
    for (const a of this._annotations) {
      if (!byBoard.has(a.boardFile)) byBoard.set(a.boardFile, []);
      byBoard.get(a.boardFile).push(a);
    }
    for (const [boardFile, ov] of this._overlaysByBoard) {
      ov.setAnnotations(byBoard.get(boardFile) || [], idxMap, this._selectedAnnId);
    }
    if (this._protoOverlay) {
      this._protoOverlay.setAnnotations(byBoard.get(this._protoOverlay.boardFile) || [], idxMap, this._selectedAnnId);
    }
    const openCount = this._annotations.filter((a) => a.status === 'open').length;
    this._commentsCountBadge.textContent = String(openCount);
    this._commentsCountBadge.dataset.zero = openCount === 0 ? 'true' : 'false';
  }

  _startDraft(draft) {
    this._draft = draft;
    this._commentsOpen = true;
    this._commentsPane.hidden = false;
    this._renderDraftForm();
  }

  _renderDraftForm() {
    clearNode(this._draftHost);
    if (!this._draft) return;
    const d = this._draft;
    const kindLbl = TOOL_LABELS[d.kind] || d.kind;
    const meta = el('div', { className: 'dc-draft-meta' },
      el('span', null, `Nueva marca (${kindLbl}) · ${d.boardFile}`),
      d.targetSelector || d.targetText
        ? el('span', { className: 'dc-draft-target', title: `${d.targetSelector} ${d.targetText}` },
          `Elemento: ${d.targetSelector || 'texto'}${d.targetText ? ` "${d.targetText.slice(0, 48)}"` : ''}`)
        : null);
    const textarea = el('textarea', {
      className: 'dc-textarea',
      placeholder: 'Escribe qué cambiar aquí (ej. "Cambiar este botón por uno más compacto con el precio")…'
    });
    const saveBtn = el('button', { type: 'button', className: 'dc-btn dc-btn-primary' }, 'Guardar comentario');
    const cancelBtn = el('button', { type: 'button', className: 'dc-btn' }, 'Cancelar');

    const submit = async () => {
      const text = textarea.value.trim();
      if (!text || !this._canvas) return;
      saveBtn.disabled = true;
      try {
        const url = `/api/canvases/${encodeURIComponent(this._canvas.projectKey)}/${encodeURIComponent(this._canvas.canvasId)}/annotations`;
        const resp = await fetch(url, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({
            pageId: d.pageId,
            boardFile: d.boardFile,
            kind: d.kind,
            geometry: d.geometry,
            targetSelector: d.targetSelector || '',
            targetText: d.targetText || '',
            comment: text,
            author: 'usuario'
          })
        });
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const data = await resp.json();
        d.clearDraft?.();
        this._draft = null;
        clearNode(this._draftHost);
        if (data.annotation) {
          this._annotations.push(data.annotation);
          this._selectedAnnId = data.annotation.id;
        } else {
          await this._reloadAnnotationsOnly();
        }
        this._syncAllOverlays();
        this._renderCommentsPane();
      } catch (err) {
        saveBtn.disabled = false;
        meta.appendChild(el('span', { className: 'dc-draft-target' }, `Error: ${err.message || err}`));
      }
    };

    saveBtn.addEventListener('click', submit);
    cancelBtn.addEventListener('click', () => {
      d.clearDraft?.();
      this._draft = null;
      clearNode(this._draftHost);
    });
    textarea.addEventListener('keydown', (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault();
        void submit();
      }
    });

    const form = el('div', { className: 'dc-draft-box-form' },
      meta,
      textarea,
      el('div', { className: 'dc-draft-actions' }, cancelBtn, saveBtn));
    this._draftHost.appendChild(form);
    requestAnimationFrame(() => textarea.focus());
  }

  _selectAnnotation(ann, scrollToBoard = true) {
    this._selectedAnnId = ann.id;
    this._commentsOpen = true;
    this._commentsPane.hidden = false;
    if (scrollToBoard) {
      if (ann.pageId && ann.pageId !== this._currentPage) {
        this._currentPage = ann.pageId;
        this._renderPageTabs();
        this._renderPage();
      }
      if (this._mode === 'proto') {
        if (this._protoFile !== ann.boardFile) this._openProto(ann.boardFile);
      } else {
        const wrap = this._boardWraps.get(ann.boardFile);
        wrap?.scrollIntoView?.({ behavior: 'smooth', block: 'center', inline: 'center' });
      }
    }
    this._syncAllOverlays();
    this._renderCommentsPane();
  }

  async _updateAnnotationStatus(ann, nextStatus) {
    if (!this._canvas) return;
    const url = `/api/canvases/${encodeURIComponent(this._canvas.projectKey)}/${encodeURIComponent(this._canvas.canvasId)}/annotations/${encodeURIComponent(ann.id)}`;
    const resp = await fetch(url, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ status: nextStatus })
    });
    if (resp.ok) {
      const data = await resp.json();
      if (data.annotation) {
        const idx = this._annotations.findIndex((a) => a.id === ann.id);
        if (idx >= 0) this._annotations[idx] = data.annotation;
      }
      this._syncAllOverlays();
      this._renderCommentsPane();
    }
  }

  async _deleteAnnotation(ann) {
    if (!this._canvas) return;
    const url = `/api/canvases/${encodeURIComponent(this._canvas.projectKey)}/${encodeURIComponent(this._canvas.canvasId)}/annotations/${encodeURIComponent(ann.id)}`;
    const resp = await fetch(url, { method: 'DELETE', headers: { Accept: 'application/json' } });
    if (resp.ok) {
      this._annotations = this._annotations.filter((a) => a.id !== ann.id);
      if (this._selectedAnnId === ann.id) this._selectedAnnId = '';
      this._syncAllOverlays();
      this._renderCommentsPane();
    }
  }

  _renderCommentsPane() {
    clearNode(this._commentsListEl);
    const idxMap = this._annotationIndexMap();
    const list = this._annotations.filter((a) => this._statusFilter === 'all' || a.status === this._statusFilter);

    if (list.length === 0) {
      this._commentsListEl.appendChild(el('div', { className: 'dc-empty' },
        this._statusFilter === 'open'
          ? 'Sin comentarios abiertos. Usa Pin, Caja o Marcador arriba para señalar cambios al agente.'
          : 'Sin comentarios en este filtro.'));
      return;
    }

    for (const ann of list) {
      const num = idxMap.get(ann.id) ?? '?';
      const card = el('article', {
        className: 'dc-comment-card',
        dataset: { selected: ann.id === this._selectedAnnId ? 'true' : 'false' }
      },
      el('div', { className: 'dc-comment-top' },
        el('span', null, `#${num} · ${ann.boardFile} · ${TOOL_LABELS[ann.kind] || ann.kind}`),
        el('span', { className: 'dc-comment-status', dataset: { status: ann.status } }, STATUS_LABELS[ann.status] || ann.status)),
      ann.targetSelector || ann.targetText
        ? el('div', { className: 'dc-draft-target', title: `${ann.targetSelector} ${ann.targetText}` },
          `${ann.targetSelector || 'elemento'}${ann.targetText ? ` · "${ann.targetText.slice(0, 48)}"` : ''}`)
        : null,
      el('p', { className: 'dc-comment-body' }, ann.comment),
      ann.resolutionNote ? el('p', { className: 'dc-comment-res' }, `Agente: ${ann.resolutionNote}`) : null);

      const btnGo = el('button', { type: 'button', className: 'dc-chip-btn' }, 'Ubicar');
      btnGo.addEventListener('click', () => this._selectAnnotation(ann, true));

      const nextStatus = ann.status === 'resolved' ? 'open' : 'resolved';
      const btnToggle = el('button', { type: 'button', className: 'dc-chip-btn' }, ann.status === 'resolved' ? 'Reabrir' : 'Resolver');
      btnToggle.addEventListener('click', () => void this._updateAnnotationStatus(ann, nextStatus));

      const btnDel = el('button', { type: 'button', className: 'dc-chip-btn' }, 'Borrar');
      btnDel.addEventListener('click', () => void this._deleteAnnotation(ann));

      card.appendChild(el('div', { className: 'dc-comment-actions' }, btnGo, btnToggle, btnDel));
      this._commentsListEl.appendChild(card);
    }
  }
}

if (!customElements.get('dx-design-canvas')) {
  customElements.define('dx-design-canvas', DxDesignCanvas);
}
