// Zuständigkeitsfinder („Wer ist zuständig?“) und Sprechzeiten.
// Aufgaben mit Namenstrennung nach Anfangsbuchstaben des Nachnamens (A–K, Sch–Sz …),
// je Bereich mehrere Zuständige und Vertretungen. Sprechzeiten je Einheit oder je Aufgabe.
import { norm, normAlt, tokens, collator, telHref } from './util.js';

let C = null; // Kontext aus app.js (Zustand + Hilfsfunktionen)
export function setup(ctx) { C = ctx; }

const DAYS = ['Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa', 'So'];
const DAYS_LONG = ['Montag', 'Dienstag', 'Mittwoch', 'Donnerstag', 'Freitag', 'Samstag', 'Sonntag'];

// ---------- Namenstrennung ----------

// wie foldName im Server: DIN 5007-1 (Ä=A, Ö=O, Ü=U, ß=SS), Akzente weg, nur A–Z
export function foldName(s) {
  return String(s || '')
    .replace(/ß/g, 'ss').replace(/ẞ/g, 'SS').replace(/[Ææ]/g, 'AE').replace(/[Œœ]/g, 'OE')
    .replace(/[Øø]/g, 'O').replace(/[Łł]/g, 'L').replace(/[Đđ]/g, 'D').replace(/[Ħħ]/g, 'H').replace(/[Ŧŧ]/g, 'T')
    .normalize('NFD').replace(/[̀-ͯ]/g, '').toUpperCase().replace(/[^A-Z]/g, '');
}
const padA = (s) => (s + 'AAAA').slice(0, 4);
const padZ = (s) => (s + 'ZZZZ').slice(0, 4);
const bounds = (r) => [padA(foldName(r.from)), padZ(foldName(r.to))];
const title = (s) => (s ? s[0] + s.slice(1).toLowerCase() : '');

export function rangeLabel(r, others) {
  if (!r.from) return others ? 'alle übrigen Namen' : 'alle Namen';
  return r.from === r.to ? r.from : `${r.from}–${r.to}`;
}

export function matchRule(rules, last) {
  const k = foldName(last);
  if (!k) return -1;
  const key = padA(k);
  let fallback = -1;
  for (let i = 0; i < rules.length; i++) {
    const r = rules[i];
    if (!r.from) { fallback = i; continue; }
    const [lo, hi] = bounds(r);
    if (lo <= key && key <= hi) return i;
  }
  return fallback;
}

function step(s, dir) { // nächster (+1) bzw. vorheriger (−1) 4-Buchstaben-Schlüssel, null am Rand
  const a = s.split('');
  const edge = dir > 0 ? 'Z' : 'A', reset = dir > 0 ? 'A' : 'Z';
  for (let i = 3; i >= 0; i--) {
    if (a[i] !== edge) {
      a[i] = String.fromCharCode(a[i].charCodeAt(0) + dir);
      for (let j = i + 1; j < 4; j++) a[j] = reset;
      return a.join('');
    }
  }
  return null;
}
function keyLabel(lo, hi) {
  const l = title(lo.replace(/A+$/, '') || 'A'), r = title(hi.replace(/Z+$/, '') || 'Z');
  return l === r ? l : `${l}–${r}`;
}

// Abdeckung A–Z: Lücken und Überschneidungen der Buchstabenbereiche
export function coverage(rules) {
  const rs = rules.filter((r) => r.from).map((r) => ({ r, b: bounds(r) })).sort((x, y) => (x.b[0] < y.b[0] ? -1 : x.b[0] > y.b[0] ? 1 : 0));
  const gaps = [], overlaps = [];
  let end = null, cur = 'AAAA', prev = null;
  for (const x of rs) {
    const [lo, hi] = x.b;
    if (prev && end && lo <= end) overlaps.push(`${rangeLabel(prev.r)} / ${rangeLabel(x.r)}`);
    else if (cur && lo > cur) gaps.push(keyLabel(cur, step(lo, -1)));
    if (!end || hi > end) { end = hi; cur = step(hi, 1); }
    prev = x;
  }
  if (rs.length && cur) gaps.push(keyLabel(cur, 'ZZZZ'));
  return { gaps, overlaps, hasAll: rules.some((r) => !r.from), ranged: rs.length };
}

// Vorschlag für den nächsten Bereich (hinter dem letzten)
function nextFrom(rules) {
  let end = null;
  for (const r of rules) if (r.from) { const hi = bounds(r)[1]; if (!end || hi > end) end = hi; }
  const n = end ? step(end, 1) : 'AAAA';
  return n ? title(n.replace(/A+$/, '') || 'A') : '';
}

