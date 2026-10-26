// Telefonbuch – Hauptansicht: links Personen, rechts Schlagworte.
import { h, $, $$, api, setCSRF, toast, norm, normAlt, digits, tokens, collator, telHref, fmtDate } from './util.js';
import { openDialog, confirmDialog, field } from './dialog.js';
import { openAvatarEditor } from './editor.js';
import * as A from './avatar.js';
import { personPicker, amtPicker } from './picker.js';
import * as F from './fun.js';

// Kategorien kommen vom Server (administrativ pflegbar)
const catName = (k) => S.catByKey.get(k)?.name || k;
const catColor = (k) => S.catByKey.get(k)?.color || '#7c8591';
const dot = (k) => h('span', { class: 'dot', style: { backgroundColor: catColor(k) } });
const FIELD_LABEL = { lastName: 'Nachname', firstName: 'Vorname', phone: 'Telefon', mobile: 'Mobil', room: 'Raum', amt: 'Amt', email: 'E-Mail' };

const S = {
  me: null,
  site: { publicDirectory: false, devAdmin: false, storage: '' },
  people: [],          // vereinheitlicht: {key, kind:'emp'|'priv', id, …}
  byKey: new Map(),
  tags: [],
  tagById: new Map(),
  cats: [],
  catByKey: new Map(),
  org: { levels: [], units: [] },
  unitById: new Map(),
  unitKids: new Map(),
  view: 'list',
  orgCur: 0,
  orgAll: false,
  today: '',
  openReports: 0,
  selPeople: new Set(), // keys
  selTags: new Set(),   // ids
  sort: { key: 'name', dir: 1 },
  catFilter: '',
  showHidden: false,
  rows: new Map(),      // key → DOM-Zeile
  tagRows: new Map(),
};

// ---------- Start / Anmeldung ----------

// ---------- Suche per Adresse (?q=…) und Übergabe an ein schon offenes Fenster ----------
// Der Windows-Client öffnet z. B. /?q=Müller. Ist das Telefonbuch schon offen, übernimmt das
// vorhandene Fenster die Suche und das neue schließt sich wieder.
const channel = (() => { try { return new BroadcastChannel('telefonbuch'); } catch { return null; } })();
if (channel) channel.addEventListener('message', (e) => {
  const m = e.data || {};
  if (m.type === 'search' && S.ready && typeof m.q === 'string') {
    applySearch(m.q);
    channel.postMessage({ type: 'ack', id: m.id });
  }
});

function handOff(q) {
  if (!channel) return Promise.resolve(false);
  const id = Math.random().toString(36).slice(2);
  return new Promise((resolve) => {
    const on = (e) => { if (e.data && e.data.type === 'ack' && e.data.id === id) { clearTimeout(t); channel.removeEventListener('message', on); resolve(true); } };
    const t = setTimeout(() => { channel.removeEventListener('message', on); resolve(false); }, 400);
    channel.addEventListener('message', on);
    channel.postMessage({ type: 'search', q, id });
  });
}

function applySearch(q) {
  if ($('#dlg').open) $('#dlg').close();
  resetAll(true);
  $('#q-main').value = q;
  update();
  if (isMobile()) setTab('people');
  try { window.focus(); } catch { /* egal */ }
}

async function boot() {
  const startQ = (new URLSearchParams(location.search).get('q') || '').trim().slice(0, 100);
  if (startQ) {
    history.replaceState(null, '', location.pathname);
    if (history.length <= 1 && await handOff(startQ)) {
      window.close(); // klappt nur in Fenstern ohne Verlauf (z. B. Edge/Chrome-App-Fenster)
      await new Promise((r) => setTimeout(r, 300));
    }
  }
  window.addEventListener('tb:logout', () => {
    if (S.site.publicDirectory) { toast('Sitzung abgelaufen – du siehst das Telefonbuch jetzt als Gast.', 'err'); setMe(null); showMain(); }
    else showLogin('Sitzung abgelaufen – bitte erneut anmelden.');
  });
  try { S.site = await api('GET', '/api/site'); } catch { /* Standardwerte */ }
  try {
    setMe(await api('GET', '/api/me'));
    await showMain();
  } catch {
    if (S.site.publicDirectory) { setMe(null); await showMain(); }
    else showLogin();
  }
  if (startQ && !$('#main').hidden) applySearch(startQ);
}

// Ohne Anmeldung: Gast – nur lesen
function guestMe() {
  return { guest: true, username: '', displayName: 'Gast', isAdmin: false, employeeId: 0, csrf: '', features: { storage: S.site.storage, devAdmin: S.site.devAdmin, adLocked: [], maxTags: 0 } };
}

function setMe(me) {
  S.me = me || guestMe();
  setCSRF(S.me.csrf);
  S.fav = new Set((S.me.prefs && S.me.prefs.favorites) || []);
}

// Extras: global freigegeben (Administration) UND persönlich eingeschaltet (Standard: aus)
function funOn(k) {
  if (!S.site.fun || S.site.fun[k] === false) return false;
  if (S.me.guest) { try { return JSON.parse(localStorage.getItem('tb.fun') || '{}')[k] === true; } catch { return false; } }
  return !!(S.me.prefs && S.me.prefs.fun && S.me.prefs.fun[k]);
}

function showLogin(msg) {
  $('#main').hidden = true;
  const box = $('#login');
  box.hidden = false;
  const err = $('#login-err');
  err.textContent = msg || '';
  err.hidden = !msg;
  const cv = $('#login-head');
  let yaw = -30, t0 = performance.now();
  const spin = () => { if (box.hidden) return; yaw = -30 + Math.sin((performance.now() - t0) / 1400) * 35; A.drawHead(cv, A.DEFAULT_AVATAR, { yaw, pitch: 18 }); requestAnimationFrame(spin); };
  spin();
  $('#login-dev').hidden = !S.site.devAdmin;
  const form = $('#login-form');
  form.onsubmit = async (e) => {
    e.preventDefault();
    const btn = $('button[type=submit]', form);
    btn.disabled = true;
    err.hidden = true;
    try {
      await api('POST', '/api/login', { username: form.username.value, password: form.password.value });
      form.password.value = '';
      setMe(await api('GET', '/api/me'));
      box.hidden = true;
      await showMain();
    } catch (ex) { err.textContent = ex.message; err.hidden = false; form.password.select(); }
    finally { btn.disabled = false; }
  };
  setTimeout(() => (form.username.value ? form.password : form.username).focus(), 50);
}

async function showMain() {
  $('#login').hidden = true;
  $('#main').hidden = false;
  bindUI();
  await reload();
  S.ready = true;
}

async function reload(keepSel = true) {
  let d;
  try { d = await api('GET', '/api/directory'); } catch (e) { toast(e.message, 'err'); return; }
  if (d.site) S.site = d.site;
  setMe(d.me);
  S.tags = d.tags.sort((a, b) => collator.compare(a.name, b.name));
  S.tagById = new Map(S.tags.map((t) => [t.id, t]));
  const people = [];
  for (const e of d.employees) people.push(mkPerson('emp', e));
  for (const p of d.private) people.push(mkPerson('priv', p));
  for (const p of d.shared || []) people.push(mkPerson('shared', p));
  S.org = d.org || { levels: [], units: [] };
  S.unitById = new Map(S.org.units.map((u) => [u.id, u]));
  S.unitKids = new Map();
  for (const u of S.org.units) {
    const k = S.unitById.has(u.parent) ? u.parent : 0;
    if (!S.unitKids.has(k)) S.unitKids.set(k, []);
    S.unitKids.get(k).push(u);
  }
  S.today = d.today || new Date().toISOString().slice(0, 10);
  S.channels = d.channels || [];
  S.chanById = new Map(S.channels.map((c) => [c.id, c]));
  S.openReports = d.openReports || 0;
  S.cats = d.categories || [];
  S.catByKey = new Map(S.cats.map((c) => [c.key, c]));
  if (S.catFilter && !S.catByKey.has(S.catFilter)) S.catFilter = '';
  renderCatFilter();
  S.people = people;
  S.byKey = new Map(people.map((p) => [p.key, p]));
  if (!keepSel) { S.selPeople.clear(); S.selTags.clear(); }
  for (const k of [...S.selPeople]) if (!S.byKey.has(k)) S.selPeople.delete(k);
  for (const t of [...S.selTags]) if (!S.tagById.has(t)) S.selTags.delete(t);
  buildRows();
  buildTagRows();
  renderHeader();
  computeRoles();
  const hasOrg = S.org.units.length > 0 || S.me.isAdmin;
  $('#vb-org').hidden = !hasOrg;
  $('#vb-village').hidden = !(hasOrg && funOn('village'));
  $('#fav-only').hidden = !!S.me.guest;
  $('#viewbar').hidden = !hasOrg && S.me.guest;
  if ((S.view === 'org' && !hasOrg) || (S.view === 'village' && $('#vb-village').hidden)) setView('list');
  if (S.me.guest) S.favOnly = false;
  $('#fav-only').classList.toggle('active', !!S.favOnly);
  document.body.classList.toggle('upside', funOn('seasonal') && F.isAprilFools(S.today));
  F.setup({ S, A, h, $, api, toast, openDialog, goPerson, orgGo, avatarURL, viewAvatar, unitLabel, unitPath, unitMembers, unitLeads, unitLevel, personContact, contactSummary, nameOf, fmtDay, absNow, setView, update, funOn, catName, catColor });
  update();
}

function mkPerson(kind, x) {
  const p = { ...x, kind, key: ({ emp: 'e', priv: 'p', shared: 's' })[kind] + x.id, tags: kind === 'emp' ? x.tags || [] : [] };
  p.sharedOut = kind === 'priv' && !!x.shares && ((x.shares.employees || []).length + (x.shares.amts || []).length) > 0;
  p.display = [p.lastName, p.firstName].filter(Boolean).join(', ');
  p.idxName = norm(p.lastName + ' ' + p.firstName + ' ' + p.firstName) + ' ' + normAlt(p.lastName + ' ' + p.firstName);
  p.idxPhone = digits(p.phone);
  p.idxMobile = digits(p.mobile);
  p.idxRoom = norm(p.room);
  p.idxAmt = norm(p.amt) + ' ' + normAlt(p.amt);
  p.tagSet = new Set(p.tags);
  p.self = kind === 'emp' && p.id === S.me.employeeId;
  return p;
}

// ---------- Oberfläche verdrahten ----------

let bound = false;
function bindUI() {
  if (bound) return;
  bound = true;
  const deb = (fn, ms = 90) => { let t; return () => { clearTimeout(t); t = setTimeout(fn, ms); }; };
  for (const id of ['q-main', 'q-room', 'q-amt']) $('#' + id).addEventListener('input', deb(update));
  const qt = $('#q-tag');
  // Klick in die Schlagwortsuche setzt links und rechts alles zurück
  qt.addEventListener('focus', () => resetAll(false));
  qt.addEventListener('input', deb(renderTags, 60));
  $$('.col-label[data-sort]').forEach((b) => b.addEventListener('click', () => {
    const k = b.dataset.sort;
    S.sort = { key: k, dir: S.sort.key === k ? -S.sort.dir : 1 };
    sortRows();
    update();
  }));
  $('#btn-reset').addEventListener('click', () => resetAll(true));
  $('#sort-sel').addEventListener('change', () => { S.sort = { key: $('#sort-sel').value, dir: S.sort.dir }; sortRows(); update(); });
  $('#sort-dir').addEventListener('click', () => { S.sort = { key: S.sort.key, dir: -S.sort.dir }; sortRows(); update(); });
  $$('#mtabs button').forEach((b) => b.addEventListener('click', () => setTab(b.dataset.tab)));
  $$('#viewbar button[data-view]').forEach((b) => b.addEventListener('click', () => { setView(b.dataset.view); update(); }));
  $('#fav-only').addEventListener('click', () => { S.favOnly = !S.favOnly; $('#fav-only').classList.toggle('active', S.favOnly); $('#fav-only').setAttribute('aria-pressed', S.favOnly ? 'true' : 'false'); update(); });
  document.addEventListener('keydown', (e) => {
    // ^ (deutsche Tastatur: Tottaste, code „Backquote“) öffnet das Terminal
    if ((e.code === 'Backquote' || e.key === '^') && !e.ctrlKey && !e.altKey && !e.metaKey && funOn('terminal') && !$('#dlg').open && !(e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement)) { e.preventDefault(); F.openTerminal(); }
  });
  try { const v = localStorage.getItem('tb.view'); if (v === 'org' || v === 'village') setView(v); } catch { /* egal */ }
  setTab('people');
  $('#user-btn').addEventListener('click', (e) => { e.stopPropagation(); toggleMenu(); });
  $('#help-btn').addEventListener('click', () => helpDialog());
  document.addEventListener('click', (e) => { if (!$('#menu').hidden && !$('#menu').contains(e.target)) $('#menu').hidden = true; });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !$('#dlg').open) {
      if (!$('#menu').hidden) { $('#menu').hidden = true; return; }
      resetAll(true);
    }
    if (e.key === '/' && document.activeElement === document.body) { e.preventDefault(); $('#q-main').focus(); }
    if ((e.key === '?' || e.key === 'F1') && !$('#dlg').open && !(e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement)) { e.preventDefault(); helpDialog(); }
  });
  // Avatare erst zeichnen, wenn sie sichtbar werden
  S.io = new IntersectionObserver((ents) => {
    for (const en of ents) if (en.isIntersecting) { const img = en.target; const p = S.byKey.get(img.dataset.key); if (p) img.src = avatarURL(p, 36); S.io.unobserve(img); }
  }, { root: $('#people'), rootMargin: '300px' });
}

function resetAll(clearTagSearch) {
  S.selPeople.clear();
  S.selTags.clear();
  for (const id of ['q-main', 'q-room', 'q-amt']) $('#' + id).value = '';
  if (clearTagSearch) $('#q-tag').value = '';
  update();
}

function avatarURL(p, size) {
  if (p.kind !== 'emp') return A.thumbURL({ ...A.DEFAULT_AVATAR, skin: '#c8cbd0', hair: '#9aa0a6', mouthStyle: 'neutral' }, size);
  return A.thumbURL(viewAvatar(p.avatar || A.DEFAULT_AVATAR), size);
}
// Avatar für die Anzeige (ggf. saisonal verkleidet)
const viewAvatar = (av) => (funOn('seasonal') ? F.seasonalAvatar(av, S.today) : av);

// ---------- Linke Liste ----------

function buildRows() {
  const list = $('#people');
  S.rows.clear();
  if (S.io) S.io.disconnect();
  const frag = document.createDocumentFragment();
  for (const p of S.people) {
    const img = h('img', { class: 'av', alt: '', width: 36, height: 36, dataset: { key: p.key } });
    const tel = (n) => (n ? h('a', { href: telHref(n), title: 'Anrufen', onclick: (e) => e.stopPropagation() }, n) : h('span', { class: 'muted' }, '–'));
    // Symbol-Beschriftung nur in der schmalen Kartenansicht sichtbar; leere Felder werden dort ausgeblendet
    const cell = (cls, lbl, title, val, content) => h('div', { class: cls + (val ? '' : ' novalue'), title: title || null }, h('span', { class: 'lbl', 'aria-hidden': 'true' }, lbl), content);
    const badges = [];
    badges.push(...privBadges(p, true));
    if (S.fav && S.fav.has(p.key)) badges.unshift(h('span', { class: 'fav-star', title: 'Favorit' }, '★'));
    if (p.self) badges.push(h('span', { class: 'badge self' }, 'ich'));
    const pref = p.kind === 'emp' ? personContact(p) : null;
    if (pref) badges.push(h('span', { class: 'badge via', title: `Bitte bevorzugt: ${contactSummary(pref.contact)} (${pref.channel.name})` }, pref.contact.ticketUrl ? '🎫' : pref.contact.email ? '✉' : '☎'));
    const away = absNow(p);
    if (away) badges.push(h('span', { class: 'badge away', title: absenceText(p, away) }, '🏖 bis ' + fmtDay(away.to)));
    if (p.kind === 'emp' && (!p.active || p.hidden)) badges.push(h('span', { class: 'badge off' }, !p.active ? 'inaktiv' : 'ausgeblendet'));
    const row = h('div', { class: 'row', role: 'option', tabindex: '-1', dataset: { key: p.key } },
      h('div', { class: 'c-name' }, img, h('span', { class: 'nm' }, h('b', null, p.lastName), p.firstName ? ', ' + p.firstName : ''), ...badges),
      ...(pref && pref.contact.hideDirect
        ? [cell('c-phone', '☎', `Bitte bevorzugt: ${contactSummary(pref.contact)} (${pref.channel.name})`, true, pref.contact.phone ? h('span', { class: 'via' }, tel(pref.contact.phone), h('small', null, ' Hotline')) : h('span', { class: 'via' }, pref.contact.ticketUrl ? '🎫 Ticket' : '✉ Mail')),
          cell('c-mobile', '📱', null, false, h('span', { class: 'muted' }, '–'))]
        : [cell('c-phone', '☎', null, p.phone, tel(p.phone)), cell('c-mobile', '📱', null, p.mobile, tel(p.mobile))]),
      cell('c-room', '🚪', p.room ? 'Raum ' + p.room : null, p.room, p.room || h('span', { class: 'muted' }, '–')),
      cell('c-amt', '🏛', p.amt, p.amt, p.amt || h('span', { class: 'muted' }, '–')));
    row.addEventListener('click', (e) => clickPerson(p.key, e));
    row.addEventListener('dblclick', () => { if (canEditPerson(p)) editPerson(p); });
    row.addEventListener('keydown', (e) => {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        const vis = $$('#people .row:not([hidden])');
        const i = vis.indexOf(row) + (e.key === 'ArrowDown' ? 1 : -1);
        if (vis[i]) { vis[i].focus(); clickPerson(vis[i].dataset.key, {}); vis[i].scrollIntoView({ block: 'nearest' }); }
      }
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); clickPerson(p.key, e); }
    });
    S.rows.set(p.key, row);
    S.io && S.io.observe(img);
    frag.append(row);
  }
  list.replaceChildren(frag);
  sortRows();
}

