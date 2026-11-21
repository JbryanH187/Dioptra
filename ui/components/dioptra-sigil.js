// Sigilo Bellum (ronda 3): dibujo de design/reliquia/bellum-reference.svg portado a tabla de datos.
// DOM SVG seguro (createElementNS), colores por clase + var(), IDs con prefijo por instancia,
// movimiento continuo en CSS y rAF solo mientras la mirada converge.
import { adoptSheets, createSheet } from './shared.js';

const NS = 'http://www.w3.org/2000/svg';
let nextId = 0;
// Mismas opciones en add/remove: simetría explícita, robusta ante polyfills/mocks.
const POINTER_OPTS = Object.freeze({ passive: true });
const SCROLL_OPTS = Object.freeze({ passive: true, capture: true });
const random = (min, max) => min + Math.random() * (max - min);
const fx = n => +n.toFixed(2);
const COS20 = Math.cos(Math.PI / 9);
const SIN20 = Math.sin(Math.PI / 9);
// mark: trazos escalados ×3.8 con mínimo 19 u (≥2 px renderizados a 36 px sobre viewBox de 340 u).
const MARK_K = 3.8;
const MARK_MIN = 19;

// ---------- Paleta: hex de la referencia como fallback de --sigil-<clave> ----------
const PAL = {
  ink: ['ink', '0E1116'], inkd: ['ink', '0B0E13'],
  abyss: '0F1720', vein: '243547', abyss2: '15212D', shade: '0A1017', vein2: '2E4358', pore: 'F8C82C',
  mini: 'FFD426', mpupil: '0B0E13', lance: '162330', 'lance-shade': '0D151E', vein3: '2E4359',
  'bronze-a': 'ECC05B', 'bronze-b': 'CE8E2E', 'bronze-c': '8C5214', 'wing-hi': 'F8D278', 'wing-low': '6E3E0C', engrave: '4A2707',
  'torso-a': 'F28E36', 'torso-b': 'DC6A21', 'torso-c': '963910', 'neck-shade': '852F0B', 'neck-cut': '3B1405',
  'spiral-hi': 'FA9F42', 'copper-deep': '87310C', spiral: '1A0D08', 'spiral-glint': 'FFB156',
  'knob-bar': '9E5218', 'knob-line': 'E59638', 'knob-ring': 'C97121', 'knob-hi': 'FFD075',
  'axle-a': 'F5B14C', 'axle-b': 'CF7522', 'axle-c': '7E390E',
  socket: '0D141C', 'striae-1': '152230', 'striae-1e': '090D12', 'striae-2': '1D2E3F', 'striae-2e': '0B1017',
  'striae-3': '25394D', 'striae-3e': '0D1219', groove: '314961',
  'iris-a': 'FFF16B', 'iris-b': 'F8C622', 'iris-c': 'DF9614', 'iris-d': 'A86408', 'iris-ring': 'EAA219',
  'core-a': 'FFF587', 'core-b': 'FFD12A', 'core-c': 'E39E14', pupil: '090B0E', glint: 'FFF9CF', veil: '070A0F',
  'sup-a': 'F08B33', 'sup-b': 'DA6820', 'sup-c': 'AE4614', 'sup-d': '87310C', 'lid-line': '963910',
  'inf-a': 'DF6E24', 'inf-b': 'BA4F18', 'inf-c': '7D2E0B', 'lid-line-2': 'F08D38',
  'dome-hi': 'F79B39', 'dome-spark': 'FFC663', 'dome-shade': '9C3C11', 'dome-arc': 'F79E3E',
  'cup-hi': 'F28F38', zig: '3B1406', 'zig-hi': 'DF7528'
};

// Gradientes: [tag, attrs, [[offset%, claveDePaleta]...]]. Los stops llevan clase c-<clave>.
const GRADS = {
  iris: ['radialGradient', { cx: '48%', cy: '46%', r: '54%' }, [[0, 'iris-a'], [48, 'iris-b'], [82, 'iris-c'], [100, 'iris-d']]],
  core: ['radialGradient', { cx: '50%', cy: '45%', r: '55%' }, [[0, 'core-a'], [65, 'core-b'], [100, 'core-c']]],
  sup: ['linearGradient', { x1: '25%', y1: '5%', x2: '72%', y2: '98%' }, [[0, 'sup-a'], [52, 'sup-b'], [88, 'sup-c'], [100, 'sup-d']]],
  inf: ['linearGradient', { x1: '20%', y1: '0%', x2: '80%', y2: '100%' }, [[0, 'inf-a'], [58, 'inf-b'], [100, 'inf-c']]],
  torso: ['linearGradient', { x1: '22%', y1: '8%', x2: '82%', y2: '95%' }, [[0, 'torso-a'], [50, 'torso-b'], [100, 'torso-c']]],
  bronze: ['linearGradient', { x1: '0%', y1: '0%', x2: '0%', y2: '100%' }, [[0, 'bronze-a'], [45, 'bronze-b'], [100, 'bronze-c']]],
  axle: ['linearGradient', { x1: '15%', y1: '0%', x2: '85%', y2: '100%' }, [[0, 'axle-a'], [52, 'axle-b'], [100, 'axle-c']]]
};
const STOP_KEYS = new Set(Object.values(GRADS).flatMap(g => g[2].map(s => s[1])));

