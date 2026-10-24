package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
)

// Avatar: kleiner Pixel-Kopf im Minecraft-Stil. Version 1: 8×8 je Seite, Version 2: 16×16 je Seite.
// Gezeichnet wird im Browser, der Server prüft nur Struktur und Wertebereiche.
type Avatar struct {
	V            int      `json:"v"`
	Skin         string   `json:"skin"`
	Hair         string   `json:"hair"`
	Eyes         string   `json:"eyes"`
	Accent       string   `json:"accent"`
	GlassesColor string   `json:"glassesColor"`
	BeardColor   string   `json:"beardColor,omitempty"`
	Eyes2        string   `json:"eyes2,omitempty"`     // zweite Augenfarbe (Heterochromie)
	Lips         string   `json:"lips,omitempty"`      // Lippenfarbe
	Highlight    string   `json:"highlight,omitempty"` // Strähnen
	Ears         string   `json:"ears,omitempty"`
	HairStyle    string   `json:"hairStyle"`
	EyeStyle     string   `json:"eyeStyle"`
	BrowStyle    string   `json:"browStyle"`
	MouthStyle   string   `json:"mouthStyle"`
	NoseStyle    string   `json:"noseStyle"`
	BeardStyle   string   `json:"beardStyle"`
	GlassesStyle string   `json:"glassesStyle"`
	Hat          string   `json:"hat"`
	Extras       []string `json:"extras"`
	Texture      bool     `json:"texture"`
	// Freihand-Pixel: layer ("base"/"over") → Seite → Index 0..255 (v1: 0..63) → Farbe ("" = durchsichtig, nur "over").
	// v2: Schlüssel "*" = ganze Seite in dieser Farbe; einzelne Indizes überschreiben sie.
	Paint map[string]map[string]map[string]string `json:"paint,omitempty"`
}

var (
	reHex   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	reStyle = regexp.MustCompile(`^[a-z0-9\-]{1,24}$`)
	faces   = map[string]bool{"front": true, "back": true, "left": true, "right": true, "top": true, "bottom": true}
)

const maxAvatarBytes = 160 << 10

func validateAvatar(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	if len(raw) > maxAvatarBytes {
		return "", errors.New("Avatar zu groß")
	}
	var a Avatar
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return "", fmt.Errorf("Avatar ungültig: %w", err)
	}
	if a.V != 1 && a.V != 2 {
		return "", errors.New("Avatar: unbekannte Version")
	}
	perFace := 64 // Pixel je Seite
	if a.V == 2 {
		perFace = 256
	}
	for name, c := range map[string]string{"skin": a.Skin, "hair": a.Hair, "eyes": a.Eyes, "accent": a.Accent, "glassesColor": a.GlassesColor} {
		if !reHex.MatchString(c) {
			return "", fmt.Errorf("Avatar: Farbe %s ungültig", name)
		}
	}
	for name, c := range map[string]string{"beardColor": a.BeardColor, "eyes2": a.Eyes2, "lips": a.Lips, "highlight": a.Highlight} {
		if c != "" && !reHex.MatchString(c) {
			return "", fmt.Errorf("Avatar: Farbe %s ungültig", name)
		}
	}
	if a.Ears != "" && !reStyle.MatchString(a.Ears) {
		return "", errors.New("Avatar: ungültige Option")
	}
	for _, s := range []string{a.HairStyle, a.EyeStyle, a.BrowStyle, a.MouthStyle, a.NoseStyle, a.BeardStyle, a.GlassesStyle, a.Hat} {
		if !reStyle.MatchString(s) {
			return "", errors.New("Avatar: ungültige Option")
		}
	}
	if len(a.Extras) > 24 {
		return "", errors.New("Avatar: zu viele Details")
	}
	for _, s := range a.Extras {
		if !reStyle.MatchString(s) {
			return "", errors.New("Avatar: ungültiges Detail")
		}
	}
	total := 0
	for layer, fm := range a.Paint {
		if layer != "base" && layer != "over" {
			return "", errors.New("Avatar: unbekannte Ebene")
		}
		for face, px := range fm {
			if !faces[face] {
				return "", errors.New("Avatar: unbekannte Seite")
			}
			for idx, col := range px {
				if idx != "*" || a.V != 2 {
					n, err := strconv.Atoi(idx)
					if err != nil || n < 0 || n >= perFace || strconv.Itoa(n) != idx {
						return "", errors.New("Avatar: ungültige Pixelposition")
					}
				}
				if !(reHex.MatchString(col) || (col == "" && layer == "over")) {
					return "", errors.New("Avatar: ungültige Pixelfarbe")
				}
				total++
			}
		}
	}
	if total > 2*6*(perFace+1) {
		return "", errors.New("Avatar: zu viele Pixel")
	}
	if a.V == 2 {
		compactPaint(a.Paint, perFace)
	}
	out, err := json.Marshal(a)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// compactPaint: Datensparsam speichern. Eine vollständig bemalte Seite wird zu "*" (häufigste Farbe)
// plus den abweichenden Pixeln; Pixel, die ohnehin "*" entsprechen, entfallen.
func compactPaint(p map[string]map[string]map[string]string, perFace int) {
	for layer, fm := range p {
		for face, px := range fm {
			fill, hasFill := px["*"]
			if !hasFill && len(px) == perFace {
				cnt := map[string]int{}
				for _, c := range px {
					cnt[c]++
				}
				best, bn := "", 0
				for c, n := range cnt {
					if n > bn || (n == bn && c < best) { // deterministisch
						best, bn = c, n
					}
				}
				if bn > 1 {
					fill, hasFill = best, true
					px["*"] = best
				}
			}
			if hasFill {
				for idx, c := range px {
					if idx != "*" && c == fill {
						delete(px, idx)
					}
				}
			}
			if len(px) == 0 {
				delete(fm, face)
			}
		}
		if len(fm) == 0 {
			delete(p, layer)
		}
	}
}

// compactStoredAvatars bringt vorhandene Avatare einmalig in die kompakte Form.
func compactStoredAvatars(ctx context.Context, st Store) {
	recs, err := st.ListEmployees(ctx, true)
	if err != nil {
		return
	}
	n, saved := 0, 0
	for _, e := range recs {
		if len(e.Avatar) == 0 || string(e.Avatar) == "null" {
			continue
		}
		out, err := validateAvatar(e.Avatar)
		if err != nil || len(out) >= len(e.Avatar) {
			continue
		}
		if st.SetAvatar(ctx, e.ID, json.RawMessage(out)) == nil {
			n++
			saved += len(e.Avatar) - len(out)
		}
	}
	if n > 0 {
		log.Printf("Avatare verdichtet: %d Einträge, %d KB gespart", n, saved/1024)
	}
}
