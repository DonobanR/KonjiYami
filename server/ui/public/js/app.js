/* ═══════════════════════════════════════════════════════════════
   KonjiYami Operations Console — app.js
   Module-based SPA: Sessions | Listeners | Implants | Arsenal
   Sprint 2: served directly from Go server on :8080 (no Node.js)
═══════════════════════════════════════════════════════════════ */

/* ────────────── STATE ────────────── */
const S = {
  mod:       'sessions',
  sessions:  [],
  beacons:   [],
  listeners: [],
  logs:      [],
  jobs:      [],
  payloads:  [],
  username:  '',
  ws: {
    open:    false,
    id:      null,
    type:    null,
  },
  listenerSort:   { col: null, asc: true },
  listenerFilter: '',
  _pollTimer: null,
  _actionInFlight: false,
  _pollAbort: null,
  _evtWs: null,
  _evtReconnect: null,
  _evtBackoff: 1000,
  _toasts: [],
};

/* ────────────── UTILS ────────────── */
const esc = s => String(s ?? '')
  .replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')
  .replace(/"/g,'&quot;').replace(/'/g,'&#39;');
const escJS = s => String(s ?? '').replace(/\\/g,'\\\\').replace(/'/g,"\\'");

const fmtDate = ts => {
  if (!ts) return '—';
  const d = new Date(typeof ts === 'number' ? ts * 1000 : ts);
  if (isNaN(d)) return '—';
  return d.toLocaleString();
};

const fmtAge = ts => {
  if (!ts) return '—';
  const d = new Date(typeof ts === 'number' ? ts * 1000 : ts);
  if (isNaN(d)) return '—';
  const diff = (Date.now() - d) / 1000;
  if (diff < 60)   return `${Math.floor(diff)}s ago`;
  if (diff < 3600) return `${Math.floor(diff/60)}m ago`;
  return `${Math.floor(diff/3600)}h ago`;
};

/* ────────────── AUTH ────────────── */
function getToken() { return localStorage.getItem('kj_access_token'); }

async function refreshAccessToken() {
  const rt = localStorage.getItem('kj_refresh_token');
  if (!rt) return false;
  try {
    const r = await fetch('/api/v1/auth/refresh', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: rt })
    });
    if (!r.ok) return false;
    const data = await r.json();
    localStorage.setItem('kj_access_token', data.access_token);
    return true;
  } catch(_) { return false; }
}

function doLogout() {
  localStorage.removeItem('kj_access_token');
  localStorage.removeItem('kj_refresh_token');
  localStorage.removeItem('kj_username');
  localStorage.removeItem('kj_role');
  window.location.href = '/login';
}
window.doLogout = doLogout;

const api = async (method, path, body, signal) => {
  const token = getToken();
  if (!token && !path.includes('/auth/')) {
    window.location.href = '/login';
    throw new Error('Not authenticated');
  }

  const opts = { method, headers: {} };
  if (signal) opts.signal = signal;
  if (token) opts.headers['Authorization'] = 'Bearer ' + token;
  if (body) {
    opts.body = JSON.stringify(body);
    opts.headers['Content-Type'] = 'application/json';
  }

  let r = await fetch(path, opts);

  if (r.status === 401 && !path.includes('/auth/')) {
    const ok = await refreshAccessToken();
    if (ok) {
      opts.headers['Authorization'] = 'Bearer ' + getToken();
      r = await fetch(path, opts);
    } else {
      doLogout();
      throw new Error('Session expired');
    }
  }

  if (!r.ok) {
    const t = await r.text();
    let msg = t || r.statusText;
    try { const j = JSON.parse(t); msg = j.detail || j.error || j.message || msg; } catch(_) {}
    throw new Error(msg);
  }
  if (r.status === 204) return null;
  return r.json().catch(() => null);
};

const apiFetch = async (url, options = {}) => {
  const token = getToken();
  if (!options.headers) options.headers = {};
  if (token) options.headers['Authorization'] = 'Bearer ' + token;
  if (!options.headers['Content-Type'] && options.body && !(options.body instanceof FormData))
    options.headers['Content-Type'] = 'application/json';
  let r = await fetch(url, options);
  if (r.status === 401) {
    const ok = await refreshAccessToken();
    if (ok) {
      options.headers['Authorization'] = 'Bearer ' + getToken();
      r = await fetch(url, options);
    }
  }
  return r;
};

let _toastTimer;
const toast = (msg, isErr = false) => {
  const el = document.getElementById('toast');
  if (!el) return;
  el.textContent = msg;
  el.className = 'show' + (isErr ? ' err' : '');
  clearTimeout(_toastTimer);
  _toastTimer = setTimeout(() => { el.className = ''; }, 3200);
};

const openOverlay  = id => {
  document.getElementById(id)?.classList.add('open');
  if (id === 'ov-listener') setTimeout(validateListenerPort, 0);
};
const closeOverlay = id => document.getElementById(id)?.classList.remove('open');

/* expose to inline onclick */
window.setModule     = setModule;
window.openOverlay   = openOverlay;
window.closeOverlay  = closeOverlay;
window.onProtoChange = onProtoChange;
window.validateListenerPort = validateListenerPort;
window.doStartListener = doStartListener;
window.killSession   = killSession;
window.stopListener  = stopListener;
window.generateImplant = generateImplant;
window.deleteImplant   = deleteImplant;
window.toggleBeaconMode = toggleBeaconMode;
window.openWS        = openWS;
window.closeWS       = closeWS;
window.wsQuick       = wsQuick;
window.wsScreenshot  = wsScreenshot;
window.wsSubmit      = wsSubmit;
window.sortListeners   = sortListeners;
window.filterListeners = filterListeners;
window.openCmdPalette  = openCmdPalette;

/* ────────────── POLLING ────────────── */
async function poll() {
  if (S._actionInFlight) return;
  if (S._pollAbort) S._pollAbort.abort();
  const ac = S._pollAbort = new AbortController();
  try {
    const data = await api('GET', '/api/v1/status', null, ac.signal);
    S.sessions  = data.sessions  ?? [];
    S.beacons   = data.beacons   ?? [];
    S.listeners = data.listeners ?? [];
    S.logs      = data.logs      ?? [];
    S.jobs      = data.jobs      ?? [];
    S.username  = data.username  ?? '';

    const uEl = document.getElementById('hdr-uname');
    if (uEl) uEl.textContent = S.username || '—';

    const aliveSess = S.sessions.filter(x => !x.isDead).length + S.beacons.length;
    setBadge('badge-sessions',  aliveSess, true);
    setBadge('badge-listeners', S.listeners.length, false);

    if (S.ws.open && S.ws.type === 'session') {
      const cur = S.sessions.find(x => (x.id ?? x.ID) === S.ws.id);
      if (cur && cur.isDead && S.ws._term) {
        if (!S.ws._deadNotified) {
          S.ws._deadNotified = true;
        }
        if (!S.ws._replShown) {
          const h = (cur.hostname ?? '').toLowerCase();
          const u = (cur.username ?? '').toLowerCase();
          const repl = S.sessions.find(x =>
            !x.isDead && (x.id ?? x.ID) !== S.ws.id &&
            (x.hostname ?? '').toLowerCase() === h &&
            (x.username ?? '').toLowerCase() === u
          );
          if (repl) {
            S.ws._replShown = true;
            const nid = repl.id ?? repl.ID ?? '';
            const nshort = nid.length > 8 ? nid.slice(0, 8) + '…' : nid;
            S.ws._term.write(`\r\n\x1b[32m✓ New session from same host: ${nshort}\x1b[0m\r\n`);
          }
        }
      }
    }

    if (!S.ws.open) RENDER[S.mod]?.();
  } catch (_) { /* silent — server may be starting */ }
}

function setBadge(id, n, alive) {
  const el = document.getElementById(id);
  if (!el) return;
  el.textContent = n > 0 ? n : '';
  el.className = 'tab-badge' + (alive && n > 0 ? ' alive' : '');
}

function startPolling() {
  poll();
  clearInterval(S._pollTimer);
  S._pollTimer = setInterval(poll, 4000);
}

function stopPolling() {
  clearInterval(S._pollTimer);
  S._pollTimer = null;
}

/* ────────────── MODULE SWITCHING ────────────── */
const RENDER = {
  sessions:  renderSessions,
  listeners: renderListeners,
  implants:  renderImplants,
};

function setModule(name) {
  S.mod = name;
  document.querySelectorAll('.mod-tab').forEach(t => {
    t.classList.toggle('active', t.dataset.mod === name);
  });
  if (!S._pollTimer) startPolling();
  RENDER[name]?.();
}

/* ════════════════════════════════════
   SESSIONS
════════════════════════════════════ */
function renderSessions() {
  const all = [
    ...S.sessions.map(s => ({ ...s, _type: 'session' })),
    ...S.beacons.map(b  => ({ ...b, _type: 'beacon'  })),
  ];

  const aliveByHost = {};
  all.forEach(s => {
    const isAlive = s.isDead !== undefined ? !s.isDead : true;
    if (!isAlive || s._type !== 'session') return;
    const key = `${(s.hostname ?? s.Hostname ?? '').toLowerCase()}|${(s.username ?? s.Username ?? '').toLowerCase()}`;
    if (key === '|') return;
    aliveByHost[key] = s;
  });

  const main = document.getElementById('main');
  if (!main) return;
  main.innerHTML = `
    <div class="module">
      <div class="mod-bar">
        <span class="mod-title">Sessions</span>
        <span class="mod-count">${all.length} connected</span>
        <span class="mod-gap"></span>
        <span style="font-size:11px;color:var(--muted)">${S._evtWs && S._evtWs.readyState === 1 ? '⚡ Real-time' : 'Polling every 4s'}</span>
      </div>
      <div class="tbl-wrap">
        ${all.length === 0 ? `
          <div class="empty">
            <div class="empty-icon">📡</div>
            <div class="empty-title">No active sessions</div>
            <div class="empty-desc">Start a listener and deploy an implant to receive callbacks here.</div>
          </div>
        ` : `
          <table>
            <thead><tr>
              <th>Status</th><th>Name</th><th>ID</th><th>Hostname</th><th>User</th>
              <th>OS / Arch</th><th>Remote IP</th><th>PID</th><th>Type</th>
              <th>Last seen</th><th>Actions</th>
            </tr></thead>
            <tbody>${all.map(s => sessRow(s, aliveByHost)).join('')}</tbody>
          </table>`}
      </div>
    </div>`;
}

