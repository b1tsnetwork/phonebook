// Pixel-Kopf im Minecraft-Stil, Version 2: 16×16 Pixel je Würfelseite + Überzug-Ebene + Aufbauten
// (Hutkrempe, Dutt, Ohren …). Erzeugt aus den Optionen eine Textur und zeichnet sie in 3D
// (Canvas 2D, affine Texturabbildung, in Kacheln tiefensortiert). Unterstützt Pixelmalerei.

export const R = 16; // Auflösung je Seite
const H8 = R / 2;    // halbe Kantenlänge in Texeln
export const FACES = ['front', 'right', 'back', 'left', 'top', 'bottom'];
export const FACE_LABELS = { front: 'Vorne', right: 'Rechts', back: 'Hinten', left: 'Links', top: 'Oben', bottom: 'Unten' };

// Standard: neutrales Grau – kein Geschlecht, keine Hautfarbe
export const DEFAULT_AVATAR = Object.freeze({
  v: 2, skin: '#a7a9ac', hair: '#6d6f73', eyes: '#3d3f42', accent: '#8a8d91', glassesColor: '#3a3a3a',
  hairStyle: 'short', eyeStyle: 'classic', browStyle: 'normal', mouthStyle: 'smile', noseStyle: 'none',
  beardStyle: 'none', glassesStyle: 'none', hat: 'none', ears: 'none', extras: [], texture: true,
});

export const OPTIONS = {
  hairStyle: [['bare', 'Ohne'], ['none', 'Glatze'], ['buzz', 'Raspel'], ['crew', 'Bürstenschnitt'], ['short', 'Kurz'], ['side', 'Seitenscheitel'], ['slick', 'Zurückgegelt'], ['quiff', 'Tolle'],
    ['fringe', 'Pony'], ['emo', 'Seitenpony'], ['undercut', 'Undercut'], ['spiky', 'Stachelig'], ['mohawk', 'Iro'], ['fauxhawk', 'Faux-Hawk'], ['curly', 'Locken'],
    ['afro', 'Afro'], ['afropuffs', 'Afro-Puffs'], ['bob', 'Bob'], ['lob', 'Schulterlang'], ['long', 'Lang'], ['wavy', 'Lang gewellt'], ['middle', 'Mittelscheitel'],
    ['ponytail', 'Pferdeschwanz'], ['highpony', 'Hoher Zopf'], ['bun', 'Dutt'], ['spacebuns', 'Doppeldutt'], ['pigtails', 'Zöpfchen'], ['braids', 'Flechtzöpfe'],
    ['mullet', 'Vokuhila'], ['dreads', 'Dreadlocks'], ['cornrows', 'Cornrows'], ['balding', 'Halbglatze'], ['headscarf', 'Kopftuch']],
  eyeStyle: [['none', 'Keine'], ['classic', 'Klassisch'], ['dots', 'Punkte'], ['small', 'Klein'], ['round', 'Rund'], ['big', 'Groß'], ['almond', 'Mandel'], ['lashes', 'Wimpern'],
    ['sleepy', 'Verschlafen'], ['narrow', 'Schmal'], ['happy', 'Fröhlich'], ['closed', 'Geschlossen'], ['wink', 'Zwinkern'], ['surprised', 'Überrascht'],
    ['cat', 'Katzenaugen'], ['glow', 'Leuchtend'], ['sparkle', 'Funkelnd'], ['tired', 'Augenringe'], ['heart', 'Verliebt']],
  browStyle: [['none', 'Keine'], ['thin', 'Dünn'], ['normal', 'Normal'], ['thick', 'Kräftig'], ['bushy', 'Buschig'], ['arched', 'Geschwungen'], ['raised', 'Hoch'],
    ['angry', 'Grimmig'], ['worried', 'Besorgt'], ['short', 'Kurz'], ['uni', 'Durchgehend']],
  noseStyle: [['none', 'Keine'], ['dot', 'Punkt'], ['button', 'Stups'], ['long', 'Lang'], ['wide', 'Breit'], ['nostrils', 'Nasenlöcher'], ['pointy', 'Spitz'], ['big', 'Groß'], ['crooked', 'Schief']],
  mouthStyle: [['none', 'Keiner'], ['smile', 'Lächeln'], ['bigsmile', 'Strahlen'], ['grin', 'Grinsen'], ['laugh', 'Lachen'], ['neutral', 'Neutral'], ['flat', 'Schmal'], ['smirk', 'Schief'],
    ['open', 'Staunen'], ['tongue', 'Zunge'], ['sad', 'Traurig'], ['pout', 'Schmollen'], ['cat', 'Katzenmund'], ['fangs', 'Vampir'], ['whistle', 'Pfeifen'], ['braces', 'Zahnspange'], ['kiss', 'Kussmund']],
  beardStyle: [['none', 'Kein Bart'], ['stubble', 'Dreitagebart'], ['mustache', 'Schnurrbart'], ['handlebar', 'Zwirbelbart'], ['soulpatch', 'Fliege'], ['goatee', 'Kinnbart'],
    ['vandyke', 'Van Dyke'], ['circle', 'Rundbart'], ['chin', 'Kinnkranz'], ['muttonchops', 'Koteletten'], ['full', 'Vollbart'], ['long', 'Langer Bart']],
  glassesStyle: [['none', 'Keine'], ['round', 'Rund'], ['square', 'Eckig'], ['rect', 'Schmal'], ['nerd', 'Nerd-Brille'], ['cateye', 'Cat-Eye'], ['aviator', 'Piloten'],
    ['sun', 'Sonnenbrille'], ['shield', 'Visier'], ['pixel', 'Pixel-Brille'], ['mono', 'Monokel'], ['3d', '3D-Brille'], ['goggles', 'Schutzbrille'], ['vr', 'VR-Brille']],
  hat: [['none', 'Nichts'], ['cap', 'Cap'], ['capback', 'Cap verkehrt'], ['beanie', 'Mütze'], ['pompom', 'Bommelmütze'], ['bucket', 'Fischerhut'], ['beret', 'Baskenmütze'],
    ['cowboy', 'Cowboyhut'], ['tophat', 'Zylinder'], ['wizard', 'Zauberhut'], ['party', 'Partyhut'], ['santa', 'Nikolausmütze'], ['chef', 'Kochmütze'], ['grad', 'Doktorhut'],
    ['viking', 'Wikingerhelm'], ['helmet', 'Bauhelm'], ['crown', 'Krone'], ['tiara', 'Diadem'], ['flowercrown', 'Blumenkranz'], ['halo', 'Heiligenschein'], ['devil', 'Teufelshörner'],
    ['catears', 'Katzenohren'], ['bunnyears', 'Hasenohren'], ['propeller', 'Propellermütze'], ['headset', 'Headset'], ['headphones', 'Kopfhörer'], ['band', 'Stirnband'],
    ['bandana', 'Bandana'], ['bow', 'Schleife'], ['flower', 'Blume'], ['pumpkin', 'Kürbis']],
  ears: [['none', 'Flach'], ['small', 'Klein'], ['normal', 'Normal'], ['big', 'Groß'], ['elf', 'Spitz (Elf)']],
  extras: [['blush', 'Rouge'], ['freckles', 'Sommersprossen'], ['mole', 'Leberfleck'], ['beautymark', 'Schönheitsfleck'], ['eyebags', 'Tränensäcke'], ['wrinkles', 'Lachfalten'],
    ['scar', 'Narbe'], ['bandaid', 'Pflaster'], ['facepaint', 'Kriegsbemalung'], ['studs', 'Ohrstecker'], ['hoops', 'Creolen'], ['nosering', 'Nasenpiercing'],
    ['browring', 'Augenbrauen-Piercing'], ['lipring', 'Lippenpiercing'], ['mask', 'Mund-Nasen-Schutz'], ['pencil', 'Stift hinterm Ohr']],
};

export const SKIN_COLORS = [
  '#d9dadc', '#c2c4c7', '#a7a9ac', '#8c8f93', '#6f7276', // Grautöne (neutral)
  '#ffe3cc', '#f6cfa9', '#ecbf96', '#e0a97d', '#c98e62', '#a36a43', '#8a5534', '#6b4027', '#4e2d1b', // Hauttöne
  '#7fc97f', '#6fa8dc', '#b4a7d6', '#f4a6c8', '#ffd966', // Fantasie
];
export const HAIR_COLORS = ['#6d6f73', '#3a3b3e', '#151515', '#2b1a10', '#4a2e1a', '#6b4224', '#8a5a2b', '#b07a3c', '#d8a95b', '#ecd28c', '#b5452b', '#d9772e', '#a3a8ad', '#ececec',
  '#7b2cbf', '#2a9df4', '#e83f8b', '#2a9d8f', '#3fbf5f'];
export const EYE_COLORS = ['#3d3f42', '#151515', '#3b2a1a', '#5a4632', '#6b8f3a', '#3f7f4f', '#4a6a8a', '#3a86ff', '#8bb8e8', '#8a6d3b', '#7b2cbf', '#e63946', '#d4a017'];
export const ACCENT_COLORS = ['#8a8d91', '#e63946', '#f4a261', '#ffd166', '#06d6a0', '#2a9d8f', '#3a86ff', '#4361ee', '#8338ec', '#ef476f', '#151515', '#f5f5f5', '#6b4f3a', '#556b2f'];
export const GLASSES_COLORS = ['#3a3a3a', '#151515', '#8b5a2b', '#c9a227', '#c0c0c0', '#e63946', '#3a86ff', '#2a9d8f', '#ef476f', '#f5f5f5'];
export const LIP_COLORS = ['#b23a48', '#e5383b', '#c9184a', '#d17a7a', '#9d4edd', '#8d5524', '#151515'];

// ---------- Farbhelfer ----------
const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
function rgb(h) { const n = parseInt(String(h).slice(1), 16); return [(n >> 16) & 255, (n >> 8) & 255, n & 255]; }
function hex([r, g, b]) { return '#' + [r, g, b].map((v) => clamp(Math.round(v), 0, 255).toString(16).padStart(2, '0')).join(''); }
export function shade(h, f) { return hex(rgb(h).map((v) => v * f)); }
export function mix(a, b, t) { const A = rgb(a), B = rgb(b); return hex(A.map((v, i) => v + (B[i] - v) * t)); }
export function luminance(h) { const [r, g, b] = rgb(h); return (0.299 * r + 0.587 * g + 0.114 * b) / 255; }
function hash(a, b, c, d) { let x = (a * 374761393 + b * 668265263 + c * 2147483647 + d * 1274126177) | 0; x = (x ^ (x >>> 13)) * 1274126177; return ((x ^ (x >>> 16)) >>> 0) / 4294967295; }
const FACE_N = { front: 1, right: 2, back: 3, left: 4, top: 5, bottom: 6 };
const isHex = (c) => /^#[0-9a-f]{6}$/i.test(c || '');

