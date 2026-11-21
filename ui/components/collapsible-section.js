// ==========================================================================
// DioptraEngine Viewer — ui/components/collapsible-section.js
// <dx-collapsible-section> (§4.4 Contrato A — Light DOM slot recursivo)
// ==========================================================================

import { adoptSheets, baseBlockSheet, clearNode, createSheet, createUiIcon, el, SEV_SET } from './shared.js';

const sectionSheet = createSheet(`
  :host {
    padding: 0;
    filter: none;
  }

  /* El summary ya es la losa: sin marco base duplicado */
  :host::before {
    display: none;
  }

  [part="details"] {
    margin: 0;
  }

  /* Apertura fluida: opacidad siempre; altura intrínseca solo con interpolate-size */
  [part="details"]::details-content {
    opacity: 0;
    transition: opacity var(--motion-duration-normal) var(--motion-easing-decelerate),
                content-visibility var(--motion-duration-normal) allow-discrete;
  }

  [part="details"][open]::details-content {
    opacity: 1;
  }

  @supports (interpolate-size: allow-keywords) {
    :host {
      interpolate-size: allow-keywords;
    }

    [part="details"]::details-content {
      block-size: 0;
      overflow: clip;
      transition: block-size var(--motion-duration-normal) var(--motion-easing-snappy),
                  opacity var(--motion-duration-normal) var(--motion-easing-decelerate),
                  content-visibility var(--motion-duration-normal) allow-discrete;
    }

    [part="details"][open]::details-content {
      block-size: auto;
    }
  }

  [part="summary"] {
    list-style: none;
    display: flex;
    align-items: center;
    gap: var(--spacing-3);
    min-height: var(--spacing-9);
    padding: var(--spacing-3) var(--spacing-6);
    cursor: pointer;
    user-select: none;
    outline: none;
    border: 2px solid var(--color-ink);
    background-color: var(--color-stone-slab);
    box-shadow: var(--bevel-stone);
    border-radius: 2px;
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard),
                box-shadow var(--motion-duration-fast) var(--motion-easing-standard);
  }

  [part="summary"]::-webkit-details-marker {
    display: none;
  }

  [part="summary"]::marker {
    content: "";
  }

  [part="summary"]:hover {
    background-color: var(--color-surface-hover);
  }

  [part="summary"]:focus-visible {
    box-shadow: var(--bevel-stone), var(--focus-ring, inset 0 0 0 2px var(--color-accent-fg));
  }

  [part="chevron"] {
    flex-shrink: 0;
    color: var(--color-bronze-text, var(--color-accent-fg));
    transition: transform 160ms var(--motion-easing-emphasized),
                color 160ms var(--motion-easing-standard);
  }

  [part="summary"]:hover [part="chevron"] {
    color: var(--color-bronze-text, var(--color-accent-fg));
  }

  [part="details"][open] > [part="summary"] [part="chevron"] {
    transform: rotate(90deg);
    color: var(--color-bronze-text, var(--color-accent-fg));
  }

  .sev-dot {
    width: 8px;
    height: 8px;
    border-radius: var(--rounded-full);
    flex-shrink: 0;
  }
  .sev-dot[data-sev="info"] { background: var(--color-sev-info); }
  .sev-dot[data-sev="success"] { background: var(--color-sev-success); }
  .sev-dot[data-sev="warning"] { background: var(--color-sev-warning); }
  .sev-dot[data-sev="error"] { background: var(--color-sev-error); }

  .summary-title {
    flex: 1 1 auto;
    font-family: var(--font-display, Georgia, serif);
    font-size: var(--fs-subheader);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-heading);
    letter-spacing: .02em;
    color: var(--color-text-primary);
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  [part="count"] {
    flex-shrink: 0;
    min-width: 22px;
    padding: 2px var(--spacing-2);
    border-radius: var(--rounded-sm);
    background: var(--color-surface-sunken);
    border: var(--border-width) solid var(--color-border-translucent);
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    font-weight: var(--fw-medium);
    line-height: 1.4;
    text-align: center;
    letter-spacing: var(--ls-caps);
    font-variant-numeric: tabular-nums;
    font-variant-ligatures: none;
    color: var(--color-text-soft);
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard),
                border-color var(--motion-duration-fast) var(--motion-easing-standard),
                color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  [part="details"][open] [part="count"] {
    background: var(--color-accent-soft);
    border-color: var(--color-accent-ring);
    color: var(--color-text-primary);
  }

  /* Contenido sin segundo marco ni fondo: solo padding + filete guía de 1px */
  [part="content"] {
    margin-inline-start: calc(var(--spacing-6) + 7px);
    padding: var(--spacing-4) var(--spacing-6) var(--spacing-6) var(--spacing-4);
    border-inline-start: 1px solid var(--color-border-strong);
    background: none;
    display: flex;
    flex-direction: column;
    gap: var(--spacing-4);
  }

  @media (prefers-reduced-motion: reduce) {
    [part="details"]::details-content,
    [part="summary"],
    [part="chevron"],
    [part="count"] {
      transition: none;
    }
  }

  @media (forced-colors: active) {
    [part="summary"] {
      border-color: CanvasText;
      background-color: Canvas;
      box-shadow: none;
    }
    [part="summary"]:focus-visible {
      outline: 2px solid Highlight;
      outline-offset: 2px;
    }
    [part="chevron"] { color: CanvasText; }
    [part="content"] { border-inline-start-color: CanvasText; }
  }
`);

