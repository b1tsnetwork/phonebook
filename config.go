package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// Duration erlaubt "8h", "30m" usw. in der JSON-Konfiguration.
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("Dauer als Text erwartet, z. B. \"8h\": %w", err)
	}
	if s == "" {
		d.Duration = 0
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

type Config struct {
	Addr string `json:"addr"`
	// Öffentliche Basis-URL, z. B. https://telefonbuch.firma.local – wird für die Origin-Prüfung genutzt.
	PublicURL string `json:"publicURL"`
	// Reverse-Proxys, deren X-Forwarded-For/-Proto vertraut wird (CIDR oder IP).
	TrustedProxies []string `json:"trustedProxies"`

	TLS struct {
		CertFile string `json:"certFile"`
		KeyFile  string `json:"keyFile"`
	} `json:"tls"`

	Storage StorageConfig `json:"storage"`

	DB struct {
		// Entweder vollständige DSN (go-sql-driver-Format) …
		DSN string `json:"dsn"`
		// … oder Einzelwerte
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		Name     string `json:"name"`
		// "", "true", "skip-verify", "preferred" oder "custom" (dann CAFile)
		TLS    string `json:"tls"`
		CAFile string `json:"caFile"`
	} `json:"db"`

	Auth struct {
		// "ldap", "local" oder "ldap+local" (LDAP zuerst, lokale Konten als Notfallzugang)
		Mode           string   `json:"mode"`
		SessionIdle    Duration `json:"sessionIdle"`
		SessionMax     Duration `json:"sessionMax"`
		CookieSecure   string   `json:"cookieSecure"` // "auto", "always", "never"
		MaxFailsPerIP  int      `json:"maxFailsPerIP"`
		MaxFailsPerUsr int      `json:"maxFailsPerUser"`
		LockDuration   Duration `json:"lockDuration"`
		// Telefonbuch ohne Anmeldung lesbar (Bearbeiten erfordert weiterhin eine Anmeldung)
		PublicDirectory bool `json:"publicDirectory"`
		// NUR FÜR ENTWICKLUNG/TEST: lokales Admin-Konto, das am AD vorbei funktioniert.
		// Passwort aus TB_DEV_ADMIN_PASSWORD (sonst zufällig, steht im Log).
		DevAdmin     bool   `json:"devAdmin"`
		DevAdminUser string `json:"devAdminUser"`
	} `json:"auth"`

	LDAP LDAPConfig `json:"ldap"`

	Sync SyncConfig `json:"adSync"`

	Features struct {
		// Dürfen Mitarbeiter neue Schlagworte anlegen? (sonst nur vorhandene wählen)
		UserCreatesTags bool `json:"userCreatesTags"`
		MaxTagsPerUser  int  `json:"maxTagsPerUser"`
		MaxPrivate      int  `json:"maxPrivateContacts"`
		// Beim ersten LDAP-Login automatisch einen Telefonbucheintrag anlegen, falls keiner existiert
		AutoCreateOnLogin bool `json:"autoCreateOnLogin"`
	} `json:"features"`

	trusted []*net.IPNet
}

// StorageConfig: Datenhaltung in MySQL (Standard) oder in einer JSON-Datei.
type StorageConfig struct {
	Mode     string `json:"mode"`     // "mysql" oder "json"
	JSONPath string `json:"jsonPath"` // Datei für den JSON-Modus bzw. für Kopien aus MySQL
	// Protokoll (Audit) in eigener Datei; leer = neben jsonPath, z. B. telefonbuch.audit.json
	JSONAuditPath string `json:"jsonAuditPath"`
	// Nur JSON-Modus: Daten aus der MySQL-Datenbank übernehmen.
	// "off" (Standard), "manual" (Knopf in der Verwaltung + Befehl), "startup" (zusätzlich bei jedem Start)
	MySQLImport         string   `json:"mysqlImport"`
	MySQLImportInterval Duration `json:"mysqlImportInterval"` // zusätzlich regelmäßig, z. B. "1h"
	// Schutz: Übernahme abbrechen, wenn MySQL weniger als diesen Anteil (in %) der JSON-Einträge hat
	MySQLImportMinPercent int `json:"mysqlImportMinPercent"`
	// Nur MySQL-Modus: regelmäßig eine JSON-Kopie nach jsonPath schreiben (Offline-Reserve), z. B. "1h"
	JSONSnapshotInterval Duration `json:"jsonSnapshotInterval"`
	// Nur MySQL-Modus: Ist MySQL beim Start nicht erreichbar, die JSON-Kopie (jsonPath) nur lesend
	// verwenden und regelmäßig (fallbackRetry) erneut verbinden. Standard: true
	FallbackToJSON *bool    `json:"fallbackToJSON"`
	FallbackRetry  Duration `json:"fallbackRetry"`
}

type LDAPConfig struct {
	URL                string   `json:"url"` // ldaps://dc01.firma.local:636
	StartTLS           bool     `json:"startTLS"`
	CAFile             string   `json:"caFile"`
	ServerName         string   `json:"serverName"`
	InsecureSkipVerify bool     `json:"insecureSkipVerify"`
	Timeout            Duration `json:"timeout"`

	BindDN       string `json:"bindDN"`
	BindPassword string `json:"bindPassword"`
	// Ohne Dienstkonto: direkte Anmeldung mit z. B. "{username}@firma.local"
	UserBindTemplate string `json:"userBindTemplate"`

	BaseDN     string `json:"baseDN"`
	UserFilter string `json:"userFilter"` // {username} wird (escaped) ersetzt

	AdminGroupDN string `json:"adminGroupDN"`
	// Optional: nur Mitglieder dieser Gruppe dürfen sich anmelden
	UserGroupDN  string `json:"userGroupDN"`
	NestedGroups bool   `json:"nestedGroups"`

	Attributes LDAPAttributes `json:"attributes"`
}

type LDAPAttributes struct {
	Username    string `json:"username"`
	LastName    string `json:"lastName"`
	FirstName   string `json:"firstName"`
	DisplayName string `json:"displayName"`
	Phone       string `json:"phone"`
	Mobile      string `json:"mobile"`
	Room        string `json:"room"`
	Amt         string `json:"amt"`
	Email       string `json:"email"`
	GUID        string `json:"guid"`
}

type SyncConfig struct {
	Enabled  bool     `json:"enabled"`
	Interval Duration `json:"interval"`
	BaseDN   string   `json:"baseDN"`
	Filter   string   `json:"filter"`
	PageSize int      `json:"pageSize"`
	// Welche Felder aus dem AD kommen (und für AD-Einträge in der Oberfläche gesperrt sind)
	Fields            []string `json:"fields"`
	DeactivateMissing bool     `json:"deactivateMissing"`
	// Schutz vor Fehlkonfiguration: Sync bricht ab, wenn mehr als dieser Anteil deaktiviert würde
	MaxDeactivatePercent int  `json:"maxDeactivatePercent"`
	SkipWithoutPhone     bool `json:"skipWithoutPhone"`
}

var syncableFields = map[string]bool{"lastName": true, "firstName": true, "phone": true, "mobile": true, "room": true, "amt": true, "email": true}

func defaultConfig() *Config {
	c := &Config{Addr: ":8080"}
	c.DB.Host, c.DB.Port, c.DB.User, c.DB.Name = "127.0.0.1", 3306, "telefonbuch", "telefonbuch"
	c.Storage.Mode = "mysql"
	c.Storage.JSONPath = "telefonbuch.json"
	c.Storage.MySQLImport = "off"
	c.Storage.MySQLImportMinPercent = 50
	c.Auth.Mode = "ldap"
	c.Auth.SessionIdle = Duration{2 * time.Hour}
	c.Auth.SessionMax = Duration{12 * time.Hour}
	c.Auth.CookieSecure = "auto"
	c.Auth.PublicDirectory = true
	c.Auth.DevAdminUser = "devadmin"
	c.Auth.MaxFailsPerIP = 20
	c.Auth.MaxFailsPerUsr = 5
	c.Auth.LockDuration = Duration{10 * time.Minute}
	c.LDAP.Timeout = Duration{10 * time.Second}
	c.LDAP.UserFilter = "(&(objectCategory=person)(objectClass=user)(sAMAccountName={username}))"
	c.LDAP.NestedGroups = true
	c.LDAP.Attributes = LDAPAttributes{
		Username: "sAMAccountName", LastName: "sn", FirstName: "givenName", DisplayName: "displayName",
		Phone: "telephoneNumber", Mobile: "mobile", Room: "physicalDeliveryOfficeName", Amt: "department",
		Email: "mail", GUID: "objectGUID",
	}
	c.Sync.Interval = Duration{6 * time.Hour}
	c.Sync.Filter = "(&(objectCategory=person)(objectClass=user)(!(userAccountControl:1.2.840.113556.1.4.803:=2)))"
	c.Sync.PageSize = 500
	c.Sync.Fields = []string{"lastName", "firstName", "phone", "mobile", "room", "amt", "email"}
	c.Sync.DeactivateMissing = true
	c.Sync.MaxDeactivatePercent = 30
	c.Features.UserCreatesTags = true
	c.Features.MaxTagsPerUser = 40
	c.Features.MaxPrivate = 300
	c.Features.AutoCreateOnLogin = true
	return c
}

func loadConfig(path string) (*Config, error) {
	c := defaultConfig()
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		dec := json.NewDecoder(f)
		dec.DisallowUnknownFields()
		if err := dec.Decode(c); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	// Geheimnisse bevorzugt aus der Umgebung, damit sie nicht in der Datei stehen müssen
	env := func(k string, dst *string) {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			*dst = v
		}
	}
	env("TB_ADDR", &c.Addr)
	env("TB_DB_DSN", &c.DB.DSN)
	env("TB_DB_HOST", &c.DB.Host)
	env("TB_DB_USER", &c.DB.User)
	env("TB_DB_PASSWORD", &c.DB.Password)
	env("TB_DB_NAME", &c.DB.Name)
	env("TB_LDAP_URL", &c.LDAP.URL)
	env("TB_LDAP_BIND_DN", &c.LDAP.BindDN)
	env("TB_LDAP_BIND_PASSWORD", &c.LDAP.BindPassword)
	env("TB_AUTH_MODE", &c.Auth.Mode)
	if v := os.Getenv("TB_DEV_ADMIN"); v == "1" || v == "true" {
		c.Auth.DevAdmin = true
	}
	if v := os.Getenv("TB_PUBLIC_DIRECTORY"); v != "" {
		c.Auth.PublicDirectory = v == "1" || v == "true"
	}
	env("TB_STORAGE", &c.Storage.Mode)
	env("TB_JSON_PATH", &c.Storage.JSONPath)
	if v := os.Getenv("TB_STORAGE_FALLBACK"); v != "" {
		b := v == "1" || strings.EqualFold(v, "true")
		c.Storage.FallbackToJSON = &b
	}
	if v := os.Getenv("TB_DB_PASSWORD_FILE"); v != "" {
		b, err := os.ReadFile(v)
		if err != nil {
			return nil, err
		}
		c.DB.Password = strings.TrimSpace(string(b))
	}
	if v := os.Getenv("TB_LDAP_BIND_PASSWORD_FILE"); v != "" {
		b, err := os.ReadFile(v)
		if err != nil {
			return nil, err
		}
		c.LDAP.BindPassword = strings.TrimSpace(string(b))
	}
	return c, c.validate()
}

