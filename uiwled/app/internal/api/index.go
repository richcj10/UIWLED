package api

// indexHTML is UIWLED's custom WLED-inspired control UI.
// v3: mobile-responsive layout, categorised searchable effect grid, palette
// editor + preset save/load (localStorage). All controls drive the existing
// REST endpoints.
const indexHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<meta name="theme-color" content="#0f0f10">
<title>UIWLED</title>
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 48 48'><defs><linearGradient id='g' x1='0%25' y1='0%25' x2='100%25' y2='100%25'><stop offset='0%25' stop-color='%23ff3040'/><stop offset='33%25' stop-color='%23ffcc00'/><stop offset='66%25' stop-color='%2300cc66'/><stop offset='100%25' stop-color='%2300b7ff'/></linearGradient></defs><rect x='6' y='10' width='36' height='28' rx='4' fill='none' stroke='url(%23g)' stroke-width='4'/><rect x='14' y='16' width='20' height='16' rx='2' fill='%23181a1c'/><path d='M17 20V28M20 20V28M23 20V28M26 20V28M29 20V28M32 20V28' stroke='%238a919c' stroke-width='0.8'/></svg>">
<style>
  :root {
    --bg:#0f0f10; --panel:#181a1c; --panel2:#22252a; --border:#2c3037;
    --text:#e6e8eb; --muted:#8a919c; --accent:#00b7ff; --accent2:#7d5cff;
    --danger:#ff4d4d; --ok:#3fdd7a; --warn:#ffb840;
    --radius:10px; --hit:44px;
  }
  * { box-sizing:border-box; -webkit-tap-highlight-color:transparent; }
  html, body { margin:0; padding:0; background:var(--bg); color:var(--text);
    font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
    overscroll-behavior-y: contain;
  }

  header {
    position: sticky; top: 0; z-index: 10;
    padding:12px 16px; background:var(--panel); border-bottom:1px solid var(--border);
    display:flex; align-items:center; gap:12px; flex-wrap:wrap;
    padding-top: max(12px, env(safe-area-inset-top));
  }
  .brand { display:flex; align-items:center; gap:10px; text-decoration:none; color:inherit; }
  .brand .logo { width:32px; height:32px; display:block; }
  .brand .logo .ring { stroke-dasharray: 8 4; animation: spinDash 6s linear infinite; }
  .brand .wordmark { font-size:16px; letter-spacing:1.2px; font-weight:600;
    font-family: 'SF Mono', 'Menlo', ui-monospace, monospace; }
  .brand .wordmark .w { color:var(--accent); }
  .brand .wordmark .led { color:var(--accent2); font-weight:500; }
  @keyframes spinDash { to { stroke-dashoffset: -60; } }

  .switch-tabs { display:flex; gap:6px; margin-left:auto; overflow-x:auto; max-width:100%; }
  .tab {
    flex-shrink:0;
    background:transparent; color:var(--muted); border:1px solid var(--border);
    padding:8px 12px; border-radius:20px; cursor:pointer; font-size:13px;
    display:flex; align-items:center; gap:8px; min-height:36px;
  }
  .tab:hover { border-color:var(--accent); color:var(--text); }
  .tab.active { background:var(--accent); color:#001; border-color:var(--accent); }
  .status-dot { width:8px; height:8px; border-radius:50%; display:inline-block; background:var(--muted); flex-shrink:0; }
  .status-dot.online { background:var(--ok); }
  .status-dot.connecting { background:var(--warn); }
  .status-dot.offline { background:var(--danger); }

  main { max-width:1100px; margin:16px auto; padding:0 12px 40px; }
  .meta { color:var(--muted); font-size:12px; }
  .panel {
    background:var(--panel); border:1px solid var(--border); border-radius:var(--radius);
    padding:14px; margin-bottom:12px;
  }
  .panel h3 { margin:0 0 12px 0; font-size:11px; letter-spacing:1.5px;
    text-transform:uppercase; color:var(--muted); display:flex; align-items:center; justify-content:space-between; }

  .toolbar { display:flex; align-items:center; gap:10px; flex-wrap:wrap; }
  .btn {
    background:var(--panel2); color:var(--text); border:1px solid var(--border);
    padding:8px 14px; border-radius:6px; cursor:pointer; font-size:13px;
    min-height:var(--hit); min-width:44px;
  }
  .btn.sm { min-height:32px; padding:6px 10px; font-size:12px; }
  .btn:hover { border-color:var(--accent); }
  .btn.primary { background:var(--accent); color:#001; border-color:var(--accent); font-weight:600; }
  .btn.danger  { color:var(--danger); border-color:#5a2828; }
  .btn.on { background:var(--ok); color:#001; border-color:var(--ok); font-weight:600; }
  .btn.off { background:var(--panel2); color:var(--muted); }

  input[type=text], input[type=number] {
    background:var(--panel2); color:var(--text); border:1px solid var(--border);
    padding:8px 10px; border-radius:5px; font-size:13px; min-height:var(--hit);
  }
  input[type=text]:focus, input[type=number]:focus {
    outline:none; border-color:var(--accent);
  }

  .grid { display:grid; gap:12px; }
  .grid.cols-2 { grid-template-columns: 1fr 1fr; }
  @media (max-width: 720px) {
    .grid.cols-2 { grid-template-columns: 1fr; }
    header { padding:10px 12px; gap:8px; }
    .brand .wordmark { font-size:14px; }
    .brand .logo { width:28px; height:28px; }
    main { padding: 0 8px 40px; margin-top:12px; }
    .panel { padding:12px; }
  }

  /* Colors */
  .color-slots { display:flex; gap:8px; }
  .slot { flex:1; text-align:center; min-width:0; }
  .slot input[type=color] {
    width:100%; height:56px; border:2px solid var(--border);
    border-radius:8px; cursor:pointer; padding:0; background:transparent;
  }
  .slot input[type=color]:hover { border-color:var(--accent); }
  .slot input[type=color]::-webkit-color-swatch-wrapper { padding:0; border-radius:6px; }
  .slot input[type=color]::-webkit-color-swatch { border:none; border-radius:6px; }
  .slot input[type=color]::-moz-color-swatch { border:none; border-radius:6px; }
  .slot-label { color:var(--muted); font-size:11px; margin-top:6px; letter-spacing:.5px; }

  /* Palettes */
  .palette-row { display:grid; grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
    gap:6px; margin-top:8px; }
  .palette-chip {
    height:38px; border-radius:5px; cursor:pointer; border:1px solid var(--border);
    position:relative; display:flex; align-items:center; justify-content:center;
    overflow:hidden;
  }
  .palette-chip:hover { border-color:var(--accent); }
  .palette-chip .label { font-size:11px; color:#fff; text-shadow: 0 0 4px #000, 0 0 8px #000;
    z-index:1; padding:0 4px; text-align:center; font-weight:500; }
  .palette-chip .del {
    position:absolute; top:2px; right:4px; font-size:14px; color:#fff;
    text-shadow: 0 0 3px #000; line-height:1; opacity:0; cursor:pointer;
    padding:2px 4px; background:rgba(0,0,0,0.4); border-radius:3px;
  }
  .palette-chip:hover .del { opacity:1; }
  .palette-chip.add {
    background: var(--panel2); color: var(--muted); border-style:dashed;
    font-size:22px; font-weight:300;
  }
  .palette-chip.add:hover { color:var(--accent); }

  /* Effects — search + categories */
  .effect-header { display:flex; gap:8px; align-items:center; margin-bottom:12px; }
  .effect-search {
    flex:1; background:var(--panel2); color:var(--text); border:1px solid var(--border);
    padding:8px 10px; border-radius:5px; font-size:13px; min-height:36px;
  }
  .effect-search:focus { outline:none; border-color:var(--accent); }
  .effect-cat { margin-bottom:6px; }
  .effect-cat-title {
    color:var(--muted); font-size:11px; letter-spacing:1px; text-transform:uppercase;
    padding:6px 4px; cursor:pointer; user-select:none; display:flex; align-items:center; gap:6px;
  }
  .effect-cat-title::before { content:'▾'; font-size:10px; transition: transform .1s; }
  .effect-cat.collapsed .effect-cat-title::before { transform: rotate(-90deg); }
  .effect-cat.collapsed .effect-grid { display:none; }
  .effect-grid {
    display:grid; grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
    gap:6px;
  }
  .effect {
    background:var(--panel2); color:var(--text); border:1px solid var(--border);
    padding:9px 10px; border-radius:5px; cursor:pointer; font-size:12px;
    text-align:left; min-height:38px;
  }
  .effect:hover { border-color:var(--accent); }
  .effect.active { background:var(--accent); color:#001; border-color:var(--accent); font-weight:600; }

  /* Sliders */
  .slider-row { margin-bottom:10px; }
  .slider-row:last-child { margin-bottom:0; }
  .slider-row label { display:flex; justify-content:space-between; font-size:12px;
    color:var(--muted); margin-bottom:4px; }
  .slider-row label .val { color:var(--text); font-variant-numeric: tabular-nums; }
  input[type=range] { width:100%; -webkit-appearance:none; background:transparent; height:32px; }
  input[type=range]::-webkit-slider-runnable-track {
    height:6px; background:var(--panel2); border-radius:3px; border:1px solid var(--border);
  }
  input[type=range]::-moz-range-track {
    height:6px; background:var(--panel2); border-radius:3px; border:1px solid var(--border);
  }
  input[type=range]::-webkit-slider-thumb {
    -webkit-appearance:none; width:20px; height:20px; border-radius:50%;
    background:var(--accent); margin-top:-8px; cursor:pointer;
    border:2px solid var(--panel);
  }
  input[type=range]::-moz-range-thumb {
    width:20px; height:20px; border-radius:50%; background:var(--accent);
    border:2px solid var(--panel); cursor:pointer;
  }

  /* Jack layout */
  .jacks { display:flex; flex-direction:column; gap:4px; overflow-x:auto; }
  .jack-row { display:flex; gap:4px; }
  .jack {
    width:26px; height:26px; border-radius:3px; background:#1a1c20;
    border:1px solid var(--border); text-align:center;
    font-size:10px; line-height:24px; color:var(--muted);
    font-variant-numeric: tabular-nums; flex-shrink:0;
  }

  /* Presets */
  .presets-row { display:flex; flex-wrap:wrap; gap:6px; margin-top:6px; }
  .preset {
    background:var(--panel2); color:var(--text); border:1px solid var(--border);
    padding:6px 10px; border-radius:5px; cursor:pointer; font-size:12px;
    display:flex; align-items:center; gap:6px;
  }
  .preset:hover { border-color:var(--accent); }
  .preset .del { color:var(--muted); font-size:14px; cursor:pointer; line-height:1; }
  .preset .del:hover { color:var(--danger); }

  /* Modal */
  .modal-back {
    position:fixed; inset:0; background:rgba(0,0,0,0.7);
    display:flex; align-items:center; justify-content:center; z-index:100;
    padding:16px;
  }
  .modal {
    background:var(--panel); border:1px solid var(--border); border-radius:var(--radius);
    padding:16px; width:100%; max-width:340px;
  }
  .modal h3 { margin:0 0 12px 0; font-size:14px; color:var(--text); text-transform:none; letter-spacing:0; }
  .modal .row { display:flex; gap:8px; align-items:center; margin-bottom:12px; }
  .modal .row label { color:var(--muted); font-size:12px; min-width:70px; }
  .modal .row input[type=text] { flex:1; }
  .modal .row input[type=color] { width:44px; height:36px; border:none; padding:0; border-radius:5px; }
  .modal .actions { display:flex; gap:8px; justify-content:flex-end; }

  .empty { color:var(--muted); text-align:center; padding:40px; }
  .toast {
    position:fixed; bottom:20px; left:50%; transform: translateX(-50%);
    background:var(--panel); border:1px solid var(--border); color:var(--text);
    padding:8px 16px; border-radius:8px; font-size:13px; z-index:200;
    box-shadow: 0 4px 12px rgba(0,0,0,0.4); opacity:0; transition: opacity .2s;
    pointer-events:none;
  }
  .toast.show { opacity:1; }
</style>
</head>
<body>

<header>
  <a class="brand" href="#" aria-label="UIWLED">
    <svg class="logo" viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <defs>
        <linearGradient id="uiwled-ring" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%"   stop-color="#ff3040"/>
          <stop offset="25%"  stop-color="#ffcc00"/>
          <stop offset="50%"  stop-color="#00cc66"/>
          <stop offset="75%"  stop-color="#00b7ff"/>
          <stop offset="100%" stop-color="#7d5cff"/>
        </linearGradient>
      </defs>
      <rect class="ring" x="4" y="8" width="40" height="32" rx="6"
            fill="none" stroke="url(#uiwled-ring)" stroke-width="3"/>
      <rect x="12" y="14" width="24" height="20" rx="2" fill="#181a1c"
            stroke="#3a3f45" stroke-width="1"/>
      <path d="M15 18V30 M18 18V30 M21 18V30 M24 18V30 M27 18V30 M30 18V30 M33 18V30"
            stroke="#8a919c" stroke-width="0.8"/>
      <rect x="20" y="34" width="8" height="4" rx="1" fill="#181a1c" stroke="#3a3f45" stroke-width="0.8"/>
    </svg>
    <span class="wordmark">UI<span class="w">W</span><span class="led">LED</span></span>
  </a>
  <div class="switch-tabs" id="tabs"></div>
</header>

<main>
  <div id="panel-wrap"></div>
</main>

<div id="toast" class="toast"></div>
<div id="modal-slot"></div>

<script>
// ==============================================================
// State
// ==============================================================
const state = {
  switches: [], effects: [], activeName: null, lastRenderJSON: '', effectSearch: '',
  collapsedCats: {}, // category name -> collapsed
};

const BUILT_IN_PALETTES = [
  { name:'Amber',     colors:[[255,160,0],[80,20,0],[0,0,0]] },
  { name:'Ocean',     colors:[[0,16,48],[0,128,255],[255,255,255]] },
  { name:'Forest',    colors:[[0,80,10],[60,180,40],[255,240,80]] },
  { name:'Sunset',    colors:[[255,60,0],[255,180,0],[80,0,80]] },
  { name:'Cyberpunk', colors:[[255,0,180],[0,220,255],[0,0,0]] },
  { name:'Fire',      colors:[[255,0,0],[255,140,0],[255,255,0]] },
  { name:'Ice',       colors:[[100,200,255],[220,240,255],[0,60,120]] },
  { name:'Rainbow',   colors:[[255,0,0],[0,255,0],[0,0,255]] },
];

// Effect categorization by ID (mirrors WLED numbering).
const CATEGORIES = [
  { name: 'Solids',    ids: [0, 1, 2, 23, 6, 41] },                                // Solid, Blink, Breathe, Strobe, Fade, Loading
  { name: 'Chases',    ids: [4, 5, 7, 15, 17, 65, 88, 91, 100] },                  // Scan, Chase, Theater Chase, Running, Two Dots, Meteor, Sinelon, Bouncing Balls, Comet
  { name: 'Rainbows',  ids: [8, 9, 12, 29] },                                       // Colorloop, Rainbow, Rainbow Cycle, Rainbow Runner
  { name: 'Themed',    ids: [26, 27, 46, 79, 92, 101, 110] },                       // Police, Fire Flicker, Christmas, Halloween, Plasma, Pacifica, Sunrise
  { name: 'Sparkle',   ids: [3, 20, 50, 74, 78, 82] },                              // Wipe, Sparkle, Ripple, Twinkle, Fireworks, Rain
];

const LS_PALETTES_KEY = 'uiwled:palettes';
const LS_PRESETS_KEY  = 'uiwled:presets';

// ==============================================================
// Utilities
// ==============================================================
async function fetchJSON(u, opts) {
  const r = await fetch(u, opts||{});
  return r.json();
}
function hex(c) {
  return '#' + [c.R||0, c.G||0, c.B||0].map(x => x.toString(16).padStart(2,'0')).join('');
}
function hexFromArr(a) {
  return '#' + a.map(x => x.toString(16).padStart(2,'0')).join('');
}
function arrFromHex(h) {
  return [parseInt(h.substr(1,2),16), parseInt(h.substr(3,2),16), parseInt(h.substr(5,2),16)];
}
function loadLS(key, fallback) {
  try { return JSON.parse(localStorage.getItem(key)) ?? fallback; }
  catch { return fallback; }
}
function saveLS(key, val) {
  try { localStorage.setItem(key, JSON.stringify(val)); }
  catch (e) { console.warn('localStorage save failed', e); }
}
function customPalettes() { return loadLS(LS_PALETTES_KEY, []); }
function savePalettes(list) { saveLS(LS_PALETTES_KEY, list); }
function presetsFor(swName) {
  const all = loadLS(LS_PRESETS_KEY, {});
  return all[swName] || [];
}
function savePresetsFor(swName, list) {
  const all = loadLS(LS_PRESETS_KEY, {});
  all[swName] = list;
  saveLS(LS_PRESETS_KEY, all);
}
function toast(msg) {
  const t = document.getElementById('toast');
  t.textContent = msg;
  t.classList.add('show');
  clearTimeout(toast._t);
  toast._t = setTimeout(() => t.classList.remove('show'), 1600);
}

// ==============================================================
// Boot + refresh
// ==============================================================
async function loadEffects() {
  state.effects = await fetchJSON('api/effects');
}
async function refresh() {
  try {
    const list = await fetchJSON('api/switches');
    state.switches = list;
    if (!state.activeName && list.length) state.activeName = list[0].name;
    const sig = JSON.stringify({ n: state.activeName, list, q: state.effectSearch, c: state.collapsedCats });
    if (sig === state.lastRenderJSON) return;
    state.lastRenderJSON = sig;
    render();
  } catch (e) { console.error('refresh failed', e); }
}
function invalidate() { state.lastRenderJSON = ''; }
function activeSwitch() { return state.switches.find(s => s.name === state.activeName); }

// ==============================================================
// Render
// ==============================================================
function renderTabs() {
  const tabs = document.getElementById('tabs');
  tabs.innerHTML = state.switches.map(sw => {
    const cls = sw.online ? 'online' : 'connecting';
    const active = sw.name === state.activeName ? ' active' : '';
    return ` + "`" + `<button class="tab${active}" data-name="${sw.name}">
      <span class="status-dot ${cls}"></span>${sw.name}
    </button>` + "`" + `;
  }).join('');
  tabs.querySelectorAll('button').forEach(el => {
    el.onclick = () => {
      if (state.activeName === el.dataset.name) return;
      state.activeName = el.dataset.name;
      invalidate();
      render();
    };
  });
}

function renderPresets(swName) {
  const list = presetsFor(swName);
  const inner = list.map(p =>
    ` + "`" + `<div class="preset" data-preset="${p.name}">${p.name}<span class="del" data-preset-del="${p.name}">×</span></div>` + "`" + `
  ).join('');
  return ` + "`" + `
    <div class="presets-row">
      ${inner || '<span class="meta">No presets saved yet.</span>'}
      <button class="btn sm" id="btn-save-preset">+ Save current</button>
    </div>
  ` + "`" + `;
}

function renderPalettes() {
  const all = BUILT_IN_PALETTES.concat(customPalettes().map(p => ({...p, custom:true})));
  const chips = all.map(p => {
    const grad = ` + "`" + `linear-gradient(90deg, ${p.colors.map(hexFromArr).join(', ')})` + "`" + `;
    const del = p.custom ? ` + "`" + `<span class="del" data-palette-del="${p.name}">×</span>` + "`" + ` : '';
    return ` + "`" + `<div class="palette-chip" style="background:${grad}" data-palette="${p.name}">
      <span class="label">${p.name}</span>${del}
    </div>` + "`" + `;
  }).join('');
  return ` + "`" + `
    <div class="palette-row">
      ${chips}
      <div class="palette-chip add" id="btn-add-palette">+</div>
    </div>
  ` + "`" + `;
}

function renderEffectsPanel(effectID) {
  const q = state.effectSearch.toLowerCase().trim();

  // Build a lookup of all effect ids the server knows about.
  const byId = {};
  for (const e of state.effects) byId[e.id] = e.name;

  // Uncategorized bucket for anything not in CATEGORIES.
  const seen = new Set();
  const cats = CATEGORIES.map(c => {
    const items = c.ids.filter(id => id in byId).map(id => { seen.add(id); return {id, name: byId[id]}; });
    return {name: c.name, items};
  });
  const other = state.effects.filter(e => !seen.has(e.id));
  if (other.length) cats.push({name: 'Other', items: other});

  const catHTML = cats.map(cat => {
    const items = cat.items.filter(e => !q || e.name.toLowerCase().includes(q));
    if (!items.length) return '';
    const collapsed = state.collapsedCats[cat.name] ? ' collapsed' : '';
    const btns = items.map(e =>
      ` + "`" + `<button class="effect ${e.id===effectID?'active':''}" data-effect="${e.id}">${e.name}</button>` + "`" + `
    ).join('');
    return ` + "`" + `
      <div class="effect-cat${collapsed}" data-cat="${cat.name}">
        <div class="effect-cat-title">${cat.name} <span style="color:var(--muted); font-size:10px; margin-left:4px">(${items.length})</span></div>
        <div class="effect-grid">${btns}</div>
      </div>
    ` + "`" + `;
  }).join('');

  return ` + "`" + `
    <div class="effect-header">
      <input type="text" class="effect-search" id="effect-search" placeholder="Search effects…" value="${state.effectSearch}">
    </div>
    ${catHTML || '<div class="empty">No effects match your search.</div>'}
  ` + "`" + `;
}

function renderPanel() {
  const wrap = document.getElementById('panel-wrap');
  const sw = activeSwitch();
  if (!sw) { wrap.innerHTML = '<div class="empty">No switches configured.</div>'; return; }

  const st = sw.state || {};
  const c0 = st.Colors ? st.Colors[0] : {R:255,G:160,B:0};
  const c1 = st.Colors ? st.Colors[1] : {R:0,G:60,B:255};
  const c2 = st.Colors ? st.Colors[2] : {R:0,G:255,B:60};
  const bright = st.Brightness ?? 128;
  const speed  = st.Speed ?? 128;
  const intensity = st.Intensity ?? 128;
  const effectID = st.EffectID ?? 0;
  const on = st.On !== false;

  const metaLine = sw.online
    ? ` + "`" + `${sw.model || 'connected'} @ ${sw.host}${sw.hostname && sw.hostname !== sw.model ? ' — ' + sw.hostname : ''}` + "`" + `
    : ` + "`" + `connecting to ${sw.host}…` + "`" + `;

  wrap.innerHTML = ` + "`" + `
    <div class="panel">
      <div class="toolbar">
        <button class="btn ${on?'on':'off'}" id="btn-power">${on ? 'ON' : 'OFF'}</button>
        <button class="btn" id="btn-identify">Identify</button>
        <button class="btn danger sm" id="btn-clear-ports">Clear overrides</button>
        <div class="meta" style="margin-left:auto; text-align:right;">${metaLine}</div>
      </div>
    </div>

    <div class="panel">
      <h3>Presets</h3>
      ${renderPresets(sw.name)}
    </div>

    <div class="grid cols-2">
      <div class="panel">
        <h3>Colors</h3>
        <div class="color-slots">
          ${[c0,c1,c2].map((c,i) => ` + "`" + `
            <div class="slot">
              <input type="color" value="${hex(c)}" data-slot="${i}" aria-label="Color ${i+1}">
              <div class="slot-label">COLOR ${i+1}</div>
            </div>` + "`" + `).join('')}
        </div>
        <h3 style="margin-top:16px">Palettes</h3>
        ${renderPalettes()}
      </div>

      <div class="panel">
        <h3>Settings</h3>
        <div class="slider-row">
          <label>Brightness <span class="val" id="v-b">${bright}</span></label>
          <input type="range" min="0" max="255" value="${bright}" id="sl-b">
        </div>
        <div class="slider-row">
          <label>Speed <span class="val" id="v-s">${speed}</span></label>
          <input type="range" min="0" max="255" value="${speed}" id="sl-s">
        </div>
        <div class="slider-row">
          <label>Intensity <span class="val" id="v-i">${intensity}</span></label>
          <input type="range" min="0" max="255" value="${intensity}" id="sl-i">
        </div>
      </div>
    </div>

    <div class="panel">
      <h3>Effects</h3>
      ${renderEffectsPanel(effectID)}
    </div>

    <div class="panel">
      <h3>Jack Layout</h3>
      <div class="jacks">${renderLayout(sw.layout)}</div>
    </div>
  ` + "`" + `;

  wireHandlers();
}

function renderLayout(layout) {
  if (!layout || !layout.length) return '<div class="empty">discovering jacks…</div>';
  return layout.map(row =>
    ` + "`" + `<div class="jack-row">${row.map(p => ` + "`" + `<div class="jack">${p}</div>` + "`" + `).join('')}</div>` + "`" + `
  ).join('');
}

// ==============================================================
// Wiring
// ==============================================================
function wireHandlers() {
  document.getElementById('btn-power').onclick = togglePower;
  document.getElementById('btn-identify').onclick = () =>
    fetch('api/identify/'+state.activeName+'?secs=5', {method:'POST'});
  document.getElementById('btn-clear-ports').onclick = clearOverrides;

  document.querySelectorAll('input[type=color][data-slot]').forEach(el => {
    el.oninput = () => pushColor(el.dataset.slot, el.value);
  });
  document.querySelectorAll('.palette-chip[data-palette]').forEach(el => {
    el.onclick = (e) => {
      if (e.target.dataset.paletteDel) return;
      applyPalette(el.dataset.palette);
    };
  });
  document.querySelectorAll('[data-palette-del]').forEach(el => {
    el.onclick = (e) => { e.stopPropagation(); deletePalette(el.dataset.paletteDel); };
  });
  const addBtn = document.getElementById('btn-add-palette');
  if (addBtn) addBtn.onclick = openAddPalette;

  document.querySelectorAll('.effect').forEach(el => {
    el.onclick = () => pushEffect(parseInt(el.dataset.effect,10));
  });
  document.querySelectorAll('.effect-cat-title').forEach(el => {
    el.onclick = () => {
      const cat = el.parentElement.dataset.cat;
      state.collapsedCats[cat] = !state.collapsedCats[cat];
      invalidate(); render();
    };
  });
  const es = document.getElementById('effect-search');
  if (es) {
    es.oninput = () => { state.effectSearch = es.value; invalidate(); render(); es.focus(); };
    // Preserve caret position after re-render
    setTimeout(() => { if (es === document.activeElement) es.selectionStart = es.value.length; }, 0);
  }

  const sB = document.getElementById('sl-b'),
        sS = document.getElementById('sl-s'),
        sI = document.getElementById('sl-i');
  sB.oninput = () => { document.getElementById('v-b').textContent = sB.value; pushBrightness(sB.value); };
  sS.oninput = () => { document.getElementById('v-s').textContent = sS.value; pushEffect(); };
  sI.oninput = () => { document.getElementById('v-i').textContent = sI.value; pushEffect(); };

  // Presets
  document.querySelectorAll('[data-preset]').forEach(el => {
    el.onclick = (e) => {
      if (e.target.dataset.presetDel) return;
      applyPreset(el.dataset.preset);
    };
  });
  document.querySelectorAll('[data-preset-del]').forEach(el => {
    el.onclick = (e) => { e.stopPropagation(); deletePreset(el.dataset.presetDel); };
  });
  const spBtn = document.getElementById('btn-save-preset');
  if (spBtn) spBtn.onclick = openSavePreset;
}

function render() { renderTabs(); renderPanel(); }

// ==============================================================
// API actions
// ==============================================================
async function togglePower() {
  const sw = activeSwitch(); if (!sw) return;
  const cur = sw.state?.On !== false;
  const next = !cur;
  await fetch('api/power/'+sw.name+'?on='+(next?'1':'0'));
  if (sw.state) sw.state.On = next;
  const btn = document.getElementById('btn-power');
  if (btn) { btn.textContent = next ? 'ON' : 'OFF'; btn.className = 'btn ' + (next ? 'on' : 'off'); }
}
async function pushColor(slot, hexStr) {
  const [r,g,b] = arrFromHex(hexStr);
  await fetch('api/color/'+state.activeName+'?slot='+slot+'&r='+r+'&g='+g+'&b='+b);
  const sw = activeSwitch();
  if (sw?.state?.Colors) sw.state.Colors[slot] = {R:r, G:g, B:b};
}
async function applyPalette(name) {
  const all = BUILT_IN_PALETTES.concat(customPalettes());
  const p = all.find(x => x.name === name); if (!p) return;
  const sw = activeSwitch();
  await Promise.all(p.colors.map(([r,g,b], i) =>
    fetch('api/color/'+state.activeName+'?slot='+i+'&r='+r+'&g='+g+'&b='+b)
  ));
  document.querySelectorAll('input[type=color][data-slot]').forEach(el => {
    const i = +el.dataset.slot;
    if (i < 3) el.value = hexFromArr(p.colors[i]);
  });
  if (sw?.state?.Colors) for (let i=0;i<3;i++) {
    const [r,g,b] = p.colors[i];
    sw.state.Colors[i] = {R:r, G:g, B:b};
  }
}
async function pushEffect(id) {
  const q = new URLSearchParams();
  if (id !== undefined) q.set('id', id);
  const sp = document.getElementById('sl-s'); if (sp) q.set('speed', sp.value);
  const it = document.getElementById('sl-i'); if (it) q.set('intensity', it.value);
  await fetch('api/effect/'+state.activeName+'?'+q.toString());
  if (id !== undefined) {
    document.querySelectorAll('.effect').forEach(e => {
      e.classList.toggle('active', parseInt(e.dataset.effect,10) === id);
    });
    const sw = activeSwitch();
    if (sw?.state) sw.state.EffectID = id;
  }
}
async function pushBrightness(v) {
  await fetch('api/brightness/'+state.activeName+'?value='+v);
}
async function clearOverrides() {
  const sw = activeSwitch(); if (!sw) return;
  await fetch('api/ports/'+sw.name, {method:'DELETE'});
  toast('Overrides cleared');
}

// ==============================================================
// Palette editor
// ==============================================================
function openAddPalette() {
  showModal(` + "`" + `
    <h3>New palette</h3>
    <div class="row">
      <label>Name</label>
      <input type="text" id="pal-name" placeholder="My palette">
    </div>
    <div class="row">
      <label>Color 1</label>
      <input type="color" id="pal-c0" value="#ff0000">
      <label>Color 2</label>
      <input type="color" id="pal-c1" value="#00ff00">
      <label>Color 3</label>
      <input type="color" id="pal-c2" value="#0000ff">
    </div>
    <div class="actions">
      <button class="btn" id="pal-cancel">Cancel</button>
      <button class="btn primary" id="pal-save">Save</button>
    </div>
  ` + "`" + `, () => {
    document.getElementById('pal-cancel').onclick = closeModal;
    document.getElementById('pal-save').onclick = () => {
      const name = document.getElementById('pal-name').value.trim();
      if (!name) { toast('Name required'); return; }
      const colors = ['pal-c0','pal-c1','pal-c2'].map(id => arrFromHex(document.getElementById(id).value));
      const list = customPalettes();
      // Overwrite if same name.
      const existing = list.findIndex(p => p.name === name);
      const entry = { name, colors };
      if (existing >= 0) list[existing] = entry; else list.push(entry);
      savePalettes(list);
      closeModal();
      invalidate(); render();
      toast('Palette saved');
    };
    document.getElementById('pal-name').focus();
  });
}
function deletePalette(name) {
  if (!confirm('Delete palette "' + name + '"?')) return;
  savePalettes(customPalettes().filter(p => p.name !== name));
  invalidate(); render();
  toast('Palette deleted');
}

// ==============================================================
// Presets (per switch, stored locally)
// ==============================================================
function openSavePreset() {
  const sw = activeSwitch(); if (!sw) return;
  showModal(` + "`" + `
    <h3>Save preset</h3>
    <div class="row"><label>Name</label>
      <input type="text" id="preset-name" placeholder="e.g. Movie, Alert">
    </div>
    <div class="actions">
      <button class="btn" id="preset-cancel">Cancel</button>
      <button class="btn primary" id="preset-save">Save</button>
    </div>
  ` + "`" + `, () => {
    document.getElementById('preset-cancel').onclick = closeModal;
    document.getElementById('preset-save').onclick = () => {
      const name = document.getElementById('preset-name').value.trim();
      if (!name) { toast('Name required'); return; }
      const list = presetsFor(sw.name);
      // Snapshot the state.
      const snap = JSON.parse(JSON.stringify(sw.state || {}));
      const existing = list.findIndex(p => p.name === name);
      const entry = { name, state: snap };
      if (existing >= 0) list[existing] = entry; else list.push(entry);
      savePresetsFor(sw.name, list);
      closeModal();
      invalidate(); render();
      toast('Preset saved');
    };
    document.getElementById('preset-name').focus();
  });
}
async function applyPreset(name) {
  const sw = activeSwitch(); if (!sw) return;
  const p = presetsFor(sw.name).find(x => x.name === name);
  if (!p || !p.state) return;
  const st = p.state;
  const calls = [];
  if (st.Colors) for (let i = 0; i < 3 && i < st.Colors.length; i++) {
    const c = st.Colors[i] || {R:0,G:0,B:0};
    calls.push(fetch('api/color/'+sw.name+'?slot='+i+'&r='+(c.R||0)+'&g='+(c.G||0)+'&b='+(c.B||0)));
  }
  if (st.Brightness !== undefined) calls.push(fetch('api/brightness/'+sw.name+'?value='+st.Brightness));
  const params = new URLSearchParams();
  if (st.EffectID !== undefined) params.set('id', st.EffectID);
  if (st.Speed !== undefined) params.set('speed', st.Speed);
  if (st.Intensity !== undefined) params.set('intensity', st.Intensity);
  calls.push(fetch('api/effect/'+sw.name+'?'+params.toString()));
  if (st.On !== undefined) calls.push(fetch('api/power/'+sw.name+'?on='+(st.On?'1':'0')));
  await Promise.all(calls);
  toast('Applied ' + name);
  invalidate();
  refresh();
}
function deletePreset(name) {
  const sw = activeSwitch(); if (!sw) return;
  if (!confirm('Delete preset "' + name + '"?')) return;
  savePresetsFor(sw.name, presetsFor(sw.name).filter(p => p.name !== name));
  invalidate(); render();
  toast('Preset deleted');
}

// ==============================================================
// Modal
// ==============================================================
function showModal(html, onReady) {
  const slot = document.getElementById('modal-slot');
  slot.innerHTML = ` + "`" + `<div class="modal-back" id="modal-back"><div class="modal">${html}</div></div>` + "`" + `;
  document.getElementById('modal-back').onclick = (e) => { if (e.target.id === 'modal-back') closeModal(); };
  if (onReady) onReady();
}
function closeModal() {
  document.getElementById('modal-slot').innerHTML = '';
}

// ==============================================================
// Boot
// ==============================================================
(async () => {
  await loadEffects();
  await refresh();
  setInterval(refresh, 5000);
})();
</script>
</body>
</html>`
