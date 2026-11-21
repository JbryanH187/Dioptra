// ==========================================================================
// DioptraEngine Viewer — ui/components/free-canvas.js
// <dx-free-canvas> (§4.9 Contrato A — Montaje dual Go /canvas/... vs preview srcdoc,
// sandbox="allow-scripts" con origen opaco aislado y sincronización postMessage)
// ==========================================================================

import { adoptSheets, baseBlockSheet, clearNode, createSheet, createUiIcon, el } from './shared.js';

const CANVAS_TOKENS = [
  '--color-bg-body',
  '--color-surface-base',
  '--color-surface-elevated',
  '--color-border',
  '--color-border-translucent',
  '--color-bg-inactive',
  '--color-text-primary',
  '--color-text-soft',
  '--color-text-disabled',
  '--color-accent-fg',
  '--color-on-primary',
  '--color-interactive-primary',
  '--color-interactive-primary-hover',
  '--color-interactive-primary-press',
  '--color-interactive-secondary',
  '--color-interactive-secondary-hover',
  '--color-interactive-secondary-press',
  '--color-sev-info',
  '--color-sev-success',
  '--color-sev-warning',
  '--color-sev-error',
  '--color-sev-info-fg',
  '--color-sev-success-fg',
  '--color-sev-warning-fg',
  '--color-sev-error-fg',
  '--color-sev-info-bg',
  '--color-sev-success-bg',
  '--color-sev-warning-bg',
  '--color-sev-error-bg',
  '--shadow-default',
  '--shadow-hover',
  '--shadow-inset',
  '--rounded-sm',
  '--rounded-md',
  '--rounded-full',
  '--font-sans',
  '--font-mono',
  '--fw-regular',
  '--fw-medium',
  '--fw-semibold',
  '--fs-caption',
  '--fs-small',
  '--fs-body',
  '--fs-subheader',
  '--fs-h3',
  '--lh-heading',
  '--lh-body',
  '--spacing-1',
  '--spacing-2',
  '--spacing-3',
  '--spacing-4',
  '--spacing-5',
  '--spacing-6',
  '--motion-duration-fast',
  '--motion-duration-normal',
  '--motion-easing-standard',
  '--motion-easing-snappy'
];