func (c *Config) validate() error {
	switch c.Storage.Mode {
	case "mysql", "json":
	default:
		return errors.New("storage.mode muss mysql oder json sein")
	}
	switch c.Storage.MySQLImport {
	case "", "off", "manual", "startup":
	default:
		return errors.New("storage.mysqlImport muss off, manual oder startup sein")
	}
	if c.Storage.MySQLImportMinPercent < 0 || c.Storage.MySQLImportMinPercent > 100 {
		return errors.New("storage.mysqlImportMinPercent muss zwischen 0 und 100 liegen")
	}
	if c.Storage.MySQLImport == "" {
		c.Storage.MySQLImport = "off"
		c.Storage.MySQLImportMinPercent = 50
	}
	if c.Storage.Mode == "json" && c.Storage.MySQLImportInterval.Duration > 0 && c.Storage.MySQLImport == "off" {
		c.Storage.MySQLImport = "manual"
	}
	for _, d := range []Duration{c.Storage.MySQLImportInterval, c.Storage.JSONSnapshotInterval} {
		if d.Duration != 0 && d.Duration < time.Minute {
			return errors.New("storage: Intervalle müssen mindestens 1m sein")
		}
	}
	if c.Storage.FallbackRetry.Duration == 0 {
		c.Storage.FallbackRetry = Duration{30 * time.Second}
	}
	if c.Storage.FallbackRetry.Duration < 5*time.Second {
		return errors.New("storage.fallbackRetry muss mindestens 5s sein")
	}
	if (c.Storage.Mode == "json" || c.Storage.JSONSnapshotInterval.Duration > 0) && c.Storage.JSONPath == "" {
		return errors.New("storage.jsonPath fehlt")
	}
	switch c.Auth.Mode {
	case "ldap", "local", "ldap+local":
	default:
		return fmt.Errorf("auth.mode muss ldap, local oder ldap+local sein")
	}
	if c.Auth.DevAdmin {
		c.Auth.DevAdminUser = normalizeLoginName(c.Auth.DevAdminUser)
		if !reLoginName.MatchString(c.Auth.DevAdminUser) {
			return errors.New("auth.devAdminUser ungültig")
		}
		// Ohne LDAP-Konfiguration einfach nur lokal anmelden
		if c.ldapEnabled() && c.LDAP.URL == "" {
			c.Auth.Mode = "local"
		}
	}
	if c.ldapEnabled() {
		if c.LDAP.URL == "" {
			return errors.New("ldap.url fehlt")
		}
		if !strings.HasPrefix(c.LDAP.URL, "ldaps://") && !c.LDAP.StartTLS {
			return errors.New("ldap.url muss ldaps:// verwenden oder startTLS aktivieren – Passwörter dürfen nicht unverschlüsselt übertragen werden")
		}
		if c.LDAP.BaseDN == "" {
			return errors.New("ldap.baseDN fehlt")
		}
		if c.LDAP.BindDN == "" && c.LDAP.UserBindTemplate == "" {
			return errors.New("ldap.bindDN (Dienstkonto) oder ldap.userBindTemplate muss gesetzt sein")
		}
		if !strings.Contains(c.LDAP.UserFilter, "{username}") {
			return errors.New("ldap.userFilter muss {username} enthalten")
		}
	}
	if c.Sync.Enabled {
		if !c.ldapEnabled() && c.LDAP.URL == "" {
			return errors.New("adSync benötigt eine LDAP-Konfiguration")
		}
		if c.LDAP.BindDN == "" {
			return errors.New("adSync benötigt ein Dienstkonto (ldap.bindDN)")
		}
		if c.Sync.Interval.Duration != 0 && c.Sync.Interval.Duration < 5*time.Minute {
			return errors.New("adSync.interval muss mindestens 5m sein (oder leer für nur manuell)")
		}
		for _, f := range c.Sync.Fields {
			if !syncableFields[f] {
				return fmt.Errorf("adSync.fields: unbekanntes Feld %q", f)
			}
		}
	}
	switch c.Auth.CookieSecure {
	case "auto", "always", "never":
	default:
		return errors.New("auth.cookieSecure muss auto, always oder never sein")
	}
	for _, p := range c.TrustedProxies {
		if !strings.Contains(p, "/") {
			if strings.Contains(p, ":") {
				p += "/128"
			} else {
				p += "/32"
			}
		}
		_, n, err := net.ParseCIDR(p)
		if err != nil {
			return fmt.Errorf("trustedProxies: %w", err)
		}
		c.trusted = append(c.trusted, n)
	}
	if c.Features.MaxTagsPerUser <= 0 {
		c.Features.MaxTagsPerUser = 40
	}
	if c.Features.MaxPrivate <= 0 {
		c.Features.MaxPrivate = 300
	}
	return nil
}