// ---------- Sprechzeiten ----------

export function hoursFor(unitId) {
  for (let u = C.S.unitById.get(unitId), n = 0; u && n < 20; u = C.S.unitById.get(u.parent), n++) {
    if (u.hours) return { hours: u.hours, unit: u };
  }
  return null;
}
export function taskHours(t) { return t.hours ? { hours: t.hours, unit: null, own: true } : hoursFor(t.orgUnit); }

function fmtDays(days) {
  const d = [...days].sort((a, b) => a - b);
  const out = [];
  for (let i = 0; i < d.length;) {
    let j = i;
    while (j + 1 < d.length && d[j + 1] === d[j] + 1) j++;
    out.push(j - i >= 2 ? `${DAYS[d[i] - 1]}–${DAYS[d[j] - 1]}` : d.slice(i, j + 1).map((x) => DAYS[x - 1]).join(', '));
    i = j + 1;
  }
  return out.join(', ');
}

// Zeilen gruppiert nach Uhrzeit: „Mo, Di, Do 08:00–12:00“
export function hoursLines(hrs) {
  const slots = [...(hrs.slots || [])].sort((a, b) => a.days[0] - b.days[0] || (a.from < b.from ? -1 : 1));
  return slots.map((s) => `${fmtDays(s.days)} ${s.from}–${s.to}`);
}

export function openState(hrs, now = new Date()) {
  const slots = hrs.slots || [];
  if (!slots.length) return null;
  const pad = (n) => String(n).padStart(2, '0');
  const t = `${pad(now.getHours())}:${pad(now.getMinutes())}`;
  const today = ((now.getDay() + 6) % 7) + 1;
  for (const s of slots) if (s.days.includes(today) && s.from <= t && t < s.to) return { open: true, text: `jetzt geöffnet · bis ${s.to}` };
  for (let k = 0; k < 8; k++) {
    const day = ((today - 1 + k) % 7) + 1;
    const c = slots.filter((s) => s.days.includes(day) && (k > 0 || s.from > t)).map((s) => s.from).sort();
    if (c.length) return { open: false, text: `geschlossen · öffnet ${k === 0 ? 'heute' : k === 1 ? 'morgen' : DAYS_LONG[day - 1]} ${c[0]}` };
  }
  return { open: false, text: 'geschlossen' };
}

export function hoursBox(info, { showUnit = true, compact = false } = {}) {
  const { h } = C;
  const st = openState(info.hours);
  const lines = hoursLines(info.hours);
  return h('div', { class: 'hours' + (compact ? ' compact' : '') },
    h('div', { class: 'hours-head' }, compact ? null : h('b', null, '🕘 Sprechzeiten'),
      showUnit && info.unit ? h('span', { class: 'muted' }, compact ? C.unitLabel(info.unit) : ` · ${C.unitLabel(info.unit)}`) : null,
      st ? h('span', { class: 'badge ' + (st.open ? 'open' : 'closed') }, st.text) : null),
    lines.length ? h('div', { class: 'hours-lines' }, lines.map((l) => h('span', null, l))) : null,
    info.hours.note ? h('div', { class: 'muted small' }, info.hours.note) : null);
}