const paletteCss = Object.entries(PAL).map(([key, value]) => {
  const [name, hex] = Array.isArray(value) ? value : [key, value];
  const v = `var(--sigil-${name}, #${hex})`;
  return `.f-${key}{fill:${v}}.s-${key}{stroke:${v}}${STOP_KEYS.has(key) ? `.c-${key}{stop-color:${v}}` : ''}`;
}).join('\n');

const slow = s => `calc(${s} * var(--sg-slow, 1))`;
const sheet = createSheet(`
  :host { display: inline-block; width: 160px; vertical-align: middle; }
  :host([size="mark"]) { width: 36px; }
  :host([size="glyph"]) { width: 18px; }
  :host([hidden]) { display: none; }
  :host([state="asleep"]) { --sg-slow: 2.857; }
  svg { display: block; width: 100%; height: auto; overflow: visible; }
  .nf { fill: none; }
  .rim { fill: none; stroke: var(--sigil-rim, transparent); }
${paletteCss}
  .root, .tents, .tentacle, .wl, .wr { transform-box: view-box; }
  .root { animation: sg-float ${slow('4.6s')} ease-in-out infinite alternate; }
  .t1 { transform-origin: 238px 362px; animation: sg-ta ${slow('3.4s')} ease-in-out -1.1s infinite alternate; }
  .t2 { transform-origin: 262px 362px; animation: sg-ta ${slow('3.6s')} ease-in-out -2.3s infinite alternate-reverse; }
  .t3 { transform-origin: 230px 356px; animation: sg-tb ${slow('3.9s')} ease-in-out -.5s infinite alternate; }
  .t4 { transform-origin: 270px 356px; animation: sg-tb ${slow('4.1s')} ease-in-out -3.1s infinite alternate-reverse; }
  .t5 { transform-origin: 250px 358px; animation: sg-tc ${slow('4.4s')} ease-in-out -1.8s infinite alternate; }
  .wl { transform-origin: 238px 304px; animation: sg-wl ${slow('2.6s')} ease-in-out infinite alternate; }
  .wr { transform-origin: 262px 304px; animation: sg-wr ${slow('2.6s')} ease-in-out infinite alternate; }
  .ml { animation: sg-m24 4.7s linear infinite; }
  .mc { animation: sg-m36 5.6s linear infinite; }
  .mr { animation: sg-m24 5.1s linear infinite; }
  [data-surge] .tents { transform-origin: 250px 356px; animation: sg-surge .6s cubic-bezier(.22,1,.36,1); }
  @keyframes sg-float { from { transform: translate(0, -5px); } to { transform: translate(0, 5px); } }
  @keyframes sg-ta { from { transform: rotate(-3deg); } to { transform: rotate(3deg); } }
  @keyframes sg-tb { from { transform: rotate(-4deg); } to { transform: rotate(4deg); } }
  @keyframes sg-tc { from { transform: rotate(-2.5deg); } to { transform: rotate(2.5deg); } }
  @keyframes sg-wl { from { transform: rotate(-3deg); } to { transform: rotate(5deg); } }
  @keyframes sg-wr { from { transform: rotate(3deg); } to { transform: rotate(-5deg); } }
  @keyframes sg-m24 { 0%, 92%, 100% { transform: translate(0, -24px); } 96% { transform: translate(0, 0); } }
  @keyframes sg-m36 { 0%, 92%, 100% { transform: translate(0, -36px); } 96% { transform: translate(0, 0); } }
  @keyframes sg-surge { 0% { transform: scale(1); } 30% { transform: scale(1.06, 1.12); } 100% { transform: scale(1); } }
  :host([state="asleep"]) .mini-lid { animation: none; transform: translate(0, 0); }
  [data-paused] * { animation-play-state: paused; }
  [data-reduced] * { animation: none !important; }
  @media (prefers-reduced-motion: reduce) { * { animation: none !important; } }
  @media (forced-colors: active) { .s-ink, .s-inkd { stroke: CanvasText; } .rim { stroke: transparent; } }
`);

// ---------- Tabla de datos del dibujo ----------
// Nodo: { t: tag, c: clases, a: attrs, k: hijos, r: ref, m: 0 => fuera de mark, ms: trazo fijo en mark,
//         rim: 1 => silueta del halo }
// '@x' en atributos => url(#sg<n>-x).
const S = (t, c, a, x = {}) => ({ t, c, a, ...x });
const P = (d, c, w, a = {}, x = {}) => S('path', c, w == null ? { d, ...a } : { d, 'stroke-width': w, ...a }, x);
const G = (c, a, k, x = {}) => ({ t: 'g', c, a, k, ...x });
const C = (cx, cy, r, c, w) => S('circle', c, w == null ? { cx, cy, r } : { cx, cy, r, 'stroke-width': w });