function sortVal(p, k) {
  switch (k) {
    case 'phone': return p.idxPhone || '￿';
    case 'mobile': return p.idxMobile || '￿';
    case 'room': return p.room || '￿';
    case 'amt': return p.amt || '￿';
    default: return p.lastName + '\u0001' + p.firstName;
  }
}
function sortRows() {
  const { key, dir } = S.sort;
  const fav = (p) => (S.fav && S.fav.has(p.key) ? 0 : 1); // Favoriten immer oben
  const sorted = [...S.people].sort((a, b) => fav(a) - fav(b) || dir * collator.compare(sortVal(a, key), sortVal(b, key)) || collator.compare(a.display, b.display));
  const list = $('#people');
  for (const p of sorted) list.append(S.rows.get(p.key));
  $('#sort-sel').value = key;
  $('#sort-dir').textContent = dir > 0 ? '▲' : '▼';
  $$('.col-label[data-sort]').forEach((b) => {
    b.classList.toggle('sorted', b.dataset.sort === key);
    b.dataset.dir = b.dataset.sort === key ? (dir > 0 ? '▲' : '▼') : '';
  });
}

function matchPerson(p, qMain, qRoom, qAmt) {
  if (S.favOnly && !S.fav.has(p.key)) return false;
  if (p.kind === 'emp' && (!p.active || p.hidden) && !S.showHidden) return false;
  for (const t of qMain) {
    const d = digits(t);
    if (p.idxName.includes(t)) continue;
    if (d && /^[\d+\-/() .]+$/.test(t) && (p.idxPhone.includes(d) || p.idxMobile.includes(d))) continue;
    return false;
  }
  for (const t of qRoom) if (!p.idxRoom.includes(t)) return false;
  for (const t of qAmt) if (!p.idxAmt.includes(t)) return false;
  for (const tid of S.selTags) if (!p.tagSet.has(tid)) return false;
  return true;
}

function update() {
  const qMain = tokens($('#q-main').value), qRoom = tokens($('#q-room').value), qAmt = tokens($('#q-amt').value);
  let n = 0;
  const visible = new Set();
  for (const p of S.people) {
    const ok = matchPerson(p, qMain, qRoom, qAmt);
    const row = S.rows.get(p.key);
    if (row.hidden === ok) row.hidden = !ok;
    if (ok) { n++; visible.add(p.key); }
  }
  // Auswahl auf sichtbare Personen beschränken
  for (const k of [...S.selPeople]) if (!visible.has(k)) S.selPeople.delete(k);
  for (const [k, row] of S.rows) {
    const on = S.selPeople.has(k);
    row.classList.toggle('sel', on);
    row.setAttribute('aria-selected', on ? 'true' : 'false');
  }
  const total = S.people.filter((p) => p.kind !== 'emp' || S.showHidden || (p.active && !p.hidden)).length;
  $('#people-foot').textContent = n === total ? `${total} Einträge` : `${n} von ${total} Einträgen`;
  $('#mt-people').textContent = n === total ? `(${total})` : `(${n}/${total})`;
  $('#people-empty').hidden = n > 0;
  $('#people').hidden = n === 0;
  renderChips();
  renderCard();
  renderTags();
  if (S.view === 'org') renderOrg();
  if (S.view === 'village') F.renderVillage();
}

// Mobil: Personen und Schlagworte als Reiter
const isMobile = () => window.matchMedia('(max-width: 760px)').matches;
function setTab(t) {
  S.tab = t;
  const app = $('#main');
  app.classList.toggle('tab-people', t === 'people');
  app.classList.toggle('tab-tags', t === 'tags');
  $$('#mtabs button').forEach((b) => b.classList.toggle('active', b.dataset.tab === t));
}

function clickPerson(key, e) {
  const multi = e.ctrlKey || e.metaKey;
  if (multi) {
    if (S.selPeople.has(key)) S.selPeople.delete(key); else S.selPeople.add(key);
  } else if (S.selPeople.size === 1 && S.selPeople.has(key)) {
    S.selPeople.clear();
  } else {
    S.selPeople.clear();
    S.selPeople.add(key);
    if (isMobile() && e.type === 'click') { update(); setTab('tags'); $('#main .pane.right').scrollTop = 0; return; }
  }
  update();
}

function renderChips() {
  const box = $('#chips');
  const chips = [];
  for (const tid of S.selTags) {
    const t = S.tagById.get(tid);
    if (!t) continue;
    chips.push(h('button', { type: 'button', class: 'chip', style: { borderColor: catColor(t.category) }, title: 'Filter entfernen', onclick: () => { S.selTags.delete(tid); update(); } }, '🏷 ', t.name, h('span', { class: 'x' }, '✕')));
  }
  if (S.selPeople.size > 1) chips.push(h('button', { type: 'button', class: 'chip', onclick: () => { S.selPeople.clear(); update(); } }, `👥 ${S.selPeople.size} Personen ausgewählt`, h('span', { class: 'x' }, '✕')));
  const any = chips.length || $('#q-main').value || $('#q-room').value || $('#q-amt').value || S.selPeople.size;
  box.replaceChildren(...chips);
  $('#btn-reset').hidden = !any;
  $('#filterbar').classList.toggle('active', !!any);
}

// ---------- Rechte Seite: Personenkarte + Schlagworte ----------

function canEditPerson(p) { return p.kind === 'priv' || (p.kind === 'emp' && S.me.isAdmin); }

// Kennzeichnung privater Kontakte: eigene (ggf. geteilt) und für mich freigegebene
function privBadges(p, short) {
  if (p.kind === 'priv') {
    if (p.sharedOut) return [h('span', { class: 'badge priv shared', title: 'Privater Kontakt – von dir freigegeben für: ' + shareSummary(p.shares) }, short ? '🔗 privat, geteilt' : '🔗 Privat, geteilt')];
    return [h('span', { class: 'badge priv', title: 'Privater Kontakt – nur für dich sichtbar' }, short ? '🔒 privat' : '🔒 privater Kontakt')];
  }
  if (p.kind === 'shared') return [h('span', { class: 'badge priv shared', title: `Privater Kontakt von ${p.ownerName}, für dich freigegeben (nur lesen)` }, short ? '🔗 von ' + p.ownerName : '🔗 Privat, geteilt von ' + p.ownerName)];
  return [];
}
function shareSummary(sh) {
  if (!sh) return '';
  const names = (sh.employees || []).map((id) => S.byKey.get('e' + id)).filter(Boolean).map((x) => [x.firstName, x.lastName].filter(Boolean).join(' '));
  return [...names, ...(sh.amts || []).map((a) => 'Amt ' + a)].join(', ');
}
function canEditTags(p) { return p.kind === 'emp' && (S.me.isAdmin || p.self); }

let cardAnim = 0;
function renderCard() {
  const card = $('#card');
  cancelAnimationFrame(cardAnim);
  if (S.selPeople.size !== 1) {
    if (S.selPeople.size > 1) {
      card.replaceChildren(h('div', { class: 'card multi' }, h('div', { class: 'stack' }, [...S.selPeople].slice(0, 6).map((k) => h('img', { src: avatarURL(S.byKey.get(k), 40), alt: '' }))),
        h('div', null, h('b', null, `${S.selPeople.size} Personen ausgewählt`), h('div', { class: 'muted' }, 'Rechts stehen alle Schlagworte dieser Personen.'))));
    } else {
      card.replaceChildren();
    }
    return;
  }
  const p = S.byKey.get([...S.selPeople][0]);
  const cv = h('canvas', { class: 'card-av', width: 96, height: 96, title: p.kind === 'emp' && (p.self || S.me.isAdmin) ? 'Avatar bearbeiten' : '' });
  const av = p.kind !== 'emp' ? null : viewAvatar(p.avatar || A.DEFAULT_AVATAR);
  if (av) {
    const t0 = performance.now();
    const anim = () => { if (!document.body.contains(cv)) return; A.drawHead(cv, av, { yaw: -28 + Math.sin((performance.now() - t0) / 1500) * 22, pitch: 16, fit: true }); cardAnim = requestAnimationFrame(anim); };
    requestAnimationFrame(anim);
    if (p.self || S.me.isAdmin) cv.addEventListener('click', () => editAvatar(p));
  }
  const line = (icon, label, val, href) => val ? h('div', { class: 'kv' }, h('span', { class: 'k' }, icon + ' ' + label), href ? h('a', { href }, val) : h('span', null, val)) : null;
  const btns = [];
  const isEmp = p.kind === 'emp';
  const own = isEmp && (p.self || S.me.isAdmin);
  if (!S.me.guest && p.kind !== 'shared') {
    const on = S.fav.has(p.key);
    btns.push(h('button', { type: 'button', class: 'fav-btn' + (on ? ' on' : ''), title: on ? 'Aus den Favoriten entfernen' : 'Zu den Favoriten (Kurzwahl, max. 10)', onclick: () => toggleFav(p) }, on ? '★ Favorit' : '☆ Favorit'));
  }
  if (canEditTags(p)) btns.push(h('button', { type: 'button', class: 'primary', onclick: () => editTags(p) }, p.self ? '🏷 Meine Schlagworte' : '🏷 Schlagworte'));
  if (p.kind === 'emp' && (p.self || S.me.isAdmin)) btns.push(h('button', { type: 'button', onclick: () => editAvatar(p) }, '🙂 Avatar'));
  if (canEditPerson(p)) btns.push(h('button', { type: 'button', onclick: () => editPerson(p) }, p.kind === 'priv' ? '✏️ Bearbeiten' : '✏️ Stammdaten'));
  if (own) btns.push(h('button', { type: 'button', onclick: () => editDeputies(p) }, '👥 Vertretung'));
  if (own && S.site.absences !== 'off') btns.push(h('button', { type: 'button', onclick: () => editAbsences(p) }, '🏖 Abwesenheit'));
  if (isEmp && !S.me.guest && S.site.reports !== false) btns.push(h('button', { type: 'button', class: 'subtle', title: 'Falsche oder veraltete Angaben melden', onclick: () => reportDialog(p) }, '🚩 Fehler melden'));
  const head = h('div', { class: 'card-head' }, av ? cv : h('div', { class: 'card-av priv' }, p.kind === 'shared' || p.sharedOut ? '🔗' : '🔒'),
    h('div', null,
      h('h3', null, p.lastName, p.firstName ? ', ' + p.firstName : ''),
      p.amt ? h('div', { class: 'muted' }, p.amt) : null,
      h('div', { class: 'badges' },
        ...privBadges(p, false),
        p.source === 'ad' ? h('span', { class: 'badge ad', title: 'Stammdaten kommen aus dem Active Directory' }, 'AD') : null,
        p.self ? h('span', { class: 'badge self' }, 'das bist du') : null,
        ...(isEmp ? personRoles(p).map((r) => h('span', { class: 'badge role', title: r.title }, r.label)) : [])),
      isEmp && p.badges && S.site.fun && S.site.fun.badges !== false ? F.badgeRow(p) : null));
  const pref = isEmp ? personContact(p) : null;
  const direct = [line('☎', 'Telefon', p.phone, telHref(p.phone)), line('📱', 'Mobil', p.mobile, telHref(p.mobile))].filter(Boolean);
  card.replaceChildren(h('div', { class: 'card' }, head,
    pref ? contactBox(pref.channel) : null,
    h('div', { class: 'kvs' },
      ...(pref && pref.contact.hideDirect && direct.length ? [h('details', { class: 'direct' }, h('summary', null, 'Direkte Durchwahl (nur wenn nötig)'), ...direct)] : direct),
      line('🚪', 'Raum', p.room),
      line('✉', 'E-Mail', p.email, p.email ? 'mailto:' + p.email : null),
      p.note ? h('div', { class: 'kv note' }, h('span', { class: 'k' }, '📝 Notiz'), h('span', null, p.note)) : null,
      p.sharedOut ? h('div', { class: 'kv note' }, h('span', { class: 'k' }, '🔗 Geteilt mit'), h('span', null, shareSummary(p.shares))) : null,
      ...(isEmp ? cardOrgLines(p) : [])),
    btns.length ? h('div', { class: 'card-btns' }, btns) : null));
}

function buildTagRows() {
  S.tagRows.clear();
  const frag = document.createDocumentFragment();
  for (const t of S.tags) {
    const row = h('div', { class: 'tag-row', role: 'option', tabindex: '-1', title: catName(t.category) },
      dot(t.category), h('span', { class: 'tn' }, t.name), h('span', { class: 'cnt' }, String(t.count)));
    row.addEventListener('click', (e) => clickTag(t.id, e));
    row.addEventListener('keydown', (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); clickTag(t.id, e); } });
    S.tagRows.set(t.id, row);
    frag.append(row);
  }
  $('#tags').replaceChildren(frag);
}

function renderTags() {
  const q = tokens($('#q-tag').value);
  let allowed = null;
  if (S.selPeople.size) {
    allowed = new Set();
    for (const k of S.selPeople) for (const t of S.byKey.get(k)?.tags || []) allowed.add(t);
  }
  let n = 0;
  for (const t of S.tags) {
    const row = S.tagRows.get(t.id);
    const ok = (!allowed || allowed.has(t.id)) && (!S.catFilter || t.category === S.catFilter) && q.every((x) => norm(t.name).includes(x) || normAlt(t.name).includes(x)) && (t.count > 0 || S.me.isAdmin);
    row.hidden = !ok;
    row.classList.toggle('sel', S.selTags.has(t.id));
    row.setAttribute('aria-selected', S.selTags.has(t.id) ? 'true' : 'false');
    if (ok) n++;
  }
  $('#mt-tags').textContent = `(${n})`;
  const foot = $('#tags-foot');
  const empty = $('#tags-empty');
  empty.hidden = n > 0;
  $('#tags').hidden = n === 0;
  if (S.selPeople.size === 1) {
    const p = S.byKey.get([...S.selPeople][0]);
    empty.textContent = p.kind !== 'emp' ? 'Private Kontakte haben keine Schlagworte.' : 'Noch keine Schlagworte hinterlegt.';
    foot.textContent = `${n} Schlagwort${n === 1 ? '' : 'e'} von ${p.firstName || p.lastName}`;
  } else if (S.selPeople.size > 1) {
    empty.textContent = 'Keine Schlagworte.';
    foot.textContent = `${n} Schlagworte der ausgewählten Personen`;
  } else {
    empty.textContent = 'Kein Schlagwort gefunden.';
    foot.textContent = `${n} Schlagworte`;
  }
}

function clickTag(id, e) {
  const multi = e.ctrlKey || e.metaKey;
  if (multi) {
    if (S.selTags.has(id)) S.selTags.delete(id); else S.selTags.add(id);
  } else if (S.selTags.size === 1 && S.selTags.has(id)) {
    S.selTags.clear();
  } else {
    S.selTags.clear();
    S.selTags.add(id);
    if (isMobile()) { update(); setTab('people'); return; }
  }
  update();
}

// ---------- Kopfzeile & Menü ----------

function myEmployee() { return S.people.find((p) => p.self); }

function renderHeader() {
  const me = myEmployee();
  $('#user-av').hidden = !!S.me.guest;
  $('#user-av').src = me ? avatarURL(me, 32) : A.thumbURL(A.DEFAULT_AVATAR, 32);
  $('#user-name').textContent = S.me.guest ? '🔑 Anmelden' : (S.me.displayName || S.me.username);
  $('#user-btn').classList.toggle('guest', !!S.me.guest);
  $('#user-caret').hidden = !!S.me.guest;
  $('#help-btn').hidden = !S.me.guest; // angemeldet: Hilfe im Benutzermenü
  $('#dev-flag').hidden = !S.site.devAdmin;
  $('#admin-flag').hidden = !S.me.isAdmin;
  // Nur im Rückfall-Betrieb (MySQL nicht erreichbar → JSON-Kopie, nur lesen), nicht im normalen JSON-Modus
  const off = $('#storage-flag');
  off.hidden = !S.site.offline;
  off.title = 'Die Datenbank ist nicht erreichbar. Angezeigt wird die JSON-Kopie' + (S.site.offlineSince ? ' vom ' + fmtDate(S.site.offlineSince) : '') + ' – Änderungen sind derzeit nicht möglich.';
}

function toggleMenu() {
  const m = $('#menu');
  if (S.me.guest) { m.hidden = true; loginDialog(); return; }
  if (!m.hidden) { m.hidden = true; return; }
  const me = myEmployee();
  const item = (label, fn, opts = {}) => h('button', { type: 'button', role: 'menuitem', disabled: opts.disabled || null, title: opts.title || null, onclick: () => { m.hidden = true; fn(); } }, label);
  const items = [
    h('div', { class: 'menu-head' }, h('b', null, S.me.displayName || S.me.username), h('small', null, S.me.username + (S.me.isAdmin ? ' · Administrator' : ''))),
    item('🙂 Mein Avatar', () => editAvatar(me), { disabled: !me, title: me ? null : 'Für dein Konto gibt es keinen Telefonbucheintrag' }),
    item('🏷 Meine Schlagworte', () => editTags(me), { disabled: !me, title: me ? null : 'Für dein Konto gibt es keinen Telefonbucheintrag' }),
    item('➕ Privaten Kontakt hinzufügen', () => editPerson(null, 'priv')),
    item('👥 Meine Vertretung', () => editDeputies(me), { disabled: !me, title: me ? null : 'Für dein Konto gibt es keinen Telefonbucheintrag' }),
  ];
  if (S.site.absences !== 'off') items.push(item('🏖 Meine Abwesenheit', () => editAbsences(me), { disabled: !me }));
  if (S.site.reports !== false) items.push(item('🚩 Meine Meldungen', myReportsDialog));
  items.push(item('★ Favoriten im Windows-Symbol …', feedDialog));
  if (funOn('quiz')) items.push(item('🎯 Wer ist das? (Quiz)', () => F.openQuiz()));
  if (funOn('terminal')) items.push(item('⌨ Terminal  ( ^ )', () => F.openTerminal()));
  items.push(item('🎮 Extras …', extrasDialog));
  if (S.me.isAdmin) {
    items.push(h('div', { class: 'menu-sep' }, 'Verwaltung'),
      item('👤 Neuen Eintrag anlegen', () => editPerson(null, 'emp')),
      item('🏷 Schlagworte verwalten', tagManager),
      item('🗂 Kategorien verwalten', categoryManager),
      item('🏢 Organisation verwalten', orgManager),
      item('💬 Kontaktwege verwalten', channelManager),
      item('📊 Datenqualität', qualityDialog),
      ...(S.site.reports !== false ? [item(`🚩 Meldungen${S.openReports ? ' (' + S.openReports + ' offen)' : ''}`, reportsDialog)] : []),
      item('🔄 AD-Abgleich', syncDialog),
      item('💾 Datenhaltung', storageDialog),
      item('📜 Protokoll', auditDialog),
      item((S.showHidden ? '☑' : '☐') + ' Ausgeblendete & inaktive zeigen', () => { S.showHidden = !S.showHidden; update(); }));
  }
  items.push(h('div', { class: 'menu-sep' }), item('💻 Windows-Symbol (Infobereich)', clientDialog), item('❔ Hilfe', helpDialog), item('🚪 Abmelden', logout));
  m.replaceChildren(...items);
  m.hidden = false;
}