// Editor für Sprechzeiten → { el, value() } (value: null = keine)
export function hoursEditor(hours) {
  const { h } = C;
  const rows = (hours && hours.slots ? hours.slots : []).map((s) => ({ days: new Set(s.days), from: s.from, to: s.to }));
  const note = h('input', { type: 'text', maxlength: 200, value: (hours && hours.note) || '', placeholder: 'z. B. „Termine auch nach Vereinbarung“' });
  const list = h('div', { class: 'hrs-rows' });
  const render = () => {
    list.replaceChildren(...rows.map((r, i) => h('div', { class: 'hrs-row' },
      h('div', { class: 'hrs-days', role: 'group', 'aria-label': 'Wochentage' }, DAYS.map((d, k) => h('button', {
        type: 'button', class: 'day' + (r.days.has(k + 1) ? ' on' : ''), 'aria-pressed': r.days.has(k + 1) ? 'true' : 'false', title: DAYS_LONG[k],
        onclick: (e) => { if (r.days.has(k + 1)) r.days.delete(k + 1); else r.days.add(k + 1); e.currentTarget.classList.toggle('on'); e.currentTarget.setAttribute('aria-pressed', r.days.has(k + 1) ? 'true' : 'false'); },
      }, d))),
      h('input', { type: 'time', value: r.from, step: 300, 'aria-label': 'von', oninput: (e) => { r.from = e.target.value; } }),
      h('span', { class: 'muted' }, 'bis'),
      h('input', { type: 'time', value: r.to, step: 300, 'aria-label': 'bis', oninput: (e) => { r.to = e.target.value; } }),
      h('button', { type: 'button', class: 'icon danger', title: 'Zeile entfernen', onclick: () => { rows.splice(i, 1); render(); } }, '🗑'))));
    if (!rows.length) list.append(h('p', { class: 'muted small' }, 'Keine festen Zeiten.'));
  };
  render();
  const add = (days, from, to) => { rows.push({ days: new Set(days), from, to }); render(); };
  const el = h('div', { class: 'hrs-edit' }, list,
    h('div', { class: 'toolbar' },
      h('button', { type: 'button', onclick: () => add([], '08:00', '12:00') }, '＋ Zeile'),
      h('button', { type: 'button', class: 'linkish', onclick: () => add([1, 2, 3, 4, 5], '08:00', '12:00') }, 'Mo–Fr vormittags'),
      h('button', { type: 'button', class: 'linkish', onclick: () => add([4], '14:00', '18:00') }, 'Do nachmittags')),
    C.field('Hinweis', note));
  return {
    el,
    value() {
      const slots = rows.filter((r) => r.days.size && r.from && r.to).map((r) => ({ days: [...r.days].sort(), from: r.from, to: r.to }));
      return slots.length || note.value.trim() ? { slots, note: note.value.trim() } : null;
    },
  };
}

// ---------- Ansicht „Zuständig“ ----------

let ui = null; // { q, name, results }

export function openTask(query, name) {
  C.setView('tasks');
  ensureUI();
  ui.q.value = query || '';
  if (name !== undefined) ui.name.value = name;
  C.update();
  if (C.isMobile()) C.setTab('people');
}

function ensureUI() {
  if (ui) return;
  const { h, $ } = C;
  const q = h('input', { type: 'search', placeholder: '🔍 Anliegen, z. B. Wohngeld, Bauantrag …', 'aria-label': 'Anliegen suchen', autocomplete: 'off' });
  const name = h('input', { type: 'search', placeholder: 'Nachname der Antragsteller:in', 'aria-label': 'Nachname für die Namenstrennung', autocomplete: 'off' });
  const results = h('div', { class: 'task-results' });
  const tools = h('div', { class: 'task-tools' });
  q.addEventListener('input', () => renderResults());
  name.addEventListener('input', () => renderResults());
  $('#taskview').replaceChildren(h('div', { class: 'task-search' }, q, name, tools), results);
  ui = { q, name, results, tools };
}

export function renderTasks() {
  ensureUI();
  const { h } = C;
  ui.tools.replaceChildren(...(C.S.me.isAdmin ? [
    h('button', { type: 'button', class: 'small-btn', title: 'Neue Aufgabe anlegen', onclick: () => taskDialog(null) }, '➕ Aufgabe'),
    h('button', { type: 'button', class: 'small-btn', title: 'Alle Aufgaben und Lücken in der Namenstrennung', onclick: () => taskManager() }, '⚙'),
  ] : []));
  renderResults();
}

function taskIndex(t) {
  const u = C.S.unitById.get(t.orgUnit);
  const people = t.rules.flatMap((r) => [...r.leads || [], ...r.deputies || []]).map(C.empByID).filter(Boolean).map((p) => p.lastName + ' ' + p.firstName);
  const main = [t.name, t.keywords, u ? C.unitLabel(u) : ''].join(' ');
  return { main: norm(main) + ' ' + normAlt(main), all: norm([main, t.description, ...people].join(' ')) + ' ' + normAlt(people.join(' ')) };
}

