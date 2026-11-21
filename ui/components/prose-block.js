// ==========================================================================
// DioptraEngine Viewer — ui/components/prose-block.js
// <dx-prose-block> (§4.8 Contrato A — Prosa técnica con AST seguro y refs)
// ==========================================================================

import {
  adoptSheets,
  baseBlockSheet,
  clearNode,
  createDegradedBanner,
  createSheet,
  markdownSheet,
  el,
  renderMarkdownBlocks
} from './shared.js';

const proseSheet = createSheet(`
  [part="prose"] {
    font-size: var(--fs-body);
    line-height: 1.65;
    color: var(--color-text-secondary);
    max-width: 72ch;
    hanging-punctuation: first;
  }

  [part="prose"] h2,
  [part="prose"] h3 {
    margin: var(--spacing-6) 0 var(--spacing-3) 0;
    font-family: var(--font-display, Georgia, serif);
    color: var(--color-text-primary);
    font-weight: var(--fw-semibold);
    letter-spacing: .02em;
    line-height: var(--lh-heading);
    text-wrap: balance;
  }

  [part="prose"] h2 {
    font-size: var(--fs-subheader);
    padding-bottom: var(--spacing-2);
    background-image: linear-gradient(90deg, var(--color-bronze), transparent);
    background-repeat: no-repeat;
    background-position: left bottom;
    background-size: 100% 2px;
  }

  [part="prose"] h2::before {
    content: "";
    display: inline-block;
    width: 0.5em;
    height: 0.5em;
    vertical-align: 0.12em;
    background: var(--color-copper);
    box-shadow: 0 0 0 1.5px var(--color-ink);
    transform: rotate(45deg);
    margin-inline-end: .55em;
  }

  [part="prose"] h3 { font-size: var(--fs-body); }

  [part="prose"] > :first-child {
    margin-top: 0;
  }

  [part="prose"] p {
    margin: 0 0 var(--spacing-4) 0;
    max-width: 72ch;
    color: var(--color-text-primary);
    line-height: var(--lh-relaxed, 1.7);
    text-wrap: pretty;
  }

  [part="prose"] li::marker {
    color: var(--color-copper);
  }

  /* Viñeta romboidal cobre (li propias del shadow DOM del componente) */
  [part="prose"] ul > li {
    list-style: none;
    position: relative;
  }

  [part="prose"] ul > li::before {
    content: "";
    position: absolute;
    inset-inline-start: calc(-1 * var(--spacing-4));
    top: 0.62em;
    width: 0.42em;
    height: 0.42em;
    background: var(--color-copper);
    box-shadow: 0 0 0 1px var(--color-ink);
    transform: rotate(45deg);
  }

  [part="prose"] blockquote {
    margin: 0 0 var(--spacing-4) 0;
    padding: var(--spacing-1) 0 var(--spacing-1) var(--spacing-4);
    border-left: 1px solid var(--color-border-strong);
    color: var(--color-text-soft);
  }

  [part="prose"] p:last-child {
    margin-bottom: 0;
  }

  [part="prose"] ul {
    margin: 0 0 var(--spacing-4) 0;
    padding-left: var(--spacing-5);
    display: flex;
    flex-direction: column;
    gap: var(--spacing-2);
  }

  [part="prose"] ul:last-child {
    margin-bottom: 0;
  }

  [part="prose"] strong {
    font-weight: 700;
    color: var(--color-text-primary);
  }

  [part="prose"] code {
    font-family: var(--font-mono);
    font-size: 0.92em;
    font-variant-ligatures: none;
    color: var(--color-gold-text, var(--color-accent-fg));
    background: var(--color-stone-abyss);
    border: 1px solid var(--color-ink);
    padding: 1px 5px;
    border-radius: 2px;
    box-decoration-break: clone;
    -webkit-box-decoration-break: clone;
  }

  [part="prose"] a {
    color: var(--color-accent-fg);
    font-weight: var(--fw-medium);
    text-decoration: underline;
    text-decoration-thickness: 1px;
    text-underline-offset: 3px;
    text-decoration-color: var(--color-accent-ring);
    border-radius: var(--rounded-xs);
    transition: text-decoration-color var(--motion-duration-fast) var(--motion-easing-standard);
  }

  [part="prose"] a:hover {
    text-decoration-color: var(--color-accent-fg);
  }

  @media (prefers-reduced-motion: reduce) {
    [part="prose"] a { transition: none; }
  }

  /* Subtítulo suelto (un "###" seguido de tabla/código): sin tarjeta, como
     encabezado del bloque siguiente. */
  :host([data-variant="heading"]) {
    padding: var(--spacing-2) var(--spacing-1) 0;
    filter: none;
  }
  :host([data-variant="heading"])::before {
    display: none;
  }
  :host([data-variant="heading"]) [part="prose"] :is(h3, h4, h5, h6) {
    margin: 0;
  }

  @media (forced-colors: active) {
    [part="prose"] h2 {
      background-image: none;
      border-bottom: 2px solid CanvasText;
    }
    [part="prose"] h2::before,
    [part="prose"] ul > li::before {
      background: CanvasText;
      box-shadow: none;
      forced-color-adjust: none;
    }
    [part="prose"] code {
      border-color: CanvasText;
      color: CanvasText;
      background: Canvas;
    }
    [part="prose"] a { color: LinkText; text-decoration-color: LinkText; }
  }
`);

export class DxProseBlock extends HTMLElement {
  constructor() {
    super();
    this._data = null;
    this.attachShadow({ mode: 'open' });
    adoptSheets(this.shadowRoot, baseBlockSheet, markdownSheet, proseSheet);
  }

  get data() {
    return this._data;
  }

  set data(val) {
    this._data = val;
    this.render();
  }

  render() {
    clearNode(this.shadowRoot);
    if (!this._data) return;

    const degradedBanner = createDegradedBanner(this._data.degraded);
    if (degradedBanner) {
      this.shadowRoot.appendChild(degradedBanner);
    }

    const mdText = String(this._data.md ?? '').trim();
    const headingOnly = !this._data.title && /^#{1,6}\s+[^\n]+$/.test(mdText);
    if (headingOnly) {
      this.dataset.variant = 'heading';
    } else {
      delete this.dataset.variant;
    }

    const article = el('article', { part: 'prose' });
    if (typeof this._data.title === 'string' && this._data.title.trim()) {
      article.appendChild(el('h2', { part: 'title' }, this._data.title.trim()));
    }
    article.appendChild(renderMarkdownBlocks(String(this._data.md ?? ''), { allowLists: true }));
    this.shadowRoot.appendChild(article);
  }
}

if (!customElements.get('dx-prose-block')) {
  customElements.define('dx-prose-block', DxProseBlock);
}
