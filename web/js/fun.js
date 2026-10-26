// Extras: Voxel-Dorf, „Wer ist das?“-Quiz, Terminal, saisonale Avatare, Abzeichen.
// Alles einzeln abschaltbar (Administration für alle, jede Person für sich – Standard: aus).
import { norm, tokens, collator, telHref, digits } from './util.js';

let C = null; // Kontext aus app.js (Zustand + Hilfsfunktionen)
export function setup(ctx) { C = ctx; }

// ---------- Saisonale Avatare ----------

export function isAprilFools(today) { return (today || '').slice(5) === '04-01'; }

export function seasonalAvatar(av, today) {
  const md = (today || '').slice(5);
  if (!av || (av.eyeStyle === 'none' && av.mouthStyle === 'none')) return av; // einfarbige Blöcke (Einrichtungen) bleiben
  if (md >= '12-01' && md <= '12-26') return { ...av, hat: 'santa' };
  if (md >= '12-31' || md === '01-01') return { ...av, hat: 'party' };
  if (md >= '10-25' && md <= '10-31') return { ...av, hat: 'pumpkin' };
  return av;
}

// ---------- Abzeichen ----------

export function badgesFor(p) {
  const tagNames = (p.tags || []).map((id) => (C.S.tagById.get(id) || {}).name || '').filter(Boolean);
  const has = (re) => tagNames.some((t) => re.test(t));
  const out = [];
  const custom = p.avatar && JSON.stringify(p.avatar) !== JSON.stringify(C.A.DEFAULT_AVATAR);
  if (p.phone && p.room && p.email && p.amt && custom && tagNames.length >= 3) out.push(['🧩', 'Profil vollständig', 'Telefon, Raum, E-Mail, Amt, eigener Avatar und mindestens drei Schlagworte']);
  if (p.avatar && p.avatar.paint) out.push(['🎨', 'Pixelkünstler:in', 'Avatar selbst gemalt']);
  else if (custom) out.push(['🙂', 'Eigener Kopf', 'Avatar gestaltet']);
  if (has(/ersthelfer|erste hilfe|sanität/i)) out.push(['🩹', 'Ersthelfer:in', 'Hilft im Notfall']);
  if (has(/brandschutz/i)) out.push(['🧯', 'Brandschutzhelfer:in', 'Kennt den Feuerlöscher persönlich']);
  if (has(/englisch|französisch|spanisch|italienisch|türkisch|polnisch|russisch|arabisch|ukrainisch|niederländisch|gebärdensprache/i)) out.push(['🗣', 'Mehrsprachig', tagNames.filter((t) => /isch|sprache/i.test(t)).join(', ')]);
  if (has(/ausbild/i)) out.push(['🧑‍🏫', 'Ausbilder:in', 'Bildet aus']);
  if ((p.deputies || []).length) out.push(['🤝', 'Gut vertreten', 'Vertretung ist hinterlegt']);
  if (tagNames.length >= 10) out.push(['🏷', 'Schlagwort-Sammler:in', `${tagNames.length} Schlagworte`]);
  return out;
}

export function badgeRow(p) {
  const list = badgesFor(p);
  if (!list.length) return null;
  return C.h('div', { class: 'badge-row', 'aria-label': 'Abzeichen' }, list.map(([icon, label, title]) => C.h('span', { class: 'achv', title: label + (title ? ' – ' + title : '') }, icon, C.h('small', null, label))));
}

// ---------- Voxel-Dorf ----------

function hashStr(s) { let x = 2166136261; for (const ch of s) { x ^= ch.codePointAt(0); x = Math.imul(x, 16777619); } return x >>> 0; }
const WALLS = ['#f4e3c1', '#e9d3b0', '#dfe7ef', '#f2d6d0', '#d9ead3', '#efe2f3', '#fbe9c6', '#d6e4f0'];
const ROOFS = ['#b5442e', '#8f3b2a', '#3f6aa1', '#4e7d3a', '#7a4d8f', '#9a6a2e', '#2f6f74', '#a33d5c'];
function shade(hex, f) { const n = parseInt(hex.slice(1), 16); const c = (v) => Math.max(0, Math.min(255, Math.round(v * f))); return `rgb(${c(n >> 16)},${c((n >> 8) & 255)},${c(n & 255)})`; }

