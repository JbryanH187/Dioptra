// ==========================================================================
// DioptraEngine Viewer — ui/preview.js
// Banco de pruebas autónomo: 4 entradas históricas en <dx-timeline> y reporte
// activo exhaustivo que ejercita los 9 componentes, degradación ADR 1,
// las 3 rutas de diagramas con pan/zoom y simulador interactivo en free-canvas.
// ==========================================================================

import './components/index.js';

// SVG limpio precompilado (simulando salida de Go para kind:"flow", §4.6 Ruta A)
// Usa exclusivamente elementos permitidos y clases temáticas dg-* sin fill/stroke inline.
const PRECOMPILED_FLOW_SVG = `
<svg viewBox="0 0 880 250" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <marker id="go-flow-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
      <path d="M 0 1.5 L 10 5 L 0 8.5 z" class="dg-arrow"/>
    </marker>
  </defs>
  <g class="dg-edge">
    <path d="M 176 125 L 254 125" marker-end="url(#go-flow-arrow)"/>
    <rect x="190" y="101" width="50" height="18" rx="4" class="dg-label-bg"/>
    <text x="215" y="114" text-anchor="middle" class="dg-label">JSON</text>
  </g>
  <g class="dg-edge">
    <path d="M 414 105 C 450 65, 470 65, 504 65" marker-end="url(#go-flow-arrow)"/>
    <rect x="430" y="54" width="58" height="18" rx="4" class="dg-label-bg"/>
    <text x="459" y="67" text-anchor="middle" class="dg-label">válido</text>
  </g>
  <g class="dg-edge">
    <path d="M 414 145 C 450 185, 470 185, 504 185" marker-end="url(#go-flow-arrow)"/>
    <rect x="424" y="174" width="68" height="18" rx="4" class="dg-label-bg"/>
    <text x="458" y="187" text-anchor="middle" class="dg-label">fallo ADR1</text>
  </g>
  <g class="dg-edge">
    <path d="M 664 185 C 695 185, 695 95, 664 75" marker-end="url(#go-flow-arrow)"/>
  </g>
  <g class="dg-edge">
    <path d="M 664 65 L 724 125" marker-end="url(#go-flow-arrow)"/>
  </g>

  <g class="dg-node dg-sev-info">
    <title>Agente Maquetador (Contrato B)</title>
    <rect x="24" y="105" width="152" height="40" rx="8"/>
    <text x="100" y="130" text-anchor="middle" class="dg-node-label">Agente Maquetador</text>
  </g>
  <g class="dg-node">
    <title>Validador Tolerante Go</title>
    <rect x="258" y="105" width="156" height="40" rx="8"/>
    <text x="336" y="130" text-anchor="middle" class="dg-node-label">Validador Go MCP</text>
  </g>
  <g class="dg-node dg-sev-success">
    <title>Enriquecedor Chroma + Diff + AST</title>
    <rect x="508" y="45" width="156" height="40" rx="8"/>
    <text x="586" y="70" text-anchor="middle" class="dg-node-label">Chroma + Diff + SVG</text>
  </g>
  <g class="dg-node dg-sev-warning">
    <title>Degradador Seguro a Prose o Code</title>
    <rect x="508" y="165" width="156" height="40" rx="8"/>
    <text x="586" y="190" text-anchor="middle" class="dg-node-label">Degradador Seguro</text>
  </g>
  <g class="dg-node dg-sev-info">
    <title>Visor Custom Elements Shadow DOM</title>
    <rect x="728" y="105" width="132" height="40" rx="8"/>
    <text x="794" y="130" text-anchor="middle" class="dg-node-label">Visor Shadow DOM</text>
  </g>
</svg>
`;

