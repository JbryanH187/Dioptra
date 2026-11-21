// ==========================================================================
// DioptraEngine Viewer — ui/components/dx-evidence.js
// <dx-evidence> (R6): visor modal de evidencia archivo:línea sobre <dialog>
// nativo (top layer, focus trap, Esc). Todo texto vía textContent (CSP).
// ==========================================================================

import { adoptSheets, createSheet, el } from './shared.js';

const FETCH_TIMEOUT_MS = 8000;
const CONTEXT_LINES = 40;
const SKELETON_ROWS = 8;

const ERROR_MESSAGES = Object.freeze({
  root_not_configured: 'El proyecto no tiene raíz registrada. Regístrala con:',
  not_found: 'No se encontró el archivo en la raíz del proyecto.',
  forbidden_path: 'Ruta no permitida (absoluta, «..» o fuera de la raíz).',
  blocked_file: 'Archivo bloqueado por seguridad (secretos o .git).',
  unsupported_file: 'Archivo binario o demasiado grande (> 2 MiB).',
  bad_request: 'Solicitud inválida: falta proyecto, ruta o línea.',
  forbidden_host: 'Origen o host no permitido por el servidor.',
  // Códigos reales emitidos por internal/server/source.go (sourceError).
  root_unavailable: 'La raíz registrada para el proyecto no existe o no es accesible. Actualízala con:',
  roots_unreadable: 'No se pudo leer roots.json en el servidor.',
  file_not_found: 'No se encontró el archivo en la raíz del proyecto.',
  not_a_file: 'La ruta no apunta a un archivo.',
  invalid_path: 'Ruta no permitida (absoluta, «..» o vacía).',
  outside_root: 'La ruta resuelta queda fuera de la raíz del proyecto.',
  blocked_path: 'Archivo bloqueado por seguridad (secretos o .git).',
  binary_file: 'Archivo binario: no se puede mostrar.',
  file_too_large: 'Archivo demasiado grande (> 2 MiB).',
  missing_project: 'El reporte no indica proyecto; no se puede resolver la ruta.',
  invalid_line: 'Línea inválida en la referencia.',
  invalid_context: 'Parámetro de contexto inválido.',
  invalid_host: 'Host no permitido por el servidor.',
  invalid_origin: 'Origen no permitido por el servidor.',
  read_error: 'Error leyendo el archivo en el servidor.',
  network: 'No se pudo contactar al servidor local.',
  timeout: 'El servidor tardó más de 8 s en responder.',
  unknown: 'Error inesperado al cargar la evidencia.'
});