function sessRow(s, aliveByHost) {
  const alive = s.isDead !== undefined ? !s.isDead : (s.alive ?? true);
  const id    = s.id ?? s.ID ?? '';
  const short = id.length > 8 ? id.slice(0, 8) + '…' : id;
  const name  = s.name ?? s.Name ?? '';
  const host  = s.hostname  ?? s.Hostname  ?? '—';
  const user  = s.username  ?? s.Username  ?? s.user ?? '—';
  const os    = s.os        ?? s.OS        ?? '—';
  const arch  = s.arch      ?? s.Arch      ?? '—';
  const ip    = s.remoteAddress ?? s.remoteAddr ?? s.RemoteAddr ?? s.ip ?? '—';
  const pid   = s.pid       ?? s.PID       ?? '—';
  const seen  = fmtAge(s.lastCheckin ?? s.lastSeen ?? s.LastSeen ?? s.checkin ?? s.Checkin);

  let reconnectTag = '';
  let reconnectBtn = '';
  if (!alive && s._type === 'session' && aliveByHost) {
    const key = `${(s.hostname ?? s.Hostname ?? '').toLowerCase()}|${(s.username ?? s.Username ?? '').toLowerCase()}`;
    const repl = aliveByHost[key];
    if (repl && (repl.id ?? repl.ID) !== id) {
      const nid = repl.id ?? repl.ID ?? '';
      const nshort = nid.length > 8 ? nid.slice(0, 8) + '…' : nid;
      reconnectTag = `<div style="font-size:9.5px;color:var(--alive);margin-top:2px">↳ Reconnected</div>`;
      reconnectBtn = `<button class="btn btn-sm" style="font-size:10px;background:rgba(46,204,113,0.12);border-color:var(--alive);color:var(--alive);margin-left:4px" onclick='openWS("${esc(repl._type)}","${esc(nid)}")'>→ ${esc(nshort)}</button>`;
    }
  }

  const nameDisplay = name
    ? `<span class="rename-label" onclick='startRename("${esc(s._type)}","${esc(id)}", this)' title="Click to rename">${esc(name)}</span>`
    : `<span class="rename-placeholder" onclick='startRename("${esc(s._type)}","${esc(id)}", this)' title="Click to set name">—</span>`;

  return `<tr${!alive ? ' style="opacity:0.55"' : ''} data-entity-id="${esc(id)}">
    <td><span class="sdot ${alive?'alive':'dead'}"></span>${alive?'Online':'Dead'}${reconnectTag}</td>
    <td class="name-cell">${nameDisplay}</td>
    <td><span class="mono hi" title="${esc(id)}">${esc(short)}</span></td>
    <td class="hi">${esc(host)}</td>
    <td>${esc(user)}</td>
    <td><span class="os-badge">${esc(os)} / ${esc(arch)}</span></td>
    <td class="mono">${esc(ip)}</td>
    <td class="mono">${esc(pid)}</td>
    <td><span class="chip">${esc(s._type)}</span></td>
    <td>${seen}</td>
    <td class="actions">
      ${alive
        ? `<button class="btn btn-gold btn-sm" onclick='openWS("${esc(s._type)}","${esc(id)}")'>Interact</button>`
        : reconnectBtn
          ? `<span style="font-size:10px;color:var(--muted)">Dead</span>${reconnectBtn}`
          : `<button class="btn btn-sm" style="opacity:0.5" onclick='openWS("${esc(s._type)}","${esc(id)}")'>Interact</button>`}
      <button class="btn btn-danger btn-sm" style="margin-left:4px" onclick='killSession("${esc(s._type)}","${esc(id)}")'>Kill</button>
    </td>
  </tr>`;
}

async function killSession(type, id) {
  if (!confirm(`Terminate session ${id}?`)) return;
  try {
    await api('DELETE', type === 'beacon' ? `/api/v1/beacons/${id}` : `/api/v1/sessions/${id}`);
    toast('Session terminated');
    poll();
  } catch(e) { toast(e.message, true); }
}

function startRename(type, id, el) {
  const cell = el.closest('.name-cell');
  if (!cell) return;
  const current = el.textContent === '—' ? '' : el.textContent;
  cell.innerHTML = `<input class="rename-input" type="text" value="${esc(current)}"
    onblur='saveRename("${esc(type)}","${esc(id)}", this)'
    onkeydown='if(event.key==="Enter"){this.blur()}else if(event.key==="Escape"){cancelRename(this,"${esc(current)}")}'
    style="width:100%;padding:2px 6px;background:var(--bg);border:1px solid var(--gold);color:var(--fg);border-radius:3px;font-family:var(--mono);font-size:11px"
  />`;
  const input = cell.querySelector('.rename-input');
  input.focus();
  input.select();
}
window.startRename = startRename;

function cancelRename(input, original) {
  const cell = input.closest('.name-cell');
  if (!cell) return;
  const row = cell.closest('tr');
  const id = row ? row.dataset.entityId : '';
  const type = row ? (row.querySelector('.chip')?.textContent?.toLowerCase() || 'session') : 'session';
  const display = original || '—';
  const isPlaceholder = !original;
  cell.innerHTML = isPlaceholder
    ? `<span class="rename-placeholder" onclick='startRename("${esc(type)}","${esc(id)}", this)' title="Click to set name">—</span>`
    : `<span class="rename-label" onclick='startRename("${esc(type)}","${esc(id)}", this)' title="Click to rename">${esc(original)}</span>`;
}
window.cancelRename = cancelRename;

async function saveRename(type, id, input) {
  const newName = input.value.trim();
  if (!newName) { cancelRename(input, ''); return; }
  const endpoint = type === 'beacon' ? `/api/v1/beacons/${id}` : `/api/v1/sessions/${id}`;
  try {
    await api('PATCH', endpoint, { name: newName });
    showToast(`Renamed to: ${newName}`, 'info');
    poll();
  } catch(e) {
    showToast(`Rename failed: ${e.message}`, 'danger');
    poll();
  }
}
window.saveRename = saveRename;

/* ════════════════════════════════════
   LISTENERS
════════════════════════════════════ */
function listenerSortIcon(col) {
  if (S.listenerSort.col !== col) return '<span style="opacity:.3;font-size:10px">⇅</span>';
  return `<span style="font-size:10px">${S.listenerSort.asc ? '↑' : '↓'}</span>`;
}

function sortListeners(col) {
  if (S.listenerSort.col === col) {
    S.listenerSort.asc = !S.listenerSort.asc;
  } else {
    S.listenerSort.col = col;
    S.listenerSort.asc = true;
  }
  renderListeners();
}

function filterListeners(val) {
  S.listenerFilter = val.toLowerCase();
  renderListeners();
}

function renderListeners() {
  let items = [...S.listeners];

  if (S.listenerFilter) {
    const q = S.listenerFilter;
    items = items.filter(l => {
      const hay = [l.id, l.ID, l.protocol, l.proto, l.port, l.Port, ...(l.domains || [])].join(' ').toLowerCase();
      return hay.includes(q);
    });
  }

  if (S.listenerSort.col) {
    const col = S.listenerSort.col;
    const dir = S.listenerSort.asc ? 1 : -1;
    items.sort((a, b) => {
      const va = a[col] ?? '';
      const vb = b[col] ?? '';
      if (typeof va === 'number' && typeof vb === 'number') return (va - vb) * dir;
      return String(va).localeCompare(String(vb)) * dir;
    });
  }

  const main = document.getElementById('main');
  if (!main) return;
  main.innerHTML = `
    <div class="module">
      <div class="mod-bar">
        <span class="mod-title">Listeners</span>
        <span class="mod-count">${S.listeners.length} active</span>
        <span class="mod-gap"></span>
        <input type="text" placeholder="Filter listeners…" value="${esc(S.listenerFilter)}"
               oninput="filterListeners(this.value)"
               style="width:180px;padding:4px 8px;font-size:12px;border:1px solid rgba(212,175,55,0.2);border-radius:4px;background:rgba(255,255,255,0.04);color:var(--sub);font-family:var(--mono)"/>
        <button class="btn btn-gold" onclick="openOverlay('ov-listener')">+ New Listener</button>
      </div>
      <div class="tbl-wrap">
        ${items.length === 0 && S.listeners.length === 0 ? `
          <div class="empty">
            <div class="empty-icon">🔊</div>
            <div class="empty-title">No listeners running</div>
            <div class="empty-desc">Create a listener to accept incoming implant callbacks.</div>
            <button class="btn btn-gold" onclick="openOverlay('ov-listener')">Start first listener</button>
          </div>
        ` : items.length === 0 ? `
          <div class="empty">
            <div class="empty-icon">🔍</div>
            <div class="empty-title">No matches</div>
            <div class="empty-desc">No listeners match the current filter.</div>
          </div>
        ` : `
          <table>
            <thead><tr>
              <th>Status</th>
              <th style="cursor:pointer;user-select:none" onclick="sortListeners('id')">ID ${listenerSortIcon('id')}</th>
              <th style="cursor:pointer;user-select:none" onclick="sortListeners('protocol')">Protocol ${listenerSortIcon('protocol')}</th>
              <th style="cursor:pointer;user-select:none" onclick="sortListeners('port')">Port ${listenerSortIcon('port')}</th>
              <th>Domains</th><th>Actions</th>
            </tr></thead>
            <tbody>${items.map(listenerRow).join('')}</tbody>
          </table>`}
      </div>
    </div>`;
}

function listenerRow(l) {
  const id    = l.id ?? l.ID ?? l.jobID ?? l.JobID ?? '';
  const proto = (l.protocol ?? l.proto ?? l.Proto ?? '—').toUpperCase();
  const port  = l.port ?? l.Port ?? '—';
  const doms  = Array.isArray(l.domains) ? l.domains.join(', ') : (l.domain ?? l.Domain ?? '—');
  return `<tr>
    <td><span class="sdot alive"></span>Running</td>
    <td><span class="mono hi">${esc(String(id).slice(0,10))}</span></td>
    <td><span class="chip">${esc(proto)}</span></td>
    <td class="mono">${esc(port)}</td>
    <td class="mono">${esc(doms)}</td>
    <td class="actions">
      <button class="btn btn-danger btn-sm" onclick="stopListener('${esc(id)}')">Stop</button>
    </td>
  </tr>`;
}

async function stopListener(id) {
  if (!confirm(`Stop listener ${id}?`)) return;
  try {
    await api('DELETE', `/api/v1/listeners/${id}`);
    poll();
  } catch(e) { toast(e.message, true); }
}

const PROTO_PORTS = { http: 80, https: 443, mtls: 8888 };

function onProtoChange() {
  const proto = document.getElementById('l-proto')?.value;
  const fld   = document.getElementById('l-dns-fld');
  const port  = document.getElementById('l-port');
  if (!fld || !port) return;
  fld.style.display = proto === 'dns' ? '' : 'none';
  port.value = PROTO_PORTS[proto] ?? 80;
  validateListenerPort();
}

