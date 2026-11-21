// ==========================================================================
// DioptraEngine Viewer — ui/components/action-checklist.js
// <dx-action-checklist> (§4.7 Contrato A — Progreso animado sin style inline)
// ==========================================================================

import {
  adoptSheets,
  baseBlockSheet,
  clearNode,
  createSheet,
  el,
  renderInlineMarkdown,
  SEV_SET,
  svgEl
} from './shared.js';

/** Sello romboidal (ojo) que reemplaza la caja visual del checkbox. */
function buildSeal() {
  const svg = svgEl('svg', { viewBox: '0 0 28 28', focusable: 'false' },
    svgEl('polygon', { class: 'seal-outer', points: '14,2 26,14 14,26 2,14' }),
    svgEl('polygon', { class: 'seal-inner', points: '14,6.5 22,14 14,21.5 6,14' }),
    svgEl('circle', { class: 'seal-pupil', cx: '14', cy: '14', r: '2.6' }),
    svgEl('circle', { class: 'seal-glint', cx: '15', cy: '13', r: '0.9' })
  );
  return el('span', { className: 'seal', 'aria-hidden': 'true' }, svg);
}

const checklistSheet = createSheet(`
  :host {
    padding: var(--spacing-5) var(--spacing-6);
  }

  [part="progress"] {
    display: flex;
    align-items: center;
    gap: var(--spacing-3);
    margin-bottom: var(--spacing-4);
  }

  [part="count"] {
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    font-weight: var(--fw-semibold);
    font-variant-numeric: tabular-nums;
    font-variant-ligatures: none;
    color: var(--color-text-soft);
    min-width: 38px;
  }

  [part="bar"] {
    flex: 1 1 auto;
    height: 4px;
    border-radius: var(--rounded-sm);
    background: var(--color-surface-sunken);
    box-shadow: inset 0 0 0 var(--border-width) var(--color-border-translucent);
    overflow: hidden;
  }

  /* Progreso: solo transform scaleX, valor vía --progress (0..1) en hoja adoptada */
  .bar-fill {
    width: 100%;
    height: 100%;
    border-radius: var(--rounded-sm);
    background: var(--gradient-accent);
    transform-origin: left center;
    transform: scaleX(var(--progress, 0));
    transition: transform var(--motion-duration-deliberate) var(--motion-easing-snappy);
    will-change: transform;
  }

  [part="list"] {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  [part="item"] {
    border-radius: 0;
    border: 0;
    border-bottom: 1px solid var(--color-border-translucent);
    background-color: transparent;
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard),
                transform var(--motion-duration-fast) var(--motion-easing-standard);
  }

  :host(:not([readonly])) [part="item"]:hover {
    background-color: var(--color-row-hover-bg);
    transform: translateX(2px);
  }

  :host(:not([readonly])) [part="item"]:active {
    background-color: var(--color-surface-hover);
  }

  label {
    display: flex;
    align-items: center;
    gap: var(--spacing-3);
    min-height: var(--row-height);
    padding: var(--spacing-1) var(--spacing-2);
    cursor: pointer;
  }

  :host([readonly]) label {
    cursor: not-allowed;
  }

  /* Checkbox nativo visualmente oculto (focusable) + sello romboidal visual */
  .check-wrap {
    position: relative;
    display: inline-grid;
    place-items: center;
    width: 22px;
    height: 22px;
    flex-shrink: 0;
  }

  input[type="checkbox"] {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    border: 0;
    overflow: hidden;
    clip: rect(0 0 0 0);
    clip-path: inset(50%);
    white-space: nowrap;
  }

  .seal {
    display: inline-block;
    width: 22px;
    height: 22px;
    pointer-events: none;
  }
  .seal svg { display: block; width: 100%; height: 100%; overflow: visible; }
  .seal-outer {
    fill: var(--color-surface-sunken);
    stroke: var(--color-ink);
    stroke-width: 2.4;
    stroke-linejoin: round;
  }
  .seal-inner {
    fill: none;
    stroke: var(--color-bronze);
    stroke-width: 1.6;
    stroke-linejoin: round;
  }
  .seal-pupil { fill: var(--color-ink); opacity: 0; }
  .seal-glint { fill: var(--color-gold-glow); opacity: 0; }
  .seal-outer, .seal-inner, .seal-pupil, .seal-glint {
    transition: fill 140ms var(--motion-easing-standard),
                stroke 140ms var(--motion-easing-standard),
                opacity 140ms var(--motion-easing-standard);
  }
  input[type="checkbox"]:checked + .seal .seal-outer { fill: var(--color-copper); }
  input[type="checkbox"]:checked + .seal .seal-inner { fill: var(--color-gold); stroke: var(--color-ink); }
  input[type="checkbox"]:checked + .seal .seal-pupil,
  input[type="checkbox"]:checked + .seal .seal-glint { opacity: 1; }
  input[type="checkbox"]:focus-visible + .seal {
    outline: 2px solid var(--color-gold);
    outline-offset: 2px;
  }
  input[type="checkbox"]:disabled + .seal { opacity: 0.6; }

  input[type="checkbox"]:disabled {
    cursor: not-allowed;
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

  [part="text"] {
    flex: 1 1 auto;
    font-size: var(--fs-small);
    color: var(--color-text-primary);
    min-width: 0;
    text-decoration-line: line-through;
    text-decoration-thickness: 1px;
    text-decoration-color: transparent;
    transition: color var(--motion-duration-normal) var(--motion-easing-standard),
                text-decoration-color var(--motion-duration-normal) var(--motion-easing-standard);
  }

  [part="text"] code {
    font-family: var(--font-mono);
    font-size: 0.92em;
    font-variant-ligatures: none;
    background: var(--color-surface-sunken);
    border: var(--border-width) solid var(--color-border-translucent);
    padding: 0 5px;
    border-radius: var(--rounded-xs);
  }

  [part="item"][data-done="true"] [part="text"] {
    color: var(--color-text-soft);
    text-decoration: line-through;
    text-decoration-color: var(--color-copper);
  }

  /* Chip @owner: pastilla mono micro */
  [part="owner"] {
    flex-shrink: 0;
    padding: 1px var(--spacing-2);
    border-radius: var(--rounded-sm);
    background: var(--color-surface-sunken);
    border: var(--border-width) solid var(--color-border-translucent);
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    font-weight: var(--fw-medium);
    font-variant-ligatures: none;
    letter-spacing: var(--ls-caps);
    line-height: 1.6;
    color: var(--color-text-soft);
    white-space: nowrap;
  }

  @media (prefers-reduced-motion: reduce) {
    .seal-outer, .seal-inner, .seal-pupil, .seal-glint { transition: none; }
    [part="item"] { transition: none; }
    :host(:not([readonly])) [part="item"]:hover { transform: none; }
  }

  @media (forced-colors: active) {
    [part="item"] { border-color: CanvasText; }
    .seal-outer { fill: Canvas; stroke: CanvasText; }
    .seal-inner { stroke: CanvasText; }
    input[type="checkbox"]:checked + .seal .seal-outer,
    input[type="checkbox"]:checked + .seal .seal-inner { fill: Highlight; stroke: CanvasText; }
    input[type="checkbox"]:checked + .seal .seal-pupil,
    input[type="checkbox"]:checked + .seal .seal-glint { fill: CanvasText; }
    input[type="checkbox"]:focus-visible + .seal { outline-color: Highlight; }
    .bar-fill { background: Highlight; }
  }
`);