const sheet = createSheet(`
  *, *::before, *::after { box-sizing: border-box; }
  :host {
    display: contents;
    --ev-ink: var(--color-ink, #06080C);
    --ev-edge: var(--color-bronze, #C88A36);
  }

  dialog {
    width: min(960px, 92vw);
    max-width: none;
    max-height: 82vh;
    padding: 0;
    border: 0;
    color: var(--color-text-primary);
    background: transparent;
    box-shadow: var(--shadow-ink-lg, none);
    font: var(--fw-regular, 400) var(--fs-small, 0.875rem)/var(--lh-body, 1.5) var(--font-sans, system-ui, sans-serif);
    overflow: visible;
  }
  dialog[open] {
    display: flex;
    flex-direction: column;
    animation: dx-ev-in var(--motion-duration-fast, 140ms) var(--motion-easing-emphasized, cubic-bezier(.2, 0, 0, 1));
  }
  dialog::backdrop { background: var(--color-scrim, rgba(0, 0, 0, .5)); }
  @keyframes dx-ev-in {
    from { opacity: 0; transform: translateY(var(--motion-distance-sm, 4px)); }
    to { opacity: 1; transform: none; }
  }

  /* Chaflán en envoltorio interior: el clip no recorta la sombra del dialog. */
  .frame {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
    max-height: 82vh;
    overflow: hidden;
    background: var(--color-stone-obsidian, var(--color-surface-elevated, #121A24));
    box-shadow: inset 0 0 0 1px var(--ev-edge), inset 0 0 0 3px var(--ev-ink);
    clip-path: var(--clip-chamfer-sm, var(--clip-chamfer-xs, polygon(4px 0, calc(100% - 4px) 0, 100% 4px, 100% calc(100% - 4px), calc(100% - 4px) 100%, 4px 100%, 0 calc(100% - 4px), 0 4px)));
  }

  .sr-only {
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

  .head {
    display: flex;
    align-items: center;
    gap: var(--spacing-3, 12px);
    padding: var(--spacing-3, 12px) var(--spacing-4, 16px);
    border-bottom: 2px solid var(--color-ink);
    flex: 0 0 auto;
  }
  .title {
    margin: 0;
    flex: 1 1 auto;
    min-width: 0;
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: var(--fs-base, 1rem);
    letter-spacing: .02em;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .title .ln { color: var(--color-accent-fg, currentColor); }
  .actions { display: flex; gap: var(--spacing-2, 8px); flex: 0 0 auto; }

  button {
    font: inherit;
    font-size: var(--fs-xs, .75rem);
    color: var(--color-text-primary);
    background: var(--color-stone-abyss, transparent);
    border: 0;
    box-shadow: inset 0 0 0 2px var(--color-ink);
    padding: var(--spacing-1, 4px) var(--spacing-3, 12px);
    cursor: pointer;
    transition: box-shadow var(--motion-duration-fast, 140ms) var(--motion-easing-emphasized, cubic-bezier(.2, 0, 0, 1));
  }
  button:hover:not(:disabled) { box-shadow: inset 0 0 0 2px var(--color-border-strong, var(--color-ink)); }
  button:focus-visible { outline: 2px solid transparent; box-shadow: inset 0 0 0 2px var(--color-ink), var(--focus-ring); }
  button:disabled { opacity: .5; cursor: not-allowed; }
  button.primary { color: var(--color-accent-fg, inherit); }

  .body {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    background: var(--code-bg, var(--color-stone-abyss));
  }

  ol.code {
    list-style: none;
    margin: 0;
    padding: var(--spacing-2, 8px) 0;
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: var(--fs-xs, .8125rem);
    line-height: 1.6;
  }
  ol.code li {
    display: grid;
    grid-template-columns: 1.25em 5ch minmax(0, 1fr);
    gap: 0 var(--spacing-2, 8px);
    padding: 0 var(--spacing-3, 12px);
    white-space: pre;
  }
  .num {
    font-variant-numeric: tabular-nums;
    text-align: right;
    color: var(--color-text-secondary);
    opacity: .7;
    user-select: none;
  }
  .mark { color: var(--color-accent-fg, currentColor); user-select: none; }
  li.hl { background: var(--code-line-hl-bg); }
  li.hl .num { opacity: 1; color: var(--color-text-primary); }
  li.hl .mark::before { content: '\\25C6'; }

  li.skel span.bar {
    display: block;
    height: .8em;
    margin: .4em 0;
    background: var(--color-border-translucent, rgba(127,127,127,.15));
    animation: dx-ev-pulse 1.1s ease-in-out infinite alternate;
  }
  li.skel:nth-child(odd) span.bar { width: 70%; }
  li.skel:nth-child(even) span.bar { width: 45%; }
  @keyframes dx-ev-pulse { from { opacity: .45; } to { opacity: 1; } }

  .err {
    padding: var(--spacing-5, 20px) var(--spacing-4, 16px);
    display: grid;
    gap: var(--spacing-3, 12px);
  }
  .err p { margin: 0; }
  .err .code-label { color: var(--color-text-secondary); font-size: var(--fs-xs, .75rem); }
  .cmd {
    display: flex;
    gap: var(--spacing-2, 8px);
    align-items: center;
  }
  .cmd code {
    flex: 1 1 auto;
    min-width: 0;
    overflow-x: auto;
    white-space: pre;
    font-family: var(--font-mono, monospace);
    padding: var(--spacing-1, 4px) var(--spacing-3, 12px);
    background: var(--color-stone-abyss);
    box-shadow: inset 0 0 0 2px var(--color-ink);
  }

  @media (prefers-reduced-motion: reduce) {
    dialog[open], li.skel span.bar { animation: none; }
    button { transition: none; }
  }
  @media (forced-colors: active) {
    dialog { border: 2px solid CanvasText; box-shadow: none; }
    .frame { box-shadow: none; clip-path: none; }
    .head { border-bottom-color: CanvasText; }
    button, .cmd code { border: 1px solid CanvasText; box-shadow: none; }
    button:disabled { color: GrayText; border-color: GrayText; }
    li.hl { outline: 1px solid CanvasText; forced-color-adjust: none; background: Highlight; color: HighlightText; }
    li.skel span.bar { background: CanvasText; }
  }
`);