const SOCKET_D = 'M -92,3 C -60,-47 56,-49 92,-2 C 58,49 -54,49 -92,3 Z';
const LID_SUP_D = 'M -108,-65 L 108,-65 L 108,4 C 54,14 -54,14 -108,4 Z';
const LID_INF_D = 'M -108,65 L 108,65 L 108,-4 C 54,-14 -54,-14 -108,-4 Z';
const DOME_D = 'M -104,9 C -108,-3 -98,-14 -87,-21 C -69,-77 -16,-102 36,-95 C 76,-89 102,-58 106,-15 C 111,-7 106,3 93,1 C 56,-48 -58,-46 -92,3 C -97,7 -101,10 -104,9 Z';
const CUP_D = 'M -98,12 C -88,11 -84,8 -78,13 C -46,51 56,50 92,-2 C 98,-4 103,1 100,9 C 92,55 44,82 -14,79 C -54,77 -82,54 -92,24 C -96,19 -100,16 -98,12 Z';
const CLIPS = {
  socket: SOCKET_D,
  ml: 'M 151,418 Q 160,405 173,411 Q 164,424 151,418 Z',
  mc: 'M 250,407 Q 261,422 250,437 Q 239,422 250,407 Z',
  mr: 'M 349,418 Q 340,405 327,411 Q 336,424 349,418 Z'
};
const RIM = { rim: 1 };

const miniEye = (clip, iris, pupil, lid, lidCls, outline, ow) => [
  G('', { 'clip-path': `@${clip}` }, [P(iris, 'f-mini'), { ...pupil, r: 'miniPupil' }, S('rect', lidCls, lid)]),
  P(outline, 'nf s-inkd', ow)
];

const TENTACLES = G('tents', {}, [
  G('tentacle t1', {}, [
    P('M 234,358 C 214,376 196,404 198,434 C 199,447 206,458 216,464 C 211,451 212,436 220,425 C 228,414 238,398 244,372 Z', 'f-abyss s-inkd', 6, {}, RIM),
    P('M 227,374 C 212,390 205,412 206,434', 'nf s-vein', 3.5)
  ]),
  G('tentacle t2', {}, [
    P('M 266,358 C 286,376 304,404 302,434 C 301,447 294,458 284,464 C 289,451 288,436 280,425 C 272,414 262,398 256,372 Z', 'f-abyss s-inkd', 6, {}, RIM),
    P('M 273,374 C 288,390 295,412 294,434', 'nf s-vein', 3.5)
  ]),
  G('tentacle t3', {}, [
    P('M 228,354 C 214,358 202,360 194,355 C 194,347 197,340 201,336 C 188,340 180,350 179,362 C 172,366 165,369 160,366 C 160,359 163,353 166,349 C 155,355 149,366 150,378 C 145,384 142,389 138,388 C 138,396 141,403 145,408 C 139,426 144,452 167,472 C 162,456 163,438 174,425 C 186,411 208,401 232,388 Z', 'f-abyss2 s-inkd', 6.2, {}, RIM),
    P('M 165,465 C 156,447 160,426 175,414 C 190,402 211,394 230,384 L 232,388 C 209,401 186,411 174,425 C 163,438 162,456 167,472 Z', 'f-shade'),
    P('M 218,364 C 201,369 183,377 169,390 C 155,403 148,421 150,439', 'nf s-vein2', 5),
    P('M 210,374 C 194,380 180,389 170,400', 'nf s-inkd', 2.6),
    C(204, 382, 3.2, 'f-pore s-inkd', 2.2), C(187, 392, 2.5, 'f-pore s-inkd', 2),
    ...miniEye('ml', 'M 150,418 Q 160,403 174,411 Q 164,426 150,418 Z', C(162, 414.5, 2.7, 'f-mpupil'),
      { x: 146, y: 399, width: 32, height: 24, transform: 'translate(0, -24)' }, 'f-abyss2 mini-lid ml',
      'M 151,418 Q 160,405 173,411 Q 164,424 151,418 Z', 3.6)
  ]),
  G('tentacle t4', {}, [
    P('M 272,354 C 286,358 298,360 306,355 C 306,347 303,340 299,336 C 312,340 320,350 321,362 C 328,366 335,369 340,366 C 340,359 337,353 334,349 C 345,355 351,366 350,378 C 355,384 358,389 362,388 C 362,396 359,403 355,408 C 361,426 356,452 333,472 C 338,456 337,438 326,425 C 314,411 292,401 268,388 Z', 'f-abyss2 s-inkd', 6.2, {}, RIM),
    P('M 335,465 C 344,447 340,426 325,414 C 310,402 289,394 270,384 L 268,388 C 291,401 314,411 326,425 C 337,438 338,456 333,472 Z', 'f-shade'),
    P('M 282,364 C 299,369 317,377 331,390 C 345,403 352,421 350,439', 'nf s-vein2', 5),
    P('M 290,374 C 306,380 320,389 330,400', 'nf s-inkd', 2.6),
    C(296, 382, 3.2, 'f-pore s-inkd', 2.2), C(313, 392, 2.5, 'f-pore s-inkd', 2),
    ...miniEye('mr', 'M 350,418 Q 340,403 326,411 Q 336,426 350,418 Z', C(338, 414.5, 2.7, 'f-mpupil'),
      { x: 322, y: 399, width: 32, height: 24, transform: 'translate(0, -24)' }, 'f-abyss2 mini-lid mr',
      'M 349,418 Q 340,405 327,411 Q 336,424 349,418 Z', 3.6)
  ]),
  G('tentacle t5', {}, [
    P('M 235,358 L 233,396 C 227,410 225,424 233,438 C 239,456 244,476 250,496 C 256,476 261,456 267,438 C 275,424 273,410 267,396 L 265,358 Z', 'f-lance s-inkd', 6.4, {}, RIM),
    P('M 250,362 L 250,491 C 256,473 261,455 266,437 C 273,423 271,410 265,396 L 263,362 Z', 'f-lance-shade'),
    P('M 240,368 L 238,398 C 233,411 232,423 238,436 L 248,476', 'nf s-vein3', 4),
    ...miniEye('mc', 'M 250,406 Q 262,422 250,438 Q 238,422 250,406 Z',
      S('ellipse', 'f-mpupil', { cx: 250, cy: 422, rx: 2.3, ry: 3.6 }),
      { x: 235, y: 404, width: 30, height: 36, transform: 'translate(0, -36)' }, 'f-lance mini-lid mc',
      'M 250,407 Q 261,422 250,437 Q 239,422 250,407 Z', 4)
  ])
], { r: 'tents' });

