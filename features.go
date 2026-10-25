package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Vertretungen, Abwesenheiten, Organigramm und Fehlermeldungen.

const (
	maxDeputies       = 10
	maxAbsences       = 20
	maxAbsenceDays    = 366
	maxAbsenceDeputy  = 5
	maxOrgLevels      = 6
	maxOrgUnits       = 2000
	maxUnitPeople     = 20 // Leitung bzw. Vertretung je Einheit
	maxMembersRequest = 2000
)

func (a *App) absencesMode() string {
	if a.cfg.Features.Absences == "" {
		return "all"
	}
	return a.cfg.Features.Absences
}

// ---------- Hilfen ----------

// cleanIDs: nur existierende Einträge, ohne Dubletten, ohne exclude, höchstens max
func (a *App) cleanIDs(ctx context.Context, ids []int64, exclude int64, max int, what string) ([]int64, error) {
	if len(ids) > max {
		return nil, fmt.Errorf("Höchstens %d %s", max, what)
	}
	seen := map[int64]bool{}
	var out []int64
	for _, id := range ids {
		if id <= 0 || id == exclude || seen[id] {
			continue
		}
		if _, err := a.st().GetEmployee(ctx, id); errors.Is(err, ErrNotFound) {
			return nil, errors.New("Ein ausgewählter Eintrag existiert nicht (mehr)")
		} else if err != nil {
			return nil, err
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

var reDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func parseDay(s string) (time.Time, bool) {
	if !reDate.MatchString(s) {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

// currentAbsences: vergangene Abwesenheiten weglassen, sortiert nach Beginn
func currentAbsences(list []Absence, today string) []Absence {
	var out []Absence
	for _, x := range list {
		if x.To >= today {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

// ---------- Vertretungen ----------

func (a *App) handleSetDeputies(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	if !a.canEditEmployee(s, id) {
		jsonError(w, http.StatusForbidden, "Du darfst nur deine eigenen Vertretungen festlegen")
		return
	}
	var in struct {
		Deputies []int64 `json:"deputies"`
	}
	if !decodeJSON(w, r, &in, 4<<10) {
		return
	}
	ctx := r.Context()
	e, err := a.st().GetEmployee(ctx, id)
	if err != nil {
		storeErr(w, err, "Eintrag nicht gefunden", "")
		return
	}
	deps, err := a.cleanIDs(ctx, in.Deputies, id, maxDeputies, "Vertretungen")
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.st().SetDeputies(ctx, id, deps); err != nil {
		storeErr(w, err, "Eintrag nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "deputies_update", empLabel(e), fmt.Sprintf("%d Vertretung(en)", len(deps)))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- Abwesenheiten ----------

func (a *App) handleSetAbsences(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	if a.absencesMode() == "off" {
		jsonError(w, http.StatusNotFound, "Abwesenheiten sind abgeschaltet")
		return
	}
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	if !a.canEditEmployee(s, id) {
		jsonError(w, http.StatusForbidden, "Du darfst nur deine eigenen Abwesenheiten eintragen")
		return
	}
	var in struct {
		Absences []Absence `json:"absences"`
	}
	if !decodeJSON(w, r, &in, 16<<10) {
		return
	}
	if len(in.Absences) > maxAbsences {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("Höchstens %d Abwesenheiten", maxAbsences))
		return
	}
	ctx := r.Context()
	e, err := a.st().GetEmployee(ctx, id)
	if err != nil {
		storeErr(w, err, "Eintrag nicht gefunden", "")
		return
	}
	today := a.cfg.today()
	var out []Absence
	for _, x := range in.Absences {
		from, ok1 := parseDay(x.From)
		to, ok2 := parseDay(x.To)
		if !ok1 || !ok2 {
			jsonError(w, http.StatusBadRequest, "Datum ungültig (Format JJJJ-MM-TT)")
			return
		}
		if to.Before(from) {
			jsonError(w, http.StatusBadRequest, "Das Ende liegt vor dem Beginn")
			return
		}
		if to.Sub(from) > maxAbsenceDays*24*time.Hour {
			jsonError(w, http.StatusBadRequest, "Eine Abwesenheit darf höchstens ein Jahr dauern")
			return
		}
		if x.To < today {
			continue // Vergangenes wird nicht gespeichert
		}
		deps, err := a.cleanIDs(ctx, x.Deputies, id, maxAbsenceDeputy, "Vertretungen je Abwesenheit")
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		out = append(out, Absence{From: x.From, To: x.To, Deputies: deps})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	for i := 1; i < len(out); i++ {
		if out[i].From <= out[i-1].To {
			jsonError(w, http.StatusBadRequest, "Abwesenheiten überschneiden sich")
			return
		}
	}
	if err := a.st().SetAbsences(ctx, id, out); err != nil {
		storeErr(w, err, "Eintrag nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "absences_update", empLabel(e), fmt.Sprintf("%d Zeitraum/Zeiträume", len(out)))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// absenceGC entfernt täglich vergangene Abwesenheiten (Datensparsamkeit).
func (a *App) absenceGC(ctx context.Context) {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		if !a.offline.Load() {
			today := a.cfg.today()
			if recs, err := a.st().ListEmployees(ctx, true); err == nil {
				for _, e := range recs {
					if len(e.Absences) == 0 {
						continue
					}
					cur := currentAbsences(e.Absences, today)
					if len(cur) != len(e.Absences) {
						if err := a.st().SetAbsences(ctx, e.ID, cur); err != nil {
							log.Printf("Abwesenheiten bereinigen (%d): %v", e.ID, err)
						}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ---------- Organisation ----------

// OrgLevel: Ebene im Organigramm. Leitung/Stellvertretung einer Einheit ergeben sich aus
// Zuordnung zur Einheit + Schlagwort der Ebene (z. B. Ebene „Amt“: „Amtsleitung“ / „Stv. Amtsleitung“).
type OrgLevel struct {
	Name      string `json:"name"`
	LeaderTag int64  `json:"leaderTag,omitempty"`
	DeputyTag int64  `json:"deputyTag,omitempty"`
}

func (a *App) orgLevels(ctx context.Context) []OrgLevel {
	v, err := a.st().GetSetting(ctx, settingOrgLevels)
	var lv []OrgLevel
	if err == nil && v != "" {
		if json.Unmarshal([]byte(v), &lv) != nil { // ältere Form: nur Namen
			var names []string
			if json.Unmarshal([]byte(v), &names) == nil {
				for _, n := range names {
					lv = append(lv, OrgLevel{Name: n})
				}
			}
		}
	}
	if len(lv) == 0 {
		for _, n := range []string{"Dezernat", "Amt", "Sachgebiet", "Team"} {
			lv = append(lv, OrgLevel{Name: n})
		}
	}
	return lv
}

// protectedTags: Leitungs-/Vertretungs-Schlagworte – nur Admins dürfen sie vergeben oder entfernen
func (a *App) protectedTags(ctx context.Context) map[int64]bool {
	out := map[int64]bool{}
	for _, l := range a.orgLevels(ctx) {
		if l.LeaderTag != 0 {
			out[l.LeaderTag] = true
		}
		if l.DeputyTag != 0 {
			out[l.DeputyTag] = true
		}
	}
	return out
}

// replaceLevelTag: nach dem Zusammenführen von Schlagworten Verweise der Ebenen umhängen
func (a *App) replaceLevelTag(ctx context.Context, from, into int64) {
	lv := a.orgLevels(ctx)
	changed := false
	for i := range lv {
		if lv[i].LeaderTag == from {
			lv[i].LeaderTag, changed = into, true
		}
		if lv[i].DeputyTag == from {
			lv[i].DeputyTag, changed = into, true
		}
	}
	if changed {
		b, _ := json.Marshal(lv)
		if err := a.st().SetSetting(ctx, settingOrgLevels, string(b)); err != nil {
			log.Printf("Ebenen-Schlagworte umhängen: %v", err)
		}
	}
}

// unitDepths: Tiefe je Einheit (Wurzel = 1); Zyklen werden abgebrochen
func unitDepths(units []OrgUnit) map[int64]int {
	byID := map[int64]*OrgUnit{}
	for i := range units {
		byID[units[i].ID] = &units[i]
	}
	depth := map[int64]int{}
	for _, u := range units {
		d := 1
		for p := u.Parent; p != 0 && d <= 64; d++ {
			pu := byID[p]
			if pu == nil {
				break
			}
			p = pu.Parent
		}
		depth[u.ID] = d
	}
	return depth
}

func (a *App) handleSetOrgLevels(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in struct {
		Levels []struct {
			Name      string `json:"name"`
			LeaderTag string `json:"leaderTag"` // Name des Schlagworts (wird bei Bedarf angelegt)
			DeputyTag string `json:"deputyTag"`
		} `json:"levels"`
	}
	if !decodeJSON(w, r, &in, 8<<10) {
		return
	}
	if len(in.Levels) == 0 || len(in.Levels) > maxOrgLevels {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("1 bis %d Ebenen", maxOrgLevels))
		return
	}
	ctx := r.Context()
	tags, err := a.st().ListTags(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	cr, err := a.categories(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	cat := cr.resolve("sonstiges")
	used := map[int64]string{}
	resolveTag := func(name string) (int64, error) {
		if strings.TrimSpace(name) == "" {
			return 0, nil
		}
		n, err := normalizeTagName(name)
		if err != nil {
			return 0, err
		}
		for _, t := range tags {
			if strings.EqualFold(t.Name, n) {
				return t.ID, nil
			}
		}
		id, err := a.st().CreateTag(ctx, n, cat, s.Username)
		if err != nil {
			return 0, err
		}
		tags = append(tags, Tag{ID: id, Name: n, Category: cat})
		a.audit(r, s.Username, "tag_create", n, cat)
		return id, nil
	}
	var lv []OrgLevel
	for i, l := range in.Levels {
		name, ok := cleanText(l.Name, 40)
		if !ok || name == "" || strings.ContainsAny(name, "<>") {
			jsonError(w, http.StatusBadRequest, "Ungültiger Ebenen-Name (1–40 Zeichen)")
			return
		}
		lt, err := resolveTag(l.LeaderTag)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		dt, err := resolveTag(l.DeputyTag)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		for _, t := range []int64{lt, dt} {
			if t == 0 {
				continue
			}
			if prev, dup := used[t]; dup {
				jsonError(w, http.StatusBadRequest, fmt.Sprintf("Ein Schlagwort kann nur einmal verwendet werden (Ebene %d und %s)", i+1, prev))
				return
			}
			used[t] = fmt.Sprintf("Ebene %d", i+1)
		}
		lv = append(lv, OrgLevel{Name: name, LeaderTag: lt, DeputyTag: dt})
	}
	units, err := a.st().ListOrgUnits(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	for _, d := range unitDepths(units) {
		if d > len(lv) {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("Es gibt Einheiten auf Ebene %d – erst diese verschieben oder löschen", d))
			return
		}
	}
	b, _ := json.Marshal(lv)
	if err := a.st().SetSetting(ctx, settingOrgLevels, string(b)); err != nil {
		dbFail(w, err)
		return
	}
	names := make([]string, len(lv))
	for i, l := range lv {
		names[i] = l.Name
	}
	a.audit(r, s.Username, "org_levels", "", strings.Join(names, " › "))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "levels": lv})
}

type orgUnitInput struct {
	Parent   int64   `json:"parent"`
	Name     string  `json:"name"`
	Short    string  `json:"short"`
	Sort     int     `json:"sort"`
	Leaders  []int64 `json:"leaders"`
	Deputies []int64 `json:"deputies"`
}

// checkUnit prüft Name, Eltern (existiert, kein Zyklus) und die Tiefe gegen die Ebenen.
func (a *App) checkUnit(ctx context.Context, id int64, in *orgUnitInput) (OrgUnit, error) {
	var ok bool
	if in.Name, ok = cleanText(in.Name, 150); !ok || in.Name == "" || strings.ContainsAny(in.Name, "<>") {
		return OrgUnit{}, errors.New("Bitte einen Namen angeben (max. 150 Zeichen)")
	}
	if in.Short, ok = cleanText(in.Short, 30); !ok || strings.ContainsAny(in.Short, "<>") {
		return OrgUnit{}, errors.New("Kürzel zu lang (max. 30 Zeichen)")
	}
	if in.Sort < 0 || in.Sort > 99999 {
		return OrgUnit{}, errors.New("Sortierung ungültig")
	}
	units, err := a.st().ListOrgUnits(ctx)
	if err != nil {
		return OrgUnit{}, err
	}
	if id == 0 && len(units) >= maxOrgUnits {
		return OrgUnit{}, fmt.Errorf("Höchstens %d Einheiten", maxOrgUnits)
	}
	byID := map[int64]OrgUnit{}
	for _, u := range units {
		byID[u.ID] = u
	}
	if id != 0 {
		if _, ok := byID[id]; !ok {
			return OrgUnit{}, ErrNotFound
		}
	}
	if in.Parent != 0 {
		if _, ok := byID[in.Parent]; !ok {
			return OrgUnit{}, errors.New("Übergeordnete Einheit existiert nicht")
		}
		for p := in.Parent; p != 0; p = byID[p].Parent {
			if p == id {
				return OrgUnit{}, errors.New("Eine Einheit kann nicht unter sich selbst hängen")
			}
		}
	}
	// Tiefe: neue Position + Höhe des eigenen Unterbaums
	trial := make([]OrgUnit, 0, len(units)+1)
	for _, u := range units {
		if u.ID == id {
			u.Parent = in.Parent
		}
		trial = append(trial, u)
	}
	if id == 0 {
		trial = append(trial, OrgUnit{ID: -1, Parent: in.Parent})
	}
	depth := unitDepths(trial)
	levels := len(a.orgLevels(ctx))
	for _, d := range depth {
		if d > levels {
			return OrgUnit{}, fmt.Errorf("Zu tief: Es sind nur %d Ebenen angelegt", levels)
		}
	}
	leaders, err := a.cleanIDs(ctx, in.Leaders, 0, maxUnitPeople, "Personen in der Leitung")
	if err != nil {
		return OrgUnit{}, err
	}
	deps, err := a.cleanIDs(ctx, in.Deputies, 0, maxUnitPeople, "Vertretungen")
	if err != nil {
		return OrgUnit{}, err
	}
	return OrgUnit{ID: id, Parent: in.Parent, Name: in.Name, Short: in.Short, Sort: in.Sort, Leaders: leaders, Deputies: deps}, nil
}

func (a *App) handleCreateOrgUnit(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in orgUnitInput
	if !decodeJSON(w, r, &in, 8<<10) {
		return
	}
	u, err := a.checkUnit(r.Context(), 0, &in)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := a.st().CreateOrgUnit(r.Context(), u)
	if err != nil {
		dbFail(w, err)
		return
	}
	a.audit(r, s.Username, "org_unit_create", u.Name, "")
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (a *App) handleUpdateOrgUnit(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var in orgUnitInput
	if !decodeJSON(w, r, &in, 8<<10) {
		return
	}
	u, err := a.checkUnit(r.Context(), id, &in)
	if errors.Is(err, ErrNotFound) {
		jsonError(w, http.StatusNotFound, "Einheit nicht gefunden")
		return
	}
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.st().UpdateOrgUnit(r.Context(), u); err != nil {
		storeErr(w, err, "Einheit nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "org_unit_update", u.Name, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleDeleteOrgUnit(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	if err := a.st().DeleteOrgUnit(r.Context(), id); err != nil {
		storeErr(w, err, "Einheit nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "org_unit_delete", fmt.Sprintf("#%d", id), "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Personen einer Einheit zuordnen (add) bzw. herausnehmen (remove)
func (a *App) handleOrgMembers(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var in struct {
		Add    []int64 `json:"add"`
		Remove []int64 `json:"remove"`
	}
	if !decodeJSON(w, r, &in, 64<<10) {
		return
	}
	if len(in.Add)+len(in.Remove) > maxMembersRequest {
		jsonError(w, http.StatusBadRequest, "Zu viele Einträge auf einmal")
		return
	}
	ctx := r.Context()
	units, err := a.st().ListOrgUnits(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	var unit *OrgUnit
	for i := range units {
		if units[i].ID == id {
			unit = &units[i]
		}
	}
	if unit == nil {
		jsonError(w, http.StatusNotFound, "Einheit nicht gefunden")
		return
	}
	added, err := a.st().SetOrgUnit(ctx, in.Add, id)
	if err != nil {
		dbFail(w, err)
		return
	}
	// Nur herausnehmen, wer wirklich in dieser Einheit ist
	var rm []int64
	for _, eid := range in.Remove {
		if e, err := a.st().GetEmployee(ctx, eid); err == nil && e.OrgUnit == id {
			rm = append(rm, eid)
		}
	}
	removed, err := a.st().SetOrgUnit(ctx, rm, 0)
	if err != nil {
		dbFail(w, err)
		return
	}
	a.audit(r, s.Username, "org_members", unit.Name, fmt.Sprintf("+%d / −%d", added, removed))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "added": added, "removed": removed})
}

// ---------- Fehlermeldungen ----------

var reportFields = map[string]string{
	"name": "Name", "phone": "Telefon", "mobile": "Mobil", "room": "Raum", "amt": "Amt",
	"email": "E-Mail", "org": "Organisation", "deputies": "Vertretung", "other": "Sonstiges",
}

func (a *App) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	if !a.cfg.reportsEnabled() {
		jsonError(w, http.StatusNotFound, "Meldungen sind abgeschaltet")
		return
	}
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var in struct {
		Field      string `json:"field"`
		Suggestion string `json:"suggestion"`
		Message    string `json:"message"`
	}
	if !decodeJSON(w, r, &in, 4<<10) {
		return
	}
	if _, ok := reportFields[in.Field]; !ok {
		jsonError(w, http.StatusBadRequest, "Bitte angeben, was nicht stimmt")
		return
	}
	var ok1, ok2 bool
	in.Suggestion, ok1 = cleanText(in.Suggestion, 255)
	in.Message, ok2 = cleanMultiline(in.Message, 500)
	if !ok1 || !ok2 {
		jsonError(w, http.StatusBadRequest, "Text zu lang (Vorschlag max. 255, Hinweis max. 500 Zeichen)")
		return
	}
	if in.Suggestion == "" && in.Message == "" {
		jsonError(w, http.StatusBadRequest, "Bitte den richtigen Wert oder einen Hinweis angeben")
		return
	}
	ctx := r.Context()
	e, err := a.st().GetEmployee(ctx, id)
	if err != nil || (!e.Active && !s.IsAdmin) {
		jsonError(w, http.StatusNotFound, "Eintrag nicht gefunden")
		return
	}
	n, err := a.st().CountReports(ctx, "open", s.Username)
	if err != nil {
		dbFail(w, err)
		return
	}
	if n >= a.cfg.Features.MaxOpenReportsUser {
		jsonError(w, http.StatusTooManyRequests, fmt.Sprintf("Du hast schon %d offene Meldungen – bitte warten, bis sie bearbeitet sind", n))
		return
	}
	rep := Report{EmployeeID: id, Target: truncate(strings.TrimSpace(e.LastName+", "+e.FirstName), 255), Field: in.Field, Suggestion: in.Suggestion,
		Message: in.Message, Reporter: s.Username, ReporterName: truncate(s.DisplayName, 200), CreatedAt: time.Now().UTC()}
	rid, err := a.st().CreateReport(ctx, rep)
	if err != nil {
		dbFail(w, err)
		return
	}
	a.audit(r, s.Username, "report_create", empLabel(e), reportFields[in.Field])
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": rid})
}

func (a *App) handleListReports(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch status {
	case "open", "done", "rejected":
	case "all":
		status = ""
	default:
		status = "open"
	}
	list, err := a.st().ListReports(r.Context(), status, 500)
	if err != nil {
		dbFail(w, err)
		return
	}
	open, _ := a.st().CountReports(r.Context(), "open", "")
	writeJSON(w, http.StatusOK, map[string]any{"reports": list, "open": open, "fields": reportFields})
}

func (a *App) handleUpdateReport(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var in struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if !decodeJSON(w, r, &in, 4<<10) {
		return
	}
	if in.Status != "open" && in.Status != "done" && in.Status != "rejected" {
		jsonError(w, http.StatusBadRequest, "Status ungültig")
		return
	}
	note, ok := cleanMultiline(in.Note, 500)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Notiz zu lang")
		return
	}
	if err := a.st().UpdateReport(r.Context(), id, in.Status, s.Username, note); err != nil {
		storeErr(w, err, "Meldung nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "report_"+in.Status, fmt.Sprintf("Meldung #%d", id), note)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Eigene Meldungen (Status-Rückmeldung für die meldende Person)
func (a *App) handleMyReports(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	list, err := a.st().ListReports(r.Context(), "", 500)
	if err != nil {
		dbFail(w, err)
		return
	}
	out := []Report{}
	for _, x := range list {
		if x.Reporter == s.Username {
			x.ResolvedBy = "" // Admin-Namen nicht offenlegen
			out = append(out, x)
		}
		if len(out) >= 50 {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": out, "fields": reportFields})
}