// Simulador interactivo real aislado para <dx-free-canvas> (§4.9 Modo V)
// Conserva su estado interno (lotes procesados, sliders, historial) al alternar tema oscuro/claro.
const INTERACTIVE_SIMULATOR_HTML = `
<style>
  .sim-wrap {
    padding: var(--spacing-4);
    display: flex;
    flex-direction: column;
    gap: var(--spacing-4);
  }
  .sim-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--spacing-3);
    flex-wrap: wrap;
  }
  .sim-title {
    margin: 0;
    font-size: var(--fs-subheader);
    font-weight: 600;
  }
  .sim-theme-tag {
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    padding: 3px 10px;
    border-radius: var(--rounded-full);
    background: var(--color-sev-info-bg);
    color: var(--color-sev-info-fg);
    border: 1px solid var(--color-border-translucent);
  }
  .sim-controls {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
    gap: var(--spacing-3);
    padding: var(--spacing-3);
    background: var(--color-surface-base);
    border: 1px solid var(--color-border);
    border-radius: var(--rounded-sm);
  }
  .ctrl {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: var(--fs-caption);
    color: var(--color-text-soft);
  }
  .ctrl strong {
    font-family: var(--font-mono);
    color: var(--color-text-primary);
  }
  input[type="range"] {
    width: 100%;
    accent-color: var(--color-interactive-primary);
  }
  .sim-metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(135px, 1fr));
    gap: var(--spacing-3);
  }
  .metric-box {
    padding: var(--spacing-3);
    border-radius: var(--rounded-sm);
    background: var(--color-surface-base);
    border: 1px solid var(--color-border-translucent);
  }
  .metric-box span {
    display: block;
    font-size: var(--fs-caption);
    color: var(--color-text-soft);
  }
  .metric-box output {
    font-family: var(--font-mono);
    font-size: var(--fs-h3);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .bars {
    display: grid;
    grid-template-columns: repeat(24, 1fr);
    align-items: end;
    gap: 4px;
    height: 76px;
    padding: var(--spacing-2);
    background: var(--color-surface-base);
    border: 1px solid var(--color-border-translucent);
    border-radius: var(--rounded-sm);
  }
  .bar {
    background: var(--color-interactive-primary);
    border-radius: 3px 3px 0 0;
    min-height: 6%;
    transition: height 120ms ease, background-color 120ms ease;
  }
  .bar[data-warn="true"] { background: var(--color-sev-warning); }
  .bar[data-crit="true"] { background: var(--color-sev-error); }
  .sim-actions {
    display: flex;
    align-items: center;
    gap: var(--spacing-2);
    flex-wrap: wrap;
  }
  button.sim-btn {
    height: 32px;
    padding: 0 12px;
    border-radius: var(--rounded-sm);
    border: 1px solid var(--color-border);
    background: var(--color-interactive-secondary);
    color: var(--color-text-primary);
    font-family: inherit;
    font-size: var(--fs-caption);
    font-weight: 600;
    cursor: pointer;
  }
  button.sim-btn.primary {
    background: var(--color-interactive-primary);
    border-color: var(--color-interactive-primary);
    color: var(--color-on-primary);
  }
  .sim-note {
    margin: 0;
    font-size: var(--fs-caption);
    color: var(--color-text-soft);
    font-family: var(--font-mono);
  }
</style>

<div class="sim-wrap">
  <div class="sim-head">
    <h3 class="sim-title">Simulador en Vivo: Cola y Backpressure MCP</h3>
    <span class="sim-theme-tag" id="themeBadge">tema: dark</span>
  </div>

  <div class="sim-controls">
    <label class="ctrl">
      <span>Tasa de arribo: <strong id="rpsVal">320</strong> req/s</span>
      <input id="rpsInput" type="range" min="50" max="900" step="10" value="320">
    </label>
    <label class="ctrl">
      <span>Workers Go: <strong id="workersVal">6</strong> goroutines</span>
      <input id="workersInput" type="range" min="1" max="16" step="1" value="6">
    </label>
    <label class="ctrl">
      <span>Payload medio: <strong id="kbVal">24</strong> KB</span>
      <input id="kbInput" type="range" min="4" max="160" step="4" value="24">
    </label>
  </div>

  <div class="sim-metrics">
    <div class="metric-box"><span>Lotes inyectados</span><output id="batchesOut">1</output></div>
    <div class="metric-box"><span>Utilización Pool</span><output id="utilOut">0%</output></div>
    <div class="metric-box"><span>Latencia P95 est.</span><output id="latOut">0 ms</output></div>
    <div class="metric-box"><span>Cambios de tema</span><output id="themeSwitchesOut">0</output></div>
  </div>

  <div class="bars" id="sparkBars" aria-label="Historial de utilización"></div>

  <div class="sim-actions">
    <button type="button" class="sim-btn primary" id="burstBtn" onclick="window.injectBurst(10)">+10 Ráfaga de lotes</button>
    <button type="button" class="sim-btn" id="stepBtn" onclick="window.injectBurst(1)">+1 Lote manual</button>
    <button type="button" class="sim-btn" id="resetBtn" onclick="window.resetSim()">Reiniciar contador</button>
    <p class="sim-note" id="sandboxProbe">Aislamiento: verificando sandbox…</p>
  </div>
</div>

<script>
  const state = {
    batches: 1,
    themeSwitches: 0,
    history: [28, 34, 31, 40, 38, 45, 42, 39, 44, 51, 48, 46, 52, 49, 43, 47, 53, 50, 44, 48, 55, 52, 49, 51]
  };

  const rpsInput = document.getElementById("rpsInput");
  const workersInput = document.getElementById("workersInput");
  const kbInput = document.getElementById("kbInput");
  const barsContainer = document.getElementById("sparkBars");

  for (let i = 0; i < 24; i++) {
    const b = document.createElement("div");
    b.className = "bar";
    barsContainer.appendChild(b);
  }

  function computeMetrics() {
    const rps = Number(rpsInput.value);
    const workers = Number(workersInput.value);
    const kb = Number(kbInput.value);
    document.getElementById("rpsVal").textContent = String(rps);
    document.getElementById("workersVal").textContent = String(workers);
    document.getElementById("kbVal").textContent = String(kb);

    const capacity = workers * (110 - Math.min(75, kb * 0.35));
    const util = Math.min(100, Math.round((rps / Math.max(1, capacity)) * 100));
    const p95 = (1.8 + (kb * 0.045) + (util > 75 ? (util - 75) * 0.9 : util * 0.05)).toFixed(1);

    document.getElementById("batchesOut").textContent = String(state.batches);
    document.getElementById("utilOut").textContent = util + "%";
    document.getElementById("latOut").textContent = p95 + " ms";
    document.getElementById("themeSwitchesOut").textContent = String(state.themeSwitches);
    document.getElementById("themeBadge").textContent = "tema: " + (document.documentElement.dataset.theme || "dark");

    const children = barsContainer.children;
    for (let i = 0; i < children.length; i++) {
      const val = state.history[i] || 10;
      children[i].style.height = Math.max(6, Math.min(100, val)) + "%";
      children[i].dataset.warn = val >= 70 && val < 88 ? "true" : "false";
      children[i].dataset.crit = val >= 88 ? "true" : "false";
    }
    return util;
  }

  window.injectBurst = function(delta) {
    state.batches += delta;
    const baseUtil = computeMetrics();
    const spike = Math.min(100, baseUtil + Math.min(32, delta * 2.5));
    state.history.shift();
    state.history.push(spike);
    computeMetrics();
  };

  window.resetSim = function() {
    state.batches = 0;
    computeMetrics();
  };

  [rpsInput, workersInput, kbInput].forEach((inp) => {
    inp.addEventListener("input", () => {
      const u = computeMetrics();
      state.history.shift();
      state.history.push(u);
      computeMetrics();
    });
  });

  // Verificación en vivo de origen opaco del sandbox (sin allow-same-origin)
  let parentBlocked = false;
  try {
    void window.parent.document.title;
  } catch {
    parentBlocked = true;
  }
  document.getElementById("sandboxProbe").textContent = parentBlocked
    ? "Sandbox verificado: origin=null · parent.document bloqueado"
    : "Advertencia: acceso al padre no bloqueado";

  addEventListener("dx-canvas-theme", () => {
    state.themeSwitches += 1;
    computeMetrics();
  });

  computeMetrics();
</script>
`;

