package main

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var reLoginName = regexp.MustCompile(`^[a-z0-9._\-]{1,64}$`)

// ---------- Passwort-Hashes für lokale Konten (PBKDF2-SHA256) ----------

const pbkdfIter = 600000

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	k, err := pbkdf2.Key(sha256.New, pw, salt, pbkdfIter, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdfIter, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(k)), nil
}

func checkPassword(stored, pw string) bool {
	p := strings.Split(stored, "$")
	if len(p) != 4 || p[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(p[1])
	if err != nil || iter < 10000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(p[2])
	want, err2 := base64.RawStdEncoding.DecodeString(p[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// Für Laufzeitgleichheit bei unbekannten Benutzern
var dummyHash, _ = hashPassword("dummy-password-for-timing")

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ---------- Sitzungen ----------

type Session struct {
	IDHash      string
	Username    string
	DisplayName string
	IsAdmin     bool
	Source      string
	CSRF        string
	LastSeen    time.Time
	ExpiresAt   time.Time
	EmployeeID  int64 // 0 = kein eigener Telefonbucheintrag
}

type ctxKey int

const sessKey ctxKey = 1

func sessionFrom(r *http.Request) *Session {
	s, _ := r.Context().Value(sessKey).(*Session)
	return s
}

func (a *App) cookieName(r *http.Request) string {
	if a.secureCookie(r) {
		return "__Host-tb_session"
	}
	return "tb_session"
}

func (a *App) secureCookie(r *http.Request) bool {
	switch a.cfg.Auth.CookieSecure {
	case "always":
		return true
	case "never":
		return false
	}
	return a.isHTTPS(r)
}

func (a *App) isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return a.fromTrustedProxy(r) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (a *App) fromTrustedProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	for _, n := range a.cfg.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (a *App) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if a.fromTrustedProxy(r) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			// von rechts den ersten nicht vertrauenswürdigen Eintrag nehmen
			for i := len(parts) - 1; i >= 0; i-- {
				p := strings.TrimSpace(parts[i])
				ip := net.ParseIP(p)
				if ip == nil {
					break
				}
				trusted := false
				for _, n := range a.cfg.trusted {
					if n.Contains(ip) {
						trusted = true
						break
					}
				}
				if !trusted || i == 0 {
					return p
				}
			}
		}
	}
	return host
}

func (a *App) createSession(ctx context.Context, w http.ResponseWriter, r *http.Request, username, display string, isAdmin bool, source string) error {
	token := randomToken(32)
	now := time.Now().UTC()
	exp := now.Add(a.cfg.Auth.SessionMax.Duration)
	ua := r.UserAgent()
	if len(ua) > 250 {
		ua = ua[:250]
	}
	err := a.st().CreateSession(ctx, &SessionRec{IDHash: sha256hex(token), Username: username, DisplayName: truncate(display, 200), IsAdmin: isAdmin,
		Source: source, CSRF: randomToken(32), CreatedAt: now, LastSeen: now, ExpiresAt: exp, IP: a.clientIP(r), UserAgent: ua})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: a.cookieName(r), Value: token, Path: "/", HttpOnly: true, Secure: a.secureCookie(r),
		SameSite: http.SameSiteStrictMode, Expires: exp,
	})
	return nil
}

func (a *App) loadSession(r *http.Request) (*Session, error) {
	c, err := r.Cookie(a.cookieName(r))
	if err != nil || c.Value == "" || len(c.Value) > 100 {
		return nil, nil
	}
	h := sha256hex(c.Value)
	rec, err := a.st().GetSession(r.Context(), h)
	if err != nil || rec == nil {
		return nil, err
	}
	now := time.Now().UTC()
	if now.After(rec.ExpiresAt) || now.Sub(rec.LastSeen) > a.cfg.Auth.SessionIdle.Duration {
		_ = a.st().DeleteSession(r.Context(), h)
		return nil, nil
	}
	if now.Sub(rec.LastSeen) > time.Minute {
		_ = a.st().TouchSession(r.Context(), h, now)
	}
	emp, err := a.st().EmployeeIDByUsername(r.Context(), rec.Username)
	if err != nil {
		return nil, err
	}
	return &Session{IDHash: h, Username: rec.Username, DisplayName: rec.DisplayName, IsAdmin: rec.IsAdmin, Source: rec.Source,
		CSRF: rec.CSRF, LastSeen: rec.LastSeen, ExpiresAt: rec.ExpiresAt, EmployeeID: emp}, nil
}