async function logout() {
  try { await api('POST', '/api/logout'); } catch { /* egal */ }
  S.selPeople.clear(); S.selTags.clear();
  if (S.site.publicDirectory) { setMe(null); await reload(false); toast('Abgemeldet', 'ok'); }
  else showLogin('Du wurdest abgemeldet.');
}

// ---------- Hilfe ----------

function helpDialog() {
  const sec = (title, ...items) => h('section', { class: 'help-sec' }, h('h3', null, title), h('ul', null, items.map((x) => h('li', null, x))));
  const kbd = (k) => h('kbd', null, k);
  const body = h('div', { class: 'help' },
    sec('🔍 Suchen',
      ['Die große Suchleiste durchsucht ', h('b', null, 'Name, Telefon und Mobil'), ' gleichzeitig. Mehrere Wörter müssen alle passen, z. B. „anna 456“.'],
      'Umlaute sind egal: „mueller“ findet „Müller“. Bei Nummern zählen nur die Ziffern: „456 12“ findet „0123 456-12“.',
      'Raum und Amt haben eigene Suchfelder.',
      'Ein Klick auf eine Spaltenüberschrift sortiert, ein zweiter Klick dreht die Reihenfolge um.'),
    sec('👥 Personen & 🏷 Schlagworte',
      ['Klick auf eine ', h('b', null, 'Person'), ' → rechts stehen nur noch ihre Schlagworte, dazu die Visitenkarte. ', kbd('Strg'), '+Klick wählt mehrere Personen.'],
      ['Klick auf ein ', h('b', null, 'Schlagwort'), ' → links stehen nur noch Personen mit diesem Schlagwort. ', kbd('Strg'), '+Klick kombiniert mehrere – dann müssen alle passen.'],
      ['Klick in die ', h('b', null, 'Schlagwortsuche'), ' oder ', kbd('Esc'), ' → alles zurücksetzen.'],
      ['Die Farbe zeigt die Kategorie (', ...S.cats.flatMap((c, i) => [i ? ', ' : '', dot(c.key), ' ', c.name]), '). Die Zahl zeigt, wie viele Personen das Schlagwort haben.'],
      'Telefonnummern sind anklickbar und wählen direkt (Softphone/Handy).'),
    sec('✏️ Selbst pflegen (nach der Anmeldung)',
      ['Menü → ', h('b', null, 'Meine Schlagworte'), ': eintippen, ', kbd('Enter'), ' – vorhandene Schlagworte werden vorgeschlagen, neue lassen sich als Tätigkeit oder Besonderheit anlegen.'],
      ['Menü → ', h('b', null, 'Mein Avatar'), ': Pixel-Kopf gestalten, drehen (ziehen), im Bereich „Malen“ pixelgenau bemalen. ', kbd('Strg'), '+', kbd('Z'), ' macht rückgängig.'],
      ['Menü → ', h('b', null, 'Privaten Kontakt hinzufügen'), ': erscheint mit 🔒 in deiner Liste und ist nur für dich sichtbar.'],
      ['Private Kontakte lassen sich unter „Freigeben“ für einzelne Personen oder ein ganzes Amt teilen (🔗 Privat, geteilt). Bearbeiten kann nur, wer den Kontakt angelegt hat.']),
    sec('🏢 Organisation, Vertretung, Abwesenheit',
      ['Oben links zwischen ', h('b', null, '☰ Liste'), ' und ', h('b', null, '🏢 Organisation'), ' wechseln: Im Organigramm klickst du dich von Ebene zu Ebene, oben stehen jeweils Leitung und Stellvertretung.'],
      'Auf der Visitenkarte steht „Vertretung: …“ – ein Klick springt direkt zu dieser Person. 🏖 zeigt eine Abwesenheit samt Zeitraum und Vertretung.',
      ['Eigene Vertretung und Abwesenheit trägst du im Menü ein. Stimmt etwas nicht, hilft ', h('b', null, '🚩 Fehler melden'), ' auf der Visitenkarte.']),
    sec('💻 Windows-Symbol',
      ['Das Telefonbuch als Symbol im Infobereich (unten rechts): Klick öffnet es, Rechtsklick → „Suchen …“. ', h('a', { href: '/client/telefonbuch-windows.zip', download: '' }, 'Herunterladen'), ', entpacken, ', h('b', null, 'install.cmd'), ' doppelklicken – ohne Administratorrechte.']),
    sec('⌨️ Tastatur',
      [kbd('/'), ' Suche fokussieren · ', kbd('↑'), kbd('↓'), ' in der Liste wechseln · ', kbd('Esc'), ' zurücksetzen · ', kbd('?'), ' diese Hilfe']),
    S.me.isAdmin ? sec('🛠 Verwaltung',
      'Doppelklick auf einen Eintrag öffnet die Stammdaten.',
      'Mit 🔒 markierte Felder kommen aus dem Active Directory und werden beim Abgleich überschrieben.',
      'Schlagworte verwalten: umbenennen, Kategorie ändern, zusammenführen (Dubletten!), unbenutzte löschen.',
      'Kategorien verwalten: anlegen, umbenennen, Farbe und Reihenfolge ändern, löschen (die Schlagworte wandern in eine Folge-Kategorie).',
      'Avatar-Editor: Avatare kopieren/einfügen, von anderen Einträgen übernehmen, auf mehrere Einträge übertragen; „■ Block“ für einfarbige Einrichtungs-Einträge.',
      'Datenhaltung: MySQL oder JSON-Datei, Übernahme aus MySQL, JSON-Kopie.') : null);
  openDialog({ title: 'Hilfe', body, buttons: [{ label: '🎮 Extras …', onClick: () => { extrasDialog(); return false; } }, 'spacer', { label: 'Schließen', cls: 'primary' }] });
}

function clientDialog() {
  openDialog({
    title: 'Telefonbuch im Infobereich (Windows)',
    body: h('div', { class: 'help' },
      h('p', null, 'Ein kleines Symbol unten rechts in der Taskleiste: ', h('b', null, 'Klick'), ' öffnet das Telefonbuch in einem eigenen Fenster (oder holt es nach vorne), ', h('b', null, 'Rechtsklick → Suchen …'), ' sucht direkt nach Name oder Nummer.'),
      h('ol', null,
        h('li', null, h('a', { href: '/client/telefonbuch-windows.zip', download: '' }, '⬇ Telefonbuch-Windows.zip'), ' herunterladen und entpacken.'),
        h('li', null, h('b', null, 'install.cmd'), ' doppelklicken. Administratorrechte sind nicht nötig, die Adresse dieses Telefonbuchs ist schon eingetragen.'),
        h('li', null, 'Das Symbol erscheint im Infobereich (unter Windows 11 ggf. unter „^“ – von dort in die Taskleiste ziehen). Es startet künftig automatisch mit Windows.')),
      h('p', { class: 'hint' }, 'Es handelt sich um reine PowerShell-Skripte, kein Programm. Entfernen mit uninstall.cmd.')),
    buttons: ['spacer', { label: 'Schließen', cls: 'primary' }],
  });
}

// Anmeldung als Dialog (wenn das Telefonbuch öffentlich lesbar ist)
function loginDialog() {
  const user = h('input', { name: 'username', autocomplete: 'username', maxlength: 128, autocapitalize: 'none', spellcheck: 'false', required: true });
  const pw = h('input', { name: 'password', type: 'password', autocomplete: 'current-password', maxlength: 512, required: true });
  const err = h('p', { class: 'err', role: 'alert', hidden: true });
  const submit = async () => {
    err.hidden = true;
    try {
      await api('POST', '/api/login', { username: user.value, password: pw.value });
      setMe(await api('GET', '/api/me'));
      await reload();
      toast(`Angemeldet als ${S.me.displayName || S.me.username}`, 'ok');
    } catch (e) { err.textContent = e.message; err.hidden = false; pw.select(); return false; }
  };
  const form = h('form', { class: 'login-form', onsubmit: (e) => { e.preventDefault(); dlgRef.foot.querySelector('button.primary').click(); } },
    h('p', { class: 'hint' }, 'Zum Bearbeiten deiner Schlagworte, deines Avatars und privater Kontakte mit dem Windows-Konto anmelden.'),
    S.site.devAdmin ? h('p', { class: 'dev-hint' }, '⚠ DEV-Modus: lokales Admin-Konto aktiv (Benutzer und Passwort stehen im Server-Log).') : null,
    field('Benutzername', user), field('Passwort', pw), err, h('button', { type: 'submit', hidden: true }));
  const dlgRef = openDialog({ title: 'Anmelden', body: form, buttons: ['spacer', { label: 'Abbrechen' }, { label: 'Anmelden', cls: 'primary', onClick: submit }] });
  setTimeout(() => user.focus(), 30);
}

// ---------- Bearbeiten ----------

function editAvatar(p) {
  if (!p) return;
  openAvatarEditor({
    employee: p, title: p.self ? 'Mein Avatar' : `Avatar von ${[p.firstName, p.lastName].filter(Boolean).join(' ')}`, onSaved: () => reload(),
    admin: S.me.isAdmin ? { people: S.people.filter((x) => x.kind === 'emp').map(pickPerson), reopen: () => editAvatar(S.byKey.get(p.key) || p) } : null,
  });
}

function editTags(p) {
  if (!p) return;
  const cur = (p.tags || []).map((id) => S.tagById.get(id)).filter(Boolean).map((t) => ({ name: t.name, category: t.category }));
  const chipsBox = h('div', { class: 'chips edit' });
  const input = h('input', { type: 'text', placeholder: 'Schlagwort eingeben und Enter drücken …', maxlength: 60, autocomplete: 'off', 'aria-label': 'Schlagwort' });
  const sugg = h('div', { class: 'sugg', role: 'listbox' });
  const catSel = h('div', { class: 'seg small' });
  let newCat = S.catByKey.has('taetigkeit') ? 'taetigkeit' : (S.cats[0]?.key || '');
  const renderCat = () => catSel.replaceChildren(...S.cats.map((c) => h('button', { type: 'button', class: c.key === newCat ? 'active' : '', onclick: () => { newCat = c.key; renderCat(); renderSugg(); } }, dot(c.key), ' ', c.name)));
  renderCat();
  // Leitungs-/Vertretungs-Schlagworte des Organigramms: nur Admins vergeben sie
  const prot = new Set(S.org.levels.flatMap((l) => [l.leaderTag, l.deputyTag]).filter(Boolean).map((id) => norm((S.tagById.get(id) || {}).name || '')));
  const locked = (name) => !S.me.isAdmin && prot.has(norm(name));
  const has = (name) => cur.some((t) => norm(t.name) === norm(name));
  const known = (name) => S.tags.find((t) => norm(t.name) === norm(name));
  const add = (name, cat) => {
    name = name.trim().replace(/\s+/g, ' ');
    if (!name || has(name)) return;
    if (cur.length >= S.me.features.maxTags) { toast(`Höchstens ${S.me.features.maxTags} Schlagworte`, 'err'); return; }
    if (locked(name)) { toast('Dieses Schlagwort (Leitung/Stellvertretung) vergeben nur Administratoren', 'err'); return; }
    const k = known(name);
    if (!k && !S.me.features.userCreatesTags && !S.me.isAdmin) { toast('Neue Schlagworte können nur Administratoren anlegen', 'err'); return; }
    cur.push(k ? { name: k.name, category: k.category } : { name, category: cat || newCat });
    input.value = '';
    render();
  };
  const render = () => {
    chipsBox.replaceChildren(...(cur.length ? cur.map((t, i) => h('span', { class: 'chip', style: { borderColor: catColor(t.category) }, title: locked(t.name) ? 'Vergibt die Administration (Organigramm)' : null }, t.name, locked(t.name) ? ' 🔒' : h('button', { type: 'button', class: 'x', title: 'Entfernen', 'aria-label': 'Entfernen', onclick: () => { cur.splice(i, 1); render(); } }, '✕'))) : [h('span', { class: 'muted' }, 'Noch keine Schlagworte.')]));
    renderSugg();
  };
  const renderSugg = () => {
    const q = norm(input.value.trim());
    const list = S.tags.filter((t) => !has(t.name) && !locked(t.name) && (!q || norm(t.name).includes(q) || normAlt(t.name).includes(q))).sort((a, b) => b.count - a.count).slice(0, q ? 12 : 18);
    const items = list.map((t) => h('button', { type: 'button', class: 'sugg-item', onclick: () => add(t.name) }, dot(t.category), t.name, h('small', null, String(t.count))));
    const exact = q && S.tags.some((t) => norm(t.name) === q);
    if (q && !exact && !has(input.value) && (S.me.features.userCreatesTags || S.me.isAdmin)) items.unshift(h('button', { type: 'button', class: 'sugg-item new', onclick: () => add(input.value) }, '＋ Neu: „', input.value.trim(), '“ als ', catName(newCat)));
    sugg.replaceChildren(...items);
    sugg.hidden = !items.length;
  };
  input.addEventListener('input', renderSugg);
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ',' || e.key === ';') { e.preventDefault(); if (input.value.trim()) add(input.value); }
    if (e.key === 'Backspace' && !input.value && cur.length && !locked(cur[cur.length - 1].name)) { cur.pop(); render(); }
  });
  render();
  openDialog({
    title: p.self ? 'Meine Schlagworte' : `Schlagworte: ${p.firstName} ${p.lastName}`,
    body: h('div', { class: 'tag-editor' },
      h('p', { class: 'hint' }, 'Was machst du, wofür bist du Ansprechpartner, was zeichnet dich aus? Z. B. „SAP“, „Ersthelfer“, „Englisch“.'),
      chipsBox, h('div', { class: 'tag-input' }, input), h('div', { class: 'newcat' }, h('span', { class: 'muted' }, 'Neue Schlagworte sind: '), catSel), sugg),
    buttons: ['spacer', { label: 'Abbrechen' }, { label: 'Speichern', cls: 'primary', onClick: async () => {
      if (input.value.trim()) add(input.value);
      try { await api('PUT', `/api/employees/${p.id}/tags`, { tags: cur }); toast('Schlagworte gespeichert', 'ok'); await reload(); }
      catch (e) { toast(e.message, 'err'); return false; }
    } }],
  });
  setTimeout(() => input.focus(), 30);
}

function editPerson(p, kindNew) {
  const kind = p ? p.kind : kindNew;
  const isNew = !p;
  const locked = new Set(kind === 'emp' && p && p.source === 'ad' ? S.me.features.adLocked : []);
  const inputs = {};
  const mk = (k, type = 'text', max = 100) => {
    const el = h('input', { type, value: p ? p[k] || '' : '', maxlength: max, disabled: locked.has(k) || null, autocomplete: 'off' });
    inputs[k] = el;
    return field(FIELD_LABEL[k] + (locked.has(k) ? ' 🔒' : ''), el, locked.has(k) ? 'kommt aus dem AD' : null);
  };
  const body = h('div', { class: 'form-grid' }, mk('lastName'), mk('firstName'), mk('phone', 'tel', 64), mk('mobile', 'tel', 64), mk('room', 'text', 64), mk('amt', 'text', 150), h('div', { class: 'full' }, mk('email', 'email', 254)));
  // Vorschläge für Amt und Raum aus vorhandenen Einträgen
  const dl = h('datalist', { id: 'amt-list' }, [...new Set(S.people.map((x) => x.amt).filter(Boolean))].sort(collator.compare).map((a) => h('option', { value: a })));
  body.append(dl);
  inputs.amt.setAttribute('list', 'amt-list');
  let note, user, hidden, orgSel, chanSel;
  let shareEmp, shareAmts;
  if (kind === 'priv') {
    note = h('textarea', { maxlength: 500, rows: 3 }, p ? p.note || '' : '');
    body.append(h('div', { class: 'full' }, field('Notiz', note)));
    shareEmp = new Set(p && p.shares ? p.shares.employees || [] : []);
    shareAmts = [...(p && p.shares ? p.shares.amts || [] : [])];
    const hint = h('p', { class: 'hint full' });
    const sumEl = h('span', { class: 'muted' });
    const updHint = () => {
      const n = shareEmp.size, m = shareAmts.length;
      hint.textContent = n || m ? '🔗 Privat, geteilt – nur du kannst den Kontakt bearbeiten.' : '🔒 Dieser Kontakt ist nur für dich sichtbar.';
      sumEl.textContent = n || m ? ` – ${n ? n + ' Person' + (n === 1 ? '' : 'en') : ''}${n && m ? ', ' : ''}${m ? m + ' Amt' + (m === 1 ? '' : '/Ämter') : ''}` : ' – nicht geteilt';
    };
    const others = S.people.filter((x) => x.kind === 'emp' && x.active && !x.hidden && !x.self);
    const amtCounts = new Map();
    for (const x of others) if (x.amt) amtCounts.set(x.amt.toLowerCase(), (amtCounts.get(x.amt.toLowerCase()) || 0) + 1);
    const pp = personPicker({ people: others.map(pickPerson), selected: shareEmp, onChange: updHint, placeholder: 'Person suchen (Name, Amt) …' });
    const ap = amtPicker({ amts: [...new Set(others.map((x) => x.amt).filter(Boolean))], selected: shareAmts, counts: amtCounts, onChange: updHint });
    const det = h('details', { class: 'share full', open: shareEmp.size || shareAmts.length ? true : null },
      h('summary', null, h('b', null, '🔗 Freigeben'), sumEl),
      h('p', { class: 'hint' }, 'Ausgewählte Personen – oder alle Personen eines Amtes – sehen diesen Kontakt in ihrer Liste (nur lesen).'),
      h('div', { class: 'share-cols' }, h('div', null, h('h4', null, '👥 Personen'), pp.el), h('div', null, h('h4', null, '🏛 Ämter'), ap.el)));
    body.append(det);
    body.prepend(hint);
    updHint();
  } else {
    orgSel = unitSelect(p ? p.orgUnit || 0 : (S.view === 'org' && S.orgCur > 0 ? S.orgCur : 0));
    chanSel = channelSelect(p ? p.channel || 0 : 0);
    body.append(h('div', { class: 'full' }, field('Bevorzugter Kontaktweg', chanSel, S.channels.length ? 'Zeigt auf der Visitenkarte „Bitte bevorzugt so kontaktieren“.' : 'Noch keine Kontaktwege angelegt (Verwaltung → Kontaktwege).')));
    body.append(h('div', { class: 'full' }, field('Organisation', orgSel, S.org.units.length ? 'Position im Organigramm. Die Anzeige „Amt“ bleibt davon unabhängig.' : 'Noch keine Einheiten angelegt (Verwaltung → Organisation).')));
    user = h('input', { type: 'text', value: p ? p.username || '' : '', maxlength: 64, autocomplete: 'off', placeholder: 'z. B. mmuster' });
    hidden = h('input', { type: 'checkbox', checked: p && p.hidden ? true : null });
    body.append(field('Benutzername (Windows-Anmeldung)', user, 'Verknüpft den Eintrag mit dem Konto – dann kann die Person ihre Schlagworte und ihren Avatar selbst pflegen.'),
      h('label', { class: 'check' }, hidden, ' Im Telefonbuch ausblenden'));
    if (p && p.source === 'ad') body.prepend(h('p', { class: 'hint full' }, 'Dieser Eintrag wird mit dem Active Directory abgeglichen. Mit 🔒 markierte Felder werden dort gepflegt.'));
  }
  const buttons = [];
  if (!isNew) buttons.push({ label: '🗑 Löschen', cls: 'danger', onClick: async () => {
    const msg = kind === 'priv' ? 'Diesen privaten Kontakt löschen?' : (p.source === 'ad' && S.me.features.syncEnabled ? 'Eintrag löschen? Hinweis: AD-Einträge werden beim nächsten Abgleich wieder angelegt – zum dauerhaften Verbergen „ausblenden“ verwenden.' : 'Eintrag mitsamt Schlagworten und Avatar löschen?');
    if (!(await confirmDialog('Löschen', msg, 'Löschen', true))) return false;
    try {
      await api('DELETE', kind === 'priv' ? `/api/private/${p.id}` : `/api/employees/${p.id}`);
      toast('Gelöscht', 'ok'); S.selPeople.clear(); await reload();
    } catch (e) { toast(e.message, 'err'); }
  } });
  buttons.push('spacer', { label: 'Abbrechen' }, { label: 'Speichern', cls: 'primary', onClick: async () => {
    const data = {};
    for (const k of Object.keys(inputs)) data[k] = inputs[k].value;
    if (kind === 'priv') { data.note = note.value; data.shares = { employees: [...shareEmp], amts: shareAmts }; }
    else { data.username = user.value; data.hidden = hidden.checked; data.orgUnit = +orgSel.value; data.channel = +chanSel.value; }
    try {
      if (kind === 'priv') await api(isNew ? 'POST' : 'PUT', isNew ? '/api/private' : `/api/private/${p.id}`, data);
      else await api(isNew ? 'POST' : 'PUT', isNew ? '/api/employees' : `/api/employees/${p.id}`, data);
      toast('Gespeichert', 'ok');
      await reload();
    } catch (e) { toast(e.message, 'err'); return false; }
  } });
  openDialog({ title: isNew ? (kind === 'priv' ? 'Privater Kontakt' : 'Neuer Eintrag') : (kind === 'priv' ? 'Privaten Kontakt bearbeiten' : 'Stammdaten bearbeiten'), body, buttons, wide: kind === 'priv' });
  setTimeout(() => (inputs.lastName.disabled ? inputs.phone : inputs.lastName).focus(), 30);
}

