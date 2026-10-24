import { h, $ } from './util.js';

let dialogGen = 0;

// Modaler Dialog (natives <dialog>): Titel, Inhalt, Fußzeile mit Knöpfen.
export function openDialog({ title, body, buttons = [], wide = false, cls = '', onClose }) {
  const dlg = $('#dlg');
  // Das "close"-Ereignis kommt asynchron. Damit ein ersetzter Dialog nicht den neuen
  // schließt (bzw. dessen onClose auslöst), zählen wir die Dialoge mit.
  const gen = ++dialogGen;
  dlg.onclose = null;
  if (dlg.open) dlg.close();
  dlg.className = (wide ? 'wide ' : '') + cls;
  dlg.replaceChildren();
  const foot = h('footer', { class: 'dlg-foot' });
  const close = (v) => { if (gen === dialogGen && dlg.open) dlg.close(v || ''); };
  for (const b of buttons) {
    if (b === 'spacer') { foot.append(h('span', { class: 'grow' })); continue; }
    const btn = h('button', { type: 'button', class: b.cls || '', title: b.title || null }, b.label);
    btn.addEventListener('click', async () => {
      if (!b.onClick) return close();
      btn.disabled = true;
      try { const r = await b.onClick(); if (r !== false) close(); } finally { btn.disabled = false; }
    });
    foot.append(btn);
  }
  dlg.append(
    h('header', { class: 'dlg-head' }, h('h2', null, title), h('button', { type: 'button', class: 'icon', 'aria-label': 'Schließen', title: 'Schließen', onclick: () => close() }, '✕')),
    h('div', { class: 'dlg-body' }, body),
    foot,
  );
  dlg.onclose = () => { if (gen !== dialogGen || dlg.open) return; if (onClose) onClose(); };
  dlg.onclick = (e) => { if (e.target === dlg) close(); };
  dlg.showModal();
  return { el: dlg, close, foot };
}

export function confirmDialog(title, text, okLabel = 'OK', danger = false) {
  return new Promise((resolve) => {
    let result = false;
    openDialog({
      title, body: h('p', null, text),
      buttons: ['spacer', { label: 'Abbrechen' }, { label: okLabel, cls: danger ? 'danger' : 'primary', onClick: () => { result = true; } }],
      onClose: () => resolve(result),
    });
  });
}

// Formularfeld-Helfer
export function field(label, input, hint) {
  return h('label', { class: 'field' }, h('span', null, label), input, hint ? h('small', null, hint) : null);
}
