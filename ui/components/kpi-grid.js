// ==========================================================================
// DioptraEngine Viewer — ui/components/kpi-grid.js
// <dx-kpi-grid> (§4.1 Contrato A)
// ==========================================================================

import { adoptSheets, baseBlockSheet, clearNode, createSheet, createUiIcon, el, SEV_SET } from './shared.js';

const kpiSheet = createSheet(`
  :host {
    background: transparent;
    border: 0;
    padding: 0;
    box-shadow: none;
    border-radius: 0;
    filter: none;
  }

  /* Cada KPI ya es la losa: sin marco base duplicado */
  :host::before {
    display: none;
  }

  [part="grid"] {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: var(--spacing-4);
    margin: 0;
    padding: 0;
  }

  /* Reliquia: losa achaflanada con contorno de tinta y sombra dura */
  [part="kpi"] {
    --kpi-i: 0;
    --kpi-seal: var(--color-bronze);
    --kpi-clip: var(--clip-chamfer-md, polygon(10px 0, calc(100% - 10px) 0, 100% 10px, 100% calc(100% - 10px), calc(100% - 10px) 100%, 10px 100%, 0 calc(100% - 10px), 0 10px));
    position: relative;
    isolation: isolate;
    display: flex;
    flex-direction: column;
    gap: var(--spacing-2);
    background: transparent;
    border: 0;
    border-radius: 0;
    padding: var(--spacing-4) var(--spacing-4) var(--spacing-4) var(--spacing-4);
    box-shadow: none;
    min-width: 0;
    filter: drop-shadow(3px 4px 0 var(--color-ink-shadow, #040609));
  }

  /* Fondo + contorno + filete superior de cobre (3px), todo recortado por el chaflán */
  [part="kpi"]::before {
    content: "";
    position: absolute;
    inset: 0;
    z-index: -1;
    background:
      linear-gradient(135deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) top left / 10px 10px no-repeat border-box,
      linear-gradient(225deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) top right / 10px 10px no-repeat border-box,
      linear-gradient(315deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) bottom right / 10px 10px no-repeat border-box,
      linear-gradient(45deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) bottom left / 10px 10px no-repeat border-box,
      linear-gradient(90deg, var(--color-copper-deep), var(--color-copper-strong), var(--color-copper-deep)) top left / 100% 3px no-repeat,
      linear-gradient(180deg, var(--color-stone-slab), var(--color-surface-base));
    box-shadow: inset 0 0 0 2px var(--color-ink), var(--bevel-stone, none);
    clip-path: var(--kpi-clip);
    pointer-events: none;
    transition: filter var(--motion-duration-fast) var(--motion-easing-standard);
  }

  [part="kpi"]:hover::before {
    filter: brightness(1.06);
  }

  /* Sello romboidal de severidad */
  [part="kpi"][data-sev]::after {
    content: "";
    position: absolute;
    top: 14px;
    right: 14px;
    width: 9px;
    height: 9px;
    transform: rotate(45deg);
    background: var(--kpi-seal);
    box-shadow: 0 0 0 2px var(--color-ink);
    pointer-events: none;
  }

  [part="kpi"][data-sev="info"]    { --kpi-seal: var(--color-sev-info-border); }
  [part="kpi"][data-sev="success"] { --kpi-seal: var(--color-sev-success-border); }
  [part="kpi"][data-sev="warning"] { --kpi-seal: var(--color-sev-warning-border); }
  [part="kpi"][data-sev="error"]   { --kpi-seal: var(--color-sev-error-border); }

  [part="kpi"][data-sev] [part="label"] {
    padding-right: 18px;
  }

  /* Stagger interno del número (máx. 12 celdas) */
  [part="kpi"]:nth-child(2)  { --kpi-i: 1; }
  [part="kpi"]:nth-child(3)  { --kpi-i: 2; }
  [part="kpi"]:nth-child(4)  { --kpi-i: 3; }
  [part="kpi"]:nth-child(5)  { --kpi-i: 4; }
  [part="kpi"]:nth-child(6)  { --kpi-i: 5; }
  [part="kpi"]:nth-child(7)  { --kpi-i: 6; }
  [part="kpi"]:nth-child(8)  { --kpi-i: 7; }
  [part="kpi"]:nth-child(9)  { --kpi-i: 8; }
  [part="kpi"]:nth-child(10) { --kpi-i: 9; }
  [part="kpi"]:nth-child(11) { --kpi-i: 10; }
  [part="kpi"]:nth-child(12) { --kpi-i: 11; }

  /* Eyebrow */
  [part="label"] {
    margin: 0;
    font-family: var(--font-display, Georgia, serif);
    font-size: var(--fs-small);
    font-weight: var(--fw-semibold);
    font-variant: small-caps;
    line-height: 1.3;
    /* accent-fg (7.01:1 claro / 6.58:1 oscuro); bronce queda solo en ornamentos */
    color: var(--color-accent-fg);
    text-transform: none;
    letter-spacing: 0.08em;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* Valor: mono tabular grande, tracking tight */
  [part="value"] {
    margin: 0;
    display: flex;
    align-items: baseline;
    gap: 4px;
    font-family: var(--font-display, Georgia, serif);
    font-size: var(--fs-h3);
    font-weight: var(--fw-semibold);
    font-variant-numeric: tabular-nums;
    font-variant-ligatures: none;
    letter-spacing: var(--ls-tight);
    line-height: var(--lh-heading);
    color: var(--color-gold-text, var(--color-text-primary));
    text-shadow: 2px 2px 0 var(--color-ink-shadow);
    animation: kpi-value-in var(--motion-duration-slow) var(--motion-easing-decelerate) both;
    animation-delay: calc(var(--motion-stagger) * var(--kpi-i));
  }

  [part="unit"] {
    font-family: var(--font-sans);
    font-size: var(--fs-small);
    font-weight: var(--fw-regular);
    letter-spacing: normal;
    color: var(--color-text-secondary);
    text-shadow: none;
  }

  /* Delta como chip tintado con flecha */
  [part="delta"] {
    align-self: flex-start;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin: 0;
    padding: 1px var(--spacing-2);
    border-radius: var(--rounded-sm);
    border: var(--border-width) solid var(--color-border-translucent);
    background: var(--color-surface-sunken);
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    font-weight: var(--fw-medium);
    font-variant-numeric: tabular-nums;
    font-variant-ligatures: none;
    line-height: 1.5;
    color: var(--color-text-secondary);
  }

  [part="delta"] svg {
    flex-shrink: 0;
  }

  [part="kpi"][data-sev="info"] [part="delta"] {
    color: var(--color-sev-info-fg);
    background: var(--color-sev-info-bg);
    border-color: var(--color-sev-info-border);
  }
  [part="kpi"][data-sev="success"] [part="delta"] {
    color: var(--color-sev-success-fg);
    background: var(--color-sev-success-bg);
    border-color: var(--color-sev-success-border);
  }
  [part="kpi"][data-sev="warning"] [part="delta"] {
    color: var(--color-sev-warning-fg);
    background: var(--color-sev-warning-bg);
    border-color: var(--color-sev-warning-border);
  }
  [part="kpi"][data-sev="error"] [part="delta"] {
    color: var(--color-sev-error-fg);
    background: var(--color-sev-error-bg);
    border-color: var(--color-sev-error-border);
  }

  @keyframes kpi-value-in {
    from {
      opacity: 0;
      transform: translateY(var(--motion-distance-sm));
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    [part="value"] {
      animation: none;
    }
    [part="kpi"],
    [part="kpi"]::before,
    [part="kpi"]::after {
      transition: none;
    }
  }

  @media (forced-colors: active) {
    [part="kpi"] { filter: none; }
    [part="kpi"]::before {
      clip-path: none;
      background: Canvas;
      box-shadow: none;
      border: 1px solid CanvasText;
    }
    [part="kpi"]:hover::before { filter: none; }
    [part="kpi"][data-sev]::after {
      background: CanvasText;
      box-shadow: none;
      forced-color-adjust: none;
    }
    [part="value"] { text-shadow: none; }
  }
`);

