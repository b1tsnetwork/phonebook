package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

var errInvalidCredentials = errors.New("Benutzername oder Passwort falsch")
var errNotAllowed = errors.New("Kein Zugriff auf das Telefonbuch")

type LDAPClient struct {
	cfg *LDAPConfig
	tls *tls.Config
}

func newLDAPClient(c *LDAPConfig) (*LDAPClient, error) {
	tc := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.InsecureSkipVerify} //nolint:gosec // bewusst konfigurierbar, Warnung beim Start
	u, err := url.Parse(c.URL)
	if err != nil {
		return nil, fmt.Errorf("ldap.url: %w", err)
	}
	tc.ServerName = u.Hostname()
	if c.ServerName != "" {
		tc.ServerName = c.ServerName
	}
	if c.CAFile != "" {
		pem, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("ldap.caFile: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("ldap.caFile enthält kein gültiges PEM-Zertifikat")
		}
		tc.RootCAs = pool
	}
	return &LDAPClient{cfg: c, tls: tc}, nil
}

func (l *LDAPClient) dial() (*ldap.Conn, error) {
	to := l.cfg.Timeout.Duration
	if to <= 0 {
		to = 10 * time.Second
	}
	conn, err := ldap.DialURL(l.cfg.URL, ldap.DialWithTLSConfig(l.tls), ldap.DialWithDialer(&net.Dialer{Timeout: to}))
	if err != nil {
		return nil, fmt.Errorf("LDAP-Verbindung: %w", err)
	}
	conn.SetTimeout(to)
	if l.cfg.StartTLS && strings.HasPrefix(l.cfg.URL, "ldap://") {
		if err := conn.StartTLS(l.tls); err != nil {
			conn.Close()
			return nil, fmt.Errorf("StartTLS: %w", err)
		}
	}
	return conn, nil
}

// Verbindung mit Dienstkonto
func (l *LDAPClient) serviceConn() (*ldap.Conn, error) {
	conn, err := l.dial()
	if err != nil {
		return nil, err
	}
	if l.cfg.BindDN == "" {
		conn.Close()
		return nil, errors.New("kein Dienstkonto (ldap.bindDN) konfiguriert")
	}
	if err := conn.Bind(l.cfg.BindDN, l.cfg.BindPassword); err != nil {
		conn.Close()
		return nil, fmt.Errorf("Bind mit Dienstkonto fehlgeschlagen: %w", err)
	}
	return conn, nil
}

func (l *LDAPClient) attrList() []string {
	a := l.cfg.Attributes
	list := []string{"dn"}
	for _, s := range []string{a.Username, a.LastName, a.FirstName, a.DisplayName, a.Phone, a.Mobile, a.Room, a.Amt, a.Email, a.GUID} {
		if s != "" {
			list = append(list, s)
		}
	}
	return list
}

// LDAPUser ist das Ergebnis einer Anmeldung bzw. eines Sync-Eintrags.
type LDAPUser struct {
	DN          string
	Username    string
	DisplayName string
	GUID        string
	Fields      map[string]string // lastName, firstName, phone, …
	IsAdmin     bool
}

func (l *LDAPClient) entryToUser(e *ldap.Entry) LDAPUser {
	a := l.cfg.Attributes
	get := func(attr string) string {
		if attr == "" {
			return ""
		}
		return strings.TrimSpace(e.GetAttributeValue(attr))
	}
	u := LDAPUser{DN: e.DN, Username: strings.ToLower(get(a.Username)), DisplayName: get(a.DisplayName), Fields: map[string]string{
		"lastName": get(a.LastName), "firstName": get(a.FirstName), "phone": get(a.Phone), "mobile": get(a.Mobile),
		"room": get(a.Room), "amt": get(a.Amt), "email": get(a.Email),
	}}
	if a.GUID != "" {
		if raw := e.GetRawAttributeValue(a.GUID); len(raw) == 16 {
			u.GUID = formatADGUID(raw)
		} else if s := get(a.GUID); s != "" { // z. B. entryUUID bei OpenLDAP
			u.GUID = strings.ToLower(s)
		}
	}
	if u.DisplayName == "" {
		u.DisplayName = strings.TrimSpace(u.Fields["firstName"] + " " + u.Fields["lastName"])
	}
	return u
}

// objectGUID: die ersten drei Gruppen sind little-endian gespeichert
func formatADGUID(b []byte) string {
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[3], b[2], b[1], b[0], b[5], b[4], b[7], b[6], b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}