// Alte Avatare (Version 1, 8×8) übernehmen: Optionen bleiben, gemalte Pixel werden auf 16×16 vergrößert.
export function normalize(av) {
  const src = av && typeof av === 'object' ? av : {};
  const a = { ...DEFAULT_AVATAR, ...src };
  a.extras = Array.isArray(src.extras) ? [...src.extras] : [];
  for (const k of ['skin', 'hair', 'eyes', 'accent', 'glassesColor']) if (!isHex(a[k])) a[k] = DEFAULT_AVATAR[k];
  for (const k of ['eyes2', 'lips', 'highlight', 'beardColor']) if (a[k] != null && !isHex(a[k])) delete a[k];
  if (src.v === 1) {
    if (a.noseStyle === 'bridge') a.noseStyle = 'long';
    if (!src.browStyle) a.browStyle = 'none';
    if (src.paint && typeof src.paint === 'object') {
      const out = {};
      for (const [layer, faces] of Object.entries(src.paint)) {
        out[layer] = {};
        for (const [f, px] of Object.entries(faces || {})) {
          out[layer][f] = {};
          for (const [i, c] of Object.entries(px || {})) {
            const n = +i, u = n % 8, v = (n / 8) | 0;
            for (const [du, dv] of [[0, 0], [1, 0], [0, 1], [1, 1]]) out[layer][f][(v * 2 + dv) * R + u * 2 + du] = c;
          }
        }
      }
      a.paint = out;
    }
  } else if (src.paint && typeof src.paint === 'object') {
    a.paint = expandPaint(src.paint); // Editor arbeitet pixelweise
  } else {
    delete a.paint;
  }
  a.v = 2;
  return a;
}

// ---------- Textur erzeugen ----------
function emptyLayer() { const o = {}; for (const f of FACES) o[f] = new Array(R * R).fill(null); return o; }

