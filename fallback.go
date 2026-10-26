package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// Rückfall-Betrieb: Ist im MySQL-Modus die Datenbank beim Start nicht erreichbar, läuft das
// Telefonbuch mit der zuletzt geschriebenen JSON-Kopie weiter – nur lesend. Ein Hintergrund-
// Job verbindet sich regelmäßig neu und schaltet zurück auf MySQL, sobald sie erreichbar ist.

// ErrReadOnly: Änderung im Rückfall-Betrieb nicht möglich
var ErrReadOnly = errors.New("Offline-Betrieb: Die Datenbank ist nicht erreichbar, Änderungen sind derzeit nicht möglich")

type storeBox struct{ Store }

func (a *App) st() Store { return a.storeP.Load().Store }

func (a *App) setStore(s Store) { a.storeP.Store(&storeBox{s}) }

// readOnlyStore lässt nur Lesezugriffe und (flüchtige) Sitzungen durch.
type readOnlyStore struct {
	*jsonStore
}

func (r *readOnlyStore) Close() error { return nil }

// UpsertLocalUser: nur im Speicher (z. B. DEV-Admin), die Datei bleibt unverändert
func (r *readOnlyStore) UpsertLocalUser(_ context.Context, u LocalUser) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[u.Username] = &u
	return nil
}
func (r *readOnlyStore) TouchLocalUser(context.Context, string, time.Time) error { return nil }
func (r *readOnlyStore) CreateEmployee(context.Context, *EmployeeRec) (int64, error) {
	return 0, ErrReadOnly
}
func (r *readOnlyStore) UpdateEmployee(context.Context, *EmployeeRec) error { return ErrReadOnly }
func (r *readOnlyStore) SetAvatar(context.Context, int64, json.RawMessage) error {
	return ErrReadOnly
}
func (r *readOnlyStore) DeleteEmployee(context.Context, int64) error { return ErrReadOnly }
func (r *readOnlyStore) LinkUsernameByGUID(context.Context, string, string) (bool, error) {
	return false, nil
}
func (r *readOnlyStore) SetEmployeeTags(context.Context, int64, []TagInput, string, bool) ([]int64, error) {
	return nil, ErrReadOnly
}
func (r *readOnlyStore) CreateTag(context.Context, string, string, string) (int64, error) {
	return 0, ErrReadOnly
}
func (r *readOnlyStore) UpdateTag(context.Context, int64, string, string) (string, error) {
	return "", ErrReadOnly
}
func (r *readOnlyStore) MergeTag(context.Context, int64, int64) (string, string, error) {
	return "", "", ErrReadOnly
}
func (r *readOnlyStore) DeleteTag(context.Context, int64) (string, error) { return "", ErrReadOnly }
func (r *readOnlyStore) DeleteUnusedTags(context.Context) (int64, error)  { return 0, ErrReadOnly }
func (r *readOnlyStore) CreateCategory(context.Context, Category) error   { return ErrReadOnly }
func (r *readOnlyStore) UpdateCategory(context.Context, Category) error   { return ErrReadOnly }
func (r *readOnlyStore) DeleteCategory(context.Context, string, string) (int, error) {
	return 0, ErrReadOnly
}
func (r *readOnlyStore) CreatePrivate(context.Context, string, PrivateContact) (int64, error) {
	return 0, ErrReadOnly
}
func (r *readOnlyStore) UpdatePrivate(context.Context, string, PrivateContact) error {
	return ErrReadOnly
}
func (r *readOnlyStore) DeletePrivate(context.Context, string, int64) error { return ErrReadOnly }
func (r *readOnlyStore) AddAudit(_ context.Context, e AuditEntry) error {
	log.Printf("Protokoll (offline, nicht gespeichert): %s %s %s %s", e.Actor, e.Action, e.Target, e.Detail)
	return nil
}
func (r *readOnlyStore) StartSyncRun(context.Context, string, time.Time) (int64, error) {
	return 0, ErrReadOnly
}
func (r *readOnlyStore) FinishSyncRun(context.Context, *SyncResult) error { return ErrReadOnly }
func (r *readOnlyStore) ApplySyncPlan(context.Context, *SyncPlan) error   { return ErrReadOnly }
func (r *readOnlyStore) Import(context.Context, *Snapshot) error          { return ErrReadOnly }

// openFallbackStore lädt die JSON-Kopie nur lesend (die Datei wird nicht verändert).
func openFallbackStore(cfg *Config) (*readOnlyStore, error) {
	path := cfg.Storage.JSONPath
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("keine JSON-Kopie unter %s", path)
	}
	snap, _, err := readSnapshotFiles(path, cfg.auditPathFor(path))
	if err != nil {
		return nil, err
	}
	js := &jsonStore{path: path, auditPath: cfg.auditPathFor(path), sessions: map[string]*SessionRec{}}
	js.apply(snap)
	if fi, err := os.Stat(path); err == nil {
		js.savedAt = fi.ModTime()
	}
	return &readOnlyStore{js}, nil
}

