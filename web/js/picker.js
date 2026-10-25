// Auswahl von Personen (Avatar + Name, Mehrfachauswahl, Suche) und Ämtern.
import { h, norm, normAlt, tokens, collator } from './util.js';

// people: [{ id, lastName, firstName, amt, avatarURL }]
// selected: Set von IDs (wird direkt verändert); onChange() nach jeder Änderung
export function personPicker({ people, selected, onChange, placeholder = 'Person suchen …', limit = 60, selectAll = false }) {
  const list = [...people].sort((a, b) => collator.compare(a.lastName + ' ' + a.firstName, b.lastName + ' ' + b.firstName));
  const byId = new Map(list.map((p) => [p.id, p]));
  const idx = new Map(list.map((p) => [p.id, norm(`${p.lastName} ${p.firstName} ${p.amt || ''}`) + ' ' + normAlt(`${p.lastName} ${p.firstName} ${p.amt || ''}`)]));
  const q = h('input', { type: 'search', placeholder, autocomplete: 'off', 'aria-label': placeholder });
  const chips = h('div', { class: 'pick-chips' });
  const box = h('div', { class: 'pick-list', role: 'listbox', 'aria-multiselectable': 'true' });
  const more = h('div', { class: 'pick-more muted' });
  let hitsNow = [];
  const allBtn = selectAll ? h('button', { type: 'button', class: 'linkish', onclick: () => { for (const p of hitsNow) selected.add(p.id); render(); onChange && onChange(); } }) : null;
  const name = (p) => [p.lastName, p.firstName].filter(Boolean).join(', ');

  const toggle = (id) => {
    if (selected.has(id)) selected.delete(id); else selected.add(id);
    render();
    onChange && onChange();
  };
  function render() {
    chips.replaceChildren(...[...selected].map((id) => byId.get(id)).filter(Boolean).map((p) =>
      h('span', { class: 'chip pick-chip' }, h('img', { src: p.avatarURL, alt: '', width: 18, height: 18 }), name(p),
        h('button', { type: 'button', class: 'x', title: 'Entfernen', 'aria-label': 'Entfernen', onclick: () => toggle(p.id) }, '✕'))));
    chips.hidden = !selected.size;
    const t = tokens(q.value);
    const hits = list.filter((p) => t.every((x) => idx.get(p.id).includes(x)));
    // Ausgewählte zuerst, damit man sie wiederfindet
    hits.sort((a, b) => (selected.has(b.id) ? 1 : 0) - (selected.has(a.id) ? 1 : 0));
    box.replaceChildren(...hits.slice(0, limit).map((p) => {
      const on = selected.has(p.id);
      return h('button', { type: 'button', class: 'pick-item' + (on ? ' on' : ''), role: 'option', 'aria-selected': on ? 'true' : 'false', onclick: () => toggle(p.id) },
        h('span', { class: 'pick-check', 'aria-hidden': 'true' }, on ? '✓' : ''),
        h('img', { src: p.avatarURL, alt: '', width: 28, height: 28, loading: 'lazy' }),
        h('span', { class: 'pick-name' }, h('b', null, p.lastName), p.firstName ? ', ' + p.firstName : ''),
        p.amt ? h('small', { class: 'muted' }, p.amt) : null);
    }));
    more.textContent = hits.length > limit ? `… ${hits.length - limit} weitere – Suche verfeinern` : (hits.length ? '' : 'Niemand gefunden.');
    hitsNow = hits;
    if (allBtn) {
      const open = hits.filter((p) => !selected.has(p.id)).length;
      allBtn.textContent = `＋ alle ${hits.length} Treffer auswählen`;
      allBtn.hidden = !q.value.trim() || !open;
    }
  }
  q.addEventListener('input', render);
  render();
  return { el: h('div', { class: 'picker' }, chips, q, allBtn, box, more), focus: () => q.focus(), render };
}

// Ämter: Eingabe mit Vorschlägen; selected: Array (wird direkt verändert)
export function amtPicker({ amts, selected, counts, onChange }) {
  const id = 'amt-pick-' + Math.random().toString(36).slice(2);
  const inp = h('input', { type: 'text', list: id, placeholder: 'Amt wählen oder eintippen …', maxlength: 150, autocomplete: 'off' });
  const dl = h('datalist', { id }, [...amts].sort(collator.compare).map((a) => h('option', { value: a })));
  const chips = h('div', { class: 'pick-chips' });
  const add = () => {
    const v = inp.value.trim().replace(/\s+/g, ' ');
    if (!v) return;
    if (!selected.some((x) => x.toLowerCase() === v.toLowerCase())) selected.push(v);
    inp.value = '';
    render();
    onChange && onChange();
  };
  inp.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); add(); } });
  inp.addEventListener('change', () => { if (amts.includes(inp.value)) add(); });
  function render() {
    chips.replaceChildren(...selected.map((a, i) => h('span', { class: 'chip pick-chip amt' }, '🏛 ', a,
      counts && counts.get(a.toLowerCase()) ? h('small', { class: 'muted' }, ` (${counts.get(a.toLowerCase())})`) : null,
      h('button', { type: 'button', class: 'x', title: 'Entfernen', 'aria-label': 'Entfernen', onclick: () => { selected.splice(i, 1); render(); onChange && onChange(); } }, '✕'))));
    chips.hidden = !selected.length;
  }
  render();
  return { el: h('div', { class: 'picker amt' }, chips, h('div', { class: 'pick-add' }, inp, dl, h('button', { type: 'button', onclick: add }, '＋ Hinzufügen'))) };
}