// Bloques de ambos temas inyectados en modo V (preview.html) para que [data-theme] funcione
// de inmediato incluso sin servidor Go, más variables computadas en vivo.
const DUAL_THEME_BASE_CSS = `
:root, [data-theme="dark"] {
  color-scheme: dark;
  --color-ink: #06080C;
  --color-ink-shadow: #040609;
  --color-bg-body: #0B1017;
  --color-surface-sunken: #0B1017;
  --color-surface-base: #121A24;
  --color-surface-raised: #172230;
  --color-surface-elevated: #1E2C3D;
  --color-border: var(--color-ink);
  --color-border-translucent: rgba(214, 203, 182, 0.18);
  --color-bg-inactive: #1E2C3D;
  --color-text-primary: #F1E9D8;
  --color-text-secondary: #D6CBB6;
  --color-text-soft: #ADA390;
  --color-text-disabled: #5C5446;
  --color-accent-fg: #EE823A;
  --color-on-primary: #06080C;
  --color-interactive-primary: #D96B27;
  --color-interactive-primary-hover: #EE823A;
  --color-interactive-primary-press: #C25A1E;
  --color-interactive-secondary: #172230;
  --color-interactive-secondary-hover: #1E2C3D;
  --color-interactive-secondary-press: #0B1017;
  --color-border-strong: #825F2E;
  --color-selection-bg: rgba(217, 107, 39, 0.24);
  --color-row-hover-bg: rgba(229, 178, 92, 0.06);
  --color-row-selected-bg: rgba(217, 107, 39, 0.12);
  --color-copper: #D96B27;
  --color-bronze: #C88A36;
  --color-gold: #F4C430;
  --font-display: "Cinzel", "Trajan Pro", "Cormorant SC", "Palatino Linotype", Georgia, serif;
  --color-sev-info: #A8B3C2;
  --color-sev-success: #3BA676;
  --color-sev-warning: #F4C430;
  --color-sev-error: #D94A38;
  --color-sev-info-fg: #A8B3C2;
  --color-sev-success-fg: #6FE3AE;
  --color-sev-warning-fg: #F4C430;
  --color-sev-error-fg: #FF8A7A;
  --color-sev-info-bg: #272F3A;
  --color-sev-success-bg: #182E2F;
  --color-sev-warning-bg: #323226;
  --color-sev-error-bg: #2E2127;
  --shadow-default: 2px 2px 0 var(--color-ink-shadow);
  --shadow-hover: 3px 4px 0 var(--color-ink-shadow);
  --shadow-inset: inset 0 1px 0 rgba(255, 224, 170, 0.16), inset 0 -2px 0 rgba(4, 6, 9, 0.75);
  --rounded-sm: 2px;
  --rounded-md: 3px;
  --rounded-full: 999px;
  --font-sans: "Poppins", system-ui, -apple-system, "Segoe UI", sans-serif;
  --font-mono: "JetBrains Mono", "Fira Code", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  --fs-caption: 12px;
  --fs-small: 14px;
  --fs-body: 16px;
  --fs-subheader: 20px;
  --fs-h3: 24px;
  --lh-heading: 1.3;
  --lh-body: 1.5;
  --spacing-1: 4px; --spacing-2: 8px; --spacing-3: 12px;
  --spacing-4: 16px; --spacing-5: 20px; --spacing-6: 24px;
  --motion-duration-fast: 140ms;
  --motion-duration-normal: 220ms;
  --motion-easing-standard: cubic-bezier(0.2, 0, 0, 1);
  --motion-easing-snappy: cubic-bezier(0.16, 1, 0.3, 1);
  --color-bronze-text: #E5B25C;
  --color-gold-text: #F4C430;
  --color-scrim: rgba(4, 6, 9, 0.62);
  --sigil-ink: #0E1116;
  --sigil-rim: rgba(229, 178, 92, 0.28);
  --clip-chamfer-xs: polygon(4px 0, calc(100% - 4px) 0, 100% 4px, 100% calc(100% - 4px), calc(100% - 4px) 100%, 4px 100%, 0 calc(100% - 4px), 0 4px);
  --color-focus: #F4C430;
  --color-control-border: #C88A36;
  --focus-ring: 0 0 0 2px var(--color-ink), 0 0 0 4px var(--color-focus);
  --lh-relaxed: 1.7;
  --gradient-sheen: none;
  --backdrop-blur-surface: none;
}
[data-theme="light"] {
  color-scheme: light;
  --color-ink: #2A1E14;
  --color-ink-shadow: rgba(60, 40, 20, 0.28);
  --shadow-ink-sm: 1px 1px 0 var(--color-ink-shadow);
  --shadow-ink-md: 2px 2px 0 var(--color-ink-shadow);
  --shadow-ink-lg: 2px 3px 0 var(--color-ink-shadow);
  --bevel-stone: inset 0 1px 0 rgba(255, 255, 255, 0.35), inset 0 -1px 0 rgba(60, 40, 20, 0.12);
  --color-scrim: rgba(42, 30, 20, 0.38);
  --sigil-ink: #2A1E14;
  --sigil-rim: transparent;
  --color-focus: #7A2E0B;
  --color-control-border: var(--color-ink);
  --focus-ring: 0 0 0 2px var(--color-ink), 0 0 0 4px var(--color-focus);
  --color-bg-body: #EDE7D5;
  --color-surface-sunken: #EDE7D5;
  --color-surface-base: #E4DEC9;
  --color-surface-raised: #D8D0B8;
  --color-surface-elevated: #C9C0AA;
  --color-border: var(--color-ink);
  --color-border-translucent: rgba(26, 20, 16, 0.21);
  --color-bg-inactive: #DEDEDE;
  --color-text-primary: #1A1410;
  --color-text-secondary: #33281F;
  --color-text-soft: #4E3F31;
  --color-text-disabled: #A4A4A4;
  --color-accent-fg: #7A2E0B;
  --color-on-primary: #FFFFFF;
  --color-interactive-primary: #B4531A;
  --color-interactive-primary-hover: #9E3F12;
  --color-interactive-primary-press: #7A2E0B;
  --color-interactive-secondary: #D8D0B8;
  --color-interactive-secondary-hover: #C9C0AA;
  --color-interactive-secondary-press: #EDE7D5;
  --color-border-strong: #8E5E1E;
  --color-selection-bg: rgba(180, 83, 26, 0.22);
  --color-row-hover-bg: rgba(142, 94, 30, 0.08);
  --color-row-selected-bg: rgba(180, 83, 26, 0.12);
  --color-copper: #B4531A;
  --color-bronze: #8E5E1E;
  --color-gold: #9A7400;
  --font-display: "Cinzel", "Trajan Pro", "Cormorant SC", "Palatino Linotype", Georgia, serif;
  --color-sev-info: #2F3A4A;
  --color-sev-success: #17533A;
  --color-sev-warning: #9A7400;
  --color-sev-error: #861D10;
  --color-sev-info-fg: #2F3A4A;
  --color-sev-success-fg: #17533A;
  --color-sev-warning-fg: #574100;
  --color-sev-error-fg: #861D10;
  --color-sev-info-bg: #CECCBD;
  --color-sev-success-bg: #C9D5BC;
  --color-sev-warning-bg: #D8CDA9;
  --color-sev-error-bg: #E2C6B2;
  --shadow-default: 2px 2px 0 var(--color-ink-shadow);
  --shadow-hover: 3px 4px 0 var(--color-ink-shadow);
  --shadow-inset: inset 0 1px 0 rgba(255, 255, 255, 0.55), inset 0 -2px 0 rgba(26, 20, 16, 0.18);
  --rounded-sm: 2px;
  --rounded-md: 3px;
  --color-bronze-text: #6B4512;
  --color-gold-text: #574100;
  --lh-relaxed: 1.7;
  --gradient-sheen: none;
  --backdrop-blur-surface: none;
}
*, *::before, *::after { box-sizing: border-box; }
html, body {
  margin: 0;
  padding: 0;
  background: var(--color-bg-body);
  color: var(--color-text-primary);
  font: 400 var(--fs-small)/var(--lh-body) var(--font-sans);
}
/* Vista en grande: el body del canvas llena el alto, sin franja del fondo base. */
html[data-dx-expanded] body { min-height: 100vh; }
`;