function toVscodeHref(absPath, line, column) {
  let p = String(absPath).replace(/\\/g, '/');
  if (!p.startsWith('/')) p = '/' + p;
  const encoded = p.split('/').map((seg) => encodeURIComponent(seg).replace(/%3A/gi, ':')).join('/');
  return `vscode://file${encoded}:${line}${column ? `:${column}` : ''}`;
}

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

// Regla de tema a nivel documento (el selector [data-theme] vive fuera del shadow root).
const DOC_CSS = '[data-theme="light"] dx-evidence { --ev-edge: transparent; }';
let docSheet = null;
let docSheetAdopted = false;

function adoptDocSheetOnce() {
  if (docSheetAdopted) return;
  try {
    if (typeof CSSStyleSheet === 'undefined' || !('adoptedStyleSheets' in document)) return;
    if (!docSheet) {
      docSheet = new CSSStyleSheet();
      docSheet.replaceSync(DOC_CSS);
    }
    document.adoptedStyleSheets = [...document.adoptedStyleSheets, docSheet];
    docSheetAdopted = true;
  } catch (_) {
    /* sin constructable stylesheets: se mantiene el contorno de bronce */
  }
}

export class DxEvidence extends HTMLElement {
  constructor() {
    super();
    adoptDocSheetOnce();
    this._shadow = this.attachShadow({ mode: 'open' });
    adoptSheets(this._shadow, sheet);
    this._ctrl = null;
    this._timer = null;
    this._origin = null;
    this._req = null;
    this._absPath = '';
    this._seq = 0;

    this._titlePath = el('span', { className: 'path' });
    this._titleLine = el('span', { className: 'ln' });
    this._title = el('h2', { className: 'title', id: 'dx-ev-title' }, this._titlePath, this._titleLine);

    this._vscodeBtn = el('button', { type: 'button', className: 'primary', disabled: true }, 'Abrir en VS Code');
    this._copyBtn = el('button', { type: 'button' }, 'Copiar ruta');
    this._closeBtn = el('button', { type: 'button', 'aria-label': 'Cerrar visor de evidencia' }, 'Cerrar');
    const actions = el('div', { className: 'actions' }, this._vscodeBtn, this._copyBtn, this._closeBtn);

    this._body = el('div', { className: 'body', tabindex: '0' });
    this._status = el('p', { className: 'sr-only', role: 'status' });
    this._copyResetTimer = null;
    this._dialog = el('dialog', { 'aria-labelledby': 'dx-ev-title' },
      el('div', { className: 'frame' },
        el('header', { className: 'head' }, this._title, actions),
        this._body
      ),
      this._status
    );
    this._shadow.appendChild(this._dialog);

    this._vscodeBtn.addEventListener('click', () => this._openVscode());
    this._copyBtn.addEventListener('click', () => this._copyPath());
    this._closeBtn.addEventListener('click', () => this.close());
    this._dialog.addEventListener('click', (e) => {
      if (e.target === this._dialog) this.close();
    });
    this._dialog.addEventListener('close', () => this._onClosed());
  }

