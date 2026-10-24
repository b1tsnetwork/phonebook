package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------- Datentypen für die API ----------

type Employee struct {
	ID        int64           `json:"id"`
	Username  string          `json:"username,omitempty"`
	LastName  string          `json:"lastName"`
	FirstName string          `json:"firstName"`
	Phone     string          `json:"phone"`
	Mobile    string          `json:"mobile"`
	Room      string          `json:"room"`
	Amt       string          `json:"amt"`
	Email     string          `json:"email"`
	Source    string          `json:"source"`
	Active    bool            `json:"active"`
	Hidden    bool            `json:"hidden"`
	Avatar    json.RawMessage `json:"avatar,omitempty"`
	Tags      []int64         `json:"tags"`
}

type PrivateContact struct {
	ID        int64   `json:"id"`
	LastName  string  `json:"lastName"`
	FirstName string  `json:"firstName"`
	Phone     string  `json:"phone"`
	Mobile    string  `json:"mobile"`
	Room      string  `json:"room"`
	Amt       string  `json:"amt"`
	Email     string  `json:"email"`
	Note      string  `json:"note"`
	Shares    *Shares `json:"shares,omitempty"`
}

type Tag struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	Count     int    `json:"count"`
	CreatedBy string `json:"createdBy,omitempty"`
}

type TagInput struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

// categoryResolver prüft Kategorie-Schlüssel gegen den Bestand.
type categoryResolver struct {
	keys     map[string]bool
	fallback string
}

func (a *App) categories(ctx context.Context) (*categoryResolver, error) {
	cats, err := a.st().ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	cr := &categoryResolver{keys: map[string]bool{}}
	for _, c := range cats {
		cr.keys[c.Key] = true
		if cr.fallback == "" {
			cr.fallback = c.Key
		}
	}
	if cr.keys["taetigkeit"] {
		cr.fallback = "taetigkeit"
	}
	return cr, nil
}

// resolve: bekannter Schlüssel oder Standardkategorie
func (c *categoryResolver) resolve(k string) string {
	if c.keys[k] {
		return k
	}
	return c.fallback
}

// ---------- Hilfen ----------

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (a *App) audit(r *http.Request, actor, action, target, detail string) {
	ip := ""
	if r != nil {
		ip = a.clientIP(r)
	}
	if err := a.st().AddAudit(context.Background(), AuditEntry{TS: time.Now().UTC(), Actor: truncate(actor, 128), Action: action, Target: truncate(target, 255), Detail: detail, IP: ip}); err != nil {
		log.Printf("Audit-Log: %v", err)
	}
}

func dbFail(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrReadOnly) {
		jsonError(w, http.StatusServiceUnavailable, ErrReadOnly.Error())
		return
	}
	log.Printf("Speicher-Fehler: %v", err)
	jsonError(w, http.StatusInternalServerError, "Interner Fehler")
}

// storeErr übersetzt typische Store-Fehler in HTTP-Antworten.
func storeErr(w http.ResponseWriter, err error, notFound, duplicate string) {
	var ut ErrUnknownTag
	switch {
	case errors.Is(err, ErrNotFound):
		jsonError(w, http.StatusNotFound, notFound)
	case errors.Is(err, ErrDuplicate):
		jsonError(w, http.StatusConflict, duplicate)
	case errors.As(err, &ut):
		jsonError(w, http.StatusBadRequest, ut.Error())
	default:
		dbFail(w, err)
	}
}

var rePhone = regexp.MustCompile(`^[0-9+()\-/ .]*$`)

type contactFields struct {
	LastName  string `json:"lastName"`
	FirstName string `json:"firstName"`
	Phone     string `json:"phone"`
	Mobile    string `json:"mobile"`
	Room      string `json:"room"`
	Amt       string `json:"amt"`
	Email     string `json:"email"`
}