export function buildTexture(avIn) {
  const av = normalize(avIn);
  const base = emptyLayer(), over = emptyLayer();
  const boxes = [];
  const noisy = av.texture !== false;
  const S = av.skin, H = av.hair, E = av.eyes, A = av.accent, G = av.glassesColor;
  const E2 = av.eyes2 || E;
  const B = av.beardColor || H;
  const HL = av.highlight;
  const Hd = shade(H, 0.82), Hl = mix(H, '#ffffff', 0.12);
  const W = mix(S, '#ffffff', 0.85);
  const L = (layer) => (layer === 1 ? base : over);
  const N = (c, layer, f, u, v, amt = 0.06) => (noisy && amt ? shade(c, 1 - amt + hash(layer, FACE_N[f], u, v) * amt * 2) : c);
  const inb = (u, v) => u >= 0 && u < R && v >= 0 && v < R;
  const set = (layer, f, u, v, c, amt) => { if (!inb(u, v)) return; L(layer)[f][v * R + u] = c == null ? null : N(c, layer, f, u, v, amt); };
  const get = (layer, f, u, v) => (inb(u, v) ? L(layer)[f][v * R + u] : null);
  const rect = (layer, f, u0, v0, u1, v1, c, amt) => { for (let v = v0; v <= v1; v++) for (let u = u0; u <= u1; u++) set(layer, f, u, v, typeof c === 'function' ? c(u, v) : c, amt); };
  const fill = (layer, f, c, amt) => rect(layer, f, 0, 0, R - 1, R - 1, c, amt);
  // Seiten symmetrisch: d = Abstand von der Vorderkante (0 = vorne)
  const side = (layer, d, v, c, amt) => { set(layer, 'right', d, v, typeof c === 'function' ? c('right', d, v) : c, amt); set(layer, 'left', R - 1 - d, v, typeof c === 'function' ? c('left', d, v) : c, amt); };
  const topd = (layer, u, d, c, amt) => set(layer, 'top', u, R - 1 - d, c, amt);
  const botd = (layer, u, d, c, amt) => set(layer, 'bottom', u, d, c, amt);
  const mirror = (layer, f, u, v, c, amt) => { set(layer, f, u, v, c, amt); set(layer, f, R - 1 - u, v, c, amt); };
  // Muster als Text: Zeichen → Farbe ('.' oder ' ' = nichts)
  const pattern = (layer, f, x0, y0, rows, map, amt = 0.02) => rows.forEach((row, j) => [...row].forEach((ch, i) => { if (map[ch]) set(layer, f, x0 + i, y0 + j, map[ch], amt); }));
  // Auge/Braue: Muster von außen nach innen, linkes Auge endet innen bei x=6, rechtes beginnt bei x=9
  const eyePattern = (layer, rows, y0, map, mapR) => rows.forEach((row, j) => {
    const w = row.length;
    [...row].forEach((ch, i) => {
      if (map[ch]) set(layer, 'front', 6 - (w - 1) + i, y0 + j, map[ch], 0.02);
      const mr = mapR || map;
      if (mr[ch]) set(layer, 'front', 9 + (w - 1) - i, y0 + j, mr[ch], 0.02);
    });
  });
  const center = (layer, f, y0, rows, map, amt = 0.02) => rows.forEach((row, j) => { const x0 = Math.round((R - row.length) / 2); [...row].forEach((ch, i) => { if (map[ch]) set(layer, f, x0 + i, y0 + j, map[ch], amt); }); });
  const box = (x0, y0, z0, x1, y1, z1, c, opt = {}) => boxes.push({ x0, y0, z0, x1, y1, z1, c, amt: noisy ? (opt.amt ?? 0.06) : 0, pat: opt.pat || null });

  // Haarfarbe mit Strähnen/Struktur
  const hairC = (f, u, v) => {
    if (HL && (u + (f === 'top' ? v : 0)) % 5 === 2) return HL;
    const h = hash(7, FACE_N[f], u, v);
    return h < 0.12 ? Hd : h > 0.93 ? Hl : H;
  };
  const hpx = (layer, f, u, v, amt = 0.05) => set(layer, f, u, v, hairC(f, u, v), amt);

  // ---------- Haut & Grundform ----------
  for (const f of FACES) fill(1, f, S, 0.03);
  // Ohne Gesicht (z. B. einfarbiger Block): keine Schatten für Kinn und Ohransatz
  if (av.eyeStyle !== 'none' || av.mouthStyle !== 'none') {
    rect(1, 'front', 0, 15, 15, 15, shade(S, 0.94), 0.02);                 // Kinnschatten
    for (let d = 6; d <= 9; d++) for (let v = 7; v <= 10; v++) side(1, d, v, shade(S, 0.9), 0.02); // Ohransatz
  }

  // Ohren (Aufbauten in Hautfarbe)
  const Se = shade(S, 0.95);
  const ear = (x0, x1, y0, y1, z0, z1, inner = true) => {
    for (const sgn of [1, -1]) {
      const a = sgn > 0 ? x0 : -x1, b = sgn > 0 ? x1 : -x0;
      box(a, y0, z0, b, y1, z1, Se, { amt: 0.04 });
      if (inner) { const d = 0.25; box(sgn > 0 ? x1 : -x1 - d, y0 + 1, z0 + 0.8, sgn > 0 ? x1 + d : -x1, y1 - 1, z1 - 0.8, shade(S, 0.72), { amt: 0.02 }); }
    }
  };
  switch (av.ears) {
    case 'small': ear(8, 9.4, -1, 2.5, -1.2, 1.2, false); break;
    case 'normal': ear(8, 10, -1.5, 3.5, -1.8, 1.4); break;
    case 'big': ear(8, 11, -3, 4.5, -2.5, 2); break;
    case 'elf':
      ear(8, 10, -1.5, 3.5, -1.8, 1.4);
      box(8, -4.5, -3, 9.6, -1.5, -0.6, Se, { amt: 0.04 }); box(-9.6, -4.5, -3, -8, -1.5, -0.6, Se, { amt: 0.04 });
      box(8, -6.5, -4.4, 9.2, -4.5, -2.6, Se, { amt: 0.04 }); box(-9.2, -6.5, -4.4, -8, -4.5, -2.6, Se, { amt: 0.04 });
      break;
  }

  // ---------- Haare ----------
  const hs = av.hairStyle;
  // Bedeckung: front[u] = Anzahl Zeilen von oben; sides[d]; back = Zeilen (Zahl oder Array)
  const cover = ({ front, sides, back, top = true, layer = 1, color }) => {
    const C = color || ((f, u, v) => hairC(f, u, v));
    if (top) for (let u = 0; u < R; u++) for (let v = 0; v < R; v++) set(layer, 'top', u, v, typeof C === 'function' ? C('top', u, v) : C, 0.05);
    if (front) front.forEach((h, u) => { for (let v = 0; v < h; v++) set(layer, 'front', u, v, typeof C === 'function' ? C('front', u, v) : C, 0.05); });
    if (sides) sides.forEach((h, d) => { for (let v = 0; v < h; v++) side(layer, d, v, typeof C === 'function' ? (f, dd, vv) => C(f, dd, vv) : C, 0.05); });
    if (back != null) for (let u = 0; u < R; u++) { const h = Array.isArray(back) ? back[u] : back; for (let v = 0; v < h; v++) set(layer, 'back', u, v, typeof C === 'function' ? C('back', u, v) : C, 0.05); }
  };
  const arr = (n, f) => Array.from({ length: n }, (_, i) => f(i));
  const SHORT_F = [6, 5, 4, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 4, 5, 6];
  const SHORT_S = [7, 6, 6, 6, 6, 6, 6, 6, 6, 7, 9, 10, 11, 11, 11, 11];
  const LONG_S = arr(16, (d) => (d < 2 ? 16 : 16));
  const shaved = mix(S, H, 0.35);
  const shortHair = () => cover({ front: SHORT_F, sides: SHORT_S, back: 11 });
  const strands = (layer, f, u0, u1, v0, v1) => { for (let u = u0; u <= u1; u++) for (let v = v0; v <= v1; v++) if ((u + v) % 4 === 0) set(layer, f, u, v, Hd, 0.04); };
  const yPx = (v) => v - H8; // Texelzeile → Modellkoordinate

  switch (hs) {
    case 'bare': break; // gar nichts – z. B. für einfarbige Blöcke (Einrichtungen)
    case 'none':
      rect(1, 'top', 4, 4, 7, 6, mix(S, '#ffffff', 0.25), 0.02);
      set(1, 'top', 5, 5, mix(S, '#ffffff', 0.45));
      break;
    case 'buzz':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 4 : 2)), sides: arr(16, (d) => (d < 6 ? 4 : d < 10 ? 5 : 9)), back: 9, color: mix(S, H, 0.55) });
      break;
    case 'crew':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 4 : 2)), sides: SHORT_S.map((x) => x - 1), back: 10 });
      for (let u = 0; u < R; u++) for (let v = 6; v < R; v++) if ((u + v) % 3 === 0) set(2, 'top', u, v, Hl);
      break;
    case 'short': shortHair(); strands(1, 'top', 0, 15, 0, 15); break;
    case 'side':
      cover({ front: [7, 7, 7, 6, 6, 6, 5, 4, 3, 3, 3, 3, 3, 4, 5, 6], sides: SHORT_S, back: 11 });
      for (let v = 0; v < R; v++) set(1, 'top', 5, v, mix(S, H, 0.5));
      for (let u = 0; u < 6; u++) set(1, 'front', u, 3, Hl);
      break;
    case 'slick':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 4 : 2)), sides: SHORT_S, back: 11 });
      for (let u = 1; u < R; u += 3) for (let v = 0; v < R; v++) set(1, 'top', u, v, Hl, 0.02);
      break;
    case 'quiff':
      shortHair();
      box(-5, -11, 2, 5, -8, 9, H, { amt: 0.08 });
      box(-4, -12, 4, 4, -11, 9.5, Hl, { amt: 0.06 });
      break;
    case 'fringe':
      cover({ front: arr(16, (u) => 6 + (u % 2)), sides: SHORT_S, back: 11 });
      for (let u = 0; u < R; u++) set(2, 'front', u, 5 + (u % 2), hairC('front', u, 5));
      break;
    case 'emo':
      cover({ front: [11, 11, 11, 10, 10, 9, 8, 7, 6, 5, 4, 3, 3, 3, 4, 5], sides: SHORT_S.map((x, d) => (d < 4 ? 10 : x)), back: 12 });
      for (let u = 0; u < 9; u++) set(2, 'front', u, 11 - Math.floor(u / 1.5), hairC('front', u, 9));
      break;
    case 'undercut':
      cover({ front: arr(16, (u) => (u < 9 ? 4 : 3)), sides: arr(16, () => 2), back: 2 });
      cover({ top: false, sides: arr(16, (d) => (d < 1 ? 0 : d < 10 ? 7 : 11)), back: 11, color: shaved });
      cover({ front: arr(16, () => 2), sides: arr(16, () => 1), back: 1, layer: 2 });
      for (let u = 0; u < 9; u++) set(2, 'front', u, 2, hairC('front', u, 2));
      break;
    case 'spiky':
      shortHair();
      for (let u = 0; u < R; u++) for (let v = 0; v < R; v++) if ((u + v * 2) % 3 === 0) set(2, 'top', u, v, (u + v) % 2 ? H : Hd);
      for (let u = 0; u < R; u += 2) set(2, 'front', u, 0, H);
      for (const [x, z] of [[-6, -5], [-2, -6], [3, -4], [6, 0], [-5, 2], [1, 3], [4, 5], [-2, 6]]) box(x - 1, -10.5, z - 1, x + 1, -8, z + 1, (x + z) % 2 ? H : Hd, { amt: 0.06 });
      break;
    case 'mohawk':
      cover({ top: false, front: arr(16, (u) => (u >= 6 && u <= 9 ? 3 : 0)), sides: arr(16, (d) => (d < 2 ? 0 : d < 10 ? 6 : 10)), back: 10, color: shaved });
      for (let u = 6; u <= 9; u++) for (let v = 0; v < R; v++) hpx(1, 'top', u, v);
      for (let u = 6; u <= 9; u++) for (let v = 0; v < 11; v++) hpx(1, 'back', u, v);
      rect(1, 'top', 0, 0, 5, 15, (u, v) => mix(S, H, 0.3), 0.08);
      rect(1, 'top', 10, 0, 15, 15, (u, v) => mix(S, H, 0.3), 0.08);
      box(-2, -14, -9, 2, -8, 7, H, { amt: 0.1 });
      box(-1.4, -16, -7, 1.4, -14, 4, Hd, { amt: 0.08 });
      break;
    case 'fauxhawk':
      cover({ front: arr(16, (u) => (u < 3 || u > 12 ? 4 : 3)), sides: SHORT_S.map((x) => x - 1), back: 10, color: (f, u, v) => (f === 'top' && (u < 4 || u > 11) ? mix(S, H, 0.5) : hairC(f, u, v)) });
      box(-3, -11, -6, 3, -8, 7, H, { amt: 0.08 });
      box(-2, -12.5, -2, 2, -11, 6, Hd, { amt: 0.06 });
      break;
    case 'curly':
      shortHair();
      for (const f of ['top', 'back', 'right', 'left']) for (let u = 0; u < R; u++) for (let v = 0; v < R; v++) {
        if (f === 'back' && v > 11) continue;
        if ((f === 'right' || f === 'left') && (v > 8 || (f === 'right' ? u : R - 1 - u) < 1)) continue;
        if ((u * 3 + v * 5) % 4 < 2) set(2, f, u, v, (u + v) % 3 ? H : Hd, 0.08);
      }
      for (let u = 0; u < R; u++) if (u % 2 === 0) { set(2, 'front', u, 0, H); set(2, 'front', u, 1, Hd); }
      break;
    case 'afro':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 8 : 4)), sides: arr(16, (d) => (d < 1 ? 8 : 14)), back: 14 });
      for (let u = 0; u < R; u++) if (u % 2) set(2, 'front', u, 3, H);
      box(-11.5, -16, -11.5, 11.5, -2, 5, H, { amt: 0.14 });
      break;
    case 'afropuffs':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 6 : 3)), sides: SHORT_S, back: 11 });
      for (let v = 0; v < R; v++) set(1, 'top', 7, v, mix(S, H, 0.5));
      box(-12, -14, -5, -4, -6, 3, H, { amt: 0.14 });
      box(4, -14, -5, 12, -6, 3, H, { amt: 0.14 });
      break;
    case 'bob':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 12 : 5)), sides: arr(16, (d) => (d < 1 ? 12 : 13)), back: 13 });
      for (let d = 0; d < R; d++) side(2, d, 12, hairC('right', d, 12));
      for (let u = 0; u < R; u++) set(2, 'back', u, 12, hairC('back', u, 12));
      for (let u = 2; u < 14; u++) set(1, 'front', u, 4, u % 3 ? H : Hd);
      break;
    case 'lob':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 16 : 4)), sides: LONG_S, back: 16 });
      box(-8.5, 8, -8.5, 8.5, 12, -4, H, { amt: 0.08 });
      box(-9, 3, -4, -7.6, 11, 3, H); box(7.6, 3, -4, 9, 11, 3, H);
      break;
    case 'long': case 'wavy': case 'middle': {
      const wave = hs === 'wavy' ? (f, u, v) => (((v + Math.floor(u / 2)) % 6) < 2 ? Hd : hairC(f, u, v)) : undefined;
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 16 : hs === 'middle' ? (u === 7 || u === 8 ? 2 : 4 + Math.abs(7.5 - u) / 2) : 4)), sides: LONG_S, back: 16, color: wave });
      if (hs === 'middle') for (let v = 0; v < R; v++) { set(1, 'top', 7, v, mix(S, H, 0.4)); }
      const C = hs === 'wavy' ? Hd : H;
      box(-9, 8, -9, 9, 18, -3, H, { amt: 0.1, pat: hs === 'wavy' ? (i, j) => ((i + j) % 4 < 2 ? H : C) : null });
      box(-9.5, 2, -3, -7.5, 16, 4, H, { amt: 0.1 }); box(7.5, 2, -3, 9.5, 16, 4, H, { amt: 0.1 });
      break;
    }
    case 'ponytail':
      cover({ front: SHORT_F, sides: SHORT_S.map((x) => x + 1), back: 14 });
      box(-1.6, -4, -10, 1.6, -1.5, -8, A, { amt: 0.02 });
      box(-2.2, -3, -12.5, 2.2, 12, -9.5, H, { amt: 0.1 });
      break;
    case 'highpony':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 4 : 2)), sides: SHORT_S, back: 12 });
      for (let u = 0; u < R; u++) for (let v = 0; v < R; v += 2) set(1, 'top', u, v, Hl, 0.02);
      box(-1.6, -10, -7, 1.6, -8, -4, A, { amt: 0.02 });
      box(-2.2, -12, -12, 2.2, -8, -6, H, { amt: 0.1 });
      box(-2, -8, -12.5, 2, 6, -9, H, { amt: 0.1 });
      break;
    case 'bun':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 4 : 2)), sides: SHORT_S, back: 12 });
      box(-3.5, -14, -6, 3.5, -8, 1, H, { amt: 0.1 });
      box(-4, -9, -6.5, 4, -8, 1.5, Hd, { amt: 0.04 });
      break;
    case 'spacebuns':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 5 : 3)), sides: SHORT_S, back: 12 });
      for (let v = 0; v < R; v++) set(1, 'top', 7, v, mix(S, H, 0.5));
      box(-8, -13, -4, -2, -8, 2, H, { amt: 0.1 }); box(2, -13, -4, 8, -8, 2, H, { amt: 0.1 });
      break;
    case 'pigtails':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 8 : 4)), sides: arr(16, (d) => (d < 3 ? 8 : 12)), back: 12 });
      box(8, -3, -3, 10, -1, 0, A, { amt: 0.02 }); box(-10, -3, -3, -8, -1, 0, A, { amt: 0.02 });
      box(8.5, -1, -3.5, 12, 8, 0.5, H, { amt: 0.1 }); box(-12, -1, -3.5, -8.5, 8, 0.5, H, { amt: 0.1 });
      break;
    case 'braids':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 9 : 3)), sides: arr(16, (d) => (d < 3 ? 9 : 14)), back: 14 });
      for (let v = 0; v < R; v++) set(1, 'top', 7, v, mix(S, H, 0.45));
      for (let i = 0; i < 6; i++) {
        const y = 1 + i * 2.2, c = i % 2 ? Hd : H;
        box(7.6, y, 0, 10, y + 2.2, 2.6, c, { amt: 0.06 }); box(-10, y, 0, -7.6, y + 2.2, 2.6, c, { amt: 0.06 });
      }
      box(7.9, 14.2, 0.4, 9.7, 15.4, 2.2, A, { amt: 0.02 }); box(-9.7, 14.2, 0.4, -7.9, 15.4, 2.2, A, { amt: 0.02 });
      break;
    case 'mullet':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 4 : 2)), sides: arr(16, (d) => (d < 6 ? 5 : d < 10 ? 6 : 16)), back: 16 });
      box(-7, 8, -9, 7, 13, -5, H, { amt: 0.1 });
      break;
    case 'dreads':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 14 : 4)), sides: LONG_S, back: 16, color: (f, u, v) => ((u % 3) === 0 ? Hd : H) });
      for (let i = -7; i <= 7; i += 2.8) box(i - 0.9, 7, -9, i + 0.9, 15 + (Math.abs(i) % 3), -7.2, i % 2 ? Hd : H, { amt: 0.08 });
      for (const s of [-1, 1]) for (const z of [-4, 0, 3]) box(s > 0 ? 7.6 : -9.4, 5, z - 0.9, s > 0 ? 9.4 : -7.6, 13 + (z % 2), z + 0.9, H, { amt: 0.08 });
      break;
    case 'cornrows':
      cover({ front: arr(16, (u) => (u < 2 || u > 13 ? 4 : 2)), sides: SHORT_S, back: 11, color: (f, u, v) => (f === 'top' || f === 'back' ? (u % 3 === 2 ? mix(S, H, 0.35) : H) : H) });
      break;
    case 'balding':
      cover({ top: false, front: arr(16, (u) => (u < 2 || u > 13 ? 6 : 0)), sides: arr(16, (d) => (d < 2 ? 0 : d < 10 ? 7 : 11)), back: 11, color: (f, u, v) => (v < 3 && f !== 'back' ? S : hairC(f, u, v)) });
      for (let u = 0; u < R; u++) for (let v = 0; v < R; v++) if (v > 11 || u < 2 || u > 13) set(1, 'top', u, v, mix(S, H, 0.55), 0.08);
      set(1, 'top', 6, 6, mix(S, '#ffffff', 0.35));
      break;
    case 'headscarf': {
      const C = H, Cd = shade(C, 0.85);
      fill(1, 'top', C, 0.03); fill(1, 'back', C, 0.03);
      for (let v = 0; v < R; v++) for (let d = 0; d < R; d++) side(1, d, v, C, 0.03);
      for (let u = 0; u < R; u++) for (let v = 0; v < 4; v++) set(1, 'front', u, v, C, 0.03);
      for (let v = 4; v < R; v++) for (const u of [0, 1, 14, 15]) set(1, 'front', u, v, C, 0.03);
      for (let u = 0; u < R; u++) for (let d = 0; d < 5; d++) botd(1, u, R - 1 - d, C, 0.03);
      for (let u = 0; u < R; u++) set(2, 'front', u, 3, Cd, 0.02);
      for (let v = 3; v < R; v++) { set(2, 'front', 1, v, Cd, 0.02); set(2, 'front', 14, v, Cd, 0.02); }
      box(-9, 8, -9, 9, 13, 2, C, { amt: 0.04 });
      break;
    }
    default: shortHair();
  }

  // ---------- Gesicht ----------
  const eyeRows = { // von außen nach innen; Zeilen ab y=6
    classic: ['', 'WEEW', 'WEPW'], dots: ['', '.PP', '.PP'], small: ['', '', '.EP'], round: ['', 'WHEW', 'WEPW', '.WW.'],
    big: ['WWWW', 'WHEE', 'WEPE', '.WWW'], almond: ['', '.LLL', 'WEPW', '.WW.'], lashes: ['L.LL', 'LEEW', 'WEPW'],
    sleepy: ['', 'LLLL', 'WEPW'], narrow: ['', 'LLLL', 'WEPW', 'LLLL'], happy: ['', '.KK.', 'K..K'], closed: ['', '', 'K..K', '.KK.'],
    surprised: ['WWWW', 'WPPW', 'WPPW', 'WWWW'], cat: ['', 'EEPE', 'EEPE', '.EP.'], glow: ['', 'GGGG', 'GGGG'], sparkle: ['', 'EHEE', 'EEHE'],
    tired: ['', 'WEEW', 'WEPW', 'SSSS'], heart: ['', 'R.R', 'RRR', '.R.'], none: [''],
  };
  const lash = shade(S, 0.35);
  const em = (e) => ({ W, E: e, P: shade(e, 0.45), H: '#ffffff', L: lash, K: shade(S, 0.3), S: shade(S, 0.8), G: mix(e, '#ffffff', 0.3), R: '#e63946' });
  let es = av.eyeStyle;
  if (!eyeRows[es] && es !== 'wink') es = 'classic';
  if (es === 'wink') { eyePattern(1, eyeRows.classic, 6, em(E), {}); eyePattern(1, eyeRows.happy, 6, {}, em(E2)); }
  else eyePattern(1, eyeRows[es], 6, em(E), em(E2));
  const tallEye = es !== 'wink' && (eyeRows[es][0] || '').length > 0;

  const BR = shade(av.beardColor || H, 0.8);
  const browRows = {
    thin: ['', '.BBB'], normal: ['', 'BBBB'], thick: ['BBBB', 'BBBB'], bushy: ['B.BB', 'BBBB'], arched: ['.BB.', 'B..B'], raised: ['BBBB', ''],
    angry: ['BB..', '..BB'], worried: ['..BB', 'BB..'], short: ['', '..BB'],
  };
  if (av.browStyle === 'uni') rect(1, 'front', 3, tallEye ? 4 : 5, 12, tallEye ? 4 : 5, BR, 0.04);
  else if (browRows[av.browStyle]) eyePattern(1, browRows[av.browStyle], tallEye ? 3 : 4, { B: BR });

  const NC = shade(S, 0.84), ND = shade(S, 0.66);
  const noseRows = {
    dot: ['', '', 'NN'], button: ['', 'NN', 'DD'], long: ['NN', 'NN', 'DD'], wide: ['', '', 'NNNN'], nostrils: ['', '', 'D..D'],
    pointy: ['', 'N.', 'NN'], big: ['', '.NN.', 'NDDN'], crooked: ['N.', '.N', 'NN'],
  };
  if (noseRows[av.noseStyle]) center(1, 'front', 8, noseRows[av.noseStyle], { N: NC, D: ND });

  const lipC = av.lips || null;
  const M = lipC || mix(shade(S, 0.5), '#5a2020', luminance(S) > 0.3 && Math.abs(rgb(S)[0] - rgb(S)[2]) > 25 ? 0.35 : 0);
  const mouthMap = { M, D: mix(shade(S, 0.3), '#2a0a0a', 0.6), T: '#f2f2f2', R: '#e5687a', G: '#9aa3ad', L: lipC || '#c9184a' };
  const mouthRows = {
    smile: ['M....M', '.MMMM.'], bigsmile: ['MMMMMM', 'MTTTTM', '.MMMM.'], grin: ['MMMMMMMM', '.TTTTTT.', '..MMMM..'], laugh: ['MMMMMM', 'MDDDDM', '.MRRM.'],
    neutral: ['', 'MMMM'], flat: ['', 'MMMMMM'], smirk: ['....M', 'MMMM.'], open: ['.MM.', 'MDDM', 'MDDM', '.MM.'], tongue: ['', 'MMMMMM', '..RR..'],
    sad: ['', '.MMMM.', 'M....M'], pout: ['', '.MM.', 'MMMM'], cat: ['', 'M.MM.M', '.M..M.'], fangs: ['MMMMMM', '.T..T.'], whistle: ['', 'MM', 'MM'],
    braces: ['MMMMMM', 'MTGGTM', '.MMMM.'], kiss: ['', 'LL', 'LL'], none: [''],
  };
  const mr = mouthRows[av.mouthStyle] || mouthRows.smile;
  center(1, 'front', mr.length >= 4 ? 10 : 11, mr, mouthMap);

  // Bart
  const isMouth = (u, v) => { const c = get(1, 'front', u, v); return v >= 10 && c && Math.abs(luminance(c) - luminance(S)) > 0.12; };
  const beardPx = (u, v) => { if (!isMouth(u, v)) set(1, 'front', u, v, B, 0.07); };
  const mustache = () => { for (let u = 4; u <= 11; u++) set(1, 'front', u, 10, B, 0.06); set(1, 'front', 3, 11, B); set(1, 'front', 12, 11, B); };
  const goatee = () => { for (let u = 6; u <= 9; u++) for (let v = 13; v <= 15; v++) beardPx(u, v); for (let u = 6; u <= 9; u++) for (let d = 0; d < 3; d++) botd(1, u, d, B, 0.06); };
  switch (av.beardStyle) {
    case 'stubble': {
      const st = mix(S, B, 0.4);
      for (let v = 10; v < R; v++) for (let u = 1; u < R - 1; u++) if ((u + v) % 2 === 0 && !isMouth(u, v) && (v > 10 || u < 5 || u > 10)) set(1, 'front', u, v, st, 0.1);
      for (let d = 0; d < 5; d++) for (let v = 9; v < R; v++) if ((d + v) % 2 === 0) side(1, d, v, st, 0.1);
      for (let u = 0; u < R; u++) for (let d = 0; d < 5; d++) if ((u + d) % 2 === 0) botd(1, u, d, st, 0.1);
      break;
    }
    case 'mustache': mustache(); break;
    case 'handlebar': mustache(); set(1, 'front', 2, 10, B); set(1, 'front', 13, 10, B); set(1, 'front', 2, 9, B); set(1, 'front', 13, 9, B); break;
    case 'soulpatch': for (const u of [7, 8]) { set(1, 'front', u, 14, B, 0.05); } break;
    case 'goatee': goatee(); break;
    case 'vandyke': mustache(); goatee(); break;
    case 'circle': mustache(); goatee(); for (let v = 11; v <= 13; v++) { set(1, 'front', 4, v, B); set(1, 'front', 11, v, B); } break;
    case 'chin':
      for (let u = 0; u < R; u++) { beardPx(u, 15); beardPx(u, 14); }
      for (let d = 0; d < 3; d++) for (let v = 9; v < R; v++) side(1, d, v, B, 0.06);
      for (const u of [0, 1, 14, 15]) for (let v = 10; v < R; v++) set(1, 'front', u, v, B, 0.06);
      for (let u = 0; u < R; u++) for (let d = 0; d < 4; d++) botd(1, u, d, B, 0.06);
      break;
    case 'muttonchops':
      for (let d = 0; d < 6; d++) for (let v = 6; v < 14; v++) side(1, d, v, B, 0.06);
      for (const u of [0, 1, 2, 13, 14, 15]) for (let v = 8; v < 13; v++) set(1, 'front', u, v, B, 0.06);
      break;
    case 'full': case 'long':
      mustache();
      for (let v = 11; v < R; v++) for (let u = 0; u < R; u++) beardPx(u, v);
      for (let d = 0; d < 7; d++) for (let v = 8; v < R; v++) side(1, d, v, B, 0.06);
      for (let u = 0; u < R; u++) for (let d = 0; d < 10; d++) botd(1, u, d, B, 0.06);
      box(-6.5, 8, 3.5, 6.5, 10, 8.6, B, { amt: 0.08 });
      if (av.beardStyle === 'long') { box(-5, 10, 4.5, 5, 15, 8.4, B, { amt: 0.08 }); box(-3, 15, 5.5, 3, 18, 8.2, B, { amt: 0.08 }); }
      break;
  }

  // Details
  const ex = new Set(av.extras || []);
  if (ex.has('eyebags')) { for (const u of [3, 4, 5, 6]) mirror(1, 'front', u, 9, shade(S, 0.82)); }
  if (ex.has('wrinkles')) { for (const [u, v] of [[3, 11], [3, 12], [4, 13]]) mirror(1, 'front', u, v, shade(S, 0.8)); }
  if (ex.has('freckles')) { const fr = shade(S, 0.78); for (const [u, v] of [[2, 10], [4, 10], [3, 11], [5, 9], [6, 10]]) mirror(1, 'front', u, v, fr); }
  if (ex.has('blush')) { const bl = mix(S, '#ff5c8a', 0.38); for (const [u, v] of [[2, 10], [3, 10], [4, 10], [3, 11]]) mirror(1, 'front', u, v, bl); }
  if (ex.has('facepaint')) { for (const v of [10, 11]) for (const u of [2, 3, 4]) mirror(1, 'front', u, v, v === 10 ? A : shade(A, 0.8)); }
  if (ex.has('mole')) set(1, 'front', 12, 12, shade(S, 0.45));
  if (ex.has('beautymark')) set(1, 'front', 4, 12, '#2a1a12');
  if (ex.has('scar')) { for (let v = 4; v <= 10; v++) set(1, 'front', 11 + (v % 2), v, mix(S, '#b5525a', 0.45)); }
  if (ex.has('bandaid')) { pattern(1, 'front', 10, 2, ['BBBB', 'BPPB', 'BBBB'], { B: '#e8c99b', P: '#d9b483' }); }
  if (ex.has('nosering')) set(1, 'front', 9, 10, '#c9a227');
  if (ex.has('browring')) { set(1, 'front', 12, 4, '#c0c0c0'); set(1, 'front', 12, 6, '#c0c0c0'); }
  if (ex.has('lipring')) set(1, 'front', 9, 13, '#c0c0c0');
  if (ex.has('studs')) { side(1, 8, 11, A, 0.02); }
  if (ex.has('hoops')) { box(8.1, 3, -1, 8.7, 6, 1, '#c9a227', { amt: 0.02 }); box(-8.7, 3, -1, -8.1, 6, 1, '#c9a227', { amt: 0.02 }); }
  if (ex.has('mask')) {
    const mc = '#9ec9e6', ms = shade(mc, 0.88);
    for (let v = 9; v <= 14; v++) for (let u = 1; u <= 14; u++) set(2, 'front', u, v, (v === 11 || v === 13) ? ms : mc, 0.02);
    for (let d = 0; d < 8; d++) side(2, d, 9, '#e6eef3', 0.02);
  }
  if (ex.has('pencil')) { box(8.3, -3.5, -6, 9.2, -2.6, 2, '#f2c230', { amt: 0.02 }); box(8.4, -3.4, 2, 9.1, -2.7, 3.2, '#e8c99b', { amt: 0.02 }); box(8.3, -3.5, -7, 9.2, -2.6, -6, '#e5687a', { amt: 0.02 }); }

  // ---------- Brillen ----------
  const lens = (u, v, tint, t) => { const c = get(1, 'front', u, v) || S; set(1, 'front', u, v, mix(c, tint, t), 0); };
  const frameRows = (rows, y0 = 5) => eyePattern(1, rows, y0, { G }, { G });
  const temples = (v = 7) => { for (let d = 0; d < 8; d++) side(1, d, v, G, 0.02); };
  switch (av.glassesStyle) {
    case 'round': frameRows(['.GGG.', 'G...G', 'G...G', '.GGG.'].map((r) => r.slice(0, 5)), 5); rect(1, 'front', 7, 6, 8, 6, G, 0.02); temples(6); break;
    case 'square': frameRows(['GGGGG', 'G....', 'G....', 'GGGGG'], 5); rect(1, 'front', 7, 6, 8, 6, G, 0.02); rect(1, 'front', 2, 5, 2, 8, G); rect(1, 'front', 13, 5, 13, 8, G); temples(6); break;
    case 'rect': frameRows(['GGGGG', '.....', 'GGGGG'], 6); rect(1, 'front', 7, 7, 8, 7, G, 0.02); temples(7); break;
    case 'nerd': frameRows(['GGGGG', 'GG...', 'G....', 'GGGGG'], 5); rect(1, 'front', 7, 6, 8, 7, G, 0.02); rect(1, 'front', 7, 6, 8, 6, '#f5f5f5'); rect(1, 'front', 1, 5, 2, 8, G); rect(1, 'front', 13, 5, 14, 8, G); temples(6); break;
    case 'cateye': frameRows(['G....', 'GGGGG', 'G....', '.GGGG'], 4); set(1, 'front', 1, 4, G); set(1, 'front', 14, 4, G); rect(1, 'front', 7, 6, 8, 6, G, 0.02); temples(6); break;
    case 'aviator':
      for (const [u0] of [[2], [9]]) for (let u = u0; u < u0 + 5; u++) for (let v = 6; v <= 9; v++) if (!(v === 9 && (u === u0 || u === u0 + 4))) lens(u, v, '#5a4a2a', 0.45);
      rect(1, 'front', 2, 5, 13, 5, G, 0.02); set(1, 'front', 7, 6, G); set(1, 'front', 8, 6, G); temples(5); break;
    case 'sun': for (const u0 of [2, 9]) rect(1, 'front', u0, 6, u0 + 4, 8, '#141414', 0.03); rect(1, 'front', 2, 5, 13, 5, G, 0.02); set(1, 'front', 3, 6, '#4a4a55'); set(1, 'front', 10, 6, '#4a4a55'); temples(6); break;
    case 'shield': for (let u = 1; u <= 14; u++) for (let v = 5; v <= 9; v++) lens(u, v, A, 0.55); rect(1, 'front', 1, 5, 14, 5, shade(A, 0.7), 0.02); temples(6); break;
    case 'pixel': pattern(1, 'front', 1, 6, ['GGGGGGGGGGGGGG', '.WGGGG..GWGGG.', '..GGG....GGG..'].map((r) => r), { G: '#111111', W: '#ffffff' }, 0); temples(6); break;
    case 'mono': pattern(1, 'front', 9, 5, ['.GGG.', 'G...G', 'G...G', '.GGG.', '....G', '....G'], { G: '#c9a227' }); break;
    case '3d': for (let u = 2; u <= 6; u++) for (let v = 6; v <= 8; v++) lens(u, v, '#e63946', 0.6); for (let u = 9; u <= 13; u++) for (let v = 6; v <= 8; v++) lens(u, v, '#00b4d8', 0.6); rect(1, 'front', 1, 5, 14, 5, '#f5f5f5', 0.02); rect(1, 'front', 1, 9, 14, 9, '#f5f5f5', 0.02); temples(6); break;
    case 'goggles':
      for (let u = 0; u < R; u++) for (const v of [5, 9]) set(2, 'front', u, v, '#2b2d31', 0.03);
      for (let u = 1; u <= 14; u++) for (let v = 6; v <= 8; v++) lens(u, v, '#9ad0ec', 0.45);
      for (const f of ['back']) for (let u = 0; u < R; u++) for (let v = 6; v <= 8; v++) set(2, f, u, v, A, 0.03);
      for (let d = 0; d < R; d++) for (let v = 6; v <= 8; v++) side(2, d, v, A, 0.03);
      break;
    case 'vr':
      box(-7.5, -3, 8, 7.5, 2, 11.5, '#2b2d31', { amt: 0.03 });
      box(-6.5, -2, 11.5, 6.5, 1, 11.9, A, { amt: 0.02 });
      for (let d = 0; d < R; d++) for (let v = 6; v <= 7; v++) side(2, d, v, '#2b2d31', 0.03);
      for (let u = 0; u < R; u++) for (let v = 6; v <= 7; v++) set(2, 'back', u, v, '#2b2d31', 0.03);
      break;
  }

  // ---------- Kopfbedeckung ----------
  const Ad = shade(A, 0.78), Al = mix(A, '#ffffff', 0.55);
  const def = (c) => (A === DEFAULT_AVATAR.accent ? c : A);
  const capTop = (rows, col = A) => { fill(2, 'top', col, 0.04); for (let u = 0; u < R; u++) for (let v = 0; v < rows; v++) { set(2, 'front', u, v, col, 0.04); set(2, 'back', u, v, col, 0.04); } for (let d = 0; d < R; d++) for (let v = 0; v < rows; v++) side(2, d, v, col, 0.04); };
  const cone = (levels, y0, colorAt, half0) => { for (let i = 0; i < levels; i++) { const hw = half0 * (1 - i / levels); box(-hw, y0 - (i + 1) * 2, -hw, hw, y0 - i * 2, hw, colorAt(i), { amt: 0.04 }); } };
  switch (av.hat) {
    case 'cap': capTop(5); rect(2, 'front', 6, 1, 9, 3, Al, 0.02); box(-9, -4.6, 9, 9, -3.8, 15, Ad, { amt: 0.03 }); break;
    case 'capback': capTop(5); rect(2, 'back', 6, 3, 9, 4, null); box(-9, -4.6, -15, 9, -3.8, -9, Ad, { amt: 0.03 }); break;
    case 'beanie': case 'pompom':
      capTop(8, A);
      for (let u = 0; u < R; u++) for (let v = 0; v < 6; v++) if (u % 2) { set(2, 'front', u, v, Ad, 0.03); set(2, 'back', u, v, Ad, 0.03); }
      for (let u = 0; u < R; u++) for (const v of [6, 7]) { set(2, 'front', u, v, Ad, 0.04); set(2, 'back', u, v, Ad, 0.04); }
      for (let d = 0; d < R; d++) for (const v of [6, 7]) side(2, d, v, Ad, 0.04);
      if (av.hat === 'pompom') box(-2.5, -14, -2.5, 2.5, -9, 2.5, Al, { amt: 0.1 });
      break;
    case 'bucket': capTop(6); box(-12, -3.6, -12, 12, -2.8, 12, Ad, { amt: 0.03 }); box(-9.2, -4.2, -9.2, 9.2, -3.6, 9.2, shade(A, 0.6), { amt: 0.02 }); break;
    case 'beret': box(-10, -11, -9, 7, -8.5, 9, def('#2b2d31'), { amt: 0.05 }); box(-0.6, -12.2, -0.6, 0.6, -11, 0.6, def('#2b2d31')); break;
    case 'cowboy': {
      const c = def('#8b5a2b');
      box(-15, -8.8, -14, 15, -7.8, 14, shade(c, 0.9), { amt: 0.05 }); box(-15, -9.8, -14, -12, -8.8, 14, shade(c, 0.9)); box(12, -9.8, -14, 15, -8.8, 14, shade(c, 0.9));
      box(-7, -15, -7, 7, -8.8, 7, c, { amt: 0.05 }); box(-7.2, -10.4, -7.2, 7.2, -8.8, 7.2, '#3b2a1a', { amt: 0.02 }); box(-2, -15.4, -7, 2, -14.6, 7, shade(c, 0.8));
      break;
    }
    case 'tophat': { const c = def('#1d1e21'); box(-11, -9.2, -11, 11, -8, 11, c, { amt: 0.04 }); box(-7, -21, -7, 7, -9.2, 7, c, { amt: 0.05 }); box(-7.2, -11.6, -7.2, 7.2, -9.2, 7.2, A === DEFAULT_AVATAR.accent ? '#7d8085' : Ad, { amt: 0.02 }); break; }
    case 'wizard': { const c = def('#4b2c83'); box(-12, -9, -12, 12, -8, 12, c, { amt: 0.05 }); cone(7, -9, (i) => (i === 3 ? '#ffd166' : c), 7.5); box(-1, -24, 2, 1, -22, 4, '#ffd166'); break; }
    case 'party': cone(6, -8, (i) => (i % 2 ? '#ffffff' : A), 5.5); box(-1.4, -22, -1.4, 1.4, -19.6, 1.4, '#ffd166', { amt: 0.08 }); break;
    case 'pumpkin': { // Halloween
      const O = '#f28c28', Od = '#d9731a';
      box(-8.6, -15, -8.6, 8.6, -7.6, 8.6, O, { amt: 0.06 });
      for (const x of [-4.5, 0, 4.5]) { box(x - 0.6, -14.8, 8.6, x + 0.6, -7.8, 8.9, Od); box(x - 0.6, -14.8, -8.9, x + 0.6, -7.8, -8.6, Od); box(8.6, -14.8, x - 0.6, 8.9, -7.8, x + 0.6, Od); box(-8.9, -14.8, x - 0.6, -8.6, -7.8, x + 0.6, Od); }
      box(-1, -18, -1, 1, -15, 1, '#4f7a28'); box(1, -17.4, -0.5, 4, -16.4, 0.5, '#6aa33a');
      break;
    }
    case 'santa': { box(-9.5, -10, -9.5, 9.5, -7, 9.5, '#f5f5f5', { amt: 0.1 }); cone(5, -10, () => '#d62828', 8); box(4, -14, 5, 8, -10, 9, '#d62828'); box(6.5, -12, 8, 10, -8.5, 11.5, '#f5f5f5', { amt: 0.1 }); break; }
    case 'chef': box(-8.6, -11, -8.6, 8.6, -7.5, 8.6, '#f2f2f2', { amt: 0.03 }); box(-9.5, -18, -9.5, 9.5, -11, 9.5, '#ffffff', { amt: 0.05 }); box(-7, -19.5, -7, 7, -18, 7, '#ffffff', { amt: 0.05 }); break;
    case 'grad': { const c = def('#1d1e21'); box(-8.6, -12, -8.6, 8.6, -7.5, 8.6, c, { amt: 0.03 }); box(-12, -13, -12, 12, -12, 12, c, { amt: 0.03 }); box(-0.6, -13.6, -0.6, 0.6, -13, 0.6, '#ffd166'); box(-0.4, -13, -11.8, 0.4, -12.6, 0, '#ffd166'); box(-0.6, -12.6, -12.4, 0.6, -7, -11.6, '#ffd166', { amt: 0.04 }); break; }
    case 'viking': {
      capTop(6, '#9aa3ad'); for (let u = 0; u < R; u++) set(2, 'front', u, 5, '#7c858f'); for (let v = 0; v < 6; v++) set(2, 'front', 7, v, '#7c858f'); set(2, 'front', 8, 6, '#7c858f'); set(2, 'front', 7, 6, '#7c858f'); set(2, 'front', 7, 7, '#7c858f'); set(2, 'front', 8, 7, '#7c858f');
      const horn = '#efe6d2';
      for (const s of [-1, 1]) { box(s * 9, -6, -1.5, s * 12, -4, 1.5, horn); box(s * 11, -9, -1.2, s * 13, -6, 1.2, horn); box(s * 11.6, -12, -0.9, s * 13, -9, 0.9, mix(horn, '#c8b991', 0.5)); }
      break;
    }
    case 'helmet': { const Y = def('#f2c230'); capTop(4, Y); rect(2, 'front', 6, 0, 9, 3, shade(Y, 0.85)); box(-10, -4.8, -10, 10, -4, 12, shade(Y, 0.9), { amt: 0.03 }); box(-1.2, -10.4, -9, 1.2, -9, 9, shade(Y, 0.92), { amt: 0.03 }); break; }
    case 'crown': {
      const gold = '#e2b33a', gd = '#b8892a';
      box(-8.8, -10.5, -8.8, 8.8, -8, -7.4, gold); box(-8.8, -10.5, 7.4, 8.8, -8, 8.8, gold); box(-8.8, -10.5, -7.4, -7.4, -8, 7.4, gold); box(7.4, -10.5, -7.4, 8.8, -8, 7.4, gold);
      for (const [x, z] of [[-8, -8], [8, -8], [-8, 8], [8, 8], [0, 8], [0, -8], [8, 0], [-8, 0]]) box(x - 0.8, -13, z - 0.8, x + 0.8, -10.5, z + 0.8, gd);
      box(-1, -10, 8.8, 1, -8.5, 9.4, def('#e63946'), { amt: 0.02 });
      break;
    }
    case 'tiara': box(-6, -8.8, 7.6, 6, -8, 8.8, '#d8dde3'); for (const [x, h] of [[-4, 1], [-2, 1.8], [0, 2.8], [2, 1.8], [4, 1]]) box(x - 0.5, -8.8 - h, 8, x + 0.5, -8.8, 8.8, '#d8dde3'); box(-0.6, -11, 8.6, 0.6, -10, 9.2, def('#3a86ff')); break;
    case 'flowercrown': {
      const cols = [A === DEFAULT_AVATAR.accent ? '#ef476f' : A, '#ffd166', '#f4a6c8', '#ffffff', '#9d4edd'];
      let i = 0;
      for (const [x, z] of [[-8, 8], [-4, 9], [0, 9], [4, 9], [8, 8], [9, 4], [9, 0], [9, -4], [8, -8], [4, -9], [0, -9], [-4, -9], [-8, -8], [-9, -4], [-9, 0], [-9, 4]]) {
        box(x - 1, -10, z - 1, x + 1, -8, z + 1, cols[i++ % cols.length], { amt: 0.04 });
      }
      break;
    }
    case 'halo': { const y0 = -15, y1 = -14, g = '#ffe066'; box(-7, y0, -7, 7, y1, -5.6, g); box(-7, y0, 5.6, 7, y1, 7, g); box(-7, y0, -5.6, -5.6, y1, 5.6, g); box(5.6, y0, -5.6, 7, y1, 5.6, g); break; }
    case 'devil': for (const s of [-1, 1]) { box(s * 4 - 1.5, -10, 2, s * 4 + 1.5, -8, 5, '#b51d1d'); box(s * 4.5 - 1, -12, 2.5, s * 4.5 + 1, -10, 4.5, '#b51d1d'); box(s * 5 - 0.5, -13.5, 3, s * 5 + 0.5, -12, 4, '#8f1414'); } break;
    case 'catears': for (const s of [-1, 1]) { const c = H; box(s * 5 - 3, -10, -1.5, s * 5 + 3, -8, 1.5, c); box(s * 5 - 2, -12, -1.2, s * 5 + 2, -10, 1.2, c); box(s * 5 - 1, -14, -0.9, s * 5 + 1, -12, 0.9, c); box(s * 5 - 1.5, -11, 1.5, s * 5 + 1.5, -8.5, 1.8, '#f4a6c8'); } break;
    case 'bunnyears': for (const s of [-1, 1]) { box(s * 4 - 1.8, -20, -1.2, s * 4 + 1.8, -8, 1.2, '#f5f5f5', { amt: 0.04 }); box(s * 4 - 0.9, -18.5, 1.2, s * 4 + 0.9, -9.5, 1.5, '#f4a6c8'); } box(-9, -9, -1, 9, -8, 1, def('#f4a6c8')); break;
    case 'propeller': {
      for (let u = 0; u < R; u++) for (let v = 0; v < R; v++) set(2, 'top', u, v, ['#e63946', '#ffd166', '#3a86ff', '#06d6a0'][((u < 8) ? 0 : 1) + ((v < 8) ? 0 : 2)], 0.03);
      for (let u = 0; u < R; u++) for (let v = 0; v < 4; v++) { set(2, 'front', u, v, u < 8 ? '#e63946' : '#ffd166'); set(2, 'back', u, v, u < 8 ? '#ffd166' : '#e63946'); }
      for (let d = 0; d < R; d++) for (let v = 0; v < 4; v++) side(2, d, v, d < 8 ? '#3a86ff' : '#06d6a0');
      box(-0.6, -12, -0.6, 0.6, -9, 0.6, '#9aa3ad'); box(-7, -12.6, -1, 7, -12, 1, def('#e63946')); box(-1, -12.6, -7, 1, -12, 7, '#3a86ff');
      break;
    }
    case 'headset': case 'headphones': {
      const dark = '#2b2d31', cup = av.hat === 'headphones' ? A : dark;
      for (let u = 0; u < R; u++) for (const v of [6, 7, 8]) set(2, 'top', u, v, dark, 0.04);
      for (let v = 0; v < 6; v++) for (const d of [6, 7, 8]) side(2, d, v, dark, 0.04);
      const r = av.hat === 'headphones' ? 4.2 : 3.4;
      box(8.4, 1 - r, -r, 11, 1 + r, r, cup, { amt: 0.04 }); box(-11, 1 - r, -r, -8.4, 1 + r, r, cup, { amt: 0.04 });
      if (av.hat === 'headphones') { box(10.6, -1.5, -1.5, 11.6, 3.5, 1.5, Ad); box(-11.6, -1.5, -1.5, -10.6, 3.5, 1.5, Ad); }
      else { box(8.6, 3, 1, 9.6, 4, 9, dark, { amt: 0.02 }); box(6, 2.6, 8.6, 9.6, 4.4, 10.2, A, { amt: 0.02 }); }
      break;
    }
    case 'band': for (let u = 0; u < R; u++) for (const v of [2, 3]) { set(2, 'front', u, v, A, 0.04); set(2, 'back', u, v, A, 0.04); } for (let d = 0; d < R; d++) for (const v of [2, 3]) side(2, d, v, A, 0.04); for (let u = 0; u < R; u += 3) set(2, 'front', u, 2, Al); break;
    case 'bandana':
      capTop(5, A);
      for (let u = 0; u < R; u++) for (let v = 0; v < 5; v++) if ((u + v * 2) % 5 === 0) { set(2, 'front', u, v, '#ffffff'); set(2, 'back', u, v, '#ffffff'); }
      box(-2, -6, -11, 2, -2, -9, Ad, { amt: 0.04 }); box(-3, -2, -10.5, -1, 3, -9.5, A); box(1, -2, -10.5, 3, 2, -9.5, A);
      break;
    case 'bow': box(2.2, -10.6, -1.6, 4.6, -8, 1.2, Ad, { amt: 0.03 }); box(-1.6, -12, -2.2, 2.2, -8, 1.6, A, { amt: 0.04 }); box(4.6, -12, -2.2, 8.4, -8, 1.6, A, { amt: 0.04 }); break;
    case 'flower':
      box(8.1, -7, -2.4, 9.2, -2.2, 2.4, A); box(8.1, -8.2, -1.2, 9.2, -1, 1.2, A);
      box(8.8, -5.6, -1, 10, -3.6, 1, '#ffd166', { amt: 0.03 });
      break;
  }

  // ---------- Freihand-Pixel ----------
  const paint = av.paint;
  if (paint && typeof paint === 'object') {
    for (const [layer, faces] of Object.entries(paint)) {
      const tgt = layer === 'over' ? over : layer === 'base' ? base : null;
      if (!tgt || !faces) continue;
      for (const [f, px] of Object.entries(faces)) {
        if (!tgt[f] || !px) continue;
        // "*" = ganze Seite in einer Farbe (zuerst), einzelne Pixel überschreiben sie
        if (typeof px['*'] === 'string') tgt[f].fill(px['*'] === '' ? null : px['*']);
        for (const [i, c] of Object.entries(px)) {
          if (i === '*') continue;
          const n = +i;
          if (n >= 0 && n < R * R) tgt[f][n] = c === '' ? null : c;
        }
      }
    }
  }
  return { base, over, boxes };
}

