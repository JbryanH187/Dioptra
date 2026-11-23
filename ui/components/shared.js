// ==========================================================================
// DioptraEngine Viewer — ui/components/shared.js
// Utilidades DOM seguras (cero inserción HTML cruda), CSSStyleSheet compartida,
// iconos vectoriales Reliquia, AST Markdown seguro y despachador ADR 1.
// ==========================================================================

const SVG_NS = 'http://www.w3.org/2000/svg';

export const SEV_SET = new Set(['info', 'success', 'warning', 'error']);

export const SEV_LABELS = Object.freeze({
  info: 'Info',
  success: 'Correcto',
  warning: 'Advertencia',
  error: 'Error'
});

export const SEV_RANK = Object.freeze({
  error: 4,
  warning: 3,
  info: 2,
  success: 1
});

export function createSheet(cssText) {
  const sheet = new CSSStyleSheet();
  sheet.replaceSync(cssText);
  return sheet;
}

export function adoptSheets(shadowRoot, ...sheets) {
  shadowRoot.adoptedStyleSheets = sheets;
}

export const baseBlockSheet = createSheet(`
  *, *::before, *::after {
    box-sizing: border-box;
  }

  :host {
    display: block;
    position: relative;
    isolation: isolate;
    padding: var(--spacing-6);
    font: var(--fw-regular) var(--fs-small)/var(--lh-body) var(--font-sans);
    color: var(--color-text-primary);
    min-width: 0;
    filter: drop-shadow(2px 2px 0 var(--color-ink-shadow, #040609));
  }

  :host::before {
    content: '';
    position: absolute;
    inset: 0;
    z-index: -1;
    pointer-events: none;
    background:
      linear-gradient(135deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) top left / 7px 7px no-repeat border-box,
      linear-gradient(225deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) top right / 7px 7px no-repeat border-box,
      linear-gradient(315deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) bottom right / 7px 7px no-repeat border-box,
      linear-gradient(45deg, transparent calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% - 0.5px), var(--color-ink, #06080C) calc(50% + 2px), transparent calc(50% + 2px)) bottom left / 7px 7px no-repeat border-box,
      var(--color-surface-base);
    box-shadow: inset 0 0 0 2px var(--color-ink, #06080C), var(--bevel-stone, none);
    clip-path: var(--clip-chamfer-sm, polygon(7px 0, calc(100% - 7px) 0, 100% 7px, 100% calc(100% - 7px), calc(100% - 7px) 100%, 7px 100%, 0 calc(100% - 7px), 0 7px));
  }

  :host([hidden]) {
    display: none;
  }

  button, input {
    font: inherit;
    color: inherit;
  }

  :focus-visible {
    outline: none;
    box-shadow: var(--focus-ring);
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }

  [part="title"] {
    margin: 0 0 var(--spacing-4) 0;
    font-family: var(--font-display, Georgia, serif);
    font-size: var(--fs-subheader);
    font-weight: var(--fw-semibold);
    letter-spacing: .02em;
    line-height: var(--lh-heading);
    color: var(--color-text-primary);
    text-wrap: balance;
  }

  button.ref {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 1px 6px;
    margin: 0;
    border: var(--border-width) solid var(--color-border-translucent);
    border-radius: var(--rounded-xs);
    background: var(--color-surface-sunken);
    color: var(--color-accent-fg);
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    font-variant-numeric: tabular-nums;
    font-variant-ligatures: none;
    line-height: 1.4;
    cursor: pointer;
    transition: background-color var(--motion-duration-fast) var(--motion-easing-standard),
                border-color var(--motion-duration-fast) var(--motion-easing-standard),
                transform var(--motion-duration-instant) var(--motion-easing-standard);
  }

  button.ref:hover {
    background: var(--color-accent-soft);
    border-color: var(--color-accent-ring);
  }

  button.ref:active {
    transform: scale(0.96);
  }

  .degraded-banner {
    display: flex;
    align-items: center;
    gap: var(--spacing-2);
    padding: var(--spacing-2) var(--spacing-3);
    margin-bottom: var(--spacing-3);
    border-radius: var(--rounded-sm);
    background: var(--color-sev-warning-bg);
    border: var(--border-width) solid var(--color-sev-warning-border);
    color: var(--color-sev-warning-fg);
    font-size: var(--fs-caption);
    font-weight: var(--fw-medium);
    line-height: 1.4;
  }

  .degraded-banner svg {
    flex-shrink: 0;
    color: var(--color-sev-warning);
  }

  .degraded-banner code {
    font-family: var(--font-mono);
    font-variant-ligatures: none;
    font-weight: var(--fw-semibold);
    padding: 0 4px;
    border: 1px solid var(--color-ink);
    border-radius: var(--rounded-xs);
    background: var(--color-stone-abyss);
    color: var(--color-gold-text);
  }

  @media (prefers-reduced-motion: reduce) {
    *, *::before, *::after, :host {
      animation-duration: 0.01ms !important;
      animation-iteration-count: 1 !important;
      animation-delay: 0ms !important;
    }
  }

  @media (forced-colors: active) {
    :host { filter: none; }
    :host::before {
      clip-path: none;
      background: Canvas;
      box-shadow: none;
      border: 1px solid CanvasText;
    }
  }

  * {
    scrollbar-width: thin;
    scrollbar-color: var(--scrollbar-thumb) transparent;
  }

  *::-webkit-scrollbar {
    width: var(--scrollbar-size);
    height: var(--scrollbar-size);
  }

  *::-webkit-scrollbar-track {
    background: transparent;
  }

  *::-webkit-scrollbar-thumb {
    background-color: var(--scrollbar-thumb);
    border-radius: var(--rounded-full);
    border: 2px solid transparent;
    background-clip: content-box;
  }

  *::-webkit-scrollbar-thumb:hover {
    background-color: var(--scrollbar-thumb-hover);
  }
`);

export function clearNode(node) {
  while (node.firstChild) {
    node.removeChild(node.firstChild);
  }
}