const WINGS = G('', {}, [
  G('wl', {}, [
    P('M 238,287 L 184,287 C 178,287 176,292 178,298 L 139,298 C 134,298 133,304 137,309 C 144,317 154,322 169,322 L 238,322 Z', 's-ink', 6, { fill: '@bronze' }, RIM),
    P('M 234,292 L 183,292 M 176,303 L 141,303', 'nf s-wing-hi', 3.2),
    P('M 146,317 L 234,317', 'nf s-wing-low', 4),
    P('M 226,305 C 222,295 206,294 202,304 C 199,312 209,316 215,311 C 218,308 215,303 211,305', 'nf s-engrave', 3.6),
    P('M 196,305 L 182,315 M 178,309 L 152,309 M 174,315 L 158,315', 'nf s-engrave', 3.2)
  ]),
  G('wr', {}, [
    P('M 262,287 L 316,287 C 322,287 324,292 322,298 L 361,298 C 366,298 367,304 363,309 C 356,317 346,322 331,322 L 262,322 Z', 's-ink', 6, { fill: '@bronze' }, RIM),
    P('M 266,292 L 317,292 M 324,303 L 359,303', 'nf s-wing-hi', 3.2),
    P('M 354,317 L 266,317', 'nf s-wing-low', 4),
    P('M 274,305 C 278,295 294,294 298,304 C 301,312 291,316 285,311 C 282,308 285,303 289,305', 'nf s-engrave', 3.6),
    P('M 304,305 L 318,315 M 322,309 L 348,309 M 326,315 L 342,315', 'nf s-engrave', 3.2)
  ])
]);

const NECK = G('', {}, [
  P('M 212,244 C 229,256 235,274 233,294 C 231,307 215,317 215,335 C 215,356 231,369 250,369 C 269,369 285,356 285,335 C 285,317 269,307 267,294 C 265,274 271,256 288,244 Z', 's-ink', 6.2, { fill: '@torso' }, RIM),
  P('M 217,246 C 231,256 236,267 250,272 C 264,267 269,256 283,246 Z', 'f-neck-shade'),
  P('M 241,257 L 250,271 L 258,256', 'nf s-neck-cut', 4),
  P('M 244,274 C 248,292 226,308 225,332 C 224,347 235,358 250,359', 'nf s-spiral-hi', 6.5),
  P('M 256,276 C 258,294 242,306 238,320', 'nf s-copper-deep', 4),
  P('M 251,336 C 256,336 258,341 254,344 C 248,348 240,343 241,335 C 242,324 256,320 264,327 C 273,335 268,352 254,356 C 238,360 224,348 226,331', 'nf s-spiral', 5.2),
  P('M 245,329 C 253,325 262,329 263,338', 'nf s-spiral-glint', 2.8)
]);

const M0 = { m: 0 };
const knob = (s, dy) => G('', {}, [
  S('rect', 'f-knob-bar s-ink', { x: s < 0 ? -136 : 90, y: -2 + dy, width: 46, height: 14, rx: 4, 'stroke-width': 5.6 }),
  S('line', 's-knob-line', { x1: s * 132, y1: 2 + dy, x2: s * 94, y2: 2 + dy, 'stroke-width': 3 }, M0),
  S('ellipse', 'f-knob-ring s-ink', { cx: s * 135, cy: 5 + dy, rx: 6, ry: 15, 'stroke-width': 5 }),
  P(s < 0 ? 'M -138,-18 C -156,-18 -160,28 -138,28 C -132,28 -132,-18 -138,-18 Z'
    : 'M 138,-25 C 156,-25 160,21 138,21 C 132,21 132,-25 138,-25 Z', 's-ink', 5.8, { fill: '@axle' }),
  P(s < 0 ? 'M -146,-9 C -150,-1 -150,11 -146,18' : 'M 145,-16 C 149,-8 149,4 145,11', 'nf s-knob-hi', 3, {}, M0)
]);