// ---------- Geometrie ----------
const FACE_GEO = {
  front: { o: [-1, -1, 1], u: [1, 0, 0], v: [0, 1, 0], n: [0, 0, 1], light: 1.0 },
  back: { o: [1, -1, -1], u: [-1, 0, 0], v: [0, 1, 0], n: [0, 0, -1], light: 0.72 },
  right: { o: [1, -1, 1], u: [0, 0, -1], v: [0, 1, 0], n: [1, 0, 0], light: 0.82 },
  left: { o: [-1, -1, -1], u: [0, 0, 1], v: [0, 1, 0], n: [-1, 0, 0], light: 0.82 },
  top: { o: [-1, -1, -1], u: [1, 0, 0], v: [0, 0, 1], n: [0, -1, 0], light: 1.1 },
  bottom: { o: [-1, 1, 1], u: [1, 0, 0], v: [0, 0, -1], n: [0, 1, 0], light: 0.6 },
};
const TILE = 4; // Kachelgröße (Texel) für die Tiefensortierung

function makeCanvas(w, h) {
  if (typeof OffscreenCanvas !== 'undefined') return new OffscreenCanvas(w, h);
  const c = document.createElement('canvas'); c.width = w; c.height = h; return c;
}

// Pixelfeld → Canvas (mit Helligkeitsfaktor)
function texCanvas(px, w, h, light) {
  const c = makeCanvas(w, h);
  const ctx = c.getContext('2d');
  const img = ctx.createImageData(w, h);
  for (let i = 0; i < w * h; i++) {
    const col = px[i];
    if (!col) continue;
    const [r, g, b] = rgb(col);
    img.data[i * 4] = clamp(r * light, 0, 255); img.data[i * 4 + 1] = clamp(g * light, 0, 255); img.data[i * 4 + 2] = clamp(b * light, 0, 255); img.data[i * 4 + 3] = 255;
  }
  ctx.putImageData(img, 0, 0);
  return c;
}