function renderResults() {
  const { h } = C;
  const tasks = C.S.tasks || [];
  const q = tokens(ui.q.value);
  const nm = ui.name.value.trim();
  let hits = tasks.map((t) => ({ t, ix: taskIndex(t) })).filter((x) => q.every((w) => x.ix.all.includes(w)));
  hits.sort((a, b) => (q.length ? (q.every((w) => b.ix.main.includes(w)) ? 1 : 0) - (q.every((w) => a.ix.main.includes(w)) ? 1 : 0) : 0) || (a.t.sort - b.t.sort) || collator.compare(a.t.name, b.t.name));
  const parts = [];
  if (!tasks.length) {
    parts.push(h('p', { class: 'muted pad' }, C.S.me.isAdmin ? 'Noch keine Aufgaben hinterlegt. Mit „➕ Aufgabe“ legst du z. B. „Wohngeld“ mit Namenstrennung A–K / L–Z an.' : 'Noch keine Zuständigkeiten hinterlegt.'));
  } else if (!hits.length) {
    parts.push(h('p', { class: 'muted pad' }, 'Keine passende Aufgabe gefunden.'));
  } else {
    if (nm && !foldName(nm)) parts.push(h('p', { class: 'hint' }, 'Für die Namenstrennung bitte einen Nachnamen mit Buchstaben eingeben.'));
    for (const { t } of hits.slice(0, 60)) parts.push(taskCard(t, nm));
    if (hits.length > 60) parts.push(h('p', { class: 'muted pad' }, `… und ${hits.length - 60} weitere – bitte genauer suchen.`));
  }
  ui.results.replaceChildren(...parts);
  const foot = C.$('#people-foot');
  if (foot && C.S.view === 'tasks') foot.textContent = hits.length === tasks.length ? `${tasks.length} Aufgaben` : `${hits.length} von ${tasks.length} Aufgaben`;
}

function personChip(p, deputy) {
  const { h } = C;
  const c = C.personContact(p);
  const phone = c && c.contact.hideDirect && c.contact.phone ? c.contact.phone : p.phone;
  return h('span', { class: 'tperson' + (deputy ? ' dep' : '') + (C.absNow(p) ? ' away' : '') }, C.personLink(p),
    phone ? h('a', { href: telHref(phone), class: 'tphone', title: c && c.contact.hideDirect ? 'Bitte bevorzugt: ' + C.contactSummary(c.contact) : 'Anrufen' }, '☎ ' + phone) : null);
}

function ruleBody(r) {
  const { h } = C;
  const leads = (r.leads || []).map(C.empByID).filter(Boolean);
  const deps = (r.deputies || []).map(C.empByID).filter(Boolean);
  const allAway = leads.length > 0 && leads.every((p) => C.absNow(p));
  return h('div', { class: 'rule-body' + (allAway ? ' lead-away' : '') },
    h('div', { class: 'rule-line' }, h('span', { class: 'k' }, '★ Zuständig'), leads.length ? h('span', { class: 'chips' }, leads.map((p) => personChip(p))) : h('span', { class: 'muted' }, 'derzeit unbesetzt')),
    allAway ? h('div', { class: 'rule-warn' }, '🏖 Zuständig ist gerade niemand erreichbar – bitte an die Vertretung wenden.') : null,
    deps.length ? h('div', { class: 'rule-line' }, h('span', { class: 'k' }, '☆ Vertretung'), h('span', { class: 'chips' }, deps.map((p) => personChip(p, true)))) : null,
    r.note ? h('div', { class: 'muted small' }, r.note) : null);
}

function taskCard(t, nm) {
  const { h } = C;
  const u = C.S.unitById.get(t.orgUnit);
  const hi = taskHours(t);
  const others = t.rules.some((r) => r.from);
  const parts = [];
  if (nm && foldName(nm) && t.rules.length > 1) {
    const i = matchRule(t.rules, nm);
    if (i >= 0) {
      const r = t.rules[i];
      parts.push(h('div', { class: 'task-hit' }, h('div', { class: 'hit-label' }, `Für „${nm}“: `, h('b', null, rangeLabel(r, others))), ruleBody(r)));
    } else {
      parts.push(h('div', { class: 'rule-warn' }, `Für „${nm}“ ist kein Bereich hinterlegt – bitte an ${u ? C.unitLabel(u) : 'die zuständige Stelle'} wenden.`));
    }
    parts.push(h('details', { class: 'task-all' }, h('summary', null, `Alle ${t.rules.length} Bereiche`), rulesTable(t, i)));
  } else if (t.rules.length === 1) {
    parts.push(ruleBody(t.rules[0]));
  } else if (t.rules.length) {
    parts.push(rulesTable(t, -1));
  }
  let warn = null;
  if (C.S.me.isAdmin) {
    const cv = coverage(t.rules);
    if (!cv.hasAll && cv.gaps.length) warn = h('div', { class: 'rule-warn' }, '⚠ Nicht abgedeckt: ' + cv.gaps.join(', '));
    const empty = t.rules.filter((r) => !(r.leads || []).some((id) => C.empByID(id)));
    if (empty.length) warn = h('div', null, warn, h('div', { class: 'rule-warn' }, `⚠ Unbesetzt: ${empty.map((r) => rangeLabel(r, others)).join(', ')}`));
  }
  return h('article', { class: 'task-card' },
    h('div', { class: 'task-head' },
      h('div', null, h('h3', null, t.name),
        u ? h('button', { type: 'button', class: 'linkish unit-link', title: 'Im Organigramm zeigen', onclick: () => C.openUnit(u.id) }, '🏢 ' + C.unitLabel(u)) : null),
      C.S.me.isAdmin ? h('button', { type: 'button', class: 'icon', title: 'Aufgabe bearbeiten', onclick: () => taskDialog(t) }, '✏️') : null),
    t.description ? h('p', { class: 'task-desc' }, t.description) : null,
    hi ? hoursBox(hi, { showUnit: !hi.own && hi.unit && hi.unit.id !== t.orgUnit }) : null,
    ...parts, warn);
}

