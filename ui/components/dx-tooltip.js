// ==========================================================================
// DioptraEngine Viewer — ui/components/dx-tooltip.js
// R5 (RONDA2-SPEC): gestor singleton de tooltips Reliquia.
// Montaje (desde dx-app): document.body.append(document.createElement('dx-tooltip'))
// - popover="manual" (top layer) con fallback position:fixed + z-index alto.
// - Detecta disparadores [data-tip] / [title] vía composedPath (atraviesa Shadow DOM).
// - Sin style= en markup: estilos con CSSStyleSheet adoptada; posición vía CSSOM.
// ==========================================================================

const TAG = 'dx-tooltip';
const SHOW_DELAY_MS = 450;
const GAP = 8;          // separación disparador ↔ tooltip
const MARGIN = 8;       // margen mínimo contra el viewport
const ARROW_INSET = 10; // la flecha nunca toca las esquinas achaflanadas

const CSS = `
dx-tooltip {
  --tip-bg: var(--color-surface-elevated, #1E2C3D);
  --tip-fg: var(--color-text-primary, #F1E9D8);
  --tip-ink: var(--color-bronze, #C88A36);
  position: fixed;
  inset: auto;
  left: var(--tip-x, 0px);
  top: var(--tip-y, 0px);
  z-index: 2147483000;
  box-sizing: border-box;
  width: max-content;
  max-width: min(32ch, calc(100vw - 16px));
  height: auto;
  margin: 0;
  border: 0;
  padding: 0;
  overflow: visible;
  background: transparent;
  color: var(--tip-fg);
  font: var(--fs-xs, 12px)/1.35 var(--font-sans, system-ui, sans-serif);
  pointer-events: none;
  opacity: 0;
  transform: translateY(var(--motion-distance-sm, 4px));
  transition:
    opacity var(--motion-duration-fast, 140ms) var(--motion-easing-standard, cubic-bezier(.2, 0, 0, 1)),
    transform var(--motion-duration-fast, 140ms) var(--motion-easing-emphasized, cubic-bezier(.2, 0, 0, 1));
  filter: drop-shadow(2px 2px 0 var(--color-ink-shadow, rgba(0, 0, 0, .55)));
}
dx-tooltip[data-side="bottom"] { transform: translateY(calc(-1 * var(--motion-distance-sm, 4px))); }
dx-tooltip[data-open] { opacity: 1; transform: none; }
dx-tooltip:not([popover]):not([data-shown]) { display: none; }
dx-tooltip::backdrop { display: none; }
[data-theme="light"] dx-tooltip {
  --tip-ink: var(--color-ink, #2A1E14);
  --tip-fg: var(--color-text-primary, #1A1410);
}
dx-tooltip .dx-tooltip__body {
  position: relative;
  z-index: 1;
  display: block;
  padding: var(--spacing-1, 4px) var(--spacing-3, 12px);
  background: var(--tip-bg);
  color: var(--tip-fg);
  box-shadow: inset 0 0 0 1.5px var(--tip-ink), var(--shadow-ink-sm, none);
  clip-path: var(--clip-chamfer-xs, polygon(4px 0, calc(100% - 4px) 0, 100% 4px, 100% calc(100% - 4px), calc(100% - 4px) 100%, 4px 100%, 0 calc(100% - 4px), 0 4px));
  overflow-wrap: anywhere;
  white-space: normal;
  text-align: left;
}
dx-tooltip .dx-tooltip__arrow {
  position: absolute;
  z-index: 0;
  left: var(--tip-arrow-x, 50%);
  width: 8px;
  height: 8px;
  margin-left: -4px;
  background: var(--tip-bg);
  box-shadow: inset 0 0 0 1.5px var(--tip-ink);
  transform: rotate(45deg);
}
dx-tooltip[data-side="top"] .dx-tooltip__arrow { bottom: -4px; }
dx-tooltip[data-side="bottom"] .dx-tooltip__arrow { top: -4px; }
@media (prefers-reduced-motion: reduce) {
  dx-tooltip, dx-tooltip[data-side="bottom"] { transition: none; transform: none; }
}
@media (forced-colors: active) {
  dx-tooltip { filter: none; }
  dx-tooltip .dx-tooltip__body {
    border: 1px solid CanvasText;
    background: Canvas;
    color: CanvasText;
    clip-path: none;
    box-shadow: none;
  }
  dx-tooltip .dx-tooltip__arrow { display: none; }
}
`;

let sheet = null;
let sheetAdopted = false;
let activeInstance = null;
let idCounter = 0;

function adoptSheetOnce() {
  if (sheetAdopted) return;
  try {
    if (typeof CSSStyleSheet === 'undefined' || !('adoptedStyleSheets' in document)) return;
    if (!sheet) {
      sheet = new CSSStyleSheet();
      sheet.replaceSync(CSS);
    }
    document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
    sheetAdopted = true;
  } catch (_) {
    /* navegador sin constructable stylesheets: el fallback CSSOM cubre posición */
  }
}

