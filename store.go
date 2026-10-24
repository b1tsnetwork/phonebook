package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

// Store kapselt die Datenhaltung. Es gibt zwei Umsetzungen: MySQL (store_mysql.go)
// und eine JSON-Datei (store_json.go). Die Geschäftslogik (Rechte, Prüfungen, AD-Abgleich)
// liegt in den Handlern und ist für beide gleich.
type Store interface {
	Kind() string // "mysql" oder "json"
	Ping(ctx context.Context) error
	Close() error

	// Sitzungen
	CreateSession(ctx context.Context, s *SessionRec) error
	GetSession(ctx context.Context, idHash string) (*SessionRec, error) // nil, nil = keine
	TouchSession(ctx context.Context, idHash string, t time.Time) error
	DeleteSession(ctx context.Context, idHash string) error
	DeleteUserSessions(ctx context.Context, username, source string) error
	PurgeSessions(ctx context.Context, expiredBefore, idleBefore time.Time) error

	// Lokale Konten
	GetLocalUser(ctx context.Context, username string) (*LocalUser, error) // ErrNotFound
	UpsertLocalUser(ctx context.Context, u LocalUser) error
	TouchLocalUser(ctx context.Context, username string, t time.Time) error

	// Einträge
	ListEmployees(ctx context.Context, includeInactive bool) ([]EmployeeRec, error)
	GetEmployee(ctx context.Context, id int64) (*EmployeeRec, error)          // ErrNotFound
	EmployeeIDByUsername(ctx context.Context, username string) (int64, error) // 0 = keiner
	CreateEmployee(ctx context.Context, e *EmployeeRec) (int64, error)        // ErrDuplicate (Benutzername)
	UpdateEmployee(ctx context.Context, e *EmployeeRec) error                 // Stammdaten, Benutzername, hidden
	SetAvatar(ctx context.Context, id int64, avatar json.RawMessage) error
	DeleteEmployee(ctx context.Context, id int64) error
	LinkUsernameByGUID(ctx context.Context, guid, username string) (bool, error)
	// Ersetzt die Schlagworte eines Eintrags; unbekannte werden angelegt (wenn allowCreate)
	SetEmployeeTags(ctx context.Context, id int64, tags []TagInput, creator string, allowCreate bool) ([]int64, error)

	// Schlagworte
	ListTags(ctx context.Context) ([]Tag, error) // Count = alle Zuordnungen
	CreateTag(ctx context.Context, name, category, creator string) (int64, error)
	UpdateTag(ctx context.Context, id int64, name, category string) (old string, err error)
	MergeTag(ctx context.Context, from, into int64) (fromName, intoName string, err error)
	DeleteTag(ctx context.Context, id int64) (name string, err error)
	DeleteUnusedTags(ctx context.Context) (int64, error)

	// Schlagwort-Kategorien (Count = Anzahl Schlagworte)
	ListCategories(ctx context.Context) ([]Category, error)
	CreateCategory(ctx context.Context, c Category) error // ErrDuplicate (Schlüssel/Name)
	UpdateCategory(ctx context.Context, c Category) error // ErrNotFound, ErrDuplicate (Name)
	// Löscht eine Kategorie; ihre Schlagworte wandern in moveTo
	DeleteCategory(ctx context.Context, key, moveTo string) (moved int, err error)

	// Private Kontakte (immer mit Besitzer)
	ListPrivate(ctx context.Context, owner string) ([]PrivateContact, error)
	CountPrivate(ctx context.Context, owner string) (int, error)
	CreatePrivate(ctx context.Context, owner string, p PrivateContact) (int64, error)
	UpdatePrivate(ctx context.Context, owner string, p PrivateContact) error // ErrNotFound
	DeletePrivate(ctx context.Context, owner string, id int64) error         // ErrNotFound
	// Mit einem Eintrag (Person) bzw. dessen Amt geteilte Kontakte anderer Besitzer
	ListSharedWith(ctx context.Context, employeeID int64, amt, exceptOwner string) ([]PrivateRec, error)

	// Protokoll & AD-Abgleich
	AddAudit(ctx context.Context, e AuditEntry) error
	ListAudit(ctx context.Context, q string, limit int) ([]AuditEntry, error)
	StartSyncRun(ctx context.Context, trigger string, started time.Time) (int64, error)
	FinishSyncRun(ctx context.Context, r *SyncResult) error
	ListSyncRuns(ctx context.Context, limit int) ([]SyncResult, error)
	ApplySyncPlan(ctx context.Context, p *SyncPlan) error

	// Gesamtabzug für JSON ↔ MySQL
	Export(ctx context.Context) (*Snapshot, error)
	Import(ctx context.Context, s *Snapshot) error // ersetzt ALLE Daten (außer Sitzungen)
}

