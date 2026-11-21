#!/usr/bin/env node
// DioptraEngine · Reliquia — medición de contraste WCAG 2.x de ui/styles/tokens.css.
// Sin dependencias. Uso: node design/reliquia/contrast-check.mjs [--json]
// Parsea el bloque oscuro (`:root, [data-theme="dark"]`) y el claro (`[data-theme="light"]`),
// resuelve var() recursivos con fallback, compone alfa sobre el plano indicado y evalúa pares.
// Sale con código 1 si algún par queda por debajo de su mínimo.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const HERE = dirname(fileURLToPath(import.meta.url));
const TOKENS = resolve(HERE, '../../ui/styles/tokens.css');

// ---------------------------------------------------------------- parseo CSS
function stripComments(src) {
  return src.replace(/\/\*[\s\S]*?\*\//g, '');
}

// Devuelve el cuerpo del primer bloque de nivel superior cuyo selector cumple `test`.
function findTopLevelBlock(src, test) {
  let depth = 0;
  let selStart = 0;
  for (let i = 0; i < src.length; i++) {
    const ch = src[i];
    if (ch === '{') {
      if (depth === 0) {
        const selector = src.slice(selStart, i).trim();
        let d = 1;
        let j = i + 1;
        for (; j < src.length && d > 0; j++) {
          if (src[j] === '{') d++;
          else if (src[j] === '}') d--;
        }
        if (test(selector)) return src.slice(i + 1, j - 1);
        i = j - 1;
        selStart = j;
        continue;
      }
      depth++;
    } else if (ch === '}') {
      depth--;
      if (depth === 0) selStart = i + 1;
    } else if (ch === ';' && depth === 0) {
      selStart = i + 1; // @import / @charset
    }
  }
  return null;
}

function parseDecls(body) {
  const out = new Map();
  let depth = 0;
  let cur = '';
  const flush = () => {
    const m = /^\s*(--[\w-]+)\s*:\s*([\s\S]*?)\s*$/.exec(cur);
    if (m) out.set(m[1], m[2]);
    cur = '';
  };
  for (const ch of body) {
    if (ch === '(') depth++;
    if (ch === ')') depth--;
    if (ch === ';' && depth === 0) { flush(); continue; }
    cur += ch;
  }
  if (cur.trim()) flush();
  return out;
}

const norm = (s) => s.replace(/\s+/g, '');
const css = stripComments(readFileSync(TOKENS, 'utf8'));
const darkBody = findTopLevelBlock(css, (s) => norm(s) === ':root,[data-theme="dark"]');
const lightBody = findTopLevelBlock(css, (s) => norm(s) === '[data-theme="light"]');
if (!darkBody || !lightBody) {
  console.error('No se encontraron los bloques de tema en tokens.css');
  process.exit(2);
}
const darkVars = parseDecls(darkBody);
// El claro hereda de :root lo que no redefine.
const lightVars = new Map([...darkVars, ...parseDecls(lightBody)]);

// ---------------------------------------------------------------- var() + color
function resolveValue(value, vars, seen = new Set()) {
  let guard = 0;
  while (/var\(/.test(value)) {
    if (++guard > 64) throw new Error('var() demasiado profundo: ' + value);
    const idx = value.indexOf('var(');
    let d = 0;
    let end = idx + 3;
    for (; end < value.length; end++) {
      if (value[end] === '(') d++;
      else if (value[end] === ')') { d--; if (d === 0) break; }
    }
    const inner = value.slice(idx + 4, end);
    let comma = -1;
    let dd = 0;
    for (let k = 0; k < inner.length; k++) {
      if (inner[k] === '(') dd++;
      else if (inner[k] === ')') dd--;
      else if (inner[k] === ',' && dd === 0) { comma = k; break; }
    }
    const name = (comma < 0 ? inner : inner.slice(0, comma)).trim();
    const fallback = comma < 0 ? null : inner.slice(comma + 1).trim();
    let rep;
    if (vars.has(name) && !seen.has(name)) {
      const next = new Set(seen).add(name);
      rep = resolveValue(vars.get(name), vars, next);
    } else if (fallback !== null) {
      rep = resolveValue(fallback, vars, seen);
    } else {
      throw new Error('Token sin definir: ' + name);
    }
    value = value.slice(0, idx) + rep + value.slice(end + 1);
  }
  return value.trim();
}

function parseColor(str) {
  const s = str.trim().toLowerCase();
  if (s === 'transparent') return [0, 0, 0, 0];
  let m = /^#([0-9a-f]{3,8})$/.exec(s);
  if (m) {
    let h = m[1];
    if (h.length === 3 || h.length === 4) h = [...h].map((c) => c + c).join('');
    const n = (i) => parseInt(h.slice(i, i + 2), 16);
    return [n(0), n(2), n(4), h.length === 8 ? n(6) / 255 : 1];
  }
  m = /^rgba?\(([^)]+)\)$/.exec(s);
  if (m) {
    const p = m[1].split(/[\s,/]+/).filter(Boolean).map((x) =>
      x.endsWith('%') ? parseFloat(x) / 100 : parseFloat(x));
    return [p[0], p[1], p[2], p.length > 3 ? p[3] : 1];
  }
  throw new Error('Color no soportado: ' + str);
}

function colorOf(ref, vars) {
  const raw = ref.startsWith('--') ? resolveValue(`var(${ref})`, vars) : resolveValue(ref, vars);
  return parseColor(raw);
}

function over(top, bottom) {
  const a = top[3];
  return [0, 1, 2].map((i) => top[i] * a + bottom[i] * (1 - a)).concat(1);
}

// bg: token | [capa superior, ..., capa inferior]. La capa inferior se apoya sobre bg-body.
function composeBg(bg, vars) {
  const layers = (Array.isArray(bg) ? bg : [bg]).map((r) => colorOf(r, vars));
  let acc = over(colorOf('--color-bg-body', vars), [0, 0, 0, 1]);
  for (let i = layers.length - 1; i >= 0; i--) acc = over(layers[i], acc);
  return acc;
}

function lum([r, g, b]) {
  const f = (c) => { c /= 255; return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4; };
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
}
function ratio(a, b) {
  const [l1, l2] = [lum(a), lum(b)].sort((x, y) => y - x);
  return (l1 + 0.05) / (l2 + 0.05);
}
const hex = (c) => '#' + c.slice(0, 3).map((v) => Math.round(v).toString(16).padStart(2, '0')).join('').toUpperCase();

// ---------------------------------------------------------------- pares
// Superficie del timeline (donde viven chips): oscuro surface-base; claro surface-raised (shell.css:162).
const TL = { dark: '--color-surface-base', light: '--color-surface-raised' };
// Placeholder (shell.css:277) = text-soft sobre el input: sunken en oscuro, bg-body en claro (shell.css:268).
const INPUT = { dark: '--color-surface-sunken', light: '--color-bg-body' };
// Cabecera sticky (shell.css:615) y botón de tema (shell.css:774-785).
const HEADER = '--color-surface-elevated';

const PAIRS = [];
const add = (name, fg, bg, min, themes = ['dark', 'light']) => PAIRS.push({ name, fg, bg, min, themes });

for (const t of ['primary', 'secondary', 'soft']) {
  add(`text-${t} / base`, `--color-text-${t}`, '--color-surface-base', 4.5);
  add(`text-${t} / raised`, `--color-text-${t}`, '--color-surface-raised', 4.5);
}
add('accent-fg / base', '--color-accent-fg', '--color-surface-base', 4.5);
add('bronze-text / base', '--color-bronze-text', '--color-surface-base', 4.5);
add('gold-text / base', '--color-gold-text', '--color-surface-base', 4.5);
add('on-primary / copper', '--color-on-primary', '--color-copper', 4.5);
for (const s of ['info', 'success', 'warning', 'error']) {
  add(`sev-${s}-fg / sev-${s}-bg`, `--color-sev-${s}-fg`, `--color-sev-${s}-bg`, 4.5);
}
for (const th of ['dark', 'light']) {
  add('chip-count / timeline', '--color-text-secondary', TL[th], 4.5, [th]);
  add('chip-count / chip pulsado', '--color-text-secondary', ['--color-match-bg', TL[th]], 4.5, [th]);
  add('placeholder (text-soft) / input', '--color-text-soft', INPUT[th], 4.5, [th]);
  add('timeline-error (sev-error-fg) / timeline', '--color-sev-error-fg', TL[th], 4.5, [th]);
}
add('focus / base', '--color-focus', '--color-surface-base', 3);
add('focus / raised', '--color-focus', '--color-surface-raised', 3);
add('focus / cabecera', '--color-focus', HEADER, 3);
// En oscuro la tinta es sombra cel-shaded (casi negra sobre piedra casi negra): por diseño
// no es un contorno perceptible; el contorno real lo dan bronce/oro. Se mide solo en claro.
add('ink / base', '--color-ink', '--color-surface-base', 3, ['light']);
PAIRS.push({ name: 'ink / base (sombra cel, informativo)', fg: '--color-ink', bg: '--color-surface-base', min: 0, themes: ['dark'], info: true });
add('border-strong / base', '--color-border-strong', '--color-surface-base', 3);
add('theme-btn contorno / cabecera', '--color-control-border', HEADER, 3);
add('theme-btn contorno / botón', '--color-control-border', '--color-interactive-secondary', 3);
add('separador (border-translucent) / base', '--color-border-translucent', '--color-surface-base', 1.5);
add('separador (border-translucent) / raised', '--color-border-translucent', '--color-surface-raised', 1.5);

// ---------------------------------------------------------------- evaluación
const THEMES = { dark: darkVars, light: lightVars };
const rows = [];
let failed = 0;
for (const p of PAIRS) {
  for (const th of p.themes) {
    const vars = THEMES[th];
    let r;
    let fgHex = '?';
    let bgHex = '?';
    let err = null;
    try {
      const bg = composeBg(p.bg, vars);
      const fg = over(colorOf(p.fg, vars), bg);
      r = ratio(fg, bg);
      fgHex = hex(fg);
      bgHex = hex(bg);
    } catch (e) {
      err = e.message;
      r = 0;
    }
    const ok = !err && r + 1e-9 >= p.min;
    if (!ok) failed++;
    rows.push({ theme: th, name: p.name, fg: fgHex, bg: bgHex, ratio: Number(r.toFixed(2)), min: p.min, ok, err, info: !!p.info });
  }
}

if (process.argv.includes('--json')) {
  console.log(JSON.stringify(rows, null, 2));
} else {
  const w = [6, 44, 8, 8, 6, 4];
  const line = (c) => c.map((v, i) => String(v).padEnd(w[i])).join(' ');
  console.log(line(['tema', 'par', 'fg', 'bg', 'ratio', 'min']) + ' estado');
  console.log('-'.repeat(86));
  for (const r of rows) {
    console.log(line([r.theme, r.name, r.fg, r.bg, r.ratio.toFixed(2), r.info ? '—' : r.min]) + ' ' +
      (r.info ? 'INFO' : r.ok ? 'PASS' : 'FAIL' + (r.err ? ' (' + r.err + ')' : '')));
  }
  console.log('-'.repeat(86));
  console.log(failed ? `FAIL: ${failed} par(es) por debajo del mínimo` : `PASS: ${rows.length} pares`);
}
process.exit(failed ? 1 : 0);