export function el(tag, attrs = null, ...children) {
  const node = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v === null || v === undefined || v === false) continue;
      if (k === 'className') {
        node.className = v;
      } else if (k === 'dataset' && typeof v === 'object') {
        for (const [dk, dv] of Object.entries(v)) {
          if (dv !== undefined && dv !== null) node.dataset[dk] = String(dv);
        }
      } else if (v === true) {
        node.setAttribute(k, '');
      } else {
        node.setAttribute(k, String(v));
      }
    }
  }
  appendChildren(node, children);
  return node;
}

export function svgEl(tag, attrs = null, ...children) {
  const node = document.createElementNS(SVG_NS, tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v === null || v === undefined || v === false) continue;
      node.setAttribute(k, String(v));
    }
  }
  appendChildren(node, children);
  return node;
}

function appendChildren(parent, children) {
  for (const child of children) {
    if (child === null || child === undefined || child === false) continue;
    if (Array.isArray(child)) {
      appendChildren(parent, child);
    } else if (child instanceof Node) {
      parent.appendChild(child);
    } else {
      parent.appendChild(document.createTextNode(String(child)));
    }
  }
}

// Iconografía vectorial Reliquia (stroke-width 2px, puntas redondas)
export function createSeverityIcon(sev, size = 24) {
  const s = SEV_SET.has(sev) ? sev : 'info';
  const root = svgEl('svg', {
    width: size,
    height: size,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    'stroke-width': '2',
    'stroke-linecap': 'round',
    'stroke-linejoin': 'round',
    'aria-hidden': 'true'
  });

  if (s === 'success') {
    root.appendChild(svgEl('circle', { cx: '12', cy: '12', r: '9' }));
    root.appendChild(svgEl('path', { d: 'M8.5 12.5l2.3 2.3 4.7-5.1' }));
  } else if (s === 'warning') {
    root.appendChild(svgEl('path', { d: 'M10.3 3.9L2.6 17.2a2 2 0 0 0 1.7 3h15.4a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z' }));
    root.appendChild(svgEl('line', { x1: '12', y1: '9', x2: '12', y2: '13' }));
    root.appendChild(svgEl('circle', { cx: '12', cy: '16.5', r: '0.6', fill: 'currentColor' }));
  } else if (s === 'error') {
    root.appendChild(svgEl('circle', { cx: '12', cy: '12', r: '9' }));
    root.appendChild(svgEl('line', { x1: '12', y1: '8', x2: '12', y2: '12.5' }));
    root.appendChild(svgEl('circle', { cx: '12', cy: '16', r: '0.6', fill: 'currentColor' }));
  } else {
    root.appendChild(svgEl('circle', { cx: '12', cy: '12', r: '9' }));
    root.appendChild(svgEl('line', { x1: '12', y1: '11', x2: '12', y2: '16' }));
    root.appendChild(svgEl('circle', { cx: '12', cy: '8', r: '0.6', fill: 'currentColor' }));
  }
  return root;
}

export function createUiIcon(name, size = 16) {
  const root = svgEl('svg', {
    width: size,
    height: size,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    'stroke-width': '2',
    'stroke-linecap': 'round',
    'stroke-linejoin': 'round',
    'aria-hidden': 'true'
  });

  switch (name) {
    case 'chevron':
      root.appendChild(svgEl('polyline', { points: '9 6 15 12 9 18' }));
      break;
    case 'search':
      root.appendChild(svgEl('circle', { cx: '11', cy: '11', r: '7' }));
      root.appendChild(svgEl('line', { x1: '20', y1: '20', x2: '16.2', y2: '16.2' }));
      break;
    case 'sun':
      root.appendChild(svgEl('circle', { cx: '12', cy: '12', r: '4' }));
      root.appendChild(svgEl('path', { d: 'M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41' }));
      break;
    case 'moon':
      root.appendChild(svgEl('path', { d: 'M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z' }));
      break;
    case 'shield':
      root.appendChild(svgEl('path', { d: 'M12 3l7 3v6c0 4.5-3 7.9-7 9-4-1.1-7-4.5-7-9V6l7-3z' }));
      break;
    case 'eye':
      root.appendChild(svgEl('path', { d: 'M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7-10-7-10-7z' }));
      root.appendChild(svgEl('circle', { cx: '12', cy: '12', r: '3' }));
      break;
    case 'trend-up':
      root.appendChild(svgEl('line', { x1: '12', y1: '19', x2: '12', y2: '5' }));
      root.appendChild(svgEl('polyline', { points: '6 11 12 5 18 11' }));
      break;
    case 'trend-down':
      root.appendChild(svgEl('line', { x1: '12', y1: '5', x2: '12', y2: '19' }));
      root.appendChild(svgEl('polyline', { points: '6 13 12 19 18 13' }));
      break;
    case 'minus':
      root.appendChild(svgEl('line', { x1: '5', y1: '12', x2: '19', y2: '12' }));
      break;
    case 'plus':
      root.appendChild(svgEl('line', { x1: '12', y1: '5', x2: '12', y2: '19' }));
      root.appendChild(svgEl('line', { x1: '5', y1: '12', x2: '19', y2: '12' }));
      break;
    case 'maximize':
      root.appendChild(svgEl('path', { d: 'M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5' }));
      break;
    case 'minimize':
      root.appendChild(svgEl('path', { d: 'M9 4v5H4M15 4v5h5M9 20v-5H4M15 20v-5h5' }));
      break;
    case 'fit':
      root.appendChild(svgEl('circle', { cx: '12', cy: '12', r: '6' }));
      root.appendChild(svgEl('path', { d: 'M12 2v4M12 18v4M2 12h4M18 12h4' }));
      break;
    case 'history':
      root.appendChild(svgEl('path', { d: 'M3 12a9 9 0 1 0 3-6.7L3 8' }));
      root.appendChild(svgEl('polyline', { points: '3 3 3 8 8 8' }));
      root.appendChild(svgEl('polyline', { points: '12 7 12 12 15.5 14' }));
      break;
    case 'close':
      root.appendChild(svgEl('line', { x1: '6', y1: '6', x2: '18', y2: '18' }));
      root.appendChild(svgEl('line', { x1: '18', y1: '6', x2: '6', y2: '18' }));
      break;
    case 'layers':
      root.appendChild(svgEl('polygon', { points: '12 2 2 7 12 12 22 7 12 2' }));
      root.appendChild(svgEl('polyline', { points: '2 12 12 17 22 12' }));
      root.appendChild(svgEl('polyline', { points: '2 17 12 22 22 17' }));
      break;
    default:
      root.appendChild(svgEl('circle', { cx: '12', cy: '12', r: '8' }));
  }
  return root;
}

