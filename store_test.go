package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Gemeinsame Prüfungen für beide Datenhaltungen. MySQL nur, wenn TB_TEST_DSN gesetzt ist
// (ACHTUNG: die Datenbank wird geleert).
func exerciseStore(t *testing.T, st Store) {
	ctx := context.Background()
	if err := st.Import(ctx, &Snapshot{Format: snapshotFormat, Version: 1}); err != nil {
		t.Fatal("leeren:", err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	e := &EmployeeRec{Username: "mmuster", LastName: "Muster", FirstName: "Max", Phone: "123", Source: "manual", Active: true, CreatedAt: now, UpdatedAt: now}
	id, err := st.CreateEmployee(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateEmployee(ctx, &EmployeeRec{Username: "mmuster", LastName: "X", Source: "manual", Active: true, CreatedAt: now, UpdatedAt: now}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("doppelter Benutzername: %v", err)
	}
	if got, _ := st.EmployeeIDByUsername(ctx, "mmuster"); got != id {
		t.Fatalf("EmployeeIDByUsername = %d", got)
	}
	// Schlagworte: Groß-/Kleinschreibung egal, Anlegen nur wenn erlaubt
	ids, err := st.SetEmployeeTags(ctx, id, []TagInput{{"SAP", "taetigkeit"}, {"sap", "taetigkeit"}, {"Ersthelfer", "besonderheit"}}, "mmuster", true)
	if err != nil || len(ids) != 2 {
		t.Fatalf("SetEmployeeTags: %v %v", ids, err)
	}
	var ut ErrUnknownTag
	if _, err := st.SetEmployeeTags(ctx, id, []TagInput{{"Neu", "taetigkeit"}}, "x", false); !errors.As(err, &ut) {
		t.Fatalf("unbekanntes Schlagwort: %v", err)
	}
	if _, err := st.CreateTag(ctx, "ERSTHELFER", "besonderheit", "admin"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("doppeltes Schlagwort: %v", err)
	}
	other, err := st.CreateTag(ctx, "Erste Hilfe", "besonderheit", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateTag(ctx, other, "sap", "taetigkeit"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Umbenennen auf vorhandenen Namen: %v", err)
	}
	// Zusammenführen: Ersthelfer → Erste Hilfe
	var ersthelfer int64
	tags, _ := st.ListTags(ctx)
	for _, tg := range tags {
		if tg.Name == "Ersthelfer" {
			ersthelfer = tg.ID
		}
	}
	if _, _, err := st.MergeTag(ctx, ersthelfer, other); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetEmployee(ctx, id)
	if err != nil || len(got.Tags) != 2 {
		t.Fatalf("nach Merge: %+v %v", got, err)
	}
	if n, _ := st.DeleteUnusedTags(ctx); n != 0 {
		t.Fatalf("nichts unbenutzt erwartet, %d gelöscht", n)
	}
	// Avatar
	if err := st.SetAvatar(ctx, id, json.RawMessage(`{"v":1,"skin":"#a7a9ac","hair":"#6d6f73","eyes":"#3d3f42","accent":"#8a8d91","glassesColor":"#3a3a3a","hairStyle":"short","eyeStyle":"classic","browStyle":"none","mouthStyle":"smile","noseStyle":"none","beardStyle":"none","glassesStyle":"none","hat":"cap","extras":[],"texture":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := st.SetAvatar(ctx, 99999, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Avatar unbekannt: %v", err)
	}
	// Private Kontakte: nur für den Besitzer
	pid, err := st.CreatePrivate(ctx, "mmuster", PrivateContact{LastName: "Zahnarzt", Phone: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := st.ListPrivate(ctx, "anna"); len(l) != 0 {
		t.Fatal("fremde private Kontakte sichtbar")
	}
	if err := st.UpdatePrivate(ctx, "anna", PrivateContact{ID: pid, LastName: "Hack"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("fremden Kontakt geändert: %v", err)
	}
	if err := st.DeletePrivate(ctx, "anna", pid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("fremden Kontakt gelöscht: %v", err)
	}
	if n, _ := st.CountPrivate(ctx, "mmuster"); n != 1 {
		t.Fatalf("CountPrivate = %d", n)
	}
	// Kategorien: Standard vorhanden, anlegen, umbenennen, löschen mit Folge-Kategorie
	cats, err := st.ListCategories(ctx)
	if err != nil || len(cats) != 3 || cats[2].Key != "sonstiges" {
		t.Fatalf("Standardkategorien: %+v %v", cats, err)
	}
	if err := st.CreateCategory(ctx, Category{Key: "x", Name: "SONSTIGES", Color: "#000000"}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("doppelter Kategoriename: %v", err)
	}
	if err := st.CreateCategory(ctx, Category{Key: "sprachen", Name: "Sprachen", Color: "#112233", Sort: 40}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateCategory(ctx, Category{Key: "sprachen", Name: "Fremdsprachen", Color: "#445566", Sort: 5}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateCategory(ctx, Category{Key: "sprachen", Name: "Tätigkeit", Color: "#445566"}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Umbenennen auf vorhandenen Namen: %v", err)
	}
	if _, err := st.UpdateTag(ctx, other, "Erste Hilfe", "sprachen"); err != nil {
		t.Fatal(err)
	}
	cats, _ = st.ListCategories(ctx)
	if cats[0].Key != "sprachen" || cats[0].Count != 1 || cats[0].Name != "Fremdsprachen" {
		t.Fatalf("Kategorien nach Änderung: %+v", cats)
	}
	if _, err := st.DeleteCategory(ctx, "sprachen", "gibtsnicht"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Folge-Kategorie unbekannt: %v", err)
	}
	if n, err := st.DeleteCategory(ctx, "sprachen", "sonstiges"); err != nil || n != 1 {
		t.Fatalf("DeleteCategory: %d %v", n, err)
	}
	tags, _ = st.ListTags(ctx)
	for _, tg := range tags {
		if tg.ID == other && tg.Category != "sonstiges" {
			t.Fatalf("Schlagwort nicht verschoben: %+v", tg)
		}
	}
	// Freigaben privater Kontakte
	anna, err := st.CreateEmployee(ctx, &EmployeeRec{Username: "anna", LastName: "Anna", Amt: "Amt 10", Source: "manual", Active: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePrivate(ctx, "mmuster", PrivateContact{ID: pid, LastName: "Zahnarzt", Phone: "1", Shares: &Shares{Employees: []int64{anna}}}); err != nil {
		t.Fatal(err)
	}
	if l, _ := st.ListSharedWith(ctx, anna, "", "anna"); len(l) != 1 || l[0].Owner != "mmuster" || l[0].LastName != "Zahnarzt" {
		t.Fatalf("Freigabe an Person: %+v", l)
	}
	if l, _ := st.ListSharedWith(ctx, id, "", "mmuster"); len(l) != 0 {
		t.Fatal("eigener Kontakt als geteilt gemeldet")
	}
	if l, _ := st.ListPrivate(ctx, "mmuster"); len(l) != 1 || l[0].Shares == nil || len(l[0].Shares.Employees) != 1 {
		t.Fatalf("Freigaben fehlen in ListPrivate: %+v", l)
	}
	if err := st.UpdatePrivate(ctx, "mmuster", PrivateContact{ID: pid, LastName: "Zahnarzt", Phone: "1", Shares: &Shares{Employees: []int64{anna}, Amts: []string{"Amt 10"}}}); err != nil {
		t.Fatal(err)
	}
	if l, _ := st.ListSharedWith(ctx, 4242, "amt 10", "x"); len(l) != 1 {
		t.Fatalf("Freigabe an Amt (Groß-/Kleinschreibung): %+v", l)
	}
	if err := st.DeleteEmployee(ctx, anna); err != nil {
		t.Fatal(err)
	}
	if l, _ := st.ListPrivate(ctx, "mmuster"); len(l[0].Shares.Employees) != 0 || len(l[0].Shares.Amts) != 1 {
		t.Fatalf("Freigabe nach Löschen der Person: %+v", l[0].Shares)
	}
	// Organisation, Vertretungen, Abwesenheiten, Meldungen
	vid, err := st.CreateEmployee(ctx, &EmployeeRec{LastName: "Vertreter", Source: "manual", Active: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	dez, _ := st.CreateOrgUnit(ctx, OrgUnit{Name: "Dezernat I", Sort: 1})
	amt, _ := st.CreateOrgUnit(ctx, OrgUnit{Parent: dez, Name: "Hauptamt", Short: "10", Leaders: []int64{id}, Deputies: []int64{vid}})
	if n, err := st.SetOrgUnit(ctx, []int64{id, vid}, amt); err != nil || n != 2 {
		t.Fatalf("SetOrgUnit: %d %v", n, err)
	}
	if err := st.SetDeputies(ctx, id, []int64{vid}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetAbsences(ctx, id, []Absence{{From: "2026-11-02", To: "2026-11-06", Deputies: []int64{vid}}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(ctx, settingOrgLevels, `["Dezernat","Amt"]`); err != nil {
		t.Fatal(err)
	}
	if v, _ := st.GetSetting(ctx, settingOrgLevels); v != `["Dezernat","Amt"]` {
		t.Fatalf("Setting: %q", v)
	}
	got, _ = st.GetEmployee(ctx, id)
	if got.OrgUnit != amt || len(got.Deputies) != 1 || len(got.Absences) != 1 || got.Absences[0].To != "2026-11-06" {
		t.Fatalf("Org/Vertretung/Abwesenheit: %+v", got)
	}
	rid, err := st.CreateReport(ctx, Report{EmployeeID: vid, Target: "Vertreter", Field: "phone", Suggestion: "0815", Reporter: "anna", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := st.CountReports(ctx, "open", "anna"); n != 1 {
		t.Fatalf("CountReports = %d", n)
	}
	if err := st.UpdateReport(ctx, rid, "done", "admin", "erledigt"); err != nil {
		t.Fatal(err)
	}
	if l, _ := st.ListReports(ctx, "done", 10); len(l) != 1 || l[0].ResolvedBy != "admin" || l[0].ResolvedAt == nil {
		t.Fatalf("ListReports: %+v", l)
	}
	if err := st.UpdateReport(ctx, 99999, "done", "x", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateReport unbekannt: %v", err)
	}
	// Vertreter löschen → überall entfernt, Meldung bleibt (ohne Verweis)
	if err := st.DeleteEmployee(ctx, vid); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetEmployee(ctx, id)
	if len(got.Deputies) != 0 || len(got.Absences) != 1 || len(got.Absences[0].Deputies) != 0 {
		t.Fatalf("Verweise nach Löschen: %+v", got)
	}
	units, _ := st.ListOrgUnits(ctx)
	for _, u := range units {
		if u.ID == amt && (len(u.Deputies) != 0 || len(u.Leaders) != 1 || u.Parent != dez || u.Short != "10") {
			t.Fatalf("Einheit nach Löschen: %+v", u)
		}
	}
	if l, _ := st.ListReports(ctx, "", 0); len(l) != 1 || l[0].EmployeeID != 0 || l[0].Target != "Vertreter" {
		t.Fatalf("Meldung nach Löschen: %+v", l)
	}
	// Einheit löschen → Personen wandern zur übergeordneten Einheit
	if err := st.DeleteOrgUnit(ctx, amt); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetEmployee(ctx, id)
	if got.OrgUnit != dez {
		t.Fatalf("nach Löschen der Einheit: %d", got.OrgUnit)
	}
	// AD-Abgleich-Plan
	plan := &SyncPlan{Now: now, Creates: []EmployeeRec{{Username: "jdoe", LastName: "Doe", ADGUID: "g-1", Source: "ad", Active: true, Tags: []int64{}}},
		Updates: []SyncUpdate{{ID: id, Fields: contactFields{LastName: "Muster", FirstName: "Max", Phone: "999"}, GUID: "g-2", DN: "CN=x", Changed: true}}}
	if err := st.ApplySyncPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetEmployee(ctx, id)
	if got.Phone != "999" || got.Source != "ad" || got.ADGUID != "g-2" || len(got.Tags) != 2 {
		t.Fatalf("nach Sync: %+v", got)
	}
	if ok, _ := st.LinkUsernameByGUID(ctx, "g-1", "other"); ok {
		t.Fatal("Benutzername überschrieben")
	}
	if err := st.ApplySyncPlan(ctx, &SyncPlan{Now: now, Deactivate: []int64{id}}); err != nil {
		t.Fatal(err)
	}
	if l, _ := st.ListEmployees(ctx, false); len(l) != 1 {
		t.Fatalf("aktive Einträge: %d", len(l))
	}
	// Konten, Protokoll, Sitzungen
	if err := st.UpsertLocalUser(ctx, LocalUser{Username: "admin", PwHash: "x", IsAdmin: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if u, err := st.GetLocalUser(ctx, "admin"); err != nil || !u.IsAdmin {
		t.Fatal(err)
	}
	_ = st.AddAudit(ctx, AuditEntry{TS: now, Actor: "admin", Action: "login"})
	_ = st.AddAudit(ctx, AuditEntry{TS: now, Actor: "anna", Action: "tags_update", Target: "Muster"})
	if l, _ := st.ListAudit(ctx, "muster", 10); len(l) != 1 {
		t.Fatalf("Audit-Suche: %d", len(l))
	}
	if err := st.CreateSession(ctx, &SessionRec{IDHash: "h1", Username: "admin", CSRF: "c", CreatedAt: now, LastSeen: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if s, _ := st.GetSession(ctx, "h1"); s == nil || s.Username != "admin" {
		t.Fatal("Sitzung fehlt")
	}
	_ = st.PurgeSessions(ctx, now.Add(2*time.Hour), now.Add(-time.Hour))
	if s, _ := st.GetSession(ctx, "h1"); s != nil {
		t.Fatal("abgelaufene Sitzung nicht entfernt")
	}
	// Gesamtabzug ist gültig
	snap, err := st.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.validate(); err != nil {
		t.Fatal(err)
	}
	if len(snap.OrgUnits) != 1 || len(snap.Reports) != 1 || snap.Settings[settingOrgLevels] == "" {
		t.Fatalf("Export Organisation/Meldungen: %+v %+v %+v", snap.OrgUnits, snap.Reports, snap.Settings)
	}
	if len(snap.Categories) != 3 || snap.Private[0].Shares == nil || len(snap.Private[0].Shares.Amts) != 1 {
		t.Fatalf("Export Kategorien/Freigaben: %+v %+v", snap.Categories, snap.Private[0].Shares)
	}
	if len(snap.Employees) != 2 || len(snap.Private) != 1 || len(snap.LocalUsers) != 1 {
		t.Fatalf("Export: %d/%d/%d", len(snap.Employees), len(snap.Private), len(snap.LocalUsers))
	}
}

func TestJSONStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "tb.json")
	st, err := newJSONStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	exerciseStore(t, st)
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("Dateirechte %v", fi.Mode().Perm())
	}
	// Neu laden: alles (außer Sitzungen) muss wieder da sein
	st2, err := newJSONStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := st.Export(context.Background())
	b, _ := st2.Export(context.Background())
	a.ExportedAt, b.ExportedAt = time.Time{}, time.Time{}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatalf("nach Neuladen verschieden:\n%s\n%s", ja, jb)
	}
	// IDs laufen nach dem Laden weiter
	id, err := st2.CreateEmployee(context.Background(), &EmployeeRec{LastName: "Neu", Source: "manual", Active: true})
	if err != nil || id != 5 {
		t.Fatalf("neue ID %d %v", id, err)
	}
	// Kaputte Datei wird abgelehnt, nicht überschrieben
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"format":"telefonbuch","version":1,"employees":[{"id":1,"tags":[42]}]}`), 0o600)
	if _, err := newJSONStore(bad, ""); err == nil {
		t.Fatal("ungültige Datei akzeptiert")
	}
	// Import legt .bak an
	if err := st2.Import(context.Background(), &Snapshot{Format: snapshotFormat, Version: 1, Source: "mysql"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatal("keine Sicherung angelegt")
	}
}

func TestMySQLStore(t *testing.T) {
	dsn := os.Getenv("TB_TEST_DSN")
	if dsn == "" {
		t.Skip("TB_TEST_DSN nicht gesetzt")
	}
	c := defaultConfig()
	c.DB.DSN = dsn
	st, err := newMySQLStore(c, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	exerciseStore(t, st)
	// Rundreise MySQL → JSON → MySQL
	js, err := newJSONStore(filepath.Join(t.TempDir(), "tb.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := copyStore(context.Background(), st, js); err != nil {
		t.Fatal(err)
	}
	if _, err := copyStore(context.Background(), js, st); err != nil {
		t.Fatal(err)
	}
	l, _ := st.ListEmployees(context.Background(), true)
	if len(l) != 2 {
		t.Fatalf("nach Rundreise %d Einträge", len(l))
	}
}

func TestSyncPlanGuard(t *testing.T) {
	cfg := defaultConfig().Sync
	cur := []EmployeeRec{}
	for i := 0; i < 10; i++ {
		cur = append(cur, EmployeeRec{ID: int64(i + 1), Username: "u" + string(rune('a'+i)), LastName: "X", ADGUID: "g" + string(rune('a'+i)), Source: "ad", Active: true})
	}
	users := []LDAPUser{{GUID: "ga", Username: "ua", Fields: map[string]string{"lastName": "X"}}}
	res := &SyncResult{}
	if p := buildSyncPlan(users, cur, &cfg, res); p != nil {
		t.Fatal("90 % Deaktivierung hätte abgebrochen werden müssen")
	}
	users = nil
	for i := 0; i < 9; i++ {
		users = append(users, LDAPUser{GUID: "g" + string(rune('a'+i)), Username: "u" + string(rune('a'+i)), Fields: map[string]string{"lastName": "X", "phone": "1"}})
	}
	users = append(users, LDAPUser{GUID: "gz", Username: "uz", Fields: map[string]string{"lastName": "Neu"}})
	res = &SyncResult{}
	p := buildSyncPlan(users, cur, &cfg, res)
	if p == nil || len(p.Creates) != 1 || len(p.Deactivate) != 1 || res.Updated != 9 {
		t.Fatalf("Plan: %+v res %+v", p, res)
	}
}

func TestJSONAuditSeparateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tb.json")
	st, err := newJSONStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := st.CreateEmployee(ctx, &EmployeeRec{LastName: "A", Source: "manual", Active: true}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	for i := 0; i < 3; i++ {
		_ = st.AddAudit(ctx, AuditEntry{TS: time.Now().UTC(), Actor: "x", Action: "login"})
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("Protokolleintrag hat die Datendatei verändert")
	}
	if strings.Contains(string(after), `"audit"`) {
		t.Fatal("Datendatei enthält Protokoll")
	}
	ap := filepath.Join(dir, "tb.audit.json")
	var af auditFile
	b, err := os.ReadFile(ap)
	if err != nil || json.Unmarshal(b, &af) != nil || len(af.Entries) != 3 || af.Format != auditFormat {
		t.Fatalf("Protokolldatei: %v %+v", err, af)
	}
	if fi, _ := os.Stat(ap); fi.Mode().Perm() != 0o600 {
		t.Fatal("Rechte der Protokolldatei")
	}
	// Export enthält das Protokoll, Neuladen liest es wieder
	snap, _ := st.Export(ctx)
	if len(snap.Audit) != 3 {
		t.Fatalf("Export-Protokoll: %d", len(snap.Audit))
	}
	st2, _ := newJSONStore(path, "")
	if l, _ := st2.ListAudit(ctx, "", 0); len(l) != 3 {
		t.Fatalf("nach Neuladen %d Einträge", len(l))
	}
	// Altes Format (Protokoll in der Datendatei) wird verschoben
	legacy := filepath.Join(dir, "alt.json")
	os.WriteFile(legacy, []byte(`{"format":"telefonbuch","version":1,"employees":[],"audit":[{"ts":"2026-01-01T00:00:00Z","actor":"a","action":"login","target":"","detail":"","ip":""}]}`), 0o600)
	st3, err := newJSONStore(legacy, "")
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := st3.ListAudit(ctx, "", 0); len(l) != 1 {
		t.Fatal("altes Protokoll verloren")
	}
	if b, _ := os.ReadFile(legacy); strings.Contains(string(b), `"audit"`) {
		t.Fatal("altes Protokoll nicht aus der Datendatei entfernt")
	}
	if _, err := os.Stat(filepath.Join(dir, "alt.audit.json")); err != nil {
		t.Fatal("Protokolldatei nicht angelegt")
	}
}

func TestFallbackReadOnly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "kopie.json")
	js, err := newJSONStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.CreateEmployee(ctx, &EmployeeRec{LastName: "Reserve", Source: "manual", Active: true}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)

	c := defaultConfig()
	c.DB.DSN = "root@tcp(127.0.0.1:1)/x" // nicht erreichbar
	c.Storage.JSONPath = path
	c.Auth.Mode = "local"
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	fb, err := openFallbackStore(c)
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := fb.ListEmployees(ctx, false); len(l) != 1 || l[0].LastName != "Reserve" {
		t.Fatalf("Lesen im Rückfall: %+v", l)
	}
	if _, err := fb.CreateEmployee(ctx, &EmployeeRec{LastName: "X"}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Schreiben im Rückfall: %v", err)
	}
	if err := fb.UpsertLocalUser(ctx, LocalUser{Username: "devadmin", PwHash: "x", IsAdmin: true}); err != nil {
		t.Fatal(err)
	}
	if err := fb.CreateSession(ctx, &SessionRec{IDHash: "s", Username: "devadmin", CSRF: "c", CreatedAt: time.Now().UTC(), LastSeen: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("JSON-Kopie wurde im Rückfall verändert")
	}

	// HTTP: ändernde Anfragen werden abgewiesen, Lesen geht
	app := &App{cfg: c, ctx: ctx, limiter: newLimiter(5, 5, time.Minute)}
	app.setStore(fb)
	app.offline.Store(true)
	h := app.routes(false)
	for _, tc := range []struct {
		method, path string
		want         int
	}{{"POST", "/api/employees", 503}, {"PUT", "/api/categories/x", 503}, {"GET", "/api/directory", 200}, {"GET", "/api/site", 200}} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s %s = %d, erwartet %d", tc.method, tc.path, rec.Code, tc.want)
		}
		if tc.path == "/api/site" && !strings.Contains(rec.Body.String(), `"offline":true`) {
			t.Fatalf("site ohne offline-Flag: %s", rec.Body.String())
		}
	}

	// Zurückschalten, sobald MySQL erreichbar ist
	if dsn := os.Getenv("TB_TEST_DSN"); dsn != "" {
		c.DB.DSN = dsn
		if !app.tryReconnect(ctx) {
			t.Fatal("kein Zurückschalten")
		}
		if app.offline.Load() || app.st().Kind() != "mysql" {
			t.Fatal("nach Zurückschalten noch offline")
		}
		if s, _ := app.st().GetSession(ctx, "s"); s == nil {
			t.Fatal("Sitzung nicht übernommen")
		}
		app.st().Close()
	}
}