function validateListenerPort() {
  const portEl = document.getElementById('l-port');
  const btn    = document.querySelector('#ov-listener .btn-gold');
  if (!portEl) return;
  const port = parseInt(portEl.value, 10);
  let warn = document.getElementById('l-port-warn');
  const used = S.listeners.some(l => (l.port ?? l.Port) === port);
  if (used) {
    if (!warn) {
      warn = document.createElement('div');
      warn.id = 'l-port-warn';
      warn.style.cssText = 'color:#e74c3c;font-size:.8rem;margin-top:4px';
      portEl.parentNode.appendChild(warn);
    }
    warn.textContent = `Port ${port} is already in use by another listener`;
    portEl.style.borderColor = '#e74c3c';
    if (btn) { btn.disabled = true; btn.style.opacity = '0.5'; }
  } else {
    if (warn) warn.remove();
    portEl.style.borderColor = '';
    if (btn) { btn.disabled = false; btn.style.opacity = ''; }
  }
}

async function doStartListener() {
  const proto  = document.getElementById('l-proto')?.value;
  const host   = document.getElementById('l-host')?.value.trim();
  const port   = parseInt(document.getElementById('l-port')?.value, 10);
  const domain = document.getElementById('l-domain')?.value.trim();
  if (!proto || !host || !port) { toast('Fill all required fields', true); return; }
  if (S.listeners.some(l => (l.port ?? l.Port) === port)) {
    toast(`Port ${port} is already in use`, true); return;
  }
  const body = { host, port };
  if (proto === 'dns' && domain) body.domain = domain;
  try {
    await api('POST', `/api/v1/listeners/${proto}`, body);
    closeOverlay('ov-listener');
    toast('Listener started'); poll();
  } catch(e) { toast(e.message, true); }
}

/* ════════════════════════════════════
   IMPLANTS
════════════════════════════════════ */
async function renderImplants() {
  try {
    const data = await api('GET', '/api/v1/payloads');
    S.payloads = data?.items ?? data ?? [];
  } catch(_) { S.payloads = []; }

  const listenerOpts = S.listeners.length > 0
    ? S.listeners.map(l => {
        const proto = (l.protocol ?? 'mtls').toLowerCase();
        const port  = l.port ?? 80;
        const url   = `${proto}://0.0.0.0:${port}`;
        return `<option value="${esc(url)}">${esc(proto.toUpperCase())} — :${esc(port)}</option>`;
      }).join('')
    : '<option value="">— No listeners running —</option>';

  const main = document.getElementById('main');
  if (!main) return;

  if (document.getElementById('impl-form-col')) {
    const tblWrap = document.querySelector('#impl-list-col .tbl-wrap');
    if (tblWrap) tblWrap.innerHTML = buildImplantTable();
    const listenerSel = document.getElementById('impl-listener');
    if (listenerSel) {
      const cur = listenerSel.value;
      listenerSel.innerHTML = listenerOpts;
      listenerSel.value = cur;
    }
    return;
  }

  main.innerHTML = `
    <div class="module" style="height:100%">
      <div class="mod-bar">
        <span class="mod-title">Implants</span>
        <span class="mod-count">Generate payload files &amp; download them</span>
      </div>
      <div id="impl-layout">

        <!-- LEFT: Generator -->
        <div id="impl-form-col">
          <div class="panel">
            <div class="panel-head">Generate New Implant</div>
            <div class="panel-body">
              <div class="form-grid">
                <div class="form-row">
                  <div class="field">
                    <label>Target OS</label>
                    <select id="impl-os">
                      <option value="windows">Windows</option>
                      <option value="linux">Linux</option>
                      <option value="darwin">macOS</option>
                    </select>
                  </div>
                  <div class="field">
                    <label>Architecture</label>
                    <select id="impl-arch">
                      <option value="amd64">x86-64</option>
                      <option value="386">x86-32</option>
                      <option value="arm64">ARM64</option>
                    </select>
                  </div>
                </div>
                <div class="field">
                  <label>Output Format</label>
                  <select id="impl-fmt"></select>
                </div>
                <div class="field">
                  <label>Implant Name <span style="color:var(--muted);font-size:10px;text-transform:none;letter-spacing:0">(optional)</span></label>
                  <input id="impl-name" type="text" placeholder="e.g. agent-01"/>
                </div>
                <div class="field">
                  <label>Callback Listener</label>
                  <select id="impl-listener">${listenerOpts}</select>
                </div>
                <div class="field">
                  <label>C2 Endpoint <span style="color:var(--muted);font-size:10px;text-transform:none;letter-spacing:0">(override, optional)</span></label>
                  <input id="impl-endpoint" type="text" placeholder="https://your-c2-host:8443"/>
                </div>
                <div class="field" style="border-top:1px solid var(--bd);padding-top:12px;margin-top:2px">
                  <label style="display:flex;align-items:center;gap:8px;cursor:pointer;text-transform:none;font-size:12px;letter-spacing:0;color:var(--text);font-weight:500">
                    <input type="checkbox" id="impl-beacon" onchange="toggleBeaconMode()" style="width:14px;height:14px;accent-color:var(--gold);cursor:pointer;flex-shrink:0"/>
                    Beacon Mode
                    <span style="font-size:10.5px;color:var(--muted);font-weight:400">(async check-in vs. interactive session)</span>
                  </label>
                </div>
                <div id="impl-beacon-fields" style="display:none" class="form-row">
                  <div class="field">
                    <label>Beacon Interval (secs)</label>
                    <input id="impl-beacon-interval" type="number" value="60" min="1" style="font-family:var(--mono)"/>
                  </div>
                  <div class="field">
                    <label>Beacon Jitter (secs)</label>
                    <input id="impl-beacon-jitter" type="number" value="10" min="0" style="font-family:var(--mono)"/>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <button class="btn btn-gold" style="width:100%;justify-content:center;padding:10px" onclick="generateImplant()">
            ⬇ Generate Implant
          </button>

          <div id="impl-result" style="display:none" class="panel">
            <div class="panel-head" style="color:var(--alive)">Generated</div>
            <div class="panel-body" id="impl-result-body"></div>
          </div>
        </div>

        <!-- RIGHT: Downloads -->
        <div id="impl-list-col">
          <div class="section-label">
            Generated Implants
            <span style="font-family:var(--mono);font-size:10.5px;color:var(--gold);letter-spacing:0;text-transform:none;font-weight:400;margin-left:8px">saved in payloads/</span>
            <span style="float:right;margin-right:18px;color:var(--muted);font-size:10.5px;font-weight:400">${S.payloads.length} file${S.payloads.length !== 1 ? 's' : ''}</span>
          </div>
          <div class="tbl-wrap">${buildImplantTable()}</div>
        </div>

      </div>
    </div>`;

  document.getElementById('impl-os')?.addEventListener('change', updateFormatOptions);
  updateFormatOptions();
}

function buildImplantTable() {
  if (!S.payloads || S.payloads.length === 0) return `
    <div class="empty">
      <div class="empty-icon">💾</div>
      <div class="empty-title">No implants yet</div>
      <div class="empty-desc">Use the form on the left to generate your first payload. All generated files appear here with a download button.</div>
    </div>`;
  return `
    <table>
      <thead><tr>
        <th>Filename</th><th>OS / Arch</th><th>Format</th>
        <th>Listener</th><th>Size</th><th>Created</th><th>Actions</th>
      </tr></thead>
      <tbody>${S.payloads.map(implantRow).join('')}</tbody>
    </table>`;
}

function implantRow(p) {
  const id   = p.id ?? p.ID ?? '';
  const name = p.name ?? p.filename ?? p.Name ?? id;
  const os   = p.os   ?? p.OS   ?? '—';
  const arch = p.arch ?? p.Arch ?? '—';
  const fmt  = p.format ?? p.Format ?? '—';
  const lstn = p.listener ?? p.listenerID ?? p.ListenerID ?? '—';
  const size = p.size
    ? (p.size > 1024*1024 ? `${(p.size/1024/1024).toFixed(1)} MB` : `${Math.ceil(p.size/1024)} KB`)
    : '—';
  const ts   = fmtDate(p.createdAt ?? p.CreatedAt ?? p.created);
  return `<tr>
    <td><span class="mono hi">${esc(name)}</span></td>
    <td><span class="os-badge">${esc(os)} / ${esc(arch)}</span></td>
    <td><span class="chip">${esc(fmt)}</span></td>
    <td><span class="mono" style="font-size:10.5px">${esc(String(lstn).slice(0,20))}</span></td>
    <td>${esc(size)}</td>
    <td>${ts}</td>
    <td class="actions" style="white-space:nowrap">
      <a href="/api/v1/payloads/${esc(id)}/download?token=${encodeURIComponent(getToken()||'')}" download="${esc(name)}" class="btn btn-gold btn-sm" style="text-decoration:none">⬇ Download</a>
      <button class="btn btn-danger btn-sm" style="margin-left:4px" onclick="deleteImplant('${esc(id)}')">Delete</button>
    </td>
  </tr>`;
}

const FORMAT_OPTIONS = {
  windows: [
    { value: 'exe',       label: 'Windows EXE' },
    { value: 'dll',       label: 'Windows DLL' },
    { value: 'service',   label: 'Windows Service' },
    { value: 'shellcode', label: 'Raw Shellcode' },
  ],
  linux: [
    { value: 'exe',       label: 'Linux Executable (ELF)' },
    { value: 'shared',    label: 'Linux Shared Library (.so)' },
    { value: 'shellcode', label: 'Raw Shellcode' },
  ],
  darwin: [
    { value: 'exe',       label: 'macOS Executable (Mach-O)' },
    { value: 'shared',    label: 'macOS Shared Library (.dylib)' },
    { value: 'shellcode', label: 'Raw Shellcode' },
  ],
};

function updateFormatOptions() {
  const os  = document.getElementById('impl-os')?.value || 'windows';
  const sel = document.getElementById('impl-fmt');
  if (!sel) return;
  const prev = sel.value;
  sel.innerHTML = (FORMAT_OPTIONS[os] || FORMAT_OPTIONS.windows)
    .map(o => `<option value="${o.value}">${o.label}</option>`).join('');
  if ([...sel.options].some(o => o.value === prev)) sel.value = prev;
}

function toggleBeaconMode() {
  const cb  = document.getElementById('impl-beacon');
  const fld = document.getElementById('impl-beacon-fields');
  if (!fld) return;
  fld.style.display = cb?.checked ? '' : 'none';
}