// Flächen: {o, U, V, w, h, n, canvases: {lit, inner}, layer, face, cull}
const geoCache = new Map();
function geometry(av) {
  const key = JSON.stringify(av || null);
  let g = geoCache.get(key);
  if (g) return g;
  const tex = buildTexture(av);
  const faces = [];
  for (const [layerName, layer, half] of [['base', tex.base, H8], ['over', tex.over, H8 * 1.125]]) {
    for (const f of FACES) {
      const G = FACE_GEO[f];
      const px = layer[f];
      // leere Überzug-Seiten bleiben drin: sie werden zwar unsichtbar gezeichnet, sind aber zum Malen anklickbar
      const step = (2 * half) / R;
      faces.push({
        o: G.o.map((x) => x * half), U: G.u.map((x) => x * step), V: G.v.map((x) => x * step), w: R, h: R, n: G.n,
        lit: texCanvas(px, R, R, G.light), inner: layerName === 'over' ? texCanvas(px, R, R, G.light * 0.55) : null,
        cull: layerName === 'base', layer: layerName, face: f,
      });
    }
  }
  for (const b of tex.boxes) {
    const lo = [b.x0, b.y0, b.z0], hi = [b.x1, b.y1, b.z1];
    const sides = [
      { n: [0, 0, 1], light: 1.0, o: [b.x0, b.y0, b.z1], u: 0, v: 1 }, { n: [0, 0, -1], light: 0.72, o: [b.x1, b.y0, b.z0], u: 0, v: 1, flipU: true },
      { n: [1, 0, 0], light: 0.82, o: [b.x1, b.y0, b.z1], u: 2, v: 1, flipU: true }, { n: [-1, 0, 0], light: 0.82, o: [b.x0, b.y0, b.z0], u: 2, v: 1 },
      { n: [0, -1, 0], light: 1.1, o: [b.x0, b.y0, b.z0], u: 0, v: 2 }, { n: [0, 1, 0], light: 0.6, o: [b.x0, b.y1, b.z1], u: 0, v: 2, flipV: true },
    ];
    for (const s of sides) {
      const lu = hi[s.u] - lo[s.u], lv = hi[s.v] - lo[s.v];
      const w = Math.max(1, Math.round(lu)), h = Math.max(1, Math.round(lv));
      const U = [0, 0, 0], V = [0, 0, 0];
      U[s.u] = (s.flipU ? -lu : lu) / w; V[s.v] = (s.flipV ? -lv : lv) / h;
      const px = new Array(w * h);
      const salt = Math.round((b.x0 + b.y0 * 3 + b.z0 * 7) * 10) + s.u * 31 + s.v * 17;
      for (let j = 0; j < h; j++) for (let i = 0; i < w; i++) {
        const c = b.pat ? b.pat(i, j) : b.c;
        px[j * w + i] = b.amt ? shade(c, 1 - b.amt + hash(9, salt, i, j) * b.amt * 2) : c;
      }
      faces.push({ o: s.o, U, V, w, h, n: s.n, lit: texCanvas(px, w, h, s.light), inner: null, cull: true, layer: 'box' });
    }
  }
  // Ausdehnung für "fit": Radius um die senkrechte Achse und Höhenbereich
  let rxz = 0, ymin = Infinity, ymax = -Infinity;
  for (const F of faces) for (const [a, b] of [[0, 0], [F.w, 0], [0, F.h], [F.w, F.h]]) {
    const x = F.o[0] + F.U[0] * a + F.V[0] * b, y = F.o[1] + F.U[1] * a + F.V[1] * b, z = F.o[2] + F.U[2] * a + F.V[2] * b;
    rxz = Math.max(rxz, Math.hypot(x, z)); ymin = Math.min(ymin, y); ymax = Math.max(ymax, y);
  }
  g = { faces, tex, ext: { rxz, ymin, ymax } };
  if (geoCache.size > 250) geoCache.delete(geoCache.keys().next().value);
  geoCache.set(key, g);
  return g;
}