const HEAD = [
  G('', {}, [knob(-1, 0), knob(1, -7)], { r: 'axle' }),
  G('', { 'clip-path': '@socket' }, [
    S('rect', 'f-socket', { x: -110, y: -65, width: 220, height: 130 }),
    G('', {}, [
      S('ellipse', 'f-striae-1 s-striae-1e', { cx: 0, cy: 0, rx: 80, ry: 48, 'stroke-width': 4 }),
      S('ellipse', 'f-striae-2 s-striae-2e', { cx: 0, cy: 0, rx: 68, ry: 43, 'stroke-width': 4 }, M0),
      S('ellipse', 'f-striae-3 s-striae-3e', { cx: 0, cy: 0, rx: 57, ry: 39, 'stroke-width': 3.5 }, M0),
      P('M -76,-22 C -68,-6 -68,10 -76,24', 'nf s-groove', 3, {}, M0),
      P('M 76,-24 C 68,-8 68,8 76,22', 'nf s-groove', 3, {}, M0)
    ], { r: 'striae' }),
    G('', {}, [
      S('circle', 's-ink', { cx: 0, cy: 0, r: 47, fill: '@iris', 'stroke-width': 6.2 }, { ms: MARK_MIN }),
      S('circle', 'nf s-iris-ring', { cx: 0, cy: 0, r: 41, 'stroke-width': 3, opacity: 0.65 }, M0),
      G('', {}, [
        // mark: trazo propio de 12 u (no el mínimo de 19) para que el doble anillo se lea sin comerse el iris.
        S('circle', 's-ink', { cx: 0, cy: 0, r: 29.5, fill: '@core', 'stroke-width': 5.8 }, { ms: 12 }),
        S('circle', 'f-pupil', { cx: 0, cy: 0, r: 16.8 }, { r: 'pupil' }),
        { ...C(10.5, -8.2, 5.6, 'f-glint'), m: 0 }
      ], { r: 'core' })
    ], { r: 'iris' }),
    P('M -100,-52 L 100,-52 L 100,-16 C 50,-38 -52,-36 -100,-12 Z', 'f-veil', null, { opacity: 0.34, 'pointer-events': 'none' }),
    G('', { transform: 'translate(0, -56)' }, [
      P(LID_SUP_D, 's-ink', 5.5, { fill: '@sup' }),
      P('M -88,-4 C -40,6 40,6 88,-4', 'nf s-lid-line', 4, {}, M0)
    ], { r: 'lidTop' }),
    G('', { transform: 'translate(0, 56)' }, [
      P(LID_INF_D, 's-ink', 5.5, { fill: '@inf' }),
      P('M -88,4 C -40,-6 40,-6 88,4', 'nf s-lid-line-2', 3.2, {}, M0)
    ], { r: 'lidBottom' })
  ]),
  P(SOCKET_D, 'nf s-ink', 6, {}, RIM),
  G('', {}, [
    P(DOME_D, 's-ink', 6.4, { fill: '@sup' }, RIM),
    P('M -68,-34 C -52,-74 -8,-91 34,-85 C 55,-82 72,-71 82,-55 C 48,-67 -24,-64 -68,-34 Z', 'f-dome-hi'),
    S('ellipse', 'f-dome-spark', { cx: 6, cy: -79, rx: 7.5, ry: 4, transform: 'rotate(8 6 -79)' }, M0),
    S('circle', 'f-dome-spark', { cx: 23, cy: -75, r: 3.2 }, M0),
    P('M 68,-68 C 86,-52 96,-33 99,-14 C 86,-26 70,-36 50,-42 C 62,-50 67,-59 68,-68 Z', 'f-dome-shade'),
    P('M -98,-2 C -58,-58 56,-60 101,-10', 'nf s-ink', 5.2),
    P('M -82,-10 C -46,-49 46,-51 86,-14', 'nf s-dome-arc', 3.6, {}, M0)
  ]),
  G('', {}, [
    P(CUP_D, 's-ink', 6.4, { fill: '@inf' }, RIM),
    P('M -84,16 C -44,61 56,60 95,6', 'nf s-ink', 5),
    P('M -72,18 C -38,53 46,52 84,8', 'nf s-cup-hi', 3.4, {}, M0),
    P('M -64,42 L -46,54 L -34,69 L -16,58 L -2,74 L 16,60 L 31,73 L 47,55 L 62,64 L 74,44 L 86,48', 'nf s-zig', 4.4, {}, M0),
    P('M -45,57 L -35,66 M -15,61 L -3,70 M 17,62 L 29,69 M 48,57 L 59,62', 'nf s-zig-hi', 2.6, {}, M0)
  ])
];

// glyph (16–24 px): 7 formas, misma silueta (cabeza a −20°), masas planas sin degradados.
const GLYPH = [
  P(SOCKET_D, 'f-socket s-ink', 18, {}, RIM),
  G('', { 'clip-path': '@socket' }, [
    G('', {}, [S('circle', 'f-iris-b', { cx: 0, cy: 0, r: 47 }), S('circle', 'f-pupil', { cx: 0, cy: 0, r: 20 }, { r: 'pupil' })], { r: 'iris' }),
    G('', { transform: 'translate(0, -55)' }, [P(LID_SUP_D, 'f-sup-b')], { r: 'lidTop' }),
    G('', { transform: 'translate(0, 55)' }, [P(LID_INF_D, 'f-inf-b')], { r: 'lidBottom' })
  ]),
  P(DOME_D, 'f-sup-b s-ink', 16, {}, RIM),
  P(CUP_D, 'f-inf-b s-ink', 16, {}, RIM)
];

const headOf = kids => G('', { transform: 'translate(250, 178) rotate(-20)' }, kids, { r: 'head' });
const LEVELS = {
  full: { vb: [0, 0, 500, 540], grads: Object.keys(GRADS), clips: Object.keys(CLIPS), nodes: [TENTACLES, WINGS, NECK, headOf(HEAD)], root: 'root' },
  mark: { vb: [80, 46, 340, 246], grads: ['iris', 'core', 'sup', 'inf', 'axle'], clips: ['socket'], nodes: [headOf(HEAD)], k: MARK_K },
  glyph: { vb: [132, 50, 238, 236], grads: [], clips: ['socket'], nodes: [headOf(GLYPH)], noRim: true }
};

