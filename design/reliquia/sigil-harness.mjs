// Harness determinista de <dx-sigil>: DOM mínimo simulado, reloj falso, rAF contable.
// Uso: node design/reliquia/sigil-harness.mjs   (sin dependencias externas, sin red)
import { pathToFileURL, fileURLToPath } from 'node:url';
import path from 'node:path';

// ---------- Reloj falso, timers y rAF ----------
const sim = { now: 0, timers: new Map(), frames: new Map(), seq: 0, animations: [], mql: [] };
globalThis.setTimeout = (fn, ms = 0) => { const id = ++sim.seq; sim.timers.set(id, { at: sim.now + Math.max(0, ms), fn }); return id; };
globalThis.clearTimeout = id => { sim.timers.delete(id); };
globalThis.requestAnimationFrame = fn => { const id = ++sim.seq; sim.frames.set(id, fn); return id; };
globalThis.cancelAnimationFrame = id => { sim.frames.delete(id); };
performance.now = () => sim.now;
let seed = 12345; // LCG determinista
Math.random = () => ((seed = (seed * 1103515245 + 12345) % 2147483648) / 2147483648);

function advance(ms) {
  const end = sim.now + ms;
  for (;;) {
    let next = null;
    for (const [id, t] of sim.timers) if (t.at <= end && (!next || t.at < next[1].at)) next = [id, t];
    if (!next) break;
    sim.timers.delete(next[0]);
    sim.now = next[1].at;
    next[1].fn();
  }
  sim.now = end;
}
function flushFrame() {
  const batch = [...sim.frames.values()]; sim.frames.clear(); sim.now += 16;
  for (const fn of batch) fn(sim.now);
  return batch.length;
}

// ---------- EventTarget con registro contable ----------
// SIGIL_NAIVE_LISTENERS=1 reproduce un mock NO conforme (identidad = opciones serializadas,
// incluyendo passive). El DOM real (WHATWG) solo usa type+callback+capture.
const NAIVE = process.env.SIGIL_NAIVE_LISTENERS === '1';
class Target {
  constructor() { this._ls = []; }
  addEventListener(type, fn, opt) {
    const capture = NAIVE ? JSON.stringify(opt ?? false) : typeof opt === 'boolean' ? opt : !!opt?.capture;
    if (!this._ls.some(l => l.type === type && l.fn === fn && l.capture === capture)) this._ls.push({ type, fn, capture });
  }
  removeEventListener(type, fn, opt) {
    const capture = NAIVE ? JSON.stringify(opt ?? false) : typeof opt === 'boolean' ? opt : !!opt?.capture;
    this._ls = this._ls.filter(l => !(l.type === type && l.fn === fn && l.capture === capture));
  }
  dispatchEvent(ev) { for (const l of [...this._ls]) if (l.type === ev.type) l.fn(ev); return true; }
  get listenerCount() { return this._ls.length; }
}

// ---------- Nodos ----------
class Node extends Target {
  constructor(tag) { super(); this.tagName = tag; this.attrs = new Map(); this.children = []; this.parentNode = null; }
  setAttribute(n, v) { const old = this.getAttribute(n); this.attrs.set(n, String(v)); this._changed(n, old, String(v)); }
  getAttribute(n) { return this.attrs.has(n) ? this.attrs.get(n) : null; }
  hasAttribute(n) { return this.attrs.has(n); }
  removeAttribute(n) { if (!this.attrs.has(n)) return; const old = this.getAttribute(n); this.attrs.delete(n); this._changed(n, old, null); }
  toggleAttribute(n, force) { const on = force === undefined ? !this.hasAttribute(n) : !!force; on ? this.setAttribute(n, '') : this.removeAttribute(n); return on; }
  _changed() {}
  append(...kids) { for (const k of kids) { if (typeof k === 'string') continue; k.parentNode = this; this.children.push(k); } }
  appendChild(k) { this.append(k); return k; }
  replaceChildren(...kids) { this.children = []; this.append(...kids); }
  animate(frames, opts) {
    const a = { node: this, frames, opts, cancelled: false, onfinish: null, oncancel: null,
      cancel() { this.cancelled = true; this.oncancel?.(); } };
    sim.animations.push(a); return a;
  }
  getScreenCTM() { return { inverse: () => ({ a: 1, d: 1, e: -100, f: -100 }) }; } // centro del ojo en (100,100)
  getBoundingClientRect() { sim.rects = (sim.rects || 0) + 1; return { left: 0, top: 0, right: 160, bottom: 173, width: 160, height: 173, x: 0, y: 0 }; }
  createSVGPoint() { return { x: 0, y: 0, matrixTransform(m) { return { x: this.x * m.a + m.e, y: this.y * m.d + m.f }; } }; }
}
class ShadowRoot extends Node { constructor() { super('#shadow'); this.adoptedStyleSheets = []; } }
class HTMLElement extends Node {
  constructor() { super('dx-sigil'); this.isConnected = false; }
  attachShadow() { this.shadowRoot = new ShadowRoot(); return this.shadowRoot; }
  get hidden() { return this.hasAttribute('hidden'); }
  set hidden(v) { this.toggleAttribute('hidden', !!v); }
  _changed(n, old, val) {
    const C = this.constructor;
    if (typeof this.attributeChangedCallback === 'function' && (C.observedAttributes || []).includes(n)) this.attributeChangedCallback(n, old, val);
  }
}
globalThis.HTMLElement = HTMLElement;
globalThis.CSSStyleSheet = class { replaceSync(t) { this.cssText = t; } };

