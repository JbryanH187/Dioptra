// ==========================================================================
// DioptraEngine Viewer — ui/app.js
// Punto de entrada productivo: conecta <dx-app> a la API Go, SSE (/api/events)
// y persistencia bidireccional de casillas (PATCH /api/reports/{id}/check).
// ==========================================================================

import './components/index.js';

const EMPTY_WELCOME_ENVELOPE = {
  v: 1,
  report: {
    id: 'welcome',
    project: 'dioptra',
    title: 'DioptraEngine listo y esperando reportes',
    ts: new Date().toISOString(),
    blocks: [
      {
        t: 'prose',
        title: 'Servidor Local Activo',
        md: [
          'El almacén local está inicializado y conectado en vivo por **Server-Sent Events (`SSE`)**. Puedes publicar tu primer reporte técnico de cualquiera de estas dos formas:',
          '- Ejecutando `dioptra open ruta/al/reporte.md` desde tu terminal.',
          '- Invocando la herramienta MCP `publish_report` desde cualquier agente de consola conectado a `/mcp` o `dioptra mcp`.'
        ].join('\n')
      },
      {
        t: 'callout',
        sev: 'info',
        title: 'Actualización instantánea sin recargar página',
        body: 'En cuanto llegue el primer documento, aparecerá automáticamente en la barra lateral izquierda y se abrirá en este lienzo.'
      }
    ]
  }
};

const app = document.getElementById('app');
const knownReportIds = new Set();
const knownRevisions = new Map();
const inFlightPublishIds = new Set();
const bootStartedAt = Date.now();
let lastTimelineSignature = '';
let displayRequestSeq = 0;
let syncInFlight = null;
// Estado del SSE en ESTA pestaña: con el stream sano el sondeo de respaldo pasa de 5 s a 60 s.
let sseHealthy = false;
let lastSyncAt = 0;
const liveChannel = typeof BroadcastChannel === 'function' ? new BroadcastChannel('dioptra-live') : null;

function getRequestedReportId() {
  try {
    return new URLSearchParams(window.location.search).get('report') || '';
  } catch {
    return '';
  }
}

function syncUrlReportParam(id) {
  if (!id || id === 'welcome') return;
  try {
    const url = new URL(window.location.href);
    if (url.searchParams.get('report') !== id) {
      url.searchParams.set('report', id);
      window.history.replaceState(null, '', url.toString());
    }
  } catch {
    // Ignorado en entornos sin History API
  }
}

function extractItemsArray(payload) {
  if (Array.isArray(payload?.items)) return payload.items;
  return Array.isArray(payload) ? payload : [];
}

function revisionKey(it) {
  return `${it?.revision || 1}|${it?.updatedAt || ''}|${it?.lastChange || ''}`;
}

function computeItemsSignature(items) {
  return items
    .map((it) => `${it?.id || ''}|${it?.ts || ''}|${it?.updatedAt || ''}|${it?.revision || 1}|${it?.lastChange || ''}|${it?.title || ''}|${it?.maxSev || ''}|${it?.warningsCount ?? 0}`)
    .join('\n');
}

async function fetchJSONNoStore(url) {
  const resp = await fetch(url, {
    cache: 'no-store',
    headers: { Accept: 'application/json' }
  });
  if (!resp.ok) {
    throw new Error(`HTTP ${resp.status} en ${url}`);
  }
  return resp.json();
}

async function fetchReportAndDisplay(id, { preserveScroll = false } = {}) {
  if (!id) return;
  const seq = ++displayRequestSeq;
  const envelope = await fetchJSONNoStore(`/api/reports/${encodeURIComponent(id)}`);
  if (seq !== displayRequestSeq) return;
  app.displayEnvelope(id, envelope, { preserveScroll });
  syncUrlReportParam(id);
}

async function handlePublishedSummary(summary) {
  if (!summary?.id || inFlightPublishIds.has(summary.id)) return;
  const isFresh = !knownReportIds.has(summary.id) || app.activeReportId !== summary.id;
  app.upsertTimelineSummary(summary, true);
  lastTimelineSignature = '';
  if (!isFresh) return;

  inFlightPublishIds.add(summary.id);
  try {
    await fetchReportAndDisplay(summary.id);
    knownReportIds.add(summary.id);
    knownRevisions.set(summary.id, revisionKey(summary));
    app.notifyToast('Nuevo reporte publicado:', summary.title || summary.id);
    document.querySelector('dx-timeline')?.celebrate?.();
  } finally {
    inFlightPublishIds.delete(summary.id);
  }
}

