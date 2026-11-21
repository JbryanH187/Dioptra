// ==========================================================================
// DioptraEngine Viewer — ui/components/severity-callout.js
// <dx-severity-callout> (§4.2 Contrato A — Alert v2.1 RES DISC-10)
// ==========================================================================

import {
  adoptSheets,
  baseBlockSheet,
  clearNode,
  createRefButton,
  createSeverityIcon,
  createSheet,
  el,
  markdownSheet,
  renderMarkdownBlocks,
  SEV_LABELS,
  SEV_SET
} from './shared.js';

const calloutSheet = createSheet(`
  :host {
    padding: 0;
    border: 0;
    border-radius: var(--rounded-sm);
    background: transparent;
    box-shadow: none;
    filter: none;
  }

  /* El callout ya es la losa: sin marco base duplicado */
  :host::before {
    display: none;
  }

  [part="callout"] {
    --sev: var(--color-sev-info);
    --sev-bg: var(--color-sev-info-bg);
    --sev-border: var(--color-sev-info-border);
    --sev-fg: var(--color-sev-info-fg);
    --callout-clip: var(--clip-chamfer-md, polygon(10px 0, calc(100% - 10px) 0, 100% 10px, 100% calc(100% - 10px), calc(100% - 10px) 100%, 10px 100%, 0 calc(100% - 10px), 0 10px));
    position: relative;
    isolation: isolate;
    display: flex;
    align-items: flex-start;
    gap: var(--spacing-3);
    padding: var(--spacing-3) var(--spacing-4);
    border: 0;
    border-radius: 0;
    background: transparent;
    box-shadow: none;
    filter: drop-shadow(2px 2px 0 var(--color-ink-shadow, #040609));
  }

  /* Reliquia: fondo teñido + contorno de tinta 2px, recortado en chaflán (sin franja lateral) */
  [part="callout"]::before {
    content: "";
    position: absolute;
    inset: 0;
    z-index: -1;
    background:
      linear-gradient(135deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) top left / 10px 10px no-repeat border-box,
      linear-gradient(225deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) top right / 10px 10px no-repeat border-box,
      linear-gradient(315deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) bottom right / 10px 10px no-repeat border-box,
      linear-gradient(45deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) bottom left / 10px 10px no-repeat border-box,
      var(--sev-bg);
    box-shadow: inset 0 0 0 2px var(--color-ink);
    clip-path: var(--callout-clip);
    pointer-events: none;
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  :host([sev="info"]) [part="callout"] {
    --sev: var(--color-sev-info);
    --sev-bg: var(--color-sev-info-bg);
    --sev-border: var(--color-sev-info-border);
    --sev-fg: var(--color-sev-info-fg);
  }
  :host([sev="success"]) [part="callout"] {
    --sev: var(--color-sev-success);
    --sev-bg: var(--color-sev-success-bg);
    --sev-border: var(--color-sev-success-border);
    --sev-fg: var(--color-sev-success-fg);
  }
  :host([sev="warning"]) [part="callout"] {
    --sev: var(--color-sev-warning);
    --sev-bg: var(--color-sev-warning-bg);
    --sev-border: var(--color-sev-warning-border);
    --sev-fg: var(--color-sev-warning-fg);
  }
  :host([sev="error"]) [part="callout"] {
    --sev: var(--color-sev-error);
    --sev-bg: var(--color-sev-error-bg);
    --sev-border: var(--color-sev-error-border);
    --sev-fg: var(--color-sev-error-fg);
  }

  /* Sello romboidal de severidad: el rombo es un pseudo-elemento rotado tras el icono */
  .icon-disc {
    position: relative;
    isolation: isolate;
    flex-shrink: 0;
    display: inline-grid;
    place-items: center;
    width: 28px;
    height: 28px;
    margin-top: 1px;
    border-radius: 0;
    background: transparent;
    box-shadow: none;
    color: var(--sev-fg);
  }

  .icon-disc::before {
    content: "";
    position: absolute;
    inset: 4px;
    z-index: -1;
    transform: rotate(45deg);
    background: var(--color-surface-base);
    box-shadow: 0 0 0 2px var(--color-ink), inset 0 0 0 2px var(--sev-border);
  }

  [part="icon"] {
    display: block;
    width: 14px;
    height: 14px;
    color: var(--sev-fg);
  }

  /* Etiqueta de severidad (si se expone visualmente): versalitas display en sev-fg */
  .sev-label,
  [part="label"] {
    font-family: var(--font-display, Georgia, serif);
    font-variant: small-caps;
    letter-spacing: 0.08em;
    color: var(--sev-fg);
  }

  [part="content"] {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding-top: 2px;
  }

  [part="title"] {
    margin: 0;
    font-family: var(--font-display, Georgia, serif);
    font-size: var(--fs-small);
    font-weight: var(--fw-semibold);
    letter-spacing: 0.02em;
    line-height: var(--lh-heading);
    color: var(--color-text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  [part="body"] {
    margin-top: 2px;
    font-size: var(--fs-small);
    line-height: var(--lh-body);
    color: var(--color-text-primary);
  }

  [part="body"] p {
    margin: 0 0 var(--spacing-2) 0;
  }

  [part="body"] p:last-child {
    margin-bottom: 0;
  }

  [part="body"] ul {
    margin: var(--spacing-1) 0;
    padding-left: var(--spacing-5);
  }

  [part="body"] code {
    font-family: var(--font-mono);
    font-size: 0.92em;
    font-variant-ligatures: none;
    background: var(--color-surface-sunken);
    border: var(--border-width) solid var(--color-border-translucent);
    padding: 0 5px;
    border-radius: var(--rounded-xs);
    color: var(--color-text-primary);
  }

  [part="body"] a {
    color: var(--color-accent-fg);
    text-decoration: none;
  }

  [part="body"] a:hover {
    text-decoration: underline;
  }

  [part="ref"] {
    align-self: flex-start;
    margin-top: var(--spacing-2);
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  @media (prefers-reduced-motion: reduce) {
    [part="callout"],
    [part="callout"]::before,
    .icon-disc::before {
      transition: none;
    }
  }

  @media (forced-colors: active) {
    [part="callout"] { filter: none; }
    [part="callout"]::before {
      clip-path: none;
      background: Canvas;
      box-shadow: none;
      border: 1px solid CanvasText;
    }
    .icon-disc::before {
      background: Canvas;
      box-shadow: none;
      border: 1px solid CanvasText;
    }
  }
`);