const registry = new Map();
globalThis.customElements = { define: (n, c) => registry.set(n, c), get: n => registry.get(n) };

const doc = new Target();
doc.hidden = false;
doc.createElementNS = (ns, tag) => new Node(tag);
doc.createElement = tag => { const C = registry.get(tag); return C ? new C() : new Node(tag); };
doc.body = {
  appendChild(el) { el.isConnected = true; el.connectedCallback?.(); return el; },
  removeChild(el) { el.isConnected = false; el.disconnectedCallback?.(); return el; }
};
globalThis.document = doc;

const win = new Target();
let reducedPref = false;
win.matchMedia = query => {
  const m = new Target(); m.media = query; m.matches = reducedPref; sim.mql.push(m); return m;
};
globalThis.window = win;
function setReduced(v) {
  reducedPref = v;
  for (const m of sim.mql) { m.matches = v; m.dispatchEvent({ type: 'change', matches: v, media: m.media }); }
}

// ---------- Utilidades de prueba ----------
function reset() {
  sim.now = 0; sim.timers.clear(); sim.frames.clear(); sim.animations = []; sim.mql = [];
  doc._ls = []; win._ls = []; doc.hidden = false; reducedPref = false; seed = 12345;
}
function mount(attrs = {}, reduced = false) {
  reset(); reducedPref = reduced;
  const el = doc.createElement('dx-sigil');
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  doc.body.appendChild(el);
  return el;
}
const pointer = (x = 300, y = 200) => doc.dispatchEvent({ type: 'pointermove', clientX: x, clientY: y });
const BLINK_MS = new Set([190, 390, 560]); // rapido / doble / siniestro
const blinks = () => sim.animations.filter(a => BLINK_MS.has(a.opts?.duration)).length;
const mqlListeners = () => sim.mql.reduce((n, m) => n + m.listenerCount, 0);
function find(node, pred) { if (pred(node)) return node; for (const c of node.children) { const r = find(c, pred); if (r) return r; } return null; }
const irisT = el => el._iris.getAttribute('transform');
const axleT = el => el._axle?.getAttribute('transform');
const axleX = el => { const m = /translate\(([-0-9.]+)/.exec(axleT(el) || ''); return m ? parseFloat(m[1]) : NaN; };
const drain = (max = 1000) => { let n = 0; while (sim.frames.size && n < max) { flushFrame(); n++; } return n; };
function walk(node, fn) { fn(node); for (const c of node.children) walk(c, fn); }
/** Elimina var(...) con paréntesis balanceados (incluye anidados y fallbacks con rgb()). */
function stripVars(css) {
  let out = '', i = 0;
  while (i < css.length) {
    if (css.startsWith('var(', i)) {
      let depth = 0, j = i + 3;
      for (; j < css.length; j++) { if (css[j] === '(') depth++; else if (css[j] === ')' && --depth === 0) break; }
      i = j + 1;
    } else out += css[i++];
  }
  return out;
}

const results = [];
function check(name, cond, detail = '') { results.push({ name, ok: !!cond, detail }); }
async function test(title, fn) {
  try { await fn(); } catch (e) { results.push({ name: `${title} :: lanzó excepción`, ok: false, detail: e.stack }); }
}

// ---------- Carga del componente ----------
const here = path.dirname(fileURLToPath(import.meta.url));
await import(pathToFileURL(path.resolve(here, '../../ui/components/dioptra-sigil.js')).href);

await test('T1', () => {
  const el = mount({ size: 'full', state: 'awake' });
  check('T1 awake: timers de parpadeo/microsacada programados', sim.timers.size > 0, `timers=${sim.timers.size}`);
  advance(8001);
  check('T1 awake: parpadeo automático ocurre en <=8s', blinks() >= 1, `blinks=${blinks()}`);
  check('T1 awake: sigue programando tras parpadear', sim.timers.size > 0);
  doc.body.removeChild(el);
});

await test('T2', () => {
  for (const via of ['sleep()', 'setAttribute(state=asleep)']) {
    const el = mount({ state: 'awake' });
    pointer();
    check(`T2 ${via}: precondición rAF pendiente`, sim.frames.size === 1, `rAF=${sim.frames.size}`);
    via === 'sleep()' ? el.sleep() : el.setAttribute('state', 'asleep');
    check(`T2 ${via}: 0 rAF pendientes`, sim.frames.size === 0, `rAF=${sim.frames.size}`);
    check(`T2 ${via}: 0 timers pendientes`, sim.timers.size === 0, `timers=${sim.timers.size}`);
    const before = blinks(); advance(20000); pointer();
    check(`T2 ${via}: sin parpadeo ni rAF dormido`, blinks() === before && sim.frames.size === 0);
    doc.body.removeChild(el);
  }
});

await test('T3', () => {
  const el = mount({ state: 'awake' });
  pointer();
  doc.hidden = true; doc.dispatchEvent({ type: 'visibilitychange' });
  check('T3 hidden: 0 rAF', sim.frames.size === 0, `rAF=${sim.frames.size}`);
  check('T3 hidden: 0 timers', sim.timers.size === 0, `timers=${sim.timers.size}`);
  pointer(10, 10);
  check('T3 hidden: pointermove no reanuda rAF', sim.frames.size === 0);
  doc.hidden = false; doc.dispatchEvent({ type: 'visibilitychange' });
  check('T3 visible: reanuda timers', sim.timers.size > 0);
  doc.body.removeChild(el);
});

await test('T4', () => {
  let el = mount({ state: 'awake' }, true);
  pointer();
  check('T4 reduced: sin seguimiento (0 rAF)', sim.frames.size === 0);
  check('T4 reduced: 0 timers', sim.timers.size === 0, `timers=${sim.timers.size}`);
  advance(20000);
  check('T4 reduced: sin parpadeo automático', blinks() === 0, `blinks=${blinks()}`);
  check('T4 reduced: ojo centrado', irisT(el) === 'translate(0 0)', irisT(el));
  doc.body.removeChild(el);

  el = mount({ state: 'awake' }, false);
  pointer(); for (let i = 0; i < 5; i++) flushFrame();
  check('T4 en caliente: precondición iris desplazado', irisT(el) !== 'translate(0 0)', irisT(el));
  setReduced(true);
  check('T4 en caliente reduce: ojo centrado', irisT(el) === 'translate(0 0)', irisT(el));
  check('T4 en caliente reduce: 0 rAF y 0 timers', sim.frames.size === 0 && sim.timers.size === 0,
    `rAF=${sim.frames.size} timers=${sim.timers.size}`);
  setReduced(false);
  check('T4 en caliente no-reduce: reprograma parpadeo', sim.timers.size > 0);
  doc.body.removeChild(el);
});

await test('T5', () => {
  const el = mount({ state: 'awake' });
  pointer();
  let frames = 0;
  while (sim.frames.size && frames < 1000) { flushFrame(); frames++; }
  check('T5 reposo: rAF converge y se detiene (<200 frames)', sim.frames.size === 0 && frames < 200, `frames=${frames}`);
  advance(10000);
  // El escaneo autónomo abre rAF tras su timer (setTimeout); debe converger y pararse.
  const opened = sim.frames.size;
  const n = drain();
  check('T5 escaneo: abre como mucho 1 rAF tras advance', opened <= 1, `rAF=${opened}`);
  check('T5 escaneo: converge y se detiene (<200 frames)', sim.frames.size === 0 && n < 200, `frames=${n}`);
  check('T5 escaneo: la mirada se movió (autónomo)', irisT(el) !== 'translate(0 0)', irisT(el));
  doc.body.removeChild(el);
});

await test('T6', () => {
  const el = mount({ state: 'awake' });
  pointer(); advance(3000);
  doc.body.removeChild(el);
  check('T6 disconnect: 0 listeners en document', doc.listenerCount === 0, `doc=${doc.listenerCount}`);
  check('T6 disconnect: 0 listeners en window', win.listenerCount === 0, `win=${win.listenerCount}`);
  check('T6 disconnect: 0 listeners en matchMedia', mqlListeners() === 0, `mql=${mqlListeners()}`);
  check('T6 disconnect: 0 timers', sim.timers.size === 0, `timers=${sim.timers.size}`);
  check('T6 disconnect: 0 rAF', sim.frames.size === 0, `rAF=${sim.frames.size}`);
  doc.body.appendChild(el); doc.body.removeChild(el); doc.body.appendChild(el);
  const docN = doc.listenerCount;
  doc.body.removeChild(el);
  check('T6 reconexión: sin duplicar listeners', docN === 4 && doc.listenerCount === 0 && win.listenerCount === 0, `docN=${docN}`);
});

await test('T7', () => {
  for (const size of ['glyph', 'mark', 'full']) {
    let el, err = null;
    try { el = mount({ size, state: 'awake' }); } catch (e) { err = e; }
    check(`T7 size=${size}: construye sin lanzar`, !err, err?.message);
    if (!el) continue;
    const svgNode = el.shadowRoot.children[0];
    check(`T7 size=${size}: <svg> en shadowRoot`, svgNode?.tagName === 'svg');
    const tent = find(svgNode, n => /\btentacle\b/.test(n.getAttribute('class') || ''));
    check(`T7 size=${size}: tentáculos solo en full`, (size === 'full') === !!tent);
    check(`T7 size=${size}: adoptedStyleSheets`, el.shadowRoot.adoptedStyleSheets.length === 1);
    doc.body.removeChild(el);
  }
});

await test('T8', () => {
  let el = mount({ decorative: '' });
  check('T8 decorative: aria-hidden=true', el.getAttribute('aria-hidden') === 'true');
  check('T8 decorative: sin role/aria-label', !el.hasAttribute('role') && !el.hasAttribute('aria-label'));
  el.removeAttribute('decorative');
  check('T8 quitar decorative: role=img + aria-label', el.getAttribute('role') === 'img' && el.getAttribute('aria-label') === 'Dioptra');
  doc.body.removeChild(el);
  el = mount({});
  check('T8 no decorative: role=img', el.getAttribute('role') === 'img');
  check('T8 no decorative: aria-label=Dioptra', el.getAttribute('aria-label') === 'Dioptra');
  check('T8 no decorative: sin aria-hidden', !el.hasAttribute('aria-hidden'));
  doc.body.removeChild(el);
});

await test('T9 bordes', () => {
  let el = mount({ state: 'awake' });
  check('T9 awake: exactamente 2 timers (blink+micro), sin duplicados', sim.timers.size === 2, `timers=${sim.timers.size}`);
  el.setAttribute('size', 'mark');
  check('T9 cambio size: sigue con 2 timers', sim.timers.size === 2, `timers=${sim.timers.size}`);
  el.hidden = true;
  check('T9 hidden attr: 0 timers y 0 rAF', sim.timers.size === 0 && sim.frames.size === 0, `timers=${sim.timers.size}`);
  el.hidden = false;
  el.sleep(); el.wake();
  check('T9 sleep→wake: 2 timers, sin duplicar', sim.timers.size === 2, `timers=${sim.timers.size}`);
  const live = () => sim.animations.filter(a => !a.cancelled).length;
  el.sleep();
  doc.body.removeChild(el);
  check('T9 disconnect a mitad de transición: animaciones canceladas', live() === 0, `vivas=${live()}`);
  el = mount({ state: 'asleep' });
  check('T9 conectar dormido: 0 timers', sim.timers.size === 0, `timers=${sim.timers.size}`);
  setReduced(true); setReduced(false);
  check('T9 reduced en caliente dormido: sigue 0 timers', sim.timers.size === 0);
  doc.body.removeChild(el);
  el = mount({ state: 'awake' });
  el.attend(300, 200);
  check('T9 attend: pupila contraída 0.85', /scale\(0\.85\)/.test(el._pupil.getAttribute('transform')));
  pointer(0, 0); const t = el._target.x;
  check('T9 attend bloquea pointermove', t > 0);
  el.release();
  let n = 0; while (sim.frames.size && n < 1000) { flushFrame(); n++; }
  check('T9 release: converge al centro y detiene rAF', irisT(el) === 'translate(0 0)' && sim.frames.size === 0, `${irisT(el)} frames=${n}`);
  doc.body.removeChild(el);
});

await test('T10 eje', () => {
  // El eje mecánico existe en full y mark, no en glyph
  for (const size of ['full', 'mark']) {
    const e = mount({ size, state: 'awake' });
    check(`T10 eje: presente en ${size}`, !!e._axle);
    doc.body.removeChild(e);
  }
  const elGlyph = mount({ size: 'glyph', state: 'awake' });
  check('T10 eje: ausente (null) en glyph', elGlyph._axle === null, String(elGlyph._axle));
  doc.body.removeChild(elGlyph);

  // Las perillas se desplazan hacia el puntero (miradaX*2.8*0.7, máx 1.96)
  const el = mount({ size: 'full', state: 'awake' });
  check('T10 eje: reposo inicial en 0', axleT(el) === 'translate(0 0)', axleT(el));
  pointer(500, 100);
  for (let i = 0; i < 10; i++) flushFrame();
  const x = axleX(el);
  check('T10 eje: se desplaza con puntero derecho (0 < x <= 1.96)', x > 0 && x <= 1.96, axleT(el));

  // 0 con reduced-motion (en caliente)
  setReduced(true);
  check('T10 eje: 0 con reduced-motion', axleT(el) === 'translate(0 0)', axleT(el));
  check('T10 eje: reduced sin rAF ni timers', sim.frames.size === 0 && sim.timers.size === 0,
    `rAF=${sim.frames.size} timers=${sim.timers.size}`);
  setReduced(false);

  // 0 en asleep
  pointer(500, 100); for (let i = 0; i < 10; i++) flushFrame();
  check('T10 eje: precondición desplazado antes de dormir', axleX(el) > 0, axleT(el));
  el.sleep();
  check('T10 eje: 0 en asleep', axleT(el) === 'translate(0 0)', axleT(el));
  check('T10 eje: asleep sin rAF', sim.frames.size === 0, `rAF=${sim.frames.size}`);

  // Converge tras release y no queda rAF en reposo
  el.wake();
  pointer(500, 100); drain();
  el.release();
  const n = drain();
  check('T10 eje: release converge a 0 (<200 frames)', axleT(el) === 'translate(0 0)' && n < 200, `${axleT(el)} frames=${n}`);
  check('T10 eje: 0 rAF en reposo', sim.frames.size === 0, `rAF=${sim.frames.size}`);
  doc.body.removeChild(el);
});

await test('T11 ids', () => {
  reset();
  const a = doc.createElement('dx-sigil'); a.setAttribute('size', 'full');
  const b = doc.createElement('dx-sigil'); b.setAttribute('size', 'full');
  doc.body.appendChild(a); doc.body.appendChild(b);
  const ids = [];
  for (const e of [a, b]) walk(e.shadowRoot, n => { const id = n.getAttribute?.('id'); if (id != null) ids.push(id); });
  check('T11 ids: hay ids en ambas instancias', ids.length > 0, `ids=${ids.length}`);
  const bad = ids.filter(id => !/^sg\d+-/.test(id));
  check('T11 ids: todos con prefijo sg<n>-', bad.length === 0, bad.slice(0, 5).join(','));
  const dup = ids.filter((id, i) => ids.indexOf(id) !== i);
  check('T11 ids: sin duplicados entre instancias', dup.length === 0, dup.slice(0, 5).join(','));
  doc.body.removeChild(a); doc.body.removeChild(b);
});

await test('T12 colores', () => {
  const el = mount({ size: 'full', state: 'awake' });
  const sheet = el.shadowRoot.adoptedStyleSheets[0];
  const css = sheet?.cssText || '';
  check('T12 colores: hoja con contenido', css.length > 0, `len=${css.length}`);
  const rest = stripVars(css);
  const hex = rest.match(/#[0-9a-f]{3,8}/gi) || [];
  check('T12 colores: sin hex fuera de var()', hex.length === 0, hex.slice(0, 5).join(','));
  check('T12 colores: sin rgb( fuera de var()', !/rgba?\(/i.test(rest));
  doc.body.removeChild(el);
});

// ---------- Reporte ----------
const failed = results.filter(r => !r.ok);
for (const r of results) console.log(`${r.ok ? 'PASS' : 'FAIL'}  ${r.name}${!r.ok && r.detail ? `  [${r.detail}]` : ''}`);
console.log(`\n${results.length - failed.length}/${results.length} aserciones OK, ${failed.length} fallidas`);
process.exitCode = failed.length ? 1 : 0;