// Receptor ligero de postMessage + ResizeObserver (§4.9)
const CANVAS_RECEIVER_SCRIPT = `
addEventListener("message", e => {
  if (e.source !== parent || !e.data || typeof e.data.dxTheme !== "string") return;
  document.documentElement.dataset.theme = e.data.dxTheme;
  if (typeof e.data.dxExpanded === "boolean") {
    document.documentElement.toggleAttribute("data-dx-expanded", e.data.dxExpanded);
  }
  const v = e.data.vars;
  if (v) {
    let live = document.getElementById("dx-live-tokens");
    if (!live) {
      live = document.createElement("style");
      live.id = "dx-live-tokens";
      document.head.appendChild(live);
    }
    const rules = [];
    for (const k in v) if (k.startsWith("--")) rules.push(k + ":" + v[k]);
    live.textContent = ":root,[data-theme]{" + rules.join(";") + "}";
  }
  dispatchEvent(new CustomEvent("dx-canvas-theme", { detail: { theme: e.data.dxTheme } }));
});
addEventListener("DOMContentLoaded", () => {
  let lastSent = 0;
  const sendHeight = () => {
    const h = Math.ceil(document.body.getBoundingClientRect().height || 320);
    if (Math.abs(h - lastSent) > 1) {
      lastSent = h;
      parent.postMessage({ dxCanvasHeight: h }, "*");
    }
  };
  new ResizeObserver(sendHeight).observe(document.body);
  sendHeight();
  parent.postMessage({ dxCanvasReady: true }, "*");
});
`;