function releaseSheet() {
  if (!sheetAdopted || !sheet) return;
  try {
    document.adoptedStyleSheets = document.adoptedStyleSheets.filter((s) => s !== sheet);
  } catch (_) { /* noop */ }
  sheetAdopted = false;
}

/** ¿`node` está dentro de `ancestor`, atravesando fronteras de Shadow DOM? */
function composedContains(ancestor, node) {
  let n = node;
  while (n) {
    if (n === ancestor) return true;
    n = n.parentNode || n.host || null;
  }
  return false;
}

function hasAccessibleName(el) {
  if (el.hasAttribute('aria-label') && el.getAttribute('aria-label').trim()) return true;
  if (el.hasAttribute('aria-labelledby') && el.getAttribute('aria-labelledby').trim()) return true;
  return Boolean((el.textContent || '').trim());
}

function clamp(v, min, max) {
  return max < min ? min : Math.min(Math.max(v, min), max);
}

export class DxTooltip extends HTMLElement {
  constructor() {
    super();
    this._owner = false;
    this._trigger = null;
    this._timer = 0;
    this._raf = 0;
    this._describedAdded = false;
    this._descriptionAdded = false;
    this._usesPopover = false;
    this._body = null;
    this._arrow = null;
    this._onPointerOver = this._onPointerOver.bind(this);
    this._onPointerOut = this._onPointerOut.bind(this);
    this._onFocusIn = this._onFocusIn.bind(this);
    this._onFocusOut = this._onFocusOut.bind(this);
    this._onKeyDown = this._onKeyDown.bind(this);
    this._onDismiss = this._onDismiss.bind(this);
  }

  connectedCallback() {
    if (activeInstance && activeInstance !== this && activeInstance.isConnected) return; // singleton
    activeInstance = this;
    this._owner = true;

    adoptSheetOnce();

    this._usesPopover = typeof HTMLElement !== 'undefined' && 'showPopover' in HTMLElement.prototype;
    if (this._usesPopover) {
      this.setAttribute('popover', 'manual');
    } else {
      this.style.setProperty('position', 'fixed');
      this.style.setProperty('z-index', '2147483000');
    }
    this.setAttribute('role', 'tooltip');
    if (!this.id) {
      idCounter += 1;
      this.id = `dx-tooltip-${idCounter}-${Math.random().toString(36).slice(2, 8)}`;
    }

    if (!this._body) {
      this.textContent = '';
      this._body = document.createElement('span');
      this._body.className = 'dx-tooltip__body';
      this._arrow = document.createElement('span');
      this._arrow.className = 'dx-tooltip__arrow';
      this._arrow.setAttribute('aria-hidden', 'true');
      this.append(this._arrow, this._body);
    }

    const opts = { capture: true, passive: true };
    document.addEventListener('pointerover', this._onPointerOver, opts);
    document.addEventListener('pointerout', this._onPointerOut, opts);
    document.addEventListener('focusin', this._onFocusIn, opts);
    document.addEventListener('focusout', this._onFocusOut, opts);
    document.addEventListener('keydown', this._onKeyDown, opts);
    document.addEventListener('pointerdown', this._onDismiss, opts);
    document.addEventListener('scroll', this._onDismiss, opts);
    window.addEventListener('resize', this._onDismiss, { passive: true });
  }

  disconnectedCallback() {
    if (!this._owner) return;
    const opts = { capture: true, passive: true };
    document.removeEventListener('pointerover', this._onPointerOver, opts);
    document.removeEventListener('pointerout', this._onPointerOut, opts);
    document.removeEventListener('focusin', this._onFocusIn, opts);
    document.removeEventListener('focusout', this._onFocusOut, opts);
    document.removeEventListener('keydown', this._onKeyDown, opts);
    document.removeEventListener('pointerdown', this._onDismiss, opts);
    document.removeEventListener('scroll', this._onDismiss, opts);
    window.removeEventListener('resize', this._onDismiss, { passive: true });
    this.hide();
    this._owner = false;
    if (activeInstance === this) activeInstance = null;
    releaseSheet();
  }

  // ---------------------------------------------------------------- detección
  _findTrigger(e) {
    const path = typeof e.composedPath === 'function' ? e.composedPath() : [e.target];
    for (const node of path) {
      if (!node || node.nodeType !== 1) continue;
      if (node === this) return null;
      if (node.hasAttribute('data-tip') || node.hasAttribute('title')) return node;
    }
    return null;
  }

  /** Normaliza title → data-tip y devuelve el texto (o '' si no hay). */
  _prepare(el) {
    if (el.hasAttribute('title')) {
      const t = (el.getAttribute('title') || '').trim();
      el.removeAttribute('title');
      if (t && !(el.getAttribute('data-tip') || '').trim()) el.setAttribute('data-tip', t);
    }
    const text = (el.getAttribute('data-tip') || '').trim();
    if (text && !hasAccessibleName(el)) el.setAttribute('aria-label', text);
    return text;
  }

