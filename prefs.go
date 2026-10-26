package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Persönliche Einstellungen: Favoriten (Kurzwahl) und Extras (Spaß-Funktionen).
// Extras sind administrativ für alle abschaltbar (Setting "fun") und pro Person aktivierbar (Standard: aus).

const (
	maxFavorites = 10
	settingFun   = "fun"
)

var funKeys = []string{"village", "quiz", "terminal", "seasonal", "badges"}

type UserPrefs struct {
	Favorites []string        `json:"favorites,omitempty"` // "e12" (Eintrag) oder "p3" (eigener privater Kontakt)
	Fun       map[string]bool `json:"fun,omitempty"`
	FeedHash  string          `json:"feedHash,omitempty"` // SHA-256 des Favoriten-Codes (Windows-Symbol)
}

func (a *App) loadPrefs(ctx context.Context, user string) UserPrefs {
	var p UserPrefs
	if user == "" {
		return p
	}
	if v, err := a.st().GetPrefs(ctx, user); err == nil && v != "" {
		_ = json.Unmarshal([]byte(v), &p)
	}
	return p
}

func (a *App) savePrefs(ctx context.Context, user string, p UserPrefs) error {
	b, _ := json.Marshal(p)
	return a.st().SetPrefs(ctx, user, string(b))
}

// funGlobal: welche Extras die Administration freigegeben hat (Standard: alle)
func (a *App) funGlobal(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	for _, k := range funKeys {
		out[k] = true
	}
	if v, err := a.st().GetSetting(ctx, settingFun); err == nil && v != "" {
		var m map[string]bool
		if json.Unmarshal([]byte(v), &m) == nil {
			for _, k := range funKeys {
				if b, ok := m[k]; ok {
					out[k] = b
				}
			}
		}
	}
	return out
}

// prefsJSON: für die Oberfläche (ohne Feed-Hash)
func (p UserPrefs) public() map[string]any {
	fav := p.Favorites
	if fav == nil {
		fav = []string{}
	}
	fun := map[string]bool{}
	for _, k := range funKeys {
		fun[k] = p.Fun[k]
	}
	return map[string]any{"favorites": fav, "fun": fun, "hasFeed": p.FeedHash != ""}
}

var reFavKey = regexp.MustCompile(`^[ep][1-9][0-9]{0,17}$`)

func (a *App) handleSetPrefs(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in struct {
		Favorites *[]string       `json:"favorites"`
		Fun       map[string]bool `json:"fun"`
	}
	if !decodeJSON(w, r, &in, 8<<10) {
		return
	}
	ctx := r.Context()
	p := a.loadPrefs(ctx, s.Username)
	if in.Favorites != nil {
		fav, err := a.cleanFavorites(ctx, s, *in.Favorites)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		p.Favorites = fav
	}
	if in.Fun != nil {
		if p.Fun == nil {
			p.Fun = map[string]bool{}
		}
		for _, k := range funKeys {
			if v, ok := in.Fun[k]; ok {
				p.Fun[k] = v
			}
		}
	}
	if err := a.savePrefs(ctx, s.Username, p); err != nil {
		dbFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "prefs": p.public()})
}

func (a *App) cleanFavorites(ctx context.Context, s *Session, keys []string) ([]string, error) {
	if len(keys) > maxFavorites {
		return nil, fmt.Errorf("Höchstens %d Favoriten", maxFavorites)
	}
	var own map[int64]bool
	seen := map[string]bool{}
	var out []string
	for _, k := range keys {
		if !reFavKey.MatchString(k) || seen[k] {
			continue
		}
		id, _ := strconv.ParseInt(k[1:], 10, 64)
		if k[0] == 'e' {
			if _, err := a.st().GetEmployee(ctx, id); err != nil {
				continue // gelöschte Einträge stillschweigend entfernen
			}
		} else {
			if own == nil {
				own = map[int64]bool{}
				list, err := a.st().ListPrivate(ctx, s.Username)
				if err != nil {
					return nil, err
				}
				for _, p := range list {
					own[p.ID] = true
				}
			}
			if !own[id] {
				continue
			}
		}
		seen[k] = true
		out = append(out, k)
	}
	return out, nil
}

// ---------- Favoriten-Code für das Windows-Symbol ----------

func hashFeedToken(t string) string {
	h := sha256.Sum256([]byte("tb-feed:" + t))
	return hex.EncodeToString(h[:])
}

func (a *App) handleCreateFeedToken(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	ctx := r.Context()
	p := a.loadPrefs(ctx, s.Username)
	tok := "tbf_" + randomToken(24)
	p.FeedHash = hashFeedToken(tok)
	if err := a.savePrefs(ctx, s.Username, p); err != nil {
		dbFail(w, err)
		return
	}
	a.audit(r, s.Username, "feed_token", "", "neu erzeugt")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": tok})
}