function el(tag, attrs = {}) {
  const node = document.createElementNS(NS, tag);
  for (const [name, value] of Object.entries(attrs)) node.setAttribute(name, String(value));
  return node;
}

/** Construye la tabla en `parent`. rim=true => solo siluetas, como trazo del halo por detrás. */
function build(parent, list, ctx, rim = false) {
  for (const n of list) {
    if (ctx.k && n.m === 0) continue;
    const group = n.t === 'g';
    if (rim && !group && !n.rim) continue;
    const node = document.createElementNS(NS, n.t);
    for (const [key, value] of Object.entries(n.a)) {
      if (rim && (key === 'fill' || key === 'clip-path' || key === 'opacity' || key === 'pointer-events')) continue;
      let v = value;
      if (key === 'stroke-width') v = fx((ctx.k && n.ms != null ? n.ms : ctx.sw(value)) + (rim ? 4 * (ctx.k || 1) : 0));
      else if (typeof value === 'string' && value[0] === '@') v = `url(#${ctx.pfx}${value.slice(1)})`;
      node.setAttribute(key, String(v));
    }
    const cls = rim && !group ? 'rim' : n.c;
    if (cls) node.setAttribute('class', cls);
    if (group) {
      build(node, n.k, ctx, rim);
      if (rim && !node.children.length) continue;
    }
    if (n.r) (ctx.refs[n.r] ||= []).push(node);
    parent.append(node);
  }
}

const CURVES = {
  rapido: [190, [[0, 1], [.45, 0], [1, 1]]],
  doble: [390, [[0, 1], [.2, 0], [.45, .85], [.68, 0], [1, 1]]],
  siniestro: [560, [[0, 1], [.3, .45], [.5, 0], [.72, 0], [1, 1]]]
};
const pickCurve = () => { const r = Math.random(); return r < .6 ? 'rapido' : r < .85 ? 'doble' : 'siniestro'; };
const LID_OPEN = 55;

export class DxSigil extends HTMLElement {
  static get observedAttributes() { return ['size', 'state', 'decorative', 'hidden']; }

  constructor() {
    super();
    adoptSheets(this.attachShadow({ mode: 'open' }), sheet);
    this._n = ++nextId;
    this._timers = new Map();
    this._animations = new Map();
    this._raf = 0;
    this._connected = false;
    this._reduced = false;
    this._attending = false;
    this._auto = false;
    this._side = 1;
    this._closed = 0;
    this._point = null;
    this._pending = null;
    this._rect = null;
    this._current = { x: 0, y: 0 };
    this._target = { x: 0, y: 0 };
    this._onPointer = event => {
      if (!this._active() || this._attending) return;
      this._point = { x: event.clientX, y: event.clientY };
      this._pending = this._point; // limitado por el bucle: se procesa una vez por cuadro
      this._lastInput = performance.now();
      this._auto = false;
      this._requestFrame();
    };
    this._onLeave = event => { if (!event.relatedTarget && !this._attending) this.release(); };
    this._onClick = () => { this.blink('rapido'); this._surge(); };
    this._onVisibility = () => this._sync();
    this._onMotion = event => { this._reduced = event.matches; this._sync(); };
    this._onGeometry = () => {
      this._rect = null; // caché invalidada (resize vía ResizeObserver, scroll vía document)
      if (this._point && !this._attending && this._active()) { this._pending = this._point; this._requestFrame(); }
    };
    this._tick = () => {
      this._raf = 0;
      if (!this._active()) return;
      if (this._pending) { const p = this._pending; this._pending = null; this._aim(p.x, p.y); }
      const dx = this._target.x - this._current.x;
      const dy = this._target.y - this._current.y;
      if (Math.abs(dx) < .002 && Math.abs(dy) < .002) {
        this._current = { ...this._target };
        this._paintLook();
        return; // convergió: sin rAF en reposo
      }
      const k = this._auto ? .075 : .16;
      this._current.x += dx * k;
      this._current.y += dy * k;
      this._paintLook();
      this._requestFrame();
    };
    this._render();
  }

  connectedCallback() {
    if (this._connected) return;
    this._connected = true;
    this._media = window.matchMedia('(prefers-reduced-motion: reduce)');
    this._reduced = this._media.matches;
    this._media.addEventListener('change', this._onMotion);
    document.addEventListener('pointermove', this._onPointer, POINTER_OPTS);
    document.addEventListener('pointerout', this._onLeave);
    document.addEventListener('visibilitychange', this._onVisibility);
    document.addEventListener('scroll', this._onGeometry, SCROLL_OPTS);
    if (typeof ResizeObserver === 'function') {
      this._resize = new ResizeObserver(this._onGeometry);
      this._resize.observe(this);
    }
    this.addEventListener('click', this._onClick);
    this._accessibility();
    this._sync();
  }

  disconnectedCallback() {
    this._connected = false;
    this._stop();
    this._svg.setAttribute('data-paused', '');
    this._media?.removeEventListener('change', this._onMotion);
    document.removeEventListener('pointermove', this._onPointer, POINTER_OPTS);
    document.removeEventListener('pointerout', this._onLeave);
    document.removeEventListener('visibilitychange', this._onVisibility);
    document.removeEventListener('scroll', this._onGeometry, SCROLL_OPTS);
    this._resize?.disconnect();
    this._resize = null;
    this.removeEventListener('click', this._onClick);
  }