function rulesTable(t, hit) {
  const { h } = C;
  const others = t.rules.some((r) => r.from);
  const list = t.rules.map((r, i) => ({ r, i })).sort((a, b) => (!a.r.from) - (!b.r.from) || (bounds(a.r)[0] < bounds(b.r)[0] ? -1 : 1));
  return h('div', { class: 'rules-table' }, list.map(({ r, i }) => h('div', { class: 'rules-row' + (i === hit ? ' hit' : '') },
    h('span', { class: 'range' }, rangeLabel(r, others)), ruleBody(r))));
}

// ---------- Visitenkarte & Organigramm ----------

export function cardLines(p) {
  const { h } = C;
  const rows = [];
  const tasks = C.S.tasks || [];
  const kv = (k, ...v) => h('div', { class: 'kv' }, h('span', { class: 'k' }, k), h('span', { class: 'kv-v' }, ...v));
  const link = (t, r) => h('button', { type: 'button', class: 'linkish', title: 'Im Zuständigkeitsfinder zeigen', onclick: () => openTask(t.name, '') }, t.name + (t.rules.length > 1 || r.from ? ` (${rangeLabel(r, t.rules.some((x) => x.from))})` : ''));
  const lead = [], dep = [];
  for (const t of tasks) for (const r of t.rules) {
    if ((r.leads || []).includes(p.id)) lead.push(link(t, r));
    else if ((r.deputies || []).includes(p.id)) dep.push(link(t, r));
  }
  const join = (l) => l.flatMap((x, i) => (i ? [', ', x] : [x]));
  if (lead.length) rows.push(kv('🧭 Zuständig', ...join(lead.slice(0, 8)), lead.length > 8 ? ` +${lead.length - 8}` : ''));
  if (dep.length) rows.push(kv('🧭 Vertretung', ...join(dep.slice(0, 8)), dep.length > 8 ? ` +${dep.length - 8}` : ''));
  const hi = hoursFor(p.orgUnit);
  if (hi) rows.push(h('div', { class: 'kv' }, h('span', { class: 'k' }, '🕘 Sprechz.'), h('span', { class: 'kv-v' }, hoursBox(hi, { compact: true, showUnit: hi.unit.id !== p.orgUnit }))));
  return rows;
}

export function unitExtras(u) {
  const { h } = C;
  const out = [];
  const hi = hoursFor(u.id);
  if (hi) out.push(hoursBox(hi, { showUnit: hi.unit.id !== u.id }));
  const tasks = (C.S.tasks || []).filter((t) => t.orgUnit === u.id);
  if (tasks.length) {
    out.push(h('div', { class: 'org-tasks' }, h('span', { class: 'k' }, '🧭 Aufgaben'),
      ...tasks.flatMap((t, i) => [i ? ', ' : '', h('button', { type: 'button', class: 'linkish', onclick: () => openTask(t.name, '') }, t.name)])));
  }
  if (C.S.me.isAdmin) out.push(h('div', { class: 'org-tasks' }, h('button', { type: 'button', class: 'linkish', onclick: () => unitHoursDialog(u) }, '🕘 Sprechzeiten bearbeiten')));
  return out;
}