func (f *contactFields) clean() error {
	var ok bool
	type fld struct {
		p    *string
		max  int
		name string
	}
	for _, x := range []fld{{&f.LastName, 100, "Nachname"}, {&f.FirstName, 100, "Vorname"}, {&f.Phone, 64, "Telefon"}, {&f.Mobile, 64, "Mobil"}, {&f.Room, 64, "Raum"}, {&f.Amt, 150, "Amt"}, {&f.Email, 254, "E-Mail"}} {
		if *x.p, ok = cleanText(*x.p, x.max); !ok {
			return fmt.Errorf("%s ist zu lang (max. %d Zeichen)", x.name, x.max)
		}
	}
	if f.LastName == "" && f.FirstName == "" {
		return errors.New("Bitte mindestens Nachname oder Vorname angeben")
	}
	if !rePhone.MatchString(f.Phone) || !rePhone.MatchString(f.Mobile) {
		return errors.New("Telefonnummern dürfen nur Ziffern, Leerzeichen und + ( ) - / . enthalten")
	}
	if f.Email != "" {
		if _, err := mail.ParseAddress(f.Email); err != nil || strings.ContainsAny(f.Email, "<> ") {
			return errors.New("E-Mail-Adresse ist ungültig")
		}
	}
	return nil
}

func normalizeTagName(s string) (string, error) {
	s, ok := cleanText(s, 60)
	if !ok {
		return "", errors.New("Schlagwort zu lang (max. 60 Zeichen)")
	}
	if s == "" {
		return "", errors.New("Leeres Schlagwort")
	}
	if strings.ContainsAny(s, "<>") {
		return "", errors.New("Schlagwort enthält unzulässige Zeichen")
	}
	return s, nil
}

func empLabel(e *EmployeeRec) string { return fmt.Sprintf("#%d %s, %s", e.ID, e.LastName, e.FirstName) }

// ---------- /api/me ----------

func (a *App) meJSON(s *Session) map[string]any {
	return map[string]any{
		"username": s.Username, "displayName": s.DisplayName, "isAdmin": s.IsAdmin, "source": s.Source,
		"employeeId": s.EmployeeID, "csrf": s.CSRF,
		"features": map[string]any{
			"userCreatesTags": a.cfg.Features.UserCreatesTags, "maxTags": a.cfg.Features.MaxTagsPerUser,
			"maxPrivate": a.cfg.Features.MaxPrivate, "syncEnabled": a.cfg.Sync.Enabled, "adLocked": a.cfg.adLockedFields(),
			"storage": a.st().Kind(), "devAdmin": a.cfg.Auth.DevAdmin, "offline": a.offline.Load(),
		},
	}
}

// siteJSON: öffentliche Angaben für die Oberfläche (auch ohne Anmeldung)
func (a *App) siteJSON() map[string]any {
	out := map[string]any{"publicDirectory": a.cfg.Auth.PublicDirectory, "devAdmin": a.cfg.Auth.DevAdmin, "storage": a.st().Kind(), "offline": a.offline.Load()}
	if ro, ok := a.st().(*readOnlyStore); ok && a.offline.Load() {
		out["offlineSince"] = ro.savedAt // Stand der Kopie
	}
	return out
}

func (a *App) handleSite(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.siteJSON())
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.meJSON(sessionFrom(r)))
}

// ---------- Verzeichnis ----------