// Parseo y creación de referencias archivo:línea (§4.0)
const REF_EXACT_RE = /^([A-Za-z0-9_./-]+\.[A-Za-z0-9_]+):(\d+)(?::(\d+))?$/;

export function parseRefValue(raw) {
  if (typeof raw !== 'string') return null;
  const trimmed = raw.trim();
  const m = REF_EXACT_RE.exec(trimmed);
  if (!m) {
    if (!trimmed) return null;
    return { label: trimmed, path: trimmed, line: 1, column: null };
  }
  const column = m[3] ? Number.parseInt(m[3], 10) : null;
  return {
    label: trimmed,
    path: m[1],
    line: Number.parseInt(m[2], 10),
    column: Number.isFinite(column) && column > 0 ? column : null
  };
}

export function createRefButton(raw, partName = 'ref') {
  const parsed = parseRefValue(raw);
  if (!parsed) return null;
  const btn = el('button', {
    type: 'button',
    className: 'ref',
    part: partName,
    // data-tip (no title): lo muestra <dx-tooltip> sin el tooltip nativo (R5).
    'data-tip': `Abrir ${parsed.path} en línea ${parsed.line}${parsed.column ? `:${parsed.column}` : ''}`
  }, parsed.label);

  btn.addEventListener('click', () => {
    btn.dispatchEvent(new CustomEvent('dx-ref', {
      bubbles: true,
      composed: true,
      detail: { path: parsed.path, line: parsed.line, column: parsed.column ?? null }
    }));
  });
  return btn;
}

// Insignia visual obligatoria de bloque degradado (§2 ADR 1)
export function createDegradedBanner(degraded) {
  if (!degraded || typeof degraded !== 'object') return null;
  const fromType = typeof degraded.from === 'string' && degraded.from ? degraded.from : 'unknown';
  const reason = typeof degraded.reason === 'string' && degraded.reason ? degraded.reason : 'estructura inválida';
  return el(
    'div',
    { className: 'degraded-banner', part: 'degraded', role: 'status' },
    createSeverityIcon('warning', 16),
    el('span', null,
      'Degradado desde ',
      el('code', null, fromType),
      `: ${reason}`
    )
  );
}

// ==========================================================================
// PARSER SEGURO DE MARKDOWN (§4.8 y §5 Contrato B) — sin innerHTML.
// Inline: **negrita**, *itálica*/_itálica_, ~~tachado~~, `código`,
//         [texto](https://…) enlace externo, [texto](ruta) enlace relativo,
//         ![alt](ruta) imagen, refs path:line.
// Bloques: párrafos, listas ordenadas/viñetas anidadas por sangría, citas (>),
//          bloques ``` de código, encabezados (#…) y separadores (---).
// El HTML crudo se muestra como texto literal.
// ==========================================================================
const INLINE_TOKEN_RE = /(!\[[^\]\n]*\]\([^\s)]+\)|\*\*[^*\n]+\*\*|~~[^~\n]+~~|`[^`\n]+`|\[[^\]\n]+\]\([^\s)]+\)|(?<![\w*])\*(?!\s)[^*\n]+?(?<!\s)\*(?![\w*])|(?<![A-Za-z0-9_])_(?!\s)[^_\n]+?(?<!\s)_(?![A-Za-z0-9_])|(?<![A-Za-z0-9_/:.-])[A-Za-z0-9_./-]+\.[A-Za-z0-9_]+:\d+(?::\d+)?)/g;

function renderInlineToken(token) {
  if (token.startsWith('![')) {
    const close = token.indexOf('](');
    const alt = token.slice(2, close) || 'imagen';
    const src = token.slice(close + 2, -1);
    // Solo data: o mismo origen pasan la CSP; las rutas del repo se muestran como referencia.
    if (/^data:image\//i.test(src) || /^\/(?!\/)/.test(src)) {
      return el('img', { src, alt, loading: 'lazy', className: 'md-img' });
    }
    return el('span', { className: 'md-img-ref', title: src, role: 'img', 'aria-label': `Imagen: ${alt}` },
      el('span', { className: 'md-img-ref__alt' }, alt),
      el('code', null, src));
  }
  if (token.startsWith('**')) return el('strong', null, renderInlineMarkdown(token.slice(2, -2)));
  if (token.startsWith('~~')) return el('del', null, renderInlineMarkdown(token.slice(2, -2)));
  if (token.startsWith('`')) return el('code', null, token.slice(1, -1));
  if (token.startsWith('[')) {
    const close = token.indexOf('](');
    const label = token.slice(1, close);
    const url = token.slice(close + 2, -1);
    if (/^https?:\/\//i.test(url)) {
      return el('a', { href: url, target: '_blank', rel: 'noopener noreferrer' }, renderInlineMarkdown(label));
    }
    if (/^#[\w-]+$/.test(url)) {
      return el('a', { href: url }, renderInlineMarkdown(label));
    }
    // Ruta relativa del repositorio: no navegable desde el visor; se muestra con su destino.
    return el('span', { className: 'md-link-rel', title: url },
      renderInlineMarkdown(label), ' ', el('code', { className: 'md-link-rel__path' }, url));
  }
  if (token.startsWith('*') || token.startsWith('_')) return el('em', null, renderInlineMarkdown(token.slice(1, -1)));
  return createRefButton(token) || document.createTextNode(token);
}