export class DxActionChecklist extends HTMLElement {
  static get observedAttributes() {
    return ['readonly'];
  }

  constructor() {
    super();
    this._data = null;
    this._items = [];
    this._countEl = null;
    this._progressEl = null;
    this._dynamicSheet = createSheet(':host { --progress: 0; }');
    this.attachShadow({ mode: 'open' });
    adoptSheets(this.shadowRoot, baseBlockSheet, checklistSheet, this._dynamicSheet);
  }

  get data() {
    return this._data;
  }

  set data(val) {
    this._data = val;
    this._items = Array.isArray(val?.items) ? val.items.map((it) => ({ ...it })) : [];
    if (val?.readonly) {
      this.setAttribute('readonly', '');
    } else {
      this.removeAttribute('readonly');
    }
    this.render();
  }

  attributeChangedCallback(name, oldVal, newVal) {
    if (name === 'readonly' && oldVal !== newVal && this._data) {
      this.render();
    }
  }

  _updateProgress() {
    const total = this._items.length;
    const doneCount = this._items.filter((i) => i.done).length;
    const ratio = total > 0 ? Math.round((doneCount / total) * 10000) / 10000 : 0;
    this._dynamicSheet.replaceSync(`:host { --progress: ${ratio}; }`);
    if (this._countEl) this._countEl.textContent = `${doneCount}/${total}`;
    if (this._progressEl) {
      this._progressEl.max = Math.max(1, total);
      this._progressEl.value = doneCount;
    }
  }

