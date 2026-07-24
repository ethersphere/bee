// Copyright 2026 The Swarm Authors. All rights reserved.
// pullsim front-end: renders the in-memory pull-sync network and its live
// per-directed-edge message state on a canvas fed by a websocket.
'use strict';

(() => {
  const $ = (id) => document.getElementById(id);
  const canvas = $('canvas');
  const ctx = canvas.getContext('2d');

  const cssVar = (n) => getComputedStyle(document.documentElement).getPropertyValue(n).trim();
  const C = {
    full: cssVar('--accent') || '#46c8b6',
    poonly: cssVar('--amber') || '#e0a84e',
    idle: cssVar('--grey') || '#4b5563',
    traced: cssVar('--traced') || '#57d977',
    text: cssVar('--text') || '#d7dde5',
    muted: cssVar('--muted') || '#8b96a5',
    panel: cssVar('--panel') || '#171b22',
  };
  const modeColor = (m) => m === 'full' ? C.full : m === 'po-only' ? C.poonly : C.idle;

  // ---- state ----
  const state = {
    config: {},
    nodes: [],
    edges: [],
    edgeDir: [],
    stats: {},
    edgeDirMap: new Map(),   // "from-to" -> entry
    edgeMap: new Map(),      // "min-max" -> {po,mode}
    pulses: [],              // {from,to,t0,count}
    tracedSince: new Map(),  // nodeIndex -> ms
    hoverNode: -1,
    hoverEdge: null,         // "from-to"
    selectNode: -1,
    paused: false,
    dpr: 1,
  };

  const now = () => performance.now();

  // ---- websocket ----
  let ws;
  function connect() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(`${proto}://${location.host}/ws`);
    ws.onopen = () => { $('wsstatus').textContent = 'connected'; $('wsdot').classList.add('ok'); };
    ws.onclose = () => {
      $('wsstatus').textContent = 'reconnecting…'; $('wsdot').classList.remove('ok');
      setTimeout(connect, 1000);
    };
    ws.onmessage = (ev) => {
      let m; try { m = JSON.parse(ev.data); } catch { return; }
      dispatch(m);
    };
  }

  function dispatch(m) {
    switch (m.t) {
      case 'hello':
        applyConfig(m.config);
        applySnapshot(m.snap);
        break;
      case 'snap':
        applySnapshot(m);
        break;
      case 'sync':
        if (m.count > 0) state.pulses.push({ from: m.from, to: m.peer, t0: now(), count: m.count });
        break;
      case 'config':
        applyConfig(m.config);
        break;
      case 'inject':
        toast(`injected ${m.count} chunk(s) at node ${m.node}`);
        break;
      case 'radius':
        // reflected in snapshots; no-op
        break;
      default: break;
    }
  }

  function applyConfig(cfg) {
    if (!cfg) return;
    state.config = cfg;
    $('c-nodes').value = cfg.nodes;
    $('c-topology').value = cfg.topology;
    $('c-degree').value = cfg.degree;
    $('c-bins').value = cfg.bins;
    $('c-latency').value = cfg.latencyMs;
    $('c-clusters').value = cfg.clusters;
    $('c-seed').value = cfg.seed;
    const rs = $('radius');
    rs.max = String(Math.max(0, cfg.bins - 1));
    rs.value = String(cfg.radius);
    $('radius-val').textContent = String(cfg.radius);
    $('i-node').max = String(cfg.nodes - 1);
  }

  function applySnapshot(s) {
    if (!s) return;
    state.nodes = s.nodes || [];
    state.edges = s.edges || [];
    state.edgeDir = s.edgeDir || [];
    state.stats = s.stats || {};

    state.edgeDirMap.clear();
    for (const e of state.edgeDir) state.edgeDirMap.set(`${e.from}-${e.to}`, e);
    state.edgeMap.clear();
    for (const e of state.edges) {
      const k = e.a < e.b ? `${e.a}-${e.b}` : `${e.b}-${e.a}`;
      state.edgeMap.set(k, e);
    }

    const t = now();
    for (const n of state.nodes) {
      if (n.hasTraced && !state.tracedSince.has(n.index)) state.tracedSince.set(n.index, t);
      if (!n.hasTraced) state.tracedSince.delete(n.index);
    }
    renderStats();
    if (state.selectNode >= 0) renderDetail();
  }

  function renderStats() {
    const s = state.stats;
    $('stats').innerHTML = `
      <div class="stat"><div class="k">chunks</div><div class="v">${s.chunks ?? 0}</div></div>
      <div class="stat"><div class="k">syncs/s</div><div class="v">${(s.syncsPerSec ?? 0).toFixed(0)}</div></div>
      <div class="stat"><div class="k">dropped</div><div class="v">${s.dropped ?? 0}</div></div>
      <div class="stat"><div class="k">goroutines</div><div class="v">${s.goroutines ?? 0}</div></div>`;
  }

  // ---- geometry ----
  function resize() {
    const stage = $('stage');
    const dpr = window.devicePixelRatio || 1;
    state.dpr = dpr;
    canvas.width = stage.clientWidth * dpr;
    canvas.height = stage.clientHeight * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }
  window.addEventListener('resize', resize);

  function dims() {
    const w = canvas.width / state.dpr, h = canvas.height / state.dpr;
    return { w, h, cx: w / 2, cy: h / 2, R: Math.min(w, h) * 0.40 };
  }

  function nodePos(n) {
    const { cx, cy, R } = dims();
    return { x: cx + R * Math.cos(n.angle - Math.PI / 2), y: cy + R * Math.sin(n.angle - Math.PI / 2) };
  }
  function posOf(i) {
    const n = state.nodes[i];
    return n ? nodePos(n) : null;
  }
  function nodeRadius(n) {
    return 5 + Math.log2(1 + (n.reserveSize || 0)) * 2.2;
  }

  // control point for a directed quad-bezier bowed to one side
  function controlPoint(p0, p1, dir) {
    const mx = (p0.x + p1.x) / 2, my = (p0.y + p1.y) / 2;
    let nx = -(p1.y - p0.y), ny = (p1.x - p0.x);
    const len = Math.hypot(nx, ny) || 1;
    const off = 22 * dir;
    return { x: mx + (nx / len) * off, y: my + (ny / len) * off };
  }
  function quad(p0, cp, p1, t) {
    const u = 1 - t;
    return {
      x: u * u * p0.x + 2 * u * t * cp.x + t * t * p1.x,
      y: u * u * p0.y + 2 * u * t * cp.y + t * t * p1.y,
    };
  }

  // ---- render ----
  function frame() {
    requestAnimationFrame(frame);
    if (state.paused) return;
    const { w, h } = dims();
    ctx.clearRect(0, 0, w, h);
    if (!state.nodes.length) return;

    drawEdges();
    drawPulses();
    drawNodes();
    drawBadges();
  }

  function drawEdges() {
    const t = now();
    for (const e of state.edges) {
      drawDirected(e.a, e.b, e.mode, t, 1);
      drawDirected(e.b, e.a, e.mode, t, -1);
    }
  }

  function drawDirected(from, to, mode, t, dir) {
    const p0 = posOf(from), p1 = posOf(to);
    if (!p0 || !p1) return;
    const cp = controlPoint(p0, p1, dir);
    const ed = state.edgeDirMap.get(`${from}-${to}`);
    const st = ed ? ed.state : 'idle';

    let color = modeColor(mode);
    let width = 1.2;
    let dash = null;
    let dashOffset = 0;
    let glow = 0;

    switch (st) {
      case 'awaiting-offer':
        dash = [6, 6]; dashOffset = -(t / 40) % 12; color = C.full; width = 1.8; break;
      case 'offer-received':
      case 'want-sent':
        width = 2.4; color = lighten(color); break;
      case 'delivering':
        width = 2.8; color = C.traced; glow = 8; break;
      case 'cursors':
        width = 1.0; break;
      default: break;
    }

    ctx.save();
    ctx.beginPath();
    ctx.moveTo(p0.x, p0.y);
    ctx.quadraticCurveTo(cp.x, cp.y, p1.x, p1.y);
    ctx.strokeStyle = color;
    ctx.lineWidth = width;
    if (dash) { ctx.setLineDash(dash); ctx.lineDashOffset = dashOffset; }
    if (glow) { ctx.shadowColor = color; ctx.shadowBlur = glow; }
    ctx.globalAlpha = st === 'idle' ? 0.35 : 0.9;
    ctx.stroke();
    ctx.restore();

    // arrowhead near "to"
    const near = quad(p0, cp, p1, 0.86);
    const back = quad(p0, cp, p1, 0.80);
    drawArrow(back, near, color, st === 'idle' ? 0.35 : 0.9);

    // moving dots while delivering
    if (st === 'delivering') {
      const phase = (t / 700) % 1;
      for (let k = 0; k < 3; k++) {
        const tt = (phase + k / 3) % 1;
        const d = quad(p0, cp, p1, tt);
        ctx.save();
        ctx.beginPath();
        ctx.arc(d.x, d.y, 2.4, 0, Math.PI * 2);
        ctx.fillStyle = C.traced;
        ctx.shadowColor = C.traced; ctx.shadowBlur = 6;
        ctx.fill();
        ctx.restore();
      }
    }
  }

  function drawArrow(from, to, color, alpha) {
    const ang = Math.atan2(to.y - from.y, to.x - from.x);
    const s = 5;
    ctx.save();
    ctx.globalAlpha = alpha;
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.moveTo(to.x, to.y);
    ctx.lineTo(to.x - s * Math.cos(ang - 0.4), to.y - s * Math.sin(ang - 0.4));
    ctx.lineTo(to.x - s * Math.cos(ang + 0.4), to.y - s * Math.sin(ang + 0.4));
    ctx.closePath();
    ctx.fill();
    ctx.restore();
  }

  function drawPulses() {
    const t = now();
    const dur = 600;
    state.pulses = state.pulses.filter((p) => t - p.t0 < dur);
    for (const p of state.pulses) {
      const p0 = posOf(p.to), p1 = posOf(p.from); // chunk flows upstream(to)->downstream(from)
      if (!p0 || !p1) continue;
      const frac = (t - p.t0) / dur;
      const cp = controlPoint(p0, p1, 1);
      const d = quad(p0, cp, p1, frac);
      const r = 2 + Math.log2(1 + p.count) * 1.4;
      ctx.save();
      ctx.beginPath();
      ctx.arc(d.x, d.y, r, 0, Math.PI * 2);
      ctx.fillStyle = C.full;
      ctx.globalAlpha = 1 - frac;
      ctx.shadowColor = C.full; ctx.shadowBlur = 8;
      ctx.fill();
      ctx.restore();
    }
  }

  function drawNodes() {
    const t = now();
    for (const n of state.nodes) {
      const p = nodePos(n);
      const r = nodeRadius(n);
      ctx.save();
      ctx.beginPath();
      ctx.arc(p.x, p.y, r, 0, Math.PI * 2);
      ctx.fillStyle = n.index === state.selectNode ? C.full : '#2b3444';
      ctx.strokeStyle = n.index === state.hoverNode ? C.text : '#40506a';
      ctx.lineWidth = 1.5;
      ctx.fill();
      ctx.stroke();
      ctx.restore();

      if (n.hasTraced) {
        ctx.save();
        ctx.beginPath();
        ctx.arc(p.x, p.y, r + 4, 0, Math.PI * 2);
        ctx.strokeStyle = C.traced;
        ctx.lineWidth = 2;
        ctx.stroke();
        ctx.restore();
        const since = state.tracedSince.get(n.index);
        if (since != null) {
          label(p.x, p.y - r - 8, `${((t - since) / 1000).toFixed(1)}s`, C.traced);
        }
      }

      // index label
      ctx.save();
      ctx.fillStyle = C.muted;
      ctx.font = '9px ui-monospace, monospace';
      ctx.textAlign = 'center';
      ctx.fillText(String(n.index), p.x, p.y + 3);
      ctx.restore();
    }
  }

  function drawBadges() {
    const keys = new Set();
    if (state.hoverEdge) keys.add(state.hoverEdge);
    if (state.selectNode >= 0) {
      for (const ed of state.edgeDir) {
        if (ed.from === state.selectNode || ed.to === state.selectNode) keys.add(`${ed.from}-${ed.to}`);
      }
    }
    let stack = 0;
    for (const key of keys) {
      const ed = state.edgeDirMap.get(key);
      if (!ed) continue;
      const txt = edgeLabel(ed);
      if (!txt) continue;
      const p0 = posOf(ed.from), p1 = posOf(ed.to);
      if (!p0 || !p1) continue;
      const dir = ed.from < ed.to ? 1 : -1;
      const cp = controlPoint(p0, p1, dir);
      const mid = quad(p0, cp, p1, 0.5);
      pill(mid.x, mid.y + (stack % 2 ? 10 : -10), txt);
      stack++;
    }
  }

  function edgeLabel(ed) {
    switch (ed.state) {
      case 'delivering': {
        let d = 0, tot = 0;
        for (const s of ed.streams) { d += s.delivered; tot += s.total; }
        return `DLV ${d}/${tot}`;
      }
      case 'want-sent': return 'WANT';
      case 'offer-received': return 'OFFER';
      case 'awaiting-offer': return `WAIT ${(ed.lastMsgAgeMs / 1000).toFixed(1)}s`;
      case 'cursors': return 'CURS';
      default: return '';
    }
  }

  function label(x, y, text, color) {
    ctx.save();
    ctx.font = '9px ui-monospace, monospace';
    ctx.fillStyle = color;
    ctx.textAlign = 'center';
    ctx.fillText(text, x, y);
    ctx.restore();
  }

  function pill(x, y, text) {
    ctx.save();
    ctx.font = '10px ui-monospace, monospace';
    const wpx = ctx.measureText(text).width + 10;
    ctx.fillStyle = 'rgba(13,16,21,.9)';
    ctx.strokeStyle = '#2a313c';
    roundRect(x - wpx / 2, y - 8, wpx, 16, 4);
    ctx.fill(); ctx.stroke();
    ctx.fillStyle = C.text;
    ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
    ctx.fillText(text, x, y);
    ctx.restore();
  }
  function roundRect(x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
  }
  function lighten(hex) {
    // naive lighten by blending toward white
    const c = hex.replace('#', '');
    const n = parseInt(c.length === 3 ? c.split('').map((x) => x + x).join('') : c, 16);
    const r = Math.min(255, ((n >> 16) & 255) + 50);
    const g = Math.min(255, ((n >> 8) & 255) + 50);
    const b = Math.min(255, (n & 255) + 50);
    return `rgb(${r},${g},${b})`;
  }

  // ---- interaction ----
  canvas.addEventListener('mousemove', (e) => {
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left, my = e.clientY - rect.top;
    state.hoverNode = pickNode(mx, my);
    state.hoverEdge = state.hoverNode >= 0 ? null : pickEdge(mx, my);
  });
  canvas.addEventListener('click', (e) => {
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left, my = e.clientY - rect.top;
    const n = pickNode(mx, my);
    if (n >= 0) {
      state.selectNode = n;
      $('i-node').value = String(n);
      renderDetail();
    } else {
      state.selectNode = -1;
      $('detail').classList.remove('show');
    }
  });

  function pickNode(mx, my) {
    for (const n of state.nodes) {
      const p = nodePos(n);
      if (Math.hypot(p.x - mx, p.y - my) <= nodeRadius(n) + 5) return n.index;
    }
    return -1;
  }
  function pickEdge(mx, my) {
    let best = null, bestD = 10;
    for (const ed of state.edgeDir) {
      const p0 = posOf(ed.from), p1 = posOf(ed.to);
      if (!p0 || !p1) continue;
      const dir = ed.from < ed.to ? 1 : -1;
      const cp = controlPoint(p0, p1, dir);
      for (let t = 0; t <= 1; t += 0.05) {
        const d = quad(p0, cp, p1, t);
        const dist = Math.hypot(d.x - mx, d.y - my);
        if (dist < bestD) { bestD = dist; best = `${ed.from}-${ed.to}`; }
      }
    }
    return best;
  }

  function renderDetail() {
    const idx = state.selectNode;
    const n = state.nodes[idx];
    if (!n) { $('detail').classList.remove('show'); return; }
    const maxBin = Math.max(1, ...(n.binCounts || [1]));
    const bins = (n.binCounts || []).map((c, i) =>
      `<div class="kv"><span>bin ${i}</span><span>${c}</span></div>
       <div class="bar"><span style="width:${(c / maxBin) * 100}%"></span></div>`).join('');

    const streams = [];
    for (const ed of state.edgeDir) {
      if (ed.from !== idx && ed.to !== idx) continue;
      for (const s of ed.streams) {
        streams.push(`<div class="stream-row">${ed.from}→${ed.to} · ${s.stream} · <b>${s.state}</b>
          · bin ${s.bin} · ${s.lastMsg || '-'} · ${(s.ageMs / 1000).toFixed(1)}s
          ${s.state === 'delivering' ? `· ${s.delivered}/${s.total}` : ''}</div>`);
      }
    }

    $('detail').innerHTML = `
      <span class="close" onclick="this.parentNode.classList.remove('show')">✕</span>
      <h3>node ${idx} · <span class="mono">${n.addrPrefix}</span></h3>
      <div class="kv"><span>reserve</span><span>${n.reserveSize}</span></div>
      <div class="kv"><span>radius</span><span>${n.radius}</span></div>
      ${bins}
      <h3 style="margin-top:10px;">live streams (${streams.length})</h3>
      ${streams.join('') || '<div class="sub">none</div>'}`;
    $('detail').classList.add('show');
  }

  function toast(msg) {
    const t = $('toast');
    t.textContent = msg;
    t.style.display = 'block';
    clearTimeout(toast._t);
    toast._t = setTimeout(() => { t.style.display = 'none'; }, 2500);
  }

  // ---- controls ----
  async function post(url, body) {
    const r = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body || {}) });
    if (!r.ok) { const e = await r.json().catch(() => ({})); toast(`error: ${e.error || r.status}`); throw new Error(e.error || r.status); }
    return r.json();
  }

  $('rebuild').onclick = () => {
    post('/api/network', {
      nodes: +$('c-nodes').value,
      topology: $('c-topology').value,
      degree: +$('c-degree').value,
      bins: +$('c-bins').value,
      latencyMs: +$('c-latency').value,
      clusters: +$('c-clusters').value,
      seed: +$('c-seed').value,
      radius: +$('radius').value,
    }).then((r) => { applyConfig(r.config); applySnapshot(r.snapshot); state.selectNode = -1; toast('rebuilt'); });
  };

  $('radius').oninput = () => {
    $('radius-val').textContent = $('radius').value;
    post('/api/radius', { radius: +$('radius').value });
  };

  $('inject').onclick = () => {
    post('/api/inject', {
      node: +$('i-node').value,
      count: +$('i-count').value,
      rate: +$('i-rate').value,
      minPo: +$('i-minpo').value,
    });
  };
  $('inject-stop').onclick = () => post('/api/inject/stop', {});

  $('pause').onclick = () => {
    state.paused = !state.paused;
    $('paused').style.display = state.paused ? 'block' : 'none';
    $('pause').textContent = state.paused ? 'Resume rendering' : 'Pause rendering';
  };

  // ---- boot ----
  resize();
  connect();
  requestAnimationFrame(frame);
})();