function deltaTrendClass(delta) {
  const s = String(delta).trim();
  // Si el texto ya trae glifo de tendencia, no duplicar el icono SVG
  if (/^(\u25B2|\u25BC|\u2191|\u2193)/.test(s)) return 'delta--flat';
  if (/^\+/.test(s)) return 'delta--up';
  if (/^(-|\u2212|\u2013)/.test(s)) return 'delta--down';
  return 'delta--flat';
}

export class DxKpiGrid extends HTMLElement {
  constructor() {
    super();
    this._data = null;
    this.attachShadow({ mode: 'open' });
    adoptSheets(this.shadowRoot, baseBlockSheet, kpiSheet);
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
    const items = Array.isArray(this._data?.items) ? this._data.items.slice(0, 12) : [];
    const dl = el('dl', { part: 'grid' });

    for (const item of items) {
      const sev = SEV_SET.has(item.sev) ? item.sev : undefined;
      const cardAttrs = { part: 'kpi' };
      if (sev) cardAttrs['data-sev'] = sev;

      const unitSpan = el('span', { part: 'unit' }, item.unit ? ` ${item.unit}` : '');
      const valueDd = el('dd', { part: 'value' }, String(item.value ?? ''), unitSpan);
      const children = [
        el('dt', { part: 'label', title: item.label || '' }, item.label || ''),
        valueDd
      ];
      if (item.delta) {
        const trend = deltaTrendClass(item.delta);
        const trendIcon = trend === 'delta--up'
          ? createUiIcon('trend-up', 12)
          : trend === 'delta--down'
            ? createUiIcon('trend-down', 12)
            : null;
        children.push(el('dd', { part: 'delta', className: trend }, trendIcon, String(item.delta)));
      }
      dl.appendChild(el('div', cardAttrs, ...children));
    }

    this.shadowRoot.appendChild(dl);
  }
}

if (!customElements.get('dx-kpi-grid')) {
  customElements.define('dx-kpi-grid', DxKpiGrid);
}
