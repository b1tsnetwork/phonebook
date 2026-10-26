package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Meldung „Fehlender Kontakt“: Jemand sucht eine Person, die im Telefonbuch nicht vorkommt.
// Die Meldung landet bei den übrigen Meldungen (Feld "missing", ohne Eintrag). Sobald die Person
// im Telefonbuch steht – nach dem AD-Abgleich oder weil die Administration sie anlegt –, wird die
// Meldung automatisch erledigt.

const fieldMissing = "missing"

// reportLabel: Anzeigename eines Meldungsfelds (inkl. „Fehlender Kontakt“)
func reportLabel(f string) string {
	if f == fieldMissing {
		return "Fehlender Kontakt"
	}
	return reportFields[f]
}

func reportLabels() map[string]string {
	out := map[string]string{fieldMissing: reportLabel(fieldMissing)}
	for k, v := range reportFields {
		out[k] = v
	}
	return out
}

func personKey(last, first string) string {
	return normOrg(last + " " + first)
}

// findPerson sucht einen Eintrag passend zu den Angaben (E-Mail oder Vor- und Nachname).
// visibleOnly: nur aktive, nicht ausgeblendete Einträge.
func findPerson(d *NewContact, emps []EmployeeRec, visibleOnly bool) *EmployeeRec {
	if d == nil {
		return nil
	}
	key := personKey(d.LastName, d.FirstName)
	mail := strings.ToLower(strings.TrimSpace(d.Email))
	for i := range emps {
		e := &emps[i]
		if visibleOnly && (!e.Active || e.Hidden) {
			continue
		}
		if mail != "" && strings.EqualFold(strings.TrimSpace(e.Email), mail) {
			return e
		}
		if d.FirstName != "" && key != "" && personKey(e.LastName, e.FirstName) == key {
			return e
		}
	}
	return nil
}

func (a *App) handleReportMissing(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	if !a.cfg.reportsEnabled() {
		jsonError(w, http.StatusNotFound, "Meldungen sind abgeschaltet")
		return
	}
	var in struct {
		contactFields
		Message string `json:"message"`
		Force   bool   `json:"force"` // trotz ähnlichem Eintrag melden
	}
	if !decodeJSON(w, r, &in, 8<<10) {
		return
	}
	if err := in.clean(); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.LastName == "" || in.FirstName == "" {
		jsonError(w, http.StatusBadRequest, "Bitte Vor- und Nachnamen angeben")
		return
	}
	msg, ok := cleanMultiline(in.Message, 500)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Hinweis zu lang (max. 500 Zeichen)")
		return
	}
	if in.Phone == "" && in.Mobile == "" && in.Email == "" && in.Amt == "" && in.Room == "" && msg == "" {
		jsonError(w, http.StatusBadRequest, "Bitte mindestens eine Angabe machen (Telefon, E-Mail, Amt …) – sonst lässt sich die Person kaum zuordnen")
		return
	}
	d := &NewContact{LastName: in.LastName, FirstName: in.FirstName, Phone: in.Phone, Mobile: in.Mobile, Email: in.Email, Room: in.Room, Amt: in.Amt}
	ctx := r.Context()
	emps, err := a.st().ListEmployees(ctx, false)
	if err != nil {
		dbFail(w, err)
		return
	}
	if e := findPerson(d, emps, true); e != nil && !in.Force {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "Diese Person steht schon im Telefonbuch", "existing": map[string]any{"id": e.ID, "name": strings.TrimSpace(e.LastName + ", " + e.FirstName), "amt": e.Amt}})
		return
	}
	open, err := a.st().ListReports(ctx, "open", 0)
	if err != nil {
		dbFail(w, err)
		return
	}
	mine := 0
	key := personKey(d.LastName, d.FirstName)
	for _, x := range open {
		if x.Reporter == s.Username {
			mine++
		}
		if x.Field == fieldMissing && x.Details != nil && personKey(x.Details.LastName, x.Details.FirstName) == key {
			jsonError(w, http.StatusConflict, "Diese Person wurde bereits gemeldet – die Meldung wird bearbeitet")
			return
		}
	}
	if mine >= a.cfg.Features.MaxOpenReportsUser {
		jsonError(w, http.StatusTooManyRequests, fmt.Sprintf("Du hast schon %d offene Meldungen – bitte warten, bis sie bearbeitet sind", mine))
		return
	}
	rep := Report{Target: truncate(d.LastName+", "+d.FirstName, 255), Field: fieldMissing, Suggestion: truncate(missingSummary(d), 255),
		Message: msg, Reporter: s.Username, ReporterName: truncate(s.DisplayName, 200), CreatedAt: time.Now().UTC(), Details: d}
	rid, err := a.st().CreateReport(ctx, rep)
	if err != nil {
		dbFail(w, err)
		return
	}
	a.audit(r, s.Username, "report_missing", rep.Target, "")
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": rid})
}

// missingSummary: Kurzfassung für Listen (z. B. „Meine Meldungen“)
func missingSummary(d *NewContact) string {
	var p []string
	for _, x := range []string{d.Amt, d.Phone, d.Mobile, d.Email, d.Room} {
		if x != "" {
			p = append(p, x)
		}
	}
	return strings.Join(p, " · ")
}

// closeFoundMissing: offene „fehlender Kontakt“-Meldungen erledigen, deren Person jetzt sichtbar im Telefonbuch steht
func (a *App) closeFoundMissing(ctx context.Context, list []Report, by string) int {
	var emps []EmployeeRec
	n := 0
	for _, rep := range list {
		if rep.Field != fieldMissing || rep.Status != "open" || rep.Details == nil {
			continue
		}
		if emps == nil {
			var err error
			if emps, err = a.st().ListEmployees(ctx, false); err != nil {
				return n
			}
		}
		e := findPerson(rep.Details, emps, true)
		if e == nil {
			continue
		}
		src := "manuell angelegt"
		if e.Source == "ad" {
			src = "aus dem Active Directory"
		}
		if a.st().UpdateReport(ctx, rep.ID, "done", by, fmt.Sprintf("Automatisch erledigt: %s, %s steht jetzt im Telefonbuch (%s).", e.LastName, e.FirstName, src)) == nil {
			n++
		}
	}
	return n
}