export function renderInlineMarkdown(text) {
  const frag = document.createDocumentFragment();
  if (typeof text !== 'string' || !text) return frag;

  const re = new RegExp(INLINE_TOKEN_RE.source, 'g');
  let lastIdx = 0;
  let match;
  while ((match = re.exec(text)) !== null) {
    if (match.index > lastIdx) {
      frag.appendChild(document.createTextNode(text.slice(lastIdx, match.index)));
    }
    frag.appendChild(renderInlineToken(match[0]));
    lastIdx = match.index + match[0].length;
  }
  if (lastIdx < text.length) {
    frag.appendChild(document.createTextNode(text.slice(lastIdx)));
  }
  return frag;
}

const LIST_ITEM_RE = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/;
const FENCE_RE = /^\s*(```|~~~)\s*([\w+-]*)\s*$/;
const HEADING_RE = /^(#{1,6})\s+(.*)$/;
const HR_RE = /^\s*([-*_])(\s*\1){2,}\s*$/;
const TASK_PREFIX_RE = /^\[([ xX])\]\s+/;

// Construye listas anidadas a partir de líneas con sangría (2+ espacios por nivel).
function buildNestedList(items) {
  const root = { children: [] };
  const stack = [{ indent: -1, node: root, list: null }];
  for (const it of items) {
    while (stack.length > 1 && it.indent <= stack[stack.length - 1].indent) stack.pop();
    const parent = stack[stack.length - 1];
    const ordered = /\d/.test(it.marker);
    const holder = parent.node;
    let list = holder.children[holder.children.length - 1];
    if (!list || list.kind !== 'list' || list.ordered !== ordered || list.indent !== it.indent) {
      list = { kind: 'list', ordered, indent: it.indent, start: ordered ? parseInt(it.marker, 10) : 1, items: [] };
      holder.children.push(list);
    }
    const node = { text: it.text, continuation: [], children: [] };
    list.items.push(node);
    stack.push({ indent: it.indent, node, list });
  }
  return root.children;
}

function renderListTree(lists) {
  const frag = document.createDocumentFragment();
  for (const list of lists) {
    const listEl = el(list.ordered ? 'ol' : 'ul');
    if (list.ordered && list.start !== 1) listEl.setAttribute('start', String(list.start));
    for (const item of list.items) {
      const li = el('li');
      const task = TASK_PREFIX_RE.exec(item.text);
      if (task) {
        li.classList.add('md-task');
        li.appendChild(el('span', { className: `md-task__box${task[1].trim() ? ' is-done' : ''}`, 'aria-hidden': 'true' }));
        li.appendChild(renderInlineMarkdown(item.text.replace(TASK_PREFIX_RE, '')));
      } else {
        li.appendChild(renderInlineMarkdown(item.text));
      }
      for (const extra of item.continuation) {
        li.appendChild(el('br'));
        li.appendChild(renderInlineMarkdown(extra));
      }
      if (item.children.length) li.appendChild(renderListTree(item.children));
      listEl.appendChild(li);
    }
    frag.appendChild(listEl);
  }
  return frag;
}

export function renderMarkdownBlocks(md, { allowLists = true } = {}) {
  const frag = document.createDocumentFragment();
  if (typeof md !== 'string' || !md.trim()) return frag;

  const lines = md.replace(/\r\n/g, '\n').split('\n');
  let paragraphBuf = [];
  let listBuf = [];
  let quoteBuf = [];

  const flushParagraph = () => {
    if (paragraphBuf.length) {
      const p = el('p');
      p.appendChild(renderInlineMarkdown(paragraphBuf.join(' ')));
      frag.appendChild(p);
      paragraphBuf = [];
    }
  };
  const flushList = () => {
    if (listBuf.length) {
      frag.appendChild(renderListTree(buildNestedList(listBuf)));
      listBuf = [];
    }
  };
  const flushQuote = () => {
    if (quoteBuf.length) {
      const bq = el('blockquote');
      bq.appendChild(renderMarkdownBlocks(quoteBuf.join('\n'), { allowLists }));
      frag.appendChild(bq);
      quoteBuf = [];
    }
  };
  const flushAll = () => { flushParagraph(); flushList(); flushQuote(); };

  for (let i = 0; i < lines.length; i++) {
    const rawLine = lines[i];
    const trimmed = rawLine.trim();

    // Cita: acumula líneas "> " y se renderiza recursivamente.
    if (/^>/.test(trimmed)) {
      flushParagraph(); flushList();
      quoteBuf.push(trimmed.replace(/^>\s?/, ''));
      continue;
    }
    if (quoteBuf.length) flushQuote();

    const fence = FENCE_RE.exec(rawLine);
    if (fence) {
      flushAll();
      const body = [];
      i++;
      while (i < lines.length && !new RegExp(`^\\s*${fence[1]}\\s*$`).test(lines[i])) {
        body.push(lines[i]);
        i++;
      }
      const pre = el('pre', { className: 'md-pre' }, el('code', { 'data-lang': fence[2] || 'text' }, body.join('\n')));
      frag.appendChild(pre);
      continue;
    }

    if (!trimmed) {
      flushParagraph(); flushList();
      continue;
    }

    const heading = HEADING_RE.exec(trimmed);
    if (heading) {
      flushAll();
      const level = Math.min(6, Math.max(3, heading[1].length));
      frag.appendChild(el(`h${level}`, null, renderInlineMarkdown(heading[2])));
      continue;
    }

    if (HR_RE.test(trimmed) && !listBuf.length) {
      flushAll();
      frag.appendChild(el('hr'));
      continue;
    }

    const li = allowLists ? LIST_ITEM_RE.exec(rawLine) : null;
    if (li) {
      flushParagraph();
      listBuf.push({ indent: li[1].replace(/\t/g, '  ').length, marker: li[2], text: li[3] });
      continue;
    }
    // Línea con sangría tras un ítem: continuación del ítem anterior.
    if (listBuf.length && /^\s{2,}\S/.test(rawLine)) {
      listBuf[listBuf.length - 1].text += ' ' + trimmed;
      continue;
    }

    flushList();
    paragraphBuf.push(trimmed);
  }

  flushAll();
  return frag;
}

// Estilos de los elementos que produce renderMarkdownBlocks (prosa y callouts).
export const markdownSheet = createSheet(`
  ol {
    margin: 0 0 var(--spacing-4) 0;
    padding-left: var(--spacing-6);
    display: flex;
    flex-direction: column;
    gap: var(--spacing-2);
  }
  ol > li { padding-left: var(--spacing-1); }
  ol > li::marker {
    color: var(--color-copper);
    font-family: var(--font-mono);
    font-size: var(--fs-small, 0.875em);
    font-variant-numeric: tabular-nums;
  }
  li > ul, li > ol { margin: var(--spacing-2) 0 0 0; }
  h3, h4, h5, h6 {
    margin: var(--spacing-5) 0 var(--spacing-2) 0;
    font-family: var(--font-display, Georgia, serif);
    color: var(--color-text-primary);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-heading);
    letter-spacing: .02em;
    text-wrap: balance;
  }
  h3 { font-size: var(--fs-body-lg, 1.125rem); }
  h4, h5, h6 { font-size: var(--fs-body); color: var(--color-text-secondary); }
  :is(h3, h4, h5, h6):first-child { margin-top: 0; }
  em { font-style: italic; }
  del { color: var(--color-text-soft); text-decoration-thickness: 1px; }
  hr {
    border: 0;
    height: 1px;
    margin: var(--spacing-6) 0;
    background: linear-gradient(90deg, transparent, var(--color-bronze), transparent);
  }
  pre.md-pre {
    margin: 0 0 var(--spacing-4) 0;
    padding: var(--spacing-3) var(--spacing-4);
    overflow-x: auto;
    background: var(--color-stone-abyss);
    border: 1px solid var(--color-ink);
    border-radius: var(--rounded-xs, 2px);
    font-family: var(--font-mono);
    font-size: var(--fs-small, 0.875em);
    line-height: 1.55;
    color: var(--color-text-primary);
  }
  pre.md-pre code { background: none; border: 0; padding: 0; color: inherit; font-size: inherit; }
  blockquote > :last-child { margin-bottom: 0; }
  .md-img { max-width: 100%; height: auto; border: 1px solid var(--color-ink); }
  .md-img-ref, .md-link-rel { display: inline-flex; align-items: baseline; gap: .4em; flex-wrap: wrap; }
  .md-img-ref__alt { font-style: italic; color: var(--color-text-secondary); }
  .md-img-ref__alt::before { content: "▣ "; color: var(--color-copper); font-style: normal; }
  .md-link-rel { text-decoration: underline dotted var(--color-accent-ring); text-underline-offset: 3px; }
  .md-link-rel__path { font-size: .82em; opacity: .85; }
  li.md-task { list-style: none; }
  li.md-task::before { display: none !important; }
  .md-task__box {
    display: inline-block;
    width: .85em; height: .85em;
    margin-inline-end: .5em;
    vertical-align: -.08em;
    border: 1.5px solid var(--color-bronze);
    border-radius: 2px;
  }
  .md-task__box.is-done { background: var(--color-copper); border-color: var(--color-copper); }
  @media (forced-colors: active) {
    hr { background: CanvasText; }
    .md-task__box { border-color: CanvasText; }
    .md-task__box.is-done { background: CanvasText; }
  }