export class DxSeverityCallout extends HTMLElement {
  static get observedAttributes() {
    return ['sev'];
  }

  constructor() {
    super();
    this._data = null;
    this.attachShadow({ mode: 'open' });
    adoptSheets(this.shadowRoot, baseBlockSheet, markdownSheet, calloutSheet);
  }

  get data() {
    return this._data;
  }

  set data(val) {
    this._data = val;
    const sev = SEV_SET.has(val?.sev) ? val.sev : 'info';
    if (this.getAttribute('sev') !== sev) {
      this.setAttribute('sev', sev);
    }
    this.render();
  }

  attributeChangedCallback(name, oldVal, newVal) {
    if (name === 'sev' && oldVal !== newVal && this._data) {
      this.render();
    }
  }

  render() {
    clearNode(this.shadowRoot);
    if (!this._data) return;

    const sev = SEV_SET.has(this.getAttribute('sev')) ? this.getAttribute('sev') : 'info';
    const label = SEV_LABELS[sev];
    const title = String(this._data.title ?? '');
    const icon = createSeverityIcon(sev, 24);
    icon.setAttribute('part', 'icon');
    const iconDisc = el('span', { className: 'icon-disc', 'aria-hidden': 'true' }, icon);

    const bodyDiv = el('div', { part: 'body' });
    bodyDiv.appendChild(renderMarkdownBlocks(String(this._data.body ?? ''), { allowLists: true }));

    const content = el(
      'div',
      { part: 'content' },
      el('h3', { part: 'title', title }, el('span', { className: 'sr-only' }, `${label}: `), title),
      bodyDiv
    );

    if (this._data.ref) {
      const refBtn = createRefButton(this._data.ref, 'ref');
      if (refBtn) content.appendChild(refBtn);
    }

    const aside = el(
      'aside',
      {
        part: 'callout',
        role: 'note',
        'aria-label': `${label}: ${title}`
      },
      iconDisc,
      content
    );

    this.shadowRoot.appendChild(aside);
  }
}

if (!customElements.get('dx-severity-callout')) {
  customElements.define('dx-severity-callout', DxSeverityCallout);
}