// ---------- Admin: Schlagworte ----------

async function tagManager() {
  let tags;
  try { tags = (await api('GET', '/api/tags')).tags; } catch (e) { toast(e.message, 'err'); return; }
  const q = h('input', { type: 'search', placeholder: 'Filtern …' });
  const tbody = h('tbody');
  const newName = h('input', { type: 'text', placeholder: 'Neues Schlagwort', maxlength: 60 });
  const newCat = h('select', null, S.cats.map((c) => h('option', { value: c.key }, c.name)));
  const refresh = async () => { tags = (await api('GET', '/api/tags')).tags; render(); reload(); };
  const render = () => {
    const qq = norm(q.value);
    tbody.replaceChildren(...tags.filter((t) => !qq || norm(t.name).includes(qq)).map((t) => {
      const name = h('input', { type: 'text', value: t.name, maxlength: 60 });
      const cat = h('select', null, S.cats.map((c) => h('option', { value: c.key, selected: c.key === t.category || null }, c.name)));
      const merge = h('select', { title: 'In anderes Schlagwort überführen' }, h('option', { value: '' }, 'zusammenführen mit …'), tags.filter((x) => x.id !== t.id).map((x) => h('option', { value: x.id }, x.name)));
      const save = async () => {
        if (name.value === t.name && cat.value === t.category) return;
        try { await api('PUT', `/api/tags/${t.id}`, { name: name.value, category: cat.value }); toast('Gespeichert', 'ok'); await refresh(); } catch (e) { toast(e.message, 'err'); }
      };
      name.addEventListener('change', save);
      cat.addEventListener('change', save);
      merge.addEventListener('change', async () => {
        const into = +merge.value;
        if (!into) return;
        const target = tags.find((x) => x.id === into);
        if (!(await confirmDialog('Zusammenführen', `„${t.name}“ (${t.count}×) in „${target.name}“ überführen? „${t.name}“ wird danach gelöscht.`, 'Zusammenführen'))) { tagManager(); return; }
        try { await api('POST', `/api/tags/${t.id}/merge`, { into }); toast('Zusammengeführt', 'ok'); } catch (e) { toast(e.message, 'err'); }
        await reload(); tagManager();
      });
      const del = h('button', { type: 'button', class: 'icon danger', title: 'Löschen', onclick: async () => {
        if (!(await confirmDialog('Schlagwort löschen', `„${t.name}“ löschen? Es wird bei ${t.count} Person(en) entfernt.`, 'Löschen', true))) { tagManager(); return; }
        try { await api('DELETE', `/api/tags/${t.id}`); toast('Gelöscht', 'ok'); } catch (e) { toast(e.message, 'err'); }
        await reload(); tagManager();
      } }, '🗑');
      return h('tr', null, h('td', null, name), h('td', null, cat), h('td', { class: 'num' }, String(t.count)), h('td', { class: 'muted' }, t.createdBy || ''), h('td', null, merge), h('td', null, del));
    }));
  };
  q.addEventListener('input', render);
  render();
  openDialog({
    title: 'Schlagworte verwalten', wide: true,
    body: h('div', { class: 'admin' },
      h('div', { class: 'toolbar' }, q, h('span', { class: 'grow' }), newName, newCat, h('button', { type: 'button', onclick: async () => {
        try { await api('POST', '/api/tags', { name: newName.value, category: newCat.value }); newName.value = ''; toast('Angelegt', 'ok'); await refresh(); } catch (e) { toast(e.message, 'err'); }
      } }, '＋ Anlegen')),
      h('div', { class: 'table-wrap' }, h('table', { class: 'tbl' }, h('thead', null, h('tr', null, ['Name', 'Kategorie', 'Personen', 'Angelegt von', '', ''].map((x) => h('th', null, x)))), tbody))),
    buttons: [{ label: '🗂 Kategorien …', onClick: () => { categoryManager(); return false; } }, { label: '🧹 Unbenutzte löschen', onClick: async () => {
      try { const r = await api('POST', '/api/tags/cleanup', {}); toast(`${r.deleted} unbenutzte Schlagworte gelöscht`, 'ok'); await refresh(); } catch (e) { toast(e.message, 'err'); }
      return false;
    } }, 'spacer', { label: 'Schließen' }],
  });
}

// ---------- Kategorien ----------

function renderCatFilter() {
  const box = $('#cat-filter');
  const btn = (key, label, d) => h('button', { type: 'button', class: S.catFilter === key ? 'active' : '', onclick: () => { S.catFilter = key; renderCatFilter(); renderTags(); } }, d, label);
  box.replaceChildren(btn('', 'Alle'), ...S.cats.filter((c) => S.me.isAdmin || S.tags.some((t) => t.category === c.key && t.count > 0)).map((c) => btn(c.key, c.name, dot(c.key))));
}

// Person für die Auswahllisten (Freigabe, Avatar übertragen)
function pickPerson(x) {
  return { id: x.id, lastName: x.lastName, firstName: x.firstName, amt: x.amt, avatar: x.avatar, avatarURL: avatarURL(x, 28) };
}

const CAT_COLORS = ['#3b7dd8', '#d48a14', '#7c8591', '#2a9d8f', '#8338ec', '#e63946', '#2f6b2f', '#c2185b', '#5d4037', '#0097a7'];

async function categoryManager() {
  await reload();
  const tbody = h('tbody');
  const rows = S.cats.map((c) => {
    const name = h('input', { type: 'text', value: c.name, maxlength: 40, 'aria-label': 'Name' });
    const color = h('input', { type: 'color', value: c.color, title: 'Farbe', 'aria-label': 'Farbe' });
    const sort = h('input', { type: 'number', value: c.sort, min: 0, max: 9999, step: 10, class: 'num-in', title: 'Reihenfolge (kleiner = weiter vorne)', 'aria-label': 'Reihenfolge' });
    const save = async () => {
      if (name.value === c.name && color.value === c.color && +sort.value === c.sort) return;
      try { await api('PUT', `/api/categories/${c.key}`, { name: name.value, color: color.value, sort: +sort.value }); toast('Gespeichert', 'ok'); await reload(); categoryManager(); }
      catch (e) { toast(e.message, 'err'); }
    };
    for (const el of [name, color, sort]) el.addEventListener('change', save);
    const del = h('button', { type: 'button', class: 'icon danger', title: 'Löschen', disabled: S.cats.length <= 1 || null, onclick: () => deleteCategory(c) }, '🗑');
    return h('tr', null, h('td', null, color), h('td', null, name), h('td', null, sort), h('td', { class: 'num' }, String(c.count ?? '')), h('td', { class: 'muted mono' }, c.key), h('td', null, del));
  });
  tbody.replaceChildren(...rows);
  const newName = h('input', { type: 'text', placeholder: 'Neue Kategorie, z. B. „Sprachen“', maxlength: 40 });
  const newColor = h('input', { type: 'color', value: CAT_COLORS[S.cats.length % CAT_COLORS.length], title: 'Farbe' });
  const create = async () => {
    try { await api('POST', '/api/categories', { name: newName.value, color: newColor.value }); toast('Kategorie angelegt', 'ok'); await reload(); categoryManager(); }
    catch (e) { toast(e.message, 'err'); }
  };
  newName.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); create(); } });
  openDialog({
    title: 'Kategorien verwalten', wide: true,
    body: h('div', { class: 'admin' },
      h('p', { class: 'hint' }, 'Kategorien gruppieren die Schlagworte (Farbe in der Schlagwortliste, Filter oben rechts). Änderungen werden sofort gespeichert.'),
      h('div', { class: 'toolbar' }, newColor, newName, h('button', { type: 'button', class: 'primary', onclick: create }, '＋ Anlegen')),
      h('div', { class: 'table-wrap' }, h('table', { class: 'tbl' }, h('thead', null, h('tr', null, ['Farbe', 'Name', 'Reihenfolge', 'Schlagworte', 'Schlüssel', ''].map((x) => h('th', null, x)))), tbody))),
    buttons: [{ label: '🏷 Schlagworte …', onClick: () => { tagManager(); return false; } }, 'spacer', { label: 'Schließen' }],
  });
  setTimeout(() => newName.focus(), 30);
}

function deleteCategory(c) {
  const others = S.cats.filter((x) => x.key !== c.key);
  const def = others.find((x) => x.key === 'sonstiges') || others[0];
  const sel = h('select', null, others.map((x) => h('option', { value: x.key, selected: x === def || null }, x.name)));
  openDialog({
    title: `Kategorie „${c.name}“ löschen`,
    body: h('div', { class: 'admin' },
      h('p', null, c.count ? `${c.count} Schlagwort${c.count === 1 ? '' : 'e'} gehör${c.count === 1 ? 't' : 'en'} zu dieser Kategorie. Sie werden in die Folge-Kategorie verschoben:` : 'Diese Kategorie enthält keine Schlagworte. Folge-Kategorie (für alle Fälle):'),
      field('Folge-Kategorie', sel)),
    buttons: ['spacer', { label: 'Abbrechen', onClick: () => { categoryManager(); return false; } }, { label: '🗑 Löschen', cls: 'danger', onClick: async () => {
      try {
        const r = await api('POST', `/api/categories/${c.key}/delete`, { moveTo: sel.value });
        toast(`Gelöscht – ${r.moved} Schlagwort${r.moved === 1 ? '' : 'e'} verschoben`, 'ok');
        if (S.catFilter === c.key) S.catFilter = '';
      } catch (e) { toast(e.message, 'err'); }
      categoryManager();
      return false;
    } }],
  });
}

// ---------- Ansicht: Liste / Organigramm ----------

function setView(v) {
  S.view = ['org', 'village'].includes(v) ? v : 'list';
  $('#main').classList.toggle('view-org', S.view !== 'list');
  $$('#viewbar button[data-view]').forEach((b) => b.classList.toggle('active', b.dataset.view === S.view));
  $('#orgview').hidden = S.view !== 'org';
  $('#village').hidden = S.view !== 'village';
  try { localStorage.setItem('tb.view', S.view); } catch { /* egal */ }
}

// ---------- Hilfen: Datum, Personen, Einheiten ----------

function fmtDay(d) {
  if (!d) return '';
  const [y, m, t] = d.split('-');
  return `${t}.${m}.${y === S.today.slice(0, 4) ? '' : y}`;
}
const absNow = (p) => (p.absences || []).find((a) => a.from <= S.today && a.to >= S.today) || null;
const empByID = (id) => S.byKey.get('e' + id) || null;
const nameOf = (x) => (x ? [x.firstName, x.lastName].filter(Boolean).join(' ') : '');
function absDeputies(p, a) { return (a && a.deputies && a.deputies.length ? a.deputies : p.deputies || []).map(empByID).filter(Boolean); }
function absenceText(p, a) {
  const dep = absDeputies(p, a).map(nameOf).join(', ');
  return `Abwesend ${fmtDay(a.from)}–${fmtDay(a.to)}` + (dep ? ` · Vertretung: ${dep}` : '');
}

// Sprung zu einer Person (Visitenkarte rechts)
function goPerson(key) {
  const p = S.byKey.get(key);
  if (!p) return;
  if (p.kind === 'emp' && (!p.active || p.hidden) && !S.showHidden) { toast('Dieser Eintrag ist ausgeblendet', 'err'); return; }
  const row = S.rows.get(key);
  if (row && row.hidden) resetAll(true);
  S.selPeople = new Set([key]);
  update();
  const r = S.rows.get(key);
  if (r && S.view === 'list') r.scrollIntoView({ block: 'nearest' });
  if (isMobile() && S.tab !== 'tags') setTab('tags');
}
function personLink(p, extra) {
  if (!p) return null;
  const a = absNow(p);
  return h('button', { type: 'button', class: 'plink', title: a ? absenceText(p, a) : 'Zur Visitenkarte', onclick: () => goPerson(p.key) },
    h('img', { src: avatarURL(p, 20), alt: '', width: 20, height: 20 }), nameOf(p) || p.display, a ? ' 🏖' : '', extra || '');
}
const links = (list) => list.flatMap((p, i) => (i ? [', ', personLink(p)] : [personLink(p)]));

function unitPath(u) {
  const out = [];
  for (let x = u, n = 0; x && n < 16; x = S.unitById.get(x.parent), n++) out.unshift(x);
  return out;
}
const unitDepth = (u) => unitPath(u).length;
const unitLevel = (u) => S.org.levels[unitDepth(u) - 1] || { name: 'Ebene ' + unitDepth(u) };
const unitLabel = (u) => (u.short ? u.short + ' – ' : '') + u.name;
const visibleEmp = (p) => p.kind === 'emp' && (S.showHidden || (p.active && !p.hidden));

function unitMembers(id, subtree) {
  const ids = new Set([id]);
  if (subtree) {
    const walk = (k) => { for (const c of S.unitKids.get(k) || []) { ids.add(c.id); walk(c.id); } };
    walk(id);
  }
  return S.people.filter((p) => visibleEmp(p) && (id === -1 ? !S.unitById.has(p.orgUnit) : ids.has(p.orgUnit)));
}

// Leitung/Stellvertretung: Zuordnung zur Einheit + Schlagwort der Ebene, dazu fest eingetragene Personen
function unitLeads(u) {
  const lv = unitLevel(u);
  const members = unitMembers(u.id, false);
  const pick = (tag, manual) => {
    const out = new Map();
    for (const id of manual || []) { const p = empByID(id); if (p && visibleEmp(p)) out.set(p.key, p); }
    if (tag) for (const p of members) if (p.tagSet.has(tag)) out.set(p.key, p);
    return [...out.values()];
  };
  return { leaders: pick(lv.leaderTag, u.leaders), deputies: pick(lv.deputyTag, u.deputies) };
}

// Rollen (Leitung/Stellvertretung) je Person – einmal pro Neuladen berechnet
function computeRoles() {
  S.roleMap = new Map();
  const add = (p, r) => { if (!S.roleMap.has(p.key)) S.roleMap.set(p.key, []); S.roleMap.get(p.key).push(r); };
  for (const u of S.org.units) {
    const { leaders, deputies } = unitLeads(u);
    const lv = unitLevel(u);
    for (const p of leaders) add(p, { unit: u.id, lead: true, label: '★ Leitung ' + (u.short || u.name), title: `Leitung (${lv.name}): ${unitLabel(u)}` });
    for (const p of deputies) if (!leaders.includes(p)) add(p, { unit: u.id, lead: false, label: '☆ Stv. Leitung ' + (u.short || u.name), title: `Stellvertretende Leitung (${lv.name}): ${unitLabel(u)}` });
  }
}
const personRoles = (p) => (S.roleMap && S.roleMap.get(p.key)) || [];