async function handleUpdatedSummary(summary, source) {
  if (!summary?.id) return;
  const isPublishUpdate = source === 'publish';
  const nextRevKey = revisionKey(summary);
  const changed = knownRevisions.get(summary.id) !== nextRevKey;

  knownReportIds.add(summary.id);
  knownRevisions.set(summary.id, nextRevKey);
  app.upsertTimelineSummary(summary, isPublishUpdate);
  lastTimelineSignature = '';

  if (source === 'check') return;
  if (isPublishUpdate) {
    const sameReport = app.activeReportId === summary.id;
    await fetchReportAndDisplay(summary.id, { preserveScroll: sameReport });
    if (changed) {
      const revTag = Number(summary.revision) > 1 ? ` (rev ${summary.revision})` : '';
      app.notifyToast(`Reporte actualizado${revTag}:`, summary.lastChange || summary.title || summary.id);
    }
    return;
  }
  if (app.activeReportId === summary.id) {
    await fetchReportAndDisplay(summary.id, { preserveScroll: true });
  }
}

async function syncReportsFromServer({ autoOpenNew = true } = {}) {
  if (!app) return;
  if (syncInFlight) return syncInFlight;

  syncInFlight = (async () => {
    try {
      const payload = await fetchJSONNoStore('/api/reports?limit=200');
      const items = extractItemsArray(payload);
      if (items.length === 0) return;

      const newItems = items.filter((it) => it?.id && !knownReportIds.has(it.id) && !inFlightPublishIds.has(it.id));
      const updatedItems = items.filter((it) => {
        if (!it?.id || !knownReportIds.has(it.id) || inFlightPublishIds.has(it.id)) return false;
        const prev = knownRevisions.get(it.id);
        return Boolean(prev) && prev !== revisionKey(it) && Number(it.revision) > 1;
      });
      const sig = computeItemsSignature(items);

      if (autoOpenNew && (newItems.length > 0 || updatedItems.length > 0)) {
        const candidates = newItems.length > 0 ? newItems : updatedItems;
        const isUpdateOnly = newItems.length === 0;
        candidates.sort((a, b) => new Date(b.updatedAt || b.ts).getTime() - new Date(a.updatedAt || a.ts).getTime());
        const target = candidates[0];
        if (!inFlightPublishIds.has(target.id)) {
          inFlightPublishIds.add(target.id);
          try {
            const sameReport = app.activeReportId === target.id;
            app.setTimelineSummaries(items, target.id);
            lastTimelineSignature = sig;
            await fetchReportAndDisplay(target.id, { preserveScroll: isUpdateOnly && sameReport });
            for (const it of items) {
              if (it?.id) {
                knownReportIds.add(it.id);
                knownRevisions.set(it.id, revisionKey(it));
              }
            }
            if (isUpdateOnly) {
              app.notifyToast(`Reporte actualizado (rev ${target.revision}):`, target.lastChange || target.title || target.id);
            } else {
              app.notifyToast('Nuevo reporte publicado:', target.title || target.id);
              document.querySelector('dx-timeline')?.celebrate?.();
            }
          } finally {
            inFlightPublishIds.delete(target.id);
          }
        }
        return;
      }

      for (const it of items) {
        if (it?.id) {
          knownReportIds.add(it.id);
          knownRevisions.set(it.id, revisionKey(it));
        }
      }
      if (sig !== lastTimelineSignature) {
        const keepId = app.activeReportId && items.some((it) => it.id === app.activeReportId)
          ? app.activeReportId
          : items[0].id;
        app.setTimelineSummaries(items, keepId);
        lastTimelineSignature = sig;
        if (!app.activeReportId || app.activeReportId === 'welcome') {
          await fetchReportAndDisplay(keepId);
        }
      }
    } catch (err) {
      console.warn('Dioptra: error sincronizando reportes:', err);
    } finally {
      syncInFlight = null;
    }
  })();

  return syncInFlight;
}

async function loadWorkspaceMeta() {
  if (!app) return;
  try {
    const payload = await fetchJSONNoStore('/api/projects');
    const projects = extractItemsArray(payload);
    app.setWorkspaceMeta?.(projects);
  } catch {
    // Sin servidor o en offline: el timeline sigue funcionando por nombre crudo.
  }
}

