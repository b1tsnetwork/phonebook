// Avatar-Editor: Optionen mit Vorschau-Kacheln, Farbfelder, 3D-Vorschau zum Drehen und Pixelmalerei.
import { h, api, toast } from './util.js';
import { openDialog, confirmDialog } from './dialog.js';
import { personPicker } from './picker.js';
import * as A from './avatar.js';

// Zwischenablage für Avatare (bleibt im Browser erhalten, falls möglich)
const CLIP_KEY = 'tb.avatarClip';
let clip = null;
function readClip() {
  if (clip) return clip;
  try { const v = localStorage.getItem(CLIP_KEY); if (v) clip = JSON.parse(v); } catch { /* egal */ }
  return clip;
}
function writeClip(av) {
  clip = structuredClone(av);
  try { localStorage.setItem(CLIP_KEY, JSON.stringify(av)); } catch { /* egal */ }
}
const TRANSPARENT = ''; // Malfarbe „durchsichtig“ (nur Überzug)

const SECTIONS = [
  ['head', '🙂 Kopf'], ['eyes', '👀 Augen'], ['mouth', '👄 Mund & Nase'], ['hair', '💇 Haare'], ['beard', '🧔 Bart'],
  ['glasses', '👓 Brille'], ['hat', '🎩 Kopfbedeckung'], ['extras', '✨ Details'], ['paint', '🖌️ Malen'],
];
// Kacheln je Option: Ansicht (Front-Zoom für Gesichtsdetails)
const FRONT = { yaw: 0, pitch: 0, zoom: 1.7, lift: 0 };
const ISO = { yaw: -32, pitch: 20, zoom: 0.85, lift: 3 };
const R = A.R;

