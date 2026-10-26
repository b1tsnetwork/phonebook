package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Zuständigkeitsfinder: „Wer ist zuständig?“ – Aufgaben (z. B. Wohngeld) mit Namenstrennung nach
// Anfangsbuchstaben des Nachnamens (A–K, L–Z, auch Sch–Sz). Je Bereich mehrere Zuständige und
// mehrere Vertretungen; eine Person kann für viele Aufgaben/Bereiche zuständig sein (n:m).
// Dazu Sprechzeiten je Organisationseinheit, die eine Aufgabe übernimmt oder selbst festlegt.

const (
	maxTasks      = 1000
	maxTaskRules  = 60
	maxRulePeople = 20
	maxHourSlots  = 21
	rangeKeyLen   = 4 // Buchstaben, die für die Namenstrennung zählen
)

// foldName: Nachname → Großbuchstaben A–Z nach DIN 5007-1 (Ä=A, Ö=O, Ü=U, ß=SS),
// Akzente werden entfernt, alles andere (Leerzeichen, Bindestriche, Apostrophe) fällt weg.
func foldName(s string) string {
	var b strings.Builder
	for _, r := range s {
		r = unicode.ToUpper(r)
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r == 'ß' || r == 'ẞ':
			b.WriteString("SS")
		default:
			if x, ok := foldMap[r]; ok {
				b.WriteString(x)
			}
		}
	}
	return b.String()
}

var foldMap = func() map[rune]string {
	m := map[rune]string{}
	for base, list := range map[string]string{
		"A": "ÀÁÂÃÄÅĀĂĄ", "C": "ÇĆĈĊČ", "D": "ĎĐ", "E": "ÈÉÊËĒĔĖĘĚ", "G": "ĜĞĠĢ", "H": "ĤĦ", "I": "ÌÍÎÏĨĪĬĮİ",
		"J": "Ĵ", "K": "Ķ", "L": "ĹĻĽĿŁ", "N": "ÑŃŅŇ", "O": "ÒÓÔÕÖØŌŎŐ", "R": "ŔŖŘ", "S": "ŚŜŞŠȘ", "T": "ŢŤŦȚ",
		"U": "ÙÚÛÜŨŪŬŮŰŲ", "W": "Ŵ", "Y": "ÝŶŸ", "Z": "ŹŻŽ", "AE": "Æ", "OE": "Œ",
	} {
		for _, r := range list {
			m[r] = base
		}
	}
	return m
}()

// rangeBounds: Bereich „A“–„K“ als vergleichbare Schlüssel (von mit A, bis mit Z aufgefüllt)
func rangeBounds(from, to string) (string, string) {
	pad := func(s string, c byte) string {
		for len(s) < rangeKeyLen {
			s += string(c)
		}
		return s[:rangeKeyLen]
	}
	return pad(from, 'A'), pad(to, 'Z')
}

// nameKey: die ersten vier Buchstaben des gefalteten Nachnamens (kürzere mit A aufgefüllt)
func nameKey(last string) string {
	k := foldName(last)
	if k == "" {
		return ""
	}
	for len(k) < rangeKeyLen {
		k += "A"
	}
	return k[:rangeKeyLen]
}

var reRange = regexp.MustCompile(`^[A-Z]{1,4}$`)

// cleanRange: Buchstabenbereich prüfen und normalisieren (leer/leer = alle Namen)
func cleanRange(from, to string) (string, string, error) {
	f, t := foldName(from), foldName(to)
	if f == "" && t == "" {
		if strings.TrimSpace(from+to) != "" {
			return "", "", errors.New("Namenstrennung: bitte nur Buchstaben angeben (z. B. A bis K oder Sch bis Sz)")
		}
		return "", "", nil
	}
	if f == "" {
		f = "A"
	}
	if t == "" {
		t = "Z"
	}
	if !reRange.MatchString(f) || !reRange.MatchString(t) {
		return "", "", errors.New("Namenstrennung: je 1–4 Buchstaben (z. B. A bis K oder Sch bis Sz)")
	}
	lo, hi := rangeBounds(f, t)
	if lo > hi {
		return "", "", fmt.Errorf("Namenstrennung: „%s“ liegt hinter „%s“", titleCase(f), titleCase(t))
	}
	return titleCase(f), titleCase(t), nil
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return s[:1] + strings.ToLower(s[1:])
}