func (a *App) handleDirectory(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	guest := s == nil
	if guest {
		s = &Session{} // Gast: nur lesen, keine Admin-/Benutzerdaten
	}
	ctx := r.Context()
	recs, err := a.st().ListEmployees(ctx, s.IsAdmin)
	if err != nil {
		dbFail(w, err)
		return
	}
	emps := make([]Employee, 0, len(recs))
	counts := map[int64]int{}
	for i := range recs {
		e := &recs[i]
		out := Employee{ID: e.ID, Username: e.Username, LastName: e.LastName, FirstName: e.FirstName, Phone: e.Phone, Mobile: e.Mobile,
			Room: e.Room, Amt: e.Amt, Email: e.Email, Source: e.Source, Active: e.Active, Hidden: e.Hidden, Avatar: e.Avatar, Tags: e.Tags}
		if !s.IsAdmin && e.ID != s.EmployeeID {
			out.Username = ""
		}
		if e.Active && !e.Hidden {
			for _, t := range e.Tags {
				counts[t]++
			}
		}
		emps = append(emps, out)
	}
	tags, err := a.st().ListTags(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	visible := []Tag{}
	for _, t := range tags {
		t.Count = counts[t.ID]
		if !s.IsAdmin {
			t.CreatedBy = ""
		}
		if t.Count > 0 || s.IsAdmin {
			visible = append(visible, t)
		}
	}
	cats, err := a.st().ListCategories(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	if !s.IsAdmin {
		for i := range cats {
			cats[i].Count = 0
		}
	}
	priv := []PrivateContact{}
	shared := []SharedContact{}
	var me any
	if !guest {
		if priv, err = a.st().ListPrivate(ctx, s.Username); err != nil {
			dbFail(w, err)
			return
		}
		if shared, err = a.sharedWithMe(ctx, s, recs); err != nil {
			dbFail(w, err)
			return
		}
		me = a.meJSON(s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"employees": emps, "tags": visible, "categories": cats, "private": priv, "shared": shared, "me": me, "site": a.siteJSON()})
}

// SharedContact: privater Kontakt eines anderen, der für mich freigegeben wurde (nur lesen)
type SharedContact struct {
	PrivateContact
	OwnerName string `json:"ownerName"`
	OwnerID   int64  `json:"ownerId,omitempty"`
}

func (a *App) sharedWithMe(ctx context.Context, s *Session, recs []EmployeeRec) ([]SharedContact, error) {
	out := []SharedContact{}
	if s.EmployeeID == 0 {
		return out, nil
	}
	amt := ""
	byUser := map[string]*EmployeeRec{}
	for i := range recs {
		if recs[i].ID == s.EmployeeID {
			amt = recs[i].Amt
		}
		if recs[i].Username != "" {
			byUser[strings.ToLower(recs[i].Username)] = &recs[i]
		}
	}
	list, err := a.st().ListSharedWith(ctx, s.EmployeeID, amt, s.Username)
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		c := p.PrivateContact
		c.Shares = nil
		sc := SharedContact{PrivateContact: c, OwnerName: p.Owner}
		if e := byUser[strings.ToLower(p.Owner)]; e != nil {
			sc.OwnerName = strings.TrimSpace(e.FirstName + " " + e.LastName)
			sc.OwnerID = e.ID
		} else if lu, err := a.st().GetLocalUser(ctx, p.Owner); err == nil && lu.DisplayName != "" {
			sc.OwnerName = lu.DisplayName
		}
		out = append(out, sc)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].LastName+" "+out[i].FirstName) < strings.ToLower(out[j].LastName+" "+out[j].FirstName)
	})
	return out, nil
}

// ---------- Schlagworte eines Mitarbeiters ----------

func (a *App) canEditEmployee(s *Session, id int64) bool {
	return s.IsAdmin || (s.EmployeeID != 0 && s.EmployeeID == id)
}

func (a *App) handleSetTags(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	if !a.canEditEmployee(s, id) {
		jsonError(w, http.StatusForbidden, "Du darfst nur deine eigenen Schlagworte bearbeiten")
		return
	}
	var req struct {
		Tags []TagInput `json:"tags"`
	}
	if !decodeJSON(w, r, &req, 32<<10) {
		return
	}
	if len(req.Tags) > a.cfg.Features.MaxTagsPerUser {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("Höchstens %d Schlagworte pro Person", a.cfg.Features.MaxTagsPerUser))
		return
	}
	cr, err := a.categories(r.Context())
	if err != nil {
		dbFail(w, err)
		return
	}
	in := make([]TagInput, 0, len(req.Tags))
	names := []string{}
	for _, t := range req.Tags {
		name, err := normalizeTagName(t.Name)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		t.Category = cr.resolve(t.Category)
		in = append(in, TagInput{Name: name, Category: t.Category})
		names = append(names, name)
	}
	ctx := r.Context()
	e, err := a.st().GetEmployee(ctx, id)
	if err != nil {
		storeErr(w, err, "Eintrag nicht gefunden", "")
		return
	}
	ids, err := a.st().SetEmployeeTags(ctx, id, in, s.Username, s.IsAdmin || a.cfg.Features.UserCreatesTags)
	if err != nil {
		storeErr(w, err, "Eintrag nicht gefunden", "")
		return
	}
	sort.Strings(names)
	a.audit(r, s.Username, "tags_update", empLabel(e), strings.Join(names, ", "))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tags": ids})
}

// ---------- Avatar ----------