export function openAvatarEditor(opts) {
  const { employee, onSaved, title, admin, initial } = opts;
  // normalize() übernimmt auch alte Avatare (8×8) inkl. gemalter Pixel
  let av = structuredClone(A.normalize(initial || employee.avatar || A.DEFAULT_AVATAR));
  const undo = [];
  const st = { section: 'head', yaw: -32, pitch: 20, spin: false, tool: 'pen', layer: 'base', color: '#e63946', hover: null, painting: false, recent: [] };

  const canvas = h('canvas', { class: 'ave-3d', width: 300, height: 300, title: 'Ziehen zum Drehen' });
  const fields = h('div', { class: 'ave-fields' });
  const nav = h('nav', { class: 'seg secnav' });
  let scene = null;

  const push = () => { undo.push(JSON.stringify(av)); if (undo.length > 60) undo.shift(); };
  const change = (fn, rerender = true) => { push(); fn(); redraw(); if (rerender) renderSection(); };

  // ---------- 3D-Vorschau ----------
  function redraw() {
    scene = A.drawHead(canvas, av, { yaw: st.yaw, pitch: st.pitch, fit: true, highlight: st.section === 'paint' ? st.hover : null });
    if (st.section === 'paint') drawNet();
  }
  let raf = 0;
  const loop = () => { if (!document.body.contains(canvas)) return; if (st.spin) { st.yaw += 0.6; redraw(); } raf = requestAnimationFrame(loop); };

  let drag = null;
  canvas.addEventListener('pointerdown', (e) => {
    canvas.setPointerCapture(e.pointerId);
    const paintMode = st.section === 'paint' && e.button === 0 && !e.shiftKey;
    drag = { x: e.clientX, y: e.clientY, yaw: st.yaw, pitch: st.pitch, paint: paintMode };
    if (paintMode) { push(); st.painting = true; paintAt(e); }
  });
  canvas.addEventListener('pointermove', (e) => {
    if (st.section === 'paint') {
      const q = hitAt(e);
      const key = q ? q.layer + q.face + q.idx : '';
      if (key !== (st.hover ? st.hover.layer + st.hover.face + st.hover.idx : '')) { st.hover = q; if (!drag) redraw(); }
    }
    if (!drag) return;
    if (drag.paint) { paintAt(e); return; }
    st.yaw = drag.yaw + (e.clientX - drag.x) * 0.6;
    st.pitch = Math.max(-80, Math.min(80, drag.pitch + (e.clientY - drag.y) * 0.4));
    redraw();
  });
  const end = () => { drag = null; st.painting = false; };
  canvas.addEventListener('pointerup', end);
  canvas.addEventListener('pointercancel', end);
  canvas.addEventListener('pointerleave', () => { if (st.hover) { st.hover = null; redraw(); } });
  canvas.addEventListener('contextmenu', (e) => e.preventDefault());

  function hitAt(e) {
    const r = canvas.getBoundingClientRect();
    return A.pick(scene, e.clientX - r.left, e.clientY - r.top, st.layer);
  }
  function paintAt(e) {
    const q = hitAt(e);
    if (q) applyTool(q.layer, q.face, q.idx);
  }

  // ---------- Malwerkzeuge ----------
  function texColor(layer, face, idx) { const t = A.buildTexture(av); return (layer === 'over' ? t.over : t.base)[face][idx]; }
  function generated(layer, face, idx) { const t = A.buildTexture({ ...av, paint: undefined }); return (layer === 'over' ? t.over : t.base)[face][idx]; }
  function setPx(layer, face, idx, val) {
    av.paint = av.paint || {};
    av.paint[layer] = av.paint[layer] || {};
    av.paint[layer][face] = av.paint[layer][face] || {};
    if (val === undefined) delete av.paint[layer][face][idx];
    else av.paint[layer][face][idx] = val;
    if (!Object.keys(av.paint[layer][face]).length) delete av.paint[layer][face];
    if (!Object.keys(av.paint[layer]).length) delete av.paint[layer];
    if (!Object.keys(av.paint).length) delete av.paint;
  }
  function applyTool(layer, face, idx) {
    if (st.tool === 'pick') {
      const c = texColor(layer, face, idx);
      if (c) st.color = c;
      else if (layer === 'over') st.color = TRANSPARENT;
      st.tool = 'pen';
      renderSection();
      return;
    }
    // Durchsichtig gibt es nur auf dem Überzug; auf dem Kopf wirkt es wie der Radierer
    const transparent = st.color === TRANSPARENT;
    if (st.tool === 'fill') {
      for (let i = 0; i < R * R; i++) setPx(layer, face, i, transparent && layer !== 'over' ? undefined : st.color);
      if (!transparent) remember(st.color);
    } else if (st.tool === 'erase' || (transparent && layer !== 'over')) {
      setPx(layer, face, idx, undefined); // zurück zum Original (Frisur, Gesicht …)
    } else {
      setPx(layer, face, idx, st.color);
      if (!transparent) remember(st.color);
    }
    redraw();
  }
  function remember(c) {
    if (st.recent[0] === c) return;
    st.recent = [c, ...st.recent.filter((x) => x !== c)].slice(0, 10);
  }

  // Abwicklung (Netz) des Würfels zum präzisen Malen
  const NET = { top: [1, 0], left: [0, 1], front: [1, 1], right: [2, 1], back: [3, 1], bottom: [1, 2] };
  const net = h('canvas', { class: 'ave-net', title: 'Abwicklung: hier pixelgenau malen' });
  let cell = 7;
  function drawNet() {
    const avail = Math.max(256, Math.min(560, (fields.clientWidth || 480) - 4));
    cell = Math.max(4, Math.min(8, Math.floor(avail / (4 * R))));
    const W = 4 * R * cell, H = 3 * R * cell;
    const dpr = window.devicePixelRatio || 1;
    if (net.width !== W * dpr) { net.width = W * dpr; net.height = H * dpr; net.style.width = W + 'px'; net.style.height = H + 'px'; }
    const ctx = net.getContext('2d');
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    const t = A.buildTexture(av);
    for (const [f, [gx, gy]] of Object.entries(NET)) {
      for (let i = 0; i < R * R; i++) {
        const x = (gx * R + (i % R)) * cell, y = (gy * R + ((i / R) | 0)) * cell;
        if (st.layer === 'over') {
          ctx.fillStyle = ((i % R) + ((i / R) | 0)) % 2 ? '#d7d9dd' : '#eceef1';
          ctx.fillRect(x, y, cell, cell);
          ctx.globalAlpha = 0.35; ctx.fillStyle = t.base[f][i]; ctx.fillRect(x, y, cell, cell); ctx.globalAlpha = 1;
          if (t.over[f][i]) { ctx.fillStyle = t.over[f][i]; ctx.fillRect(x, y, cell, cell); }
        } else {
          ctx.fillStyle = t.base[f][i];
          ctx.fillRect(x, y, cell, cell);
        }
      }
      ctx.strokeStyle = 'rgba(0,0,0,.35)'; ctx.lineWidth = 1;
      ctx.strokeRect(gx * R * cell + 0.5, gy * R * cell + 0.5, R * cell - 1, R * cell - 1);
      ctx.fillStyle = 'rgba(0,0,0,.6)'; ctx.font = '10px system-ui, sans-serif';
      ctx.fillText(A.FACE_LABELS[f], gx * R * cell + 3, gy * R * cell + 11);
    }
    if (st.hover && st.hover.layer === st.layer && NET[st.hover.face]) {
      const [gx, gy] = NET[st.hover.face];
      ctx.strokeStyle = '#000'; ctx.lineWidth = 1.5;
      ctx.strokeRect((gx * R + (st.hover.idx % R)) * cell + 0.75, (gy * R + ((st.hover.idx / R) | 0)) * cell + 0.75, cell - 1.5, cell - 1.5);
    }
  }
  const netHit = (e) => {
    const r = net.getBoundingClientRect();
    const k = (4 * R * cell) / r.width; // CSS-Skalierung (z. B. mobil) berücksichtigen
    const cx = Math.floor(((e.clientX - r.left) * k) / cell), cy = Math.floor(((e.clientY - r.top) * k) / cell);
    for (const [f, [gx, gy]] of Object.entries(NET)) {
      const u = cx - gx * R, v = cy - gy * R;
      if (u >= 0 && u < R && v >= 0 && v < R) return { face: f, idx: v * R + u, layer: st.layer };
    }
    return null;
  };
  let netDrag = false;
  net.addEventListener('pointerdown', (e) => { net.setPointerCapture(e.pointerId); const q = netHit(e); if (!q) return; push(); netDrag = true; applyTool(q.layer, q.face, q.idx); });
  net.addEventListener('pointermove', (e) => {
    const q = netHit(e);
    st.hover = q;
    if (netDrag && q) applyTool(q.layer, q.face, q.idx); else redraw();
  });
  net.addEventListener('pointerup', () => (netDrag = false));
  net.addEventListener('pointerleave', () => { netDrag = false; st.hover = null; redraw(); });

  // ---------- Bausteine ----------
  function tiles(key, options, view) {
    const wrap = h('div', { class: 'tiles' });
    for (const [v, label] of options) {
      const sample = { ...av, [key]: v, paint: undefined };
      if (view === FRONT) { sample.glassesStyle = key === 'glassesStyle' ? v : 'none'; if (key !== 'hat') sample.hat = 'none'; }
      const on = (av[key] ?? '') === v;
      const btn = h('button', { type: 'button', class: 'tile' + (on ? ' active' : ''), title: label, 'aria-pressed': on ? 'true' : 'false', onclick: () => {
        // Die anderen Kacheln dieser Gruppe ändern sich dadurch nicht → nur Markierung umsetzen (schnell)
        change(() => { av[key] = v; }, false);
        for (const b of wrap.children) { const act = b === btn; b.classList.toggle('active', act); b.setAttribute('aria-pressed', act ? 'true' : 'false'); }
      } }, h('img', { src: A.thumbURL(sample, 72, view), alt: '', loading: 'lazy' }), h('span', null, label));
      wrap.append(btn);
    }
    return wrap;
  }
  function multiTiles(key, options) {
    const wrap = h('div', { class: 'tiles' });
    for (const [v, label] of options) {
      const sample = { ...av, extras: [v], glassesStyle: 'none', hat: 'none', paint: undefined };
      const on = (av[key] || []).includes(v);
      wrap.append(h('button', { type: 'button', class: 'tile' + (on ? ' active' : ''), title: label, 'aria-pressed': on ? 'true' : 'false', onclick: () => change(() => {
        const s = new Set(av[key] || []);
        if (s.has(v)) s.delete(v); else s.add(v);
        av[key] = [...s];
      }) }, h('img', { src: A.thumbURL(sample, 72, ['studs', 'hoops', 'pencil'].includes(v) ? { yaw: -60, pitch: 8, zoom: 1.2, lift: 0.5 } : FRONT), alt: '' }), h('span', null, label)));
    }
    return wrap;
  }
  function swatches(key, colors, noneLabel) {
    const cur = av[key];
    const wrap = h('div', { class: 'swatches' });
    if (noneLabel) wrap.append(h('button', { type: 'button', class: 'swatch none' + (!cur ? ' active' : ''), title: noneLabel, onclick: () => change(() => { delete av[key]; }) }, '✕'));
    for (const c of colors) {
      const b = h('button', { type: 'button', class: 'swatch' + (c.toLowerCase() === (cur || '').toLowerCase() ? ' active' : ''), title: c, onclick: () => change(() => { av[key] = c; }) });
      b.style.background = c;
      wrap.append(b);
    }
    const inp = h('input', { type: 'color', value: cur || '#888888', title: 'Eigene Farbe' });
    inp.addEventListener('input', () => { av[key] = inp.value; redraw(); });
    inp.addEventListener('change', () => { push(); av[key] = inp.value; renderSection(); });
    wrap.append(inp);
    return wrap;
  }
  const group = (label, ...content) => h('div', { class: 'ave-group' }, h('h4', null, label), ...content);

  function renderSection() {
    for (const b of nav.children) b.classList.toggle('active', b.dataset.sec === st.section);
    canvas.classList.toggle('painting', st.section === 'paint');
    canvas.title = st.section === 'paint' ? 'Klicken/Ziehen zum Malen · Umschalt+Ziehen oder Rechtsklick-Ziehen zum Drehen' : 'Ziehen zum Drehen';
    let body;
    switch (st.section) {
      case 'head': body = [
        group('Hautfarbe', h('p', { class: 'hint' }, 'Grau ist neutral und der Standard. Du kannst jede beliebige Farbe wählen.'), swatches('skin', A.SKIN_COLORS)),
        group('Ohren', tiles('ears', A.OPTIONS.ears, { yaw: 0, pitch: 4, zoom: 1.15, lift: 0.5 })),
        group('Oberfläche', h('div', { class: 'seg' },
          h('button', { type: 'button', class: av.texture !== false ? 'active' : '', onclick: () => change(() => { av.texture = true; }) }, '▦ Pixel-Textur'),
          h('button', { type: 'button', class: av.texture === false ? 'active' : '', onclick: () => change(() => { av.texture = false; }) }, '■ Glatt'))),
      ]; break;
      case 'eyes': body = [group('Augen', tiles('eyeStyle', A.OPTIONS.eyeStyle, FRONT)), group('Augenfarbe', swatches('eyes', A.EYE_COLORS)),
        group('Zweite Augenfarbe (rechtes Auge)', swatches('eyes2', A.EYE_COLORS, 'wie das linke')), group('Augenbrauen', tiles('browStyle', A.OPTIONS.browStyle, FRONT))]; break;
      case 'mouth': body = [group('Mund', tiles('mouthStyle', A.OPTIONS.mouthStyle, FRONT)), group('Lippenfarbe', swatches('lips', A.LIP_COLORS, 'natürlich')), group('Nase', tiles('noseStyle', A.OPTIONS.noseStyle, FRONT))]; break;
      case 'hair': body = [group('Frisur', tiles('hairStyle', A.OPTIONS.hairStyle, ISO)), group(av.hairStyle === 'headscarf' ? 'Farbe des Kopftuchs' : 'Haarfarbe', swatches('hair', A.HAIR_COLORS)),
        group('Strähnen', swatches('highlight', A.HAIR_COLORS, 'keine'))]; break;
      case 'beard': body = [group('Bart', tiles('beardStyle', A.OPTIONS.beardStyle, ISO)), group('Bartfarbe', swatches('beardColor', A.HAIR_COLORS, 'wie die Haare'))]; break;
      case 'glasses': body = [group('Brille', tiles('glassesStyle', A.OPTIONS.glassesStyle, { yaw: -20, pitch: 6, zoom: 1.3, lift: 1 })), group('Gestellfarbe', swatches('glassesColor', A.GLASSES_COLORS))]; break;
      case 'hat': body = [group('Kopfbedeckung & Kopfhörer', tiles('hat', A.OPTIONS.hat, { yaw: -32, pitch: 18, zoom: 0.72, lift: 5 })), group('Farbe (auch für Ohrringe, Zopfgummi)', swatches('accent', A.ACCENT_COLORS))]; break;
      case 'extras': body = [group('Details (mehrfach wählbar)', multiTiles('extras', A.OPTIONS.extras)), group('Akzentfarbe', swatches('accent', A.ACCENT_COLORS))]; break;
      case 'paint': body = paintPanel(); break;
    }
    fields.replaceChildren(...body.filter(Boolean));
    redraw();
  }

  function paintPanel() {
    const tool = (id, label, tip) => h('button', { type: 'button', class: st.tool === id ? 'active' : '', title: tip, onclick: () => { st.tool = id; renderSection(); } }, label);
    const layer = (id, label, tip) => h('button', { type: 'button', class: st.layer === id ? 'active' : '', title: tip, onclick: () => { st.layer = id; renderSection(); } }, label);
    const col = h('input', { type: 'color', value: st.color || '#e63946', title: 'Malfarbe' });
    col.addEventListener('input', () => { st.color = col.value; if (st.tool !== 'pen' && st.tool !== 'fill') st.tool = 'pen'; });
    col.addEventListener('change', () => renderSection());
    const pal = [...new Set([...st.recent, av.skin, av.hair, av.eyes, av.accent, '#ffffff', '#151515', '#e63946', '#ffd166', '#06d6a0', '#3a86ff', '#8338ec', '#f4a6c8'])].slice(0, 18);
    const over = st.layer === 'over';
    const tr = h('button', { type: 'button', class: 'swatch transparent' + (st.color === TRANSPARENT && st.tool !== 'erase' ? ' active' : ''), disabled: over ? null : true,
      title: over ? 'Durchsichtig – entfernt Pixel des Überzugs (auch Haare, Mütze …)' : 'Durchsichtig gibt es nur auf der Ebene „Überzug“', 'aria-label': 'Durchsichtig',
      onclick: () => { st.color = TRANSPARENT; if (st.tool !== 'fill') st.tool = 'pen'; renderSection(); } });
    const sw = h('div', { class: 'swatches' }, col, tr, pal.filter(Boolean).map((c) => {
      const b = h('button', { type: 'button', class: 'swatch' + (c === st.color ? ' active' : ''), title: c, onclick: () => { st.color = c; if (st.tool === 'erase' || st.tool === 'pick') st.tool = 'pen'; renderSection(); } });
      b.style.background = c;
      return b;
    }));
    const painted = av.paint ? Object.values(av.paint).reduce((n, l) => n + Object.values(l).reduce((m, f) => m + Object.keys(f).length, 0), 0) : 0;
    return [
      h('p', { class: 'hint' }, 'Male direkt auf den Kopf links oder pixelgenau in der Abwicklung. Drehen: Umschalt + Ziehen oder die Pfeile unter der Vorschau. Auf dem „Überzug“ macht die Farbe ▨ „durchsichtig“ Pixel unsichtbar.'),
      group('Werkzeug', h('div', { class: 'seg' }, tool('pen', '✏️ Stift', 'Einzelne Pixel malen'), tool('erase', '🧽 Radierer', 'Gemalte Pixel entfernen – zurück zum Original (Frisur, Gesicht …)'), tool('pick', '💧 Pipette', 'Farbe aufnehmen'), tool('fill', '🪣 Seite füllen', 'Eine ganze Seite einfärben'))),
      group('Ebene', h('div', { class: 'seg' }, layer('base', '🙂 Kopf', 'Die Oberfläche des Kopfes'), layer('over', '🎩 Überzug', 'Äußere Hülle – für Haare, Mützen, Bärte mit Tiefe; darf durchsichtig sein'))),
      group('Farbe', sw),
      group('Abwicklung', net),
      painted ? h('button', { type: 'button', class: 'linkish', onclick: () => change(() => { delete av.paint; }) }, `🗑 Alle ${painted} gemalten Pixel entfernen`) : null,
    ];
  }

  for (const [k, l] of SECTIONS) nav.append(h('button', { type: 'button', dataset: { sec: k }, onclick: () => { st.section = k; st.hover = null; renderSection(); } }, l));

  const rotBtn = (label, tip, fn) => h('button', { type: 'button', class: 'icon', title: tip, onclick: () => { fn(); redraw(); } }, label);
  const spinBtn = h('button', { type: 'button', class: 'icon', title: 'Automatisch drehen', onclick: () => { st.spin = !st.spin; spinBtn.classList.toggle('active', st.spin); } }, '⟳');
  const preview = h('div', { class: 'ave-preview' },
    canvas,
    h('div', { class: 'ave-ctrls' },
      rotBtn('◀', 'Nach links drehen', () => (st.yaw -= 45)),
      rotBtn('Vorne', 'Von vorne', () => { st.yaw = 0; st.pitch = 0; }),
      rotBtn('3D', 'Schräg von oben', () => { st.yaw = -32; st.pitch = 20; }),
      rotBtn('Hinten', 'Von hinten', () => { st.yaw = 180; st.pitch = 12; }),
      rotBtn('▶', 'Nach rechts drehen', () => (st.yaw += 45)),
      spinBtn),
    h('div', { class: 'ave-actions' },
      h('button', { type: 'button', title: 'Zufälliges Aussehen', onclick: () => change(() => { av = A.randomAvatar(); }) }, '🎲 Zufall'),
      h('button', { type: 'button', title: 'Zurück zum neutralen grauen Standard', onclick: () => change(() => { av = { ...A.DEFAULT_AVATAR, extras: [] }; }) }, '↺ Standard'),
      h('button', { type: 'button', title: 'Einfarbiger Block ohne Gesicht – z. B. für Einrichtungen. Farbe danach unter „Kopf“ wählen.', onclick: () => change(() => { av = A.blockAvatar(av.hairStyle === 'bare' ? av.skin : '#3b7dd8'); st.section = 'head'; }) }, '■ Block'),
      h('button', { type: 'button', title: 'Rückgängig (Strg+Z)', onclick: () => doUndo() }, '↶ Zurück')),
    clipBar());

  // Kopieren / Einfügen; für Admins zusätzlich übernehmen und übertragen
  function clipBar() {
    const paste = h('button', { type: 'button', title: 'Avatar aus der Zwischenablage einsetzen', disabled: readClip() ? null : true, onclick: () => { const c = readClip(); if (c) change(() => { av = structuredClone(A.normalize(c)); }); } }, '📥 Einfügen');
    const copy = h('button', { type: 'button', title: 'Diesen Avatar in die Zwischenablage kopieren', onclick: () => { writeClip(clean(av)); paste.disabled = false; toast('Avatar kopiert', 'ok'); } }, '📋 Kopieren');
    const items = [h('span', { class: 'lbl' }, 'Avatar kopieren'), copy, paste];
    if (admin) {
      const others = admin.people.filter((p) => p.id !== employee.id);
      const id = 'ave-src-' + Math.random().toString(36).slice(2);
      const label = (p) => `${[p.lastName, p.firstName].filter(Boolean).join(', ')}${p.amt ? ' – ' + p.amt : ''} #${p.id}`;
      const src = h('input', { type: 'text', list: id, placeholder: 'Von Eintrag übernehmen …', autocomplete: 'off', 'aria-label': 'Avatar von Eintrag übernehmen' });
      src.addEventListener('change', () => {
        const m = /#(\d+)$/.exec(src.value);
        const p = m && others.find((x) => x.id === +m[1]);
        if (!p) return;
        change(() => { av = structuredClone(A.normalize(p.avatar || A.DEFAULT_AVATAR)); });
        toast(`Avatar von ${p.firstName || p.lastName} übernommen – noch nicht gespeichert`, 'ok');
        src.value = '';
      });
      items.push(h('div', { class: 'copy-src' }, src, h('datalist', { id }, others.map((p) => h('option', { value: label(p) })))),
        h('button', { type: 'button', title: 'Diesen Avatar auf mehrere Einträge übertragen', onclick: () => transfer() }, '📤 Auf andere übertragen …'));
    }
    return h('div', { class: 'ave-admin' }, items);
  }

  // Admin: aktuellen Avatar auf ausgewählte Einträge schreiben, danach zurück in den Editor
  function transfer() {
    const keep = structuredClone(av);
    const back = () => openAvatarEditor({ ...opts, initial: keep });
    const sel = new Set();
    const count = h('b', null, '0');
    const pp = personPicker({ people: admin.people.filter((p) => p.id !== employee.id), selected: sel, onChange: () => { count.textContent = String(sel.size); }, placeholder: 'Einträge suchen (Name, Amt) …' });
    const prev = h('img', { src: A.thumbURL(keep, 64), alt: '', width: 64, height: 64, class: 'pick-prev' });
    openDialog({
      title: 'Avatar übertragen', wide: true,
      body: h('div', { class: 'admin' }, h('div', { class: 'copy-src' }, prev, h('p', { class: 'hint' }, 'Der Avatar wird bei allen ausgewählten Einträgen gespeichert (deren bisheriger Avatar wird ersetzt). Ausgewählt: ', count)), pp.el),
      buttons: ['spacer', { label: 'Zurück', onClick: () => { back(); return false; } }, { label: '📤 Übertragen', cls: 'primary', onClick: async () => {
        if (!sel.size) { toast('Bitte mindestens einen Eintrag auswählen', 'err'); return false; }
        if (!(await confirmDialog('Avatar übertragen', `Avatar bei ${sel.size} Eintr${sel.size === 1 ? 'ag' : 'ägen'} ersetzen?`, 'Übertragen'))) { back(); return false; }
        const body = { avatar: clean(keep) };
        let ok = 0, err = '';
        for (const id of sel) {
          try { await api('PUT', `/api/employees/${id}/avatar`, body); ok++; } catch (e) { err = e.message; }
        }
        toast(err ? `${ok} übertragen, Fehler: ${err}` : `Avatar auf ${ok} Eintr${ok === 1 ? 'ag' : 'äge'} übertragen`, err ? 'err' : 'ok');
        onSaved && onSaved();
        back();
        return false;
      } }],
    });
    setTimeout(() => pp.focus(), 30);
  }

  function doUndo() { const prev = undo.pop(); if (prev) { av = JSON.parse(prev); renderSection(); } }
  const keyH = (e) => { if (!document.body.contains(canvas)) { document.removeEventListener('keydown', keyH); return; } if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z' && !(e.target instanceof HTMLInputElement && e.target.type === 'text')) { e.preventDefault(); doUndo(); } };
  document.addEventListener('keydown', keyH);

  openDialog({
    title: title || 'Mein Avatar',
    wide: true,
    cls: 'ave-dlg',
    body: h('div', { class: 'ave' }, preview, h('div', { class: 'ave-panel' }, nav, fields)),
    buttons: [
      'spacer',
      { label: 'Abbrechen' },
      { label: 'Speichern', cls: 'primary', onClick: async () => {
        const isDefault = JSON.stringify(clean(av)) === JSON.stringify(clean(A.DEFAULT_AVATAR));
        try {
          await api('PUT', `/api/employees/${employee.id}/avatar`, { avatar: isDefault ? null : clean(av) });
          toast('Avatar gespeichert', 'ok');
          onSaved && onSaved();
        } catch (e) { toast(e.message, 'err'); return false; }
      } },
    ],
    onClose: () => { document.removeEventListener('keydown', keyH); cancelAnimationFrame(raf); },
  });
  renderSection();
  raf = requestAnimationFrame(loop);
}

function clean(av) {
  const out = { v: 2 };
  for (const k of Object.keys(A.DEFAULT_AVATAR)) if (k !== 'v') out[k] = av[k] ?? A.DEFAULT_AVATAR[k];
  out.extras = [...(av.extras || [])];
  for (const k of ['beardColor', 'eyes2', 'lips', 'highlight']) if (av[k]) out[k] = av[k];
  const paint = A.compactPaint(av.paint, { ...av, paint: undefined }); // einheitliche Flächen nur als ein Eintrag
  if (paint) out.paint = paint;
  return out;
}