func (a *App) handleDeleteFeedToken(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	ctx := r.Context()
	p := a.loadPrefs(ctx, s.Username)
	p.FeedHash = ""
	if err := a.savePrefs(ctx, s.Username, p); err != nil {
		dbFail(w, err)
		return
	}
	a.audit(r, s.Username, "feed_token", "", "widerrufen")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type feedItem struct {
	Name    string `json:"name"`
	Phone   string `json:"phone,omitempty"`
	Mobile  string `json:"mobile,omitempty"`
	Email   string `json:"email,omitempty"`
	Amt     string `json:"amt,omitempty"`
	Via     string `json:"via,omitempty"`     // bevorzugter Kontaktweg (Name)
	ViaTel  string `json:"viaTel,omitempty"`  // dessen Hotline
	ViaURL  string `json:"viaUrl,omitempty"`  // dessen Ticketsystem
	Private bool   `json:"private,omitempty"` // eigener privater Kontakt
	Away    string `json:"away,omitempty"`    // abwesend bis
}

// GET /api/favorites/feed?t=… – ohne Sitzung, nur mit Favoriten-Code; liefert nur die Favoriten
func (a *App) handleFavoritesFeed(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("t")
	ip := a.clientIP(r)
	if blocked, _ := a.limiter.blocked(ip, ""); blocked {
		jsonError(w, http.StatusTooManyRequests, "Zu viele Versuche – bitte später erneut")
		return
	}
	if !strings.HasPrefix(tok, "tbf_") || len(tok) > 100 {
		a.limiter.fail(ip, "")
		time.Sleep(300 * time.Millisecond)
		jsonError(w, http.StatusNotFound, "Unbekannter Code")
		return
	}
	ctx := r.Context()
	all, err := a.st().ListPrefs(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	want := hashFeedToken(tok)
	user := ""
	var p UserPrefs
	for u, v := range all {
		var x UserPrefs
		if json.Unmarshal([]byte(v), &x) == nil && x.FeedHash != "" && subtle.ConstantTimeCompare([]byte(x.FeedHash), []byte(want)) == 1 {
			user, p = u, x
		}
	}
	if user == "" {
		a.limiter.fail(ip, "")
		time.Sleep(300 * time.Millisecond)
		jsonError(w, http.StatusNotFound, "Unbekannter Code")
		return
	}
	chans, _ := a.st().ListChannels(ctx)
	chByID := map[int64]ContactChannel{}
	for _, c := range chans {
		chByID[c.ID] = c
	}
	var priv []PrivateContact
	today := a.cfg.today()
	items := []feedItem{}
	for _, k := range p.Favorites {
		id, _ := strconv.ParseInt(k[1:], 10, 64)
		if k[0] == 'e' {
			e, err := a.st().GetEmployee(ctx, id)
			if err != nil || !e.Active || e.Hidden {
				continue
			}
			it := feedItem{Name: strings.TrimSpace(strings.TrimSuffix(e.LastName+", "+e.FirstName, ", ")), Phone: e.Phone, Mobile: e.Mobile, Email: e.Email, Amt: e.Amt}
			if c, ok := chByID[e.Channel]; ok {
				it.Via, it.ViaTel, it.ViaURL = c.Name, c.Phone, c.TicketURL
				if c.HideDirect && c.Phone != "" {
					it.Phone, it.Mobile = "", ""
				}
			}
			if a.absencesMode() == "all" {
				for _, ab := range currentAbsences(e.Absences, today) {
					if ab.From <= today {
						it.Away = ab.To
					}
				}
			}
			items = append(items, it)
		} else {
			if priv == nil {
				if priv, err = a.st().ListPrivate(ctx, user); err != nil {
					dbFail(w, err)
					return
				}
			}
			for _, pc := range priv {
				if pc.ID == id {
					items = append(items, feedItem{Name: strings.TrimSpace(strings.TrimSuffix(pc.LastName+", "+pc.FirstName, ", ")), Phone: pc.Phone, Mobile: pc.Mobile, Email: pc.Email, Amt: pc.Amt, Private: true})
				}
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "items": items})
}

// ---------- Extras: Freigabe durch die Administration ----------

func (a *App) handleSetFunGlobal(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in map[string]bool
	if !decodeJSON(w, r, &in, 2<<10) {
		return
	}
	out := map[string]bool{}
	for _, k := range funKeys {
		v, ok := in[k]
		if !ok {
			jsonError(w, http.StatusBadRequest, "Bitte alle Schalter angeben")
			return
		}
		out[k] = v
	}
	b, _ := json.Marshal(out)
	if err := a.st().SetSetting(r.Context(), settingFun, string(b)); err != nil {
		dbFail(w, err)
		return
	}
	a.audit(r, s.Username, "fun_settings", "", string(b))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// badgeUsers: Benutzernamen, die ihre Abzeichen zeigen möchten
func (a *App) badgeUsers(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	if !a.funGlobal(ctx)["badges"] {
		return out
	}
	all, err := a.st().ListPrefs(ctx)
	if err != nil {
		return out
	}
	for u, v := range all {
		var p UserPrefs
		if json.Unmarshal([]byte(v), &p) == nil && p.Fun["badges"] {
			out[strings.ToLower(u)] = true
		}
	}
	return out
}
