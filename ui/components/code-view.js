// ==========================================================================
// DioptraEngine Viewer — ui/components/code-view.js
// <dx-code-view> (§4.5 Contrato A — Código, resaltado hl[], diff y copiado)
// ==========================================================================

import {
  adoptSheets,
  baseBlockSheet,
  clearNode,
  createDegradedBanner,
  createRefButton,
  createSheet,
  el
} from './shared.js';

const codeSheet = createSheet(`
  :host {
    padding: var(--spacing-4);
    filter: none;
  }

  /* El hijo ya es la losa: sin marco base duplicado */
  :host::before {
    display: none;
  }

  /* Tótem de obsidiana: cabecera losa→obsidiana con filete de tinta */
  [part="bar"] {
    display: flex;
    align-items: center;
    gap: var(--spacing-2);
    min-height: var(--row-height);
    padding: var(--spacing-1) var(--spacing-2) var(--spacing-1) var(--spacing-3);
    flex-wrap: wrap;
    background: linear-gradient(90deg, var(--color-stone-slab), var(--color-stone-obsidian));
    border: 2px solid var(--color-ink);
    border-bottom: 2px solid var(--color-ink);
    border-radius: var(--rounded-sm) var(--rounded-sm) 0 0;
    box-shadow: inset 0 1px 0 var(--color-bronze-light);
  }

  /* Rótulo del lenguaje: versalitas display precedidas de rombo cobre */
  [part="lang"] {
    display: inline-flex;
    align-items: center;
    gap: 0.55em;
    font-family: var(--font-display);
    font-size: var(--fs-caption);
    font-weight: var(--fw-semibold);
    font-variant: small-caps;
    font-variant-ligatures: none;
    text-transform: lowercase;
    letter-spacing: .08em;
    line-height: 1.4;
    color: var(--color-accent-fg);
    padding: 1px var(--spacing-1);
    background: transparent;
    border: 0;
  }

  [part="lang"]::before {
    content: "";
    flex-shrink: 0;
    width: 0.55em;
    height: 0.55em;
    background: var(--color-copper);
    box-shadow: 0 0 0 1.5px var(--color-ink);
    transform: rotate(45deg);
  }

  /* Nombre de archivo (ref path:línea) en mono dentro de la barra */
  [part="bar"] [part="ref"] {
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    font-variant-ligatures: none;
    font-variant-numeric: tabular-nums;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .bar-spacer {
    flex: 1 1 auto;
  }

  [part="wrap"],
  [part="copy"] {
    height: 28px;
    padding: 0 var(--spacing-3);
    border-radius: var(--rounded-sm);
    /* Sello de piedra: contorno tinta + bisel + sombra dura */
    border: 2px solid var(--color-ink);
    background: linear-gradient(180deg, var(--color-stone-steel), var(--color-stone-slab));
    box-shadow: inset 0 1px 0 var(--color-bronze-light), 2px 2px 0 var(--color-ink-shadow);
    color: var(--color-text-primary);
    font-family: var(--font-display);
    font-variant: small-caps;
    letter-spacing: .06em;
    font-size: var(--fs-caption);
    font-weight: var(--fw-semibold);
    cursor: pointer;
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard),
                border-color var(--motion-duration-fast) var(--motion-easing-standard),
                color var(--motion-duration-fast) var(--motion-easing-standard),
                transform var(--motion-duration-instant) var(--motion-easing-standard),
                box-shadow var(--motion-duration-instant) var(--motion-easing-standard);
  }

  [part="wrap"]:hover,
  [part="copy"]:hover {
    background: var(--color-stone-steel);
    border-color: var(--color-ink);
    color: var(--color-accent-fg);
  }

  [part="wrap"]:active,
  [part="copy"]:active {
    transform: translate(1px, 1px);
    box-shadow: inset 0 1px 0 var(--color-bronze-light), 1px 1px 0 var(--color-ink-shadow);
  }

  [part="wrap"]:focus-visible,
  [part="copy"]:focus-visible {
    outline: none;
    box-shadow: inset 0 1px 0 var(--color-bronze-light), var(--focus-ring);
  }

  [part="wrap"][aria-pressed="true"] {
    background: var(--color-interactive-primary);
    border-color: var(--color-ink);
    box-shadow: var(--bevel-copper), 2px 2px 0 var(--color-ink-shadow);
    color: var(--color-on-primary);
  }

  /* Feedback de copiado: señal success (tinte + borde + texto) con pop spring */
  [part="copy"][data-copied="true"],
  [part="copy"][data-copied="true"]:hover {
    color: var(--color-sev-success-fg);
    border-color: var(--color-sev-success-border);
    background: var(--color-sev-success-bg);
    animation: copy-ok var(--motion-duration-normal) var(--motion-easing-emphasized);
  }

  @keyframes copy-ok {
    from { transform: scale(0.92); }
    to { transform: scale(1); }
  }

  @media (prefers-reduced-motion: reduce) {
    [part="copy"][data-copied="true"],
    [part="copy"][data-copied="true"]:hover {
      animation: none;
    }
    [part="wrap"]:active,
    [part="copy"]:active {
      transform: none;
    }
    [part="wrap"],
    [part="copy"] {
      transition: none;
    }
  }

  [part="pre"] {
    margin: 0;
    padding: var(--spacing-3) 0;
    background: var(--color-surface-sunken);
    border: 2px solid var(--color-ink);
    border-top: 0;
    border-radius: 0 0 var(--rounded-sm) var(--rounded-sm);
    box-shadow: inset 0 2px 0 var(--color-ink-shadow), 3px 4px 0 var(--color-ink-shadow);
    overflow: auto;
    max-height: 60vh;
    font-family: var(--font-mono);
    font-size: var(--fs-small);
    line-height: var(--lh-body);
    tab-size: 2;
    font-variant-ligatures: none;
  }

  code {
    display: table;
    min-width: 100%;
    font-family: inherit;
  }

  .code-row {
    display: table-row;
  }

  /* Línea resaltada: solo tinte (sin franja lateral; el gutter cobre marca la línea) */
  .code-row.hl {
    background: var(--code-line-hl-bg);
  }

  .code-row.diff-add {
    background: var(--diff-add-bg);
  }

  .code-row.diff-del {
    background: var(--diff-del-bg);
  }

  /* Gutter: text-soft atenuado, separador translúcido y no seleccionable */
  .ln {
    display: table-cell;
    width: 1%;
    min-width: 44px;
    padding: 0 var(--spacing-3);
    text-align: right;
    user-select: none;
    -webkit-user-select: none;
    color: var(--color-text-soft);
    opacity: 0.7;
    font-variant-numeric: tabular-nums;
    border-right: var(--border-width) solid var(--color-border-translucent);
    white-space: nowrap;
  }

  .code-row.hl .ln {
    color: var(--color-accent-fg);
    opacity: 1;
  }

  .code-row.diff-add .ln {
    color: var(--color-sev-success-fg);
    opacity: 1;
  }

  .code-row.diff-del .ln {
    color: var(--color-sev-error-fg);
    opacity: 1;
  }

  .diff-sign {
    display: table-cell;
    width: 20px;
    text-align: center;
    user-select: none;
    -webkit-user-select: none;
    font-weight: var(--fw-semibold);
  }

  .code-row.diff-add .diff-sign { color: var(--color-sev-success-fg); }
  .code-row.diff-del .diff-sign { color: var(--color-sev-error-fg); }
  /* Cabeceras de archivo (--- / +++ / diff --git) y de hunk (@@ … @@) */
  .code-row.diff-hdr .line {
    color: var(--color-text-primary);
    font-weight: var(--fw-semibold, 600);
  }
  .code-row.diff-hunk {
    background: var(--color-sev-info-bg, transparent);
  }
  .code-row.diff-hunk .line {
    color: var(--color-sev-info-fg, var(--color-text-soft));
  }

  .line {
    display: table-cell;
    padding: 0 var(--spacing-4) 0 var(--spacing-2);
    white-space: pre;
    color: var(--syn-text);
  }

  :host([wrap]) .line {
    white-space: pre-wrap;
    word-break: break-word;
  }

  .syn-keyword { color: var(--syn-keyword); font-weight: var(--fw-semibold); }
  .syn-string { color: var(--syn-string); }
  .syn-number { color: var(--syn-number); }
  .syn-function { color: var(--syn-function); }
  .syn-type { color: var(--syn-type); }
  .syn-comment { color: var(--syn-comment); font-style: italic; }

  @media (forced-colors: active) {
    [part="bar"] {
      background: Canvas;
      border-color: CanvasText;
      box-shadow: none;
    }
    [part="lang"] {
      color: CanvasText;
    }
    [part="lang"]::before {
      background: CanvasText;
      box-shadow: none;
      forced-color-adjust: none;
    }
    [part="wrap"],
    [part="copy"] {
      border-color: ButtonText;
      box-shadow: none;
    }
    [part="wrap"]:focus-visible,
    [part="copy"]:focus-visible {
      outline: 2px solid Highlight;
      outline-offset: 2px;
    }
    [part="wrap"][aria-pressed="true"] {
      background: Highlight;
      color: HighlightText;
      forced-color-adjust: none;
    }
    .code-row.hl {
      outline: 1px solid Highlight;
      outline-offset: -1px;
    }
  }
`);