func (a *App) destroySession(w http.ResponseWriter, r *http.Request) {
	if s := sessionFrom(r); s != nil {
		_ = a.st().DeleteSession(r.Context(), s.IDHash)
	}
	http.SetCookie(w, &http.Cookie{Name: a.cookieName(r), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.secureCookie(r), SameSite: http.SameSiteStrictMode})
}

func (a *App) sessionGC(ctx context.Context) {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for {
		now := time.Now().UTC()
		_ = a.st().PurgeSessions(ctx, now, now.Add(-a.cfg.Auth.SessionIdle.Duration))
		a.limiter.gc()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ---------- Middleware ----------

// requireAuth lädt die Sitzung, prüft bei ändernden Anfragen CSRF-Token, Origin und Content-Type.
func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := a.loadSession(r)
		if err != nil {
			log.Printf("Sitzung laden: %v", err)
			jsonError(w, http.StatusInternalServerError, "Interner Fehler")
			return
		}
		if s == nil {
			jsonError(w, http.StatusUnauthorized, "Bitte anmelden")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !a.sameOrigin(r) {
				jsonError(w, http.StatusForbidden, "Ungültige Herkunft der Anfrage")
				return
			}
			tok := r.Header.Get("X-CSRF-Token")
			if tok == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(s.CSRF)) != 1 {
				jsonError(w, http.StatusForbidden, "CSRF-Token fehlt oder ist ungültig – bitte Seite neu laden")
				return
			}
		}
		next(w, r.WithContext(context.WithValue(r.Context(), sessKey, s)))
	}
}

// optionalAuth: ohne Anmeldung nur lesend (wenn auth.publicDirectory), sonst wie requireAuth.
func (a *App) optionalAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := a.loadSession(r)
		if err != nil {
			log.Printf("Sitzung laden: %v", err)
			jsonError(w, http.StatusInternalServerError, "Interner Fehler")
			return
		}
		if s == nil {
			if !a.cfg.Auth.PublicDirectory || r.Method != http.MethodGet {
				jsonError(w, http.StatusUnauthorized, "Bitte anmelden")
				return
			}
			next(w, r)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), sessKey, s)))
	}
}

func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if !sessionFrom(r).IsAdmin {
			jsonError(w, http.StatusForbidden, "Nur für Administratoren")
			return
		}
		next(w, r)
	})
}

// Origin/Referer muss zum eigenen Host passen (zusätzlich zu SameSite=Strict und CSRF-Token).
func (a *App) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		// Kein Browser-Kontext (z. B. curl) – CSRF-Token ist trotzdem erforderlich
		return true
	}
	if a.cfg.PublicURL != "" && strings.HasPrefix(origin, strings.TrimRight(a.cfg.PublicURL, "/")) {
		return true
	}
	scheme := "http://"
	if a.isHTTPS(r) {
		scheme = "https://"
	}
	host := r.Host
	if a.fromTrustedProxy(r) && r.Header.Get("X-Forwarded-Host") != "" {
		host = r.Header.Get("X-Forwarded-Host")
	}
	base := scheme + host
	return origin == base || strings.HasPrefix(origin, base+"/")
}

// ---------- Login-Ratenbegrenzung ----------

type limiter struct {
	mu    sync.Mutex
	ip    map[string]*failRec
	user  map[string]*failRec
	maxIP int
	maxU  int
	lock  time.Duration
}

type failRec struct {
	n     int
	first time.Time
	until time.Time
}

func newLimiter(maxIP, maxU int, lock time.Duration) *limiter {
	return &limiter{ip: map[string]*failRec{}, user: map[string]*failRec{}, maxIP: maxIP, maxU: maxU, lock: lock}
}

func (l *limiter) blocked(ip, user string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for _, rec := range []*failRec{l.ip[ip], l.user[user]} {
		if rec != nil && now.Before(rec.until) {
			return true, rec.until.Sub(now)
		}
	}
	return false, 0
}

