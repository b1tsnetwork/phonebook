package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SyncResult struct {
	ID          int64      `json:"id"`
	StartedAt   time.Time  `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
	TriggerBy   string     `json:"triggerBy"`
	Status      string     `json:"status"`
	Seen        int        `json:"seen"`
	Created     int        `json:"created"`
	Updated     int        `json:"updated"`
	Deactivated int        `json:"deactivated"`
	Reactivated int        `json:"reactivated"`
	Message     string     `json:"message"`
}

type syncer struct {
	mu      sync.Mutex
	running bool
}

func sanitizeSyncFields(f *contactFields) {
	f.LastName, _ = cleanText(f.LastName, 1000)
	f.FirstName, _ = cleanText(f.FirstName, 1000)
	f.Phone, _ = cleanText(f.Phone, 1000)
	f.Mobile, _ = cleanText(f.Mobile, 1000)
	f.Room, _ = cleanText(f.Room, 1000)
	f.Amt, _ = cleanText(f.Amt, 1000)
	f.Email, _ = cleanText(f.Email, 1000)
	f.LastName, f.FirstName = truncate(f.LastName, 100), truncate(f.FirstName, 100)
	f.Phone, f.Mobile, f.Room = truncate(f.Phone, 64), truncate(f.Mobile, 64), truncate(f.Room, 64)
	f.Amt, f.Email = truncate(f.Amt, 150), truncate(f.Email, 254)
}

// RunSync gleicht die Stammdaten mit dem Active Directory ab.
func (a *App) RunSync(ctx context.Context, trigger string) (*SyncResult, error) {
	a.sync.mu.Lock()
	if a.sync.running {
		a.sync.mu.Unlock()
		return nil, errors.New("Ein Abgleich läuft bereits")
	}
	a.sync.running = true
	a.sync.mu.Unlock()
	defer func() { a.sync.mu.Lock(); a.sync.running = false; a.sync.mu.Unlock() }()

	res := &SyncResult{StartedAt: time.Now().UTC(), TriggerBy: trigger, Status: "running"}
	id, err := a.st().StartSyncRun(ctx, trigger, res.StartedAt)
	if err != nil {
		return nil, err
	}
	res.ID = id

	err = a.doSync(ctx, res)
	now := time.Now().UTC()
	res.FinishedAt = &now
	if err != nil {
		res.Status = "error"
		res.Message = err.Error()
	} else {
		res.Status = "ok"
	}
	_ = a.st().FinishSyncRun(context.Background(), res)
	a.audit(nil, trigger, "ad_sync", res.Status, fmt.Sprintf("gelesen %d, neu %d, geändert %d, deaktiviert %d, reaktiviert %d %s", res.Seen, res.Created, res.Updated, res.Deactivated, res.Reactivated, res.Message))
	log.Printf("AD-Sync (%s): %s – gelesen %d, neu %d, geändert %d, deaktiviert %d, reaktiviert %d %s", trigger, res.Status, res.Seen, res.Created, res.Updated, res.Deactivated, res.Reactivated, res.Message)
	return res, err
}

func (a *App) doSync(ctx context.Context, res *SyncResult) error {
	if a.ldap == nil {
		return errors.New("LDAP ist nicht konfiguriert")
	}
	users, err := a.ldap.SearchAll(a.cfg.Sync.BaseDN, a.cfg.Sync.Filter, a.cfg.Sync.PageSize)
	if err != nil {
		return err
	}
	current, err := a.st().ListEmployees(ctx, true)
	if err != nil {
		return err
	}
	plan := buildSyncPlan(users, current, &a.cfg.Sync, res)
	if plan == nil {
		return errors.New(res.Message)
	}
	return a.st().ApplySyncPlan(ctx, plan)
}

// buildSyncPlan vergleicht AD-Benutzer mit den vorhandenen Einträgen (rein, ohne Speicherzugriff).
// Bei Überschreiten der Deaktivierungsgrenze: nil und Meldung in res.Message.
func buildSyncPlan(users []LDAPUser, current []EmployeeRec, cfg *SyncConfig, res *SyncResult) *SyncPlan {
	fields := map[string]bool{}
	for _, f := range cfg.Fields {
		fields[f] = true
	}
	byGUID := map[string]*EmployeeRec{}
	byUser := map[string]*EmployeeRec{}
	adCount := 0
	for i := range current {
		e := &current[i]
		if e.ADGUID != "" {
			byGUID[strings.ToLower(e.ADGUID)] = e
		}
		if e.Username != "" {
			byUser[strings.ToLower(e.Username)] = e
		}
		if e.Source == "ad" && e.Active {
			adCount++
		}
	}
	plan := &SyncPlan{Now: time.Now().UTC()}
	seen := map[int64]bool{}
	newGUID := map[string]bool{}
	newUser := map[string]bool{}
	for _, u := range users {
		if u.GUID == "" && u.Username == "" {
			continue
		}
		f := contactFields{LastName: u.Fields["lastName"], FirstName: u.Fields["firstName"], Phone: u.Fields["phone"], Mobile: u.Fields["mobile"], Room: u.Fields["room"], Amt: u.Fields["amt"], Email: u.Fields["email"]}
		sanitizeSyncFields(&f)
		if f.LastName == "" && f.FirstName == "" {
			continue // Dienstkonten o. Ä. ohne Namen
		}
		if cfg.SkipWithoutPhone && f.Phone == "" && f.Mobile == "" {
			continue
		}
		res.Seen++
		var e *EmployeeRec
		if u.GUID != "" {
			e = byGUID[u.GUID]
		}
		if e == nil && u.Username != "" {
			if c := byUser[u.Username]; c != nil && (c.ADGUID == "" || c.ADGUID == u.GUID) {
				e = c
			}
		}
		if e == nil {
			if (u.GUID != "" && newGUID[u.GUID]) || (u.GUID == "" && newUser[u.Username]) {
				continue // doppelt im AD-Ergebnis
			}
			user := u.Username
			if user != "" && (byUser[user] != nil || newUser[user]) {
				user = ""
			}
			ne := EmployeeRec{Username: user, Source: "ad", ADGUID: u.GUID, ADDN: truncate(u.DN, 512), Active: true, Tags: []int64{}}
			ne.setFields(f)
			plan.Creates = append(plan.Creates, ne)
			if u.GUID != "" {
				newGUID[u.GUID] = true
			}
			if user != "" {
				newUser[user] = true
			}
			res.Created++
			continue
		}
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		old := e.fields()
		merged := old
		if fields["lastName"] {
			merged.LastName = f.LastName
		}
		if fields["firstName"] {
			merged.FirstName = f.FirstName
		}
		if fields["phone"] {
			merged.Phone = f.Phone
		}
		if fields["mobile"] {
			merged.Mobile = f.Mobile
		}
		if fields["room"] {
			merged.Room = f.Room
		}
		if fields["amt"] {
			merged.Amt = f.Amt
		}
		if fields["email"] {
			merged.Email = f.Email
		}
		changed := merged != old || e.Source != "ad" || !e.Active || (u.GUID != "" && e.ADGUID != u.GUID)
		if !e.Active {
			res.Reactivated++
		}
		if changed {
			res.Updated++
		}
		up := SyncUpdate{ID: e.ID, Fields: merged, GUID: u.GUID, DN: truncate(u.DN, 512), Changed: changed}
		if e.Username == "" && u.Username != "" && byUser[u.Username] == nil && !newUser[u.Username] {
			up.SetUsername = u.Username
			newUser[u.Username] = true
		}
		plan.Updates = append(plan.Updates, up)
	}

	// Im AD nicht mehr vorhandene (oder deaktivierte) Konten ausblenden – Schlagworte bleiben erhalten
	if cfg.DeactivateMissing {
		for i := range current {
			e := &current[i]
			if e.Source == "ad" && e.Active && !seen[e.ID] && (e.ADGUID != "" || e.Username != "") {
				plan.Deactivate = append(plan.Deactivate, e.ID)
			}
		}
		if adCount > 0 && len(plan.Deactivate)*100 > adCount*cfg.MaxDeactivatePercent {
			res.Message = fmt.Sprintf("Abbruch: %d von %d AD-Einträgen würden deaktiviert (Grenze %d %%). Filter/BaseDN prüfen oder adSync.maxDeactivatePercent erhöhen", len(plan.Deactivate), adCount, cfg.MaxDeactivatePercent)
			res.Created, res.Updated, res.Reactivated = 0, 0, 0
			return nil
		}
		res.Deactivated = len(plan.Deactivate)
	}
	return plan
}

func (a *App) syncLoop(ctx context.Context) {
	iv := a.cfg.Sync.Interval.Duration
	if !a.cfg.Sync.Enabled || iv <= 0 {
		return
	}
	// Erster Lauf kurz nach dem Start
	t := time.NewTimer(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			_, _ = a.RunSync(sctx, "zeitgesteuert")
			cancel()
			t.Reset(iv)
		}
	}
}

func (a *App) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	runs, err := a.st().ListSyncRuns(r.Context(), 15)
	if err != nil {
		dbFail(w, err)
		return
	}
	a.sync.mu.Lock()
	running := a.sync.running
	a.sync.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": a.cfg.Sync.Enabled, "configured": a.ldap != nil && a.cfg.LDAP.BindDN != "", "running": running,
		"interval": a.cfg.Sync.Interval.String(), "fields": a.cfg.Sync.Fields, "deactivateMissing": a.cfg.Sync.DeactivateMissing,
		"runs": runs,
	})
}

func (a *App) handleSyncRun(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	if !a.cfg.Sync.Enabled {
		jsonError(w, http.StatusBadRequest, "Der AD-Abgleich ist in der Konfiguration nicht aktiviert (adSync.enabled)")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	res, err := a.RunSync(ctx, s.Username)
	if res == nil && err != nil {
		jsonError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *App) handleLDAPTest(w http.ResponseWriter, r *http.Request) {
	if a.ldap == nil {
		jsonError(w, http.StatusBadRequest, "LDAP ist nicht konfiguriert")
		return
	}
	start := time.Now()
	if err := a.ldap.Ping(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ms": time.Since(start).Milliseconds()})
}