export function invalidate() { geoCache.clear(); }

// ---------- Zeichnen ----------
// opts: yaw, pitch (Grad), zoom, lift, bg, highlight {layer, face, idx}
// Rückgabe: Szene für pick()
export function drawHead(canvas, av, opts = {}) {
  const ctx = canvas.getContext('2d');
  const dpr = opts.dpr || window.devicePixelRatio || 1;
  const w = opts.width || canvas.clientWidth || canvas.width / dpr;
  const h = opts.height || canvas.clientHeight || canvas.height / dpr;
  if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) { canvas.width = Math.round(w * dpr); canvas.height = Math.round(h * dpr); }
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);
  if (opts.bg) { ctx.fillStyle = opts.bg; ctx.fillRect(0, 0, w, h); }
  ctx.imageSmoothingEnabled = false;
  const yaw = ((opts.yaw ?? -32) * Math.PI) / 180, pitch = ((opts.pitch ?? 22) * Math.PI) / 180;
  const cy = Math.cos(yaw), sy = Math.sin(yaw), cp = Math.cos(pitch), sp = Math.sin(pitch);
  const { faces, ext } = geometry(av);
  let scale = (Math.min(w, h) / 35) * (opts.zoom || 1);
  let ox = w / 2, oy = h / 2 + scale * (opts.lift ?? 1.2);
  if (opts.fit) {
    // drehunabhängig einpassen (Hüte, Haare, Ohren passen immer ins Bild, Größe bleibt beim Drehen gleich)
    const y0 = ext.ymin * cp - ext.rxz * Math.abs(sp), y1 = ext.ymax * cp + ext.rxz * Math.abs(sp);
    scale = Math.min((w * 0.92) / (2 * ext.rxz), (h * 0.92) / (y1 - y0), Math.min(w, h) / 26) * (opts.zoom || 1);
    oy = h / 2 - ((y0 + y1) / 2) * scale;
  }
  const rot = (p) => {
    const x = p[0] * cy + p[2] * sy, z0 = -p[0] * sy + p[2] * cy;
    return [x, p[1] * cp + z0 * sp, -p[1] * sp + z0 * cp];
  };
  const items = [];
  for (const F of faces) {
    const n = rot(F.n);
    const facing = n[2] > 0.001;
    if (F.cull && !facing) continue;
    const o = rot(F.o), U = rot(F.U), V = rot(F.V);
    const canvasT = facing ? F.lit : F.inner;
    if (!canvasT) continue;
    const m = [U[0] * scale, U[1] * scale, V[0] * scale, V[1] * scale, ox + o[0] * scale, oy + o[1] * scale];
    // in Kacheln zerlegen → feinere Tiefensortierung
    for (let tv = 0; tv < F.h; tv += TILE) for (let tu = 0; tu < F.w; tu += TILE) {
      const tw = Math.min(TILE, F.w - tu), th = Math.min(TILE, F.h - tv);
      const cu = tu + tw / 2, cv = tv + th / 2;
      const depth = o[2] + U[2] * cu + V[2] * cv + (F.layer === 'over' && facing ? 0.05 : 0) + (F.layer === 'box' ? 0.01 : 0);
      items.push({ F, m, tu, tv, tw, th, depth, img: canvasT, facing });
    }
  }
  items.sort((a, b) => a.depth - b.depth);
  const pad = 0.1; // Überlappung gegen Haarlinien zwischen Kacheln
  for (const it of items) {
    const m = it.m;
    ctx.setTransform(dpr * m[0], dpr * m[1], dpr * m[2], dpr * m[3], dpr * m[4], dpr * m[5]);
    ctx.drawImage(it.img, it.tu, it.tv, it.tw, it.th, it.tu - pad, it.tv - pad, it.tw + 2 * pad, it.th + 2 * pad);
  }
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  const scene = { items, dpr };
  if (opts.highlight) {
    const hl = opts.highlight;
    const it = items.find((x) => x.F.layer === hl.layer && x.F.face === hl.face && x.facing);
    if (it) {
      const u = hl.idx % R, v = (hl.idx / R) | 0, m = it.m;
      ctx.setTransform(dpr * m[0], dpr * m[1], dpr * m[2], dpr * m[3], dpr * m[4], dpr * m[5]);
      const det = Math.abs(m[0] * m[3] - m[1] * m[2]);
      ctx.lineWidth = 1.6 / Math.sqrt(det || 1);
      ctx.strokeStyle = 'rgba(0,0,0,.85)';
      ctx.strokeRect(u, v, 1, 1);
      ctx.strokeStyle = 'rgba(255,255,255,.9)';
      ctx.lineWidth *= 0.5;
      ctx.strokeRect(u, v, 1, 1);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }
  }
  return scene;
}