func (c *Config) ldapEnabled() bool  { return strings.Contains(c.Auth.Mode, "ldap") }
func (c *Config) localEnabled() bool { return strings.Contains(c.Auth.Mode, "local") }

// auditPathFor: Protokolldatei zu einer JSON-Datendatei
func (c *Config) auditPathFor(jsonPath string) string {
	if c.Storage.JSONAuditPath != "" && jsonPath == c.Storage.JSONPath {
		return c.Storage.JSONAuditPath
	}
	return auditPathFor(jsonPath)
}

// Darf sich dieses Konto lokal (ohne AD) anmelden?
func (c *Config) localAllowed(user string) bool {
	return c.localEnabled() || (c.Auth.DevAdmin && user == c.Auth.DevAdminUser)
}

// Felder, die bei AD-Einträgen nur der Sync ändern darf
func (c *Config) adLockedFields() []string {
	if !c.Sync.Enabled {
		return []string{}
	}
	return c.Sync.Fields
}

// fallbackEnabled: Rückfall auf die JSON-Kopie erlaubt?
func (c *Config) fallbackEnabled() bool {
	return c.Storage.Mode == "mysql" && c.Storage.JSONPath != "" && (c.Storage.FallbackToJSON == nil || *c.Storage.FallbackToJSON)
}