async function generateImplant() {
  const os        = document.getElementById('impl-os')?.value;
  const arch      = document.getElementById('impl-arch')?.value;
  const format    = document.getElementById('impl-fmt')?.value;
  const listenerURL = document.getElementById('impl-listener')?.value;
  const endpoint  = document.getElementById('impl-endpoint')?.value.trim();
  const name      = document.getElementById('impl-name')?.value.trim();
  const isBeacon  = document.getElementById('impl-beacon')?.checked ?? false;
  const beaconInterval = parseInt(document.getElementById('impl-beacon-interval')?.value ?? '60', 10);
  const beaconJitter   = parseInt(document.getElementById('impl-beacon-jitter')?.value  ?? '10', 10);
  if (!os || !arch || !format) { toast('Fill OS, arch and format', true); return; }

  const genBtn = document.querySelector('#main .btn.btn-gold[onclick*="generateImplant"]');
  const overlay = document.getElementById('gen-overlay');
  if (genBtn) { genBtn.disabled = true; genBtn.style.opacity = '0.4'; }
  if (overlay) overlay.classList.add('open');

  const body = { os, arch, format, listener: endpoint || listenerURL || '', isBeacon };
  if (name) body.name = name;
  if (isBeacon) { body.beaconInterval = beaconInterval; body.beaconJitter = beaconJitter; }
  try {
    const res = await api('POST', '/api/v1/payloads', body);

    const data = await api('GET', '/api/v1/payloads');
    S.payloads = data?.items ?? data ?? [];

    const rb = document.getElementById('impl-result');
    const bb = document.getElementById('impl-result-body');
    if (rb && bb) {
      rb.style.display = '';
      const newID = res?.id ?? res?.ID ?? '';
      bb.innerHTML = `
        <div style="display:flex;flex-direction:column;gap:8px">
          <div style="color:var(--alive)">✓ Implant generated</div>
          ${res?.name ? `<div class="mono" style="font-size:12px;color:var(--sub)">${esc(res.name)}</div>` : ''}
          ${newID ? `<a href="/api/v1/payloads/${esc(newID)}/download?token=${encodeURIComponent(getToken()||'')}" download="${esc(res?.name ?? 'implant')}" class="btn btn-gold btn-sm" style="text-decoration:none;width:fit-content">⬇ Download Now</a>` : ''}
        </div>`;
    }

    const tw = document.querySelector('#impl-list-col .tbl-wrap');
    if (tw) tw.innerHTML = buildImplantTable();
    toast('Implant generated');
  } catch(e) {
    toast(e.message || 'Generation failed', true);
  } finally {
    if (overlay) overlay.classList.remove('open');
    if (genBtn) { genBtn.disabled = false; genBtn.style.opacity = ''; }
  }
}

async function deleteImplant(id) {
  if (!confirm('Delete this implant file?')) return;
  try {
    await api('DELETE', `/api/v1/payloads/${id}`);
    const data = await api('GET', '/api/v1/payloads');
    S.payloads = data?.items ?? data ?? [];
    const tw = document.querySelector('#impl-list-col .tbl-wrap');
    if (tw) tw.innerHTML = buildImplantTable();
    toast('Deleted');
  } catch(e) { toast(e.message, true); }
}

/* ════════════════════════════════════
   ARSENAL
════════════════════════════════════ */
function renderArsenal() {
  const main = document.getElementById('main');
  if (!main) return;
  const jobs = S.jobs ?? [];
  main.innerHTML = `
    <div class="module" style="height:100%">
      <div class="mod-bar">
        <span class="mod-title">Arsenal</span>
        <span class="mod-count">File staging &amp; execution jobs</span>
      </div>
      <div id="arsenal-layout">

        <div id="arsenal-upload-col">
          <div class="section-label" style="padding-top:20px">Stage a File</div>
          <div class="panel" style="margin:0 20px">
            <div class="panel-body">
              <div class="form-grid">
                <div class="field">
                  <label>File to upload</label>
                  <input type="file" id="arsen-file" style="color:var(--sub);font-family:var(--mono);font-size:12px;background:rgba(255,255,255,0.03);border:1px dashed rgba(212,175,55,0.2);border-radius:6px;padding:8px"/>
                </div>
                <div class="field">
                  <label>Destination path (on target, optional)</label>
                  <input id="arsen-dest" type="text" placeholder="/tmp/payload"/>
                </div>
              </div>
              <div style="margin-top:12px">
                <button class="btn btn-gold" onclick="uploadArsenal()">Stage File</button>
              </div>
            </div>
          </div>
        </div>

        <div id="arsenal-jobs-col">
          <div class="section-label" style="padding-top:20px">Jobs (${jobs.length})</div>
          ${jobs.length === 0 ? `
            <div class="empty">
              <div class="empty-icon">⚙️</div>
              <div class="empty-title">No jobs running</div>
              <div class="empty-desc">File transfers and remote executions will appear here.</div>
            </div>
          ` : `
            <table>
              <thead><tr>
                <th>Job ID</th><th>Type</th><th>Session</th><th>Status</th><th>Started</th>
              </tr></thead>
              <tbody>${jobs.map(j => `
                <tr>
                  <td><span class="mono hi">${esc((j.id ?? j.ID ?? '').slice(0,10))}</span></td>
                  <td><span class="chip">${esc(j.type ?? j.Type ?? '—')}</span></td>
                  <td class="mono">${esc(j.sessionID ?? j.SessionID ?? '—')}</td>
                  <td>${esc(j.status ?? j.Status ?? '—')}</td>
                  <td>${fmtDate(j.startedAt ?? j.StartedAt)}</td>
                </tr>`).join('')}
              </tbody>
            </table>`}
        </div>

      </div>
    </div>`;
}

async function uploadArsenal() {
  const fi   = document.getElementById('arsen-file');
  const dest = document.getElementById('arsen-dest')?.value.trim();
  if (!fi?.files?.length) { toast('Select a file first', true); return; }
  const fd = new FormData();
  fd.append('file', fi.files[0]);
  if (dest) fd.append('dest', dest);
  try {
    const hdrs = {};
    const token = getToken();
    if (token) hdrs['Authorization'] = 'Bearer ' + token;
    const r = await fetch('/api/v1/upload', { method: 'POST', body: fd, headers: hdrs });
    if (!r.ok) throw new Error(await r.text());
    toast('File staged');
    if (fi) fi.value = '';
  } catch(e) { toast(e.message, true); }
}


/* ════════════════════════════════════
   SESSION WORKSPACE (xterm.js + WebSocket shell)
════════════════════════════════════ */
function openWS(type, id) {
  cleanupShell();

  S.ws.open = true;
  S.ws.type = type;
  S.ws.id   = id;
  S.ws._deadNotified = false;
  S.ws._replShown = false;

  const src = type === 'beacon'
    ? S.beacons.find(b  => (b.id ?? b.ID) === id)
    : S.sessions.find(s => (s.id ?? s.ID) === id);

  const sname = src?.name ?? src?.Name ?? '';
  const host = src?.hostname ?? src?.Hostname ?? id;
  const user = src?.username ?? src?.Username ?? '—';
  const ip   = src?.remoteAddress ?? src?.remoteAddr ?? src?.RemoteAddr ?? '—';
  const os   = (src?.os ?? src?.OS ?? '').toLowerCase();
  const arch = src?.arch ?? src?.Arch ?? '—';
  const isWin = os.includes('win');

  document.getElementById('ws-name').textContent = sname ? `${sname} (${user}@${host})` : `${user}@${host}`;
  document.getElementById('ws-meta').textContent = `${ip}  •  ${os || '—'}/${arch}  •  ${id}`;
  document.getElementById('ws-badge').textContent = type.toUpperCase();

  // Reset to terminal tab and render
  FB.cwd = '';
  FB.files = [];
  FB.loading = false;
  S.ws._activeTab = 'terminal';
  document.querySelectorAll('#ws-tabs .ws-tab').forEach(t => t.classList.toggle('active', t.dataset.tab === 'terminal'));
  document.getElementById('ws-qa').style.display = '';
  renderTerminalTab();

  document.getElementById('ws').classList.add('open');
}

function initShellTerminal(sessionID) {
  const container = document.getElementById('ws-term');
  if (!container || typeof Terminal === 'undefined') return;

  const term = new Terminal({
    cursorBlink: true,
    fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
    fontSize: 13,
    lineHeight: 1.3,
    scrollback: 10000,
    theme: {
      background:  '#0a0a08',
      foreground:  '#e8e8e6',
      cursor:      '#d4af37',
      cursorAccent:'#0a0a08',
      black:       '#1a1a18',
      red:         '#e05252',
      green:       '#3dd68c',
      yellow:      '#d4af37',
      blue:        '#5a9bcf',
      magenta:     '#c678dd',
      cyan:        '#56b6c2',
      white:       '#e8e8e6',
      brightBlack: '#5a5a56',
      brightRed:   '#e06c75',
      brightGreen: '#98c379',
      brightYellow:'#f0c040',
      brightBlue:  '#61afef',
      brightMagenta:'#c678dd',
      brightCyan:  '#56b6c2',
      brightWhite: '#ffffff',
      selectionBackground: 'rgba(212,175,55,0.25)',
    }
  });

  let fitAddon = null;
  if (typeof FitAddon !== 'undefined') {
    fitAddon = new FitAddon.FitAddon();
    term.loadAddon(fitAddon);
  }

  term.open(container);

  const src = S.sessions.find(s => (s.id ?? s.ID) === sessionID);
  const host = src?.hostname ?? src?.Hostname ?? 'host';
  const os   = (src?.os ?? src?.OS ?? '').toLowerCase();
  const isWin = os.includes('win');
  const promptChar = isWin ? '>' : '$';
  const cwd = isWin ? 'C:\\' : '~';

  let cmdLine = '';
  let busy = false;
  let history = [];
  let histIdx = -1;
  let currentPromptCwd = cwd;

  function writePrompt() {
    term.write(`\x1b[32m${host}\x1b[0m:\x1b[34m${currentPromptCwd}\x1b[0m${promptChar} `);
  }

  term.write('\x1b[33mKonjiYami REST Console\x1b[0m\r\n');
  term.write('\x1b[90mType commands and press Enter. Output via REST API.\x1b[0m\r\n\r\n');
  writePrompt();

  function doFit() { if (fitAddon) try { fitAddon.fit(); } catch(_) {} }

  const resizeObserver = new ResizeObserver(() => {
    if (document.getElementById('ws-term').offsetHeight > 0) {
      doFit();
    }
  });
  resizeObserver.observe(document.getElementById('ws-term'));
  window.addEventListener('resize', doFit);

  async function executeCommand(cmd) {
    if (!cmd.trim()) { term.write('\r\n'); writePrompt(); return; }
    busy = true;
    term.write('\r\n');
    let dead = false;
    const abort = new AbortController();
    const timer = setTimeout(() => abort.abort(), 35000);
    try {
      const resp = await apiFetch(`/api/v1/sessions/${sessionID}/console`, {
        method: 'POST',
        body: JSON.stringify({ command: cmd }),
        signal: abort.signal,
      });
      if (!resp.ok) {
        const errText = await resp.text().catch(() => resp.statusText);
        if (resp.status === 410 || resp.status === 404) {
          dead = true;
          markDead();
          return;
        }
        term.write(`\x1b[31m[error ${resp.status}] ${errText}\x1b[0m\r\n`);
        return;
      }
      const data = await resp.json();
      if (data.output) {
        const lines = data.output.replace(/\r?\n$/, '').split('\n');
        for (const line of lines) {
          term.write(line + '\r\n');
        }
      }
      if (cmd.trim().toLowerCase().startsWith('cd ') && data.status === 0) {
        const arg = cmd.trim().substring(3).trim();
        if (arg) currentPromptCwd = arg;
      }
    } catch (err) {
      if (err.name === 'AbortError') {
        term.write('\x1b[31m[timeout] Command took too long.\x1b[0m\r\n');
      } else if (err.message && (err.message.includes('fetch') || err.message.includes('Failed'))) {
        dead = true;
        markDead();
      } else {
        term.write(`\x1b[31m[error] ${err.message}\x1b[0m\r\n`);
      }
    } finally {
      clearTimeout(timer);
      if (!dead) {
        writePrompt();
        busy = false;
      }
    }
  }

  function markDead() {
    S.ws._deadNotified = true;
    term.write('\r\n\x1b[31;1m⚠ Session lost — the implant disconnected.\x1b[0m\r\n');
    term.write('\x1b[90mKeyboard disabled. Click "← Sessions" to return.\x1b[0m\r\n');
    document.querySelectorAll('#ws-qa .qa').forEach(b => b.disabled = true);
    busy = false;
  }

  term.onKey(({ key, domEvent }) => {
    if (S.ws._deadNotified || busy) return;
    const ev = domEvent;
    const code = ev.keyCode;

    if (code === 13) {
      if (cmdLine.trim()) { history.push(cmdLine); histIdx = history.length; }
      executeCommand(cmdLine);
      cmdLine = '';
      return;
    }
    if (code === 8) {
      if (cmdLine.length > 0) {
        cmdLine = cmdLine.slice(0, -1);
        term.write('\b \b');
      }
      return;
    }
    if (code === 38) {
      if (history.length === 0) return;
      if (histIdx > 0) histIdx--;
      clearLine();
      cmdLine = history[histIdx] || '';
      term.write(cmdLine);
      return;
    }
    if (code === 40) {
      if (histIdx < history.length - 1) { histIdx++; clearLine(); cmdLine = history[histIdx]; term.write(cmdLine); }
      else { histIdx = history.length; clearLine(); cmdLine = ''; }
      return;
    }
    if (ev.ctrlKey && code === 67) {
      cmdLine = '';
      term.write('^C\r\n');
      writePrompt();
      return;
    }
    if (key.length === 1 && !ev.ctrlKey && !ev.altKey && !ev.metaKey) {
      cmdLine += key;
      term.write(key);
    }
  });

  function clearLine() {
    const eraseLen = cmdLine.length;
    term.write('\b'.repeat(eraseLen) + ' '.repeat(eraseLen) + '\b'.repeat(eraseLen));
  }

  S.ws._term = term;
  S.ws._fitAddon = fitAddon;
  S.ws._resizeObs = resizeObserver;
  S.ws._execCmd = executeCommand;

  term.focus();
}