// Treffer bestimmen: oberste Kachel der gewünschten Ebene unter dem Mauszeiger.
// layer 'base' malt "durch" den Überzug hindurch; 'over' trifft auch leere (durchsichtige) Pixel.
export function pick(scene, x, y, layer) {
  if (!scene) return null;
  for (let i = scene.items.length - 1; i >= 0; i--) {
    const it = scene.items[i];
    if (it.F.layer !== layer || !it.facing) continue;
    const [a, b, c, d, e, f] = it.m;
    const det = a * d - b * c;
    if (Math.abs(det) < 1e-6) continue;
    const px = x - e, py = y - f;
    const u = (d * px - c * py) / det, v = (-b * px + a * py) / det;
    if (u >= it.tu && u < it.tu + it.tw && v >= it.tv && v < it.tv + it.th) {
      const iu = Math.floor(u), iv = Math.floor(v);
      return { layer, face: it.F.face, idx: iv * R + iu };
    }
  }
  return null;
}

// ---------- Vorschaubilder (gecacht) ----------
const thumbCache = new Map();
export function thumbURL(av, size = 40, opts = {}) {
  const key = size + '|' + JSON.stringify(opts) + '|' + JSON.stringify(av || null);
  let u = thumbCache.get(key);
  if (u) return u;
  const c = document.createElement('canvas');
  drawHead(c, av, { width: size, height: size, dpr: Math.min(2, window.devicePixelRatio || 1), ...opts });
  u = c.toDataURL('image/png');
  if (thumbCache.size > 3000) thumbCache.delete(thumbCache.keys().next().value);
  thumbCache.set(key, u);
  return u;
}

