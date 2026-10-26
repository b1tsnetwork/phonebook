package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// Kontaktwege: eigenständig angelegt (z. B. „IT-Support“: Ticketsystem, Postfach, Hotline) und
// Personen gezielt zugewiesen – unabhängig von der Organisation.

const maxChannels = 200

type channelInput struct {
	Name string `json:"name"`
	ContactPref
}

func (a *App) cleanChannel(id int64, in channelInput) (ContactChannel, error) {
	name, ok := cleanText(in.Name, 100)
	if !ok || name == "" || strings.ContainsAny(name, "<>") {
		return ContactChannel{}, errors.New("Bitte einen Namen angeben (max. 100 Zeichen), z. B. „IT-Support“")
	}
	c, err := cleanContact(&in.ContactPref)
	if err != nil {
		return ContactChannel{}, err
	}
	if c == nil {
		return ContactChannel{}, errors.New("Bitte mindestens Ticketsystem, E-Mail oder Hotline angeben")
	}
	return ContactChannel{ID: id, Name: name, ContactPref: *c}, nil
}

func (a *App) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in channelInput
	if !decodeJSON(w, r, &in, 8<<10) {
		return
	}
	c, err := a.cleanChannel(0, in)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if list, err := a.st().ListChannels(r.Context()); err == nil && len(list) >= maxChannels {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("Höchstens %d Kontaktwege", maxChannels))
		return
	}
	id, err := a.st().CreateChannel(r.Context(), c)
	if err != nil {
		dbFail(w, err)
		return
	}
	a.audit(r, s.Username, "channel_create", c.Name, "")
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (a *App) handleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	var in channelInput
	if !decodeJSON(w, r, &in, 8<<10) {
		return
	}
	c, err := a.cleanChannel(id, in)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.st().UpdateChannel(r.Context(), c); err != nil {
		storeErr(w, err, "Kontaktweg nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "channel_update", c.Name, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, http.StatusBadRequest, "Ungültige ID")
		return
	}
	if err := a.st().DeleteChannel(r.Context(), id); err != nil {
		storeErr(w, err, "Kontaktweg nicht gefunden", "")
		return
	}
	a.audit(r, s.Username, "channel_delete", fmt.Sprintf("#%d", id), "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Personen zuweisen (add) bzw. entfernen (remove)
func (a *App) handleChannelMembers(w http.ResponseWriter, r *http.Request) {
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
	if !decodeJSON(w, r, &in, 128<<10) {
		return
	}
	if len(in.Add)+len(in.Remove) > maxMembersRequest {
		jsonError(w, http.StatusBadRequest, "Zu viele Einträge auf einmal")
		return
	}
	ctx := r.Context()
	ch, err := a.channelByID(ctx, id)
	if err != nil {
		storeErr(w, err, "Kontaktweg nicht gefunden", "")
		return
	}
	added, err := a.st().SetChannel(ctx, in.Add, id)
	if err != nil {
		dbFail(w, err)
		return
	}
	var rm []int64
	for _, eid := range in.Remove {
		if e, err := a.st().GetEmployee(ctx, eid); err == nil && e.Channel == id {
			rm = append(rm, eid)
		}
	}
	removed, err := a.st().SetChannel(ctx, rm, 0)
	if err != nil {
		dbFail(w, err)
		return
	}
	a.audit(r, s.Username, "channel_members", ch.Name, fmt.Sprintf("+%d / −%d", added, removed))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "added": added, "removed": removed})
}

func (a *App) channelByID(ctx context.Context, id int64) (*ContactChannel, error) {
	list, err := a.st().ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, ErrNotFound
}

// applyChannel setzt den Kontaktweg eines Eintrags, wenn angegeben (0 = keiner)
func (a *App) applyChannel(ctx context.Context, id int64, ch *int64) error {
	if ch == nil {
		return nil
	}
	if *ch != 0 {
		if _, err := a.channelByID(ctx, *ch); err != nil {
			return errors.New("Kontaktweg existiert nicht")
		}
	}
	_, err := a.st().SetChannel(ctx, []int64{id}, *ch)
	return err
}

// migrateUnitContacts: Kontaktwege, die (Vorversion) an Einheiten hingen, in eigenständige Kontaktwege
// umwandeln und den bisher betroffenen Personen zuweisen – einmalig beim Start.
func migrateUnitContacts(ctx context.Context, st Store) {
	units, err := st.ListOrgUnits(ctx)
	if err != nil {
		return
	}
	byID := map[int64]OrgUnit{}
	has := false
	for _, u := range units {
		byID[u.ID] = u
		has = has || u.Contact != nil
	}
	if !has {
		return
	}
	recs, err := st.ListEmployees(ctx, true)
	if err != nil {
		return
	}
	chanFor := map[int64]int64{} // Einheit → neuer Kontaktweg
	for _, u := range units {
		if u.Contact == nil {
			continue
		}
		name := u.Name
		if u.Short != "" {
			name = u.Short + " – " + u.Name
		}
		id, err := st.CreateChannel(ctx, ContactChannel{Name: truncate(name, 100), ContactPref: *u.Contact})
		if err != nil {
			log.Printf("Kontaktweg übernehmen (%s): %v", u.Name, err)
			return
		}
		chanFor[u.ID] = id
	}
	n := 0
	for _, e := range recs {
		if e.Channel != 0 || e.OrgUnit == 0 {
			continue
		}
		for p, k := e.OrgUnit, 0; p != 0 && k < 64; p, k = byID[p].Parent, k+1 {
			if ch, ok := chanFor[p]; ok {
				if _, err := st.SetChannel(ctx, []int64{e.ID}, ch); err == nil {
					n++
				}
				break
			}
		}
	}
	for _, u := range units {
		if u.Contact != nil {
			u.Contact = nil
			_ = st.UpdateOrgUnit(ctx, u)
		}
	}
	log.Printf("Kontaktwege: %d aus Organisationseinheiten übernommen, %d Personen zugewiesen", len(chanFor), n)
}