function cleanupShell() {
  if (S.ws._blobUrls) {
    S.ws._blobUrls.forEach(u => URL.revokeObjectURL(u));
    S.ws._blobUrls = null;
  }
  const overlay = document.getElementById('screenshot-overlay');
  if (overlay) overlay.remove();
  if (S.ws._resizeObs) {
    S.ws._resizeObs.disconnect();
    S.ws._resizeObs = null;
  }
  if (S.ws._term) {
    S.ws._term.dispose();
    S.ws._term = null;
  }
  S.ws._fitAddon = null;
  S.ws._execCmd = null;
}

function closeWS() {
  cleanupShell();
  S.ws.open = false;
  document.getElementById('ws').classList.remove('open');
  startPolling();
}

// --- Beacon tasking ---
async function beaconLoadTasks() {
  if (!S.ws.id || S.ws.type !== 'beacon') return;
  try {
    const data = await api('GET', `/api/v1/beacons/${S.ws.id}/tasks`);
    renderBeaconTasks(data.tasks || []);
  } catch (err) {
    const tbody = document.getElementById('beacon-tasks-body');
    if (tbody) tbody.innerHTML = `<tr><td colspan="4" style="color:var(--err);padding:8px">Failed to load tasks: ${esc(err.message)}</td></tr>`;
  }
}

function renderBeaconTasks(tasks) {
  const tbody = document.getElementById('beacon-tasks-body');
  if (!tbody) return;
  if (!tasks.length) {
    tbody.innerHTML = `<tr><td colspan="4" style="color:var(--muted);padding:8px;text-align:center">No tasks queued yet</td></tr>`;
    return;
  }
  const sorted = tasks.slice().sort((a, b) => b.createdAt - a.createdAt);
  tbody.innerHTML = sorted.map(t => {
    const stateColor = t.state === 'completed' ? '#4ade80' : t.state === 'pending' ? 'var(--gold)' : t.state === 'sent' ? '#60a5fa' : 'var(--muted)';
    const created = new Date(t.createdAt * 1000).toLocaleTimeString();
    const output = t.output ? esc(t.output).replace(/\n/g, '<br>') : '';
    return `<tr style="border-bottom:1px solid rgba(90,90,86,0.3)">` +
      `<td style="padding:4px 8px;color:var(--fg);max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="${esc(t.command)}">${esc(t.command)}</td>` +
      `<td style="padding:4px 8px;color:${stateColor};font-weight:600;text-transform:uppercase;font-size:10px">${esc(t.state)}</td>` +
      `<td style="padding:4px 8px;color:var(--muted)">${created}</td>` +
      `<td style="padding:4px 8px;color:var(--fg);font-size:10px;word-break:break-all">${output || '<span style="color:var(--muted)">—</span>'}</td>` +
      `</tr>`;
  }).join('');
}

async function beaconQueueTask() {
  const input = document.getElementById('beacon-cmd');
  if (!input || !S.ws.id) return;
  const cmd = input.value.trim();
  if (!cmd) return;
  input.disabled = true;
  try {
    await api('POST', `/api/v1/beacons/${S.ws.id}/tasks`, { command: cmd });
    input.value = '';
    beaconLoadTasks();
  } catch (err) {
    showToast(`Task error: ${err.message}`, 'danger');
  } finally {
    input.disabled = false;
    input.focus();
  }
}
window.beaconQueueTask = beaconQueueTask;

function wsQuick(cmd) {
  if (S.ws._deadNotified || !cmd) return;
  if (S.ws._execCmd) {
    if (S.ws._term) S.ws._term.write(cmd);
    S.ws._execCmd(cmd);
  }
}

async function wsScreenshot() {
  if (!S.ws.id || S.ws._deadNotified) return;
  const btn = document.querySelector('#ws-qa .qa[onclick*="wsScreenshot"]');
  if (btn) { btn.disabled = true; btn.textContent = 'capturing…'; }
  try {
    const resp = await apiFetch(`/api/v1/sessions/${S.ws.id}/actions`, {
      method: 'POST',
      body: JSON.stringify({ action: 'screenshot' }),
    });
    const data = await resp.json();
    if (data.ok && data.output) {
      const imgResp = await apiFetch(data.output);
      if (!imgResp.ok) throw new Error(`image fetch failed: ${imgResp.status}`);
      const blob = await imgResp.blob();
      const blobUrl = URL.createObjectURL(blob);
      if (!S.ws._blobUrls) S.ws._blobUrls = [];
      S.ws._blobUrls.push(blobUrl);

      const overlay = document.createElement('div');
      overlay.id = 'screenshot-overlay';
      overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,.85);display:flex;flex-direction:column;align-items:center;justify-content:center;z-index:9999;gap:12px';
      const img = document.createElement('img');
      img.src = blobUrl;
      img.style.cssText = 'max-width:90vw;max-height:75vh;border:2px solid #d4af37;border-radius:6px';
      const bar = document.createElement('div');
      bar.style.cssText = 'display:flex;gap:12px';
      const dlBtn = document.createElement('a');
      dlBtn.href = blobUrl;
      dlBtn.download = 'screenshot.png';
      dlBtn.textContent = 'Download';
      dlBtn.style.cssText = 'padding:6px 18px;background:#d4af37;color:#0a0a08;border-radius:4px;text-decoration:none;font-weight:600;font-size:13px';
      const closeBtn = document.createElement('button');
      closeBtn.textContent = 'Close';
      closeBtn.style.cssText = 'padding:6px 18px;background:transparent;color:#e8e8e6;border:1px solid #5a5a56;border-radius:4px;cursor:pointer;font-size:13px';
      closeBtn.onclick = () => overlay.remove();
      overlay.onclick = (e) => { if (e.target === overlay) overlay.remove(); };
      bar.append(dlBtn, closeBtn);
      overlay.append(img, bar);
      document.body.appendChild(overlay);

      if (S.ws._term) S.ws._term.write(`\r\n\x1b[33m[screenshot captured]\x1b[0m\r\n`);
    } else {
      if (S.ws._term) S.ws._term.write(`\r\n\x1b[31m[screenshot failed]\x1b[0m\r\n`);
    }
  } catch (err) {
    if (S.ws._term) S.ws._term.write(`\r\n\x1b[31m[screenshot error: ${err.message}]\x1b[0m\r\n`);
  } finally {
    if (btn) { btn.disabled = false; btn.textContent = 'screenshot'; }
  }
}

async function wsSubmit(e) {
  if (e) e.preventDefault();
}


/* ════════════════════════════════════
   REAL-TIME EVENTS (WebSocket)
════════════════════════════════════ */
function connectEventStream() {
  if (S._evtWs && S._evtWs.readyState <= 1) return;
  const token = getToken();
  if (!token) return;
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const url = `${proto}//${location.host}/api/v1/events?token=${encodeURIComponent(token)}`;
  const ws = new WebSocket(url);

  ws.onopen = () => {
    S._evtBackoff = 1000;
    stopPolling();
    poll();
  };

  ws.onmessage = (e) => {
    try {
      const evt = JSON.parse(e.data);
      handleServerEvent(evt);
    } catch(_) {}
  };

  ws.onclose = () => {
    S._evtWs = null;
    startPolling();
    clearTimeout(S._evtReconnect);
    S._evtReconnect = setTimeout(() => {
      if (getToken()) connectEventStream();
    }, Math.min(S._evtBackoff, 15000));
    S._evtBackoff = Math.min(S._evtBackoff * 1.5, 15000);
  };

  ws.onerror = () => { ws.close(); };
  S._evtWs = ws;
}