function cardOrgLines(p) {
  const rows = [];
  const kv = (k, ...v) => h('div', { class: 'kv' }, h('span', { class: 'k' }, k), h('span', { class: 'kv-v' }, ...v));
  const u = S.unitById.get(p.orgUnit);
  if (u) {
    const path = unitPath(u);
    rows.push(kv('🏢 Einheit', ...path.flatMap((x, i) => [i ? ' › ' : '', h('button', { type: 'button', class: 'linkish', title: unitLabel(x), onclick: () => { setView('org'); S.orgCur = x.id; update(); if (isMobile()) setTab('people'); } }, i === path.length - 1 ? unitLabel(x) : x.short || x.name)])));
  }
  const a = absNow(p);
  if (a) rows.push(h('div', { class: 'kv away' }, h('span', { class: 'k' }, '🏖 Abwesend'), h('span', { class: 'kv-v' }, `${fmtDay(a.from)} – ${fmtDay(a.to)}`, ...(absDeputies(p, a).length ? [h('br'), 'Vertretung: ', ...links(absDeputies(p, a))] : []))));
  const deps = (p.deputies || []).map(empByID).filter(Boolean);
  if (deps.length && !(a && a.deputies && a.deputies.length)) rows.push(kv('👥 Vertretung', ...links(deps)));
  else if (deps.length) rows.push(kv('👥 Vertretung', ...links(deps), h('small', { class: 'muted' }, ' (sonst)')));
  const next = (p.absences || []).filter((x) => x.from > S.today).slice(0, 2);
  if (next.length) rows.push(kv('📅 Geplant', next.map((x) => `${fmtDay(x.from)}–${fmtDay(x.to)}`).join(', ')));
  const reverse = S.people.filter((x) => x.kind === 'emp' && (x.deputies || []).includes(p.id) && visibleEmp(x));
  if (reverse.length) rows.push(kv('↩ Vertritt', ...links(reverse.slice(0, 8)), reverse.length > 8 ? ` +${reverse.length - 8}` : ''));
  return rows;
}

// Auswahlfeld: Einheiten eingerückt nach Ebene
function unitSelect(cur, exclude) {
  const sel = h('select', null, h('option', { value: 0 }, '— keine —'));
  const walk = (k, depth) => {
    for (const u of S.unitKids.get(k) || []) {
      if (exclude && u.id === exclude) continue;
      sel.append(h('option', { value: u.id, selected: u.id === cur || null }, '  '.repeat(depth) + unitLabel(u) + ` (${(S.org.levels[depth] || {}).name || ''})`));
      walk(u.id, depth + 1);
    }
  };
  walk(0, 0);
  return sel;
}

// ---------- Bevorzugter Kontaktweg (eigenständig, Personen gezielt zugewiesen) ----------

function personContact(p) {
  const ch = p && p.channel ? S.chanById.get(p.channel) : null;
  return ch ? { contact: ch, channel: ch } : null;
}
function contactSummary(c) {
  return [c.ticketUrl ? (c.ticketLabel || 'Ticketsystem') : '', c.email || '', c.phone ? 'Hotline ' + c.phone : ''].filter(Boolean).join(' · ');
}
function contactBox(c) {
  return h('div', { class: 'contact-pref' },
    h('div', { class: 'cp-head' }, '💬 Bitte bevorzugt so kontaktieren', c.name ? h('small', { class: 'muted' }, ' – ' + c.name) : null),
    h('div', { class: 'cp-btns' },
      c.ticketUrl && /^https?:\/\//i.test(c.ticketUrl) ? h('a', { class: 'btn primary', href: c.ticketUrl, target: '_blank', rel: 'noopener noreferrer' }, '🎫 ', c.ticketLabel || 'Ticket erstellen') : null,
      c.email ? h('a', { class: 'btn', href: 'mailto:' + c.email }, '✉ ', c.email) : null,
      c.phone ? h('a', { class: 'btn', href: telHref(c.phone) }, '☎ Hotline ', c.phone) : null),
    c.note ? h('div', { class: 'cp-note muted' }, c.note) : null);
}

function channelSelect(cur) {
  return h('select', null, h('option', { value: 0 }, '— keiner (direkt erreichbar) —'), S.channels.map((c) => h('option', { value: c.id, selected: c.id === cur || null }, c.name + ' – ' + contactSummary(c))));
}

// ---------- Admin: Kontaktwege ----------

async function channelManager() {
  await reload();
  const count = (id) => S.people.filter((p) => p.kind === 'emp' && p.channel === id).length;
  const rows = S.channels.map((c) => h('div', { class: 'org-trow' },
    h('b', null, c.name),
    h('span', { class: 'badge via' }, contactSummary(c)),
    c.hideDirect ? h('span', { class: 'badge', title: 'Liste: Hotline statt Durchwahl, Karte: Durchwahl eingeklappt' }, 'Durchwahl zurückhaltend') : null,
    h('span', { class: 'muted' }, `${count(c.id)} Person(en)`),
    h('span', { class: 'grow' }),
    h('button', { type: 'button', class: 'icon', title: 'Personen zuweisen', onclick: () => channelMembersDialog(c) }, '👥'),
    h('button', { type: 'button', class: 'icon', title: 'Bearbeiten', onclick: () => channelDialog(c) }, '✏️'),
    h('button', { type: 'button', class: 'icon danger', title: 'Löschen', onclick: async () => {
      if (!(await confirmDialog('Kontaktweg löschen', `„${c.name}“ löschen? Bei ${count(c.id)} Person(en) entfällt der Hinweis.`, 'Löschen', true))) { channelManager(); return; }
      try { await api('DELETE', `/api/channels/${c.id}`); toast('Gelöscht', 'ok'); } catch (e) { toast(e.message, 'err'); }
      channelManager();
    } }, '🗑')));
  openDialog({
    title: 'Kontaktwege verwalten', wide: true,
    body: h('div', { class: 'admin' },
      h('p', { class: 'hint' }, 'Ein Kontaktweg sagt „Bitte bevorzugt so erreichen“ – z. B. „IT-Support“ mit Ticketsystem, Funktionspostfach und Hotline. Er wird unabhängig von der Organisation angelegt und Personen gezielt zugewiesen (👥 oder im Stammdaten-Dialog).'),
      h('div', { class: 'toolbar' }, h('button', { type: 'button', class: 'primary', onclick: () => channelDialog(null) }, '＋ Kontaktweg anlegen')),
      rows.length ? h('div', { class: 'org-tree' }, rows) : h('p', { class: 'muted' }, 'Noch keine Kontaktwege.')),
    buttons: ['spacer', { label: 'Schließen', cls: 'primary' }],
  });
}

function channelDialog(c) {
  const c0 = c || {};
  const ci = {
    name: h('input', { type: 'text', value: c0.name || '', maxlength: 100, placeholder: 'z. B. IT-Support' }),
    ticketUrl: h('input', { type: 'url', value: c0.ticketUrl || '', maxlength: 500, placeholder: 'https://ticket.firma.local/neu' }),
    ticketLabel: h('input', { type: 'text', value: c0.ticketLabel || '', maxlength: 40, placeholder: 'Ticket erstellen' }),
    email: h('input', { type: 'email', value: c0.email || '', maxlength: 254, placeholder: 'it-support@firma.local' }),
    phone: h('input', { type: 'tel', value: c0.phone || '', maxlength: 64, placeholder: 'z. B. 0123 456-999' }),
    note: h('input', { type: 'text', value: c0.note || '', maxlength: 200, placeholder: 'z. B. Bitte nicht direkt anrufen – so geht nichts verloren.' }),
    hideDirect: h('input', { type: 'checkbox', checked: c0.hideDirect || null }),
  };
  openDialog({
    title: c ? `Kontaktweg: ${c.name}` : 'Kontaktweg anlegen', wide: true,
    body: h('div', { class: 'form-grid' },
      h('div', { class: 'full' }, field('Name', ci.name, 'Nur für die Verwaltung und als Hinweis auf der Visitenkarte.')),
      field('Ticketsystem (Adresse)', ci.ticketUrl), field('Beschriftung', ci.ticketLabel), field('E-Mail (Funktionspostfach)', ci.email), field('Hotline', ci.phone),
      h('div', { class: 'full' }, field('Hinweis', ci.note)),
      h('label', { class: 'check full' }, ci.hideDirect, ' Direkte Durchwahlen zurückhaltend zeigen (Liste: Hotline statt Durchwahl, Karte: Durchwahl eingeklappt)'),
      h('p', { class: 'hint full' }, 'Mindestens Ticketsystem, E-Mail oder Hotline angeben.')),
    buttons: ['spacer', { label: 'Zurück', onClick: () => { channelManager(); return false; } }, { label: 'Speichern', cls: 'primary', onClick: async () => {
      const body = { name: ci.name.value, ticketUrl: ci.ticketUrl.value, ticketLabel: ci.ticketLabel.value, email: ci.email.value, phone: ci.phone.value, note: ci.note.value, hideDirect: ci.hideDirect.checked };
      try {
        if (c) await api('PUT', `/api/channels/${c.id}`, body);
        else { const r = await api('POST', '/api/channels', body); toast('Angelegt – jetzt Personen zuweisen', 'ok'); await reload(); channelMembersDialog(S.chanById.get(r.id) || { id: r.id, name: body.name }); return false; }
        toast('Gespeichert', 'ok');
      } catch (e) { toast(e.message, 'err'); return false; }
      channelManager();
      return false;
    } }],
  });
  setTimeout(() => ci.name.focus(), 30);
}

function channelMembersDialog(c) {
  const current = new Set(S.people.filter((p) => p.kind === 'emp' && p.channel === c.id).map((p) => p.id));
  const sel = new Set(current);
  const info = h('span', { class: 'muted' });
  const upd = () => {
    const add = [...sel].filter((id) => !current.has(id)).length, rm = [...current].filter((id) => !sel.has(id)).length;
    info.textContent = `${sel.size} zugewiesen` + (add || rm ? ` (＋${add} / −${rm})` : '');
  };
  const people = S.people.filter((p) => p.kind === 'emp').map((p) => {
    const x = pickPerson(p);
    const o = p.channel && p.channel !== c.id ? S.chanById.get(p.channel) : null;
    if (o) x.amt = (x.amt ? x.amt + ' · ' : '') + 'jetzt: ' + o.name;
    return x;
  });
  const pp = personPicker({ people, selected: sel, onChange: upd, placeholder: 'Personen suchen (Name, Amt) – dann ggf. „alle Treffer auswählen“', selectAll: true, limit: 100 });
  // Hilfe: alle Personen einer Einheit auf einmal auswählen
  const unitSel = unitSelect(0);
  unitSel.options[0].textContent = '— Einheit wählen —';
  const addUnit = h('button', { type: 'button', onclick: () => {
    const id = +unitSel.value;
    if (!id) return;
    for (const p of unitMembers(id, true)) sel.add(p.id);
    pp.render(); upd();
  } }, '＋ alle aus dieser Einheit');
  upd();
  openDialog({
    title: `Personen zuweisen: ${c.name}`, wide: true,
    body: h('div', { class: 'admin' },
      h('p', { class: 'hint' }, 'Nur ausgewählte Personen bekommen den Hinweis „Bitte bevorzugt so kontaktieren“. Wer schon einen anderen Kontaktweg hat, wird umgestellt.'),
      S.org.units.length ? h('div', { class: 'toolbar' }, unitSel, addUnit, h('span', { class: 'muted small' }, 'inkl. Untereinheiten')) : null,
      info, pp.el),
    buttons: ['spacer', { label: 'Zurück', onClick: () => { channelManager(); return false; } }, { label: 'Speichern', cls: 'primary', onClick: async () => {
      const add = [...sel].filter((id) => !current.has(id)), remove = [...current].filter((id) => !sel.has(id));
      try { const r = await api('POST', `/api/channels/${c.id}/members`, { add, remove }); toast(`＋${r.added} / −${r.removed}`, 'ok'); } catch (e) { toast(e.message, 'err'); return false; }
      channelManager();
      return false;
    } }],
  });
  setTimeout(() => pp.focus(), 30);
}

// ---------- Organigramm (linke Seite) ----------

function orgGo(id) { S.orgCur = id; S.selPeople.clear(); update(); $('#orgview').scrollTop = 0; }

function renderOrg() {
  const box = $('#orgview');
  const qMain = tokens($('#q-main').value), qRoom = tokens($('#q-room').value), qAmt = tokens($('#q-amt').value);
  const filtered = qMain.length || qRoom.length || qAmt.length || S.selTags.size;
  if (S.orgCur > 0 && !S.unitById.has(S.orgCur)) S.orgCur = 0;
  const cur = S.orgCur > 0 ? S.unitById.get(S.orgCur) : null;
  const crumbs = h('nav', { class: 'crumbs', 'aria-label': 'Pfad' }, h('button', { type: 'button', class: 'crumb', onclick: () => orgGo(0) }, '🏢 Organisation'),
    ...(cur ? unitPath(cur) : S.orgCur === -1 ? [{ id: -1, name: 'Ohne Zuordnung' }] : []).flatMap((u) => [h('span', { class: 'sep' }, '›'), h('button', { type: 'button', class: 'crumb', onclick: () => orgGo(u.id) }, u.id === -1 ? u.name : unitLabel(u))]));
  const parts = [crumbs];
  const personRow = (p) => {
    const a = absNow(p);
    const row = h('div', { class: 'org-person' + (S.selPeople.has(p.key) ? ' sel' : ''), role: 'option', tabindex: '-1', onclick: (e) => clickPerson(p.key, e) },
      h('img', { src: avatarURL(p, 32), alt: '', width: 32, height: 32 }),
      h('span', { class: 'nm' }, h('span', null, h('b', null, p.lastName), p.firstName ? ', ' + p.firstName : ''), ...personRoles(p).filter((r) => !cur || r.unit === cur.id).map((r) => h('span', { class: 'badge role', title: r.title }, r.lead ? '★ Leitung' : '☆ Stv.')), a ? h('span', { class: 'badge away', title: absenceText(p, a) }, '🏖 bis ' + fmtDay(a.to)) : null),
      (() => {
        const c = personContact(p);
        if (c && c.contact.hideDirect) return h('span', { class: 'via', title: 'Bitte bevorzugt: ' + contactSummary(c.contact) }, c.contact.phone ? h('a', { href: telHref(c.contact.phone), onclick: (e) => e.stopPropagation() }, c.contact.phone) : (c.contact.ticketUrl ? '🎫 Ticket' : '✉ Mail'));
        return p.phone ? h('a', { href: telHref(p.phone), onclick: (e) => e.stopPropagation() }, p.phone) : h('span', { class: 'muted' }, '–');
      })(),
      h('span', { class: 'muted org-amt' }, p.amt || ''));
    return row;
  };
  if (cur) {
    const lv = unitLevel(cur);
    const { leaders, deputies } = unitLeads(cur);
    parts.push(h('div', { class: 'org-head' },
      h('div', { class: 'org-level' }, lv.name),
      h('h3', null, unitLabel(cur)),
      h('div', { class: 'org-leads' },
        h('div', null, h('span', { class: 'k' }, '★ Leitung'), leaders.length ? links(leaders) : h('span', { class: 'muted' }, lv.leaderTag ? '–' : '– (kein Leitungs-Schlagwort für diese Ebene)')),
        h('div', null, h('span', { class: 'k' }, '☆ Stellvertretung'), deputies.length ? links(deputies) : h('span', { class: 'muted' }, '–')))));
  }
  // Untereinheiten
  const kids = S.orgCur === -1 ? [] : S.unitKids.get(cur ? cur.id : 0) || [];
  if (kids.length || (!cur && S.orgCur !== -1)) {
    const cards = kids.map((u) => {
      const n = unitMembers(u.id, true).filter((p) => !filtered || matchPerson(p, qMain, qRoom, qAmt)).length;
      const { leaders } = unitLeads(u);
      return h('button', { type: 'button', class: 'org-card', onclick: () => orgGo(u.id) },
        h('span', { class: 'org-level' }, unitLevel(u).name),
        h('b', null, unitLabel(u)),
        h('span', { class: 'muted' }, `${n} ${n === 1 ? 'Person' : 'Personen'}${(S.unitKids.get(u.id) || []).length ? ` · ${(S.unitKids.get(u.id) || []).length} Untereinheit(en)` : ''}`),
        leaders.length ? h('span', { class: 'org-lead' }, '★ ', leaders.map(nameOf).join(', ')) : null);
    });
    if (!cur) {
      const none = unitMembers(-1, false).length;
      if (none) cards.push(h('button', { type: 'button', class: 'org-card none', onclick: () => orgGo(-1) }, h('span', { class: 'org-level' }, '—'), h('b', null, 'Ohne Zuordnung'), h('span', { class: 'muted' }, `${none} Personen`)));
    }
    if (!kids.length && !cur) parts.push(h('p', { class: 'muted pad' }, S.me.isAdmin ? 'Noch keine Organisationseinheiten. Anlegen unter Verwaltung → 🏢 Organisation verwalten.' : 'Noch keine Organisationseinheiten angelegt.'));
    parts.push(h('div', { class: 'org-grid' }, cards));
  }
  // Personen
  if (cur || S.orgCur === -1) {
    const hasKids = (S.unitKids.get(S.orgCur) || []).length > 0;
    const all = unitMembers(S.orgCur, S.orgAll && hasKids);
    const list = all.filter((p) => matchPerson(p, qMain, qRoom, qAmt)).sort((a, b) => {
      const ra = personRoles(a).length ? 0 : 1, rb = personRoles(b).length ? 0 : 1;
      return ra - rb || collator.compare(a.lastName + ' ' + a.firstName, b.lastName + ' ' + b.firstName);
    });
    parts.push(h('div', { class: 'org-sub' },
      h('b', null, `👥 ${list.length}${filtered ? ' von ' + all.length : ''} ${all.length === 1 ? 'Person' : 'Personen'}`),
      hasKids ? h('label', { class: 'check' }, h('input', { type: 'checkbox', checked: S.orgAll || null, onchange: (e) => { S.orgAll = e.target.checked; update(); } }), ' inkl. Untereinheiten') : null,
      S.me.isAdmin && cur ? h('button', { type: 'button', class: 'linkish', onclick: () => membersDialog(cur) }, '✏️ Zuordnung') : null),
      list.length ? h('div', { class: 'org-people', role: 'listbox' }, list.map(personRow)) : h('p', { class: 'muted pad' }, filtered ? 'Niemand passt zum Filter.' : 'Hier ist (direkt) niemand zugeordnet.'));
  }
  box.replaceChildren(...parts);
}

// ---------- Vertretung & Abwesenheit ----------

function pickList(exceptId) {
  return S.people.filter((x) => x.kind === 'emp' && x.active && !x.hidden && x.id !== exceptId).map(pickPerson);
}