func normalizeLoginName(s string) string {
	s = strings.TrimSpace(s)
	// DOMAIN\user → user, user@domain → user
	if i := strings.LastIndex(s, `\`); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.Index(s, "@"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}

// Authenticate prüft Benutzername/Passwort per Bind und liefert die Stammdaten.
func (l *LDAPClient) Authenticate(username, password string) (*LDAPUser, error) {
	// Leeres Passwort wäre bei vielen Servern ein "unauthenticated bind" und damit erfolgreich!
	if username == "" || password == "" {
		return nil, errInvalidCredentials
	}
	if !reLoginName.MatchString(username) {
		return nil, errInvalidCredentials
	}
	filter := strings.ReplaceAll(l.cfg.UserFilter, "{username}", ldap.EscapeFilter(username))
	search := ldap.NewSearchRequest(l.cfg.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 10, false, filter, l.attrList(), nil)

	var conn *ldap.Conn
	var err error
	var entry *ldap.Entry
	if l.cfg.BindDN != "" {
		conn, err = l.serviceConn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		res, err := conn.Search(search)
		if err != nil {
			return nil, fmt.Errorf("LDAP-Suche: %w", err)
		}
		if len(res.Entries) != 1 {
			return nil, errInvalidCredentials
		}
		entry = res.Entries[0]
		// Passwort prüfen: Bind als Benutzer auf eigener Verbindung
		uc, err := l.dial()
		if err != nil {
			return nil, err
		}
		berr := uc.Bind(entry.DN, password)
		uc.Close()
		if berr != nil {
			if ldap.IsErrorWithCode(berr, ldap.LDAPResultInvalidCredentials) {
				return nil, errInvalidCredentials
			}
			return nil, fmt.Errorf("LDAP-Bind: %w", berr)
		}
	} else {
		conn, err = l.dial()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		bindName := strings.ReplaceAll(l.cfg.UserBindTemplate, "{username}", username)
		if err := conn.Bind(bindName, password); err != nil {
			if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
				return nil, errInvalidCredentials
			}
			return nil, fmt.Errorf("LDAP-Bind: %w", err)
		}
		res, err := conn.Search(search)
		if err != nil {
			return nil, fmt.Errorf("LDAP-Suche: %w", err)
		}
		if len(res.Entries) != 1 {
			return nil, errInvalidCredentials
		}
		entry = res.Entries[0]
	}

	u := l.entryToUser(entry)
	if u.Username == "" {
		u.Username = username
	}
	if l.cfg.UserGroupDN != "" {
		ok, err := l.isMember(conn, entry.DN, l.cfg.UserGroupDN)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errNotAllowed
		}
	}
	if l.cfg.AdminGroupDN != "" {
		ok, err := l.isMember(conn, entry.DN, l.cfg.AdminGroupDN)
		if err != nil {
			return nil, err
		}
		u.IsAdmin = ok
	}
	return &u, nil
}

// Gruppenmitgliedschaft prüfen; bei AD optional inkl. verschachtelter Gruppen (LDAP_MATCHING_RULE_IN_CHAIN).
func (l *LDAPClient) isMember(conn *ldap.Conn, userDN, groupDN string) (bool, error) {
	filter := "(memberOf=" + ldap.EscapeFilter(groupDN) + ")"
	if l.cfg.NestedGroups {
		filter = "(memberOf:1.2.840.113556.1.4.1941:=" + ldap.EscapeFilter(groupDN) + ")"
	}
	req := ldap.NewSearchRequest(userDN, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 10, false, filter, []string{"dn"}, nil)
	res, err := conn.Search(req)
	if err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			return false, nil
		}
		return false, fmt.Errorf("Gruppenprüfung: %w", err)
	}
	return len(res.Entries) > 0, nil
}

// SearchAll liefert alle Benutzer für den AD-Sync (seitenweise).
func (l *LDAPClient) SearchAll(baseDN, filter string, pageSize int) ([]LDAPUser, error) {
	conn, err := l.serviceConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if baseDN == "" {
		baseDN = l.cfg.BaseDN
	}
	if pageSize <= 0 {
		pageSize = 500
	}
	req := ldap.NewSearchRequest(baseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 120, false, filter, l.attrList(), nil)
	res, err := conn.SearchWithPaging(req, uint32(pageSize))
	if err != nil {
		return nil, fmt.Errorf("LDAP-Suche: %w", err)
	}
	out := make([]LDAPUser, 0, len(res.Entries))
	for _, e := range res.Entries {
		out = append(out, l.entryToUser(e))
	}
	return out, nil
}

// Test der Verbindung (für die Admin-Oberfläche)
func (l *LDAPClient) Ping() error {
	conn, err := l.serviceConn()
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
