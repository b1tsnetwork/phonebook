package main

import (
	"strings"
	"testing"
)

func TestPSQuoteAndCommand(t *testing.T) {
	if got := psQuote("O'Neil ’x’ $(calc)"); got != `'O''Neil ’’x’’ $(calc)'` {
		t.Fatalf("psQuote: %s", got)
	}
	a := &App{cfg: defaultConfig()}
	a.cfg.LDAP.URL = "ldaps://dc01.firma.local:636"
	e := &EmployeeRec{ID: 1, Source: "ad", ADGUID: "0F8FAD5B-D9CB-469F-A165-70867728950E", LastName: "Peters", FirstName: "Felix", Phone: "0123 456-110"}
	cmd, why := a.psCommand(Report{Field: "phone", Suggestion: "0123 456-999"}, e)
	if why != "" || cmd != `Set-ADUser -Identity '0f8fad5b-d9cb-469f-a165-70867728950e' -Server 'dc01.firma.local' -Replace @{'telephoneNumber'='0123 456-999'}` {
		t.Fatalf("Befehl: %q %q", cmd, why)
	}
	if cmd, _ := a.psCommand(Report{Field: "mobile", Suggestion: "-"}, e); !strings.Contains(cmd, "-Clear 'mobile'") {
		t.Fatalf("Leeren: %s", cmd)
	}
	if cmd, _ := a.psCommand(Report{Field: "name", Suggestion: "Peters-Neu, Felix"}, e); !strings.Contains(cmd, `'givenName'='Felix'; 'sn'='Peters-Neu'`) {
		t.Fatalf("Name: %s", cmd)
	}
	for _, bad := range []Report{{Field: "phone", Suggestion: "0123'; Remove-ADUser x"}, {Field: "other", Suggestion: "x"}, {Field: "email", Suggestion: "kein mail"}} {
		if cmd, _ := a.psCommand(bad, e); cmd != "" {
			t.Fatalf("unsicher/ungültig akzeptiert: %s", cmd)
		}
	}
	if cmd, why := a.psCommand(Report{Field: "phone", Suggestion: "1"}, &EmployeeRec{Source: "manual"}); cmd != "" || why == "" {
		t.Fatal("manueller Eintrag erzeugt AD-Befehl")
	}
	if !reportApplied(Report{Field: "phone", Suggestion: "0123/456 999"}, &EmployeeRec{Phone: "0123 456-999"}) {
		t.Fatal("Telefonnummer-Vergleich")
	}
}

func TestOrgMatcher(t *testing.T) {
	m := newOrgMatcher([]OrgUnit{
		{ID: 1, Name: "Verwaltung", Short: "I"},
		{ID: 2, Parent: 1, Name: "Hauptamt", Short: "10"},
		{ID: 3, Parent: 2, Name: "Personal", Short: "10.1"},
		{ID: 4, Name: "Straßenbau"},
		{ID: 5, Name: "Schulamt"}, {ID: 6, Name: "Schulamt"},
	})
	cases := []struct {
		amt  string
		unit int64
		kind string
	}{
		{"10 – Hauptamt", 2, "exakt"}, {"Hauptamt", 2, "exakt"}, {"HAUPTAMT", 2, "exakt"}, {"10 - Hauptamt", 2, "exakt"},
		{"10.1 Personalwesen", 3, "kuerzel"}, {"Amt für Strassenbau", 4, "aehnlich"}, {"Schulamt", 0, "exakt"}, {"Bürgerbüro", 0, ""},
	}
	for _, c := range cases {
		got := m.match(c.amt)
		if got.Unit != c.unit || got.Kind != c.kind {
			t.Fatalf("%q → %+v, erwartet %d/%s", c.amt, got, c.unit, c.kind)
		}
	}
	if !m.isWithin(3, 2) || m.isWithin(2, 3) {
		t.Fatal("isWithin")
	}
}
