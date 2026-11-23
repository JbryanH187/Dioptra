// ==========================================================================
// DioptraEngine Viewer — ui/components/dx-sessions.js
// <dx-sessions>: sesiones abiertas de Genius Shell / Gentle Shell (adaptación de
// Gentle Tower al estilo Reliquia). Lee GET /api/sessions; quien te necesita
// (estado "blocked") aparece primero y resaltado. Clickeable para ubicar y
// filtrar su workspace correspondiente en Dioptra. Light DOM (estilos en shell.css).
// ==========================================================================

import { clearNode, el } from './shared.js';

const POLL_MS = 6000;
const STATE_LABEL = {
  blocked: 'Te necesita',
  working: 'Trabajando',
  idle: 'En espera',
};

function relativeAge(iso) {
  const ms = Date.now() - new Date(iso).getTime();
  if (!Number.isFinite(ms) || ms < 0) return '';
  const s = Math.round(ms / 1000);
  if (s < 60) return `hace ${s}s`;
  const m = Math.round(s / 60);
  if (m < 60) return `hace ${m} min`;
  return `hace ${Math.round(m / 60)} h`;
}

export class DxSessions extends HTMLElement {
  constructor() {
    super();
    this._timer = null;
    this._sig = '';
    this._list = [];
    this._activeProject = '';
    this._open = true;
    this._onVisibility = () => {
      if (document.visibilityState === 'visible') this._load();
    };
  }

  connectedCallback() {
    this.hidden = true;
    this._load();
    this._timer = setInterval(() => {
      if (document.visibilityState === 'visible') this._load();
    }, POLL_MS);
    document.addEventListener('visibilitychange', this._onVisibility);
  }

  disconnectedCallback() {
    clearInterval(this._timer);
    document.removeEventListener('visibilitychange', this._onVisibility);
  }

  setActiveProject(projectKey) {
    const next = String(projectKey || '').trim().toLowerCase();
    if (next === this._activeProject) return;
    this._activeProject = next;
    if (this._list.length) this._render(this._list);
  }

  async _load() {
    try {
      const resp = await fetch('/api/sessions', { cache: 'no-store', headers: { Accept: 'application/json' } });
      if (!resp.ok) return;
      const data = await resp.json();
      const list = Array.isArray(data.sessions) ? data.sessions : [];
      const sig = list
        .map((s) => `${s.session}|${s.state}|${s.task || ''}|${s.sessionTitle || ''}|${s.canonicalProject || ''}|${s.parentWorkspace || ''}|${s.stale}`)
        .join('\n');
      this._list = list;
      if (sig === this._sig) {
        this._refreshAges(list);
        return;
      }
      this._sig = sig;
      this._render(list);
    } catch {
      // Sin servidor o sin red local: el panel simplemente no aparece.
    }
  }

  _refreshAges(list) {
    const ages = this.querySelectorAll('.dx-session-age');
    ages.forEach((node, i) => {
      if (list[i]) node.textContent = relativeAge(list[i].updatedAt);
    });
  }

  _render(list) {
    clearNode(this);
    this.hidden = list.length === 0;
    if (!list.length) return;

    const needsYou = list.filter((s) => s.state === 'blocked' && !s.stale).length;
    const toggle = el('button', {
      type: 'button',
      className: 'dx-sessions-toggle',
      'aria-expanded': String(this._open),
    },
    el('span', { className: 'dx-sessions-title' }, 'Sesiones'),
    el('span', { className: 'dx-sessions-count' }, String(list.length)),
    needsYou ? el('span', { className: 'dx-sessions-alert', title: 'Sesiones esperando tu respuesta' }, `? ${needsYou}`) : null);
    toggle.addEventListener('click', () => {
      this._open = !this._open;
      toggle.setAttribute('aria-expanded', String(this._open));
      listEl.hidden = !this._open;
    });

    const listEl = el('ul', { className: 'dx-sessions-list', 'aria-label': 'Sesiones de agentes abiertas' });
    listEl.hidden = !this._open;
    for (const s of list) {
      const state = s.stale ? 'stale' : s.state;
      const label = s.stale ? 'Sin actividad' : (STATE_LABEL[s.state] || s.state);
      const targetProject = s.canonicalProject || s.project || 'proyecto';
      const displayProject = s.parentWorkspace && s.parentWorkspace !== targetProject
        ? `${s.parentWorkspace} · ${targetProject.replace(new RegExp(`^${s.parentWorkspace}-`), '')}`
        : (s.project || targetProject);
      const subtitle = s.task || s.sessionTitle || label;
      const isActive = Boolean(
        this._activeProject &&
        (this._activeProject === targetProject.toLowerCase() ||
          (s.parentWorkspace && this._activeProject === s.parentWorkspace.toLowerCase()))
      );

      const btn = el('button', {
        type: 'button',
        className: 'dx-session',
        'aria-pressed': isActive ? 'true' : 'false',
        dataset: { state, active: isActive ? 'true' : 'false' },
        title: `${s.agent || 'agente'} · ${s.cwd || ''} · clic para filtrar workspace`,
      },
      el('span', { className: 'dx-session-dot', 'aria-hidden': 'true' }),
      el('span', { className: 'dx-session-main' },
        el('span', { className: 'dx-session-project' }, displayProject),
        el('span', { className: 'dx-session-task' }, subtitle)),
      el('span', { className: 'dx-session-meta' },
        el('span', { className: 'dx-session-state' }, label),
        el('span', { className: 'dx-session-age' }, relativeAge(s.updatedAt))));

      btn.addEventListener('click', () => {
        this.dispatchEvent(new CustomEvent('dx-focus-workspace', {
          bubbles: true,
          composed: true,
          detail: {
            project: targetProject,
            parentWorkspace: s.parentWorkspace || '',
            label: displayProject,
            session: s.session,
            cwd: s.cwd || '',
          }
        }));
      });

      listEl.appendChild(el('li', { className: 'dx-session-row' }, btn));
    }
    this.appendChild(toggle);
    this.appendChild(listEl);
  }
}

if (!customElements.get('dx-sessions')) {
  customElements.define('dx-sessions', DxSessions);
}