export function unitHoursDialog(u, back) {
  const { h } = C;
  const ed = hoursEditor(u.hours);
  const inh = u.hours ? null : hoursFor(u.parent);
  C.openDialog({
    title: `Sprechzeiten: ${C.unitLabel(u)}`, wide: true,
    body: h('div', { class: 'admin' },
      h('p', { class: 'hint' }, 'Gelten auch für Untereinheiten ohne eigene Sprechzeiten und für Aufgaben dieser Einheit (sofern die Aufgabe keine eigenen hat).' + (inh ? ` Derzeit übernommen von ${C.unitLabel(inh.unit)}.` : '')),
      ed.el),
    buttons: ['spacer', { label: back ? 'Zurück' : 'Abbrechen', onClick: () => { if (back) { back(); return false; } } }, { label: 'Speichern', cls: 'primary', onClick: async () => {
      try { await C.api('PUT', `/api/org/units/${u.id}/hours`, { hours: ed.value() }); C.toast('Sprechzeiten gespeichert', 'ok'); await C.reload(); } catch (e) { C.toast(e.message, 'err'); return false; }
      if (back) { back(); return false; }
    } }],
  });
}

// ---------- Verwaltung ----------

export async function taskManager() {
  const { h } = C;
  await C.reload();
  const tasks = [...(C.S.tasks || [])].sort((a, b) => a.sort - b.sort || collator.compare(a.name, b.name));
  const rows = tasks.map((t) => {
    const u = C.S.unitById.get(t.orgUnit);
    const cv = coverage(t.rules);
    const others = t.rules.some((r) => r.from);
    const empty = t.rules.filter((r) => !(r.leads || []).some((id) => C.empByID(id)));
    const issues = [];
    if (!cv.hasAll && cv.gaps.length) issues.push('Lücken: ' + cv.gaps.join(', '));
    if (empty.length) issues.push('unbesetzt: ' + empty.map((r) => rangeLabel(r, others)).join(', '));
    if (!taskHours(t)) issues.push('keine Sprechzeiten');
    return h('tr', null,
      h('td', null, h('b', null, t.name), t.keywords ? h('div', { class: 'muted small' }, t.keywords) : null),
      h('td', null, u ? C.unitLabel(u) : h('span', { class: 'muted' }, '—')),
      h('td', null, t.rules.map((r) => rangeLabel(r, others)).join(' · ')),
      h('td', { class: issues.length ? 'warn' : 'muted' }, issues.length ? issues.map((x) => h('div', null, '⚠ ' + x)) : '✓'),
      h('td', null, h('button', { type: 'button', class: 'icon', title: 'Bearbeiten', onclick: () => taskDialog(t, taskManager) }, '✏️')));
  });
  C.openDialog({
    title: '🧭 Zuständigkeiten verwalten', wide: true,
    body: h('div', { class: 'admin' },
      h('p', { class: 'hint' }, 'Aufgaben mit Namenstrennung nach Anfangsbuchstaben des Nachnamens. Je Bereich mehrere Zuständige und Vertretungen; eine Person kann für viele Aufgaben zuständig sein. Sprechzeiten pflegst du an der Einheit (Organisation → Einheit → 🕘) oder abweichend an der Aufgabe.'),
      h('div', { class: 'toolbar' }, h('button', { type: 'button', class: 'primary', onclick: () => taskDialog(null, taskManager) }, '➕ Neue Aufgabe')),
      rows.length ? h('div', { class: 'table-wrap tall' }, h('table', { class: 'tbl' },
        h('thead', null, h('tr', null, ['Aufgabe', 'Einheit', 'Namenstrennung', 'Prüfung', ''].map((x) => h('th', null, x)))), h('tbody', null, rows)))
        : h('p', { class: 'muted' }, 'Noch keine Aufgaben.')),
    buttons: ['spacer', { label: 'Schließen', cls: 'primary' }],
  });
}