// Carga inicial aislada: reinvocable desde el «Reintentar» del timeline sin
// duplicar listeners ni la suscripción en vivo (que viven en bootstrap()).
async function loadReports() {
  let listReady = false;
  try {
    app.setTimelineState?.('loading');
    const [payload] = await Promise.all([
      fetchJSONNoStore('/api/reports'),
      loadWorkspaceMeta()
    ]);
    const items = extractItemsArray(payload);
    app.setTimelineState?.('ready');
    listReady = true;

    if (items.length === 0) {
      app.setTimelineSummaries([], '');
      app.displayEnvelope('', EMPTY_WELCOME_ENVELOPE);
    } else {
      for (const it of items) {
        if (it?.id) {
          knownReportIds.add(it.id);
          knownRevisions.set(it.id, revisionKey(it));
        }
      }
      const requested = getRequestedReportId();
      const initialId = requested && items.some((it) => it.id === requested)
        ? requested
        : items[0].id;
      app.setTimelineSummaries(items, initialId);
      lastTimelineSignature = computeItemsSignature(items);
      await fetchReportAndDisplay(initialId);
    }
  } catch (err) {
    app.notifyToast('Error de conexión con servidor local:', String(err.message || err));
    // Solo si falló la lista; un fallo del detalle no debe ocultar el timeline.
    if (!listReady) app.setTimelineState?.('error', { onRetry: () => loadReports() });
  }
}

function getRequestedCanvasParams() {
  try {
    const sp = new URLSearchParams(window.location.search);
    const canvas = sp.get('canvas') || '';
    const project = sp.get('project') || '';
    return canvas ? { project, canvas } : null;
  } catch {
    return null;
  }
}

async function bootstrap() {
  if (!app) return;

  await loadReports();
  const reqCanvas = getRequestedCanvasParams();
  if (reqCanvas && reqCanvas.project) {
    app.openDesignCanvas?.(reqCanvas.project, reqCanvas.canvas);
  }

  // Selección manual desde <dx-timeline>
  app.addEventListener('dx-select-report', async (e) => {
    const id = e.detail?.id;
    if (!id) return;
    if (e.detail && typeof e.detail === 'object') e.detail.handled = true;
    try {
      await fetchReportAndDisplay(id);
    } catch (err) {
      app.notifyToast('Error al cargar reporte:', String(err.message || err));
    }
  });

  // Persistencia bidireccional de casillas de <dx-action-checklist> -> SQLite + archivo .md
  app.addEventListener('dx-check', async (e) => {
    const activeId = app.activeReportId;
    const checkId = e.detail?.id;
    const done = Boolean(e.detail?.done);
    if (!activeId || !checkId) return;

    try {
      const resp = await fetch(`/api/reports/${encodeURIComponent(activeId)}/check`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({ id: checkId, done })
      });
      if (!resp.ok) {
        throw new Error(`HTTP ${resp.status}`);
      }
    } catch (err) {
      app.notifyToast('No se pudo guardar la casilla:', String(err.message || err));
    }
  });

  // Suscripción en vivo (SSE con reconexión + resync al despertar/enfocar + canal multi-pestaña)
  connectLiveUpdates();
}

function startLocalSSEStream() {
  if (typeof EventSource !== 'function') return () => {};

  let es = null;
  let closed = false;
  let hasOpenedOnce = false;
  let retryDelayMs = 1500;
  let reconnectTimer = null;

  const scheduleReconnect = () => {
    if (closed || reconnectTimer) return;
    const delay = retryDelayMs;
    retryDelayMs = Math.min(retryDelayMs * 2, 10000);
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      openStream();
    }, delay);
  };

  const openStream = () => {
    if (closed) return;
    if (es) {
      try { es.close(); } catch { /* noop */ }
    }
    const stream = new EventSource('/api/events');
    es = stream;

    stream.addEventListener('open', () => {
      sseHealthy = true;
      retryDelayMs = 1500;
      if (hasOpenedOnce || Date.now() - bootStartedAt > 1500) {
        liveChannel?.postMessage({ type: 'sse_reconnected' });
        void syncReportsFromServer({ autoOpenNew: true });
      }
      hasOpenedOnce = true;
    });

    stream.addEventListener('report_published', async (ev) => {
      try {
        const data = JSON.parse(ev.data);
        const summary = data?.summary || data;
        if (!summary?.id) return;
        liveChannel?.postMessage({ type: 'report_published', summary });
        await handlePublishedSummary(summary);
      } catch (err) {
        console.warn('Dioptra: error procesando report_published:', err);
        void syncReportsFromServer({ autoOpenNew: true });
      }
    });

    stream.addEventListener('report_updated', async (ev) => {
      try {
        const data = JSON.parse(ev.data);
        const summary = data?.summary || data;
        if (!summary?.id) return;
        liveChannel?.postMessage({ type: 'report_updated', summary, source: data.source });
        await handleUpdatedSummary(summary, data.source);
      } catch (err) {
        console.warn('Dioptra: error procesando report_updated:', err);
      }
    });

    stream.addEventListener('workspaces_updated', async (ev) => {
      try {
        const data = JSON.parse(ev.data || '{}');
        liveChannel?.postMessage({ type: 'workspaces_updated', summary: data?.summary || '' });
        await loadWorkspaceMeta();
        if (data?.summary) app.notifyToast('Workspaces actualizados:', data.summary);
      } catch {
        void loadWorkspaceMeta();
      }
    });

    stream.addEventListener('canvas_updated', (ev) => {
      try {
        const data = JSON.parse(ev.data || '{}');
        liveChannel?.postMessage({ type: 'canvas_updated', detail: data });
        app.notifyCanvasSSE?.('canvas_updated', data);
      } catch {
        // noop
      }
    });

    stream.addEventListener('canvas_annotations_updated', (ev) => {
      try {
        const data = JSON.parse(ev.data || '{}');
        liveChannel?.postMessage({ type: 'canvas_annotations_updated', detail: data });
        app.notifyCanvasSSE?.('canvas_annotations_updated', data);
      } catch {
        // noop
      }
    });

    stream.addEventListener('error', () => {
      sseHealthy = false;
      if (closed) return;
      if (stream.readyState === EventSource.CLOSED) {
        try { stream.close(); } catch { /* noop */ }
        if (es === stream) es = null;
        scheduleReconnect();
      }
    });
  };

  openStream();

  return () => {
    closed = true;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (es) {
      try { es.close(); } catch { /* noop */ }
      es = null;
    }
  };
}