  attributeChangedCallback(name, oldValue, newValue) {
    if (oldValue === newValue) return;
    if (name === 'decorative') { this._accessibility(); return; }
    if (name === 'size') { this._stop(); this._render(); }
    this._sync();
    if (name === 'state' && (oldValue === 'asleep') !== (newValue === 'asleep')) {
      this._transition(newValue !== 'asleep');
    }
  }

  _accessibility() {
    if (this.hasAttribute('decorative')) {
      this.setAttribute('aria-hidden', 'true');
      this.removeAttribute('role');
      this.removeAttribute('aria-label');
    } else {
      this.removeAttribute('aria-hidden');
      this.setAttribute('role', 'img');
      this.setAttribute('aria-label', 'Dioptra');
    }
  }

  _render() {
    const size = this.getAttribute('size');
    const level = size === 'mark' || size === 'glyph' ? size : 'full';
    const L = LEVELS[level];
    const pfx = `sg${this._n}-`;
    const k = L.k || 0;
    const ctx = { pfx, k, refs: {}, sw: v => (k ? Math.max(v * k, MARK_MIN) : v) };
    const [vx, vy, vw, vh] = L.vb;
    this._eye = [(250 - vx) / vw, (178 - vy) / vh];
    this._rect = null;
    this._svg = el('svg', {
      viewBox: L.vb.join(' '), 'aria-hidden': 'true', focusable: 'false', 'data-paused': '',
      'data-level': level, 'stroke-linecap': 'round', 'stroke-linejoin': 'round'
    });
    const defs = el('defs');
    for (const name of L.grads) {
      const [tag, attrs, stops] = GRADS[name];
      const grad = el(tag, { id: pfx + name, ...attrs });
      for (const [offset, key] of stops) grad.append(el('stop', { offset: `${offset}%`, class: `c-${key}` }));
      defs.append(grad);
    }
    for (const name of L.clips) {
      const clip = el('clipPath', { id: pfx + name });
      clip.append(el('path', { d: CLIPS[name] }));
      defs.append(clip);
    }
    const root = el('g', { class: L.root || 'still' });
    if (!L.noRim) {
      const rimLayer = el('g', { class: 'rim-layer' });
      build(rimLayer, L.nodes, ctx, true);
      root.append(rimLayer);
    }
    build(root, L.nodes, ctx);
    this._svg.append(defs, root);
    this._refs = ctx.refs;
    this._iris = ctx.refs.iris?.[0] ?? null;
    this._pupil = ctx.refs.pupil?.[0] ?? null;
    this._axle = ctx.refs.axle?.[0] ?? null;
    this.shadowRoot.replaceChildren(this._svg);
    this._paintLook();
  }

  _active() {
    return this._connected && !document.hidden && !this.hidden &&
      this.getAttribute('state') !== 'asleep' && !this._reduced;
  }

  _clearTimer(name) {
    clearTimeout(this._timers.get(name));
    this._timers.delete(name);
  }

  _later(name, delay, callback) {
    this._clearTimer(name);
    this._timers.set(name, setTimeout(() => {
      this._timers.delete(name);
      if (this._active()) callback();
    }, delay));
  }

  _stop() {
    cancelAnimationFrame(this._raf);
    this._raf = 0;
    for (const timer of this._timers.values()) clearTimeout(timer);
    this._timers.clear();
    for (const animation of this._animations.values()) animation.cancel();
    this._animations.clear();
  }

  _sync() {
    this._stop();
    const active = this._active();
    const asleep = this.getAttribute('state') === 'asleep';
    // Pausa total solo si no se ve; dormido las animaciones CSS siguen al 35 % (--sg-slow).
    this._svg.toggleAttribute('data-paused', !this._connected || document.hidden || this.hidden);
    this._svg.toggleAttribute('data-reduced', this._reduced);
    this._svg.removeAttribute('data-surge');
    this._closed = asleep ? 1 : 0;
    const rest = asleep && !this._reduced ? { x: 0, y: .1 } : { x: 0, y: 0 };
    this._current = { ...rest };
    this._target = { ...rest };
    this._auto = false;
    this._pending = null;
    this._rect = null;
    this._paintLook();
    if (active) {
      this._lastInput = performance.now();
      if (this._point) { this._pending = this._point; this._requestFrame(); }
      this._scheduleBlink();
      this._scheduleScan();
    }
  }

  _requestFrame() {
    if (!this._raf && this._active()) this._raf = requestAnimationFrame(this._tick);
  }

  /** Puntero (clientX/Y) -> mirada normalizada en el marco de la cabeza (vector rotado +20°). */
  _aim(x, y) {
    if (!this._rect) this._rect = typeof this.getBoundingClientRect === 'function' ? this.getBoundingClientRect() : null;
    const r = this._rect;
    if (!r || !(r.width > 0) || !(r.height > 0)) return;
    const dx = x - (r.left + r.width * this._eye[0]);
    const dy = y - (r.top + r.height * this._eye[1]);
    if (!Number.isFinite(dx) || !Number.isFinite(dy)) return;
    const reach = Math.max(180, r.width * 1.4);
    const clamp = v => Math.max(-1, Math.min(1, v));
    this._target = {
      x: clamp((dx * COS20 - dy * SIN20) / reach),
      y: clamp((dx * SIN20 + dy * COS20) / reach)
    };
    this._auto = false;
  }