const KEYWORDS = new Set([
  'func', 'package', 'import', 'return', 'if', 'else', 'for', 'range', 'switch',
  'case', 'default', 'struct', 'interface', 'type', 'var', 'const', 'defer', 'go',
  'select', 'chan', 'map', 'fallthrough', 'break', 'continue', 'nil', 'true', 'false',
  'export', 'class', 'extends', 'async', 'await', 'try', 'catch', 'throw', 'new',
  'let', 'from', 'SELECT', 'FROM', 'WHERE', 'INSERT', 'UPDATE', 'JOIN', 'AND', 'OR'
]);

const TYPES = new Set([
  'string', 'int', 'int64', 'int32', 'uint', 'uint64', 'byte', 'bool', 'error',
  'float64', 'Context', 'ResponseWriter', 'Request', 'Time', 'Duration', 'Buffer',
  'HTMLElement', 'Node', 'Document', 'Promise', 'Array', 'Object', 'Map', 'Set'
]);

const SYN_CLASSES = new Set([
  'syn-keyword', 'syn-string', 'syn-number', 'syn-function', 'syn-type', 'syn-comment', 'syn-text'
]);

function renderTokenizedLine(lineText, preInjectedSpans) {
  const frag = document.createDocumentFragment();
  if (Array.isArray(preInjectedSpans)) {
    for (const seg of preInjectedSpans) {
      if (!seg || typeof seg.t !== 'string') continue;
      if (seg.c && SYN_CLASSES.has(seg.c)) {
        frag.appendChild(el('span', { className: seg.c }, seg.t));
      } else {
        frag.appendChild(document.createTextNode(seg.t));
      }
    }
    return frag;
  }

  // Respaldo léxico ligero en vista previa local cuando Go aún no inyecta tokens
  const tokenRe = /(\/\/.*$|#.*$|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`[^`]*`|\b\d+(?:\.\d+)?\b|\b[A-Za-z_][A-Za-z0-9_]*\b(?=\s*\()|\b[A-Za-z_][A-Za-z0-9_]*\b)/g;
  let last = 0;
  let m;
  while ((m = tokenRe.exec(lineText)) !== null) {
    if (m.index > last) {
      frag.appendChild(document.createTextNode(lineText.slice(last, m.index)));
    }
    const tok = m[0];
    let cls = '';
    if (tok.startsWith('//') || tok.startsWith('#')) {
      cls = 'syn-comment';
    } else if (tok.startsWith('"') || tok.startsWith("'") || tok.startsWith('`')) {
      cls = 'syn-string';
    } else if (/^\d/.test(tok)) {
      cls = 'syn-number';
    } else if (KEYWORDS.has(tok)) {
      cls = 'syn-keyword';
    } else if (TYPES.has(tok)) {
      cls = 'syn-type';
    } else if (lineText[m.index + tok.length] === '(') {
      cls = 'syn-function';
    }

    if (cls) {
      frag.appendChild(el('span', { className: cls }, tok));
    } else {
      frag.appendChild(document.createTextNode(tok));
    }
    last = m.index + tok.length;
  }
  if (last < lineText.length) {
    frag.appendChild(document.createTextNode(lineText.slice(last)));
  }
  if (lineText.length === 0) {
    frag.appendChild(document.createTextNode('\n'));
  }
  return frag;
}

function computeUnifiedDiff(oldStr, newStr) {
  const a = oldStr.replace(/\r\n/g, '\n').split('\n');
  const b = newStr.replace(/\r\n/g, '\n').split('\n');
  const m = a.length;
  const n = b.length;

  // LCS matricial acotado para bloques de código típicos; fallback lineal si es masivo
  if (m * n > 250000) {
    const out = [];
    a.forEach((line, i) => out.push({ kind: 'del', ln: i + 1, text: line }));
    b.forEach((line, i) => out.push({ kind: 'add', ln: i + 1, text: line }));
    return out;
  }

  const dp = Array.from({ length: m + 1 }, () => new Uint16Array(n + 1));
  for (let i = m - 1; i >= 0; i--) {
    for (let j = n - 1; j >= 0; j--) {
      dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }

  const rows = [];
  let i = 0;
  let j = 0;
  while (i < m && j < n) {
    if (a[i] === b[j]) {
      rows.push({ kind: 'ctx', ln: j + 1, text: b[j] });
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      rows.push({ kind: 'del', ln: i + 1, text: a[i] });
      i++;
    } else {
      rows.push({ kind: 'add', ln: j + 1, text: b[j] });
      j++;
    }
  }
  while (i < m) {
    rows.push({ kind: 'del', ln: i + 1, text: a[i++] });
  }
  while (j < n) {
    rows.push({ kind: 'add', ln: j + 1, text: b[j++] });
  }
  return rows;
}

export class DxCodeView extends HTMLElement {
  constructor() {
    super();
    this._data = null;
    this._copyTimer = null;
    this.attachShadow({ mode: 'open' });
    adoptSheets(this.shadowRoot, baseBlockSheet, codeSheet);
  }

  get data() {
    return this._data;
  }

  set data(val) {
    this._data = val;
    const lang = typeof val?.lang === 'string' && val.lang ? val.lang : 'text';
    const isDiff = typeof val?.old === 'string' || Array.isArray(val?.hunks);
    this.setAttribute('lang', lang);
    this.setAttribute('mode', isDiff ? 'diff' : 'code');
    this.render();
  }

  render() {
    clearNode(this.shadowRoot);
    if (!this._data) return;

    const degradedBanner = createDegradedBanner(this._data.degraded);
    if (degradedBanner) {
      this.shadowRoot.appendChild(degradedBanner);
    }

    const lang = this.getAttribute('lang') || 'text';
    const mode = this.getAttribute('mode') || 'code';
    const isWrap = this.hasAttribute('wrap');

    const wrapBtn = el('button', {
      type: 'button',
      part: 'wrap',
      'aria-pressed': isWrap ? 'true' : 'false'
    }, 'Ajustar');
    wrapBtn.addEventListener('click', () => {
      const next = !this.hasAttribute('wrap');
      if (next) this.setAttribute('wrap', '');
      else this.removeAttribute('wrap');
      wrapBtn.setAttribute('aria-pressed', next ? 'true' : 'false');
    });

    const copyBtn = el('button', {
      type: 'button',
      part: 'copy',
      'aria-live': 'polite'
    }, 'Copiar');
    copyBtn.addEventListener('click', async () => {
      const textToCopy = String(this._data.src ?? '');
      try {
        if (navigator.clipboard?.writeText) {
          await navigator.clipboard.writeText(textToCopy);
        }
      } catch {
        // Ignorado en contextos restringidos sin permiso de portapapeles
      }
      copyBtn.textContent = 'Copiado';
      copyBtn.dataset.copied = 'true';
      if (this._copyTimer) clearTimeout(this._copyTimer);
      this._copyTimer = setTimeout(() => {
        copyBtn.textContent = 'Copiar';
        delete copyBtn.dataset.copied;
      }, 1500);
      this.dispatchEvent(new CustomEvent('dx-copy', {
        bubbles: true,
        composed: true,
        detail: { lang }
      }));
    });

    const barChildren = [el('span', { part: 'lang' }, mode === 'diff' && lang !== 'diff' ? `${lang} · diff` : lang)];
    if (this._data.ref) {
      const refBtn = createRefButton(this._data.ref, 'ref');
      if (refBtn) barChildren.push(refBtn);
    }
    barChildren.push(el('span', { className: 'bar-spacer' }), wrapBtn, copyBtn);
    this.shadowRoot.appendChild(el('div', { part: 'bar' }, ...barChildren));

    const codeEl = el('code');
    const hlSet = new Set(Array.isArray(this._data.hl) ? this._data.hl : []);

    if (mode === 'diff') {
      const diffRows = Array.isArray(this._data.hunks)
        ? this._data.hunks
        : computeUnifiedDiff(String(this._data.old ?? ''), String(this._data.src ?? ''));

      for (const r of diffRows) {
        const rowCls = r.kind === 'add' ? 'code-row diff-add'
          : r.kind === 'del' ? 'code-row diff-del'
          : r.kind === 'hunk' ? 'code-row diff-hunk'
          : r.kind === 'hdr' ? 'code-row diff-hdr'
          : 'code-row';
        const sign = r.kind === 'add' ? '+' : r.kind === 'del' ? '−' : ' ';
        const lineCell = el('span', { className: 'line' });
        lineCell.appendChild(renderTokenizedLine(String(r.text ?? ''), r.tokens));
        codeEl.appendChild(el(
          'span',
          { className: rowCls },
          el('span', { className: 'ln' }, r.ln ? String(r.ln) : ''),
          el('span', { className: 'diff-sign' }, sign),
          lineCell
        ));
      }
    } else {
      const srcLines = String(this._data.src ?? '').replace(/\r\n/g, '\n').split('\n');
      const tokenLines = Array.isArray(this._data.tokens) ? this._data.tokens : null;

      srcLines.forEach((lineText, idx) => {
        const ln = idx + 1;
        const isHl = hlSet.has(ln);
        const lineCell = el('span', { className: 'line' });
        lineCell.appendChild(renderTokenizedLine(lineText, tokenLines ? tokenLines[idx] : undefined));
        codeEl.appendChild(el(
          'span',
          { className: isHl ? 'code-row hl' : 'code-row' },
          el('span', { className: 'ln' }, String(ln)),
          lineCell
        ));
      });
    }

    this.shadowRoot.appendChild(el('pre', { part: 'pre' }, codeEl));
  }
}

if (!customElements.get('dx-code-view')) {
  customElements.define('dx-code-view', DxCodeView);
}