// kleines isometrisches Haus, pixelig (niedrige Auflösung, per CSS hochskaliert)
function drawHouse(canvas, seed, floors) {
  const W = 64, H = 60;
  canvas.width = W; canvas.height = H;
  const g = canvas.getContext('2d');
  g.imageSmoothingEnabled = false;
  const wall = WALLS[seed % WALLS.length], roof = ROOFS[(seed >>> 4) % ROOFS.length];
  const w = 14 + (seed % 5), d = 12 + ((seed >>> 3) % 4), hgt = 8 + Math.min(4, floors) * 4, rh = 8;
  const P = (x, y, z) => [Math.round(W / 2 + (x - z) * 0.87), Math.round(H - 6 + (x + z) * 0.5 - y - (w + d) * 0.25)];
  const poly = (pts, fill) => { g.beginPath(); pts.forEach(([x, y, z], i) => { const [a, b] = P(x, y, z); if (i) g.lineTo(a, b); else g.moveTo(a, b); }); g.closePath(); g.fillStyle = fill; g.fill(); };
  const x0 = -w / 2, z0 = -d / 2, x1 = w / 2, z1 = d / 2;
  // Schatten
  poly([[x0 - 1, 0, z0 - 1], [x1 + 3, 0, z0 - 1], [x1 + 3, 0, z1 + 3], [x0 - 1, 0, z1 + 3]], 'rgba(0,0,0,.18)');
  // hintere Dachfläche, Wände, Giebel, vordere Dachfläche
  poly([[x0, hgt, z0], [x1, hgt, z0], [x1, hgt + rh, 0], [x0, hgt + rh, 0]], shade(roof, 0.75));
  poly([[x1, 0, z0], [x1, 0, z1], [x1, hgt, z1], [x1, hgt, z0]], shade(wall, 0.82));
  poly([[x1, hgt, z0], [x1, hgt, z1], [x1, hgt + rh, 0]], shade(wall, 0.78));
  poly([[x0, 0, z1], [x1, 0, z1], [x1, hgt, z1], [x0, hgt, z1]], wall);
  // Fenster und Tür (vorne)
  const win = '#5d8fc4';
  for (let f = 0; f < Math.min(3, floors); f++) {
    for (const xx of [x0 + 2.5, x0 + w / 2 + 2]) poly([[xx, 4 + f * 4, z1], [xx + 2.5, 4 + f * 4, z1], [xx + 2.5, 6.5 + f * 4, z1], [xx, 6.5 + f * 4, z1]], win);
  }
  poly([[x1 - 4, 0, z1], [x1 - 1.5, 0, z1], [x1 - 1.5, 4.5, z1], [x1 - 4, 4.5, z1]], '#6b4423');
  for (const zz of [z0 + 2, z0 + d / 2 + 1.5]) poly([[x1, 5, zz], [x1, 5, zz + 2.5], [x1, 7.5, zz + 2.5], [x1, 7.5, zz]], shade(win, 0.85));
  poly([[x0, hgt, z1], [x1, hgt, z1], [x1, hgt + rh, 0], [x0, hgt + rh, 0]], roof);
  // Schornstein
  poly([[x0 + 3, hgt + rh - 2, -1], [x0 + 5, hgt + rh - 2, -1], [x0 + 5, hgt + rh + 3, -1], [x0 + 3, hgt + rh + 3, -1]], '#7a7f86');
}