export class DxCollapsibleSection extends HTMLElement {
  static get observedAttributes() {
    return ['open', 'sev'];
  }

  constructor() {
    super();
    this._data = null;
    this._detailsEl = null;
    this._syncingOpen = false;
    this.attachShadow({ mode: 'open' });
    adoptSheets(this.shadowRoot, baseBlockSheet, sectionSheet);
  }

  get data() {
    return this._data;
  }

  set data(val) {
    this._data = val;
    const isOpen = Boolean(val?.open);
    this._syncingOpen = true;
    if (isOpen) {
      this.setAttribute('open', '');
    } else {
      this.removeAttribute('open');
    }
    if (SEV_SET.has(val?.sev)) {
      this.setAttribute('sev', val.sev);
    } else {
      this.removeAttribute('sev');
    }
    this._syncingOpen = false;
    this.render();
  }

  attributeChangedCallback(name, oldVal, newVal) {
    if (oldVal === newVal || this._syncingOpen) return;
    if (name === 'open' && this._detailsEl) {
      this._detailsEl.open = this.hasAttribute('open');
    } else if (name === 'sev' && this._data) {
      this.render();
    }
  }

  render() {
    clearNode(this.shadowRoot);
    const title = typeof this._data?.title === 'string' && this._data.title ? this._data.title : '(sin título)';
    const sev = SEV_SET.has(this.getAttribute('sev')) ? this.getAttribute('sev') : undefined;
    const isOpen = this.hasAttribute('open');
    const blockCount = Array.isArray(this._data?.blocks) ? this._data.blocks.length : this.children.length;

    const chevron = createUiIcon('chevron', 16);
    chevron.setAttribute('part', 'chevron');

    const summaryChildren = [chevron];
    if (sev) {
      summaryChildren.push(el('span', { className: 'sev-dot', 'data-sev': sev, 'aria-hidden': 'true' }));
    }
    summaryChildren.push(el('span', { className: 'summary-title', title }, title));
    summaryChildren.push(el('span', { part: 'count', title: `${blockCount} bloques` }, String(blockCount)));

    const summary = el('summary', { part: 'summary' }, ...summaryChildren);
    const content = el('div', { part: 'content' }, el('slot'));
    const details = el('details', { part: 'details', open: isOpen }, summary, content);

    details.addEventListener('toggle', () => {
      this._syncingOpen = true;
      if (details.open) {
        this.setAttribute('open', '');
      } else {
        this.removeAttribute('open');
      }
      this._syncingOpen = false;
      this.dispatchEvent(new CustomEvent('dx-toggle', {
        bubbles: true,
        composed: true,
        detail: { open: details.open }
      }));
    });

    this._detailsEl = details;
    this.shadowRoot.appendChild(details);
  }
}

if (!customElements.get('dx-collapsible-section')) {
  customElements.define('dx-collapsible-section', DxCollapsibleSection);
}
