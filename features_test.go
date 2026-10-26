package main

import "testing"

func TestUnitDepthsAndAbsences(t *testing.T) {
	d := unitDepths([]OrgUnit{{ID: 1}, {ID: 2, Parent: 1}, {ID: 3, Parent: 2}, {ID: 4, Parent: 99}})
	if d[1] != 1 || d[2] != 2 || d[3] != 3 || d[4] != 1 {
		t.Fatalf("Tiefen: %v", d)
	}
	cur := currentAbsences([]Absence{{From: "2026-11-01", To: "2026-11-02"}, {From: "2026-01-01", To: "2026-01-05"}, {From: "2026-10-20", To: "2026-10-25"}}, "2026-10-25")
	if len(cur) != 2 || cur[0].From != "2026-10-20" {
		t.Fatalf("aktuelle Abwesenheiten: %+v", cur)
	}
	if _, ok := parseDay("2026-02-30"); ok {
		t.Fatal("ungültiges Datum akzeptiert")
	}
}

func TestMigrateUnitContacts(t *testing.T) {
	ctx := t.Context()
	st, err := newJSONStore(t.TempDir()+"/tb.json", "")
	if err != nil {
		t.Fatal(err)
	}
	it, _ := st.CreateOrgUnit(ctx, OrgUnit{Name: "IT", Short: "12", Contact: &ContactPref{TicketURL: "https://t.example/", HideDirect: true}})
	sg, _ := st.CreateOrgUnit(ctx, OrgUnit{Parent: it, Name: "Support"})
	other, _ := st.CreateOrgUnit(ctx, OrgUnit{Name: "Kämmerei"})
	e1, _ := st.CreateEmployee(ctx, &EmployeeRec{LastName: "A", Active: true, OrgUnit: sg})
	e2, _ := st.CreateEmployee(ctx, &EmployeeRec{LastName: "B", Active: true, OrgUnit: other})
	migrateUnitContacts(ctx, st)
	ch, _ := st.ListChannels(ctx)
	if len(ch) != 1 || ch[0].Name != "12 – IT" || !ch[0].HideDirect {
		t.Fatalf("Kontaktwege: %+v", ch)
	}
	a, _ := st.GetEmployee(ctx, e1)
	b, _ := st.GetEmployee(ctx, e2)
	if a.Channel != ch[0].ID || b.Channel != 0 {
		t.Fatalf("Zuweisung: %d %d", a.Channel, b.Channel)
	}
	us, _ := st.ListOrgUnits(ctx)
	for _, u := range us {
		if u.Contact != nil {
			t.Fatal("Einheit hat noch Kontaktweg")
		}
	}
	migrateUnitContacts(ctx, st) // zweiter Lauf: nichts mehr zu tun
	if ch, _ := st.ListChannels(ctx); len(ch) != 1 {
		t.Fatal("doppelt übernommen")
	}
}

func TestFunGlobalAndPrefs(t *testing.T) {
	ctx := t.Context()
	st, err := newJSONStore(t.TempDir()+"/tb.json", "")
	if err != nil {
		t.Fatal(err)
	}
	a := &App{cfg: defaultConfig()}
	a.setStore(st)
	if g := a.funGlobal(ctx); !g["village"] || !g["terminal"] {
		t.Fatalf("Standard: alle freigegeben, %v", g)
	}
	_ = st.SetSetting(ctx, settingFun, `{"village":false}`)
	if g := a.funGlobal(ctx); g["village"] || !g["quiz"] {
		t.Fatalf("Freigabe: %v", g)
	}
	p := a.loadPrefs(ctx, "anna")
	if p.public()["fun"].(map[string]bool)["quiz"] {
		t.Fatal("persönlich muss standardmäßig aus sein")
	}
	p.Fun = map[string]bool{"badges": true}
	_ = a.savePrefs(ctx, "anna", p)
	_ = st.SetSetting(ctx, settingFun, `{}`)
	if !a.badgeUsers(ctx)["anna"] {
		t.Fatal("Abzeichen-Opt-in nicht erkannt")
	}
	_ = st.SetSetting(ctx, settingFun, `{"badges":false}`)
	if a.badgeUsers(ctx)["anna"] {
		t.Fatal("global abgeschaltet, trotzdem Abzeichen")
	}
	if hashFeedToken("x") == hashFeedToken("y") || len(hashFeedToken("x")) != 64 {
		t.Fatal("Feed-Hash")
	}
}