  open({ project, path, line, column, origin } = {}) {
    const safeLine = Number.isFinite(Number(line)) && Number(line) > 0 ? Math.floor(Number(line)) : 1;
    const safeCol = Number.isFinite(Number(column)) && Number(column) > 0 ? Math.floor(Number(column)) : null;
    this._abort();
    this._origin = origin instanceof Element ? origin : null;
    this._req = { project: String(project || ''), path: String(path || ''), line: safeLine, column: safeCol };
    this._absPath = '';
    this._vscodeBtn.disabled = true;
    this._titlePath.textContent = this._req.path || '(sin ruta)';
    this._titleLine.textContent = `:${safeLine}${safeCol ? `:${safeCol}` : ''}`;
    this._title.setAttribute('data-tip', this._titlePath.textContent);

    if (!this._dialog.open) {
      try { this._dialog.showModal(); } catch { this._dialog.setAttribute('open', ''); }
    }
    this._renderSkeleton();
    this._load();
  }

  close() {
    if (this._dialog.open) this._dialog.close();
    else this._onClosed();
  }

  disconnectedCallback() {
    this._abort();
  }

  _abort() {
    if (this._timer) { clearTimeout(this._timer); this._timer = null; }
    if (this._ctrl) { this._ctrl.abort(); this._ctrl = null; }
  }

  _onClosed() {
    this._abort();
    this._seq++;
    const origin = this._origin;
    this._origin = null;
    if (origin && origin.isConnected && typeof origin.focus === 'function') {
      try { origin.focus({ preventScroll: true }); } catch { /* sin foco */ }
    }
  }

  async _load() {
    const req = this._req;
    if (!req.project || !req.path) {
      this._renderError({ error: 'bad_request' });
      return;
    }
    const seq = ++this._seq;
    const ctrl = new AbortController();
    this._ctrl = ctrl;
    let timedOut = false;
    this._timer = setTimeout(() => { timedOut = true; ctrl.abort(); }, FETCH_TIMEOUT_MS);

    const qs = new URLSearchParams({
      project: req.project,
      path: req.path,
      line: String(req.line),
      context: String(CONTEXT_LINES)
    });

    try {
      const res = await fetch('/api/source?' + qs.toString(), {
        signal: ctrl.signal,
        headers: { Accept: 'application/json' },
        cache: 'no-store'
      });
      let data = null;
      try { data = await res.json(); } catch { data = null; }
      if (seq !== this._seq) return;
      if (!res.ok || !data || !Array.isArray(data.lines)) {
        const err = data && typeof data.error === 'string' ? data : { error: 'unknown' };
        if (!err.message) err.message = `HTTP ${res.status}`;
        this._renderError(err);
        return;
      }
      this._renderCode(data);
    } catch (e) {
      if (seq !== this._seq) return;
      if (timedOut) this._renderError({ error: 'timeout' });
      else if (e && e.name === 'AbortError') return;
      else this._renderError({ error: 'network', message: e?.message || '' });
    } finally {
      if (this._ctrl === ctrl) {
        clearTimeout(this._timer);
        this._timer = null;
        this._ctrl = null;
      }
    }
  }

  _renderSkeleton() {
    this._body.replaceChildren();
    this._body.setAttribute('aria-busy', 'true');
    const ol = el('ol', { className: 'code', 'aria-label': 'Cargando evidencia' });
    for (let i = 0; i < SKELETON_ROWS; i++) {
      ol.appendChild(el('li', { className: 'skel', 'aria-hidden': 'true' },
        el('span', { className: 'mark' }),
        el('span', { className: 'num' }, ''),
        el('span', { className: 'bar' })
      ));
    }
    this._body.appendChild(ol);
  }

