// Genera design/reliquia/sigil-preview.svg: 3 niveles (full, mark, glyph) en tema oscuro y claro.
// DOM mínimo propio (el harness ejecuta tests al importarse). Clases -> hex desde la hoja del componente.
import { pathToFileURL, fileURLToPath } from 'node:url';
import path from 'node:path';
import fs from 'node:fs';

class Node {
  constructor(tag) { this.tagName = tag; this.attrs = new Map(); this.children = []; }
  setAttribute(n, v) { this.attrs.set(n, String(v)); }
  getAttribute(n) { return this.attrs.has(n) ? this.attrs.get(n) : null; }
  hasAttribute(n) { return this.attrs.has(n); }
  removeAttribute(n) { this.attrs.delete(n); }
  toggleAttribute(n, f) { const on = f === undefined ? !this.attrs.has(n) : !!f; on ? this.attrs.set(n, '') : this.attrs.delete(n); return on; }
  append(...k) { for (const c of k) if (typeof c !== 'string') this.children.push(c); }
  replaceChildren(...k) { this.children = []; this.append(...k); }
  addEventListener() {} removeEventListener() {}
}
globalThis.HTMLElement = class extends Node {
  constructor() { super('dx-sigil'); }
  attachShadow() { this.shadowRoot = new Node('#shadow'); this.shadowRoot.adoptedStyleSheets = []; return this.shadowRoot; }
  get hidden() { return false; }
};
globalThis.CSSStyleSheet = class { replaceSync(t) { this.cssText = t; } };
globalThis.document = { hidden: false, createElementNS: (_, t) => new Node(t), addEventListener() {}, removeEventListener() {} };
globalThis.window = { matchMedia: () => ({ matches: true, addEventListener() {}, removeEventListener() {} }) };
globalThis.customElements = { define() {}, get() {} };
globalThis.requestAnimationFrame = () => 0; globalThis.cancelAnimationFrame = () => {};

const here = path.dirname(fileURLToPath(import.meta.url));
const { DxSigil } = await import(pathToFileURL(path.resolve(here, '../../ui/components/dioptra-sigil.js')).href);

// Paleta: .f-key{fill:var(--sigil-name, #hex)}
const css = new DxSigil().shadowRoot.adoptedStyleSheets[0].cssText;
const rules = {};
for (const m of css.matchAll(/\.([fsc])-([\w-]+)\{(fill|stroke|stop-color):var\(--sigil-([\w-]+), (#[0-9A-Fa-f]{3,8})\)\}/g)) {
  rules[`${m[1]}-${m[2]}`] = { prop: m[3], token: m[4], hex: m[5] };
}
const THEMES = {
  dark: { bg: '#0B1017', ink: null, rim: 'rgba(229,178,92,.28)' },
  light: { bg: '#E4DEC9', ink: '#2A1E14', rim: 'transparent' }
};

const esc = s => s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
function ser(node, theme, depth) {
  const attrs = new Map(node.attrs);
  const cls = (attrs.get('class') || '').split(/\s+/).filter(Boolean);
  attrs.delete('class');
  for (const name of ['data-paused', 'data-level', 'data-reduced', 'aria-hidden', 'focusable']) attrs.delete(name);
  for (const c of cls) {
    if (c === 'nf') attrs.set('fill', 'none');
    else if (c === 'rim') { attrs.set('fill', 'none'); attrs.set('stroke', theme.rim); }
    else if (rules[c]) {
      const r = rules[c];
      const value = r.token === 'ink' && theme.ink ? theme.ink : r.hex;
      attrs.set(r.prop, value);
    }
  }
  const pad = '  '.repeat(depth);
  const a = [...attrs].map(([k, v]) => ` ${k}="${esc(v)}"`).join('');
  if (!node.children.length) return `${pad}<${node.tagName}${a}/>`;
  return `${pad}<${node.tagName}${a}>\n${node.children.map(c => ser(c, theme, depth + 1)).join('\n')}\n${pad}</${node.tagName}>`;
}

const SLOTS = [['full', 380], ['mark', 56], ['glyph', 24]];
const ROW_H = 440, W = 760;
const out = [`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${ROW_H * 2}" width="${W}" height="${ROW_H * 2}">`,
  '  <!-- Generado por design/reliquia/tmp-preview.mjs: full 380px, mark 56px, glyph 24px; fila oscura y clara -->'];
Object.values(THEMES).forEach((theme, row) => {
  const y0 = row * ROW_H;
  out.push(`  <rect x="0" y="${y0}" width="${W}" height="${ROW_H}" fill="${theme.bg}"/>`);
  let x = 30;
  for (const [size, px] of SLOTS) {
    const s = new DxSigil(); s.setAttribute('size', size); s.attributeChangedCallback?.('size', null, size);
    const svg = s.shadowRoot.children[0];
    const [, , vw, vh] = svg.getAttribute('viewBox').split(' ').map(Number);
    const h = px * vh / vw;
    svg.setAttribute('x', x); svg.setAttribute('y', fxy(y0 + (ROW_H - h) / 2)); svg.setAttribute('width', px); svg.setAttribute('height', fxy(h));
    svg.setAttribute('overflow', 'visible');
    out.push(ser(svg, theme, 1));
    x += px + 60;
  }
});
out.push('</svg>\n');
function fxy(n) { return +n.toFixed(2); }
const dest = path.resolve(here, 'sigil-preview.svg');
fs.writeFileSync(dest, out.join('\n'));
const txt = out.join('\n');
console.log(`OK ${dest} bytes=${txt.length} reglas=${Object.keys(rules).length} class=${/ class=/.test(txt)} var=${/var\(/.test(txt)}`);