func TestMissingContactAutoClose(t *testing.T) {
	ctx := t.Context()
	st, err := newJSONStore(t.TempDir()+"/tb.json", "")
	if err != nil {
		t.Fatal(err)
	}
	a := &App{cfg: defaultConfig()}
	a.setStore(st)
	d := &NewContact{LastName: "Groß-Müller", FirstName: "Lena", Amt: "20"}
	rid, _ := st.CreateReport(ctx, Report{Target: "Groß-Müller, Lena", Field: fieldMissing, Reporter: "anna", Details: d})
	if n := a.autoCloseReports(ctx, "test"); n != 0 {
		t.Fatal("ohne Eintrag erledigt")
	}
	if _, why := a.psCommand(Report{Field: fieldMissing}, nil); why == "" {
		t.Fatal("kein Grund für fehlenden PowerShell-Befehl")
	}
	// ausgeblendet zählt nicht als „im Telefonbuch“
	id, _ := st.CreateEmployee(ctx, &EmployeeRec{LastName: "Gross Müller", FirstName: "lena", Active: true, Hidden: true})
	if n := a.autoCloseReports(ctx, "test"); n != 0 {
		t.Fatal("ausgeblendeter Eintrag hat Meldung erledigt")
	}
	emps, _ := st.ListEmployees(ctx, true)
	if findPerson(d, emps, false) == nil {
		t.Fatal("Schreibweise (ß/Bindestrich/Groß-klein) nicht erkannt")
	}
	e, _ := st.GetEmployee(ctx, id)
	e.Hidden = false
	if err := st.UpdateEmployee(ctx, e); err != nil {
		t.Fatal(err)
	}
	if n := a.autoCloseReports(ctx, "test"); n != 1 {
		t.Fatalf("nicht automatisch erledigt: %d", n)
	}
	l, _ := st.ListReports(ctx, "done", 0)
	if len(l) != 1 || l[0].ID != rid || l[0].Note == "" {
		t.Fatalf("Meldung: %+v", l)
	}
}

func TestNameRanges(t *testing.T) {
	if foldName("Müller-Lüdenscheidt") != "MULLERLUDENSCHEIDT" || foldName("Groß") != "GROSS" || foldName("Çelik") != "CELIK" || foldName("O'Neill") != "ONEILL" {
		t.Fatalf("foldName: %q %q %q", foldName("Müller-Lüdenscheidt"), foldName("Groß"), foldName("Çelik"))
	}
	if f, to, err := cleanRange("sch", "SZ"); err != nil || f != "Sch" || to != "Sz" {
		t.Fatalf("cleanRange: %q %q %v", f, to, err)
	}
	if _, _, err := cleanRange("M", "A"); err == nil {
		t.Fatal("verkehrter Bereich akzeptiert")
	}
	if _, _, err := cleanRange("A1", "B"); err != nil {
		t.Fatal("Ziffern sollten wegfallen") // „A1“ → „A“
	}
	rules := []TaskRule{{From: "A", To: "M"}, {From: "N", To: "Sch"}, {From: "Sci", To: "Z"}, {}}
	for name, want := range map[string]int{"Ahrens": 0, "Mzyk": 0, "Nagel": 1, "Schulz": 1, "Schwarz": 1, "Scivoli": 2, "Sander": 1, "Ötztal": 1, "Zander": 2, "Ägidius": 0, "-": 3} {
		if got := matchRule(rules, name); got != want {
			t.Errorf("matchRule(%q) = %d, erwartet %d", name, got, want)
		}
	}
	if _, _, ok := overlap([]TaskRule{{From: "A", To: "K"}, {From: "K", To: "Z"}}); !ok {
		t.Fatal("Überschneidung A–K/K–Z nicht erkannt")
	}
	if _, _, ok := overlap([]TaskRule{{From: "A", To: "Sch"}, {From: "Sci", To: "Z"}}); ok {
		t.Fatal("A–Sch/Sci–Z fälschlich als Überschneidung")
	}
	h, err := cleanHours(&Hours{Slots: []HourSlot{{Days: []int{4, 1, 1}, From: "08:00", To: "12:00"}}})
	if err != nil || len(h.Slots[0].Days) != 2 || h.Slots[0].Days[0] != 1 {
		t.Fatalf("cleanHours: %+v %v", h, err)
	}
	if _, err := cleanHours(&Hours{Slots: []HourSlot{{Days: []int{1}, From: "12:00", To: "08:00"}}}); err == nil {
		t.Fatal("Ende vor Beginn akzeptiert")
	}
	if h, _ := cleanHours(&Hours{}); h != nil {
		t.Fatal("leere Sprechzeiten sollten nil sein")
	}
}
