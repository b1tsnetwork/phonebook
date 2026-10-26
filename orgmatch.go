package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Massenzuweisung: Das AD liefert nur „Amt“ (department) als Text. Die Einheiten im Organigramm
// heißen meist genauso – also wird der Amt-Text den Einheiten zugeordnet, mit Vorschau (ja/nein)
// und optional automatisch nach jedem AD-Abgleich (nur eindeutige Treffer, nur Nicht-Zugeordnete).

const settingOrgAutoAssign = "orgAutoAssign"

// normOrg: Kleinbuchstaben, Satz-/Bindestriche als Leerzeichen, Leerraum zusammengefasst
func normOrg(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ReplaceAll(strings.ToLower(s), "ß", "ss") {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.') {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return strings.Trim(b.String(), ". ")
}

// führendes Kürzel im Amt-Text: „10 – Hauptamt“ → „10“, „12.1 Infrastruktur“ → „12.1“
var reLeadShort = regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)*)(?:\s|[-–—:/]|$)`)

type orgMatch struct {
	Unit       int64   `json:"unit"`
	Kind       string  `json:"match"` // exakt, kuerzel, aehnlich
	Candidates []int64 `json:"candidates,omitempty"`
}

type orgMatcher struct {
	units  []OrgUnit
	exact  map[string][]int64
	short  map[string][]int64
	byID   map[int64]OrgUnit
	parent map[int64]int64
}

func newOrgMatcher(units []OrgUnit) *orgMatcher {
	m := &orgMatcher{units: units, exact: map[string][]int64{}, short: map[string][]int64{}, byID: map[int64]OrgUnit{}, parent: map[int64]int64{}}
	add := func(mp map[string][]int64, k string, id int64) {
		if k == "" {
			return
		}
		for _, x := range mp[k] {
			if x == id {
				return
			}
		}
		mp[k] = append(mp[k], id)
	}
	for _, u := range units {
		m.byID[u.ID] = u
		m.parent[u.ID] = u.Parent
		add(m.exact, normOrg(u.Name), u.ID)
		if u.Short != "" {
			add(m.exact, normOrg(u.Short+" "+u.Name), u.ID)
			add(m.short, strings.ToLower(strings.TrimSpace(u.Short)), u.ID)
		}
	}
	return m
}

// match: beste Zuordnung für einen Amt-Text (Unit 0 = keine oder mehrdeutig)
func (m *orgMatcher) match(amt string) orgMatch {
	na := normOrg(amt)
	if na == "" {
		return orgMatch{}
	}
	pick := func(ids []int64, kind string) orgMatch {
		if len(ids) == 1 {
			return orgMatch{Unit: ids[0], Kind: kind}
		}
		return orgMatch{Kind: kind, Candidates: ids}
	}
	if ids := m.exact[na]; len(ids) > 0 {
		return pick(ids, "exakt")
	}
	if sm := reLeadShort.FindStringSubmatch(amt); sm != nil {
		if ids := m.short[strings.ToLower(sm[1])]; len(ids) > 0 {
			return pick(ids, "kuerzel")
		}
	}
	// ähnlich: Einheitenname kommt als ganze Wortfolge im Amt-Text vor (oder umgekehrt)
	var sim []int64
	for _, u := range m.units {
		nn := normOrg(u.Name)
		if len([]rune(nn)) < 5 {
			continue
		}
		if strings.Contains(" "+na+" ", " "+nn+" ") || (len([]rune(na)) >= 5 && strings.Contains(" "+nn+" ", " "+na+" ")) {
			sim = append(sim, u.ID)
		}
	}
	if len(sim) > 0 {
		return pick(sim, "aehnlich")
	}
	return orgMatch{}
}

// isWithin: liegt unit innerhalb von (oder gleich) root?
func (m *orgMatcher) isWithin(unit, root int64) bool {
	for p, n := unit, 0; p != 0 && n < 64; p, n = m.parent[p], n+1 {
		if p == root {
			return true
		}
	}
	return false
}

type orgSuggestion struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Amt      string `json:"amt"`
	Current  int64  `json:"current,omitempty"`
	Proposed int64  `json:"proposed,omitempty"`
	orgMatch
	Change  bool `json:"change"`  // ist schon woanders zugeordnet
	Default bool `json:"default"` // Vorauswahl „übernehmen: ja“
}

func (a *App) orgSuggestions(ctx context.Context) ([]orgSuggestion, error) {
	units, err := a.st().ListOrgUnits(ctx)
	if err != nil {
		return nil, err
	}
	recs, err := a.st().ListEmployees(ctx, false)
	if err != nil {
		return nil, err
	}
	m := newOrgMatcher(units)
	out := []orgSuggestion{}
	for _, e := range recs {
		mt := m.match(e.Amt)
		if mt.Unit == 0 && len(mt.Candidates) == 0 {
			continue
		}
		cur := e.OrgUnit
		if _, ok := m.byID[cur]; !ok {
			cur = 0
		}
		if mt.Unit != 0 && cur != 0 && m.isWithin(cur, mt.Unit) {
			continue // passt schon (ggf. in einer Untereinheit)
		}
		if mt.Unit == 0 && cur != 0 {
			inside := false
			for _, c := range mt.Candidates {
				inside = inside || m.isWithin(cur, c)
			}
			if inside {
				continue
			}
		}
		s := orgSuggestion{ID: e.ID, Name: strings.TrimSpace(e.LastName + ", " + e.FirstName), Amt: e.Amt, Current: cur, Proposed: mt.Unit, orgMatch: mt, Change: cur != 0}
		s.Default = mt.Unit != 0 && cur == 0 && (mt.Kind == "exakt" || mt.Kind == "kuerzel")
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Default != out[j].Default {
			return out[i].Default
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (a *App) handleOrgSuggest(w http.ResponseWriter, r *http.Request) {
	list, err := a.orgSuggestions(r.Context())
	if err != nil {
		dbFail(w, err)
		return
	}
	v, _ := a.st().GetSetting(r.Context(), settingOrgAutoAssign)
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "autoAssign": v == "1"})
}

func (a *App) handleOrgAssign(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in struct {
		Assignments []struct {
			ID   int64 `json:"id"`
			Unit int64 `json:"unit"`
		} `json:"assignments"`
	}
	if !decodeJSON(w, r, &in, 512<<10) {
		return
	}
	if len(in.Assignments) > 5000 {
		jsonError(w, http.StatusBadRequest, "Zu viele Einträge auf einmal")
		return
	}
	ctx := r.Context()
	units, err := a.st().ListOrgUnits(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	ok := map[int64]bool{0: true}
	for _, u := range units {
		ok[u.ID] = true
	}
	group := map[int64][]int64{}
	for _, x := range in.Assignments {
		if !ok[x.Unit] {
			jsonError(w, http.StatusBadRequest, "Unbekannte Organisationseinheit")
			return
		}
		group[x.Unit] = append(group[x.Unit], x.ID)
	}
	n := 0
	for unit, ids := range group {
		k, err := a.st().SetOrgUnit(ctx, ids, unit)
		if err != nil {
			dbFail(w, err)
			return
		}
		n += k
	}
	a.audit(r, s.Username, "org_assign", "", fmt.Sprintf("%d Person(en) zugeordnet", n))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "assigned": n})
}

func (a *App) handleOrgAutoAssignSetting(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &in, 1<<10) {
		return
	}
	v := "0"
	if in.Enabled {
		v = "1"
	}
	if err := a.st().SetSetting(r.Context(), settingOrgAutoAssign, v); err != nil {
		dbFail(w, err)
		return
	}
	a.audit(r, s.Username, "org_auto_assign", "", v)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// autoAssignOrg: nach dem AD-Abgleich eindeutige Treffer (exakt/Kürzel) für noch nicht Zugeordnete übernehmen
func (a *App) autoAssignOrg(ctx context.Context, by string) int {
	if a.offline.Load() {
		return 0
	}
	if v, _ := a.st().GetSetting(ctx, settingOrgAutoAssign); v != "1" {
		return 0
	}
	list, err := a.orgSuggestions(ctx)
	if err != nil {
		return 0
	}
	n := 0
	for _, s := range list {
		if !s.Default {
			continue
		}
		if k, err := a.st().SetOrgUnit(ctx, []int64{s.ID}, s.Proposed); err == nil {
			n += k
		}
	}
	if n > 0 {
		log.Printf("Organisation automatisch zugeordnet (%s): %d", by, n)
		a.audit(nil, by, "org_assign", "automatisch", fmt.Sprintf("%d Person(en) zugeordnet", n))
	}
	return n
}