func (a *App) handleSetAvatar(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	if !a.canEditEmployee(s, id) {
		jsonError(w, http.StatusForbidden, "Du darfst nur deinen eigenen Avatar bearbeiten")
		return
	}
	var req struct {
		Avatar json.RawMessage `json:"avatar"`
	}
	if !decodeJSON(w, r, &req, maxAvatarBytes+1024) {
		return
	}
	av, err := validateAvatar(req.Avatar)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.st().SetAvatar(r.Context(), id, json.RawMessage(av)); err != nil {
		storeErr(w, err, "Eintrag nicht gefunden", "")
		return
	}
	if s.EmployeeID != id {
		if e, err := a.st().GetEmployee(r.Context(), id); err == nil {
			a.audit(r, s.Username, "avatar_update", empLabel(e), "")
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- Stammdaten (nur Admins) ----------

type employeeInput struct {
	contactFields
	Username *string `json:"username"`
	Hidden   bool    `json:"hidden"`
}

func cleanUsername(p *string) (string, error) {
	if p == nil {
		return "", nil
	}
	u := normalizeLoginName(*p)
	if u == "" {
		return "", nil
	}
	if !reLoginName.MatchString(u) {
		return "", errors.New("Benutzername ungültig (nur a–z, 0–9, Punkt, Bindestrich, Unterstrich)")
	}
	return u, nil
}

const dupUser = "Dieser Benutzername ist bereits einem anderen Eintrag zugeordnet"

func (a *App) handleCreateEmployee(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in employeeInput
	if !decodeJSON(w, r, &in, 16<<10) {
		return
	}
	if err := in.clean(); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := cleanUsername(in.Username)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	e := &EmployeeRec{Username: user, Source: "manual", Active: true, Hidden: in.Hidden, CreatedAt: now, UpdatedAt: now, Tags: []int64{}}
	e.setFields(in.contactFields)
	id, err := a.st().CreateEmployee(r.Context(), e)
	if err != nil {
		storeErr(w, err, "", dupUser)
		return
	}
	e.ID = id
	a.audit(r, s.Username, "employee_create", empLabel(e), "")
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (a *App) handleUpdateEmployee(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var in employeeInput
	if !decodeJSON(w, r, &in, 16<<10) {
		return
	}
	ctx := r.Context()
	cur, err := a.st().GetEmployee(ctx, id)
	if err != nil {
		storeErr(w, err, "Eintrag nicht gefunden", "")
		return
	}
	old := cur.fields()
	// Vom AD gepflegte Felder behalten ihren Wert
	if cur.Source == "ad" {
		for _, f := range a.cfg.adLockedFields() {
			switch f {
			case "lastName":
				in.LastName = old.LastName
			case "firstName":
				in.FirstName = old.FirstName
			case "phone":
				in.Phone = old.Phone
			case "mobile":
				in.Mobile = old.Mobile
			case "room":
				in.Room = old.Room
			case "amt":
				in.Amt = old.Amt
			case "email":
				in.Email = old.Email
			}
		}
	}
	if err := in.clean(); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := cleanUsername(in.Username)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	cur.Username = user
	cur.setFields(in.contactFields)
	cur.Hidden = in.Hidden
	cur.UpdatedAt = time.Now().UTC()
	if err := a.st().UpdateEmployee(ctx, cur); err != nil {
		storeErr(w, err, "Eintrag nicht gefunden", dupUser)
		return
	}
	a.audit(r, s.Username, "employee_update", empLabel(cur), diffContact(old, in.contactFields))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func diffContact(a, b contactFields) string {
	var ch []string
	add := func(name, x, y string) {
		if x != y {
			ch = append(ch, fmt.Sprintf("%s: „%s“ → „%s“", name, x, y))
		}
	}
	add("Nachname", a.LastName, b.LastName)
	add("Vorname", a.FirstName, b.FirstName)
	add("Telefon", a.Phone, b.Phone)
	add("Mobil", a.Mobile, b.Mobile)
	add("Raum", a.Room, b.Room)
	add("Amt", a.Amt, b.Amt)
	add("E-Mail", a.Email, b.Email)
	return strings.Join(ch, "; ")
}

func (a *App) handleDeleteEmployee(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	e, err := a.st().GetEmployee(r.Context(), id)
	if err == nil {
		err = a.st().DeleteEmployee(r.Context(), id)
	}
	if err != nil {
		storeErr(w, err, "Eintrag nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "employee_delete", empLabel(e), "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- Schlagwort-Verwaltung (nur Admins) ----------

func (a *App) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := a.st().ListTags(r.Context())
	if err != nil {
		dbFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (a *App) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in TagInput
	if !decodeJSON(w, r, &in, 4<<10) {
		return
	}
	name, err := normalizeTagName(in.Name)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	cr, err := a.categories(r.Context())
	if err != nil {
		dbFail(w, err)
		return
	}
	in.Category = cr.resolve(in.Category)
	id, err := a.st().CreateTag(r.Context(), name, in.Category, s.Username)
	if err != nil {
		storeErr(w, err, "", "Dieses Schlagwort gibt es schon")
		return
	}
	a.audit(r, s.Username, "tag_create", name, in.Category)
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (a *App) handleUpdateTag(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var in TagInput
	if !decodeJSON(w, r, &in, 4<<10) {
		return
	}
	name, err := normalizeTagName(in.Name)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	cr, err := a.categories(r.Context())
	if err != nil {
		dbFail(w, err)
		return
	}
	if !cr.keys[in.Category] {
		jsonError(w, http.StatusBadRequest, "Unbekannte Kategorie")
		return
	}
	old, err := a.st().UpdateTag(r.Context(), id, name, in.Category)
	if err != nil {
		storeErr(w, err, "Schlagwort nicht gefunden", "Ein anderes Schlagwort heißt schon so – zum Zusammenführen „Zusammenführen“ verwenden")
		return
	}
	a.audit(r, s.Username, "tag_update", old, name+" ("+in.Category+")")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleMergeTag(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var in struct {
		Into int64 `json:"into"`
	}
	if !decodeJSON(w, r, &in, 1<<10) {
		return
	}
	if in.Into == id || in.Into <= 0 {
		jsonError(w, http.StatusBadRequest, "Ziel-Schlagwort ungültig")
		return
	}
	from, into, err := a.st().MergeTag(r.Context(), id, in.Into)
	if err != nil {
		storeErr(w, err, "Schlagwort nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "tag_merge", from, "→ "+into)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	name, err := a.st().DeleteTag(r.Context(), id)
	if err != nil {
		storeErr(w, err, "Schlagwort nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "tag_delete", name, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleDeleteUnusedTags(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	n, err := a.st().DeleteUnusedTags(r.Context())
	if err != nil {
		dbFail(w, err)
		return
	}
	a.audit(r, s.Username, "tag_cleanup", "", fmt.Sprintf("%d entfernt", n))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": n})
}

// ---------- Private Kontakte (nur für den Besitzer sichtbar) ----------

type privateInput struct {
	contactFields
	Note   string  `json:"note"`
	Shares *Shares `json:"shares"`
}

const (
	maxShareEmployees = 100
	maxShareAmts      = 20
)

// cleanShares prüft Freigaben: nur existierende Einträge, keine Dubletten, Ämter bereinigt.
func (a *App) cleanShares(ctx context.Context, s *Session, sh *Shares) (*Shares, error) {
	if sh.empty() {
		return nil, nil
	}
	if len(sh.Employees) > maxShareEmployees || len(sh.Amts) > maxShareAmts {
		return nil, fmt.Errorf("Höchstens %d Personen und %d Ämter pro Freigabe", maxShareEmployees, maxShareAmts)
	}
	out := &Shares{Employees: []int64{}, Amts: []string{}}
	seen := map[int64]bool{}
	for _, id := range sh.Employees {
		if id <= 0 || seen[id] || id == s.EmployeeID {
			continue
		}
		if _, err := a.st().GetEmployee(ctx, id); errors.Is(err, ErrNotFound) {
			return nil, errors.New("Freigabe: Eintrag existiert nicht (mehr)")
		} else if err != nil {
			return nil, err
		}
		seen[id] = true
		out.Employees = append(out.Employees, id)
	}
	seenA := map[string]bool{}
	for _, am := range sh.Amts {
		am, ok := cleanText(am, 150)
		if !ok {
			return nil, errors.New("Freigabe: Amt zu lang")
		}
		if am == "" || seenA[strings.ToLower(am)] {
			continue
		}
		seenA[strings.ToLower(am)] = true
		out.Amts = append(out.Amts, am)
	}
	sort.Slice(out.Employees, func(i, j int) bool { return out.Employees[i] < out.Employees[j] })
	return out.clone(), nil
}

func (p *privateInput) clean() error {
	if err := p.contactFields.clean(); err != nil {
		return err
	}
	var ok bool
	if p.Note, ok = cleanMultiline(p.Note, 500); !ok {
		return errors.New("Notiz zu lang (max. 500 Zeichen)")
	}
	return nil
}

func (p *privateInput) contact(id int64) PrivateContact {
	f := p.contactFields
	return PrivateContact{ID: id, LastName: f.LastName, FirstName: f.FirstName, Phone: f.Phone, Mobile: f.Mobile, Room: f.Room, Amt: f.Amt, Email: f.Email, Note: p.Note, Shares: p.Shares}
}

func (a *App) handleCreatePrivate(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in privateInput
	if !decodeJSON(w, r, &in, 8<<10) {
		return
	}
	if err := in.clean(); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	sh, err := a.cleanShares(r.Context(), s, in.Shares)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Shares = sh
	n, err := a.st().CountPrivate(r.Context(), s.Username)
	if err != nil {
		dbFail(w, err)
		return
	}
	if n >= a.cfg.Features.MaxPrivate {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("Höchstens %d private Kontakte", a.cfg.Features.MaxPrivate))
		return
	}
	id, err := a.st().CreatePrivate(r.Context(), s.Username, in.contact(0))
	if err != nil {
		dbFail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (a *App) handleUpdatePrivate(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var in privateInput
	if !decodeJSON(w, r, &in, 8<<10) {
		return
	}
	if err := in.clean(); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	sh, err := a.cleanShares(r.Context(), s, in.Shares)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Shares = sh
	if err := a.st().UpdatePrivate(r.Context(), s.Username, in.contact(id)); err != nil {
		storeErr(w, err, "Kontakt nicht gefunden", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleDeletePrivate(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	if err := a.st().DeletePrivate(r.Context(), s.Username, id); err != nil {
		storeErr(w, err, "Kontakt nicht gefunden", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- Admin: Protokoll ----------

func (a *App) handleAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 100 {
		q = q[:100]
	}
	out, err := a.st().ListAudit(r.Context(), q, limit)
	if err != nil {
		dbFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out})
}

// ---------- Beim ersten LDAP-Login eigenen Eintrag verknüpfen/anlegen ----------

func (a *App) ensureEmployeeForLogin(ctx context.Context, u *LDAPUser) {
	id, err := a.st().EmployeeIDByUsername(ctx, u.Username)
	if err != nil {
		log.Printf("Login-Verknüpfung: %v", err)
		return
	}
	if id != 0 {
		return
	}
	if u.GUID != "" {
		if ok, err := a.st().LinkUsernameByGUID(ctx, u.GUID, u.Username); err == nil && ok {
			return
		}
	}
	if !a.cfg.Features.AutoCreateOnLogin {
		return
	}
	f := contactFields{LastName: u.Fields["lastName"], FirstName: u.Fields["firstName"], Phone: u.Fields["phone"], Mobile: u.Fields["mobile"], Room: u.Fields["room"], Amt: u.Fields["amt"], Email: u.Fields["email"]}
	if f.LastName == "" && f.FirstName == "" {
		f.LastName = u.DisplayName
		if f.LastName == "" {
			f.LastName = u.Username
		}
	}
	sanitizeSyncFields(&f)
	now := time.Now().UTC()
	e := &EmployeeRec{Username: u.Username, Source: "manual", Active: true, CreatedAt: now, UpdatedAt: now, Tags: []int64{}}
	if u.GUID != "" {
		e.Source, e.ADGUID, e.ADDN, e.LastSyncedAt = "ad", u.GUID, truncate(u.DN, 512), &now
	}
	e.setFields(f)
	if _, err := a.st().CreateEmployee(ctx, e); err != nil && !errors.Is(err, ErrDuplicate) {
		log.Printf("Eintrag beim Login anlegen: %v", err)
	}
}

// ---------- Kategorien (nur Admins) ----------

var reColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type categoryInput struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	Sort  *int   `json:"sort"`
}

func (c *categoryInput) clean() error {
	name, ok := cleanText(c.Name, 40)
	if !ok {
		return errors.New("Name zu lang (max. 40 Zeichen)")
	}
	if name == "" || strings.ContainsAny(name, "<>") {
		return errors.New("Bitte einen gültigen Namen angeben")
	}
	c.Name = name
	if c.Color == "" {
		c.Color = "#7c8591"
	}
	if !reColor.MatchString(c.Color) {
		return errors.New("Farbe ungültig (Format #rrggbb)")
	}
	c.Color = strings.ToLower(c.Color)
	if c.Sort != nil && (*c.Sort < 0 || *c.Sort > 9999) {
		return errors.New("Sortierung muss zwischen 0 und 9999 liegen")
	}
	return nil
}

// slugify bildet aus dem Namen einen stabilen Schlüssel (a-z, 0-9, -)
func slugify(s string) string {
	r := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", "Ä", "ae", "Ö", "oe", "Ü", "ue")
	s = strings.ToLower(r.Replace(s))
	var b strings.Builder
	dash := false
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 32 {
		out = strings.Trim(out[:32], "-")
	}
	if out == "" {
		out = "kategorie"
	}
	return out
}

func (a *App) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in categoryInput
	if !decodeJSON(w, r, &in, 2<<10) {
		return
	}
	if err := in.clean(); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	cats, err := a.st().ListCategories(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	if len(cats) >= 30 {
		jsonError(w, http.StatusBadRequest, "Höchstens 30 Kategorien")
		return
	}
	used, maxSort := map[string]bool{}, 0
	for _, c := range cats {
		used[c.Key] = true
		maxSort = max(maxSort, c.Sort)
	}
	base := slugify(in.Name)
	key := base
	for i := 2; used[key]; i++ {
		key = fmt.Sprintf("%s-%d", base, i)
	}
	c := Category{Key: key, Name: in.Name, Color: in.Color, Sort: maxSort + 10}
	if in.Sort != nil {
		c.Sort = *in.Sort
	}
	if err := a.st().CreateCategory(ctx, c); err != nil {
		storeErr(w, err, "", "Eine Kategorie mit diesem Namen gibt es schon")
		return
	}
	a.audit(r, s.Username, "category_create", c.Name, c.Key+" "+c.Color)
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "key": key})
}

func (a *App) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	key := r.PathValue("key")
	if !reCatKey.MatchString(key) {
		jsonError(w, http.StatusBadRequest, "Ungültiger Schlüssel")
		return
	}
	var in categoryInput
	if !decodeJSON(w, r, &in, 2<<10) {
		return
	}
	if err := in.clean(); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	cats, err := a.st().ListCategories(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	var cur *Category
	for i := range cats {
		if cats[i].Key == key {
			cur = &cats[i]
		}
	}
	if cur == nil {
		jsonError(w, http.StatusNotFound, "Kategorie nicht gefunden")
		return
	}
	c := Category{Key: key, Name: in.Name, Color: in.Color, Sort: cur.Sort}
	if in.Sort != nil {
		c.Sort = *in.Sort
	}
	if err := a.st().UpdateCategory(ctx, c); err != nil {
		storeErr(w, err, "Kategorie nicht gefunden", "Eine Kategorie mit diesem Namen gibt es schon")
		return
	}
	a.audit(r, s.Username, "category_update", cur.Name, fmt.Sprintf("%s %s %d", c.Name, c.Color, c.Sort))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	key := r.PathValue("key")
	var in struct {
		MoveTo string `json:"moveTo"`
	}
	if !decodeJSON(w, r, &in, 1<<10) {
		return
	}
	if !reCatKey.MatchString(key) || !reCatKey.MatchString(in.MoveTo) {
		jsonError(w, http.StatusBadRequest, "Bitte eine Folge-Kategorie auswählen")
		return
	}
	if key == in.MoveTo {
		jsonError(w, http.StatusBadRequest, "Die Folge-Kategorie muss eine andere sein")
		return
	}
	ctx := r.Context()
	cats, err := a.st().ListCategories(ctx)
	if err != nil {
		dbFail(w, err)
		return
	}
	names := map[string]string{}
	for _, c := range cats {
		names[c.Key] = c.Name
	}
	if len(cats) <= 1 {
		jsonError(w, http.StatusBadRequest, "Die letzte Kategorie kann nicht gelöscht werden")
		return
	}
	n, err := a.st().DeleteCategory(ctx, key, in.MoveTo)
	if err != nil {
		storeErr(w, err, "Kategorie nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "category_delete", names[key], fmt.Sprintf("%d Schlagworte → %s", n, names[in.MoveTo]))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "moved": n})
}