function connectLiveUpdates() {
  if (liveChannel) {
    liveChannel.addEventListener('message', async (ev) => {
      const msg = ev.data;
      if (!msg || typeof msg !== 'object') return;
      try {
        if (msg.type === 'report_published' && msg.summary?.id) {
          await handlePublishedSummary(msg.summary);
        } else if (msg.type === 'report_updated' && msg.summary?.id) {
          await handleUpdatedSummary(msg.summary, msg.source);
        } else if (msg.type === 'sse_reconnected') {
          await Promise.all([loadWorkspaceMeta(), syncReportsFromServer({ autoOpenNew: true })]);
        } else if (msg.type === 'workspaces_updated') {
          await loadWorkspaceMeta();
        } else if (msg.type === 'canvas_updated') {
          app.notifyCanvasSSE?.('canvas_updated', msg.detail);
        } else if (msg.type === 'canvas_annotations_updated') {
          app.notifyCanvasSSE?.('canvas_annotations_updated', msg.detail);
        }
      } catch (err) {
        console.warn('Dioptra: error procesando mensaje multi-pestaña:', err);
        void syncReportsFromServer({ autoOpenNew: true });
      }
    });
  }

  // Una sola pestaña líder sostiene la conexión SSE cuando Web Locks está disponible
  // para no agotar el límite HTTP/1.1 de 6 conexiones por origen al abrir múltiples reportes.
  let stopLeaderSSE = null;
  let releaseLeaderLock = null;
  if (navigator?.locks?.request && liveChannel) {
    navigator.locks.request('dioptra-sse-leader', () => new Promise((resolve) => {
      releaseLeaderLock = resolve;
      stopLeaderSSE = startLocalSSEStream();
    })).catch(() => {
      stopLeaderSSE = startLocalSSEStream();
    });
  } else {
    stopLeaderSSE = startLocalSSEStream();
  }

  const cleanup = () => {
    if (stopLeaderSSE) {
      stopLeaderSSE();
      stopLeaderSSE = null;
    }
    if (releaseLeaderLock) {
      releaseLeaderLock();
      releaseLeaderLock = null;
    }
  };
  window.addEventListener('pagehide', cleanup);
  window.addEventListener('beforeunload', cleanup);

  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') {
      void syncReportsFromServer({ autoOpenNew: true });
    }
  });
  window.addEventListener('focus', () => {
    void syncReportsFromServer({ autoOpenNew: true });
  });
  window.addEventListener('pageshow', (e) => {
    if (e.persisted) {
      void syncReportsFromServer({ autoOpenNew: true });
    }
  });

  // Respaldo mientras la pestaña está visible: cada 5 s si el SSE no está sano en
  // esta pestaña; si lo está, solo una red de seguridad cada 60 s (socket congelado).
  setInterval(() => {
    if (document.visibilityState !== 'visible') return;
    if (sseHealthy && Date.now() - lastSyncAt < 60000) return;
    lastSyncAt = Date.now();
    void syncReportsFromServer({ autoOpenNew: true });
  }, 5000);
}

bootstrap();