export function renderVillage() {
  const { S, h, $ } = C;
  const box = $('#village');
  if (S.villCur && !S.unitById.has(S.villCur)) S.villCur = 0;
  const cur = S.villCur ? S.unitById.get(S.villCur) : null;
  const kids = (S.unitKids.get(cur ? cur.id : 0) || []);
  const go = (id) => { S.villCur = id; S.selPeople.clear(); C.update(); box.scrollTop = 0; };
  const parts = [h('nav', { class: 'crumbs' }, h('button', { type: 'button', class: 'crumb', onclick: () => go(0) }, '🏘 Dorfplatz'),
    ...(cur ? C.unitPath(cur) : []).flatMap((u) => [h('span', { class: 'sep' }, '›'), h('button', { type: 'button', class: 'crumb', onclick: () => go(u.id) }, C.unitLabel(u))]))];
  const head = (p, size = 26) => h('button', { type: 'button', class: 'v-head' + (S.selPeople.has(p.key) ? ' sel' : ''), title: (p.firstName ? p.firstName + ' ' : '') + p.lastName + (p.phone ? ' · ' + p.phone : ''), onclick: (e) => { e.stopPropagation(); C.goPerson(p.key); } },
    h('img', { src: C.avatarURL(p, size), alt: '', width: size, height: size }));
  if (kids.length) {
    parts.push(h('div', { class: 'v-grid' }, kids.map((u) => {
      const members = C.unitMembers(u.id, true);
      const { leaders } = C.unitLeads(u);
      const show = [...leaders, ...members.filter((p) => !leaders.includes(p))].slice(0, 7);
      const cv = h('canvas', { class: 'v-house', 'aria-hidden': 'true' });
      drawHouse(cv, hashStr(u.name), Math.max(1, Math.ceil(members.length / 6)));
      return h('div', { class: 'v-lot', role: 'button', tabindex: '0', title: `${C.unitLevel(u).name}: ${C.unitLabel(u)} – ${members.length} Personen`, onclick: () => go(u.id), onkeydown: (e) => { if (e.key === 'Enter') go(u.id); } },
        cv, h('div', { class: 'v-sign' }, h('b', null, u.short || u.name), u.short ? h('small', null, u.name) : null),
        h('div', { class: 'v-heads' }, show.map((p) => head(p)), members.length > show.length ? h('span', { class: 'v-more' }, '+' + (members.length - show.length)) : null));
    })));
  }
  if (cur) {
    const members = C.unitMembers(cur.id, false);
    const { leaders, deputies } = C.unitLeads(cur);
    parts.push(h('div', { class: 'v-square' },
      h('div', { class: 'v-square-title' }, kids.length ? `Direkt in ${C.unitLabel(cur)}` : `Marktplatz ${C.unitLabel(cur)}`),
      members.length ? h('div', { class: 'v-crowd' }, [...leaders.filter((p) => members.includes(p)), ...deputies.filter((p) => members.includes(p) && !leaders.includes(p)), ...members.filter((p) => !leaders.includes(p) && !deputies.includes(p))]
        .map((p) => h('div', { class: 'v-person' }, head(p, 40), h('small', null, (leaders.includes(p) ? '★ ' : deputies.includes(p) ? '☆ ' : '') + (p.firstName || p.lastName))))) : h('p', { class: 'muted' }, 'Hier wohnt (direkt) niemand.')));
  }
  if (!kids.length && !cur) parts.push(h('p', { class: 'muted pad' }, 'Noch keine Organisationseinheiten – das Dorf ist noch leer.'));
  box.replaceChildren(...parts);
}

// ---------- „Wer ist das?“-Quiz ----------

const shuffle = (a) => { for (let i = a.length - 1; i > 0; i--) { const j = Math.floor(Math.random() * (i + 1)); [a[i], a[j]] = [a[j], a[i]]; } return a; };