var (
	ErrNotFound  = errors.New("nicht gefunden")
	ErrDuplicate = errors.New("bereits vorhanden")
)

// ErrUnknownTag: Schlagwort existiert nicht und darf nicht angelegt werden
type ErrUnknownTag struct{ Name string }

func (e ErrUnknownTag) Error() string {
	return fmt.Sprintf("Das Schlagwort „%s“ gibt es noch nicht – neue Schlagworte legen Administratoren an", e.Name)
}

// ---------- Datensätze ----------

type SessionRec struct {
	IDHash      string
	Username    string
	DisplayName string
	IsAdmin     bool
	Source      string
	CSRF        string
	CreatedAt   time.Time
	LastSeen    time.Time
	ExpiresAt   time.Time
	IP          string
	UserAgent   string
}

type LocalUser struct {
	Username    string     `json:"username"`
	DisplayName string     `json:"displayName"`
	PwHash      string     `json:"pwHash"`
	IsAdmin     bool       `json:"isAdmin"`
	Disabled    bool       `json:"disabled"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLogin   *time.Time `json:"lastLogin,omitempty"`
}

type EmployeeRec struct {
	ID           int64           `json:"id"`
	Username     string          `json:"username,omitempty"`
	LastName     string          `json:"lastName"`
	FirstName    string          `json:"firstName"`
	Phone        string          `json:"phone"`
	Mobile       string          `json:"mobile"`
	Room         string          `json:"room"`
	Amt          string          `json:"amt"`
	Email        string          `json:"email"`
	Source       string          `json:"source"`
	ADGUID       string          `json:"adGuid,omitempty"`
	ADDN         string          `json:"adDn,omitempty"`
	Active       bool            `json:"active"`
	Hidden       bool            `json:"hidden"`
	Avatar       json.RawMessage `json:"avatar,omitempty"`
	Tags         []int64         `json:"tags"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
	LastSyncedAt *time.Time      `json:"lastSyncedAt,omitempty"`
}

func (e *EmployeeRec) fields() contactFields {
	return contactFields{e.LastName, e.FirstName, e.Phone, e.Mobile, e.Room, e.Amt, e.Email}
}

func (e *EmployeeRec) setFields(f contactFields) {
	e.LastName, e.FirstName, e.Phone, e.Mobile, e.Room, e.Amt, e.Email = f.LastName, f.FirstName, f.Phone, f.Mobile, f.Room, f.Amt, f.Email
}

type TagRec struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Category  string    `json:"category"`
	CreatedBy string    `json:"createdBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Category: frei verwaltbare Schlagwort-Kategorie
type Category struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Sort  int    `json:"sort"`
	Count int    `json:"count"`
}

var defaultCategories = []Category{
	{Key: "taetigkeit", Name: "Tätigkeit", Color: "#3b7dd8", Sort: 10},
	{Key: "besonderheit", Name: "Besonderheit", Color: "#d48a14", Sort: 20},
	{Key: "sonstiges", Name: "Sonstiges", Color: "#7c8591", Sort: 30},
}

// Shares: Freigabe eines privaten Kontakts an Einträge (Personen) und/oder ganze Ämter
type Shares struct {
	Employees []int64  `json:"employees"`
	Amts      []string `json:"amts"`
}

func (s *Shares) empty() bool { return s == nil || (len(s.Employees) == 0 && len(s.Amts) == 0) }

// sharedWith: gilt die Freigabe für diesen Eintrag bzw. dieses Amt?
func (s *Shares) sharedWith(empID int64, amt string) bool {
	if s == nil {
		return false
	}
	for _, e := range s.Employees {
		if empID != 0 && e == empID {
			return true
		}
	}
	for _, a := range s.Amts {
		if amt != "" && strings.EqualFold(a, amt) {
			return true
		}
	}
	return false
}

type PrivateRec struct {
	PrivateContact
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AuditEntry struct {
	TS     time.Time `json:"ts"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target"`
	Detail string    `json:"detail"`
	IP     string    `json:"ip"`
}

// SyncPlan: berechnet in adsync.go, umgesetzt atomar vom Store
type SyncPlan struct {
	Now        time.Time
	Creates    []EmployeeRec
	Updates    []SyncUpdate
	Deactivate []int64
}

type SyncUpdate struct {
	ID          int64
	Fields      contactFields
	GUID        string
	DN          string
	SetUsername string // nur wenn bisher leer
	Changed     bool
}