func (l *limiter) fail(ip, user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	bump := func(m map[string]*failRec, k string, max int) {
		rec := m[k]
		if rec == nil || now.Sub(rec.first) > l.lock {
			rec = &failRec{first: now}
			m[k] = rec
		}
		rec.n++
		if rec.n >= max {
			rec.until = now.Add(l.lock)
		}
	}
	bump(l.ip, ip, l.maxIP)
	if user != "" {
		bump(l.user, user, l.maxU)
	}
}

func (l *limiter) success(ip, user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.user, user)
	if rec := l.ip[ip]; rec != nil && rec.n > 0 {
		rec.n--
	}
}

func (l *limiter) gc() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for _, m := range []map[string]*failRec{l.ip, l.user} {
		for k, v := range m {
			if now.Sub(v.first) > l.lock && now.After(v.until) {
				delete(m, k)
			}
		}
	}
}

// ---------- Login / Logout ----------

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !a.sameOrigin(r) {
		jsonError(w, http.StatusForbidden, "Ungültige Herkunft der Anfrage")
		return
	}
	var req loginReq
	if !decodeJSON(w, r, &req, 4<<10) {
		return
	}
	user := normalizeLoginName(req.Username)
	ip := a.clientIP(r)
	if blocked, wait := a.limiter.blocked(ip, user); blocked {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		jsonError(w, http.StatusTooManyRequests, fmt.Sprintf("Zu viele Fehlversuche – bitte in %d Minuten erneut versuchen", int(wait.Minutes())+1))
		return
	}
	if len(req.Password) > 512 || !reLoginName.MatchString(user) {
		a.limiter.fail(ip, "")
		jsonError(w, http.StatusUnauthorized, errInvalidCredentials.Error())
		return
	}

	var display, source string
	var isAdmin, ok bool
	var lu *LDAPUser
	var authErr error

	// DEV-Admin: nie gegen das AD prüfen (keine Sperren/Fehlversuche dort)
	devLogin := a.cfg.Auth.DevAdmin && user == a.cfg.Auth.DevAdminUser
	if a.cfg.ldapEnabled() && a.ldap != nil && !devLogin {
		lu, authErr = a.ldap.Authenticate(user, req.Password)
		if authErr == nil {
			ok, display, isAdmin, source = true, lu.DisplayName, lu.IsAdmin, "ldap"
			user = lu.Username
		} else if !errors.Is(authErr, errInvalidCredentials) && !errors.Is(authErr, errNotAllowed) {
			log.Printf("LDAP-Anmeldung %q: %v", user, authErr)
		}
	}
	if !ok && a.cfg.localAllowed(user) && !errors.Is(authErr, errNotAllowed) {
		loc, err := a.st().GetLocalUser(r.Context(), user)
		if err == nil && !loc.Disabled && checkPassword(loc.PwHash, req.Password) {
			ok, source, display, isAdmin = true, "local", loc.DisplayName, loc.IsAdmin
			if devLogin {
				source = "dev"
			}
			_ = a.st().TouchLocalUser(r.Context(), user, time.Now().UTC())
		} else if err != nil {
			checkPassword(dummyHash, req.Password)
		}
	}
	if !ok {
		a.limiter.fail(ip, user)
		a.audit(r, user, "login_failed", "", "")
		time.Sleep(300 * time.Millisecond)
		if errors.Is(authErr, errNotAllowed) {
			jsonError(w, http.StatusForbidden, errNotAllowed.Error())
			return
		}
		if authErr != nil && !errors.Is(authErr, errInvalidCredentials) && !a.cfg.localEnabled() {
			jsonError(w, http.StatusServiceUnavailable, "Anmeldeserver nicht erreichbar – bitte später erneut versuchen")
			return
		}
		jsonError(w, http.StatusUnauthorized, errInvalidCredentials.Error())
		return
	}
	a.limiter.success(ip, user)
	if lu != nil {
		a.ensureEmployeeForLogin(r.Context(), lu)
	}
	if display == "" {
		display = user
	}
	// Alte Sitzung (Session Fixation) entfernen
	if old, _ := a.loadSession(r); old != nil {
		_ = a.st().DeleteSession(r.Context(), old.IDHash)
	}
	if err := a.createSession(r.Context(), w, r, user, display, isAdmin, source); err != nil {
		log.Printf("Sitzung anlegen: %v", err)
		jsonError(w, http.StatusInternalServerError, "Interner Fehler")
		return
	}
	a.audit(r, user, "login", source, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.destroySession(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