export function openQuiz() {
  const { S, h, A } = C;
  const pool = S.people.filter((p) => p.kind === 'emp' && p.active && !p.hidden && !p.self && p.firstName && (p.avatar || (p.tags || []).length));
  if (pool.length < 4) { C.toast('Für das Quiz braucht es mindestens vier Personen mit Avatar oder Schlagworten', 'err'); return; }
  const ROUNDS = Math.min(10, pool.length);
  const order = shuffle([...pool]).slice(0, ROUNDS);
  let i = 0, score = 0, raf = 0;
  const body = h('div', { class: 'quiz' });
  const next = () => {
    cancelAnimationFrame(raf);
    if (i >= ROUNDS) {
      const pct = score / ROUNDS;
      body.replaceChildren(h('div', { class: 'quiz-end' }, h('div', { class: 'quiz-big' }, pct === 1 ? '🏆' : pct >= 0.7 ? '🎉' : pct >= 0.4 ? '🙂' : '🤓'),
        h('h3', null, `${score} von ${ROUNDS} richtig`), h('p', { class: 'muted' }, pct === 1 ? 'Du kennst wirklich alle!' : 'Mit jeder Runde lernst du mehr Kolleg:innen kennen.'),
        h('button', { type: 'button', class: 'primary', onclick: () => openQuiz() }, 'Nochmal')));
      return;
    }
    const p = order[i];
    const wrong = shuffle(pool.filter((x) => x !== p)).sort((a, b) => (b.amt === p.amt) - (a.amt === p.amt)).slice(0, 3); // gern aus demselben Amt – schwieriger
    const opts = shuffle([p, ...wrong]);
    const cv = h('canvas', { class: 'quiz-av', width: 160, height: 160 });
    const av = C.viewAvatar(p.avatar || A.DEFAULT_AVATAR);
    const t0 = performance.now();
    const spin = () => { if (!document.body.contains(cv)) return; A.drawHead(cv, av, { yaw: -30 + Math.sin((performance.now() - t0) / 1200) * 40, pitch: 15, fit: true }); raf = requestAnimationFrame(spin); };
    let answered = false, hint = false;
    const amtBox = h('div', { class: 'quiz-hint muted' });
    const tags = (p.tags || []).map((id) => S.tagById.get(id)).filter(Boolean).slice(0, 8);
    const btns = opts.map((o) => h('button', { type: 'button', class: 'quiz-opt', onclick: () => {
      if (answered) return;
      answered = true;
      const ok = o === p;
      if (ok) score += hint ? 0.5 : 1;
      btns.forEach((b, k) => { b.disabled = true; if (opts[k] === p) b.classList.add('right'); else if (opts[k] === o) b.classList.add('wrong'); });
      fb.replaceChildren(h('b', null, ok ? '✔ Richtig!' : `✘ Das ist ${p.firstName} ${p.lastName}.`), ' ',
        h('button', { type: 'button', class: 'linkish', onclick: () => { C.$('#dlg').close(); C.goPerson(p.key); } }, 'Visitenkarte'), ' ',
        h('button', { type: 'button', class: 'primary', onclick: () => { i++; next(); } }, i + 1 < ROUNDS ? 'Weiter →' : 'Ergebnis'));
    } }, o.firstName + ' ' + o.lastName));
    const fb = h('div', { class: 'quiz-fb' });
    body.replaceChildren(
      h('div', { class: 'quiz-top' }, h('span', { class: 'muted' }, `Frage ${i + 1} / ${ROUNDS}`), h('span', { class: 'muted' }, `Punkte: ${score}`)),
      h('div', { class: 'quiz-main' }, cv, h('div', null,
        h('h3', null, 'Wer ist das?'),
        tags.length ? h('div', { class: 'quiz-tags' }, tags.map((t) => h('span', { class: 'chip', style: { borderColor: C.catColor(t.category) } }, t.name))) : h('p', { class: 'muted' }, 'Keine Schlagworte – nur der Kopf hilft.'),
        p.amt ? h('button', { type: 'button', class: 'linkish', onclick: (e) => { hint = true; amtBox.textContent = '🏛 ' + p.amt; e.target.remove(); } }, '💡 Tipp: Amt zeigen (halber Punkt)') : null, amtBox)),
      h('div', { class: 'quiz-opts' }, btns), fb);
    raf = requestAnimationFrame(spin);
  };
  C.openDialog({ title: '🎯 Wer ist das?', wide: true, body, buttons: ['spacer', { label: 'Beenden' }], onClose: () => cancelAnimationFrame(raf) });
  next();
}