export function randomAvatar() {
  const pick1 = (a) => a[Math.floor(Math.random() * a.length)];
  // Zufall ohne „Ohne“-Frisur und ohne gesichtslose Varianten
  const o = (k) => pick1(OPTIONS[k].filter(([v]) => v !== 'bare' && !((k === 'eyeStyle' || k === 'mouthStyle') && v === 'none')))[0];
  const r = (p) => Math.random() < p;
  return {
    ...DEFAULT_AVATAR,
    skin: pick1(SKIN_COLORS.slice(0, 14)), hair: pick1(HAIR_COLORS), eyes: pick1(EYE_COLORS), accent: pick1(ACCENT_COLORS), glassesColor: pick1(GLASSES_COLORS),
    hairStyle: o('hairStyle'), eyeStyle: r(0.5) ? 'classic' : o('eyeStyle'), browStyle: o('browStyle'), mouthStyle: o('mouthStyle'), noseStyle: o('noseStyle'),
    beardStyle: r(0.3) ? o('beardStyle') : 'none', glassesStyle: r(0.35) ? o('glassesStyle') : 'none', hat: r(0.35) ? o('hat') : 'none', ears: r(0.4) ? o('ears') : 'none',
    extras: OPTIONS.extras.filter(() => r(0.1)).map((x) => x[0]), texture: true,
    ...(r(0.15) ? { highlight: pick1(HAIR_COLORS) } : {}), ...(r(0.15) ? { lips: pick1(LIP_COLORS) } : {}), ...(r(0.05) ? { eyes2: pick1(EYE_COLORS) } : {}),
  };
}

// Einfarbiger Block (z. B. für Einrichtungen wie „Schule xy“): kein Gesicht, keine Haare, glatt
export function blockAvatar(color = '#3b7dd8') {
  return { ...DEFAULT_AVATAR, skin: color, hair: color, hairStyle: 'bare', eyeStyle: 'none', browStyle: 'none', mouthStyle: 'none', noseStyle: 'none',
    beardStyle: 'none', glassesStyle: 'none', hat: 'none', ears: 'none', extras: [], texture: false };
}

// ---------- Gemalte Pixel: kompakt speichern ----------
// Mit dem Avatar (av) wird gegen die erzeugte Textur verglichen:
//  - Pixel, die genau dem Original entsprechen, entfallen (z. B. „durchsichtig“ auf ohnehin leerem Überzug).
//  - Ist eine Seite effektiv einheitlich (bemalte Pixel + unbemalte Pixel mit gleichem Original), steht
//    die Farbe einmal als "*" da, nur Abweichungen einzeln. Beispiel: alle Haar-Pixel einer Seite
//    durchsichtig gemalt → "over": { "back": { "*": "" } }.
export function compactPaint(paint, av) {
  if (!paint || typeof paint !== 'object') return undefined;
  const gen = av ? buildTexture({ ...av, paint: undefined }) : null;
  const out = {};
  for (const [layer, faces] of Object.entries(paint)) {
    const gl = gen ? (layer === 'over' ? gen.over : gen.base) : null;
    for (const [f, px0] of Object.entries(faces || {})) {
      const px = expandFace(px0);
      const g = gl && gl[f] ? gl[f].map((c) => c ?? '') : null; // Original: '' = leer/durchsichtig
      // 1) einzelne Pixel ohne Wirkung weglassen
      const sparse = {};
      for (const [i, c] of Object.entries(px)) if (!g || g[i] !== c) sparse[i] = c;
      // 2) einheitliche Fläche suchen
      let best = null;
      const eff = [];
      let forced = null, ok = true;
      for (let i = 0; i < R * R; i++) {
        if (i in px) { eff.push(px[i]); continue; }
        if (!g) { ok = false; break; }           // ohne Original nur bei vollständig bemalter Seite
        eff.push(g[i]);
        if (forced === null) forced = g[i];
        else if (forced !== g[i]) { ok = false; break; }
      }
      if (ok) {
        if (forced !== null) best = forced;
        else {
          const cnt = new Map();
          for (const c of eff) cnt.set(c, (cnt.get(c) || 0) + 1);
          let bn = 0;
          for (const [c, n] of cnt) if (n > bn) { best = c; bn = n; }
        }
      }
      let face = sparse;
      if (best !== null) {
        const filled = { '*': best };
        for (let i = 0; i < R * R; i++) if (eff[i] !== best) filled[i] = eff[i];
        if (Object.keys(filled).length < Object.keys(sparse).length) face = filled;
      }
      if (Object.keys(face).length) (out[layer] = out[layer] || {})[f] = face;
    }
  }
  return Object.keys(out).length ? out : undefined;
}

function expandFace(px) {
  const out = {};
  if (!px || typeof px !== 'object') return out;
  if (typeof px['*'] === 'string') for (let i = 0; i < R * R; i++) out[i] = px['*'];
  for (const [i, c] of Object.entries(px)) if (i !== '*') out[i] = c;
  return out;
}

export function expandPaint(paint) {
  const out = {};
  for (const [layer, faces] of Object.entries(paint || {})) {
    for (const [f, px] of Object.entries(faces || {})) {
      const e = expandFace(px);
      if (Object.keys(e).length) (out[layer] = out[layer] || {})[f] = e;
    }
  }
  return out;
}
