// Kleine Helfer: DOM-Bau ohne innerHTML (CSP-/XSS-sicher), API-Aufrufe, Toasts, Suche.

export function h(tag, attrs, ...children) {
  const el = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v == null || v === false) continue;
      if (k === 'class') el.className = v;
      else if (k === 'dataset') Object.assign(el.dataset, v);
      else if (k === 'style') Object.assign(el.style, v);
      else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
      else if (k === 'text') el.textContent = v;
      else if (v === true) el.setAttribute(k, '');
      else el.setAttribute(k, v);
    }
  }
  for (const c of children.flat(Infinity)) {
    if (c == null || c === false) continue;
    el.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return el;
}

export const $ = (s, r = document) => r.querySelector(s);
export const $$ = (s, r = document) => [...r.querySelectorAll(s)];

let csrf = '';
export function setCSRF(t) { csrf = t || ''; }
export class APIError extends Error { constructor(msg, status) { super(msg); this.status = status; } }

export async function api(method, path, body) {
  const opt = { method, headers: { Accept: 'application/json' }, credentials: 'same-origin' };
  if (body !== undefined) { opt.headers['Content-Type'] = 'application/json'; opt.body = JSON.stringify(body); }
  if (method !== 'GET') opt.headers['X-CSRF-Token'] = csrf;
  let r;
  try { r = await fetch(path, opt); } catch { throw new APIError('Server nicht erreichbar', 0); }
  let data = null;
  try { data = await r.json(); } catch { /* leer */ }
  if (!r.ok) {
    if (r.status === 401 && path !== '/api/login' && path !== '/api/me') window.dispatchEvent(new CustomEvent('tb:logout'));
    throw new APIError((data && data.error) || `Fehler ${r.status}`, r.status);
  }
  return data;
}

let toastT;
export function toast(msg, kind = '') {
  const t = $('#toast');
  t.textContent = msg;
  t.className = 'show ' + kind;
  clearTimeout(toastT);
  toastT = setTimeout(() => (t.className = ''), kind === 'err' ? 5000 : 2600);
}

// ---------- Suche ----------
export function norm(s) {
  return String(s || '').toLowerCase().replace(/ß/g, 'ss').replace(/ı/g, 'i').normalize('NFD').replace(/[̀-ͯ]/g, '');
}
// "Müller" soll auch bei "mueller" gefunden werden
export function normAlt(s) {
  return norm(String(s || '').toLowerCase().replace(/ä/g, 'ae').replace(/ö/g, 'oe').replace(/ü/g, 'ue'));
}
export const digits = (s) => String(s || '').replace(/\D/g, '');
export const tokens = (q) => norm(q).split(/[\s,;]+/).filter(Boolean);

export const collator = new Intl.Collator('de', { sensitivity: 'base', numeric: true });

export function telHref(n) {
  const d = String(n || '').replace(/[^\d+]/g, '');
  return d ? 'tel:' + d : null;
}

export function fmtDate(s) {
  if (!s) return '';
  const d = new Date(s);
  return d.toLocaleString('de-DE', { dateStyle: 'short', timeStyle: 'short' });
}
