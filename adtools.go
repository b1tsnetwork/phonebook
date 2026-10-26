package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Meldungen → Active Directory: Die Anwendung schreibt bewusst NICHT selbst ins AD (das Dienstkonto
// bleibt nur lesend). Stattdessen erzeugt sie geprüfte PowerShell-Befehle (Set-ADUser) zum Kopieren.
// Nach dem nächsten AD-Abgleich werden Meldungen, deren Vorschlag übernommen ist, automatisch erledigt.

var reGUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// psQuote: PowerShell-String in einfachen Anführungszeichen. PowerShell wertet auch die typografischen
// Varianten ‘ ’ ‚ ‛ als Anführungszeichen – sie werden wie ' verdoppelt. Kein $-Ersetzen in '…'.
func psQuote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\'', '‘', '’', '‚', '‛':
			b.WriteRune(r)
			b.WriteRune(r)
		default:
			if r < 0x20 || r == 0x7f {
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// reportADChange liefert für eine Meldung die AD-Attribute (attr → Wert, "" = leeren) oder einen Grund.
func (a *App) reportADChange(rep Report, e *EmployeeRec) (map[string]string, string) {
	if e == nil {
		return nil, "Eintrag gelöscht"
	}
	if e.Source != "ad" || !reGUID.MatchString(e.ADGUID) {
		return nil, "kein AD-Eintrag – direkt im Telefonbuch korrigieren"
	}
	sugg := strings.TrimSpace(rep.Suggestion)
	if sugg == "" {
		return nil, "kein Vorschlag angegeben"
	}
	clear := sugg == "-" || strings.EqualFold(sugg, "(leer)")
	at := a.cfg.LDAP.Attributes
	one := func(attr string, max int, ok func(string) bool) (map[string]string, string) {
		if attr == "" {
			return nil, "kein AD-Attribut konfiguriert"
		}
		if clear {
			return map[string]string{attr: ""}, ""
		}
		if len([]rune(sugg)) > max || (ok != nil && !ok(sugg)) {
			return nil, "Vorschlag passt nicht zum Feld – bitte prüfen"
		}
		return map[string]string{attr: sugg}, ""
	}
	phoneOK := func(s string) bool { return rePhone.MatchString(s) }
	switch rep.Field {
	case "phone":
		return one(at.Phone, 64, phoneOK)
	case "mobile":
		return one(at.Mobile, 64, phoneOK)
	case "room":
		return one(at.Room, 64, nil)
	case "amt":
		return one(at.Amt, 150, nil)
	case "email":
		return one(at.Email, 254, func(s string) bool {
			_, err := mail.ParseAddress(s)
			return err == nil && !strings.ContainsAny(s, " <>")
		})
	case "name":
		last, first, ok := strings.Cut(sugg, ",")
		last, first = strings.TrimSpace(last), strings.TrimSpace(first)
		if !ok || last == "" || first == "" || len([]rune(last)) > 100 || len([]rune(first)) > 100 {
			return nil, "Name bitte als „Nachname, Vorname“ melden"
		}
		if at.LastName == "" || at.FirstName == "" {
			return nil, "kein AD-Attribut konfiguriert"
		}
		return map[string]string{at.LastName: last, at.FirstName: first}, ""
	}
	return nil, "dieses Feld wird nicht im AD gepflegt"
}

// adServerArg: -Server aus ldap.url (Domänencontroller), sonst leer
func (a *App) adServerArg() string {
	u, err := url.Parse(a.cfg.LDAP.URL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return " -Server " + psQuote(u.Hostname())
}

func (a *App) psCommand(rep Report, e *EmployeeRec) (string, string) {
	attrs, why := a.reportADChange(rep, e)
	if attrs == nil {
		return "", why
	}
	var repl, clr []string
	for _, k := range sortedKeys(attrs) {
		if attrs[k] == "" {
			clr = append(clr, psQuote(k))
		} else {
			repl = append(repl, psQuote(k)+"="+psQuote(attrs[k]))
		}
	}
	cmd := "Set-ADUser -Identity " + psQuote(strings.ToLower(e.ADGUID)) + a.adServerArg()
	if len(repl) > 0 {
		cmd += " -Replace @{" + strings.Join(repl, "; ") + "}"
	}
	if len(clr) > 0 {
		cmd += " -Clear " + strings.Join(clr, ",")
	}
	if rep.Field == "name" {
		cmd += "   # displayName/Kontoname bei Bedarf zusätzlich anpassen"
	}
	return cmd, ""
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// GET /api/admin/reports/powershell?ids=1,2,3 – Skript zum Kopieren (ohne ids: alle offenen)
func (a *App) handleReportsPowerShell(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	ctx := r.Context()
	want := map[int64]bool{}
	for _, x := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err == nil && id > 0 {
			want[id] = true
		}
	}
	list, err := a.st().ListReports(ctx, "open", 500)
	if err != nil {
		dbFail(w, err)
		return
	}
	var b strings.Builder
	loc := a.cfg.loc
	if loc == nil {
		loc = time.Local
	}
	fmt.Fprintf(&b, "# Telefonbuch – Korrekturen aus Meldungen, erzeugt %s von %s\r\n", time.Now().In(loc).Format("02.01.2006 15:04"), s.Username)
	b.WriteString("# Bitte vor dem Ausführen prüfen! Benötigt das Modul ActiveDirectory (RSAT) und Schreibrechte.\r\n")
	b.WriteString("Import-Module ActiveDirectory\r\n")
	b.WriteString("$WhatIfPreference = $true   # Probelauf – zum Ausführen auf $false setzen\r\n\r\n")
	n := 0
	for _, rep := range list {
		if len(want) > 0 && !want[rep.ID] {
			continue
		}
		var e *EmployeeRec
		if rep.EmployeeID != 0 {
			e, _ = a.st().GetEmployee(ctx, rep.EmployeeID)
		}
		cmd, why := a.psCommand(rep, e)
		label := reportFields[rep.Field]
		if cmd == "" {
			fmt.Fprintf(&b, "# #%d %s – %s: übersprungen (%s)\r\n\r\n", rep.ID, oneLine(rep.Target), label, why)
			continue
		}
		fmt.Fprintf(&b, "# #%d %s – %s → %s (gemeldet von %s)\r\n%s\r\n\r\n", rep.ID, oneLine(rep.Target), label, oneLine(rep.Suggestion), oneLine(nonEmpty(rep.ReporterName, rep.Reporter)), cmd)
		n++
	}
	fmt.Fprintf(&b, "# %d Befehl(e). Danach im Telefonbuch „AD-Abgleich“ starten – übernommene Meldungen werden automatisch erledigt.\r\n", n)
	a.audit(r, s.Username, "reports_powershell", "", fmt.Sprintf("%d Befehl(e)", n))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(b.String()))
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ---------- Automatisch erledigen ----------

func normPhone(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' || r == '+' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// reportApplied: entspricht der aktuelle Wert dem Vorschlag?
func reportApplied(rep Report, e *EmployeeRec) bool {
	sugg := strings.TrimSpace(rep.Suggestion)
	if sugg == "" || e == nil {
		return false
	}
	clear := sugg == "-" || strings.EqualFold(sugg, "(leer)")
	eq := func(cur string) bool {
		if clear {
			return strings.TrimSpace(cur) == ""
		}
		return strings.EqualFold(oneLine(cur), oneLine(sugg))
	}
	switch rep.Field {
	case "phone", "mobile":
		cur := e.Phone
		if rep.Field == "mobile" {
			cur = e.Mobile
		}
		if clear {
			return strings.TrimSpace(cur) == ""
		}
		return normPhone(cur) != "" && normPhone(cur) == normPhone(sugg)
	case "room":
		return eq(e.Room)
	case "amt":
		return eq(e.Amt)
	case "email":
		return eq(e.Email)
	case "name":
		return eq(e.LastName + ", " + e.FirstName)
	}
	return false
}

// autoCloseReports erledigt offene Meldungen, deren Vorschlag inzwischen im Eintrag steht.
func (a *App) autoCloseReports(ctx context.Context, by string) int {
	if !a.cfg.reportsEnabled() || a.offline.Load() {
		return 0
	}
	list, err := a.st().ListReports(ctx, "open", 0)
	if err != nil {
		return 0
	}
	n := 0
	for _, rep := range list {
		if rep.EmployeeID == 0 {
			continue
		}
		e, err := a.st().GetEmployee(ctx, rep.EmployeeID)
		if err != nil || !reportApplied(rep, e) {
			continue
		}
		if err := a.st().UpdateReport(ctx, rep.ID, "done", by, "Automatisch erledigt: Der gemeldete Wert ist übernommen."); err == nil {
			n++
		}
	}
	if n > 0 {
		log.Printf("Meldungen automatisch erledigt (%s): %d", by, n)
		a.audit(nil, by, "reports_autoclose", "", fmt.Sprintf("%d Meldung(en)", n))
	}
	return n
}

func (a *App) handleReportsAutoClose(w http.ResponseWriter, r *http.Request) {
	n := a.autoCloseReports(r.Context(), sessionFrom(r).Username)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "closed": n})
}