const canvasSheet = createSheet(`
  :host {
    padding: 0;
    overflow: hidden;
    /* El marco interno asume borde y radio para evitar doble línea */
    border-color: transparent;
    background: transparent;
    box-shadow: none;
  }

  :host(:hover) {
    border-color: transparent;
  }

  [part="frame"] {
    margin: 0;
    display: flex;
    flex-direction: column;
    border: var(--border-width) solid var(--color-border);
    border-radius: var(--rounded-md);
    overflow: hidden;
    background: var(--color-surface-base);
    box-shadow: var(--shadow-inset), var(--shadow-default);
    transition: border-color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  :host(:hover) [part="frame"] {
    border-color: var(--color-border-strong);
  }

  [part="badge"] {
    display: flex;
    align-items: center;
    gap: var(--spacing-2);
    padding: var(--spacing-2) var(--spacing-4);
    background: var(--color-surface-raised);
    border-bottom: var(--border-width) solid var(--color-border-translucent);
    color: var(--color-text-soft);
    font-size: var(--fs-micro);
    font-weight: var(--fw-medium);
    text-transform: uppercase;
    letter-spacing: var(--ls-caps);
    user-select: none;
  }

  [part="badge"] svg {
    flex: none;
    color: var(--color-text-soft);
  }

  [part="badge"] {
    text-transform: none;
    letter-spacing: 0.01em;
  }

  [part="expand"] {
    margin-inline-start: auto;
    display: inline-grid;
    place-items: center;
    width: 26px;
    height: 26px;
    padding: 0;
    border: var(--border-width) solid var(--color-border-translucent);
    border-radius: var(--rounded-sm);
    background: transparent;
    color: var(--color-text-soft);
    cursor: pointer;
  }

  [part="expand"]:hover {
    color: var(--color-text-primary);
    border-color: var(--color-border-strong);
  }

  [part="expand"]:focus-visible {
    outline: none;
    box-shadow: var(--focus-ring);
  }

  /* Vista en grande: el canvas ocupa casi toda la pantalla (capa superior nativa). */
  :host(:popover-open) {
    position: fixed;
    inset: var(--spacing-5);
    width: auto;
    height: auto;
    max-width: none;
    max-height: none;
    margin: 0;
    padding: 0;
    border: 0;
    overflow: hidden;
    background: transparent;
    filter: drop-shadow(0 24px 48px rgba(0, 0, 0, 0.45));
  }

  :host(:popover-open)::backdrop {
    background: rgba(4, 6, 9, 0.72);
  }

  :host(:popover-open) [part="frame"] {
    height: 100%;
  }

  :host(:popover-open) [part="iframe"] {
    flex: 1 1 auto;
    height: auto !important;
    min-height: 0;
  }

  [part="iframe"] {
    display: block;
    width: 100%;
    border: 0;
    background: var(--color-bg-body);
  }

  /* Estado de carga: shimmer hasta el evento load del iframe */
  [part="frame"][data-loading] [part="iframe"] {
    background: linear-gradient(
      90deg,
      var(--color-surface-base) 0%,
      var(--color-surface-hover) 50%,
      var(--color-surface-base) 100%
    );
    background-size: 200% 100%;
    animation: fc-shimmer var(--motion-duration-deliberate) var(--motion-easing-standard) infinite;
  }

  @keyframes fc-shimmer {
    from { background-position: 200% 0; }
    to { background-position: -200% 0; }
  }

  @media (prefers-reduced-motion: reduce) {
    [part="frame"][data-loading] [part="iframe"] {
      animation: none;
      background: var(--color-surface-base);
    }
  }

  .unavailable {
    padding: var(--spacing-5);
    color: var(--color-sev-warning-fg);
    font-size: var(--fs-small);
    background: var(--color-bg-body);
  }
`);

const VOID_HTML_TAGS = new Set([
  'area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input',
  'link', 'meta', 'param', 'source', 'track', 'wbr'
]);