// reconnectLoop: im Rückfall-Betrieb regelmäßig MySQL versuchen und zurückschalten.
func (a *App) reconnectLoop(ctx context.Context) {
	if !a.offline.Load() {
		return
	}
	t := time.NewTicker(a.cfg.Storage.FallbackRetry.Duration)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if a.tryReconnect(ctx) {
				return
			}
		}
	}
}

func (a *App) tryReconnect(ctx context.Context) bool {
	ms, err := newMySQLStore(a.cfg, 1)
	if err != nil {
		return false
	}
	old, _ := a.st().(*readOnlyStore)
	// Sitzungen aus dem Rückfall-Betrieb übernehmen, damit niemand abgemeldet wird
	if old != nil {
		old.mu.RLock()
		sess := make([]SessionRec, 0, len(old.sessions))
		for _, s := range old.sessions {
			sess = append(sess, *s)
		}
		old.mu.RUnlock()
		now := time.Now().UTC()
		for i := range sess {
			// MySQL (strict mode) lehnt Nullzeiten ab
			if sess[i].CreatedAt.IsZero() {
				sess[i].CreatedAt = now
			}
			if sess[i].LastSeen.IsZero() {
				sess[i].LastSeen = now
			}
			if sess[i].ExpiresAt.IsZero() {
				continue
			}
			if err := ms.CreateSession(ctx, &sess[i]); err != nil && !isDuplicate(err) {
				log.Printf("Sitzung übernehmen: %v", err)
			}
		}
	}
	a.setStore(ms)
	a.offline.Store(false)
	log.Printf("MySQL wieder erreichbar – Rückfall-Betrieb beendet")
	a.audit(nil, "system", "storage_online", "mysql", "Rückfall auf JSON-Kopie beendet")
	if a.cfg.Auth.DevAdmin && a.devHash != "" {
		_ = ms.UpsertLocalUser(ctx, LocalUser{Username: a.cfg.Auth.DevAdminUser, DisplayName: "DEV-Admin", PwHash: a.devHash, IsAdmin: true, CreatedAt: time.Now().UTC()})
	}
	go a.storageLoop(ctx)
	return true
}

// readOnlyGuard: im Rückfall-Betrieb alle ändernden Anfragen (außer An-/Abmelden) sofort ablehnen
func (a *App) readOnlyGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.offline.Load() && r.Method != http.MethodGet && r.Method != http.MethodHead &&
			r.URL.Path != "/api/login" && r.URL.Path != "/api/logout" {
			jsonError(w, http.StatusServiceUnavailable, ErrReadOnly.Error())
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (r *readOnlyStore) SetDeputies(context.Context, int64, []int64) error   { return ErrReadOnly }
func (r *readOnlyStore) SetAbsences(context.Context, int64, []Absence) error { return ErrReadOnly }
func (r *readOnlyStore) SetOrgUnit(context.Context, []int64, int64) (int, error) {
	return 0, ErrReadOnly
}
func (r *readOnlyStore) CreateOrgUnit(context.Context, OrgUnit) (int64, error) { return 0, ErrReadOnly }
func (r *readOnlyStore) UpdateOrgUnit(context.Context, OrgUnit) error          { return ErrReadOnly }
func (r *readOnlyStore) DeleteOrgUnit(context.Context, int64) error            { return ErrReadOnly }
func (r *readOnlyStore) SetSetting(context.Context, string, string) error      { return ErrReadOnly }
func (r *readOnlyStore) CreateReport(context.Context, Report) (int64, error)   { return 0, ErrReadOnly }
func (r *readOnlyStore) UpdateReport(context.Context, int64, string, string, string) error {
	return ErrReadOnly
}

func (r *readOnlyStore) CreateChannel(context.Context, ContactChannel) (int64, error) {
	return 0, ErrReadOnly
}
func (r *readOnlyStore) UpdateChannel(context.Context, ContactChannel) error { return ErrReadOnly }
func (r *readOnlyStore) DeleteChannel(context.Context, int64) error          { return ErrReadOnly }
func (r *readOnlyStore) CreateTask(context.Context, Task) (int64, error)     { return 0, ErrReadOnly }
func (r *readOnlyStore) UpdateTask(context.Context, Task) error              { return ErrReadOnly }
func (r *readOnlyStore) DeleteTask(context.Context, int64) error             { return ErrReadOnly }
func (r *readOnlyStore) SetChannel(context.Context, []int64, int64) (int, error) {
	return 0, ErrReadOnly
}

// SetPrefs: im Rückfall-Betrieb nur im Speicher (z. B. Favoriten), die Datei bleibt unverändert
func (r *readOnlyStore) SetPrefs(_ context.Context, user, prefs string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefs[user] = prefs
	return nil
}