`);

// Resaltado seguro de coincidencias de búsqueda con <mark>
export function renderHighlightedText(text, query, markClass = '') {
  const str = String(text ?? '');
  const frag = document.createDocumentFragment();
  if (!query) {
    frag.appendChild(document.createTextNode(str));
    return frag;
  }
  const qLower = query.toLowerCase();
  const sLower = str.toLowerCase();
  let start = 0;
  let idx;
  while ((idx = sLower.indexOf(qLower, start)) !== -1) {
    if (idx > start) {
      frag.appendChild(document.createTextNode(str.slice(start, idx)));
    }
    const mark = el('mark', markClass ? { className: markClass } : null, str.slice(idx, idx + query.length));
    frag.appendChild(mark);
    start = idx + query.length;
  }
  if (start < str.length) {
    frag.appendChild(document.createTextNode(str.slice(start)));
  }
  return frag;
}

// ==========================================================================
// DESPACHADOR CON TOLERANCIA CONTROLADA (§2 ADR 1)
// Ignora claves desconocidas, repara excesos recuperables y degrada fallos
// estructurales a `prose` o `code` con insignia visible `degraded`.
// ==========================================================================
const ALLOWED_ENVELOPE_KEYS = new Set(['v', 'report', 'warnings']);
const ALLOWED_REPORT_KEYS = new Set(['id', 'project', 'title', 'ts', 'updatedAt', 'revision', 'lastChange', 'blocks', 'maxSev', 'excerpt']);

const ALLOWED_BLOCK_KEYS = Object.freeze({
  kpi: new Set(['t', 'items']),
  callout: new Set(['t', 'sev', 'title', 'body', 'ref']),
  table: new Set(['t', 'cols', 'rows', 'sort', 'filter']),
  section: new Set(['t', 'title', 'sev', 'open', 'blocks']),
  code: new Set(['t', 'lang', 'src', 'old', 'ref', 'hl', 'tokens', 'hunks', 'degraded']),
  diagram: new Set(['t', 'kind', 'nodes', 'edges', 'svg', 'src', 'direction']),
  checklist: new Set(['t', 'items', 'readonly']),
  prose: new Set(['t', 'title', 'md', 'degraded']),
  canvas: new Set(['t', 'html', 'src', 'h'])
});

export function normalizeReportEnvelope(input) {
  const warnings = [];

  if (!input || typeof input !== 'object' || Array.isArray(input)) {
    return { ok: false, error: 'El sobre raíz debe ser un objeto JSON válido (ruta: root).' };
  }
  if (input.v === undefined || input.v === null) {
    return { ok: false, error: 'Falta campo obligatorio "v" en el sobre raíz (ruta: v).' };
  }
  const rep = input.report;
  if (!rep || typeof rep !== 'object' || Array.isArray(rep)) {
    return { ok: false, error: 'Falta objeto "report" en el sobre raíz (ruta: report).' };
  }
  if (typeof rep.title !== 'string' || !rep.title.trim()) {
    return { ok: false, error: 'Falta "report.title" válido (ruta: report.title).' };
  }
  if (!Array.isArray(rep.blocks)) {
    return { ok: false, error: 'Falta arreglo "report.blocks[]" (ruta: report.blocks).' };
  }

  if (Array.isArray(input.warnings)) {
    for (const w of input.warnings) {
      if (w && typeof w === 'object' && typeof w.path === 'string' && typeof w.reason === 'string') {
        warnings.push({
          path: w.path,
          action: typeof w.action === 'string' ? w.action : 'repaired',
          reason: w.reason
        });
      }
    }
  }

  for (const k of Object.keys(input)) {
    if (!ALLOWED_ENVELOPE_KEYS.has(k)) {
      warnings.push({ path: k, action: 'ignored', reason: `clave desconocida "${k}" ignorada` });
    }
  }
  for (const k of Object.keys(rep)) {
    if (!ALLOWED_REPORT_KEYS.has(k)) {
      warnings.push({ path: `report.${k}`, action: 'ignored', reason: `clave desconocida "${k}" ignorada` });
    }
  }

  const normalizedBlocks = [];
  rep.blocks.forEach((rawBlock, idx) => {
    const res = normalizeBlock(rawBlock, `blocks[${idx}]`, 1, warnings);
    if (Array.isArray(res)) {
      normalizedBlocks.push(...res);
    } else if (res) {
      normalizedBlocks.push(res);
    }
  });

  return {
    ok: true,
    warnings,
    report: {
      id: typeof rep.id === 'string' ? rep.id : 'r_local',
      project: typeof rep.project === 'string' ? rep.project : 'dioptra-core',
      title: rep.title.trim(),
      ts: typeof rep.ts === 'string' ? rep.ts : new Date().toISOString(),
      updatedAt: typeof rep.updatedAt === 'string' ? rep.updatedAt : '',
      revision: typeof rep.revision === 'number' && Number.isFinite(rep.revision) && rep.revision >= 1 ? Math.round(rep.revision) : 1,
      lastChange: typeof rep.lastChange === 'string' ? rep.lastChange : '',
      blocks: normalizedBlocks
    }
  };
}

function degradeBlock(rawBlock, path, fromType, reason, warnings) {
  warnings.push({ path, action: 'degraded', reason: `${fromType}: ${reason}` });
  const degradedMeta = { from: fromType || 'unknown', reason };

  if (rawBlock && typeof rawBlock === 'object' && !Array.isArray(rawBlock)) {
    if (typeof rawBlock.src === 'string' || typeof rawBlock.html === 'string') {
      const codeSrc = typeof rawBlock.src === 'string' ? rawBlock.src : rawBlock.html;
      const defaultLang = typeof rawBlock.html === 'string' ? 'html' : 'text';
      const lang = typeof rawBlock.lang === 'string' && rawBlock.lang ? rawBlock.lang : defaultLang;
      return {
        t: 'code',
        data: {
          lang: lang,
          src: truncateLines(codeSrc, 2000, `${path}.src`, warnings),
          degraded: degradedMeta
        }
      };
    }

    const proseBody = [rawBlock.body, rawBlock.md, rawBlock.text].find((v) => typeof v === 'string' && v.trim());
    const hasTitle = typeof rawBlock.title === 'string' && rawBlock.title.trim();
    if (proseBody || hasTitle) {
      return {
        t: 'prose',
        data: {
          title: hasTitle ? rawBlock.title.trim() : undefined,
          md: proseBody || rawBlock.title.trim(),
          degraded: degradedMeta
        }
      };
    }
  }

  return {
    t: 'code',
    data: {
      lang: 'json',
      src: truncateLines(safeJsonStringify(rawBlock), 2000, path, warnings),
      degraded: degradedMeta
    }
  };
}

function safeJsonStringify(val) {
  try {
    return JSON.stringify(val, null, 2) ?? String(val);
  } catch {
    return String(val);
  }
}

function truncateLines(str, maxLines, path, warnings) {
  const lines = str.replace(/\r\n/g, '\n').split('\n');
  if (lines.length <= maxLines) return str;
  warnings.push({
    path,
    action: 'repaired',
    reason: `truncado de ${lines.length} a ${maxLines} líneas`
  });
  return `${lines.slice(0, maxLines).join('\n')}\n// … truncado a ${maxLines} líneas por límite de contrato`;
}

