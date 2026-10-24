package main

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSON verlangt application/json (erzwingt CORS-Preflight bei fremden Seiten) und begrenzt die Größe.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if ct != "application/json" {
		jsonError(w, http.StatusUnsupportedMediaType, "Content-Type application/json erwartet")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			jsonError(w, http.StatusRequestEntityTooLarge, "Anfrage zu groß")
		} else {
			jsonError(w, http.StatusBadRequest, "Ungültige Anfrage: "+err.Error())
		}
		return false
	}
	if _, err := dec.Token(); err != io.EOF {
		jsonError(w, http.StatusBadRequest, "Ungültige Anfrage: zusätzliche Daten")
		return false
	}
	return true
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// cleanText entfernt Steuerzeichen, fasst Leerraum zusammen und prüft die Länge.
func cleanText(s string, max int) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}
	var b strings.Builder
	space := false
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if unicode.IsControl(r) || r == '\u200b' || r == '\ufeff' {
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	out := b.String()
	return out, utf8.RuneCountInString(out) <= max
}

// Mehrzeiliger Text (Notizen): Zeilenumbrüche bleiben erhalten.
func cleanMultiline(s string, max int) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if r == '\n' || (!unicode.IsControl(r) && r != '\u200b') {
			b.WriteRune(r)
		}
	}
	out := b.String()
	return out, utf8.RuneCountInString(out) <= max
}