// Snapshot ist das Format der JSON-Datenhaltung und des Exports.
type Snapshot struct {
	Format     string        `json:"format"` // "telefonbuch"
	Version    int           `json:"version"`
	ExportedAt time.Time     `json:"exportedAt"`
	Source     string        `json:"source,omitempty"` // woher die Daten stammen (z. B. "mysql")
	ImportedAt *time.Time    `json:"importedFromMySQLAt,omitempty"`
	Employees  []EmployeeRec `json:"employees"`
	Categories []Category    `json:"categories,omitempty"`
	Tags       []TagRec      `json:"tags"`
	Private    []PrivateRec  `json:"privateContacts"`
	LocalUsers []LocalUser   `json:"localUsers"`
	Audit      []AuditEntry  `json:"audit,omitempty"` // im JSON-Betrieb in eigener Datei
	SyncRuns   []SyncResult  `json:"syncRuns"`
}

const snapshotFormat = "telefonbuch"

var reCatKey = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

func (s *Snapshot) validate() error {
	if s.Format != snapshotFormat || s.Version != 1 {
		return errors.New("keine Telefonbuch-Datei (format/version passt nicht)")
	}
	if len(s.Categories) == 0 { // ältere Dateien: Standardkategorien
		s.Categories = append([]Category{}, defaultCategories...)
	}
	cats := map[string]bool{}
	for i, c := range s.Categories {
		if !reCatKey.MatchString(c.Key) || cats[c.Key] {
			return fmt.Errorf("Kategorie %q ungültig/doppelt", c.Key)
		}
		cats[c.Key] = true
		if strings.TrimSpace(c.Name) == "" {
			s.Categories[i].Name = c.Key
		}
		if !reColor.MatchString(c.Color) {
			s.Categories[i].Color = "#7c8591"
		}
		s.Categories[i].Count = 0
	}
	fallback := s.Categories[0].Key
	if cats["sonstiges"] {
		fallback = "sonstiges"
	}
	for i := range s.Tags {
		if !cats[s.Tags[i].Category] {
			s.Tags[i].Category = fallback
		}
	}
	tagIDs := map[int64]bool{}
	for _, t := range s.Tags {
		if t.ID <= 0 || tagIDs[t.ID] {
			return fmt.Errorf("Schlagwort-ID %d ungültig/doppelt", t.ID)
		}
		tagIDs[t.ID] = true
	}
	empIDs := map[int64]bool{}
	users := map[string]bool{}
	for i, e := range s.Employees {
		if e.ID <= 0 || empIDs[e.ID] {
			return fmt.Errorf("Eintrags-ID %d ungültig/doppelt", e.ID)
		}
		empIDs[e.ID] = true
		if e.Username != "" {
			if users[e.Username] {
				return fmt.Errorf("Benutzername %q mehrfach vergeben", e.Username)
			}
			users[e.Username] = true
		}
		for _, t := range e.Tags {
			if !tagIDs[t] {
				return fmt.Errorf("Eintrag %d verweist auf unbekanntes Schlagwort %d", e.ID, t)
			}
		}
		if len(e.Avatar) > 0 && string(e.Avatar) != "null" {
			if out, err := validateAvatar(e.Avatar); err != nil {
				log.Printf("Eintrag %d: ungültiger Avatar wird verworfen (%v)", e.ID, err)
				s.Employees[i].Avatar = nil
			} else if len(out) < len(e.Avatar) {
				s.Employees[i].Avatar = json.RawMessage(out) // kompakt (einfarbige Seiten)
			}
		}
	}
	privIDs := map[int64]bool{}
	for i, p := range s.Private {
		if p.ID <= 0 || privIDs[p.ID] || p.Owner == "" {
			return fmt.Errorf("privater Kontakt %d ungültig", p.ID)
		}
		privIDs[p.ID] = true
		if sh := p.Shares; sh != nil { // Freigaben an gelöschte Einträge verwerfen
			keep := []int64{}
			for _, id := range sh.Employees {
				if empIDs[id] {
					keep = append(keep, id)
				}
			}
			sh.Employees = keep
			if sh.Amts == nil {
				sh.Amts = []string{}
			}
			s.Private[i].Shares = sh.clone()
		}
	}
	return nil
}

// copyStore überträgt alle Daten von src nach dst (dst wird ersetzt).
func copyStore(ctx context.Context, src, dst Store) (*Snapshot, error) {
	snap, err := src.Export(ctx)
	if err != nil {
		return nil, fmt.Errorf("Lesen aus %s: %w", src.Kind(), err)
	}
	snap.Source = src.Kind()
	if err := snap.validate(); err != nil {
		return nil, err
	}
	if err := dst.Import(ctx, snap); err != nil {
		return nil, fmt.Errorf("Schreiben nach %s: %w", dst.Kind(), err)
	}
	return snap, nil
}

// clone liefert eine tiefe Kopie (nil bei leerer Freigabe).
func (s *Shares) clone() *Shares {
	if s.empty() {
		return nil
	}
	return &Shares{Employees: append([]int64{}, s.Employees...), Amts: append([]string{}, s.Amts...)}
}
