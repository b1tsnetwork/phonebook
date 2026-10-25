package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"strings"
	"time"
)

// seedDemo füllt ein leeres Telefonbuch mit Beispieldaten und legt zwei lokale Konten an.
func seedDemo(a *App) error {
	ctx := context.Background()
	existing, err := a.st().ListEmployees(ctx, true)
	if err != nil {
		return err
	}
	n := len(existing)
	pw := os.Getenv("TB_DEMO_PASSWORD")
	gen := pw == ""
	if gen {
		pw = randomToken(9)
	}
	h, err := hashPassword(pw)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, u := range []struct {
		user, name string
		admin      bool
	}{{"admin", "Administrator", true}, {"mmuster", "Max Muster", false}} {
		if err := a.st().UpsertLocalUser(ctx, LocalUser{Username: u.user, DisplayName: u.name, PwHash: h, IsAdmin: u.admin, CreatedAt: now}); err != nil {
			return err
		}
	}
	if gen {
		log.Printf("╔════════════════════════════════════════════════════════╗")
		log.Printf("║ Demo-Zugänge: admin bzw. mmuster  –  Passwort: %-8s║", pw)
		log.Printf("╚════════════════════════════════════════════════════════╝")
	} else {
		log.Printf("Demo-Zugänge: admin (Admin) und mmuster – Passwort aus TB_DEMO_PASSWORD")
	}
	if n > 0 {
		return nil
	}

	amter := []string{"10 – Hauptamt", "11 – Personalamt", "12 – IT-Abteilung", "20 – Kämmerei", "32 – Ordnungsamt", "40 – Schulamt", "50 – Sozialamt", "61 – Stadtplanung", "63 – Bauordnung", "66 – Tiefbauamt"}
	tagsT := map[string][]string{
		"12 – IT-Abteilung": {"Active Directory", "Firewall", "Netzwerk", "VMware", "Backup", "Telefonanlage", "Drucker", "Microsoft 365", "Linux", "Fachverfahren", "Datenbanken", "Helpdesk", "Informationssicherheit"},
		"20 – Kämmerei":     {"Haushalt", "Kasse", "Steuern", "Mahnwesen", "Anlagenbuchhaltung"},
		"11 – Personalamt":  {"Lohnbuchhaltung", "Stellenausschreibungen", "Zeiterfassung", "Fortbildung"},
		"32 – Ordnungsamt":  {"Gewerbeamt", "Bußgeldstelle", "Fundbüro", "Veranstaltungen"},
		"63 – Bauordnung":   {"Bauanträge", "Denkmalschutz", "Bauaufsicht"},
		"61 – Stadtplanung": {"Bebauungspläne", "GIS", "Verkehrsplanung"},
		"10 – Hauptamt":     {"Ratsangelegenheiten", "Poststelle", "Vergabestelle", "Datenschutz"},
		"40 – Schulamt":     {"Schulverwaltung", "Schülerbeförderung", "Kita-Plätze"},
		"50 – Sozialamt":    {"Wohngeld", "Grundsicherung", "Asyl"},
		"66 – Tiefbauamt":   {"Straßenbau", "Winterdienst", "Kanalnetz"},
	}
	besonder := []string{"Ersthelfer", "Brandschutzhelfer", "Englisch", "Französisch", "Türkisch", "Gebärdensprache", "Personalrat", "Ausbilder", "Rufbereitschaft", "Schwerbehindertenvertretung"}
	last := []string{"Müller", "Schmidt", "Schneider", "Fischer", "Weber", "Meyer", "Wagner", "Becker", "Schulz", "Hoffmann", "Koch", "Richter", "Klein", "Wolf", "Schröder", "Neumann", "Schwarz", "Zimmermann", "Braun", "Krüger", "Hofmann", "Hartmann", "Lange", "Schmitt", "Werner", "Krause", "Meier", "Lehmann", "Schmid", "Schulze", "Maier", "Köhler", "Herrmann", "König", "Walter", "Mayer", "Huber", "Kaiser", "Fuchs", "Peters", "Yılmaz", "Nowak", "Öztürk", "Kowalski"}
	first := []string{"Anna", "Lukas", "Lea", "Jonas", "Sophie", "Leon", "Marie", "Finn", "Emma", "Paul", "Mia", "Elias", "Laura", "Noah", "Hannah", "Felix", "Julia", "Ben", "Lena", "Tim", "Sarah", "David", "Lisa", "Jan", "Nina", "Tobias", "Katharina", "Stefan", "Sabine", "Thomas", "Petra", "Michael", "Andrea", "Ayşe", "Mehmet", "Fatma", "Piotr", "Olga", "Kai", "Sascha"}
	rng := rand.New(rand.NewPCG(42, 2026))
	tagID := map[string]int64{}
	ensureTag := func(name, cat string) int64 {
		if id, ok := tagID[name]; ok {
			return id
		}
		id, err := a.st().CreateTag(ctx, name, cat, "demo")
		if err != nil {
			log.Printf("Tag %s: %v", name, err)
			return 0
		}
		tagID[name] = id
		return id
	}
	avatars := []string{
		`{"v":2,"skin":"#ecbf96","hair":"#6b4224","eyes":"#3f7f4f","accent":"#3a86ff","glassesColor":"#3a3a3a","hairStyle":"long","eyeStyle":"classic","browStyle":"normal","mouthStyle":"bigsmile","noseStyle":"button","beardStyle":"none","glassesStyle":"round","hat":"none","ears":"small","extras":["freckles","studs"],"texture":true}`,
		`{"v":2,"skin":"#8a5534","hair":"#151515","eyes":"#3b2a1a","accent":"#e63946","glassesColor":"#3a3a3a","hairStyle":"short","eyeStyle":"classic","browStyle":"thick","mouthStyle":"grin","noseStyle":"none","beardStyle":"full","glassesStyle":"none","hat":"headset","ears":"normal","extras":[],"texture":true}`,
		`{"v":2,"skin":"#ffe3cc","hair":"#ecd28c","eyes":"#4a6a8a","accent":"#ef476f","glassesColor":"#3a3a3a","hairStyle":"bob","eyeStyle":"lashes","browStyle":"normal","mouthStyle":"smile","noseStyle":"none","beardStyle":"none","glassesStyle":"none","hat":"flower","ears":"none","extras":["blush"],"texture":true,"lips":"#c9184a"}`,
		`{"v":2,"skin":"#c98e62","hair":"#4a2e1a","eyes":"#5a4632","accent":"#8a8d91","glassesColor":"#3a3a3a","hairStyle":"curly","eyeStyle":"classic","browStyle":"normal","mouthStyle":"neutral","noseStyle":"long","beardStyle":"stubble","glassesStyle":"nerd","hat":"none","ears":"none","extras":[],"texture":true}`,
		`{"v":2,"skin":"#a7a9ac","hair":"#2a9df4","eyes":"#151515","accent":"#8338ec","glassesColor":"#3a3a3a","hairStyle":"mohawk","eyeStyle":"happy","browStyle":"normal","mouthStyle":"grin","noseStyle":"none","beardStyle":"none","glassesStyle":"pixel","hat":"none","ears":"none","extras":[],"texture":true}`,
		`{"v":2,"skin":"#6b4027","hair":"#151515","eyes":"#3b2a1a","accent":"#06d6a0","glassesColor":"#3a3a3a","hairStyle":"afro","eyeStyle":"classic","browStyle":"normal","mouthStyle":"laugh","noseStyle":"none","beardStyle":"none","glassesStyle":"none","hat":"headphones","ears":"normal","extras":[],"texture":true}`,
		`{"v":2,"skin":"#e0a97d","hair":"#b5452b","eyes":"#3a86ff","accent":"#ffd166","glassesColor":"#3a3a3a","hairStyle":"braids","eyeStyle":"sparkle","browStyle":"normal","mouthStyle":"smile","noseStyle":"none","beardStyle":"none","glassesStyle":"none","hat":"none","ears":"none","extras":["freckles"],"texture":true}`,
		`{"v":2,"skin":"#f6cfa9","hair":"#a3a8ad","eyes":"#4a6a8a","accent":"#8a8d91","glassesColor":"#3a3a3a","hairStyle":"balding","eyeStyle":"classic","browStyle":"bushy","mouthStyle":"smirk","noseStyle":"none","beardStyle":"vandyke","glassesStyle":"aviator","hat":"none","ears":"none","extras":["wrinkles"],"texture":true}`,
		`{"v":2,"skin":"#a36a43","hair":"#2b1a10","eyes":"#3b2a1a","accent":"#2a9d8f","glassesColor":"#3a3a3a","hairStyle":"headscarf","eyeStyle":"almond","browStyle":"normal","mouthStyle":"smile","noseStyle":"none","beardStyle":"none","glassesStyle":"none","hat":"none","ears":"none","extras":[],"texture":true}`,
		`{"v":2,"skin":"#a7a9ac","hair":"#3fbf5f","eyes":"#d4a017","accent":"#8a8d91","glassesColor":"#3a3a3a","hairStyle":"spiky","eyeStyle":"cat","browStyle":"normal","mouthStyle":"cat","noseStyle":"none","beardStyle":"none","glassesStyle":"none","hat":"catears","ears":"elf","extras":[],"texture":true}`,
	}
	ext := 100
	for i := 0; i < 64; i++ {
		ln, fn := last[rng.IntN(len(last))], first[rng.IntN(len(first))]
		amt := amter[rng.IntN(len(amter))]
		if i < 14 {
			amt = "12 – IT-Abteilung"
		}
		user := ""
		if i == 0 {
			ln, fn, user = "Muster", "Max", "mmuster"
		}
		ext += 1 + rng.IntN(7)
		phone := fmt.Sprintf("0123 456-%d", ext)
		mobile := ""
		if rng.IntN(3) == 0 {
			mobile = fmt.Sprintf("0171 %07d", rng.IntN(9999999))
		}
		room := fmt.Sprintf("%s %d.%02d", []string{"A", "B", "C"}[rng.IntN(3)], rng.IntN(4), 1+rng.IntN(30))
		email := fmt.Sprintf("%s.%s@stadt.example", asciiLower(fn), asciiLower(ln))
		e := &EmployeeRec{Username: user, LastName: ln, FirstName: fn, Phone: phone, Mobile: mobile, Room: room, Amt: amt, Email: email,
			Source: "manual", Active: true, CreatedAt: now, UpdatedAt: now, Tags: []int64{}}
		if rng.IntN(3) == 0 {
			e.Avatar = json.RawMessage(avatars[rng.IntN(len(avatars))])
		}
		id, err := a.st().CreateEmployee(ctx, e)
		if err != nil {
			return err
		}
		pool := tagsT[amt]
		k := 1 + rng.IntN(4)
		used := map[int64]bool{}
		for j := 0; j < k && len(pool) > 0; j++ {
			used[ensureTag(pool[rng.IntN(len(pool))], "taetigkeit")] = true
		}
		if rng.IntN(3) == 0 {
			used[ensureTag(besonder[rng.IntN(len(besonder))], "besonderheit")] = true
		}
		var tin []TagInput
		for tid := range used {
			for name, x := range tagID {
				if x == tid && tid != 0 {
					tin = append(tin, TagInput{Name: name})
				}
			}
		}
		if _, err := a.st().SetEmployeeTags(ctx, id, tin, "demo", false); err != nil {
			return err
		}
	}
	// Einrichtungs-Einträge mit einfarbigem Block als Avatar
	for i, x := range []struct{ name, color, phone, tag string }{{"Grundschule am Park", "#3b7dd8", "0123 789-100", "Schulsekretariat"}, {"Stadtbibliothek", "#2a9d8f", "0123 789-200", "Fernleihe"}} {
		block := fmt.Sprintf(`{"v":2,"skin":"%[1]s","hair":"%[1]s","eyes":"#3d3f42","accent":"#8a8d91","glassesColor":"#3a3a3a","hairStyle":"bare","eyeStyle":"none","browStyle":"none","mouthStyle":"none","noseStyle":"none","beardStyle":"none","glassesStyle":"none","hat":"none","ears":"none","extras":[],"texture":false}`, x.color)
		e := &EmployeeRec{LastName: x.name, Phone: x.phone, Room: fmt.Sprintf("Außenstelle %d", i+1), Amt: "40 – Schulamt", Source: "manual", Active: true, CreatedAt: now, UpdatedAt: now, Tags: []int64{}, Avatar: json.RawMessage(block)}
		id, err := a.st().CreateEmployee(ctx, e)
		if err != nil {
			return err
		}
		ensureTag(x.tag, "sonstiges")
		if _, err := a.st().SetEmployeeTags(ctx, id, []TagInput{{Name: x.tag}}, "demo", false); err != nil {
			return err
		}
	}
	if err := seedOrg(ctx, a, amter); err != nil {
		return err
	}
	log.Printf("Demo-Daten angelegt (64 Einträge + 2 Einrichtungen)")
	return nil
}