function isStrictTree(nodes, edges) {
  if (nodes.length <= 1) return edges.length === 0;
  const ids = new Set(nodes.map((n) => n.id));
  const indegree = new Map(nodes.map((n) => [n.id, 0]));
  const adj = new Map(nodes.map((n) => [n.id, []]));

  for (const [u, v] of edges) {
    if (!ids.has(u) || !ids.has(v) || u === v) return false;
    const deg = (indegree.get(v) || 0) + 1;
    if (deg > 1) return false;
    indegree.set(v, deg);
    adj.get(u).push(v);
  }

  const roots = nodes.filter((n) => indegree.get(n.id) === 0);
  if (roots.length !== 1) return false;

  const visited = new Set();
  const stack = [roots[0].id];
  while (stack.length > 0) {
    const cur = stack.pop();
    if (visited.has(cur)) return false;
    visited.add(cur);
    for (const nxt of adj.get(cur) || []) {
      stack.push(nxt);
    }
  }
  return visited.size === nodes.length;
}

function sanitizeOptionalSev(val, path, warnings, fallback = undefined) {
  if (val === undefined || val === null || val === '') return fallback;
  if (SEV_SET.has(val)) return val;
  warnings.push({
    path,
    action: 'repaired',
    reason: `severidad inválida "${String(val)}"${fallback ? ` reemplazada por "${fallback}"` : ' omitida'}`
  });
  return fallback;
}