function editDeputies(p) {
  if (!p) return;
  const sel = new Set(p.deputies || []);
  const pp = personPicker({ people: pickList(p.id), selected: sel, placeholder: 'Vertretung suchen (Name, Amt) …' });
  openDialog({
    title: p.self ? 'Meine Vertretung' : `Vertretung: ${nameOf(p)}`, wide: true,
    body: h('div', { class: 'admin' }, h('p', { class: 'hint' }, 'Wer vertritt dich? Die Personen erscheinen auf deiner Visitenkarte („Vertretung: …“) – in der Reihenfolge der Auswahl. Höchstens 10.'), pp.el),
    buttons: ['spacer', { label: 'Abbrechen' }, { label: 'Speichern', cls: 'primary', onClick: async () => {
      try { await api('PUT', `/api/employees/${p.id}/deputies`, { deputies: [...sel] }); toast('Vertretung gespeichert', 'ok'); await reload(); } catch (e) { toast(e.message, 'err'); return false; }
    } }],
  });
  setTimeout(() => pp.focus(), 30);
}

function editAbsences(p) {
  if (!p) return;
  const rows = (p.absences || []).map((a) => ({ from: a.from, to: a.to, deputies: new Set(a.deputies || []) }));
  const list = h('div', { class: 'abs-list' });
  const render = () => {
    list.replaceChildren(...(rows.length ? rows.map((r, i) => {
      const from = h('input', { type: 'date', value: r.from, min: S.today, required: true, onchange: (e) => { r.from = e.target.value; if (!r.to || r.to < r.from) { r.to = r.from; render(); } } });
      const to = h('input', { type: 'date', value: r.to, min: S.today, required: true, onchange: (e) => { r.to = e.target.value; } });
      const pp = personPicker({ people: pickList(p.id), selected: r.deputies, placeholder: 'Abweichende Vertretung …' });
      return h('div', { class: 'abs-row' },
        h('div', { class: 'abs-dates' }, field('Von', from), field('Bis (einschließlich)', to),
          h('button', { type: 'button', class: 'icon danger', title: 'Entfernen', onclick: () => { rows.splice(i, 1); render(); } }, '🗑')),
        h('details', { open: r.deputies.size ? true : null }, h('summary', null, `Vertretung für diesen Zeitraum${r.deputies.size ? ` (${r.deputies.size})` : ' – sonst die übliche'}`), pp.el));
    }) : [h('p', { class: 'muted' }, 'Keine Abwesenheit eingetragen.')]));
  };
  render();
  openDialog({
    title: p.self ? 'Meine Abwesenheit' : `Abwesenheit: ${nameOf(p)}`, wide: true,
    body: h('div', { class: 'admin' },
      h('p', { class: 'hint' }, 'Nur der Zeitraum wird angezeigt – kein Grund. Vergangene Einträge werden automatisch gelöscht.' + (S.site.absences === 'all' ? ' Sichtbar für alle, die das Telefonbuch öffnen.' : ' Sichtbar für angemeldete Personen.')),
      list,
      h('button', { type: 'button', onclick: () => { rows.push({ from: S.today, to: S.today, deputies: new Set() }); render(); } }, '＋ Zeitraum hinzufügen')),
    buttons: ['spacer', { label: 'Abbrechen' }, { label: 'Speichern', cls: 'primary', onClick: async () => {
      try {
        await api('PUT', `/api/employees/${p.id}/absences`, { absences: rows.map((r) => ({ from: r.from, to: r.to, deputies: [...r.deputies] })) });
        toast('Abwesenheit gespeichert', 'ok'); await reload();
      } catch (e) { toast(e.message, 'err'); return false; }
    } }],
  });
}

// ---------- Fehler melden ----------

const REPORT_FIELDS = [['phone', 'Telefon'], ['mobile', 'Mobil'], ['room', 'Raum'], ['amt', 'Amt'], ['name', 'Name'], ['email', 'E-Mail'], ['org', 'Organisation'], ['deputies', 'Vertretung'], ['other', 'Sonstiges']];
function currentValue(p, f) {
  if (!p) return '';
  switch (f) {
    case 'name': return p.display;
    case 'org': { const u = S.unitById.get(p.orgUnit); return u ? unitPath(u).map(unitLabel).join(' › ') : '—'; }
    case 'deputies': return (p.deputies || []).map(empByID).filter(Boolean).map(nameOf).join(', ') || '—';
    case 'other': return '';
    default: return p[f] || '—';
  }
}

function reportDialog(p) {
  const fieldSel = h('select', null, REPORT_FIELDS.map(([k, l]) => h('option', { value: k }, l)));
  const curEl = h('div', { class: 'kv' });
  const sugg = h('input', { type: 'text', maxlength: 255, placeholder: 'Wie lautet die richtige Angabe?' });
  const msg = h('textarea', { rows: 3, maxlength: 500, placeholder: 'z. B. „ist ins Amt 20 gewechselt“' });
  const ad = new Set(p.source === 'ad' ? S.me.features.adLocked || [] : []);
  const showCur = () => {
    const f = fieldSel.value;
    const v = currentValue(p, f);
    const fmap = { name: 'lastName' };
    curEl.replaceChildren(h('span', { class: 'k' }, 'Aktuell'), h('span', null, v || '—', ad.has(fmap[f] || f) ? h('small', { class: 'muted' }, ' (kommt aus dem Active Directory)') : ''));
    curEl.hidden = f === 'other';
  };
  fieldSel.addEventListener('change', showCur);
  showCur();
  openDialog({
    title: `Fehler melden: ${p.display}`,
    body: h('div', { class: 'form-grid' },
      h('p', { class: 'hint full' }, 'Die Meldung geht an die Administration. Du siehst den Bearbeitungsstand unter Menü → 🚩 Meine Meldungen.'),
      h('div', { class: 'full' }, field('Was stimmt nicht?', fieldSel)), h('div', { class: 'full' }, curEl),
      h('div', { class: 'full' }, field('Richtig wäre', sugg)), h('div', { class: 'full' }, field('Hinweis (optional)', msg))),
    buttons: ['spacer', { label: 'Abbrechen' }, { label: '🚩 Melden', cls: 'primary', onClick: async () => {
      try { await api('POST', `/api/employees/${p.id}/reports`, { field: fieldSel.value, suggestion: sugg.value, message: msg.value }); toast('Danke! Die Meldung ist eingegangen.', 'ok'); }
      catch (e) { toast(e.message, 'err'); return false; }
    } }],
  });
  setTimeout(() => sugg.focus(), 30);
}

const REPORT_STATUS = { open: '⏳ offen', done: '✅ erledigt', rejected: '✖ abgelehnt' };

async function myReportsDialog() {
  let r;
  try { r = await api('GET', '/api/reports/mine'); } catch (e) { toast(e.message, 'err'); return; }
  openDialog({
    title: 'Meine Meldungen', wide: true,
    body: h('div', { class: 'admin' }, r.reports.length ? h('div', { class: 'table-wrap' }, h('table', { class: 'tbl' },
      h('thead', null, h('tr', null, ['Datum', 'Eintrag', 'Was', 'Vorschlag', 'Stand', 'Antwort'].map((x) => h('th', null, x)))),
      h('tbody', null, r.reports.map((x) => h('tr', null, h('td', null, fmtDate(x.createdAt)), h('td', null, x.target), h('td', null, r.fields[x.field] || x.field), h('td', null, x.suggestion || x.message || ''), h('td', null, REPORT_STATUS[x.status] || x.status), h('td', { class: 'msg' }, x.note || '')))))) : h('p', { class: 'muted' }, 'Du hast noch nichts gemeldet.')),
    buttons: ['spacer', { label: 'Schließen', cls: 'primary' }],
  });
}

async function reportsDialog(status = 'open') {
  let r;
  try { r = await api('GET', '/api/admin/reports?status=' + status); } catch (e) { toast(e.message, 'err'); return; }
  S.openReports = r.open;
  const setStatus = async (x, st) => {
    let note = x.note || '';
    const inp = $(`#rn-${x.id}`);
    if (inp) note = inp.value;
    try { await api('PUT', `/api/admin/reports/${x.id}`, { status: st, note }); toast(REPORT_STATUS[st], 'ok'); } catch (e) { toast(e.message, 'err'); }
    reportsDialog(status);
  };
  const psCount = r.reports.filter((x) => x.ps).length;
  const tools = h('div', { class: 'toolbar' },
    psCount ? h('button', { type: 'button', title: 'Alle offenen AD-Meldungen als PowerShell-Skript (Set-ADUser)', onclick: () => psScriptDialog() }, `⧉ PowerShell-Skript (${psCount})`) : null,
    h('button', { type: 'button', title: 'Offene Meldungen erledigen, deren Wert schon im Eintrag steht', onclick: async () => {
      try { const x = await api('POST', '/api/admin/reports/autoclose', {}); toast(`${x.closed} Meldung(en) automatisch erledigt`, 'ok'); } catch (e) { toast(e.message, 'err'); }
      reportsDialog(status);
    } }, '✓ Übernommene erledigen'),
    S.me.features.syncEnabled ? h('button', { type: 'button', onclick: () => syncDialog() }, '🔄 AD-Abgleich …') : null);
  const tabs = h('div', { class: 'seg small' }, Object.entries({ open: 'Offen', done: 'Erledigt', rejected: 'Abgelehnt', all: 'Alle' }).map(([k, l]) => h('button', { type: 'button', class: k === status ? 'active' : '', onclick: () => reportsDialog(k) }, l + (k === 'open' ? ` (${r.open})` : ''))));
  const rows = r.reports.map((x) => {
    const p = x.employeeId ? empByID(x.employeeId) : null;
    const fmap = { name: 'lastName' };
    const adLocked = p && p.source === 'ad' && (S.me.features.adLocked || []).includes(fmap[x.field] || x.field);
    return h('tr', { class: x.status },
      h('td', null, fmtDate(x.createdAt)),
      h('td', null, p ? h('button', { type: 'button', class: 'linkish', onclick: () => { $('#dlg').close(); goPerson(p.key); } }, x.target) : h('span', { class: 'muted', title: 'Eintrag gelöscht' }, x.target)),
      h('td', null, r.fields[x.field] || x.field, adLocked ? h('div', { class: 'badge ad', title: 'Feld kommt aus dem AD – dort korrigieren' }, 'AD') : null),
      h('td', null, p ? currentValue(p, x.field) : ''),
      h('td', null, x.suggestion ? h('b', null, x.suggestion) : '', x.message ? h('div', { class: 'muted' }, x.message) : '',
        x.applied ? h('div', null, h('span', { class: 'badge away', title: 'Der Eintrag enthält den gemeldeten Wert bereits' }, '✓ schon übernommen')) : null,
        x.ps ? h('div', { class: 'ps-line' }, h('code', { title: x.ps }, x.ps), h('button', { type: 'button', class: 'linkish', title: 'PowerShell-Befehl kopieren', onclick: () => copyText(x.ps, 'Befehl kopiert') }, '⧉ PowerShell')) : (x.status === 'open' && x.psWhy && adLocked ? h('div', { class: 'muted small' }, 'Kein AD-Befehl: ' + x.psWhy) : null)),
      h('td', null, x.reporterName || x.reporter),
      h('td', { class: 'rep-act' },
        h('input', { id: `rn-${x.id}`, type: 'text', value: x.note || '', placeholder: 'Antwort/Notiz', maxlength: 500 }),
        h('div', { class: 'btnrow' },
          x.status !== 'done' ? h('button', { type: 'button', title: 'Erledigt', onclick: () => setStatus(x, 'done') }, '✅') : null,
          x.status !== 'rejected' ? h('button', { type: 'button', title: 'Ablehnen', onclick: () => setStatus(x, 'rejected') }, '✖') : null,
          x.status !== 'open' ? h('button', { type: 'button', title: 'Wieder öffnen', onclick: () => setStatus(x, 'open') }, '↺') : null,
          p && !adLocked && x.field !== 'other' ? h('button', { type: 'button', title: 'Eintrag bearbeiten', onclick: () => { if (x.field === 'deputies') editDeputies(p); else editPerson(p); } }, '✏️') : null,
          x.status !== 'open' && x.resolvedBy ? h('small', { class: 'muted' }, `${x.resolvedBy}, ${fmtDate(x.resolvedAt)}`) : null)));
  });
  openDialog({
    title: 'Gemeldete Fehler', wide: true,
    body: h('div', { class: 'admin' }, h('div', { class: 'toolbar' }, tabs, h('span', { class: 'grow' }), tools),
      rows.length ? h('div', { class: 'table-wrap tall' }, h('table', { class: 'tbl reports' },
        h('thead', null, h('tr', null, ['Datum', 'Eintrag', 'Was', 'Aktuell', 'Vorschlag / Hinweis', 'Von', 'Bearbeitung'].map((x) => h('th', null, x)))), h('tbody', null, rows)))
        : h('p', { class: 'muted' }, status === 'open' ? 'Keine offenen Meldungen. 🎉' : 'Keine Meldungen.'),
      h('p', { class: 'hint' }, 'Felder mit „AD“ werden im Active Directory gepflegt. Das Telefonbuch schreibt nicht selbst ins AD, erzeugt aber geprüfte PowerShell-Befehle (Set-ADUser) zum Kopieren. Nach dem nächsten AD-Abgleich werden Meldungen, deren Wert übernommen ist, automatisch erledigt. Die Notiz sieht die meldende Person unter „Meine Meldungen“.')),
    buttons: ['spacer', { label: 'Schließen', cls: 'primary' }],
    onClose: () => reload(),
  });
}

// ---------- Admin: Organisation ----------

async function orgManager() {
  await reload();
  const allTags = S.tags.map((t) => t.name).sort(collator.compare);
  const dlId = 'org-tags';
  const tagName = (id) => (id ? (S.tagById.get(id) || {}).name || '' : '');
  const levels = S.org.levels.map((l) => ({ name: l.name, leaderTag: tagName(l.leaderTag), deputyTag: tagName(l.deputyTag) }));
  const lvBox = h('div', { class: 'lv-list' });
  const renderLv = () => {
    lvBox.replaceChildren(h('div', { class: 'lv-row head' }, h('span', null, 'Ebene'), h('span', null, 'Name'), h('span', null, '★ Schlagwort Leitung'), h('span', null, '☆ Schlagwort Stellvertretung'), h('span')),
      ...levels.map((l, i) => h('div', { class: 'lv-row' },
        h('span', { class: 'muted' }, String(i + 1)),
        h('input', { type: 'text', value: l.name, maxlength: 40, oninput: (e) => { l.name = e.target.value; } }),
        h('input', { type: 'text', value: l.leaderTag, list: dlId, maxlength: 60, placeholder: `z. B. ${l.name || 'Amt'}sleitung`, oninput: (e) => { l.leaderTag = e.target.value; } }),
        h('input', { type: 'text', value: l.deputyTag, list: dlId, maxlength: 60, placeholder: `z. B. Stv. ${l.name || 'Amt'}sleitung`, oninput: (e) => { l.deputyTag = e.target.value; } }),
        h('button', { type: 'button', class: 'icon danger', title: 'Ebene entfernen', disabled: levels.length <= 1 || null, onclick: () => { levels.splice(i, 1); renderLv(); } }, '🗑'))),
      h('datalist', { id: dlId }, allTags.map((t) => h('option', { value: t }))));
  };
  renderLv();
  const tree = h('div', { class: 'org-tree' });
  const walk = (k, depth) => {
    for (const u of S.unitKids.get(k) || []) {
      const n = unitMembers(u.id, false).length;
      const { leaders } = unitLeads(u);
      tree.append(h('div', { class: 'org-trow', style: { paddingLeft: 8 + depth * 22 + 'px' } },
        h('span', { class: 'org-level' }, (S.org.levels[depth] || {}).name || ''),
        h('b', null, unitLabel(u)),
        h('span', { class: 'muted' }, `${n} direkt${leaders.length ? ' · ★ ' + leaders.map(nameOf).join(', ') : ''}`),
        h('span', { class: 'grow' }),
        depth + 1 < S.org.levels.length ? h('button', { type: 'button', class: 'icon', title: 'Untereinheit anlegen', onclick: () => unitDialog(null, u.id) }, '➕') : null,
        h('button', { type: 'button', class: 'icon', title: 'Personen zuordnen', onclick: () => membersDialog(u, true) }, '👥'),
        h('button', { type: 'button', class: 'icon', title: 'Bearbeiten', onclick: () => unitDialog(u) }, '✏️'),
        h('button', { type: 'button', class: 'icon danger', title: 'Löschen', onclick: async () => {
          const kids = (S.unitKids.get(u.id) || []).length;
          if (!(await confirmDialog('Einheit löschen', `„${unitLabel(u)}“ löschen? ${kids ? kids + ' Untereinheit(en) und ' : ''}${n} zugeordnete Person(en) wandern eine Ebene nach oben.`, 'Löschen', true))) { orgManager(); return; }
          try { await api('DELETE', `/api/org/units/${u.id}`); toast('Gelöscht', 'ok'); } catch (e) { toast(e.message, 'err'); }
          orgManager();
        } }, '🗑')));
      walk(u.id, depth + 1);
    }
  };
  walk(0, 0);
  if (!tree.children.length) tree.append(h('p', { class: 'muted' }, 'Noch keine Einheiten.'));
  openDialog({
    title: 'Organisation verwalten', wide: true,
    body: h('div', { class: 'admin' },
      h('h4', null, 'Ebenen'),
      h('p', { class: 'hint' }, 'Je Ebene kannst du zwei Schlagworte festlegen. Wer einer Einheit zugeordnet ist und das Leitungs- bzw. Stellvertretungs-Schlagwort dieser Ebene trägt, erscheint dort als Leitung bzw. Stellvertretung. Diese Schlagworte vergeben nur Administratoren. Neue Schlagworte werden in „Sonstiges“ angelegt.'),
      lvBox,
      h('div', { class: 'toolbar' },
        h('button', { type: 'button', disabled: levels.length >= 6 || null, onclick: () => { levels.push({ name: '', leaderTag: '', deputyTag: '' }); renderLv(); } }, '＋ Ebene'),
        h('button', { type: 'button', class: 'primary', onclick: async () => {
          try { await api('PUT', '/api/org/levels', { levels }); toast('Ebenen gespeichert', 'ok'); } catch (e) { toast(e.message, 'err'); return; }
          orgManager();
        } }, 'Ebenen speichern')),
      h('h4', null, 'Einheiten'),
      h('div', { class: 'toolbar' }, h('button', { type: 'button', onclick: () => unitDialog(null, 0) }, `＋ ${(S.org.levels[0] || {}).name || 'Einheit'} anlegen`),
        S.org.units.length ? h('button', { type: 'button', class: 'primary', title: 'Personen anhand ihres Amt-Textes den gleichnamigen Einheiten zuordnen', onclick: () => orgSuggestDialog() }, '🪄 Nach Amt zuordnen …') : null),
      tree),
    buttons: ['spacer', { label: 'Schließen', cls: 'primary' }],
  });
}

