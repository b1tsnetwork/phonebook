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