export function taskDialog(t, back) {
  const { h, field } = C;
  const isNew = !t;
  const name = h('input', { type: 'text', maxlength: 150, value: t ? t.name : '', placeholder: 'z. B. Wohngeld' });
  const keywords = h('input', { type: 'text', maxlength: 500, value: t ? t.keywords || '' : '', placeholder: 'weitere Suchbegriffe, z. B. Mietzuschuss, Lastenzuschuss' });
  const desc = h('textarea', { rows: 2, maxlength: 1000, placeholder: 'optional: was gehört dazu, welche Unterlagen …' }, t ? t.description || '' : '');
  const unitSel = C.unitSelect(t ? t.orgUnit || 0 : (C.S.view === 'org' && C.S.orgCur > 0 ? C.S.orgCur : 0));
  const sort = h('input', { type: 'number', min: 0, max: 99999, value: t ? t.sort || 0 : 0 });
  // Sprechzeiten: von der Einheit oder eigene
  const he = hoursEditor(t && t.hours);
  const inhText = h('span', { class: 'muted' });
  const ownBox = h('div', { class: 'hrs-own' }, he.el);
  const rOwn = h('input', { type: 'radio', name: 'hrs-mode', checked: t && t.hours ? true : null });
  const rUnit = h('input', { type: 'radio', name: 'hrs-mode', checked: t && t.hours ? null : true });
  const updHours = () => {
    const hi = hoursFor(+unitSel.value);
    inhText.textContent = hi ? ` (${hoursLines(hi.hours).join(' · ') || hi.hours.note})` : ' (bei der Einheit sind keine hinterlegt)';
    ownBox.hidden = !rOwn.checked;
  };
  rOwn.addEventListener('change', updHours); rUnit.addEventListener('change', updHours); unitSel.addEventListener('change', updHours);
  // Bereiche
  const rules = (t && t.rules.length ? t.rules : [{}]).map((r) => ({ from: r.from || '', to: r.to || '', note: r.note || '', leads: new Set(r.leads || []), deputies: new Set(r.deputies || []), open: isNew }));
  const cov = h('div', { class: 'cov' });
  const ruleBox = h('div', { class: 'rule-edit-list' });
  const people = C.pickList(0);
  // wie der Server: nur „von“ → bis Z, nur „bis“ → ab A, beides leer → alle (übrigen) Namen
  const normRule = (r) => {
    const f = foldName(r.from).slice(0, 4), to = foldName(r.to).slice(0, 4);
    return f || to ? { from: title(f || 'A'), to: title(to || 'Z') } : { from: '', to: '' };
  };
  const updCov = () => {
    const fixed = rules.map(normRule);
    const c = coverage(fixed);
    const msgs = [];
    if (c.overlaps.length) msgs.push(h('div', { class: 'rule-warn' }, '⚠ Überschneidung: ' + c.overlaps.join(', ')));
    if (fixed.filter((r) => !r.from).length > 1) msgs.push(h('div', { class: 'rule-warn' }, '⚠ Nur ein Bereich darf für alle (übrigen) Namen gelten'));
    if (!c.hasAll && c.gaps.length) msgs.push(h('div', { class: 'rule-warn' }, '⚠ Nicht abgedeckt: ' + c.gaps.join(', ') + ' – ergänzen oder einen Bereich ohne Buchstaben für „alle übrigen“ anlegen'));
    if (!msgs.length) msgs.push(h('div', { class: 'ok' }, c.ranged ? (c.hasAll ? '✓ Bereiche passen, übrige Namen gehen an den Bereich „alle übrigen“' : '✓ A–Z vollständig abgedeckt') : '✓ Ein Bereich für alle Namen'));
    cov.replaceChildren(...msgs);
  };
  const summary = (r) => {
    const n = normRule(r);
    const lbl = n.from ? rangeLabel(n) : (rules.some((x) => x !== r && normRule(x).from) ? 'alle übrigen Namen' : 'alle Namen');
    return `${lbl} · ${r.leads.size} zuständig · ${r.deputies.size} Vertretung`;
  };
  const renderRules = () => {
    ruleBox.replaceChildren(...rules.map((r, i) => {
      const sum = h('span', null, summary(r));
      const upd = () => { sum.textContent = summary(r); updCov(); };
      const from = h('input', { type: 'text', class: 'letters', maxlength: 4, value: r.from, placeholder: 'A', 'aria-label': 'von (Buchstaben)', oninput: (e) => { r.from = e.target.value; upd(); } });
      const to = h('input', { type: 'text', class: 'letters', maxlength: 4, value: r.to, placeholder: 'Z', 'aria-label': 'bis (Buchstaben)', oninput: (e) => { r.to = e.target.value; upd(); } });
      const note = h('input', { type: 'text', maxlength: 150, value: r.note, placeholder: 'Hinweis (optional), z. B. „nur Erstanträge“', oninput: (e) => { r.note = e.target.value; } });
      const lp = C.personPicker({ people, selected: r.leads, onChange: upd, placeholder: 'Zuständige suchen …', limit: 30 });
      const dp = C.personPicker({ people, selected: r.deputies, onChange: upd, placeholder: 'Vertretung suchen …', limit: 30 });
      const det = h('details', { class: 'rule-edit', open: r.open || rules.length === 1 ? true : null, ontoggle: (e) => { r.open = e.target.open; } },
        h('summary', null, h('b', null, `Bereich ${i + 1}`), ' · ', sum,
          rules.length > 1 ? h('button', { type: 'button', class: 'icon danger', title: 'Bereich entfernen', onclick: (e) => { e.preventDefault(); rules.splice(i, 1); renderRules(); } }, '🗑') : null),
        h('div', { class: 'rule-range' }, h('span', null, 'Nachnamen von'), from, h('span', null, 'bis'), to,
          h('small', { class: 'muted' }, 'leer = alle (übrigen) Namen; auch mehrbuchstabig, z. B. Sch bis Sz')),
        note,
        h('div', { class: 'share-cols' }, h('div', null, h('h4', null, '★ Zuständig'), lp.el), h('div', null, h('h4', null, '☆ Vertretung'), dp.el)));
      return det;
    }));
    updCov();
  };
  renderRules();
  const addRule = () => {
    for (const r of rules) r.open = false;
    const nf = nextFrom(rules.map(normRule));
    rules.push({ from: nf === 'A' && !rules.some((r) => normRule(r).from) ? '' : nf, to: '', note: '', leads: new Set(), deputies: new Set(), open: true });
    renderRules();
  };
  // Schnell aufteilen: A–Z gleichmäßig auf n Bereiche
  const split = (n) => {
    const cuts = { 2: [['A', 'K'], ['L', 'Z']], 3: [['A', 'G'], ['H', 'R'], ['S', 'Z']], 4: [['A', 'E'], ['F', 'K'], ['L', 'R'], ['S', 'Z']] }[n];
    const old = rules.splice(0);
    cuts.forEach(([f, to], i) => rules.push({ from: f, to, note: '', leads: old[i] ? old[i].leads : new Set(), deputies: old[i] ? old[i].deputies : new Set(), open: i === 0 }));
    renderRules();
  };
  const buttons = [];
  if (!isNew) buttons.push({ label: '🗑 Löschen', cls: 'danger', onClick: async () => {
    if (!(await C.confirmDialog('Aufgabe löschen', `„${t.name}“ mit allen Bereichen löschen?`, 'Löschen', true))) { taskDialog(t, back); return false; }
    try { await C.api('DELETE', `/api/tasks/${t.id}`); C.toast('Gelöscht', 'ok'); await C.reload(); } catch (e) { C.toast(e.message, 'err'); }
    if (back) back();
  } });
  buttons.push('spacer', { label: back ? 'Zurück' : 'Abbrechen', onClick: () => { if (back) { back(); return false; } } }, { label: 'Speichern', cls: 'primary', onClick: async () => {
    const body = {
      name: name.value, keywords: keywords.value, description: desc.value, orgUnit: +unitSel.value, sort: +sort.value || 0,
      hours: rOwn.checked ? he.value() : null,
      rules: rules.map((r) => ({ from: r.from, to: r.to, note: r.note, leads: [...r.leads], deputies: [...r.deputies] })),
    };
    try {
      if (isNew) await C.api('POST', '/api/tasks', body); else await C.api('PUT', `/api/tasks/${t.id}`, body);
      C.toast('Gespeichert', 'ok');
      await C.reload();
    } catch (e) { C.toast(e.message, 'err'); return false; }
    if (back) { back(); return false; }
  } });
  C.openDialog({
    title: isNew ? '➕ Neue Aufgabe' : `Aufgabe: ${t.name}`, wide: true,
    body: h('div', { class: 'form-grid task-form' },
      field('Aufgabe', name), field('Organisationseinheit', unitSel),
      h('div', { class: 'full' }, field('Suchbegriffe', keywords, 'Damit man die Aufgabe auch unter anderen Wörtern findet.')),
      h('div', { class: 'full' }, field('Beschreibung', desc)),
      h('div', { class: 'full hrs-mode' }, h('b', null, '🕘 Sprechzeiten'),
        h('label', { class: 'check' }, rUnit, ' wie die Einheit', inhText),
        h('label', { class: 'check' }, rOwn, ' eigene Sprechzeiten für diese Aufgabe'), ownBox),
      h('div', { class: 'full' }, h('div', { class: 'rule-head' }, h('b', null, '🔤 Namenstrennung, Zuständige und Vertretung'),
        h('span', { class: 'grow' }),
        h('span', { class: 'muted small' }, 'aufteilen:'),
        ...[2, 3, 4].map((n) => h('button', { type: 'button', class: 'linkish', title: `A–Z in ${n} Bereiche aufteilen`, onclick: () => split(n) }, `${n}×`))),
        cov, ruleBox,
        h('button', { type: 'button', onclick: addRule }, '＋ Bereich')),
      field('Sortierung', sort, 'Kleinere Zahlen stehen weiter oben.')),
    buttons,
  });
  updHours();
  setTimeout(() => name.focus(), 30);
}