  // ---------------------------------------------------------------- eventos
  _onPointerOver(e) {
    const trigger = this._findTrigger(e);
    if (!trigger) return;
    if (trigger === this._trigger) return; // ya visible o pendiente
    const text = this._prepare(trigger);
    this.hide();
    if (!text) return;
    this._trigger = trigger;
    this._timer = setTimeout(() => {
      this._timer = 0;
      if (this._trigger === trigger && trigger.isConnected) this._show(trigger);
    }, SHOW_DELAY_MS);
  }

  _onPointerOut(e) {
    if (!this._trigger) return;
    const related = e.relatedTarget;
    if (related && composedContains(this._trigger, related)) return;
    this.hide();
  }

  _onFocusIn(e) {
    const trigger = this._findTrigger(e);
    if (!trigger) return;
    const focused = (typeof e.composedPath === 'function' && e.composedPath()[0]) || e.target;
    let keyboard = true;
    try { keyboard = focused.matches(':focus-visible'); } catch (_) { keyboard = true; }
    if (!keyboard) return;
    const text = this._prepare(trigger);
    this.hide();
    if (!text) return;
    this._trigger = trigger;
    this._show(trigger);
  }

  _onFocusOut() { if (this._trigger) this.hide(); }

  _onKeyDown(e) {
    if ((e.key === 'Escape' || e.key === 'Esc') && this._trigger) this.hide();
  }

  _onDismiss() { if (this._trigger) this.hide(); }

  // ---------------------------------------------------------------- mostrar / ocultar
  _show(trigger) {
    const text = (trigger.getAttribute('data-tip') || '').trim();
    if (!text) { this.hide(); return; }
    this._body.textContent = text;

    // Visible pero transparente para medir.
    this.removeAttribute('data-open');
    this.setAttribute('data-side', 'top');
    this.style.setProperty('--tip-x', '0px');
    this.style.setProperty('--tip-y', '0px');
    this.setAttribute('data-shown', '');
    if (this._usesPopover) {
      try { if (!this.matches(':popover-open')) this.showPopover(); } catch (_) { /* noop */ }
    }

    this._position(trigger);

    if (trigger.getRootNode && trigger.getRootNode() === document) {
      const current = (trigger.getAttribute('aria-describedby') || '').split(/\s+/).filter(Boolean);
      if (!current.includes(this.id)) {
        current.push(this.id);
        trigger.setAttribute('aria-describedby', current.join(' '));
        this._describedAdded = true;
      }
    } else if (!trigger.hasAttribute('aria-description')) {
      // aria-describedby no cruza fronteras de shadow root: descripción en línea.
      trigger.setAttribute('aria-description', text);
      trigger.setAttribute('data-tip-desc', '');
      this._descriptionAdded = true;
    }

    cancelAnimationFrame(this._raf);
    this._raf = requestAnimationFrame(() => {
      this._raf = 0;
      if (this._trigger === trigger) this.setAttribute('data-open', '');
    });
  }

  _position(trigger) {
    const t = trigger.getBoundingClientRect();
    const r = this.getBoundingClientRect();
    const de = document.documentElement;
    const vw = (de && de.clientWidth) || window.innerWidth || 0;
    const vh = (de && de.clientHeight) || window.innerHeight || 0;
    const w = r.width;
    const h = r.height;
    const cx = t.left + t.width / 2;

    let side = 'top';
    let y = t.top - GAP - h;
    if (y < MARGIN) {
      const below = t.bottom + GAP;
      // Abajo si cabe, o si abajo hay más espacio que arriba.
      if (below + h <= vh - MARGIN || (vh - t.bottom) > t.top) {
        side = 'bottom';
        y = below;
      }
    }
    const x = clamp(cx - w / 2, MARGIN, vw - w - MARGIN);
    const arrowX = clamp(cx - x, ARROW_INSET, w - ARROW_INSET);

    this.setAttribute('data-side', side);
    this.style.setProperty('--tip-x', `${Math.round(x)}px`);
    this.style.setProperty('--tip-y', `${Math.round(y)}px`);
    this.style.setProperty('--tip-arrow-x', `${Math.round(arrowX)}px`);
  }

  hide() {
    if (this._timer) { clearTimeout(this._timer); this._timer = 0; }
    if (this._raf) { cancelAnimationFrame(this._raf); this._raf = 0; }
    const trigger = this._trigger;
    this._trigger = null;
    if (trigger && this._describedAdded) {
      const rest = (trigger.getAttribute('aria-describedby') || '')
        .split(/\s+/).filter((id) => id && id !== this.id);
      if (rest.length) trigger.setAttribute('aria-describedby', rest.join(' '));
      else trigger.removeAttribute('aria-describedby');
    }
    this._describedAdded = false;
    if (trigger && this._descriptionAdded && trigger.hasAttribute('data-tip-desc')) {
      trigger.removeAttribute('aria-description');
      trigger.removeAttribute('data-tip-desc');
    }
    this._descriptionAdded = false;
    this.removeAttribute('data-open');
    this.removeAttribute('data-shown');
    if (this._usesPopover) {
      try { if (this.matches(':popover-open')) this.hidePopover(); } catch (_) { /* noop */ }
    }
  }
}

if (typeof customElements !== 'undefined' && !customElements.get(TAG)) {
  customElements.define(TAG, DxTooltip);
}