const REPORTS = [
  // 1. Reporte Activo Exhaustivo (Hoy — Prueba los 9 componentes + degradación + pan/zoom + free-canvas)
  {
    v: 1,
    _ignoredEnvelopeFlag: true,
    report: {
      id: 'r_dioptra_audit_01',
      project: 'dioptra-core',
      title: 'Auditoría Integral de Seguridad, Latencia MCP y Catálogo UI v1',
      ts: '2026-11-17T22:55:00-07:00',
      blocks: [
        {
          t: 'prose',
          title: 'Resumen Ejecutivo de Arquitectura',
          md: [
            'El motor **DioptraEngine** opera sobre un servidor local en Go (`MCP` + HTTP estático) acoplado a `Custom Elements` nativos con **Shadow DOM** y hojas compartidas `adoptedStyleSheets`. Toda validación estructural ocurre en internal/mcp/validator.go:88 antes de persistir el documento.',
            '- Cero frameworks reactivos y cero uso de inserción HTML insegura fuera del marco aislado en internal/http/canvas.go:41.',
            '- Cumplimiento estricto de contraste **WCAG 2.1 AA/AAA** según [Guía Oficial W3C](https://www.w3.org/TR/WCAG21/) y variables selladas en ui/styles/tokens.css:40.',
            '- Conmutación instantánea entre esquema oscuro técnico y esquema claro **Reliquia Light**.'
          ].join('\n')
        },
        {
          t: 'kpi',
          items: [
            { label: 'Hallazgos Críticos', value: 2, delta: '−3 vs sprint previo', sev: 'error' },
            { label: 'Latencia P99 MCP', value: '4.8', unit: 'ms', delta: '−1.4 ms', sev: 'success' },
            { label: 'Cobertura Schema', value: '98.6', unit: '%', delta: '+4.2%', sev: 'success' },
            { label: 'Avisos Tolerados', value: 3, delta: 'ADR 1 activo', sev: 'warning' },
            { label: 'Peso JS Visor', value: '19.4', unit: 'KB gz', delta: 'límite ≤ 60 KB', sev: 'info' },
            { label: 'Fugas Sandbox', value: 0, unit: 'eventos', delta: 'origin opaco OK' }
          ]
        },
        {
          t: 'callout',
          sev: 'error',
          title: 'Verificación obligatoria de expiración exp y audiencia en tokens de sesión local',
          body: 'El validador anterior aceptaba firmas válidas sin exigir el reclamo `exp` cuando el reloj tenía deriva menor. Se corrigió el flujo en `ParseAndVerify` aplicando margen estricto de **5 segundos** y rechazo inmediato de algoritmos `none`.',
          ref: 'internal/auth/jwt.go:42'
        },
        {
          t: 'callout',
          sev: 'warning',
          title: 'Bloque de prueba degradado intencionalmente para validar tolerancia ADR 1',
          body: 'A continuación se inyecta un bloque `table` malformado (sin arreglo `cols`) y una propiedad extra desconocida (`_traceDebug`). El despachador ignora la clave extra y degrada la tabla rota a `code-view` con su insignia de advertencia visible.',
          ref: 'internal/mcp/validator.go:131'
        },
        // Bloque roto real: `table` sin `cols` -> el despachador lo degrada a `code` (JSON) con insignia visible
        {
          t: 'table',
          _traceDebug: 'propiedad_desconocida_ignorada',
          rows: [
            { servicio: 'legacy-ingestor', estado: 'timeout_parcial', reintentos: 4 },
            { servicio: 'metrics-flusher', estado: 'ok', reintentos: 0 }
          ]
        },
        {
          t: 'table',
          sort: 'sev',
          filter: true,
          cols: [
            { key: 'sev', label: 'Severidad', type: 'sev' },
            { key: 'module', label: 'Módulo / Subsistema', type: 'text' },
            { key: 'p99', label: 'Latencia P99 (ms)', type: 'num' },
            { key: 'allocs', label: 'Allocs / op', type: 'num' },
            { key: 'ref', label: 'Ubicación', type: 'ref' }
          ],
          rows: [
            { sev: 'error', module: 'Validador JWT de transporte MCP', p99: '14.20', allocs: 18, ref: 'internal/auth/jwt.go:42' },
            { sev: 'warning', module: 'Precompilador D2/Graphviz de grafos flow', p99: '9.65', allocs: 42, ref: 'internal/diagram/flow.go:77' },
            { sev: 'warning', module: 'Despachador tolerante de bloques rotos', p99: '1.85', allocs: 6, ref: 'internal/mcp/validator.go:131' },
            { sev: 'info', module: 'Endpoint aislado /canvas/{docId}/{blockId}', p99: '0.92', allocs: 3, ref: 'internal/http/canvas.go:41' },
            { sev: 'info', module: 'Stream de eventos en vivo SSE /api/events', p99: '0.45', allocs: 2, ref: 'internal/http/sse.go:19' },
            { sev: 'success', module: 'Tokenizador Chroma de bloques de código', p99: '3.10', allocs: 11, ref: 'internal/render/chroma.go:54' },
            { sev: 'success', module: 'Sanitizador AST de Markdown seguro', p99: '0.68', allocs: 4, ref: 'internal/render/markdown.go:29' },
            { sev: 'success', module: ' Cabeceras CSP desacopladas sin nonce', p99: '0.12', allocs: 1, ref: 'internal/http/csp.go:15' }
          ]
        },
        {
          t: 'section',
          title: 'Inspección Profunda: Ruta /canvas/... y Comparativa Diff de Cabeceras CSP',
          sev: 'warning',
          open: true,
          blocks: [
            {
              t: 'code',
              lang: 'go',
              ref: 'internal/http/canvas.go:41',
              hl: [4, 5, 11, 12],
              src: [
                'func (s *Server) handleCanvasFrame(w http.ResponseWriter, r *http.Request) {',
                '\tdocID, blockID := r.PathValue("docId"), r.PathValue("blockId")',
                '\thtmlBody, ok := s.store.LookupCanvas(docID, blockID)',
                '\tif !ok {',
                '\t\thttp.NotFound(w, r)',
                '\t\treturn',
                '\t}',
                '\ttheme := normalizeTheme(r.URL.Query().Get("theme"))',
                '\tw.Header().Set("Content-Security-Policy", canvasIsolatedCSP)',
                '\tw.Header().Set("X-Content-Type-Options", "nosniff")',
                '\tw.Header().Set("Cache-Control", "no-store")',
                '\trenderCanvasDocument(w, theme, s.tokensCSS, htmlBody)',
                '}'
              ].join('\n')
            },
            {
              t: 'code',
              lang: 'go',
              ref: 'internal/http/csp.go:15',
              old: [
                'func applyMainCSP(w http.ResponseWriter, nonce string) {',
                '\tpolicy := "default-src \'self\'; script-src \'self\' \'nonce-" + nonce + "\'; style-src \'self\'"',
                '\tw.Header().Set("Content-Security-Policy", policy)',
                '}'
              ].join('\n'),
              src: [
                'func applyMainCSP(w http.ResponseWriter) {',
                '\tconst policy = "default-src \'self\'; script-src \'self\'; style-src \'self\'; connect-src \'self\'; object-src \'none\'; base-uri \'none\'; frame-ancestors \'none\'"',
                '\tw.Header().Set("Content-Security-Policy", policy)',
                '\tw.Header().Set("X-Content-Type-Options", "nosniff")',
                '}'
              ].join('\n')
            }
          ]
        },
        {
          t: 'diagram',
          kind: 'flow',
          svg: PRECOMPILED_FLOW_SVG,
          nodes: [
            { id: 'agent', label: 'Agente Maquetador', sev: 'info' },
            { id: 'val', label: 'Validador Go MCP' },
            { id: 'enrich', label: 'Chroma + Diff + SVG', sev: 'success' },
            { id: 'deg', label: 'Degradador Seguro', sev: 'warning' },
            { id: 'ui', label: 'Visor Shadow DOM', sev: 'info' }
          ],
          edges: [
            ['agent', 'val', 'JSON'],
            ['val', 'enrich', 'válido'],
            ['val', 'deg', 'fallo ADR1'],
            ['deg', 'enrich', 'prose/code'],
            ['enrich', 'ui', 'HTTP + SSE']
          ]
        },
        {
          t: 'diagram',
          kind: 'seq',
          nodes: [
            { id: 'ag', label: 'Agente CLI', sev: 'info' },
            { id: 'go', label: 'Servidor Go MCP', sev: 'success' },
            { id: 'sh', label: 'dx-app Shell' },
            { id: 'cv', label: 'iframe /canvas', sev: 'warning' }
          ],
          edges: [
            ['ag', 'go', 'POST reporte (Contrato B)'],
            ['go', 'go', 'normaliza + extrae html canvas'],
            ['go', 'sh', 'SSE nuevo reporte {id, src}'],
            ['sh', 'cv', 'GET /canvas/r_01/b_9?theme=dark'],
            ['cv', 'sh', 'postMessage {dxCanvasReady:true}'],
            ['sh', 'cv', 'postMessage {dxTheme:"light"}']
          ]
        },
        {
          t: 'diagram',
          kind: 'tree',
          nodes: [
            { id: 'root', label: 'dx-app (Shell)', sev: 'info' },
            { id: 'tl', label: 'dx-timeline' },
            { id: 'rp', label: 'dx-report', sev: 'success' },
            { id: 'c1', label: 'dx-kpi-grid' },
            { id: 'c2', label: 'dx-data-table' },
            { id: 'c3', label: 'dx-collapsible-section', sev: 'warning' },
            { id: 'c4', label: 'dx-free-canvas', sev: 'info' },
            { id: 'c5', label: 'dx-code-view (slot)' }
          ],
          edges: [
            ['root', 'tl', 'sidebar'],
            ['root', 'rp', 'main'],
            ['rp', 'c1', ''],
            ['rp', 'c2', ''],
            ['rp', 'c3', ''],
            ['rp', 'c4', ''],
            ['c3', 'c5', 'light DOM']
          ]
        },
        {
          t: 'checklist',
          items: [
            { id: 'chk-1', text: 'Sellar paleta dual en `ui/styles/tokens.css` con contraste ≥ AA', done: true, owner: '@design-sys', sev: 'success' },
            { id: 'chk-2', text: 'Aislar `dx-free-canvas` sin `allow-same-origin` y sincronizar tema vía `postMessage`', done: true, owner: '@sec-arch', sev: 'success' },
            { id: 'chk-3', text: 'Implementar controles de zoom `0.25x`–`4x` con `Ctrl + rueda` en `ui/components/diagram-block.js:354`', done: true, owner: '@ui-core', sev: 'info' },
            { id: 'chk-4', text: 'Conectar persistencia `POST /api/reports/:id/checks` en el binario Go', done: false, owner: '@backend-go', sev: 'warning' },
            { id: 'chk-5', text: 'Empaquetar fuentes `.woff2` (`Poppins` y `JetBrains Mono`) con `//go:embed`', done: false, owner: '@release', sev: 'info' },
            { id: 'chk-6', text: 'Ejecutar suite de regresión con payload hostil sobre internal/http/canvas.go:41', done: false, owner: '@qa-sec', sev: 'error' }
          ]
        },
        {
          t: 'canvas',
          h: 430,
          html: INTERACTIVE_SIMULATOR_HTML
        }
      ]
    }
  },

  // 2. Entrada Histórica #2 (Hoy temprano — Proyecto payments-api)
  {
    v: 1,
    report: {
      id: 'r_payments_sql_02',
      project: 'payments-api',
      title: 'Perfilado de Consultas SQL y Pool de Conexiones Region US-East',
      ts: '2026-11-17T16:40:00-07:00',
      blocks: [
        {
          t: 'prose',
          title: 'Diagnóstico de Contención en Checkout',
          md: 'Se detectó saturación transitoria de conexiones durante el pico vespertino en `us-east-1`. El ajuste de índice compuesto en db/migrations/042_idx_payments.sql:12 redujo las lecturas secuenciales en un **84%**.'
        },
        {
          t: 'kpi',
          items: [
            { label: 'Tiempo Query P95', value: '11.2', unit: 'ms', delta: 'antes 68.5 ms', sev: 'success' },
            { label: 'Conexiones Activas', value: 14, unit: '/ 32', delta: 'estable', sev: 'info' },
            { label: 'Timeouts Pico', value: 0, delta: '−9 incidentes', sev: 'success' }
          ]
        },
        {
          t: 'callout',
          sev: 'warning',
          title: 'Revisar límite de idle_in_transaction_session_timeout en réplicas locales',
          body: 'Dos workers mantuvieron transacciones abiertas por más de `30s` tras interrupción de red.',
          ref: 'internal/store/postgres.go:94'
        },
        {
          t: 'code',
          lang: 'sql',
          ref: 'db/migrations/042_idx_payments.sql:12',
          hl: [2, 3],
          src: [
            '-- Índice cubriente para consulta de pagos por cuenta y fecha',
            'CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_payments_account_created',
            '  ON ledger.payment_events (account_id, created_at DESC)',
            '  INCLUDE (receipt_id, amount_cents, sync_status);'
          ].join('\n')
        }
      ]
    }
  },

  // 3. Entrada Histórica #3 (Ayer — Proyecto design-tokens)
  {
    v: 1,
    report: {
      id: 'r_tokens_wcag_03',
      project: 'design-tokens',
      title: 'Verificación Matemática WCAG 2.1 sobre Tokens Duales',
      ts: '2026-11-16T19:15:00-07:00',
      blocks: [
        {
          t: 'callout',
          sev: 'success',
          title: 'Correcciones AUD-02, AUD-03 y AUD-06 aprobadas y selladas',
          body: 'Todos los pares de texto normal superan **4.5:1** en ambos esquemas cromáticos. Se adoptó `#60A5FA` para enlaces en oscuro (`7.55:1`) y `#2E8241` para texto de éxito en claro (`4.79:1`).',
          ref: 'ui/styles/tokens.css:56'
        },
        {
          t: 'table',
          sort: 'ratio',
          filter: true,
          cols: [
            { key: 'sev', label: 'Estado', type: 'sev' },
            { key: 'token', label: 'Par Evaluado', type: 'text' },
            { key: 'ratio', label: 'Ratio WCAG', type: 'num' },
            { key: 'ref', label: 'Definición', type: 'ref' }
          ],
          rows: [
            { sev: 'success', token: '--color-text-primary (#F1F5F9) / #0B0F17', ratio: '17.43', ref: 'ui/styles/tokens.css:53' },
            { sev: 'success', token: '--color-accent-fg (#60A5FA) / #0B0F17', ratio: '7.55', ref: 'ui/styles/tokens.css:56' },
            { sev: 'success', token: '--color-on-primary (#FFFFFF) / #3759EB', ratio: '5.54', ref: 'ui/styles/tokens.css:61' },
            { sev: 'success', token: '--color-sev-success-fg (#2E8241) / #FFFFFF', ratio: '4.79', ref: 'ui/styles/tokens.css:169' }
          ]
        }
      ]
    }
  },

  // 4. Entrada Histórica #4 (Fecha anterior — Proyecto dioptra-core)
  {
    v: 1,
    report: {
      id: 'r_parser_bench_04',
      project: 'dioptra-core',
      title: 'Benchmark Inicial del Parser AST y Sanitizador SVG',
      ts: '2026-11-14T11:20:00-07:00',
      blocks: [
        {
          t: 'kpi',
          items: [
            { label: 'Throughput AST', value: '142', unit: 'MB/s', sev: 'info' },
            { label: 'Payloads XSS Bloqueados', value: '64/64', delta: '100% suite fuzz', sev: 'success' }
          ]
        },
        {
          t: 'prose',
          title: 'Notas del Banco de Fuzzing',
          md: 'Se ejecutaron `64` vectores con etiquetas `<script>`, `<foreignObject>` y atributos `onload` sobre internal/diagram/sanitize.go:51; el 100% fue neutralizado antes de llegar al cliente.'
        },
        {
          t: 'checklist',
          readonly: true,
          items: [
            { id: 'b-1', text: 'Bloquear `foreignObject` y `script` en lista blanca SVG', done: true, owner: '@sec-arch', sev: 'success' },
            { id: 'b-2', text: 'Eliminar `width` y `height` rígidos preservando `viewBox`', done: true, owner: '@ui-core', sev: 'info' }
          ]
        }
      ]
    }
  }
];

const app = document.getElementById('app');
if (app) {
  app.loadReports(REPORTS, 'r_dioptra_audit_01');
}