function handleServerEvent(evt) {
  switch (evt.type) {
    case 'session.new': {
      const d = evt.data || {};
      showToast(`New session: ${d.username || '?'}@${d.hostname || '?'}`, 'session');
      poll();
      break;
    }
    case 'session.dead': {
      const d = evt.data || {};
      const sid = d.id || '?';
      showToast(`Session lost: ${sid.slice(0,8)}`, 'danger');
      poll();
      break;
    }
    case 'beacon.checkin': {
      const d = evt.data || {};
      showToast(`Beacon: ${d.username || '?'}@${d.hostname || '?'}`, 'session');
      poll();
      if (S.ws.open && S.ws.type === 'beacon') beaconLoadTasks();
      break;
    }
    case 'session.updated': {
      const d = evt.data || {};
      if (d.name) showToast(`Session renamed: ${d.name}`, 'info');
      poll();
      break;
    }
    case 'beacon.renamed': {
      const d = evt.data || {};
      if (d.name) showToast(`Beacon renamed: ${d.name}`, 'info');
      poll();
      break;
    }
    case 'beacon.taskresult': {
      if (S.ws.open && S.ws.type === 'beacon') beaconLoadTasks();
      break;
    }
    case 'listener.started': {
      const d = evt.data || {};
      showToast(`Listener started: ${(d.protocol || '').toUpperCase()} :${d.port || '?'}`, 'info');
      poll();
      break;
    }
    case 'listener.stopped': {
      const d = evt.data || {};
      showToast(`Listener stopped: ${d.name || d.id || '?'}`, 'info');
      poll();
      break;
    }
    case 'build.completed': {
      const d = evt.data || {};
      showToast(`Build complete: ${d.name || 'implant'}`, 'info');
      break;
    }
    default:
      poll();
  }
}

/* ────────────── TOAST NOTIFICATIONS ────────────── */
function showToast(msg, kind) {
  const container = document.getElementById('toast-stack');
  if (!container) return;
  const div = document.createElement('div');
  div.className = 'kj-toast kj-toast-' + (kind || 'info');
  const icon = kind === 'danger' ? '⚠' : kind === 'session' ? '📡' : 'ℹ';
  div.innerHTML = `<span class="kj-toast-icon">${icon}</span><span class="kj-toast-msg">${esc(msg)}</span>`;
  container.appendChild(div);
  requestAnimationFrame(() => div.classList.add('show'));
  setTimeout(() => {
    div.classList.remove('show');
    div.classList.add('hide');
    setTimeout(() => div.remove(), 400);
  }, 5000);
}

/* ════════════════════════════════════
   INIT
════════════════════════════════════ */
// Auth gate: redirect to login if no token
if (!getToken()) {
  window.location.href = '/login';
}

// Show stored username immediately
(function() {
  var u = localStorage.getItem('kj_username');
  var el = document.getElementById('hdr-uname');
  if (u && el) el.textContent = u;
})();

/* ════════════════════════════════════
   COMMAND PALETTE  (Ctrl+K)
════════════════════════════════════ */
const CMD_ITEMS = [
  { id: 'execute-assembly', icon: '🔧', label: 'Execute Assembly',  desc: 'Load & run .NET assembly',   os: 'windows' },
  { id: 'sideload',         icon: '📦', label: 'Sideload',          desc: 'Inject DLL/EXE into process', os: 'windows' },
  { id: 'inject',           icon: '💉', label: 'Shellcode Inject',  desc: 'Inject raw shellcode' },
  { id: 'ps',               icon: '📋', label: 'Process List',      desc: 'List running processes' },
  { id: 'getsystem',        icon: '⬆',  label: 'GetSystem',         desc: 'Elevate to SYSTEM',          os: 'windows' },
];

let cmdActiveIdx = 0;

function openCmdPalette() {
  if (!S.ws.open) return;
  const palette = document.getElementById('cmd-palette');
  const search = document.getElementById('cmd-search');
  if (!palette) return;
  cmdActiveIdx = 0;
  palette.classList.add('open');
  search.value = '';
  renderCmdList('');
  setTimeout(() => search.focus(), 30);
}

function closeCmdPalette() {
  document.getElementById('cmd-palette')?.classList.remove('open');
}

function renderCmdList(query) {
  const list = document.getElementById('cmd-list');
  if (!list) return;
  const q = query.toLowerCase();
  const sessionOS = (S.ws.os || '').toLowerCase();
  const filtered = q ? CMD_ITEMS.filter(c => c.label.toLowerCase().includes(q) || c.desc.toLowerCase().includes(q)) : CMD_ITEMS;
  if (cmdActiveIdx >= filtered.length) cmdActiveIdx = Math.max(0, filtered.length - 1);
  list.innerHTML = filtered.length === 0
    ? `<div style="padding:20px;text-align:center;color:var(--muted);font-size:12px">No matching commands</div>`
    : filtered.map((c, i) => {
      const disabled = c.os && c.os !== sessionOS;
      return `<div class="cmd-item${i === cmdActiveIdx ? ' active' : ''}${disabled ? ' disabled' : ''}" data-cmd="${c.id}" ${disabled ? '' : `onclick="selectCmd('${c.id}')"`} style="${disabled ? 'opacity:0.35;cursor:not-allowed;pointer-events:auto' : ''}">` +
      `<span class="cmd-item-icon">${c.icon}</span>` +
      `<span class="cmd-item-label">${esc(c.label)}${disabled ? ' <span style="font-size:10px;color:var(--muted);font-weight:400">(Windows only)</span>' : ''}</span>` +
      `<span class="cmd-item-desc">${esc(c.desc)}</span>` +
      `</div>`;
    }).join('');
}

function selectCmd(id) {
  closeCmdPalette();
  openExecModal(id);
}
window.selectCmd = selectCmd;

document.addEventListener('keydown', (e) => {
  const palette = document.getElementById('cmd-palette');
  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault();
    if (palette?.classList.contains('open')) closeCmdPalette();
    else openCmdPalette();
    return;
  }
  if (!palette?.classList.contains('open')) return;
  const q = document.getElementById('cmd-search')?.value || '';
  const filtered = q.toLowerCase()
    ? CMD_ITEMS.filter(c => c.label.toLowerCase().includes(q.toLowerCase()) || c.desc.toLowerCase().includes(q.toLowerCase()))
    : CMD_ITEMS;
  if (e.key === 'Escape') { closeCmdPalette(); e.preventDefault(); return; }
  if (e.key === 'ArrowDown') { cmdActiveIdx = Math.min(cmdActiveIdx + 1, filtered.length - 1); renderCmdList(q); e.preventDefault(); return; }
  if (e.key === 'ArrowUp') { cmdActiveIdx = Math.max(cmdActiveIdx - 1, 0); renderCmdList(q); e.preventDefault(); return; }
  if (e.key === 'Enter' && filtered.length > 0) {
    const item = filtered[cmdActiveIdx];
    const sessionOS = (S.ws.os || '').toLowerCase();
    if (item.os && item.os !== sessionOS) return;
    selectCmd(item.id); e.preventDefault(); return;
  }
});

document.getElementById('cmd-search')?.addEventListener('input', (e) => {
  cmdActiveIdx = 0;
  renderCmdList(e.target.value);
});

document.getElementById('cmd-palette')?.addEventListener('click', (e) => {
  if (e.target.id === 'cmd-palette') closeCmdPalette();
});

/* ════════════════════════════════════
   EXECUTION MODAL
════════════════════════════════════ */
let execTarget = { type: null, id: null };

function openExecModal(cmdId) {
  if (!S.ws.open) return;
  execTarget = { type: S.ws.type, id: S.ws.id };
  const sel = document.getElementById('exec-type');
  if (sel) sel.value = cmdId;
  onExecTypeChange();
  document.getElementById('exec-output')?.classList.remove('visible');
  document.getElementById('exec-spinner')?.classList.remove('visible');
  document.getElementById('exec-ps-picker')?.classList.remove('visible');
  const fileInput = document.getElementById('exec-file');
  if (fileInput) fileInput.value = '';
  const argsInput = document.getElementById('exec-args');
  if (argsInput) argsInput.value = '';
  const pidInput = document.getElementById('exec-pid');
  if (pidInput) pidInput.value = '';
  const entryInput = document.getElementById('exec-entry');
  if (entryInput) entryInput.value = '';
  const processInput = document.getElementById('exec-process');
  if (processInput) processInput.value = '';
  openOverlay('ov-exec');
}
window.openExecModal = openExecModal;

function onExecTypeChange() {
  const t = document.getElementById('exec-type')?.value;
  const fileField = document.getElementById('exec-file-field');
  const argsField = document.getElementById('exec-args-field');
  const pidField = document.getElementById('exec-pid-field');
  const entryField = document.getElementById('exec-entry-field');
  const processField = document.getElementById('exec-process-field');
  const fileLabel = document.getElementById('exec-file-label');
  const submitBtn = document.getElementById('exec-submit');

  // Hide all optional fields first
  [fileField, argsField, pidField, entryField, processField].forEach(f => { if (f) f.style.display = 'none'; });
  document.getElementById('exec-ps-picker')?.classList.remove('visible');

  switch (t) {
    case 'execute-assembly':
      if (fileField) fileField.style.display = '';
      if (argsField) argsField.style.display = '';
      if (fileLabel) fileLabel.textContent = 'File (.NET assembly / DLL)';
      if (submitBtn) submitBtn.textContent = 'Execute Assembly';
      break;
    case 'sideload':
      if (fileField) fileField.style.display = '';
      if (argsField) argsField.style.display = '';
      if (entryField) entryField.style.display = '';
      if (fileLabel) fileLabel.textContent = 'File (DLL / EXE)';
      if (submitBtn) submitBtn.textContent = 'Sideload';
      break;
    case 'inject':
      if (fileField) fileField.style.display = '';
      if (pidField) pidField.style.display = '';
      if (fileLabel) fileLabel.textContent = 'File (raw shellcode .bin)';
      if (submitBtn) submitBtn.textContent = 'Inject Shellcode';
      break;
    case 'ps':
      if (submitBtn) submitBtn.textContent = 'List Processes';
      break;
    case 'getsystem':
      if (processField) processField.style.display = '';
      if (submitBtn) submitBtn.textContent = 'GetSystem';
      break;
  }
}
window.onExecTypeChange = onExecTypeChange;

async function loadProcessPicker() {
  if (!execTarget.id) return;
  const picker = document.getElementById('exec-ps-picker');
  if (!picker) return;
  picker.classList.add('visible');
  picker.innerHTML = '<div style="padding:10px;color:var(--muted);font-size:11px;text-align:center">Loading processes…</div>';
  try {
    const endpoint = execTarget.type === 'beacon'
      ? `/api/v1/beacons/${execTarget.id}/ps`
      : `/api/v1/sessions/${execTarget.id}/ps`;
    const data = await api('POST', endpoint, {});
    if (data.processes) {
      const procs = data.processes;
      picker.innerHTML =
        `<table><thead><tr><th>PID</th><th>Name</th><th>Owner</th><th>Arch</th></tr></thead>` +
        `<tbody>${procs.map(p =>
          `<tr onclick="pickProcess(${p.pid})">` +
          `<td class="mono">${p.pid}</td>` +
          `<td>${esc(p.executable)}</td>` +
          `<td>${esc(p.owner)}</td>` +
          `<td>${esc(p.architecture)}</td>` +
          `</tr>`
        ).join('')}</tbody></table>`;
    } else if (data.id) {
      picker.innerHTML = `<div style="padding:10px;color:var(--gold);font-size:11px;text-align:center">Task queued (beacon). Check tasks for results.</div>`;
    }
  } catch (err) {
    picker.innerHTML = `<div style="padding:10px;color:var(--err);font-size:11px">${esc(err.message)}</div>`;
  }
}
window.loadProcessPicker = loadProcessPicker;