  render() {
    clearNode(this.shadowRoot);
    const isReadonly = this.hasAttribute('readonly');
    const total = this._items.length;
    const doneCount = this._items.filter((i) => i.done).length;

    this._countEl = el('span', { part: 'count' }, `${doneCount}/${total}`);
    this._progressEl = el('progress', {
      className: 'sr-only',
      max: Math.max(1, total),
      value: doneCount,
      'aria-label': 'Progreso de tareas completadas'
    });
    const bar = el('div', { part: 'bar' }, el('div', { className: 'bar-fill' }));
    this.shadowRoot.appendChild(el('header', { part: 'progress' }, this._countEl, this._progressEl, bar));

    const ul = el('ul', { part: 'list', role: 'list' });

    for (const item of this._items) {
      const sev = SEV_SET.has(item.sev) ? item.sev : undefined;
      const liAttrs = { part: 'item', 'data-done': item.done ? 'true' : 'false' };
      if (sev) liAttrs['data-sev'] = sev;

      const checkbox = el('input', {
        type: 'checkbox',
        checked: item.done,
        disabled: isReadonly,
        'aria-readonly': isReadonly ? 'true' : undefined
      });

      const textSpan = el('span', { part: 'text' });
      textSpan.appendChild(renderInlineMarkdown(String(item.text ?? '')));

      const labelChildren = [
        el('span', { className: 'check-wrap' }, checkbox, buildSeal())
      ];
      if (sev) {
        labelChildren.push(el('span', { className: 'sev-dot', 'data-sev': sev, 'aria-hidden': 'true' }));
      }
      labelChildren.push(textSpan);
      if (item.owner) {
        labelChildren.push(el('span', { part: 'owner' }, String(item.owner)));
      }

      const li = el('li', liAttrs, el('label', null, ...labelChildren));

      checkbox.addEventListener('change', () => {
        item.done = checkbox.checked;
        li.setAttribute('data-done', item.done ? 'true' : 'false');
        this._updateProgress();
        this.dispatchEvent(new CustomEvent('dx-check', {
          bubbles: true,
          composed: true,
          detail: { id: item.id, done: item.done }
        }));
        if (item.done === true) {
          this.dispatchEvent(new CustomEvent('dx-sigil-blink', {
            bubbles: true,
            composed: true,
            detail: { flash: true }
          }));
        }
      });

      ul.appendChild(li);
    }

    this.shadowRoot.appendChild(ul);
    this._updateProgress();
  }
}

if (!customElements.get('dx-action-checklist')) {
  customElements.define('dx-action-checklist', DxActionChecklist);
}