  _set(name, value) {
    const list = this._refs?.[name];
    if (list) for (const node of list) node.setAttribute('transform', value);
  }

  _paintLook() {
    if (!this._iris) return;
    const fixed = this._reduced;
    const gx = fixed ? 0 : this._current.x;
    const gy = fixed ? 0 : this._current.y;
    this._set('head', `translate(250 178) rotate(${fx(-20 + gx * 2.6)})`);
    this._set('iris', `translate(${fx(gx * 33)} ${fx(gy * 11)})`);
    this._set('core', `translate(${fx(gx * 6.8)} ${fx(gy * 3.2)})`);
    this._set('striae', `translate(${fx(gx * 33 * .14)} ${fx(gy * 11 * .14)})`);
    this._set('axle', `translate(${fx(gx * 2.8 * .7)} 0)`);
    this._set('pupil', `scale(${this._attending && !fixed ? .85 : 1})`);
    const open = 1 - this._closed;
    this._set('lidTop', `translate(0 ${fx((-LID_OPEN + gy * 4.5) * open)})`);
    this._set('lidBottom', `translate(0 ${fx((LID_OPEN + gy * 4.5) * open)})`);
    this._set('miniPupil', `translate(${fx(gx * 2.2)} ${fx(gy * 1.4)})`);
  }

  _scheduleScan(delay = random(1150, 2600)) {
    this._later('scan', delay, () => {
      const idle = performance.now() - this._lastInput;
      if (!this._attending && idle > 2500) {
        this._side = -this._side;
        this._auto = true;
        this._target = { x: this._side * random(.55, .95), y: random(-.3, .3) };
        this._requestFrame();
      }
      this._scheduleScan();
    });
  }

  _scheduleBlink() {
    this._later('blink', random(2400, 5800), () => {
      this.blink();
      this._scheduleBlink();
    });
  }

  _animate(name, node, frames, duration) {
    this._animations.get(name)?.cancel();
    if (typeof node?.animate !== 'function') return;
    const animation = node.animate(frames, { duration, easing: 'ease-in-out' });
    this._animations.set(name, animation);
    const cleanup = () => {
      if (this._animations.get(name) === animation) this._animations.delete(name);
    };
    animation.onfinish = cleanup;
    animation.oncancel = cleanup;
  }

  _surge() {
    if (!this._active() || !this._refs.tents) return;
    this._svg.setAttribute('data-surge', '');
    this._later('surge', 620, () => this._svg.removeAttribute('data-surge'));
  }

  /** Parpadeo: 'rapido' (190 ms), 'doble' (390 ms) o 'siniestro' (560 ms); sin argumento, al azar. */
  blink(curve) {
    if (!this._active()) return;
    const [duration, points] = CURVES[CURVES[curve] ? curve : pickCurve()];
    const gy = this._current.y * 4.5;
    for (const [name, base] of [['lidTop', -LID_OPEN], ['lidBottom', LID_OPEN]]) {
      const open = base + gy;
      this._animate(name, this._refs[name]?.[0],
        points.map(([offset, f]) => ({ transform: `translate(0px, ${fx(open * f)}px)`, offset })), duration);
    }
  }

  /** Coordenadas clientX/clientY (píxeles CSS relativos al viewport). */
  attend(x, y) {
    if (!Number.isFinite(x) || !Number.isFinite(y)) return;
    this._attending = true;
    this._point = { x, y };
    this._pending = null;
    if (this._active()) {
      this._lastInput = performance.now();
      this._aim(x, y);
      this._requestFrame();
    }
    this._paintLook();
  }

  /** Libera la atención, centra suavemente y vuelve al seguimiento/escaneo normal. */
  release() {
    this._attending = false;
    this._point = null;
    this._pending = null;
    this._auto = false;
    this._target = { x: 0, y: 0 };
    this._paintLook();
    if (this._active()) {
      this._lastInput = performance.now();
      this._requestFrame();
    }
  }

  /** Respuesta expresiva al recibir un reporte: parpadeo doble + impulso de tentáculos. */
  celebrate() {
    if (!this._active()) return;
    this._scheduleBlink();
    this.blink('doble');
    this._surge();
  }

  wake() { this.setAttribute('state', 'awake'); }
  sleep() { this.setAttribute('state', 'asleep'); }

  _transition(awake) {
    if (!this._connected || document.hidden || this.hidden || this._reduced) return;
    for (const [name, sign] of [['lidTop', -1], ['lidBottom', 1]]) {
      const t = v => `translate(0px, ${sign * v}px)`;
      const frames = awake ? [
        { transform: t(0) }, { transform: t(4), offset: .12 }, { transform: t(60), offset: .55 },
        { transform: t(LID_OPEN), offset: .67 }, { transform: t(0), offset: .82 }, { transform: t(LID_OPEN) }
      ] : [{ transform: t(LID_OPEN) }, { transform: t(0) }];
      this._animate(name, this._refs[name]?.[0], frames, awake ? 420 : 200);
    }
  }
}

if (!customElements.get('dx-sigil')) customElements.define('dx-sigil', DxSigil);