function pickProcess(pid) {
  const input = document.getElementById('exec-pid');
  if (input) input.value = pid;
  document.getElementById('exec-ps-picker')?.classList.remove('visible');
}
window.pickProcess = pickProcess;

async function doExecSubmit() {
  const t = document.getElementById('exec-type')?.value;
  const outputEl = document.getElementById('exec-output');
  const spinner = document.getElementById('exec-spinner');
  const submitBtn = document.getElementById('exec-submit');
  if (!execTarget.id || !t) return;

  if (outputEl) { outputEl.textContent = ''; outputEl.classList.remove('visible'); outputEl.style.color = ''; }
  if (spinner) spinner.classList.add('visible');
  if (submitBtn) { submitBtn.disabled = true; submitBtn.style.opacity = '0.4'; }

  const isBeacon = execTarget.type === 'beacon';
  const prefix = isBeacon ? `/api/v1/beacons/${execTarget.id}` : `/api/v1/sessions/${execTarget.id}`;

  const abort = new AbortController();
  const timeoutMs = (t === 'getsystem') ? 180000 : 35000;
  const timer = setTimeout(() => abort.abort(), timeoutMs);

  try {
    let result;
    switch (t) {
      case 'execute-assembly': {
        const fileInput = document.getElementById('exec-file');
        if (!fileInput?.files?.length) throw new Error('Select a .NET assembly file');
        const fd = new FormData();
        fd.append('file', fileInput.files[0]);
        fd.append('arguments', document.getElementById('exec-args')?.value || '');
        const resp = await apiFetch(`${prefix}/execute-assembly`, { method: 'POST', body: fd, signal: abort.signal });
        if (!resp.ok) throw new Error(await resp.text());
        result = await resp.json();
        break;
      }
      case 'sideload': {
        const fileInput = document.getElementById('exec-file');
        if (!fileInput?.files?.length) throw new Error('Select a DLL/EXE file');
        const fd = new FormData();
        fd.append('file', fileInput.files[0]);
        fd.append('args', document.getElementById('exec-args')?.value || '');
        fd.append('entryPoint', document.getElementById('exec-entry')?.value || '');
        const resp = await apiFetch(`${prefix}/sideload`, { method: 'POST', body: fd, signal: abort.signal });
        if (!resp.ok) throw new Error(await resp.text());
        result = await resp.json();
        break;
      }
      case 'inject': {
        const fileInput = document.getElementById('exec-file');
        if (!fileInput?.files?.length) throw new Error('Select a shellcode file');
        const fd = new FormData();
        fd.append('file', fileInput.files[0]);
        fd.append('pid', document.getElementById('exec-pid')?.value || '0');
        const resp = await apiFetch(`${prefix}/inject`, { method: 'POST', body: fd, signal: abort.signal });
        if (!resp.ok) throw new Error(await resp.text());
        result = await resp.json();
        break;
      }
      case 'ps': {
        result = await api('POST', `${prefix}/ps`, {}, abort.signal);
        break;
      }
      case 'getsystem': {
        const hostProc = document.getElementById('exec-process')?.value || 'spoolsv.exe';
        result = await api('POST', `${prefix}/getsystem`, { hostingProcess: hostProc }, abort.signal);
        break;
      }
    }

    if (outputEl) {
      outputEl.classList.add('visible');
      if (result.ok === false && result.error) {
        outputEl.textContent = `Error: ${result.error}${result.output ? '\n\n' + result.output : ''}`;
        outputEl.style.color = 'var(--err)';
      } else if (result.processes) {
        const lines = [`PID      PPID     EXECUTABLE                     OWNER`];
        for (const p of result.processes) {
          lines.push(`${String(p.pid).padEnd(8)} ${String(p.ppid).padEnd(8)} ${(p.executable || '').padEnd(30)} ${p.owner || ''}`);
        }
        outputEl.textContent = lines.join('\n');
      } else if (result.output) {
        outputEl.textContent = result.output;
      } else if (result.id || result.command) {
        outputEl.textContent = `Task queued: ${result.command || result.id}\nState: ${result.state || 'pending'}`;
      } else {
        outputEl.textContent = JSON.stringify(result, null, 2);
      }
    }
    if (result.ok === false) showToast(`Execution error: ${result.error}`, 'danger');
    else if (isBeacon) showToast(`Task queued: ${t}`, 'info');
    else showToast(`${t} executed`, 'info');

  } catch (err) {
    let msg = err.name === 'AbortError' ? `${t} timed out (35s). The implant may lack privileges or is unresponsive.` : err.message;
    try { const j = JSON.parse(msg); msg = j.detail || j.message || msg; } catch(_) {}
    if (outputEl) { outputEl.classList.add('visible'); outputEl.textContent = `Error: ${msg}`; outputEl.style.color = 'var(--err)'; }
    showToast(`Execution failed: ${msg}`, 'danger');
  } finally {
    clearTimeout(timer);
    if (spinner) spinner.classList.remove('visible');
    if (submitBtn) { submitBtn.disabled = false; submitBtn.style.opacity = ''; }
  }
}
window.doExecSubmit = doExecSubmit;

/* ════════════════════════════════════
   WORKSPACE TABS
════════════════════════════════════ */

function wsSetTab(tab) {
  document.querySelectorAll('#ws-tabs .ws-tab').forEach(t => t.classList.toggle('active', t.dataset.tab === tab));
  S.ws._activeTab = tab;
  if (tab === 'terminal') {
    document.getElementById('ws-qa').style.display = '';
    renderTerminalTab();
  } else if (tab === 'files') {
    document.getElementById('ws-qa').style.display = 'none';
    renderFilesTab();
  }
}
window.wsSetTab = wsSetTab;

function renderTerminalTab() {
  const { type, id } = S.ws;
  const src = type === 'beacon'
    ? S.beacons.find(b => (b.id ?? b.ID) === id)
    : S.sessions.find(s => (s.id ?? s.ID) === id);
  const os = (src?.os ?? src?.OS ?? '').toLowerCase();
  const isWin = os.includes('win');

  if (type === 'beacon') {
    document.getElementById('ws-qa').style.display = '';
    document.getElementById('ws-qa').innerHTML =
      `<div style="display:flex;gap:6px;align-items:center;width:100%">` +
      `<input id="beacon-cmd" type="text" placeholder="Type command to queue…" style="flex:1;padding:5px 10px;background:var(--bg);border:1px solid var(--border);color:var(--fg);border-radius:4px;font-family:var(--mono);font-size:12px" />` +
      `<button class="qa" onclick="beaconQueueTask()" style="border-color:var(--gold-dim);color:var(--gold);white-space:nowrap">Queue Task</button>` +
      `<button class="qa" onclick="openCmdPalette()" style="border-color:var(--gold-dim);color:var(--gold);white-space:nowrap" title="Ctrl+K">⌘ Exec</button>` +
      `</div>`;
    document.getElementById('ws-body').innerHTML =
      `<div style="flex:1;overflow-y:auto;padding:14px 18px;font-family:var(--mono);font-size:12px;line-height:1.65">` +
      `<div class="out-sys" style="margin-bottom:8px">Beacon tasking — async check-in every ${src?.interval ?? '?'}s</div>` +
      `<table id="beacon-tasks-table" style="width:100%;border-collapse:collapse;font-size:11px">` +
      `<thead><tr style="text-align:left;color:var(--gold);border-bottom:1px solid var(--border)">` +
      `<th style="padding:4px 8px">Command</th><th style="padding:4px 8px;width:80px">State</th>` +
      `<th style="padding:4px 8px;width:140px">Created</th><th style="padding:4px 8px">Output</th>` +
      `</tr></thead><tbody id="beacon-tasks-body"></tbody></table></div>`;
    const cmdInput = document.getElementById('beacon-cmd');
    if (cmdInput) cmdInput.addEventListener('keydown', e => { if (e.key === 'Enter') beaconQueueTask(); });
    beaconLoadTasks();
  } else {
    const qaButtons = [
      { label:'whoami', cmd:'whoami' }, { label:'pwd', cmd:'pwd' },
      { label:'ls / dir', cmd: isWin ? 'dir' : 'ls -la' },
      { label:'ps / tasklist', cmd: isWin ? 'tasklist' : 'ps aux' },
      { label:'ipconfig/ifconfig', cmd: isWin ? 'ipconfig /all' : 'ifconfig' },
      { label:'uname -a', cmd:'uname -a' },
    ].map(a => `<button class="qa" onclick='wsQuick(${JSON.stringify(a.cmd)})'>${esc(a.label)}</button>`).join('');
    const statelessHint = `<span style="font-size:10px;color:var(--muted);margin-left:8px;opacity:0.7">Stateless</span>`;
    const screenshotBtn = `<button class="qa" onclick='wsScreenshot()' style="border-color:var(--gold-dim);color:var(--gold)">screenshot</button>`;
    const execBtn = `<button class="qa" onclick='openCmdPalette()' style="margin-left:auto;border-color:var(--gold-dim);color:var(--gold)" title="Ctrl+K">⌘ Exec</button>`;
    document.getElementById('ws-qa').innerHTML = qaButtons + screenshotBtn + statelessHint + execBtn;
    document.getElementById('ws-body').innerHTML = '<div id="ws-term"></div>';
    initShellTerminal(id);
  }
}

/* ════════════════════════════════════
   FILE BROWSER (Sprint 9)
════════════════════════════════════ */

const FB = { cwd: '', files: [], loading: false };

function renderFilesTab() {
  const body = document.getElementById('ws-body');
  body.innerHTML = `
    <div id="file-browser">
      <div id="fb-toolbar">
        <div id="fb-breadcrumb"></div>
        <button class="fb-btn" onclick="fbGoUp()" title="Go up one level">&#x2191; Up</button>
        <button class="fb-btn" onclick="fbMkdir()" title="New folder">+ Folder</button>
        <button class="fb-btn" onclick="document.getElementById('fb-upload-input').click()" title="Upload file">Upload</button>
        <button class="fb-btn" onclick="fbRefresh()">Refresh</button>
        <input type="file" id="fb-upload-input" style="display:none" onchange="fbUploadFile(this)" multiple />
      </div>
      <div id="fb-table-wrap">
        <table id="fb-table">
          <thead><tr>
            <th style="width:45%">Name</th><th style="width:15%">Size</th>
            <th style="width:18%">Modified</th><th style="width:12%">Perms</th><th style="width:10%">Actions</th>
          </tr></thead>
          <tbody id="fb-tbody"></tbody>
        </table>
      </div>
      <div id="fb-upload-zone" onclick="document.getElementById('fb-upload-input').click()">
        Drop files here or click to upload
      </div>
      <div id="fb-status"></div>
    </div>`;

  const zone = document.getElementById('fb-upload-zone');
  zone.addEventListener('dragover', e => { e.preventDefault(); zone.classList.add('dragover'); });
  zone.addEventListener('dragleave', () => zone.classList.remove('dragover'));
  zone.addEventListener('drop', e => { e.preventDefault(); zone.classList.remove('dragover'); fbUploadDropped(e.dataTransfer.files); });

  if (!FB.cwd) {
    fbPwd();
  } else {
    fbLs(FB.cwd);
  }
}