export function normalizeBlock(rawBlock, path, depth, warnings) {
  if (!rawBlock || typeof rawBlock !== 'object' || Array.isArray(rawBlock)) {
    return degradeBlock(rawBlock, path, 'invalid', 'el bloque no es un objeto', warnings);
  }

  const t = typeof rawBlock.t === 'string' ? rawBlock.t : '';
  if (!ALLOWED_BLOCK_KEYS[t]) {
    return degradeBlock(rawBlock, path, t || 'unknown', `tipo de bloque desconocido "${t || 'vacío'}"`, warnings);
  }

  const allowed = ALLOWED_BLOCK_KEYS[t];
  for (const k of Object.keys(rawBlock)) {
    if (!allowed.has(k)) {
      warnings.push({ path: `${path}.${k}`, action: 'ignored', reason: `propiedad desconocida "${k}" ignorada` });
    }
  }

  switch (t) {
    case 'kpi': {
      if (!Array.isArray(rawBlock.items) || rawBlock.items.length === 0) {
        return degradeBlock(rawBlock, path, 'kpi', 'falta items[] válido', warnings);
      }
      let items = rawBlock.items;
      if (items.length > 12) {
        warnings.push({ path: `${path}.items`, action: 'repaired', reason: `KPIs truncados de ${items.length} a 12` });
        items = items.slice(0, 12);
      }
      const cleanItems = [];
      for (let i = 0; i < items.length; i++) {
        const it = items[i];
        if (!it || typeof it !== 'object' || typeof it.label !== 'string' || (typeof it.value !== 'string' && typeof it.value !== 'number')) {
          return degradeBlock(rawBlock, path, 'kpi', `items[${i}] carece de label o value`, warnings);
        }
        cleanItems.push({
          label: it.label,
          value: it.value,
          unit: typeof it.unit === 'string' ? it.unit : '',
          delta: typeof it.delta === 'string' ? it.delta : '',
          sev: sanitizeOptionalSev(it.sev, `${path}.items[${i}].sev`, warnings)
        });
      }
      return { t: 'kpi', data: { items: cleanItems } };
    }

    case 'callout': {
      if (typeof rawBlock.title !== 'string' || typeof rawBlock.body !== 'string') {
        return degradeBlock(rawBlock, path, 'callout', 'falta title o body', warnings);
      }
      const sev = sanitizeOptionalSev(rawBlock.sev, `${path}.sev`, warnings, 'info');
      return {
        t: 'callout',
        data: {
          sev,
          title: rawBlock.title,
          body: rawBlock.body,
          ref: typeof rawBlock.ref === 'string' ? rawBlock.ref : undefined
        }
      };
    }

    case 'table': {
      if (!Array.isArray(rawBlock.cols) || rawBlock.cols.length === 0) {
        return degradeBlock(rawBlock, path, 'table', 'falta cols', warnings);
      }
      if (!Array.isArray(rawBlock.rows)) {
        return degradeBlock(rawBlock, path, 'table', 'falta rows[]', warnings);
      }
      const validColTypes = new Set(['text', 'num', 'sev', 'ref']);
      const cleanCols = [];
      for (let i = 0; i < rawBlock.cols.length; i++) {
        const c = rawBlock.cols[i];
        if (!c || typeof c.key !== 'string' || typeof c.label !== 'string') {
          return degradeBlock(rawBlock, path, 'table', `cols[${i}] inválido`, warnings);
        }
        const colType = validColTypes.has(c.type) ? c.type : 'text';
        cleanCols.push({ key: c.key, label: c.label, type: colType });
      }
      return {
        t: 'table',
        data: {
          cols: cleanCols,
          rows: rawBlock.rows.filter((r) => r && typeof r === 'object'),
          sort: typeof rawBlock.sort === 'string' ? rawBlock.sort : undefined,
          filter: Boolean(rawBlock.filter)
        }
      };
    }

    case 'section': {
      if (!Array.isArray(rawBlock.blocks)) {
        return degradeBlock(rawBlock, path, 'section', 'falta blocks[]', warnings);
      }
      let title = typeof rawBlock.title === 'string' && rawBlock.title.trim() ? rawBlock.title.trim() : '';
      if (!title) {
        title = '(sin título)';
        warnings.push({ path: `${path}.title`, action: 'repaired', reason: 'section sin título reparada a "(sin título)"' });
      }
      const sev = sanitizeOptionalSev(rawBlock.sev, `${path}.sev`, warnings);
      const childBlocks = [];
      rawBlock.blocks.forEach((child, idx) => {
        const res = normalizeBlock(child, `${path}.blocks[${idx}]`, depth + 1, warnings);
        if (Array.isArray(res)) childBlocks.push(...res);
        else if (res) childBlocks.push(res);
      });

      if (depth > 3) {
        warnings.push({ path, action: 'repaired', reason: 'anidación > 3 aplanada en el nivel 3' });
        return childBlocks;
      }

      return {
        t: 'section',
        data: {
          title,
          sev,
          open: Boolean(rawBlock.open),
          blocks: childBlocks
        }
      };
    }

    case 'code': {
      if (typeof rawBlock.src !== 'string') {
        return degradeBlock(rawBlock, path, 'code', 'falta src', warnings);
      }
      const lang = typeof rawBlock.lang === 'string' && rawBlock.lang ? rawBlock.lang : 'text';
      const src = truncateLines(rawBlock.src, 2000, `${path}.src`, warnings);
      return {
        t: 'code',
        data: {
          lang,
          src,
          old: typeof rawBlock.old === 'string' ? truncateLines(rawBlock.old, 2000, `${path}.old`, warnings) : undefined,
          ref: typeof rawBlock.ref === 'string' ? rawBlock.ref : undefined,
          hl: Array.isArray(rawBlock.hl) ? rawBlock.hl.filter((n) => Number.isInteger(n)) : [],
          tokens: Array.isArray(rawBlock.tokens) ? rawBlock.tokens : undefined,
          hunks: Array.isArray(rawBlock.hunks) ? rawBlock.hunks : undefined,
          degraded: rawBlock.degraded && typeof rawBlock.degraded === 'object' ? rawBlock.degraded : undefined
        }
      };
    }

    case 'diagram': {
      const kind = rawBlock.kind;
      if (kind === 'mermaid') {
        if (typeof rawBlock.src !== 'string' || !rawBlock.src.trim()) {
          return degradeBlock(rawBlock, path, 'diagram', 'diagrama mermaid sin src', warnings);
        }
        return {
          t: 'diagram',
          data: {
            kind: 'mermaid',
            src: rawBlock.src
          }
        };
      }
      if (kind !== 'flow' && kind !== 'seq' && kind !== 'tree') {
        return degradeBlock(rawBlock, path, 'diagram', `kind inválido "${String(kind)}"`, warnings);
      }
      const nodes = Array.isArray(rawBlock.nodes) ? rawBlock.nodes : [];
      const edges = Array.isArray(rawBlock.edges) ? rawBlock.edges : [];
      const hasSvg = typeof rawBlock.svg === 'string' && rawBlock.svg.trim().length > 0;

      if (nodes.length === 0 && !hasSvg) {
        return degradeBlock(rawBlock, path, 'diagram', 'diagrama sin nodes[]', warnings);
      }
      if (kind === 'seq' && (nodes.length > 12 || edges.length > 40)) {
        return degradeBlock(rawBlock, path, 'diagram', `excede límites seq (${nodes.length}/12 nodos, ${edges.length}/40 mensajes)`, warnings);
      }
      if (kind === 'tree' && nodes.length > 60) {
        return degradeBlock(rawBlock, path, 'diagram', `excede límite tree (${nodes.length}/60 nodos)`, warnings);
      }
      if (kind === 'flow' && nodes.length > 80) {
        return degradeBlock(rawBlock, path, 'diagram', `excede límite flow (${nodes.length}/80 nodos)`, warnings);
      }

      const cleanNodes = [];
      for (let i = 0; i < nodes.length; i++) {
        const n = nodes[i];
        if (!n || typeof n.id !== 'string' || typeof n.label !== 'string') {
          return degradeBlock(rawBlock, path, 'diagram', `nodes[${i}] inválido`, warnings);
        }
        cleanNodes.push({
          id: n.id,
          label: n.label,
          sev: sanitizeOptionalSev(n.sev, `${path}.nodes[${i}].sev`, warnings)
        });
      }

      const cleanEdges = [];
      for (let i = 0; i < edges.length; i++) {
        const e = edges[i];
        if (!Array.isArray(e) || typeof e[0] !== 'string' || typeof e[1] !== 'string') {
          return degradeBlock(rawBlock, path, 'diagram', `edges[${i}] inválido`, warnings);
        }
        cleanEdges.push([e[0], e[1], typeof e[2] === 'string' ? e[2] : '']);
      }

      let effectiveKind = kind;
      if (kind === 'tree' && !isStrictTree(cleanNodes, cleanEdges)) {
        effectiveKind = 'flow';
        warnings.push({
          path: `${path}.kind`,
          action: 'repaired',
          reason: 'estructura no arbórea reclasificada de "tree" a "flow"'
        });
      }

      return {
        t: 'diagram',
        data: {
          kind: effectiveKind,
          nodes: cleanNodes,
          edges: cleanEdges,
          direction: typeof rawBlock.direction === 'string' ? rawBlock.direction : undefined,
          svg: hasSvg ? rawBlock.svg : undefined
        }
      };
    }

    case 'checklist': {
      if (!Array.isArray(rawBlock.items) || rawBlock.items.length === 0) {
        return degradeBlock(rawBlock, path, 'checklist', 'falta items[]', warnings);
      }
      const cleanItems = [];
      for (let i = 0; i < rawBlock.items.length; i++) {
        const it = rawBlock.items[i];
        if (!it || typeof it.id !== 'string' || typeof it.text !== 'string' || typeof it.done !== 'boolean') {
          return degradeBlock(rawBlock, path, 'checklist', `items[${i}] inválido`, warnings);
        }
        cleanItems.push({
          id: it.id,
          text: it.text,
          done: it.done,
          owner: typeof it.owner === 'string' ? it.owner : '',
          sev: sanitizeOptionalSev(it.sev, `${path}.items[${i}].sev`, warnings)
        });
      }
      return {
        t: 'checklist',
        data: {
          items: cleanItems,
          readonly: Boolean(rawBlock.readonly)
        }
      };
    }

    case 'prose': {
      if (typeof rawBlock.md !== 'string') {
        return degradeBlock(rawBlock, path, 'prose', 'falta md', warnings);
      }
      return {
        t: 'prose',
        data: {
          title: typeof rawBlock.title === 'string' ? rawBlock.title : undefined,
          md: rawBlock.md,
          degraded: rawBlock.degraded && typeof rawBlock.degraded === 'object' ? rawBlock.degraded : undefined
        }
      };
    }

    case 'canvas': {
      const hasSrc = typeof rawBlock.src === 'string' && rawBlock.src.trim().length > 0;
      const hasHtml = typeof rawBlock.html === 'string';
      if (!hasSrc && !hasHtml) {
        return degradeBlock(rawBlock, path, 'canvas', 'falta html o src', warnings);
      }
      if (hasHtml && new TextEncoder().encode(rawBlock.html).byteLength > 200 * 1024) {
        return degradeBlock(rawBlock, path, 'canvas', 'html excede el límite de 200 KB', warnings);
      }
      const h = typeof rawBlock.h === 'number' && Number.isFinite(rawBlock.h)
        ? Math.min(1200, Math.max(120, Math.round(rawBlock.h)))
        : 320;
      return {
        t: 'canvas',
        data: {
          src: hasSrc ? rawBlock.src : undefined,
          html: hasHtml ? rawBlock.html : undefined,
          h
        }
      };
    }

    default:
      return degradeBlock(rawBlock, path, t, 'tipo no soportado', warnings);
  }
}