function unitDialog(u, parent) {
  const isNew = !u;
  const name = h('input', { type: 'text', value: u ? u.name : '', maxlength: 150 });
  const short = h('input', { type: 'text', value: u ? u.short || '' : '', maxlength: 30, placeholder: 'z. B. 10 oder 10.1' });
  const sort = h('input', { type: 'number', value: u ? u.sort : 0, min: 0, max: 99999, class: 'num-in' });
  // eigene Einheit und deren Untereinheiten sind als Elternteil nicht zulässig
  const parentSel = unitSelect(u ? u.parent || 0 : parent || 0, u ? u.id : 0);
  if (u) {
    const bad = new Set();
    const walk = (k) => { for (const c of S.unitKids.get(k) || []) { bad.add(c.id); walk(c.id); } };
    walk(u.id);
    for (const o of [...parentSel.options]) if (bad.has(+o.value)) o.remove();
  }
  parentSel.options[0].textContent = '— oberste Ebene —';
  const leaders = new Set(u ? u.leaders || [] : []), deputies = new Set(u ? u.deputies || [] : []);
  const lp = personPicker({ people: pickList(0), selected: leaders, placeholder: 'Person suchen …' });
  const dp = personPicker({ people: pickList(0), selected: deputies, placeholder: 'Person suchen …' });
  openDialog({
    title: isNew ? 'Einheit anlegen' : `Einheit: ${unitLabel(u)}`, wide: true,
    body: h('div', { class: 'form-grid' },
      field('Name', name), field('Kürzel', short), field('Übergeordnet', parentSel), field('Sortierung', sort),
      h('details', { class: 'full share' }, h('summary', null, h('b', null, 'Leitung fest eintragen (optional)')),
        h('p', { class: 'hint' }, 'Normalerweise ergibt sich die Leitung aus Zuordnung + Schlagwort der Ebene. Hier eingetragene Personen kommen zusätzlich dazu – z. B. wenn sie keiner Einheit direkt zugeordnet sind.'),
        h('div', { class: 'share-cols' }, h('div', null, h('h4', null, '★ Leitung'), lp.el), h('div', null, h('h4', null, '☆ Stellvertretung'), dp.el)))),
    buttons: ['spacer', { label: 'Zurück', onClick: () => { orgManager(); return false; } }, { label: 'Speichern', cls: 'primary', onClick: async () => {
      const body = { name: name.value, short: short.value, sort: +sort.value || 0, parent: +parentSel.value, leaders: [...leaders], deputies: [...deputies] };
      try {
        if (isNew) await api('POST', '/api/org/units', body); else await api('PUT', `/api/org/units/${u.id}`, body);
        toast('Gespeichert', 'ok');
      } catch (e) { toast(e.message, 'err'); return false; }
      orgManager();
      return false;
    } }],
  });
  setTimeout(() => name.focus(), 30);
}

function membersDialog(u, fromManager) {
  const current = new Set(S.people.filter((p) => p.kind === 'emp' && p.orgUnit === u.id).map((p) => p.id));
  const sel = new Set(current);
  const info = h('span', { class: 'muted' });
  const upd = () => {
    const add = [...sel].filter((id) => !current.has(id)).length, rm = [...current].filter((id) => !sel.has(id)).length;
    info.textContent = `${sel.size} zugeordnet` + (add || rm ? ` (＋${add} / −${rm})` : '');
  };
  const people = S.people.filter((p) => p.kind === 'emp').map((p) => {
    const x = pickPerson(p);
    const o = S.unitById.get(p.orgUnit);
    if (o && o.id !== u.id) x.amt = (x.amt ? x.amt + ' · ' : '') + 'jetzt: ' + (o.short || o.name);
    return x;
  });
  const pp = personPicker({ people, selected: sel, onChange: upd, placeholder: 'Suchen – z. B. den Amt-Text, dann „alle Treffer auswählen“', selectAll: true, limit: 100 });
  upd();
  const back = () => (fromManager ? orgManager() : reload());
  openDialog({
    title: `Zuordnung: ${unitLabel(u)}`, wide: true,
    body: h('div', { class: 'admin' }, h('p', { class: 'hint' }, 'Wer gehört direkt zu dieser Einheit? Personen aus anderen Einheiten werden dabei umgehängt. Die Anzeige „Amt“ bleibt unverändert.'), info, pp.el),
    buttons: ['spacer', { label: 'Abbrechen', onClick: () => { back(); return false; } }, { label: 'Speichern', cls: 'primary', onClick: async () => {
      const add = [...sel].filter((id) => !current.has(id)), remove = [...current].filter((id) => !sel.has(id));
      try { const r = await api('POST', `/api/org/units/${u.id}/members`, { add, remove }); toast(`＋${r.added} / −${r.removed}`, 'ok'); } catch (e) { toast(e.message, 'err'); return false; }
      back();
      return false;
    } }],
  });
  setTimeout(() => pp.focus(), 30);
}

// ---------- Favoriten ----------

async function saveFavorites(list) {
  try {
    const r = await api('PUT', '/api/me/prefs', { favorites: list });
    S.me.prefs = r.prefs;
    S.fav = new Set(r.prefs.favorites);
  } catch (e) { toast(e.message, 'err'); }
  buildRows();
  update();
}

function toggleFav(p) {
  const list = [...S.fav];
  if (S.fav.has(p.key)) list.splice(list.indexOf(p.key), 1);
  else {
    if (list.length >= 10) { toast('Höchstens 10 Favoriten – bitte erst einen entfernen', 'err'); return; }
    list.push(p.key);
  }
  saveFavorites(list);
}

async function feedDialog() {
  const box = h('div', { class: 'admin' });
  const render = (token) => {
    const has = token || (S.me.prefs && S.me.prefs.hasFeed);
    box.replaceChildren(
      h('p', null, 'Deine Favoriten (', h('b', null, String(S.fav.size)), ') können im Windows-Symbol unten rechts als Kurzwahl erscheinen: Rechtsklick → ★ Favoriten → Name anklicken, und es wird gewählt.'),
      h('ol', null,
        h('li', null, 'Hier einen persönlichen Code erzeugen und kopieren.'),
        h('li', null, 'Im Windows-Symbol: Rechtsklick → ★ Favoriten → „Verbinden …“ → Code einfügen.')),
      token ? h('div', { class: 'feed-code' }, h('code', null, token), h('button', { type: 'button', class: 'primary', onclick: () => copyText(token, 'Code kopiert') }, '⧉ Kopieren')) : null,
      token ? h('p', { class: 'hint' }, 'Der Code wird nur jetzt angezeigt. Er erlaubt nur das Lesen deiner Favoriten – behandle ihn trotzdem wie ein Passwort.') : null,
      has && !token ? h('p', { class: 'hint' }, '✅ Ein Code ist eingerichtet. Ein neuer Code macht den alten ungültig.') : null);
  };
  render(null);
  openDialog({
    title: '★ Favoriten im Windows-Symbol', body: box,
    buttons: [
      ...(S.me.prefs && S.me.prefs.hasFeed ? [{ label: 'Code widerrufen', cls: 'danger', onClick: async () => {
        try { await api('DELETE', '/api/me/feed-token'); S.me.prefs.hasFeed = false; toast('Code widerrufen', 'ok'); } catch (e) { toast(e.message, 'err'); }
      } }] : []),
      'spacer', { label: 'Schließen' },
      { label: 'Code erzeugen', cls: 'primary', onClick: async () => {
        try { const r = await api('POST', '/api/me/feed-token', {}); S.me.prefs.hasFeed = true; render(r.token); } catch (e) { toast(e.message, 'err'); }
        return false;
      } }],
  });
}

// ---------- Extras (persönlich + Freigabe durch die Administration) ----------

const FUN = [
  ['village', '🏘 Organigramm als Voxel-Dorf', 'Jede Einheit ist ein kleines Pixel-Haus, davor stehen die Köpfe. Klick aufs Haus öffnet die Einheit.'],
  ['quiz', '🎯 „Wer ist das?“-Quiz', 'Avatar und Schlagworte werden gezeigt – du rätst den Namen. Ideal zum Kennenlernen.'],
  ['terminal', '⌨ Terminal-Modus', 'Mit ^ öffnet sich eine Konsole: whois, finger, grep, ping, ls, cd … inklusive ASCII-Avatar.'],
  ['seasonal', '🎃 Saisonale Avatare', 'Im Dezember Nikolausmützen, an Halloween Kürbisse, am 1. April steht alles Kopf.'],
  ['badges', '🏅 Meine Abzeichen zeigen', 'Zeigt auf deiner Visitenkarte Abzeichen wie „Profil vollständig“ oder „Ersthelfer“. Keine Rangliste.'],
];

function extrasDialog() {
  const global = { ...(S.site.fun || {}) };
  let mine = {};
  if (S.me.guest) { try { mine = JSON.parse(localStorage.getItem('tb.fun') || '{}'); } catch { mine = {}; } }
  else mine = { ...((S.me.prefs && S.me.prefs.fun) || {}) };
  const rows = FUN.map(([k, label, desc]) => {
    const avail = global[k] !== false;
    const cb = h('input', { type: 'checkbox', checked: mine[k] ? true : null, disabled: avail && !(k === 'badges' && S.me.guest) ? null : true, onchange: (e) => { mine[k] = e.target.checked; } });
    return h('label', { class: 'fun-row' + (avail ? '' : ' off') }, cb, h('span', null, h('b', null, label), h('small', { class: 'muted' }, avail ? desc : 'Von der Administration abgeschaltet.' ), k === 'badges' && S.me.guest ? h('small', { class: 'muted' }, ' (nach der Anmeldung)') : null));
  });
  const adminRows = S.me.isAdmin ? FUN.map(([k, label]) => h('label', { class: 'check' }, h('input', { type: 'checkbox', checked: global[k] !== false ? true : null, onchange: (e) => { global[k] = e.target.checked; } }), ' ', label)) : [];
  openDialog({
    title: '🎮 Extras', wide: true,
    body: h('div', { class: 'admin' },
      h('p', { class: 'hint' }, 'Kleine Spielereien – standardmäßig aus. Schalte ein, was dir Spaß macht. Die Einstellung gilt nur für dich' + (S.me.guest ? ' (in diesem Browser).' : '.')),
      h('div', { class: 'fun-list' }, rows),
      S.me.isAdmin ? h('div', null, h('h4', null, 'Für alle freigeben (Administration)'), h('p', { class: 'hint' }, 'Abgeschaltete Extras stehen niemandem zur Verfügung – auch nicht, wer sie persönlich eingeschaltet hat.'), h('div', { class: 'fun-admin' }, adminRows)) : null),
    buttons: ['spacer', { label: 'Abbrechen' }, { label: 'Speichern', cls: 'primary', onClick: async () => {
      try {
        if (S.me.isAdmin) await api('PUT', '/api/admin/fun', Object.fromEntries(FUN.map(([k]) => [k, global[k] !== false])));
        if (S.me.guest) { try { localStorage.setItem('tb.fun', JSON.stringify(mine)); } catch { /* egal */ } }
        else { const r = await api('PUT', '/api/me/prefs', { fun: Object.fromEntries(FUN.map(([k]) => [k, !!mine[k]])) }); S.me.prefs = r.prefs; }
        try { S.site = await api('GET', '/api/site'); } catch { /* egal */ }
        toast('Gespeichert', 'ok');
        await reload();
      } catch (e) { toast(e.message, 'err'); return false; }
    } }],
  });
}

// ---------- Admin: Datenqualität ----------

async function qualityDialog() {
  await reload();
  let reports = [];
  try { reports = (await api('GET', '/api/admin/reports?status=open')).reports; } catch { /* egal */ }
  const emps = S.people.filter((p) => p.kind === 'emp' && p.active && !p.hidden);
  const inact = S.people.filter((p) => p.kind === 'emp' && (!p.active || p.hidden));
  const dupPhones = new Map();
  for (const p of emps) if (p.idxPhone) { if (!dupPhones.has(p.idxPhone)) dupPhones.set(p.idxPhone, []); dupPhones.get(p.idxPhone).push(p); }
  const ageDays = (d) => Math.floor((Date.now() - new Date(d).getTime()) / 86400000);
  const units = S.org.units;
  const checks = [
    ['☎', 'ohne Telefon', emps.filter((p) => !p.phone && !personContact(p))],
    ['🚪', 'ohne Raum', emps.filter((p) => !p.room)],
    ['✉', 'ohne E-Mail', emps.filter((p) => !p.email)],
    ['🏛', 'ohne Amt (Text)', emps.filter((p) => !p.amt)],
    ...(units.length ? [['🏢', 'ohne Organisationseinheit', emps.filter((p) => !S.unitById.has(p.orgUnit))]] : []),
    ['🔁', 'doppelte Durchwahl', [...dupPhones.values()].filter((l) => l.length > 1).flat()],
    ['🔑', 'ohne verknüpftes Konto (können nichts selbst pflegen)', emps.filter((p) => !p.username && p.source === 'manual' && (p.firstName || '').length)],
    ['🏷', 'ohne Schlagworte', emps.filter((p) => !p.tags.length)],
    ['👥', 'ohne Vertretung', emps.filter((p) => !(p.deputies || []).length)],
    ['💤', 'inaktiv oder ausgeblendet', inact],
  ];
  const unitRows = units.length ? [
    ['★', 'Einheiten ohne Leitung', units.filter((u) => !unitLeads(u).leaders.length)],
    ['∅', 'Einheiten ohne Personen (inkl. Untereinheiten)', units.filter((u) => !unitMembers(u.id, true).length)],
  ] : [];
  const oldReports = reports.filter((r) => ageDays(r.createdAt) >= 14);
  const detail = h('div', { class: 'dq-detail' });
  const showPeople = (title, list) => {
    detail.replaceChildren(h('h4', null, `${title} (${list.length})`), list.length ? h('div', { class: 'dq-list' }, list.slice(0, 300).map((p) => h('div', { class: 'dq-item' },
      h('button', { type: 'button', class: 'plink', onclick: () => { $('#dlg').close(); goPerson(p.key); } }, h('img', { src: avatarURL(p, 20), alt: '', width: 20, height: 20 }), p.display),
      h('span', { class: 'muted' }, [p.amt, p.phone].filter(Boolean).join(' · ')),
      h('button', { type: 'button', class: 'icon', title: 'Stammdaten bearbeiten', onclick: () => editPerson(p) }, '✏️')))) : h('p', { class: 'muted' }, 'Alles in Ordnung. 🎉'));
  };
  const showUnits = (title, list) => {
    detail.replaceChildren(h('h4', null, `${title} (${list.length})`), h('div', { class: 'dq-list' }, list.map((u) => h('div', { class: 'dq-item' },
      h('button', { type: 'button', class: 'linkish', onclick: () => { $('#dlg').close(); setView('org'); orgGo(u.id); } }, unitPath(u).map((x) => x.short || x.name).join(' › ') + ' – ' + u.name),
      h('button', { type: 'button', class: 'icon', title: 'Bearbeiten', onclick: () => unitDialog(u) }, '✏️')))));
  };
  const card = (icon, label, n, total, onclick, warn) => h('button', { type: 'button', class: 'dq-card' + (n ? (warn ? ' warn' : ' todo') : ' ok'), onclick },
    h('span', { class: 'dq-n' }, String(n)), h('span', null, icon, ' ', label), total ? h('span', { class: 'dq-bar' }, h('span', { style: { width: Math.round(100 * (total - n) / total) + '%' } })) : null);
  const complete = emps.filter((p) => p.phone && p.room && p.email && p.amt).length;
  openDialog({
    title: '📊 Datenqualität', wide: true,
    body: h('div', { class: 'admin' },
      h('p', null, h('b', null, `${emps.length} aktive Einträge`), ` – davon ${complete} mit Telefon, Raum, E-Mail und Amt (${emps.length ? Math.round(100 * complete / emps.length) : 0} %).`),
      h('div', { class: 'dq-grid' },
        ...checks.map(([i, l, list]) => card(i, l, list.length, i === '💤' ? 0 : emps.length, () => showPeople(l, list), false)),
        ...unitRows.map(([i, l, list]) => card(i, l, list.length, 0, () => showUnits(l, list), false)),
        card('🚩', 'offene Meldungen', reports.length, 0, () => reportsDialog('open'), false),
        card('⏰', 'Meldungen älter als 14 Tage', oldReports.length, 0, () => reportsDialog('open'), true)),
      detail),
    buttons: ['spacer', { label: 'Schließen', cls: 'primary' }],
  });
}

// ---------- Zwischenablage ----------

async function copyText(text, msg) {
  try { await navigator.clipboard.writeText(text); toast(msg || 'Kopiert', 'ok'); return; } catch { /* Fallback unten */ }
  const ta = h('textarea', { class: 'offscreen' }, text);
  document.body.append(ta);
  ta.select();
  let ok = false;
  try { ok = document.execCommand('copy'); } catch { /* egal */ }
  ta.remove();
  toast(ok ? (msg || 'Kopiert') : 'Kopieren nicht möglich – bitte markieren und Strg+C', ok ? 'ok' : 'err');
}

async function psScriptDialog() {
  let text;
  try {
    const r = await fetch('/api/admin/reports/powershell', { credentials: 'same-origin' });
    if (!r.ok) throw new Error('Skript konnte nicht erzeugt werden');
    text = await r.text();
  } catch (e) { toast(e.message, 'err'); return; }
  const ta = h('textarea', { class: 'ps-script', rows: 18, readonly: true, spellcheck: 'false' }, text);
  const url = URL.createObjectURL(new Blob(['\ufeff' + text], { type: 'text/plain;charset=utf-8' }));
  openDialog({
    title: 'PowerShell: Korrekturen ins AD übernehmen', wide: true,
    body: h('div', { class: 'admin' },
      h('p', { class: 'hint' }, 'Bitte prüfen. Das Skript startet als Probelauf ($WhatIfPreference = $true). Zum Ausführen die Zeile auf $false setzen. Danach den AD-Abgleich starten – übernommene Meldungen werden automatisch erledigt.'),
      ta),
    buttons: [{ label: '⬇ Als .ps1 speichern', onClick: () => { const a = h('a', { href: url, download: 'telefonbuch-korrekturen.ps1' }); document.body.append(a); a.click(); a.remove(); return false; } },
      'spacer', { label: 'Zurück', onClick: () => { reportsDialog('open'); return false; } }, { label: '⧉ Kopieren', cls: 'primary', onClick: () => { copyText(text, 'Skript kopiert'); return false; } }],
    onClose: () => URL.revokeObjectURL(url),
  });
}