function fbApiBase() {
  const { type, id } = S.ws;
  return `/api/v1/${type === 'beacon' ? 'beacons' : 'sessions'}/${id}/files`;
}

async function fbPwd() {
  fbSetStatus('Getting current directory…');
  try {
    const r = await api('POST', `${fbApiBase()}/pwd`, {});
    FB.cwd = r.path || '';
    fbLs(FB.cwd);
  } catch (e) {
    fbSetStatus(`Error: ${e.message}`);
  }
}

async function fbLs(path) {
  FB.loading = true;
  fbSetStatus('Loading…');
  try {
    const r = await api('POST', `${fbApiBase()}/ls`, { path: path || '' });
    FB.cwd = r.path || path || '';
    FB.files = (r.files || []).sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    fbRenderBreadcrumb();
    fbRenderTable();
    fbSetStatus(`${FB.files.length} items`);
  } catch (e) {
    fbSetStatus(`Error: ${e.message}`);
  } finally {
    FB.loading = false;
  }
}

function fbRenderBreadcrumb() {
  const el = document.getElementById('fb-breadcrumb');
  if (!el) return;
  const sep = FB.cwd.includes('/') ? '/' : '\\';
  let parts = FB.cwd.split(/[/\\]/).filter(Boolean);
  let root = sep === '\\' ? (parts.length ? parts.shift() + '\\' : 'C:\\') : '/';
  let crumbs = `<span class="fb-crumb" onclick="fbNavigate('${escJS(root)}')">${esc(root)}</span>`;
  let accumulated = root;
  for (const p of parts) {
    accumulated += p + sep;
    crumbs += `<span class="fb-sep">${esc(sep === '\\' ? '›' : '/')}</span>`;
    crumbs += `<span class="fb-crumb" onclick="fbNavigate('${escJS(accumulated)}')">${esc(p)}</span>`;
  }
  el.innerHTML = crumbs;
}

function fbRenderTable() {
  const tbody = document.getElementById('fb-tbody');
  if (!tbody) return;
  if (FB.files.length === 0) {
    tbody.innerHTML = `<tr><td colspan="5" style="text-align:center;color:var(--muted);padding:30px">Empty directory</td></tr>`;
    return;
  }
  const sep = FB.cwd.includes('/') ? '/' : '\\';
  let rows = `<tr>
    <td><span class="fb-icon">📁</span><span class="fb-name-link" onclick="fbGoUp()">..</span></td>
    <td></td><td></td><td></td><td></td>
  </tr>`;
  for (const f of FB.files) {
    const icon = f.isDir ? '📁' : '📄';
    const size = f.isDir ? '—' : fmtSize(f.size);
    const mod = f.modTime ? fmtDate(f.modTime) : '—';
    const mode = f.mode || '—';
    const fullPath = FB.cwd + (FB.cwd.endsWith(sep) ? '' : sep) + f.name;
    const nameClick = f.isDir
      ? `onclick="fbNavigate('${escJS(fullPath)}')" class="fb-name-link"`
      : `onclick="fbPreview('${escJS(fullPath)}','${escJS(f.name)}',${f.size})" class="fb-name-link"`;
    const actions = f.isDir
      ? `<button class="fb-btn fb-btn-danger" onclick="fbRm('${escJS(fullPath)}',true)">rm</button>`
      : `<button class="fb-btn" onclick="fbDownload('${escJS(fullPath)}')">dl</button>
         <button class="fb-btn fb-btn-danger" onclick="fbRm('${escJS(fullPath)}',false)">rm</button>`;
    rows += `<tr>
      <td><span class="fb-icon">${icon}</span><span ${nameClick}>${esc(f.name)}</span></td>
      <td style="color:var(--sub)">${size}</td>
      <td style="color:var(--sub)">${mod}</td>
      <td style="color:var(--sub)">${mode}</td>
      <td class="fb-actions">${actions}</td>
    </tr>`;
  }
  tbody.innerHTML = rows;
}

function fbNavigate(path) {
  if (FB.loading) return;
  FB.cwd = path;
  fbLs(path);
}
window.fbNavigate = fbNavigate;

function fbGoUp() {
  if (FB.loading) return;
  const sep = FB.cwd.includes('/') ? '/' : '\\';
  let parts = FB.cwd.replace(/[/\\]$/, '').split(/[/\\]/);
  if (parts.length <= 1) return;
  parts.pop();
  let parent = parts.join(sep);
  if (sep === '\\' && parent.length === 2 && parent[1] === ':') parent += '\\';
  if (sep === '/' && parent === '') parent = '/';
  fbNavigate(parent);
}
window.fbGoUp = fbGoUp;

async function fbDownload(path) {
  fbSetStatus(`Downloading ${path}…`);
  try {
    const token = getToken();
    const resp = await fetch(`${fbApiBase()}/download`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      body: JSON.stringify({ path }),
    });
    if (!resp.ok) {
      const err = await resp.json().catch(() => ({}));
      throw new Error(err.detail || `HTTP ${resp.status}`);
    }
    const blob = await resp.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = path.split(/[/\\]/).pop() || 'download';
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    fbSetStatus(`Downloaded ${a.download}`);
  } catch (e) {
    fbSetStatus(`Download error: ${e.message}`);
    showToast(`Download failed: ${e.message}`, 'danger');
  }
}
window.fbDownload = fbDownload;

async function fbUploadFile(input) {
  const files = input.files;
  if (!files || files.length === 0) return;
  await fbUploadDropped(files);
  input.value = '';
}
window.fbUploadFile = fbUploadFile;

async function fbUploadDropped(fileList) {
  const sep = FB.cwd.includes('/') ? '/' : '\\';
  for (const file of fileList) {
    const destPath = FB.cwd + (FB.cwd.endsWith(sep) ? '' : sep) + file.name;
    fbSetStatus(`Uploading ${file.name}…`);
    try {
      const token = getToken();
      const form = new FormData();
      form.append('path', destPath);
      form.append('file', file);
      const resp = await fetch(`${fbApiBase()}/upload`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` },
        body: form,
      });
      if (!resp.ok) {
        const err = await resp.json().catch(() => ({}));
        throw new Error(err.detail || `HTTP ${resp.status}`);
      }
      showToast(`Uploaded ${file.name}`, 'info');
    } catch (e) {
      showToast(`Upload failed: ${e.message}`, 'danger');
    }
  }
  fbLs(FB.cwd);
}

async function fbMkdir() {
  const name = prompt('New folder name:');
  if (!name) return;
  const sep = FB.cwd.includes('/') ? '/' : '\\';
  const path = FB.cwd + (FB.cwd.endsWith(sep) ? '' : sep) + name;
  fbSetStatus(`Creating ${name}…`);
  try {
    await api('POST', `${fbApiBase()}/mkdir`, { path });
    showToast(`Created folder: ${name}`, 'info');
    fbLs(FB.cwd);
  } catch (e) {
    showToast(`mkdir failed: ${e.message}`, 'danger');
    fbSetStatus(`Error: ${e.message}`);
  }
}
window.fbMkdir = fbMkdir;

async function fbRm(path, recursive) {
  const name = path.split(/[/\\]/).pop();
  if (!confirm(`Delete ${name}?${recursive ? ' (recursive)' : ''}`)) return;
  fbSetStatus(`Deleting ${name}…`);
  try {
    await api('POST', `${fbApiBase()}/rm`, { path, recursive, force: true });
    showToast(`Deleted: ${name}`, 'info');
    fbLs(FB.cwd);
  } catch (e) {
    showToast(`Delete failed: ${e.message}`, 'danger');
    fbSetStatus(`Error: ${e.message}`);
  }
}
window.fbRm = fbRm;

async function fbPreview(path, name, size) {
  if (size > 1024 * 1024) { fbDownload(path); return; }
  const ext = name.split('.').pop().toLowerCase();
  const textExts = ['txt','log','md','json','xml','yaml','yml','toml','ini','cfg','conf','bat','ps1','sh','py','go','js','css','html','csv','env'];
  if (!textExts.includes(ext)) { fbDownload(path); return; }
  fbSetStatus(`Reading ${name}…`);
  try {
    const token = getToken();
    const resp = await fetch(`${fbApiBase()}/download`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      body: JSON.stringify({ path }),
    });
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const text = await resp.text();
    const body = document.getElementById('ws-body');
    body.innerHTML = `
      <div style="flex:1;display:flex;flex-direction:column;overflow:hidden">
        <div style="padding:8px 16px;background:var(--s1);border-bottom:1px solid var(--bd);display:flex;align-items:center;gap:10px">
          <span style="color:var(--gold);font-family:var(--mono);font-size:12px">${esc(name)}</span>
          <span style="color:var(--muted);font-size:11px">${fmtSize(size)}</span>
          <span style="flex:1"></span>
          <button class="fb-btn" onclick="fbDownload('${escJS(path)}')">Download</button>
          <button class="fb-btn" onclick="renderFilesTab()">Back</button>
        </div>
        <pre style="flex:1;overflow:auto;padding:14px 18px;margin:0;font-family:var(--mono);font-size:12px;line-height:1.5;color:var(--sub);white-space:pre-wrap;word-break:break-all">${esc(text)}</pre>
      </div>`;
    fbSetStatus('');
  } catch (e) { fbSetStatus(`Preview error: ${e.message}`); }
}
window.fbPreview = fbPreview;

function fbRefresh() { fbLs(FB.cwd); }
window.fbRefresh = fbRefresh;

function fbSetStatus(msg) {
  const el = document.getElementById('fb-status');
  if (el) el.textContent = msg;
}

function fmtSize(bytes) {
  if (bytes == null || bytes < 0) return '—';
  if (bytes === 0) return '0 B';
  const units = ['B','KB','MB','GB','TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return (bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
}

/* ════════════════════════════════════
   INIT (final)
════════════════════════════════════ */
window.addEventListener('beforeunload', () => {
  stopPolling();
  if (S._evtWs) { S._evtWs.close(); S._evtWs = null; }
  if (S._pollAbort) { S._pollAbort.abort(); S._pollAbort = null; }
});
setModule('sessions');
connectEventStream();