function escapeHtmlText(str) {
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function escapeHtmlAttr(str) {
  return escapeHtmlText(str).replace(/"/g, '&quot;');
}

// Serializador HTML5 determinista sobre nodos inertes de DOMParser (§4.9)
function serializeHtmlNode(node) {
  if (node.nodeType === Node.TEXT_NODE) {
    const parentTag = node.parentNode?.nodeName?.toLowerCase();
    if (parentTag === 'script' || parentTag === 'style') {
      return node.nodeValue || '';
    }
    return escapeHtmlText(node.nodeValue || '');
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return '';
  }
  const tag = node.tagName.toLowerCase();
  let out = `<${tag}`;
  for (const attr of Array.from(node.attributes)) {
    out += ` ${attr.name}="${escapeHtmlAttr(attr.value)}"`;
  }
  out += '>';
  if (VOID_HTML_TAGS.has(tag)) {
    return out;
  }
  for (const child of Array.from(node.childNodes)) {
    out += serializeHtmlNode(child);
  }
  out += `</${tag}>`;
  return out;
}

function serializeChildNodes(container) {
  if (!container) return '';
  let out = '';
  for (const child of Array.from(container.childNodes)) {
    out += serializeHtmlNode(child);
  }
  return out;
}

function computeCanvasTokens() {
  const cs = getComputedStyle(document.documentElement);
  const vars = {};
  for (const k of CANVAS_TOKENS) {
    const val = cs.getPropertyValue(k).trim();
    if (val) vars[k] = val;
  }
  return vars;
}

function buildPreviewSrcdoc(rawHtml, activeTheme) {
  const parser = new DOMParser();
  const parsed = parser.parseFromString(rawHtml, 'text/html');
  const vars = computeCanvasTokens();
  const varRules = Object.entries(vars)
    .map(([k, v]) => `${k}:${v}`)
    .join(';');

  const safeTheme = activeTheme === 'light' ? 'light' : 'dark';
  const headContent = serializeChildNodes(parsed.head);
  const bodyContent = serializeChildNodes(parsed.body);

  return [
    '<!doctype html>',
    `<html lang="es" data-theme="${safeTheme}">`,
    '<head>',
    '<meta charset="utf-8">',
    `<style>${DUAL_THEME_BASE_CSS}</style>`,
    `<style id="dx-live-tokens">:root,[data-theme]{${varRules}}</style>`,
    `<script>${CANVAS_RECEIVER_SCRIPT.replace(/<\/script/gi, '<\\/script')}</script>`,
    headContent,
    '</head>',
    `<body>${bodyContent}</body>`,
    '</html>'
  ].join('');
}

function withThemeParam(rawUrl, theme) {
  const safeTheme = theme === 'light' ? 'light' : 'dark';
  const sep = rawUrl.includes('?') ? '&' : '?';
  return `${rawUrl}${sep}theme=${safeTheme}`;
}

export class DxFreeCanvas extends HTMLElement {
  constructor() {
    super();
    this._data = null;
    this._iframe = null;
    this._mode = 'none';
    this._ready = false;
    this._queuedTheme = null;
    this._lastHeightTs = 0;
    this._heightTimer = null;
    this._heightSheet = createSheet('[part="iframe"] { height: 320px; }');

    this._onWindowMessage = this._handleWindowMessage.bind(this);
    this._onThemeChange = this._handleThemeChange.bind(this);

    this.attachShadow({ mode: 'open' });
    adoptSheets(this.shadowRoot, baseBlockSheet, canvasSheet, this._heightSheet);
  }

  connectedCallback() {
    if (!this._toggleBound) {
      this._toggleBound = true;
      this.addEventListener('toggle', () => this._onToggle());
    }
    window.addEventListener('message', this._onWindowMessage);
    window.addEventListener('dx-theme', this._onThemeChange);
  }

  _toggleExpanded() {
    if (!this.hasAttribute('popover')) this.setAttribute('popover', 'auto');
    try {
      if (this.matches(':popover-open')) this.hidePopover();
      else this.showPopover();
    } catch {
      // Sin soporte de popover: el canvas sigue en su tamaño normal.
    }
  }

  _onToggle() {
    const on = this.matches(':popover-open');
    if (!on) this.removeAttribute('popover');
    // Avisa al canvas para que su body ocupe todo el alto mientras está en grande.
    if (this._iframe?.contentWindow && this._ready) {
      const theme = document.documentElement.dataset.theme === 'light' ? 'light' : 'dark';
      this._iframe.contentWindow.postMessage({ dxTheme: theme, dxExpanded: on }, '*');
    }
    if (this._expandBtn) {
      this._expandBtn.replaceChildren(createUiIcon(on ? 'minimize' : 'maximize', 14));
      this._expandBtn.setAttribute('aria-label', on ? 'Cerrar vista en grande' : 'Ver en grande');
      this._expandBtn.title = on ? 'Cerrar (Esc)' : 'Ver en grande · Esc para cerrar';
    }
  }

  disconnectedCallback() {
    window.removeEventListener('message', this._onWindowMessage);
    window.removeEventListener('dx-theme', this._onThemeChange);
    if (this._heightTimer) clearTimeout(this._heightTimer);
  }

  get data() {
    return this._data;
  }

  set data(val) {
    this._data = val;
    this.render();
  }

  _setHeight(px) {
    const clamped = Math.min(1200, Math.max(120, Math.round(px)));
    this._heightSheet.replaceSync(`[part="iframe"] { height: ${clamped}px; }`);
    if (this._iframe) {
      this._iframe.setAttribute('height', String(clamped));
    }
  }

  _handleWindowMessage(e) {
    if (!this._iframe || e.source !== this._iframe.contentWindow) return;
    if (e.origin !== 'null') return;
    const payload = e.data;
    if (!payload || typeof payload !== 'object') return;

    if (payload.dxCanvasReady === true) {
      this._ready = true;
      this._iframe?.parentNode?.removeAttribute('data-loading');
      if (this._queuedTheme) {
        const pending = this._queuedTheme;
        this._queuedTheme = null;
        this._postThemeToIframe(pending);
      }
      return;
    }

    if (typeof payload.dxCanvasHeight === 'number' && Number.isFinite(payload.dxCanvasHeight)) {
      const now = performance.now();
      const apply = () => {
        this._lastHeightTs = performance.now();
        this._setHeight(payload.dxCanvasHeight);
      };
      if (now - this._lastHeightTs >= 100) {
        apply();
      } else {
        if (this._heightTimer) clearTimeout(this._heightTimer);
        this._heightTimer = setTimeout(apply, 100);
      }
    }
  }

  _handleThemeChange(e) {
    const nextTheme = e.detail?.theme === 'light' ? 'light' : 'dark';
    if (!this._iframe) return;
    if (!this._ready) {
      this._queuedTheme = nextTheme;
      return;
    }
    this._postThemeToIframe(nextTheme);
  }

  _postThemeToIframe(theme) {
    if (!this._iframe?.contentWindow) return;
    const msg = { dxTheme: theme };
    if (this._mode === 'preview') {
      msg.vars = computeCanvasTokens();
    }
    this._iframe.contentWindow.postMessage(msg, '*');
  }

  render() {
    clearNode(this.shadowRoot);
    this._iframe = null;
    this._ready = false;
    this._queuedTheme = null;
    this._mode = 'none';

    if (!this._data) return;

    const initialH = typeof this._data.h === 'number' && Number.isFinite(this._data.h)
      ? Math.min(1200, Math.max(120, Math.round(this._data.h)))
      : 320;
    this._setHeight(initialH);

    const activeTheme = document.documentElement.dataset.theme === 'light' ? 'light' : 'dark';
    const hasSrc = typeof this._data.src === 'string' && this._data.src.trim().length > 0;
    const hasHtml = typeof this._data.html === 'string';
    const isPreviewDoc = document.documentElement.dataset.dxMode === 'preview';

    const figure = el('figure', { part: 'frame' });

    if (hasSrc) {
      // Modo (P) Producción — Ruta propia en Go
      this._mode = 'production';
      figure.appendChild(
        el('figcaption', { part: 'badge' }, createUiIcon('shield', 14), 'Contenido aislado')
      );
      const iframe = el('iframe', {
        part: 'iframe',
        sandbox: 'allow-scripts',
        referrerpolicy: 'no-referrer',
        loading: 'lazy',
        title: 'Simulación del agente',
        height: String(initialH),
        src: withThemeParam(this._data.src.trim(), activeTheme)
      });
      this._iframe = iframe;
      figure.appendChild(iframe);
    } else if (hasHtml && isPreviewDoc) {
      // Modo (V) Vista previa local — <html data-dx-mode="preview">
      this._mode = 'preview';
      figure.appendChild(
        el('figcaption', { part: 'badge' }, createUiIcon('shield', 14), 'Contenido aislado · vista previa')
      );
      const iframe = el('iframe', {
        part: 'iframe',
        sandbox: 'allow-scripts',
        referrerpolicy: 'no-referrer',
        loading: 'lazy',
        title: 'Simulación del agente',
        height: String(initialH),
        srcdoc: buildPreviewSrcdoc(this._data.html, activeTheme)
      });
      this._iframe = iframe;
      figure.appendChild(iframe);
    } else {
      // Bloqueo seguro si llega html crudo fuera de modo preview (§4.9)
      figure.appendChild(
        el('figcaption', { part: 'badge' }, createUiIcon('shield', 14), 'Contenido aislado')
      );
      figure.appendChild(el('div', { className: 'unavailable' }, 'Canvas no disponible'));
    }

    if (this._iframe && typeof this.showPopover === 'function') {
      const badge = figure.querySelector('[part="badge"]');
      const btn = el('button', { type: 'button', part: 'expand', 'aria-label': 'Ver en grande', title: 'Ver en grande · Esc para cerrar' }, createUiIcon('maximize', 14));
      btn.addEventListener('click', () => this._toggleExpanded());
      this._expandBtn = btn;
      if (badge) badge.appendChild(btn);
    }

    if (this._iframe) {
      // Indicador visual de carga (solo presentación; no altera el handshake dxCanvasReady)
      figure.setAttribute('data-loading', '');
      const clearLoading = () => figure.removeAttribute('data-loading');
      this._iframe.addEventListener('load', clearLoading, { once: true });
      this._iframe.addEventListener('error', clearLoading, { once: true });
      // Red de seguridad: nunca dejar el shimmer indefinido
      setTimeout(clearLoading, 4000);
    }

    this.shadowRoot.appendChild(figure);
  }
}

if (!customElements.get('dx-free-canvas')) {
  customElements.define('dx-free-canvas', DxFreeCanvas);
}