func asciiLower(s string) string {
	out := []rune{}
	for _, r := range s {
		switch r {
		case 'ä', 'Ä':
			out = append(out, 'a', 'e')
		case 'ö', 'Ö':
			out = append(out, 'o', 'e')
		case 'ü', 'Ü':
			out = append(out, 'u', 'e')
		case 'ß':
			out = append(out, 's', 's')
		case 'ş', 'Ş':
			out = append(out, 's')
		case 'ı', 'İ':
			out = append(out, 'i')
		default:
			if r >= 'A' && r <= 'Z' {
				r += 32
			}
			if r >= 'a' && r <= 'z' {
				out = append(out, r)
			}
		}
	}
	return string(out)
}

// seedOrg: Beispiel-Organigramm (Dezernat › Amt › Sachgebiet) mit Leitungs-Schlagworten,
// Zuordnung nach Amt-Text, Vertretungen und einer Abwesenheit.
func seedOrg(ctx context.Context, a *App, amter []string) error {
	st := a.st()
	tagID := func(name string) int64 {
		id, err := st.CreateTag(ctx, name, "sonstiges", "demo")
		if err != nil {
			return 0
		}
		return id
	}
	levels := []OrgLevel{
		{Name: "Dezernat", LeaderTag: tagID("Dezernatsleitung")},
		{Name: "Amt", LeaderTag: tagID("Amtsleitung"), DeputyTag: tagID("Stv. Amtsleitung")},
		{Name: "Sachgebiet", LeaderTag: tagID("Sachgebietsleitung")},
		{Name: "Team"},
	}
	b, _ := json.Marshal(levels)
	if err := st.SetSetting(ctx, settingOrgLevels, string(b)); err != nil {
		return err
	}
	dez := map[string]int64{}
	for i, d := range []string{"Verwaltung", "Finanzen & Soziales", "Bauen"} {
		id, err := st.CreateOrgUnit(ctx, OrgUnit{Name: d, Short: []string{"I", "II", "III"}[i], Sort: i + 1})
		if err != nil {
			return err
		}
		dez[[]string{"I", "II", "III"}[i]] = id
	}
	parentOf := map[string]string{"1": "I", "2": "II", "3": "I", "4": "II", "5": "II", "6": "III"}
	recs, err := st.ListEmployees(ctx, true)
	if err != nil {
		return err
	}
	leadSet := func(e EmployeeRec, tag string) {
		var tin []TagInput
		for _, t := range e.Tags {
			if tg, err := st.ListTags(ctx); err == nil {
				for _, x := range tg {
					if x.ID == t {
						tin = append(tin, TagInput{Name: x.Name})
					}
				}
			}
		}
		tin = append(tin, TagInput{Name: tag})
		_, _ = st.SetEmployeeTags(ctx, e.ID, tin, "demo", true)
	}
	for i, amt := range amter {
		num, name, _ := strings.Cut(amt, " – ")
		uid, err := st.CreateOrgUnit(ctx, OrgUnit{Parent: dez[parentOf[num[:1]]], Name: name, Short: num, Sort: i})
		if err != nil {
			return err
		}
		var members []EmployeeRec
		for _, e := range recs {
			if e.Amt == amt {
				members = append(members, e)
			}
		}
		ids := make([]int64, len(members))
		for j, e := range members {
			ids[j] = e.ID
		}
		if _, err := st.SetOrgUnit(ctx, ids, uid); err != nil {
			return err
		}
		if len(members) > 2 {
			leadSet(members[1], "Amtsleitung")
			leadSet(members[2], "Stv. Amtsleitung")
			_ = st.SetDeputies(ctx, members[1].ID, []int64{members[2].ID})
		}
		if num == "12" && len(members) > 6 { // Sachgebiete der IT
			for k, sg := range []string{"Infrastruktur", "Anwendungen & Support"} {
				sid, err := st.CreateOrgUnit(ctx, OrgUnit{Parent: uid, Name: sg, Short: fmt.Sprintf("12.%d", k+1), Sort: k})
				if err != nil {
					return err
				}
				part := members[3+k*((len(members)-3)/2) : 3+(k+1)*((len(members)-3)/2)]
				pid := make([]int64, len(part))
				for j, e := range part {
					pid[j] = e.ID
				}
				_, _ = st.SetOrgUnit(ctx, pid, sid)
				if len(part) > 0 {
					leadSet(part[0], "Sachgebietsleitung")
				}
			}
		}
	}
	// Max Muster: Vertretung und eine aktuelle Abwesenheit
	if mid, _ := st.EmployeeIDByUsername(ctx, "mmuster"); mid != 0 && len(recs) > 5 {
		_ = st.SetDeputies(ctx, mid, []int64{recs[3].ID, recs[4].ID})
		today := a.cfg.today()
		t, _ := time.Parse("2006-01-02", today)
		_ = st.SetAbsences(ctx, mid, []Absence{
			{From: today, To: t.AddDate(0, 0, 4).Format("2006-01-02"), Deputies: []int64{recs[5].ID}},
			{From: t.AddDate(0, 0, 21).Format("2006-01-02"), To: t.AddDate(0, 0, 25).Format("2006-01-02")},
		})
	}
	return nil
}