  _renderCode(data) {
    const target = Number(data.line) || this._req.line;
    const start = Number(data.startLine) > 0 ? Number(data.startLine) : 1;
    if (typeof data.path === 'string' && data.path) this._titlePath.textContent = data.path;
    this._absPath = typeof data.absPath === 'string' ? data.absPath : '';
    this._vscodeBtn.disabled = !this._absPath;
    if (this._absPath) this._title.setAttribute('data-tip', this._absPath);

    const ol = el('ol', { className: 'code', start: String(start) });
    if (data.language) ol.dataset.language = String(data.language);
    let hlEl = null;
    data.lines.forEach((text, i) => {
      const n = start + i;
      const li = el('li', null,
        el('span', { className: 'mark', 'aria-hidden': 'true' }),
        el('span', { className: 'num', 'aria-hidden': 'true' }, String(n)),
        el('span', { className: 'src' }, typeof text === 'string' ? text : String(text ?? ''))
      );
      if (n === target) {
        li.classList.add('hl');
        li.setAttribute('aria-current', 'true');
        hlEl = li;
      }
      ol.appendChild(li);
    });

    this._body.replaceChildren(ol);
    this._body.setAttribute('aria-busy', 'false');
    this._announce(`Evidencia cargada: ${data.lines.length} líneas, línea ${target} resaltada`);
    if (hlEl) {
      requestAnimationFrame(() => {
        const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
        hlEl.scrollIntoView({ block: 'center', behavior: reduce ? 'auto' : 'smooth' });
      });
    }
  }

  _renderError(err) {
    const code = typeof err.error === 'string' && ERROR_MESSAGES[err.error] ? err.error : 'unknown';
    const box = el('div', { className: 'err', role: 'alert' },
      el('p', null, ERROR_MESSAGES[code])
    );

    if (code === 'root_not_configured' || code === 'root_unavailable') {
      const project = this._req?.project || '<proyecto>';
      const cmd = `dioptra roots set ${project} "<ruta-del-repo>"`;
      const copy = el('button', { type: 'button' }, 'Copiar');
      let resetTimer = null;
      copy.addEventListener('click', async () => {
        const ok = await copyText(cmd);
        copy.textContent = ok ? 'Copiado' : 'No se pudo copiar';
        if (ok) this._announce('Comando copiado');
        clearTimeout(resetTimer);
        resetTimer = setTimeout(() => { copy.textContent = 'Copiar'; }, 1400);
      });
      box.appendChild(el('div', { className: 'cmd' }, el('code', null, cmd), copy));
    } else if (code === 'network' || code === 'timeout' || code === 'unknown'
      || code === 'read_error' || code === 'roots_unreadable') {
      const retry = el('button', { type: 'button' }, 'Reintentar');
      retry.addEventListener('click', () => { this._renderSkeleton(); this._load(); });
      box.appendChild(el('div', { className: 'cmd' }, retry));
    }

    const detail = [err.error && code !== err.error ? err.error : code, err.message].filter(Boolean).join(' · ');
    if (detail) box.appendChild(el('p', { className: 'code-label' }, detail));

    this._body.replaceChildren(box);
    this._body.setAttribute('aria-busy', 'false');
  }

  _openVscode() {
    if (!this._absPath || !this._req) return;
    const a = document.createElement('a');
    a.href = toVscodeHref(this._absPath, this._req.line, this._req.column);
    a.rel = 'noopener';
    a.click();
  }

  async _copyPath() {
    const text = this._absPath || this._req?.path || '';
    if (!text) return;
    const ok = await copyText(text);
    this._copyBtn.textContent = ok ? 'Copiada' : 'No se pudo copiar';
    if (ok) this._announce('Ruta copiada');
    clearTimeout(this._copyResetTimer);
    this._copyResetTimer = setTimeout(() => {
      this._copyResetTimer = null;
      this._copyBtn.textContent = 'Copiar ruta';
    }, 1400);
  }

  /** Anuncio para lectores de pantalla vía p.sr-only[role=status] (textContent, CSP). */
  _announce(msg) {
    this._status.textContent = '';
    requestAnimationFrame(() => { this._status.textContent = msg; });
  }
}

if (!customElements.get('dx-evidence')) {
  customElements.define('dx-evidence', DxEvidence);
}