// ---------- Massenzuweisung nach Amt-Text ----------

async function orgSuggestDialog() {
  let r;
  try { r = await api('GET', '/api/org/suggest'); } catch (e) { toast(e.message, 'err'); return; }
  const MATCH = { exakt: ['exakt', 'Amt-Text = Name der Einheit'], kuerzel: ['Kürzel', 'Kürzel am Anfang des Amt-Textes'], aehnlich: ['ähnlich', 'Name der Einheit ist im Amt-Text enthalten – bitte prüfen'] };
  const rows = r.items.map((x) => ({ ...x, take: x.default, unit: x.proposed || 0 }));
  const tbody = h('tbody');
  const count = h('b');
  const filterSel = h('select', null, h('option', { value: 'all' }, 'alle Vorschläge'), h('option', { value: 'new' }, 'nur bisher nicht zugeordnete'), h('option', { value: 'change' }, 'nur Umzuordnungen'), h('option', { value: 'unsure' }, 'nur unsichere/mehrdeutige'));
  const visible = (x) => ({ all: true, new: !x.change, change: x.change, unsure: x.match === 'aehnlich' || !x.proposed })[filterSel.value];
  const render = () => {
    count.textContent = String(rows.filter((x) => x.take && x.unit).length);
    tbody.replaceChildren(...rows.filter(visible).map((x) => {
      const opts = [...new Set([x.proposed, ...(x.candidates || [])].filter(Boolean))];
      const sel = h('select', { onchange: (e) => { x.unit = +e.target.value; if (x.unit) x.take = true; render(); } },
        h('option', { value: 0 }, '— bitte wählen —'), opts.map((id) => { const u = S.unitById.get(id); return u ? h('option', { value: id, selected: id === x.unit || null }, unitPath(u).map((p) => p.short || p.name).join(' › ') + (unitPath(u).length > 1 ? ' – ' + u.name : '')) : null; }));
      const cb = h('input', { type: 'checkbox', checked: x.take && x.unit ? true : null, disabled: x.unit ? null : true, onchange: (e) => { x.take = e.target.checked; render(); } });
      const m = MATCH[x.match] || ['?', ''];
      const cur = S.unitById.get(x.current);
      return h('tr', { class: x.take && x.unit ? 'take' : '' },
        h('td', null, h('label', { class: 'check' }, cb, x.take && x.unit ? ' ja' : ' nein')),
        h('td', null, x.name), h('td', null, x.amt),
        h('td', null, sel),
        h('td', null, h('span', { class: 'badge ' + (x.match === 'exakt' ? 'away' : x.match === 'kuerzel' ? 'via' : 'role'), title: m[1] }, x.proposed ? m[0] : 'mehrdeutig')),
        h('td', { class: 'muted' }, cur ? '→ ändert ' + unitLabel(cur) : 'neu'));
    }));
  };
  filterSel.addEventListener('change', render);
  render();
  const auto = h('input', { type: 'checkbox', checked: r.autoAssign || null });
  const setAll = (fn) => { for (const x of rows) if (visible(x) && x.unit) x.take = fn(x); render(); };
  openDialog({
    title: 'Organisation: nach Amt zuordnen', wide: true,
    body: h('div', { class: 'admin' },
      h('p', { class: 'hint' }, 'Der Amt-Text (z. B. aus dem AD, Feld department) wird mit den Namen und Kürzeln der Einheiten verglichen. Vorausgewählt („ja“) sind eindeutige exakte oder Kürzel-Treffer für bisher nicht Zugeordnete. Wer schon in einer passenden Untereinheit steckt, erscheint nicht.'),
      rows.length ? h('div', { class: 'toolbar' }, filterSel,
        h('button', { type: 'button', onclick: () => setAll((x) => x.match !== 'aehnlich') }, 'Sichere auswählen'),
        h('button', { type: 'button', onclick: () => setAll(() => true) }, 'Alle (sichtbaren) ja'),
        h('button', { type: 'button', onclick: () => setAll(() => false) }, 'Alle (sichtbaren) nein'),
        h('span', { class: 'grow' }), h('span', null, 'Übernehmen: ', count)) : null,
      rows.length ? h('div', { class: 'table-wrap tall' }, h('table', { class: 'tbl suggest' }, h('thead', null, h('tr', null, ['Übernehmen', 'Name', 'Amt (Text)', 'Einheit', 'Treffer', 'Aktuell'].map((t) => h('th', null, t)))), tbody))
        : h('p', { class: 'muted' }, 'Keine Vorschläge – alle passenden Personen sind bereits zugeordnet. 🎉'),
      h('label', { class: 'check' }, auto, ' Nach jedem AD-Abgleich automatisch zuordnen (nur eindeutige exakte/Kürzel-Treffer, nur bisher nicht Zugeordnete)')),
    buttons: ['spacer', { label: 'Zurück', onClick: async () => { await saveAuto(); orgManager(); return false; } }, { label: 'Zuordnung übernehmen', cls: 'primary', onClick: async () => {
      const assignments = rows.filter((x) => x.take && x.unit).map((x) => ({ id: x.id, unit: x.unit }));
      try {
        await saveAuto();
        if (assignments.length) { const x = await api('POST', '/api/org/assign', { assignments }); toast(`${x.assigned} Person(en) zugeordnet`, 'ok'); }
      } catch (e) { toast(e.message, 'err'); return false; }
      orgManager();
      return false;
    } }],
  });
  async function saveAuto() {
    if (!!auto.checked === !!r.autoAssign) return;
    try { await api('PUT', '/api/org/auto-assign', { enabled: auto.checked }); r.autoAssign = auto.checked; toast(auto.checked ? 'Automatische Zuordnung nach dem AD-Abgleich: an' : 'Automatische Zuordnung: aus', 'ok'); } catch (e) { toast(e.message, 'err'); }
  }
}

// ---------- Admin: AD-Abgleich ----------

async function syncDialog() {
  let st;
  try { st = await api('GET', '/api/admin/sync'); } catch (e) { toast(e.message, 'err'); return; }
  const status = h('div', { class: 'sync-status' });
  const runs = h('tbody');
  const render = () => {
    status.replaceChildren(
      h('div', { class: 'kv' }, h('span', { class: 'k' }, 'Abgleich'), h('span', null, st.enabled ? `aktiv${st.interval && st.interval !== '0s' ? ' – alle ' + st.interval : ' (nur manuell)'}` : 'nicht aktiviert (adSync.enabled in config.json)')),
      h('div', { class: 'kv' }, h('span', { class: 'k' }, 'Felder aus dem AD'), h('span', null, (st.fields || []).map((f) => FIELD_LABEL[f] || f).join(', '))),
      h('div', { class: 'kv' }, h('span', { class: 'k' }, 'Fehlende Konten'), h('span', null, st.deactivateMissing ? 'werden deaktiviert (Schlagworte bleiben erhalten)' : 'bleiben unverändert')));
    runs.replaceChildren(...(st.runs.length ? st.runs : [null]).map((r) => r ? h('tr', { class: r.status }, h('td', null, fmtDate(r.startedAt)), h('td', null, r.triggerBy), h('td', null, r.status === 'ok' ? '✅' : r.status === 'error' ? '❌' : '⏳'),
      h('td', { class: 'num' }, r.seen), h('td', { class: 'num' }, r.created), h('td', { class: 'num' }, r.updated), h('td', { class: 'num' }, r.deactivated), h('td', { class: 'num' }, r.reactivated), h('td', { class: 'msg' }, r.message || '')) : h('tr', null, h('td', { colspan: 9, class: 'muted' }, 'Noch kein Abgleich gelaufen.'))));
  };
  render();
  const out = h('div', { class: 'muted' });
  openDialog({
    title: 'Active-Directory-Abgleich', wide: true,
    body: h('div', { class: 'admin' }, status, out,
      h('div', { class: 'table-wrap' }, h('table', { class: 'tbl' }, h('thead', null, h('tr', null, ['Zeit', 'Ausgelöst von', '', 'Gelesen', 'Neu', 'Geändert', 'Deaktiviert', 'Reaktiviert', 'Meldung'].map((x) => h('th', null, x)))), runs))),
    buttons: [
      { label: '🔌 Verbindung testen', onClick: async () => {
        out.textContent = 'Teste …';
        try { const r = await api('POST', '/api/admin/ldap-test', {}); out.textContent = r.ok ? `✅ Verbindung und Anmeldung des Dienstkontos OK (${r.ms} ms)` : '❌ ' + r.error; } catch (e) { out.textContent = '❌ ' + e.message; }
        return false;
      } },
      'spacer',
      { label: 'Schließen' },
      { label: '🔄 Jetzt abgleichen', cls: 'primary', onClick: async () => {
        out.textContent = 'Abgleich läuft …';
        try {
          const r = await api('POST', '/api/admin/sync', {});
          out.textContent = r.status === 'ok' ? `✅ Fertig: ${r.seen} gelesen, ${r.created} neu, ${r.updated} geändert, ${r.deactivated} deaktiviert.` : '❌ ' + r.message;
          st = await api('GET', '/api/admin/sync'); render(); reload();
        } catch (e) { out.textContent = '❌ ' + e.message; }
        return false;
      } },
    ],
  });
}

// ---------- Admin: Protokoll ----------

// ---------- Admin: Datenhaltung (MySQL / JSON-Datei) ----------

async function storageDialog() {
  let st;
  try { st = await api('GET', '/api/admin/storage'); } catch (e) { toast(e.message, 'err'); return; }
  const box = h('div', { class: 'sync-status' });
  const out = h('div', { class: 'muted' });
  const kb = (n) => (n > 1048576 ? (n / 1048576).toFixed(1) + ' MB' : Math.max(1, Math.round(n / 1024)) + ' KB');
  const IMPORT = { off: 'aus', manual: 'manuell (Knopf / Befehl)', startup: 'bei jedem Start' };
  const row = (k, v) => (v ? h('div', { class: 'kv' }, h('span', { class: 'k' }, k), h('span', null, v)) : null);
  const render = () => {
    const j = st.json || {};
    const json = st.mode === 'json' && !st.offline;
    box.replaceChildren(...[
      row('Modus', st.offline ? '⚠️ Rückfall-Betrieb: MySQL nicht erreichbar – JSON-Kopie wird nur gelesen' : json ? '📄 JSON-Datei (ohne Datenbank, offline-fähig)' : '🗄 MySQL-Datenbank'),
      row(json ? 'Datei' : 'JSON-Kopie', st.jsonPath + (json && j.size ? ` (${kb(j.size)}, ${j.employees} Einträge)` : '')),
      json ? row('Protokolldatei', (j.auditPath || '') + (j.auditSize ? ` (${kb(j.auditSize)}, ${j.auditEntries} Einträge)` : '')) : null,
      json ? row('Zuletzt gespeichert', j.savedAt && !j.savedAt.startsWith('0001') ? fmtDate(j.savedAt) : '—') : null,
      json ? row('Aus MySQL übernommen', j.importedAt ? fmtDate(j.importedAt) : 'noch nie') : null,
      json ? row('Übernahme aus MySQL', (IMPORT[st.mysqlImport] || st.mysqlImport) + (st.mysqlImportInterval && st.mysqlImportInterval !== '0s' ? ', zusätzlich alle ' + st.mysqlImportInterval : '')) : null,
      json && st.lastImport ? row('Letzter Versuch', `${fmtDate(st.lastImport)} – ${st.lastImportOK ? '✅' : '❌'} ${st.lastImportMsg}`) : null,
      !json ? row('Automatische Kopie', st.snapshotInterval && st.snapshotInterval !== '0s' ? 'alle ' + st.snapshotInterval : 'aus (storage.jsonSnapshotInterval)') : null,
      !json && st.lastSnapshot ? row('Letzte Kopie', `${fmtDate(st.lastSnapshot)} – ${st.lastSnapshotMsg}`) : null,
    ].filter(Boolean));
  };
  render();
  const buttons = ['spacer', { label: 'Schließen' }];
  if (st.mode === 'json' && !st.offline && st.mysqlImport !== 'off') {
    buttons.push({ label: '⬇ Aus MySQL übernehmen', cls: 'primary', onClick: async () => {
      if (!(await confirmDialog('Aus MySQL übernehmen', 'Alle Daten der JSON-Datei (Einträge, Schlagworte, Avatare, private Kontakte, lokale Konten) werden durch den Stand der MySQL-Datenbank ersetzt. Die bisherige Datei wird als .bak gesichert. Fortfahren?', 'Übernehmen'))) { storageDialog(); return false; }
      openDialog({ title: 'Datenhaltung', body: h('p', null, '⏳ Übernahme läuft …') });
      try {
        let r = await api('POST', '/api/admin/storage/import-mysql', { force: false });
        if (!r.ok && r.canForce && await confirmDialog('Deutlich weniger Daten in MySQL', r.error + ' Trotzdem übernehmen?', 'Trotzdem übernehmen', true)) {
          r = await api('POST', '/api/admin/storage/import-mysql', { force: true });
        }
        toast(r.ok ? `Übernommen: ${r.employees} Einträge, ${r.tags} Schlagworte` : r.error, r.ok ? 'ok' : 'err');
        if (r.ok) await reload(false);
      } catch (e) { toast(e.message, 'err'); }
      storageDialog();
      return false;
    } });
  }
  if (st.mode === 'mysql') {
    buttons.unshift({ label: '💾 JSON-Kopie jetzt schreiben', onClick: async () => {
      out.textContent = 'Schreibe …';
      try {
        let r = await api('POST', '/api/admin/storage/snapshot', { force: false });
        if (!r.ok && r.canForce && await confirmDialog('Deutlich weniger Daten in MySQL', r.error + ' Trotzdem überschreiben?', 'Überschreiben', true)) {
          r = await api('POST', '/api/admin/storage/snapshot', { force: true });
          toast(r.ok ? 'JSON-Kopie geschrieben' : r.error, r.ok ? 'ok' : 'err');
          storageDialog();
          return false;
        }
        out.textContent = r.ok ? '✅ Kopie geschrieben: ' + r.path : '❌ ' + r.error;
        st = await api('GET', '/api/admin/storage'); render();
      } catch (e) { out.textContent = '❌ ' + e.message; }
      return false;
    } });
  }
  openDialog({
    title: 'Datenhaltung', wide: false,
    body: h('div', { class: 'admin' }, box, out,
      h('p', { class: 'hint' }, st.mode === 'json'
        ? 'Im JSON-Modus liegen alle Daten in einer Datei – ideal als Offline-Betrieb. Eine Übernahme aus MySQL ersetzt den Inhalt vollständig; Änderungen, die nur in der JSON-Datei gemacht wurden, gehen dabei verloren (Sicherung als .bak).'
        : 'Die JSON-Kopie enthält alle Daten und kann jederzeit für den Offline-Betrieb (storage.mode = json) verwendet werden.')),
    buttons,
  });
}

async function auditDialog() {
  const q = h('input', { type: 'search', placeholder: 'Suchen (Benutzer, Aktion, Eintrag …)' });
  const tbody = h('tbody');
  const ACT = { mysql_import: 'Übernahme aus MySQL', login: 'Anmeldung', login_failed: 'Anmeldung fehlgeschlagen', tags_update: 'Schlagworte geändert', avatar_update: 'Avatar geändert', employee_create: 'Eintrag angelegt', employee_update: 'Stammdaten geändert', employee_delete: 'Eintrag gelöscht', tag_create: 'Schlagwort angelegt', tag_update: 'Schlagwort geändert', tag_merge: 'Schlagworte zusammengeführt', tag_delete: 'Schlagwort gelöscht', tag_cleanup: 'Unbenutzte Schlagworte gelöscht', ad_sync: 'AD-Abgleich', category_create: 'Kategorie angelegt', category_update: 'Kategorie geändert', category_delete: 'Kategorie gelöscht', storage_online: 'Datenbank wieder erreichbar', deputies_update: 'Vertretung geändert', absences_update: 'Abwesenheit geändert', report_create: 'Fehler gemeldet', report_done: 'Meldung erledigt', report_rejected: 'Meldung abgelehnt', report_open: 'Meldung wieder geöffnet', org_levels: 'Ebenen geändert', org_unit_create: 'Einheit angelegt', org_unit_update: 'Einheit geändert', org_unit_delete: 'Einheit gelöscht', org_members: 'Einheit: Zuordnung', org_assign: 'Organisation zugeordnet', channel_create: 'Kontaktweg angelegt', channel_update: 'Kontaktweg geändert', channel_delete: 'Kontaktweg gelöscht', channel_members: 'Kontaktweg zugewiesen', feed_token: 'Favoriten-Code', fun_settings: 'Extras geändert', org_auto_assign: 'Automatische Zuordnung', reports_powershell: 'PowerShell-Skript erzeugt', reports_autoclose: 'Meldungen automatisch erledigt' };
  const load = async () => {
    try {
      const r = await api('GET', '/api/admin/audit?limit=500&q=' + encodeURIComponent(q.value));
      tbody.replaceChildren(...r.entries.map((e) => h('tr', { class: e.action === 'login_failed' ? 'warn' : '' }, h('td', null, fmtDate(e.ts)), h('td', null, e.actor), h('td', null, ACT[e.action] || e.action), h('td', null, e.target), h('td', { class: 'msg' }, e.detail), h('td', { class: 'muted' }, e.ip))));
    } catch (e) { toast(e.message, 'err'); }
  };
  let t;
  q.addEventListener('input', () => { clearTimeout(t); t = setTimeout(load, 250); });
  await load();
  openDialog({
    title: 'Protokoll', wide: true,
    body: h('div', { class: 'admin' }, h('div', { class: 'toolbar' }, q), h('div', { class: 'table-wrap tall' }, h('table', { class: 'tbl' }, h('thead', null, h('tr', null, ['Zeit', 'Benutzer', 'Aktion', 'Eintrag', 'Details', 'IP'].map((x) => h('th', null, x)))), tbody))),
    buttons: ['spacer', { label: 'Schließen' }],
  });
}

boot();