// ---------- Terminal ----------

let term = null;

export function openTerminal() {
  if (term) { term.toggle(); return; }
  const { h } = C;
  const out = h('div', { class: 'term-out', role: 'log', 'aria-live': 'polite' });
  const input = h('input', { class: 'term-in', type: 'text', autocomplete: 'off', spellcheck: 'false', 'aria-label': 'Befehl' });
  const promptEl = h('span', { class: 'term-ps' });
  const el = h('div', { class: 'terminal', role: 'dialog', 'aria-label': 'Terminal' },
    h('div', { class: 'term-bar' }, h('span', null, '● ● ●'), h('span', null, 'tb-shell — ^ oder exit zum Schließen'), h('button', { type: 'button', class: 'term-x', 'aria-label': 'Schließen', onclick: () => term.toggle(false) }, '✕')),
    out, h('div', { class: 'term-line' }, promptEl, input));
  document.body.append(el);
  const hist = [];
  let hpos = 0;
  let cwd = 0; // Einheit
  const user = () => (C.S.me.guest ? 'gast' : (C.S.me.username || 'user').toLowerCase());
  const pwd = () => '/' + (cwd ? C.unitPath(C.S.unitById.get(cwd)).map((u) => (u.short || u.name).replace(/\s+/g, '_')).join('/') : '');
  const setPrompt = () => { promptEl.textContent = `${user()}@telefonbuch:${pwd() === '/' ? '~' : pwd()}$ `; };
  const print = (...nodes) => { out.append(h('div', { class: 'term-row' }, ...nodes)); out.scrollTop = out.scrollHeight; };
  const txt = (s, cls) => h('span', { class: cls || null }, s);
  const persons = () => C.S.people.filter((p) => p.kind === 'emp' && p.active && !p.hidden);
  const findPersons = (q) => { const t = tokens(q); return persons().filter((p) => t.every((x) => p.idxName.includes(x) || (digits(x) && p.idxPhone.includes(digits(x))))); };
  const findUnit = (q, base) => {
    const n = norm(q);
    const cand = C.S.org.units.filter((u) => norm(u.name) === n || norm(u.short || '') === n || norm(C.unitLabel(u)) === n);
    if (cand.length) return cand.find((u) => u.parent === base) || cand[0];
    return C.S.org.units.find((u) => norm(C.unitLabel(u)).includes(n)) || null;
  };
  const asciiAvatar = (p) => {
    const t = C.A.buildTexture(C.viewAvatar(p.avatar || C.A.DEFAULT_AVATAR));
    const px = (i) => t.over.front[i] || t.base.front[i] || '#000000';
    const R = 16, lines = [];
    for (let y = 0; y < R; y += 2) {
      const row = h('div', { class: 'term-av' });
      for (let x = 0; x < R; x++) { const s = h('span', null, '▀'); s.style.color = px(y * R + x); s.style.backgroundColor = px((y + 1) * R + x); row.append(s, s.cloneNode(true)); }
      lines.push(row);
    }
    return lines;
  };
  const card = (p) => {
    const via = C.personContact(p);
    const a = C.absNow(p);
    const u = C.S.unitById.get(p.orgUnit);
    const kv = (k, v) => (v ? h('div', null, txt(k.padEnd(11), 'term-k'), txt(v)) : null);
    return h('div', { class: 'term-card' }, h('div', { class: 'term-avs' }, asciiAvatar(p)), h('div', null,
      h('div', null, txt(`${p.lastName}${p.firstName ? ', ' + p.firstName : ''}`, 'term-h')),
      kv('Telefon', p.phone), kv('Mobil', p.mobile), kv('Raum', p.room), kv('Amt', p.amt), kv('E-Mail', p.email),
      kv('Einheit', u ? C.unitPath(u).map((x) => x.short || x.name).join(' › ') : ''),
      kv('Kontaktweg', via ? via.channel.name + ' – ' + C.contactSummary(via.contact) : ''),
      kv('Status', a ? `abwesend bis ${C.fmtDay(a.to)}` : ''),
      kv('Tags', (p.tags || []).map((id) => (C.S.tagById.get(id) || {}).name).filter(Boolean).join(', '))));
  };
  const FORTUNES = ['Es ist immer DNS.', 'Hast du es schon aus- und wieder eingeschaltet?', 'There is no place like 127.0.0.1', 'Das Problem sitzt meist zwischen Stuhl und Tastatur. Meistens.', 'Backups sind etwas für Leute, die Daten verlieren könnten.', 'sudo make me a sandwich', 'Ein Drucker funktioniert genau so lange, bis man ihn braucht.', 'Kaffee: der eigentliche Treiber jeder IT.'];
  const CMDS = {
    help: { d: 'diese Hilfe', f: () => Object.entries(CMDS).forEach(([k, v]) => print(txt(k.padEnd(10), 'term-k'), txt(v.d))) },
    whois: { d: 'whois <name|nummer> – Visitenkarte mit ASCII-Avatar', f: (a) => {
      if (!a) return print(txt('Aufruf: whois müller', 'term-err'));
      const l = findPersons(a);
      if (!l.length) return print(txt(`whois: ${a}: Keine Einträge gefunden`, 'term-err'));
      l.slice(0, 3).forEach((p) => print(card(p)));
      if (l.length > 3) print(txt(`… ${l.length - 3} weitere Treffer – genauer suchen`));
    } },
    finger: { d: 'finger <einheit|amt> – wer gehört dazu', f: (a) => {
      if (!a) return print(txt('Aufruf: finger it-abteilung', 'term-err'));
      const u = findUnit(a, cwd);
      const list = u ? C.unitMembers(u.id, true) : persons().filter((p) => norm(p.amt).includes(norm(a)));
      if (!list.length) return print(txt(`finger: ${a}: niemand zu Hause`, 'term-err'));
      if (u) { const { leaders, deputies } = C.unitLeads(u); print(txt(C.unitLabel(u), 'term-h'), txt(`  (${list.length})`)); if (leaders.length) print(txt('Leitung    ', 'term-k'), txt(leaders.map(C.nameOf).join(', '))); if (deputies.length) print(txt('Stv.       ', 'term-k'), txt(deputies.map(C.nameOf).join(', '))); }
      print(txt('Login'.padEnd(26) + 'Telefon'.padEnd(18) + 'Raum'.padEnd(10) + 'Status', 'term-k'));
      list.sort((x, y) => collator.compare(x.lastName, y.lastName)).slice(0, 60).forEach((p) => print(txt(`${p.lastName}, ${p.firstName}`.slice(0, 25).padEnd(26) + (p.phone || '-').padEnd(18) + (p.room || '-').padEnd(10) + (C.absNow(p) ? 'away' : 'on'))));
    } },
    grep: { d: 'grep <schlagwort> – wer kann was', f: (a) => {
      if (!a) return print(txt('Aufruf: grep ersthelfer', 'term-err'));
      const n = norm(a);
      const tags = C.S.tags.filter((t) => norm(t.name).includes(n));
      if (!tags.length) return print(txt(`grep: kein Schlagwort passt zu „${a}“`, 'term-err'));
      tags.forEach((t) => {
        const l = persons().filter((p) => p.tagSet.has(t.id));
        print(txt(t.name, 'term-h'), txt(` (${l.length})`));
        l.slice(0, 30).forEach((p) => print(txt('  ' + `${p.lastName}, ${p.firstName}`.padEnd(28)), txt(p.phone || '', 'term-k')));
      });
    } },
    ping: { d: 'ping <name|kontaktweg|hotline> – erreichbar?', f: (a) => {
      if (!a) return print(txt('Aufruf: ping hotline', 'term-err'));
      const n = norm(a);
      const ch = C.S.channels.filter((c) => n === 'hotline' ? c.phone : norm(c.name).includes(n));
      const targets = ch.length ? ch.map((c) => ({ name: c.name, tel: c.phone || c.email || c.ticketUrl, away: null })) : findPersons(a).slice(0, 1).map((p) => ({ name: C.nameOf(p), tel: p.phone, away: C.absNow(p), p }));
      if (!targets.length) return print(txt(`ping: ${a}: Name oder Dienst nicht bekannt`, 'term-err'));
      targets.forEach((t) => {
        print(txt(`PING ${t.name} (${t.tel || 'keine Nummer'}): 56 Bytes Kaffee`));
        if (t.away) { for (let k = 0; k < 3; k++) print(txt('Request timeout', 'term-err')); print(txt(`--- abwesend bis ${C.fmtDay(t.away.to)}${(t.p.deputies || []).length ? ', Vertretung: ' + t.p.deputies.map((id) => C.nameOf(C.S.byKey.get('e' + id))).filter(Boolean).join(', ') : ''} ---`)); return; }
        for (let k = 0; k < 3; k++) print(txt(`64 Bytes von ${t.tel || t.name}: seq=${k} ttl=64 Zeit=${(Math.random() * 0.8 + 0.1).toFixed(2)} ms`));
        print(txt('--- 3 gesendet, 3 empfangen, 0 % Verlust ---'));
        if (t.tel && /\d/.test(t.tel)) print(h('a', { href: telHref(t.tel), class: 'term-link' }, `☎ ${t.tel} anrufen`));
      });
    } },
    ls: { d: 'ls – Einheiten hier', f: () => {
      const kids = C.S.unitKids.get(cwd) || [];
      if (!kids.length) return print(txt('(leer)'));
      kids.forEach((u) => print(txt('d ', 'term-k'), txt((u.short || u.name).replace(/\s+/g, '_').padEnd(14), 'term-dir'), txt(`${u.name}  (${C.unitMembers(u.id, true).length})`)));
    } },
    cd: { d: 'cd <einheit|..|/>', f: (a) => {
      if (!a || a === '/' || a === '~') { cwd = 0; return; }
      if (a === '..') { cwd = cwd ? (C.S.unitById.get(cwd).parent || 0) : 0; return; }
      const u = findUnit(a.replace(/_/g, ' '), cwd);
      if (!u) return print(txt(`cd: ${a}: Einheit nicht gefunden`, 'term-err'));
      cwd = u.id;
    } },
    pwd: { d: 'aktueller Ort', f: () => print(txt(pwd())) },
    tree: { d: 'Organigramm als Baum', f: () => {
      const walk = (k, pre) => (C.S.unitKids.get(k) || []).forEach((u, i, arr) => { const last = i === arr.length - 1; print(txt(pre + (last ? '└── ' : '├── '), 'term-k'), txt(C.unitLabel(u))); walk(u.id, pre + (last ? '    ' : '│   ')); });
      print(txt(pwd(), 'term-dir')); walk(cwd, '');
    } },
    open: { d: 'open <name> – Visitenkarte öffnen', f: (a) => { const l = findPersons(a || ''); if (!l.length) return print(txt('open: niemand gefunden', 'term-err')); C.goPerson(l[0].key); print(txt(`→ ${C.nameOf(l[0])}`)); } },
    whoami: { d: 'wer bin ich', f: () => print(txt(C.S.me.guest ? 'gast (nicht angemeldet)' : `${C.S.me.username} (${C.S.me.displayName})${C.S.me.isAdmin ? ' – root-ish' : ''}`)) },
    date: { d: 'Datum', f: () => print(txt(new Date().toLocaleString('de-DE', { dateStyle: 'full', timeStyle: 'medium' }))) },
    uptime: { d: 'wie lange schon', f: () => print(txt(`${new Date().toLocaleTimeString('de-DE')} up ${Math.floor(performance.now() / 60000)} min, ${persons().length} users, Kaffee-Load: 0.42 0.99 1.37`)) },
    fortune: { d: 'Weisheit des Tages', f: () => print(txt(FORTUNES[Math.floor(Math.random() * FORTUNES.length)])) },
    cowsay: { d: 'cowsay <text>', f: (a) => { const s = a || 'Muh.'; print(h('pre', { class: 'term-pre' }, ` ${'_'.repeat(s.length + 2)}\n< ${s} >\n ${'-'.repeat(s.length + 2)}\n        \\   ^__^\n         \\  (oo)\\_______\n            (__)\\       )\\/\\\n                ||----w |\n                ||     ||`)); } },
    sudo: { d: 'sudo …', f: () => print(txt(`${user()} ist nicht in der sudoers-Datei. Dieser Vorfall wird gemeldet. 🙂`, 'term-err')) },
    clear: { d: 'Bildschirm leeren', f: () => out.replaceChildren() },
    exit: { d: 'schließen', f: () => term.toggle(false) },
  };
  CMDS.man = { d: 'man <befehl>', f: (a) => print(txt(CMDS[a] ? `${a} – ${CMDS[a].d}` : `Kein Handbucheintrag für ${a || '?'}`)) };
  CMDS.rm = { d: 'rm …', f: () => print(txt('rm: Das Telefonbuch ist schreibgeschützt. Zum Glück.', 'term-err')) };
  const run = (line) => {
    print(txt(promptEl.textContent, 'term-ps'), txt(line));
    const [cmd, ...rest] = line.trim().split(/\s+/);
    if (!cmd) return;
    const c = CMDS[cmd.toLowerCase()];
    if (!c) { print(txt(`${cmd}: Befehl nicht gefunden – „help“ zeigt alle Befehle`, 'term-err')); return; }
    try { c.f(rest.join(' ')); } catch (e) { print(txt(String(e.message || e), 'term-err')); }
    setPrompt();
  };
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { const v = input.value; input.value = ''; if (v.trim()) { hist.push(v); hpos = hist.length; } run(v); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); if (hpos > 0) input.value = hist[--hpos]; }
    else if (e.key === 'ArrowDown') { e.preventDefault(); hpos = Math.min(hist.length, hpos + 1); input.value = hist[hpos] || ''; }
    else if (e.key === 'Escape' || ((e.code === 'Backquote' || e.key === '^' || e.key === 'Dead') && !input.value)) { e.preventDefault(); term.toggle(false); }
    else if (e.key === 'Tab') {
      e.preventDefault();
      const [c0, ...r] = input.value.split(' ');
      if (!r.length) { const m = Object.keys(CMDS).filter((k) => k.startsWith(c0)); if (m.length === 1) input.value = m[0] + ' '; else if (m.length) print(txt(m.join('  '))); }
    } else if (e.key === 'l' && e.ctrlKey) { e.preventDefault(); out.replaceChildren(); }
  });
  el.addEventListener('click', () => input.focus());
  term = {
    toggle(on) {
      const show = on === undefined ? el.hidden : on;
      el.hidden = !show;
      if (show) { setPrompt(); setTimeout(() => input.focus(), 10); }
    },
  };
  print(txt('Telefonbuch-Shell 1.0 – „help“ für alle Befehle, Tab ergänzt, ↑↓ Verlauf.', 'term-h'));
  print(txt('Versuch mal: whois muster · finger it · grep ersthelfer · ping hotline · tree', 'term-k'));
  term.toggle(true);
}
