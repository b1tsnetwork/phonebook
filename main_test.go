package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPassword(t *testing.T) {
	h, err := hashPassword("richtig-langes-passwort")
	if err != nil {
		t.Fatal(err)
	}
	if !checkPassword(h, "richtig-langes-passwort") {
		t.Fatal("korrektes Passwort abgelehnt")
	}
	if checkPassword(h, "falsch") || checkPassword("kaputt", "x") || checkPassword("pbkdf2-sha256$5$aa$bb", "x") {
		t.Fatal("falsches Passwort akzeptiert")
	}
}

func TestLoginName(t *testing.T) {
	for in, want := range map[string]string{`FIRMA\MMuster`: "mmuster", "mmuster@firma.local": "mmuster", "  Max.Muster ": "max.muster"} {
		if got := normalizeLoginName(in); got != want {
			t.Errorf("%q → %q, erwartet %q", in, got, want)
		}
	}
	for _, bad := range []string{"a*b", "a)(b", "x y", "", strings.Repeat("a", 70)} {
		if reLoginName.MatchString(normalizeLoginName(bad)) {
			t.Errorf("%q hätte abgelehnt werden müssen", bad)
		}
	}
}

func TestADGUID(t *testing.T) {
	b := []byte{0x33, 0x22, 0x11, 0x00, 0x55, 0x44, 0x77, 0x66, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	if g := formatADGUID(b); g != "00112233-4455-6677-8899-aabbccddeeff" {
		t.Fatal(g)
	}
}

func TestCleanText(t *testing.T) {
	s, ok := cleanText("  Max\t\n  Muster​\x00 ", 20)
	if !ok || s != "Max Muster" {
		t.Fatalf("%q %v", s, ok)
	}
	if _, ok := cleanText(strings.Repeat("ä", 11), 10); ok {
		t.Fatal("Länge nicht geprüft")
	}
}

func TestContactValidation(t *testing.T) {
	c := contactFields{LastName: "Muster", Phone: "+49 (123) 456-78"}
	if err := c.clean(); err != nil {
		t.Fatal(err)
	}
	c = contactFields{LastName: "Muster", Phone: "<script>"}
	if c.clean() == nil {
		t.Fatal("ungültige Telefonnummer akzeptiert")
	}
	c = contactFields{}
	if c.clean() == nil {
		t.Fatal("leerer Name akzeptiert")
	}
	c = contactFields{LastName: "X", Email: "a@b.de"}
	if err := c.clean(); err != nil {
		t.Fatal(err)
	}
	c = contactFields{LastName: "X", Email: "kein mail"}
	if c.clean() == nil {
		t.Fatal("ungültige E-Mail akzeptiert")
	}
}

func TestAvatarValidation(t *testing.T) {
	good := `{"v":1,"skin":"#a7a9ac","hair":"#6d6f73","eyes":"#3d3f42","accent":"#8a8d91","glassesColor":"#3a3a3a","hairStyle":"short","eyeStyle":"classic","browStyle":"none","mouthStyle":"smile","noseStyle":"none","beardStyle":"none","glassesStyle":"none","hat":"none","extras":["blush"],"texture":true,"paint":{"base":{"front":{"12":"#ff0000"}},"over":{"top":{"0":""}}}}`
	out, err := validateAvatar(json.RawMessage(good))
	if err != nil || out == "" {
		t.Fatal(err)
	}
	bad := []string{
		strings.Replace(good, `"#a7a9ac"`, `"red; background:url(x)"`, 1),
		strings.Replace(good, `"short"`, `"<img>"`, 1),
		strings.Replace(good, `"12"`, `"64"`, 1),
		strings.Replace(good, `"base":{"front":{"12":"#ff0000"}}`, `"base":{"front":{"12":""}}`, 1),
		strings.Replace(good, `"front"`, `"inside"`, 1),
		strings.Replace(good, `"v":1`, `"v":1,"evil":true`, 1),
		strings.Replace(good, `"v":1`, `"v":3`, 1),
	}
	for i, b := range bad {
		if _, err := validateAvatar(json.RawMessage(b)); err == nil {
			t.Errorf("ungültiger Avatar %d akzeptiert", i)
		}
	}
	if out, err := validateAvatar(json.RawMessage("null")); err != nil || out != "" {
		t.Fatal("null sollte Standard-Avatar bedeuten")
	}
}

func TestTagName(t *testing.T) {
	if n, err := normalizeTagName("  Active   Directory "); err != nil || n != "Active Directory" {
		t.Fatal(n, err)
	}
	for _, b := range []string{"", "   ", "<b>", strings.Repeat("x", 61)} {
		if _, err := normalizeTagName(b); err == nil {
			t.Errorf("%q akzeptiert", b)
		}
	}
}

func TestSplitSQL(t *testing.T) {
	b, _ := migrationFS.ReadFile("migrations/001_init.sql")
	st := splitSQL(string(b))
	if len(st) != 8 {
		t.Fatalf("%d Anweisungen", len(st))
	}
}

func TestLimiter(t *testing.T) {
	l := newLimiter(100, 3, time.Minute)
	for i := 0; i < 3; i++ {
		l.fail("1.2.3.4", "max")
	}
	if b, _ := l.blocked("9.9.9.9", "max"); !b {
		t.Fatal("Benutzer sollte gesperrt sein")
	}
	if b, _ := l.blocked("9.9.9.9", "anna"); b {
		t.Fatal("anderer Benutzer gesperrt")
	}
}

func TestConfigValidation(t *testing.T) {
	c := defaultConfig()
	c.LDAP.URL = "ldap://dc.firma.local"
	c.LDAP.BaseDN = "DC=firma,DC=local"
	c.LDAP.BindDN = "CN=svc"
	if err := c.validate(); err == nil || !strings.Contains(err.Error(), "ldaps") {
		t.Fatal("unverschlüsseltes LDAP sollte abgelehnt werden")
	}
	c.LDAP.URL = "ldaps://dc.firma.local"
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDevAdminConfig(t *testing.T) {
	c := defaultConfig()
	c.Auth.DevAdmin = true
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	if c.Auth.Mode != "local" {
		t.Fatalf("ohne LDAP-URL sollte nur lokal angemeldet werden, Modus %q", c.Auth.Mode)
	}
	c = defaultConfig()
	c.LDAP.URL, c.LDAP.BaseDN, c.LDAP.BindDN = "ldaps://dc", "DC=x", "CN=svc"
	c.Auth.DevAdmin = true
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	if !c.localAllowed("devadmin") || c.localAllowed("mmuster") {
		t.Fatal("im LDAP-Modus darf nur der DEV-Admin lokal")
	}
	c.Auth.DevAdmin = false
	if c.localAllowed("devadmin") {
		t.Fatal("ohne devAdmin keine lokale Anmeldung")
	}
	if !defaultConfig().Auth.PublicDirectory {
		t.Fatal("Telefonbuch sollte standardmäßig ohne Anmeldung lesbar sein")
	}
}

func TestAvatarV2(t *testing.T) {
	v2 := `{"v":2,"skin":"#a7a9ac","hair":"#6d6f73","eyes":"#3d3f42","accent":"#8a8d91","glassesColor":"#3a3a3a","hairStyle":"braids","eyeStyle":"heart","browStyle":"normal","mouthStyle":"bigsmile","noseStyle":"button","beardStyle":"none","glassesStyle":"vr","hat":"wizard","ears":"elf","eyes2":"#3a86ff","lips":"#b23a48","highlight":"#ececec","extras":["mask"],"texture":true,"paint":{"base":{"front":{"255":"#ff0000"}}}}`
	if _, err := validateAvatar(json.RawMessage(v2)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(v2, `"255"`, `"256"`, 1),
		strings.Replace(v2, `"ears":"elf"`, `"ears":"<x>"`, 1),
		strings.Replace(v2, `"lips":"#b23a48"`, `"lips":"red"`, 1),
		strings.Replace(strings.Replace(v2, `"v":2`, `"v":1`, 1), `"255"`, `"64"`, 1),
	} {
		if _, err := validateAvatar(json.RawMessage(bad)); err == nil {
			t.Errorf("ungültiger Avatar akzeptiert: %.80s", bad)
		}
	}
}
