// Telefonbuch – Hauptansicht: links Personen, rechts Schlagworte.
import { h, $, $$, api, setCSRF, toast, norm, normAlt, digits, tokens, collator, telHref, fmtDate } from './util.js';
import { openDialog, confirmDialog, field } from './dialog.js';
import { openAvatarEditor } from './editor.js';
import * as A from './avatar.js';
import { personPicker, amtPicker } from './picker.js';

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

function setMe(me) { S.me = me || guestMe(); setCSRF(S.me.csrf); }

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
  return A.thumbURL(p.avatar || A.DEFAULT_AVATAR, size);
}

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
    if (p.self) badges.push(h('span', { class: 'badge self' }, 'ich'));
    if (p.kind === 'emp' && (!p.active || p.hidden)) badges.push(h('span', { class: 'badge off' }, !p.active ? 'inaktiv' : 'ausgeblendet'));
    const row = h('div', { class: 'row', role: 'option', tabindex: '-1', dataset: { key: p.key } },
      h('div', { class: 'c-name' }, img, h('span', { class: 'nm' }, h('b', null, p.lastName), p.firstName ? ', ' + p.firstName : ''), ...badges),
      cell('c-phone', '☎', null, p.phone, tel(p.phone)),
      cell('c-mobile', '📱', null, p.mobile, tel(p.mobile)),
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
  const sorted = [...S.people].sort((a, b) => dir * collator.compare(sortVal(a, key), sortVal(b, key)) || collator.compare(a.display, b.display));
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
  const av = p.kind !== 'emp' ? null : (p.avatar || A.DEFAULT_AVATAR);
  if (av) {
    const t0 = performance.now();
    const anim = () => { if (!document.body.contains(cv)) return; A.drawHead(cv, av, { yaw: -28 + Math.sin((performance.now() - t0) / 1500) * 22, pitch: 16, fit: true }); cardAnim = requestAnimationFrame(anim); };
    requestAnimationFrame(anim);
    if (p.self || S.me.isAdmin) cv.addEventListener('click', () => editAvatar(p));
  }
  const line = (icon, label, val, href) => val ? h('div', { class: 'kv' }, h('span', { class: 'k' }, icon + ' ' + label), href ? h('a', { href }, val) : h('span', null, val)) : null;
  const btns = [];
  if (canEditTags(p)) btns.push(h('button', { type: 'button', class: 'primary', onclick: () => editTags(p) }, p.self ? '🏷 Meine Schlagworte' : '🏷 Schlagworte'));
  if (p.kind === 'emp' && (p.self || S.me.isAdmin)) btns.push(h('button', { type: 'button', onclick: () => editAvatar(p) }, '🙂 Avatar'));
  if (canEditPerson(p)) btns.push(h('button', { type: 'button', onclick: () => editPerson(p) }, p.kind === 'priv' ? '✏️ Bearbeiten' : '✏️ Stammdaten'));
  const head = h('div', { class: 'card-head' }, av ? cv : h('div', { class: 'card-av priv' }, p.kind === 'shared' || p.sharedOut ? '🔗' : '🔒'),
    h('div', null,
      h('h3', null, p.lastName, p.firstName ? ', ' + p.firstName : ''),
      p.amt ? h('div', { class: 'muted' }, p.amt) : null,
      h('div', { class: 'badges' },
        ...privBadges(p, false),
        p.source === 'ad' ? h('span', { class: 'badge ad', title: 'Stammdaten kommen aus dem Active Directory' }, 'AD') : null,
        p.self ? h('span', { class: 'badge self' }, 'das bist du') : null)));
  card.replaceChildren(h('div', { class: 'card' }, head,
    h('div', { class: 'kvs' },
      line('☎', 'Telefon', p.phone, telHref(p.phone)),
      line('📱', 'Mobil', p.mobile, telHref(p.mobile)),
      line('🚪', 'Raum', p.room),
      line('✉', 'E-Mail', p.email, p.email ? 'mailto:' + p.email : null),
      p.note ? h('div', { class: 'kv note' }, h('span', { class: 'k' }, '📝 Notiz'), h('span', null, p.note)) : null,
      p.sharedOut ? h('div', { class: 'kv note' }, h('span', { class: 'k' }, '🔗 Geteilt mit'), h('span', null, shareSummary(p.shares))) : null),
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
  ];
  if (S.me.isAdmin) {
    items.push(h('div', { class: 'menu-sep' }, 'Verwaltung'),
      item('👤 Neuen Eintrag anlegen', () => editPerson(null, 'emp')),
      item('🏷 Schlagworte verwalten', tagManager),
      item('🗂 Kategorien verwalten', categoryManager),
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
  openDialog({ title: 'Hilfe', body, buttons: ['spacer', { label: 'Schließen', cls: 'primary' }] });
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
  const has = (name) => cur.some((t) => norm(t.name) === norm(name));
  const known = (name) => S.tags.find((t) => norm(t.name) === norm(name));
  const add = (name, cat) => {
    name = name.trim().replace(/\s+/g, ' ');
    if (!name || has(name)) return;
    if (cur.length >= S.me.features.maxTags) { toast(`Höchstens ${S.me.features.maxTags} Schlagworte`, 'err'); return; }
    const k = known(name);
    if (!k && !S.me.features.userCreatesTags && !S.me.isAdmin) { toast('Neue Schlagworte können nur Administratoren anlegen', 'err'); return; }
    cur.push(k ? { name: k.name, category: k.category } : { name, category: cat || newCat });
    input.value = '';
    render();
  };
  const render = () => {
    chipsBox.replaceChildren(...(cur.length ? cur.map((t, i) => h('span', { class: 'chip', style: { borderColor: catColor(t.category) } }, t.name, h('button', { type: 'button', class: 'x', title: 'Entfernen', 'aria-label': 'Entfernen', onclick: () => { cur.splice(i, 1); render(); } }, '✕'))) : [h('span', { class: 'muted' }, 'Noch keine Schlagworte.')]));
    renderSugg();
  };
  const renderSugg = () => {
    const q = norm(input.value.trim());
    const list = S.tags.filter((t) => !has(t.name) && (!q || norm(t.name).includes(q) || normAlt(t.name).includes(q))).sort((a, b) => b.count - a.count).slice(0, q ? 12 : 18);
    const items = list.map((t) => h('button', { type: 'button', class: 'sugg-item', onclick: () => add(t.name) }, dot(t.category), t.name, h('small', null, String(t.count))));
    const exact = q && S.tags.some((t) => norm(t.name) === q);
    if (q && !exact && !has(input.value) && (S.me.features.userCreatesTags || S.me.isAdmin)) items.unshift(h('button', { type: 'button', class: 'sugg-item new', onclick: () => add(input.value) }, '＋ Neu: „', input.value.trim(), '“ als ', catName(newCat)));
    sugg.replaceChildren(...items);
    sugg.hidden = !items.length;
  };
  input.addEventListener('input', renderSugg);
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ',' || e.key === ';') { e.preventDefault(); if (input.value.trim()) add(input.value); }
    if (e.key === 'Backspace' && !input.value && cur.length) { cur.pop(); render(); }
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
  let note, user, hidden;
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
    else { data.username = user.value; data.hidden = hidden.checked; }
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
  const ACT = { mysql_import: 'Übernahme aus MySQL', login: 'Anmeldung', login_failed: 'Anmeldung fehlgeschlagen', tags_update: 'Schlagworte geändert', avatar_update: 'Avatar geändert', employee_create: 'Eintrag angelegt', employee_update: 'Stammdaten geändert', employee_delete: 'Eintrag gelöscht', tag_create: 'Schlagwort angelegt', tag_update: 'Schlagwort geändert', tag_merge: 'Schlagworte zusammengeführt', tag_delete: 'Schlagwort gelöscht', tag_cleanup: 'Unbenutzte Schlagworte gelöscht', ad_sync: 'AD-Abgleich', category_create: 'Kategorie angelegt', category_update: 'Kategorie geändert', category_delete: 'Kategorie gelöscht', storage_online: 'Datenbank wieder erreichbar' };
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