var reClock = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$|^24:00$`)

// cleanHours: Sprechzeiten prüfen; leer → nil
func cleanHours(h *Hours) (*Hours, error) {
	if h == nil {
		return nil, nil
	}
	note, ok := cleanText(h.Note, 200)
	if !ok || strings.ContainsAny(note, "<>") {
		return nil, errors.New("Hinweis zu den Sprechzeiten zu lang (max. 200 Zeichen)")
	}
	if len(h.Slots) > maxHourSlots {
		return nil, fmt.Errorf("Höchstens %d Zeilen Sprechzeiten", maxHourSlots)
	}
	out := &Hours{Note: note}
	for _, s := range h.Slots {
		seen := map[int]bool{}
		var days []int
		for _, d := range s.Days {
			if d < 1 || d > 7 {
				return nil, errors.New("Sprechzeiten: ungültiger Wochentag")
			}
			if !seen[d] {
				seen[d] = true
				days = append(days, d)
			}
		}
		sort.Ints(days)
		if len(days) == 0 {
			return nil, errors.New("Sprechzeiten: bitte mindestens einen Wochentag wählen")
		}
		if !reClock.MatchString(s.From) || !reClock.MatchString(s.To) || s.From >= s.To {
			return nil, errors.New("Sprechzeiten: Uhrzeiten bitte als 08:00 bis 12:00 (Beginn vor Ende)")
		}
		out.Slots = append(out.Slots, HourSlot{Days: days, From: s.From, To: s.To})
	}
	if len(out.Slots) == 0 && out.Note == "" {
		return nil, nil
	}
	return out, nil
}

type taskInput struct {
	Name        string     `json:"name"`
	Keywords    string     `json:"keywords"`
	Description string     `json:"description"`
	OrgUnit     int64      `json:"orgUnit"`
	Hours       *Hours     `json:"hours"`
	Rules       []TaskRule `json:"rules"`
	Sort        int        `json:"sort"`
}

func (a *App) checkTask(ctx context.Context, id int64, in *taskInput) (Task, error) {
	var ok bool
	t := Task{ID: id, Sort: in.Sort}
	if t.Name, ok = cleanText(in.Name, 150); !ok || t.Name == "" || strings.ContainsAny(t.Name, "<>") {
		return t, errors.New("Bitte einen Namen für die Aufgabe angeben (max. 150 Zeichen)")
	}
	if t.Keywords, ok = cleanText(in.Keywords, 500); !ok || strings.ContainsAny(t.Keywords, "<>") {
		return t, errors.New("Suchbegriffe zu lang (max. 500 Zeichen)")
	}
	if t.Description, ok = cleanMultiline(in.Description, 1000); !ok {
		return t, errors.New("Beschreibung zu lang (max. 1000 Zeichen)")
	}
	if in.Sort < 0 || in.Sort > 99999 {
		return t, errors.New("Sortierung ungültig")
	}
	var err error
	if t.Hours, err = cleanHours(in.Hours); err != nil {
		return t, err
	}
	list, err := a.st().ListTasks(ctx)
	if err != nil {
		return t, err
	}
	found := id == 0
	for _, x := range list {
		if x.ID == id {
			found = true
		} else if strings.EqualFold(x.Name, t.Name) {
			return t, fmt.Errorf("Eine Aufgabe „%s“ gibt es schon", x.Name)
		}
	}
	if !found {
		return t, ErrNotFound
	}
	if id == 0 && len(list) >= maxTasks {
		return t, fmt.Errorf("Höchstens %d Aufgaben", maxTasks)
	}
	if in.OrgUnit != 0 {
		units, err := a.st().ListOrgUnits(ctx)
		if err != nil {
			return t, err
		}
		for _, u := range units {
			if u.ID == in.OrgUnit {
				t.OrgUnit = u.ID
			}
		}
		if t.OrgUnit == 0 {
			return t, errors.New("Die gewählte Organisationseinheit existiert nicht (mehr)")
		}
	}
	if len(in.Rules) > maxTaskRules {
		return t, fmt.Errorf("Höchstens %d Bereiche je Aufgabe", maxTaskRules)
	}
	t.Rules = []TaskRule{}
	all := 0
	for i, r := range in.Rules {
		var rr TaskRule
		if rr.From, rr.To, err = cleanRange(r.From, r.To); err != nil {
			return t, err
		}
		if rr.Note, ok = cleanText(r.Note, 150); !ok || strings.ContainsAny(rr.Note, "<>") {
			return t, errors.New("Hinweis zum Bereich zu lang (max. 150 Zeichen)")
		}
		label := fmt.Sprintf("Bereich %d", i+1)
		if rr.Leads, err = a.cleanIDs(ctx, r.Leads, 0, maxRulePeople, "Zuständige je Bereich"); err != nil {
			return t, err
		}
		if len(rr.Leads) == 0 {
			return t, fmt.Errorf("%s (%s): bitte mindestens eine zuständige Person auswählen", label, rangeLabel(rr))
		}
		if rr.Deputies, err = a.cleanIDs(ctx, r.Deputies, 0, maxRulePeople, "Vertretungen je Bereich"); err != nil {
			return t, err
		}
		if rr.From == "" {
			all++
		}
		t.Rules = append(t.Rules, rr)
	}
	if all > 1 {
		return t, errors.New("Nur ein Bereich darf für alle Namen gelten")
	}
	if a, b, ok := overlap(t.Rules); ok {
		return t, fmt.Errorf("Die Bereiche %s und %s überschneiden sich", rangeLabel(a), rangeLabel(b))
	}
	return t, nil
}

func rangeLabel(r TaskRule) string {
	if r.From == "" {
		return "alle Namen"
	}
	if r.From == r.To {
		return r.From
	}
	return r.From + "–" + r.To
}

// overlap: überschneiden sich zwei Buchstabenbereiche?
func overlap(rules []TaskRule) (TaskRule, TaskRule, bool) {
	var rs []TaskRule
	for _, r := range rules {
		if r.From != "" {
			rs = append(rs, r)
		}
	}
	sort.Slice(rs, func(i, j int) bool {
		a, _ := rangeBounds(foldName(rs[i].From), foldName(rs[i].To))
		b, _ := rangeBounds(foldName(rs[j].From), foldName(rs[j].To))
		return a < b
	})
	for i := 1; i < len(rs); i++ {
		_, prevHi := rangeBounds(foldName(rs[i-1].From), foldName(rs[i-1].To))
		lo, _ := rangeBounds(foldName(rs[i].From), foldName(rs[i].To))
		if lo <= prevHi {
			return rs[i-1], rs[i], true
		}
	}
	return TaskRule{}, TaskRule{}, false
}

// matchRule: welcher Bereich gilt für diesen Nachnamen? (-1 = keiner)
func matchRule(rules []TaskRule, last string) int {
	k := nameKey(last)
	fallback := -1
	for i, r := range rules {
		if r.From == "" {
			fallback = i
			continue
		}
		lo, hi := rangeBounds(foldName(r.From), foldName(r.To))
		if k != "" && lo <= k && k <= hi {
			return i
		}
	}
	return fallback
}

func (a *App) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in taskInput
	if !decodeJSON(w, r, &in, 128<<10) {
		return
	}
	t, err := a.checkTask(r.Context(), 0, &in)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := a.st().CreateTask(r.Context(), t)
	if err != nil {
		storeErr(w, err, "", "")
		return
	}
	a.audit(r, s.Username, "task_create", t.Name, fmt.Sprintf("%d Bereich(e)", len(t.Rules)))
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (a *App) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var in taskInput
	if !decodeJSON(w, r, &in, 128<<10) {
		return
	}
	t, err := a.checkTask(r.Context(), id, &in)
	if errors.Is(err, ErrNotFound) {
		jsonError(w, http.StatusNotFound, "Aufgabe nicht gefunden")
		return
	}
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.st().UpdateTask(r.Context(), t); err != nil {
		storeErr(w, err, "Aufgabe nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "task_update", t.Name, fmt.Sprintf("%d Bereich(e)", len(t.Rules)))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	if err := a.st().DeleteTask(r.Context(), id); err != nil {
		storeErr(w, err, "Aufgabe nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "task_delete", fmt.Sprintf("#%d", id), "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// PUT /api/org/units/{id}/hours – Sprechzeiten einer Einheit (null/leer = keine)
func (a *App) handleSetUnitHours(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var in struct {
		Hours *Hours `json:"hours"`
	}
	if !decodeJSON(w, r, &in, 16<<10) {
		return
	}
	h, err := cleanHours(in.Hours)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	units, err := a.st().ListOrgUnits(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	for _, u := range units {
		if u.ID == id {
			u.Hours = h
			if err := a.st().UpdateOrgUnit(ctx, u); err != nil {
				storeErr(w, err, "Einheit nicht gefunden", "")
				return
			}
			b, _ := json.Marshal(h)
			a.audit(r, s.Username, "org_unit_hours", u.Name, truncate(string(b), 400))
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
	}
	jsonError(w, http.StatusNotFound, "Einheit nicht gefunden")
}

// tasksForDirectory: Verweise auf nicht (mehr) vorhandene Einträge und Einheiten ausblenden
func tasksForDirectory(list []Task, known map[int64]bool, units []OrgUnit) []Task {
	uok := map[int64]bool{}
	for _, u := range units {
		uok[u.ID] = true
	}
	keep := func(ids []int64) []int64 {
		out := []int64{}
		for _, id := range ids {
			if known[id] {
				out = append(out, id)
			}
		}
		return out
	}
	for i := range list {
		if !uok[list[i].OrgUnit] {
			list[i].OrgUnit = 0
		}
		for j := range list[i].Rules {
			list[i].Rules[j].Leads = keep(list[i].Rules[j].Leads)
			list[i].Rules[j].Deputies = keep(list[i].Rules[j].Deputies)
		}
	}
	return list
}
