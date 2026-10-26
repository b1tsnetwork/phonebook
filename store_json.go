package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	jsonAuditMax    = 5000 // ältere Protokolleinträge fallen aus der Datei
	jsonSyncRunsMax = 50
)

// jsonStore hält alle Daten im Speicher und schreibt sie bei jeder Änderung atomar
// (temporäre Datei + fsync + rename) in eine JSON-Datei. Sitzungen bleiben nur im Speicher.
type jsonStore struct {
	path      string
	auditPath string // Protokoll in eigener Datei
	mu        sync.RWMutex

	emp        map[int64]*EmployeeRec
	tags       map[int64]*TagRec
	priv       map[int64]*PrivateRec
	cats       map[string]*Category
	units      map[int64]*OrgUnit
	settings   map[string]string
	reports    []Report // älteste zuerst
	nextUnit   int64
	nextReport int64
	channels   map[int64]*ContactChannel
	nextChan   int64
	tasks      map[int64]*Task
	nextTask   int64
	prefs      map[string]string
	users      map[string]*LocalUser
	audit      []AuditEntry // älteste zuerst
	syncRuns   []SyncResult // älteste zuerst
	sessions   map[string]*SessionRec
	nextEmp    int64
	nextTag    int64
	nextPriv   int64
	nextRun    int64
	source     string
	importedAt *time.Time
	savedAt    time.Time
}

// auditPathFor leitet den Pfad der Protokolldatei ab: daten.json → daten.audit.json
func auditPathFor(path string) string {
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		return path[:len(path)-5] + ".audit.json"
	}
	return path + ".audit.json"
}

func newJSONStore(path, auditPath string) (*jsonStore, error) {
	if path == "" {
		return nil, errors.New("storage.jsonPath fehlt")
	}
	if auditPath == "" {
		auditPath = auditPathFor(path)
	}
	if auditPath == path {
		return nil, errors.New("storage.jsonAuditPath darf nicht gleich storage.jsonPath sein")
	}
	s := &jsonStore{path: path, auditPath: auditPath, sessions: map[string]*SessionRec{}}
	for _, p := range []string{path, auditPath} {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return nil, err
		}
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Snapshot) normalize() {
	if s.Employees == nil {
		s.Employees = []EmployeeRec{}
	}
	for i := range s.Employees {
		if s.Employees[i].Tags == nil {
			s.Employees[i].Tags = []int64{}
		}
	}
	if s.Tags == nil {
		s.Tags = []TagRec{}
	}
	if s.Private == nil {
		s.Private = []PrivateRec{}
	}
	if s.LocalUsers == nil {
		s.LocalUsers = []LocalUser{}
	}
	if s.Audit == nil {
		s.Audit = []AuditEntry{}
	}
	if s.SyncRuns == nil {
		s.SyncRuns = []SyncResult{}
	}
}

// Format der separaten Protokolldatei
type auditFile struct {
	Format  string       `json:"format"` // "telefonbuch-audit"
	Version int          `json:"version"`
	Entries []AuditEntry `json:"entries"` // neueste zuerst
}

const auditFormat = "telefonbuch-audit"

// readSnapshotFiles liest Datendatei + Protokolldatei. legacyAudit = true, wenn das Protokoll
// noch (altes Format) in der Datendatei stand und keine eigene Protokolldatei existiert.
func readSnapshotFiles(path, auditPath string) (snap *Snapshot, legacyAudit bool, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	snap = &Snapshot{}
	if err := json.Unmarshal(b, snap); err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	snap.normalize()
	if err := snap.validate(); err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	ab, err := os.ReadFile(auditPath)
	switch {
	case err == nil:
		var af auditFile
		if err := json.Unmarshal(ab, &af); err != nil {
			return nil, false, fmt.Errorf("%s: %w", auditPath, err)
		}
		if af.Format != auditFormat {
			return nil, false, fmt.Errorf("%s: keine Telefonbuch-Protokolldatei", auditPath)
		}
		snap.Audit = af.Entries
		if snap.Audit == nil {
			snap.Audit = []AuditEntry{}
		}
	case errors.Is(err, os.ErrNotExist):
		legacyAudit = len(snap.Audit) > 0
	default:
		return nil, false, err
	}
	return snap, legacyAudit, nil
}

// readSnapshotFile: Datendatei + daneben liegende Protokolldatei (Standardname)
func readSnapshotFile(path string) (*Snapshot, error) {
	snap, _, err := readSnapshotFiles(path, auditPathFor(path))
	return snap, err
}

func (s *jsonStore) load() error {
	snap, legacy, err := readSnapshotFiles(s.path, s.auditPath)
	if errors.Is(err, os.ErrNotExist) {
		snap = &Snapshot{Format: snapshotFormat, Version: 1}
		snap.normalize()
		_ = snap.validate() // Standardkategorien
		s.apply(snap)
		// gleich anlegen, damit Rechte-/Pfadfehler sofort auffallen
		if err := s.persistAudit(); err != nil {
			return err
		}
		return s.persist()
	}
	if err != nil {
		return err
	}
	for _, p := range []string{s.path, s.auditPath} {
		if fi, err := os.Stat(p); err == nil && fi.Mode().Perm()&0o077 != 0 {
			_ = os.Chmod(p, 0o600)
		}
	}
	s.apply(snap)
	if _, err := os.Stat(s.auditPath); errors.Is(err, os.ErrNotExist) || legacy {
		// Umstellung: Protokoll aus der Datendatei in die eigene Datei verschieben
		if err := s.persistAudit(); err != nil {
			return err
		}
		if legacy {
			log.Printf("Protokoll (%d Einträge) aus %s nach %s verschoben", len(s.audit), s.path, s.auditPath)
			return s.persist()
		}
	}
	return nil
}

// apply ersetzt den Speicherinhalt durch einen Snapshot (Sitzungen bleiben erhalten).
func (s *jsonStore) apply(snap *Snapshot) {
	s.emp, s.tags, s.priv, s.users = map[int64]*EmployeeRec{}, map[int64]*TagRec{}, map[int64]*PrivateRec{}, map[string]*LocalUser{}
	s.nextEmp, s.nextTag, s.nextPriv, s.nextRun = 1, 1, 1, 1
	for i := range snap.Employees {
		e := snap.Employees[i]
		s.emp[e.ID] = &e
		s.nextEmp = max(s.nextEmp, e.ID+1)
	}
	for i := range snap.Tags {
		t := snap.Tags[i]
		s.tags[t.ID] = &t
		s.nextTag = max(s.nextTag, t.ID+1)
	}
	for i := range snap.Private {
		p := snap.Private[i]
		s.priv[p.ID] = &p
		s.nextPriv = max(s.nextPriv, p.ID+1)
	}
	s.units, s.settings, s.reports = map[int64]*OrgUnit{}, map[string]string{}, nil
	s.nextUnit, s.nextReport = 1, 1
	s.channels, s.nextChan = map[int64]*ContactChannel{}, 1
	s.tasks, s.nextTask = map[int64]*Task{}, 1
	for i := range snap.Tasks {
		t := snap.Tasks[i].clone()
		s.tasks[t.ID] = &t
		s.nextTask = max(s.nextTask, t.ID+1)
	}
	s.prefs = map[string]string{}
	for k, v := range snap.Prefs {
		s.prefs[k] = v
	}
	for i := range snap.Channels {
		c := snap.Channels[i]
		s.channels[c.ID] = &c
		s.nextChan = max(s.nextChan, c.ID+1)
	}
	for i := range snap.OrgUnits {
		u := copyUnit(&snap.OrgUnits[i])
		s.units[u.ID] = &u
		s.nextUnit = max(s.nextUnit, u.ID+1)
	}
	for k, v := range snap.Settings {
		s.settings[k] = v
	}
	for _, r := range snap.Reports {
		s.reports = append(s.reports, r)
		s.nextReport = max(s.nextReport, r.ID+1)
	}
	sort.Slice(s.reports, func(i, j int) bool { return s.reports[i].ID < s.reports[j].ID })
	s.cats = map[string]*Category{}
	for i := range snap.Categories {
		c := snap.Categories[i]
		c.Count = 0
		s.cats[c.Key] = &c
	}
	for i := range snap.LocalUsers {
		u := snap.LocalUsers[i]
		s.users[u.Username] = &u
	}
	// In der Datei: neueste zuerst (wie in der Oberfläche); intern: älteste zuerst
	s.audit = make([]AuditEntry, 0, len(snap.Audit))
	for i := len(snap.Audit) - 1; i >= 0; i-- {
		s.audit = append(s.audit, snap.Audit[i])
	}
	s.syncRuns = make([]SyncResult, 0, len(snap.SyncRuns))
	for i := len(snap.SyncRuns) - 1; i >= 0; i-- {
		s.syncRuns = append(s.syncRuns, snap.SyncRuns[i])
		s.nextRun = max(s.nextRun, snap.SyncRuns[i].ID+1)
	}
	s.source = snap.Source
	s.importedAt = snap.ImportedAt
}

// snapshot liefert den Datenbestand; das Protokoll nur mit withAudit (Export).
func (s *jsonStore) snapshot(withAudit bool) *Snapshot {
	snap := &Snapshot{Format: snapshotFormat, Version: 1, ExportedAt: time.Now().UTC(), Source: s.source, ImportedAt: s.importedAt}
	for _, e := range s.emp {
		snap.Employees = append(snap.Employees, copyEmp(e))
	}
	for _, u := range s.units {
		snap.OrgUnits = append(snap.OrgUnits, copyUnit(u))
	}
	sort.Slice(snap.OrgUnits, func(i, j int) bool { return snap.OrgUnits[i].ID < snap.OrgUnits[j].ID })
	if len(s.settings) > 0 {
		snap.Settings = map[string]string{}
		for k, v := range s.settings {
			snap.Settings[k] = v
		}
	}
	snap.Reports = append([]Report(nil), s.reports...)
	for _, c := range s.channels {
		snap.Channels = append(snap.Channels, *c)
	}
	sort.Slice(snap.Channels, func(i, j int) bool { return snap.Channels[i].ID < snap.Channels[j].ID })
	for _, t := range s.tasks {
		snap.Tasks = append(snap.Tasks, t.clone())
	}
	sort.Slice(snap.Tasks, func(i, j int) bool { return snap.Tasks[i].ID < snap.Tasks[j].ID })
	if len(s.prefs) > 0 {
		snap.Prefs = map[string]string{}
		for k, v := range s.prefs {
			snap.Prefs[k] = v
		}
	}
	sort.Slice(snap.Employees, func(i, j int) bool { return snap.Employees[i].ID < snap.Employees[j].ID })
	for _, t := range s.tags {
		snap.Tags = append(snap.Tags, *t)
	}
	sort.Slice(snap.Tags, func(i, j int) bool { return snap.Tags[i].ID < snap.Tags[j].ID })
	for _, p := range s.priv {
		c := *p
		c.Shares = p.Shares.clone()
		snap.Private = append(snap.Private, c)
	}
	snap.Categories = s.categoryList(false)
	sort.Slice(snap.Private, func(i, j int) bool { return snap.Private[i].ID < snap.Private[j].ID })
	for _, u := range s.users {
		snap.LocalUsers = append(snap.LocalUsers, *u)
	}
	sort.Slice(snap.LocalUsers, func(i, j int) bool { return snap.LocalUsers[i].Username < snap.LocalUsers[j].Username })
	if withAudit {
		snap.Audit = s.auditNewestFirst()
	}
	for i := len(s.syncRuns) - 1; i >= 0; i-- {
		snap.SyncRuns = append(snap.SyncRuns, s.syncRuns[i])
	}
	snap.normalize()
	if !withAudit {
		snap.Audit = nil // eigene Datei
	}
	return snap
}

func (s *jsonStore) auditNewestFirst() []AuditEntry {
	out := make([]AuditEntry, 0, len(s.audit))
	for i := len(s.audit) - 1; i >= 0; i-- {
		out = append(out, s.audit[i])
	}
	return out
}

// writeSnapshotFiles schreibt Daten- und Protokolldatei (jeweils atomar).
func writeSnapshotFiles(path, auditPath string, snap *Snapshot) error {
	entries := snap.Audit
	if entries == nil {
		entries = []AuditEntry{}
	}
	if err := writeJSONAtomic(auditPath, auditFile{Format: auditFormat, Version: 1, Entries: entries}); err != nil {
		return err
	}
	main := *snap
	main.Audit = nil
	return writeJSONAtomic(path, &main)
}

// writeJSONAtomic schreibt atomar: erst komplett in eine temporäre Datei, dann umbenennen.
func writeJSONAtomic(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".telefonbuch-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

func (s *jsonStore) persist() error {
	if err := writeJSONAtomic(s.path, s.snapshot(false)); err != nil {
		return fmt.Errorf("JSON-Datei schreiben: %w", err)
	}
	s.savedAt = time.Now()
	return nil
}

func (s *jsonStore) persistAudit() error {
	if err := writeJSONAtomic(s.auditPath, auditFile{Format: auditFormat, Version: 1, Entries: s.auditNewestFirst()}); err != nil {
		return fmt.Errorf("Protokolldatei schreiben: %w", err)
	}
	return nil
}

// commit speichert; schlägt das fehl, wird der letzte Stand von der Platte zurückgeholt,
// damit Speicher und Datei nicht auseinanderlaufen.
func (s *jsonStore) commit() error {
	if err := s.persist(); err != nil {
		if snap, _, rerr := readSnapshotFiles(s.path, s.auditPath); rerr == nil {
			s.apply(snap)
		}
		return err
	}
	return nil
}

func (s *jsonStore) Kind() string { return "json" }
func (s *jsonStore) Ping(ctx context.Context) error {
	_, err := os.Stat(s.path)
	return err
}
func (s *jsonStore) Close() error { return nil }

// ---------- Sitzungen (nur im Speicher) ----------

func (s *jsonStore) CreateSession(ctx context.Context, r *SessionRec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *r
	s.sessions[r.IDHash] = &c
	return nil
}

func (s *jsonStore) GetSession(ctx context.Context, h string) (*SessionRec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r := s.sessions[h]; r != nil {
		c := *r
		return &c, nil
	}
	return nil, nil
}

func (s *jsonStore) TouchSession(ctx context.Context, h string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.sessions[h]; r != nil {
		r.LastSeen = t
	}
	return nil
}

func (s *jsonStore) DeleteSession(ctx context.Context, h string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, h)
	return nil
}

func (s *jsonStore) DeleteUserSessions(ctx context.Context, user, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, r := range s.sessions {
		if r.Username == user && r.Source == source {
			delete(s.sessions, k)
		}
	}
	return nil
}

func (s *jsonStore) PurgeSessions(ctx context.Context, expired, idle time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, r := range s.sessions {
		if r.ExpiresAt.Before(expired) || r.LastSeen.Before(idle) {
			delete(s.sessions, k)
		}
	}
	return nil
}

// ---------- Lokale Konten ----------

func (s *jsonStore) GetLocalUser(ctx context.Context, user string) (*LocalUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if u := s.users[user]; u != nil {
		c := *u
		return &c, nil
	}
	return nil, ErrNotFound
}

func (s *jsonStore) UpsertLocalUser(ctx context.Context, u LocalUser) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.users[u.Username]; old != nil {
		u.CreatedAt, u.LastLogin = old.CreatedAt, old.LastLogin
	}
	s.users[u.Username] = &u
	return s.commit()
}

func (s *jsonStore) TouchLocalUser(ctx context.Context, user string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.users[user]; u != nil {
		u.LastLogin = &t
		return s.commit()
	}
	return nil
}

// ---------- Einträge ----------

func copyEmp(e *EmployeeRec) EmployeeRec {
	c := *e
	c.Tags = append([]int64{}, e.Tags...)
	c.Deputies = copyIDs(e.Deputies)
	c.Absences = copyAbsences(e.Absences)
	return c
}

func copyIDs(l []int64) []int64 {
	if len(l) == 0 {
		return nil
	}
	return append([]int64{}, l...)
}

func copyAbsences(l []Absence) []Absence {
	if len(l) == 0 {
		return nil
	}
	out := make([]Absence, len(l))
	for i, a := range l {
		a.Deputies = copyIDs(a.Deputies)
		out[i] = a
	}
	return out
}

func copyUnit(u *OrgUnit) OrgUnit {
	c := *u
	c.Leaders, c.Deputies = copyIDs(u.Leaders), copyIDs(u.Deputies)
	c.Contact = u.Contact.clone()
	c.Hours = u.Hours.clone()
	return c
}

func (s *jsonStore) ListEmployees(ctx context.Context, all bool) ([]EmployeeRec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EmployeeRec, 0, len(s.emp))
	for _, e := range s.emp {
		if all || (e.Active && !e.Hidden) {
			out = append(out, copyEmp(e))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *jsonStore) GetEmployee(ctx context.Context, id int64) (*EmployeeRec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e := s.emp[id]; e != nil {
		c := copyEmp(e)
		return &c, nil
	}
	return nil, ErrNotFound
}

func (s *jsonStore) userTaken(user string, except int64) bool {
	if user == "" {
		return false
	}
	for _, e := range s.emp {
		if e.ID != except && e.Username == user {
			return true
		}
	}
	return false
}

func (s *jsonStore) EmployeeIDByUsername(ctx context.Context, user string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.emp {
		if e.Username == user {
			return e.ID, nil
		}
	}
	return 0, nil
}

func (s *jsonStore) CreateEmployee(ctx context.Context, e *EmployeeRec) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userTaken(e.Username, 0) {
		return 0, ErrDuplicate
	}
	if e.ADGUID != "" {
		for _, x := range s.emp {
			if x.ADGUID == e.ADGUID {
				return 0, ErrDuplicate
			}
		}
	}
	c := copyEmp(e)
	c.ID = s.nextEmp
	s.nextEmp++
	s.emp[c.ID] = &c
	if err := s.commit(); err != nil {
		return 0, err
	}
	return c.ID, nil
}

func (s *jsonStore) UpdateEmployee(ctx context.Context, e *EmployeeRec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.emp[e.ID]
	if cur == nil {
		return ErrNotFound
	}
	if s.userTaken(e.Username, e.ID) {
		return ErrDuplicate
	}
	cur.Username = e.Username
	cur.setFields(e.fields())
	cur.Hidden = e.Hidden
	cur.UpdatedAt = e.UpdatedAt
	return s.commit()
}

func (s *jsonStore) SetAvatar(ctx context.Context, id int64, av json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.emp[id]
	if cur == nil {
		return ErrNotFound
	}
	if len(av) == 0 || string(av) == "null" {
		cur.Avatar = nil
	} else {
		cur.Avatar = append(json.RawMessage{}, av...)
	}
	cur.UpdatedAt = time.Now().UTC()
	return s.commit()
}

func (s *jsonStore) DeleteEmployee(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.emp[id] == nil {
		return ErrNotFound
	}
	delete(s.emp, id)
	for _, e := range s.emp {
		e.Deputies, _ = removeID(e.Deputies, id)
		for i := range e.Absences {
			e.Absences[i].Deputies, _ = removeID(e.Absences[i].Deputies, id)
		}
	}
	for _, u := range s.units {
		u.Leaders, _ = removeID(u.Leaders, id)
		u.Deputies, _ = removeID(u.Deputies, id)
	}
	for _, t := range s.tasks {
		t.removeEmp(id)
	}
	for i := range s.reports {
		if s.reports[i].EmployeeID == id {
			s.reports[i].EmployeeID = 0
		}
	}
	for _, p := range s.priv {
		if p.Shares != nil {
			p.Shares.Employees = slicesDeleteFunc(p.Shares.Employees, func(x int64) bool { return x == id })
			if p.Shares.empty() {
				p.Shares = nil
			}
		}
	}
	return s.commit()
}

func slicesDeleteFunc(in []int64, del func(int64) bool) []int64 {
	out := in[:0]
	for _, x := range in {
		if !del(x) {
			out = append(out, x)
		}
	}
	return out
}

func (s *jsonStore) LinkUsernameByGUID(ctx context.Context, guid, user string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userTaken(user, 0) {
		return false, nil
	}
	for _, e := range s.emp {
		if e.ADGUID == guid && e.Username == "" {
			e.Username = user
			return true, s.commit()
		}
	}
	return false, nil
}

func (s *jsonStore) tagByName(name string) *TagRec {
	var best *TagRec
	for _, t := range s.tags {
		if strings.EqualFold(t.Name, name) && (best == nil || t.ID < best.ID) {
			best = t
		}
	}
	return best
}

func (s *jsonStore) SetEmployeeTags(ctx context.Context, id int64, tags []TagInput, creator string, allowCreate bool) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.emp[id]
	if e == nil {
		return nil, ErrNotFound
	}
	ids := []int64{}
	seen := map[int64]bool{}
	var created []int64
	for _, in := range tags {
		t := s.tagByName(in.Name)
		if t == nil {
			if !allowCreate {
				for _, c := range created {
					delete(s.tags, c)
				}
				return nil, ErrUnknownTag{in.Name}
			}
			t = &TagRec{ID: s.nextTag, Name: in.Name, Category: in.Category, CreatedBy: creator, CreatedAt: time.Now().UTC()}
			s.nextTag++
			s.tags[t.ID] = t
			created = append(created, t.ID)
		}
		if !seen[t.ID] {
			seen[t.ID] = true
			ids = append(ids, t.ID)
		}
	}
	e.Tags = append([]int64{}, ids...)
	sort.Slice(e.Tags, func(i, j int) bool { return e.Tags[i] < e.Tags[j] })
	return ids, s.commit()
}

// ---------- Schlagworte ----------

func (s *jsonStore) ListTags(ctx context.Context) ([]Tag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cnt := map[int64]int{}
	for _, e := range s.emp {
		for _, t := range e.Tags {
			cnt[t]++
		}
	}
	out := make([]Tag, 0, len(s.tags))
	for _, t := range s.tags {
		out = append(out, Tag{ID: t.ID, Name: t.Name, Category: t.Category, CreatedBy: t.CreatedBy, Count: cnt[t.ID]})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

func (s *jsonStore) CreateTag(ctx context.Context, name, cat, creator string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tagByName(name) != nil {
		return 0, ErrDuplicate
	}
	t := &TagRec{ID: s.nextTag, Name: name, Category: cat, CreatedBy: creator, CreatedAt: time.Now().UTC()}
	s.nextTag++
	s.tags[t.ID] = t
	return t.ID, s.commit()
}

func (s *jsonStore) UpdateTag(ctx context.Context, id int64, name, cat string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tags[id]
	if t == nil {
		return "", ErrNotFound
	}
	if o := s.tagByName(name); o != nil && o.ID != id {
		return "", ErrDuplicate
	}
	old := t.Name
	t.Name, t.Category = name, cat
	return old, s.commit()
}

func (s *jsonStore) removeTagFromAll(id int64, replaceWith int64) {
	for _, e := range s.emp {
		out := e.Tags[:0]
		has := false
		for _, t := range e.Tags {
			if t == replaceWith {
				has = true
			}
		}
		hit := false
		for _, t := range e.Tags {
			if t == id {
				hit = true
				continue
			}
			out = append(out, t)
		}
		if hit && replaceWith != 0 && !has {
			out = append(out, replaceWith)
			sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		}
		e.Tags = out
	}
}

func (s *jsonStore) MergeTag(ctx context.Context, from, into int64) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, t := s.tags[from], s.tags[into]
	if f == nil || t == nil {
		return "", "", ErrNotFound
	}
	s.removeTagFromAll(from, into)
	delete(s.tags, from)
	return f.Name, t.Name, s.commit()
}

func (s *jsonStore) DeleteTag(ctx context.Context, id int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tags[id]
	if t == nil {
		return "", ErrNotFound
	}
	s.removeTagFromAll(id, 0)
	delete(s.tags, id)
	return t.Name, s.commit()
}

func (s *jsonStore) DeleteUnusedTags(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	used := map[int64]bool{}
	for _, e := range s.emp {
		for _, t := range e.Tags {
			used[t] = true
		}
	}
	var n int64
	for id := range s.tags {
		if !used[id] {
			delete(s.tags, id)
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	return n, s.commit()
}

func (s *jsonStore) ListSharedWith(ctx context.Context, empID int64, amt, except string) ([]PrivateRec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []PrivateRec{}
	for _, p := range s.priv {
		if p.Owner != except && p.Shares.sharedWith(empID, amt) {
			c := *p
			c.Shares = nil
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ---------- Kategorien ----------

func (s *jsonStore) categoryList(withCount bool) []Category {
	cnt := map[string]int{}
	if withCount {
		for _, t := range s.tags {
			cnt[t.Category]++
		}
	}
	out := make([]Category, 0, len(s.cats))
	for _, c := range s.cats {
		x := *c
		x.Count = cnt[c.Key]
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sort != out[j].Sort {
			return out[i].Sort < out[j].Sort
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (s *jsonStore) ListCategories(ctx context.Context) ([]Category, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.categoryList(true), nil
}

func (s *jsonStore) catNameTaken(name, except string) bool {
	for _, c := range s.cats {
		if c.Key != except && strings.EqualFold(c.Name, name) {
			return true
		}
	}
	return false
}

func (s *jsonStore) CreateCategory(ctx context.Context, c Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cats[c.Key] != nil || s.catNameTaken(c.Name, "") {
		return ErrDuplicate
	}
	c.Count = 0
	s.cats[c.Key] = &c
	return s.commit()
}

func (s *jsonStore) UpdateCategory(ctx context.Context, c Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.cats[c.Key]
	if cur == nil {
		return ErrNotFound
	}
	if s.catNameTaken(c.Name, c.Key) {
		return ErrDuplicate
	}
	cur.Name, cur.Color, cur.Sort = c.Name, c.Color, c.Sort
	return s.commit()
}

func (s *jsonStore) DeleteCategory(ctx context.Context, key, moveTo string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cats[key] == nil || s.cats[moveTo] == nil {
		return 0, ErrNotFound
	}
	n := 0
	for _, t := range s.tags {
		if t.Category == key {
			t.Category = moveTo
			n++
		}
	}
	delete(s.cats, key)
	return n, s.commit()
}

// ---------- Private Kontakte ----------

func (s *jsonStore) ListPrivate(ctx context.Context, owner string) ([]PrivateContact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []PrivateContact{}
	for _, p := range s.priv {
		if p.Owner == owner {
			c := p.PrivateContact
			c.Shares = p.Shares.clone()
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].LastName+" "+out[i].FirstName), strings.ToLower(out[j].LastName+" "+out[j].FirstName)
		return a < b
	})
	return out, nil
}

func (s *jsonStore) CountPrivate(ctx context.Context, owner string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, p := range s.priv {
		if p.Owner == owner {
			n++
		}
	}
	return n, nil
}

func (s *jsonStore) CreatePrivate(ctx context.Context, owner string, p PrivateContact) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	p.ID = s.nextPriv
	s.nextPriv++
	p.Shares = p.Shares.clone()
	s.priv[p.ID] = &PrivateRec{PrivateContact: p, Owner: owner, CreatedAt: now, UpdatedAt: now}
	return p.ID, s.commit()
}

func (s *jsonStore) UpdatePrivate(ctx context.Context, owner string, p PrivateContact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.priv[p.ID]
	if cur == nil || cur.Owner != owner {
		return ErrNotFound
	}
	p.Shares = p.Shares.clone()
	cur.PrivateContact = p
	cur.UpdatedAt = time.Now().UTC()
	return s.commit()
}

func (s *jsonStore) DeletePrivate(ctx context.Context, owner string, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.priv[id]
	if cur == nil || cur.Owner != owner {
		return ErrNotFound
	}
	delete(s.priv, id)
	return s.commit()
}

// ---------- Protokoll & Sync ----------

func (s *jsonStore) AddAudit(ctx context.Context, e AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit = append(s.audit, e)
	if len(s.audit) > jsonAuditMax {
		s.audit = append([]AuditEntry{}, s.audit[len(s.audit)-jsonAuditMax:]...)
	}
	return s.persistAudit() // nur die Protokolldatei schreiben
}

func (s *jsonStore) ListAudit(ctx context.Context, q string, limit int) ([]AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q = strings.ToLower(q)
	out := []AuditEntry{}
	for i := len(s.audit) - 1; i >= 0 && (limit <= 0 || len(out) < limit); i-- {
		e := s.audit[i]
		if q != "" && !strings.Contains(strings.ToLower(e.Actor+"\x00"+e.Action+"\x00"+e.Target+"\x00"+e.Detail), q) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *jsonStore) StartSyncRun(ctx context.Context, trigger string, started time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := SyncResult{ID: s.nextRun, StartedAt: started, TriggerBy: trigger, Status: "running"}
	s.nextRun++
	s.syncRuns = append(s.syncRuns, r)
	if len(s.syncRuns) > jsonSyncRunsMax {
		s.syncRuns = append([]SyncResult{}, s.syncRuns[len(s.syncRuns)-jsonSyncRunsMax:]...)
	}
	return r.ID, s.commit()
}

func (s *jsonStore) FinishSyncRun(ctx context.Context, r *SyncResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.syncRuns {
		if s.syncRuns[i].ID == r.ID {
			s.syncRuns[i] = *r
		}
	}
	return s.commit()
}

func (s *jsonStore) ListSyncRuns(ctx context.Context, limit int) ([]SyncResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []SyncResult{}
	for i := len(s.syncRuns) - 1; i >= 0 && (limit <= 0 || len(out) < limit); i-- {
		out = append(out, s.syncRuns[i])
	}
	return out, nil
}

func (s *jsonStore) ApplySyncPlan(ctx context.Context, p *SyncPlan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range p.Creates {
		e := copyEmp(&c)
		e.ID = s.nextEmp
		s.nextEmp++
		if s.userTaken(e.Username, 0) {
			e.Username = ""
		}
		e.Source, e.Active, e.Hidden = "ad", true, false
		e.CreatedAt, e.UpdatedAt = p.Now, p.Now
		now := p.Now
		e.LastSyncedAt = &now
		s.emp[e.ID] = &e
	}
	for _, u := range p.Updates {
		e := s.emp[u.ID]
		if e == nil {
			continue
		}
		if u.SetUsername != "" && e.Username == "" && !s.userTaken(u.SetUsername, e.ID) {
			e.Username = u.SetUsername
		}
		e.setFields(u.Fields)
		e.Source = "ad"
		if u.GUID != "" {
			e.ADGUID = u.GUID
		}
		e.ADDN = u.DN
		e.Active = true
		now := p.Now
		e.LastSyncedAt = &now
		if u.Changed {
			e.UpdatedAt = p.Now
		}
	}
	for _, id := range p.Deactivate {
		if e := s.emp[id]; e != nil {
			e.Active = false
			e.UpdatedAt = p.Now
		}
	}
	return s.commit()
}

// ---------- Gesamtabzug ----------

func (s *jsonStore) Export(ctx context.Context) (*Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := s.snapshot(true)
	snap.Source = "json"
	return snap, nil
}

// Import ersetzt den Inhalt; die bisherige Datei wird vorher als .bak gesichert.
func (s *jsonStore) Import(ctx context.Context, snap *Snapshot) error {
	if err := snap.validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range []string{s.path, s.auditPath} {
		if b, err := os.ReadFile(p); err == nil {
			if err := os.WriteFile(p+".bak", b, 0o600); err != nil {
				return fmt.Errorf("Sicherung %s.bak: %w", p, err)
			}
		}
	}
	s.apply(snap)
	if snap.Source == "mysql" {
		now := time.Now().UTC()
		s.importedAt = &now
	}
	if err := s.persistAudit(); err != nil {
		return err
	}
	return s.commit()
}

// Info für die Admin-Oberfläche
func (s *jsonStore) info() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var size, asize int64
	if fi, err := os.Stat(s.path); err == nil {
		size = fi.Size()
	}
	if fi, err := os.Stat(s.auditPath); err == nil {
		asize = fi.Size()
	}
	return map[string]any{"path": s.path, "size": size, "auditPath": s.auditPath, "auditSize": asize, "auditEntries": len(s.audit), "savedAt": s.savedAt, "source": s.source, "importedAt": s.importedAt, "employees": len(s.emp)}
}

// ---------- Vertretungen, Abwesenheiten, Organisation ----------

func (s *jsonStore) SetDeputies(ctx context.Context, id int64, deps []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.emp[id]
	if e == nil {
		return ErrNotFound
	}
	e.Deputies = copyIDs(deps)
	return s.commit()
}

func (s *jsonStore) SetAbsences(ctx context.Context, id int64, abs []Absence) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.emp[id]
	if e == nil {
		return ErrNotFound
	}
	e.Absences = copyAbsences(abs)
	return s.commit()
}

func (s *jsonStore) SetOrgUnit(ctx context.Context, ids []int64, unit int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, id := range ids {
		if e := s.emp[id]; e != nil {
			e.OrgUnit = unit
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	return n, s.commit()
}

func (s *jsonStore) ListOrgUnits(ctx context.Context) ([]OrgUnit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]OrgUnit, 0, len(s.units))
	for _, u := range s.units {
		out = append(out, copyUnit(u))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sort != out[j].Sort {
			return out[i].Sort < out[j].Sort
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *jsonStore) CreateOrgUnit(ctx context.Context, u OrgUnit) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := copyUnit(&u)
	c.ID = s.nextUnit
	s.nextUnit++
	s.units[c.ID] = &c
	return c.ID, s.commit()
}

func (s *jsonStore) UpdateOrgUnit(ctx context.Context, u OrgUnit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.units[u.ID] == nil {
		return ErrNotFound
	}
	c := copyUnit(&u)
	s.units[u.ID] = &c
	return s.commit()
}

func (s *jsonStore) DeleteOrgUnit(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.units[id]
	if u == nil {
		return ErrNotFound
	}
	for _, c := range s.units {
		if c.Parent == id {
			c.Parent = u.Parent
		}
	}
	for _, e := range s.emp {
		if e.OrgUnit == id {
			e.OrgUnit = u.Parent
		}
	}
	for _, t := range s.tasks {
		if t.OrgUnit == id {
			t.OrgUnit = u.Parent
		}
	}
	delete(s.units, id)
	return s.commit()
}

func (s *jsonStore) GetSetting(ctx context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings[key], nil
}

func (s *jsonStore) SetSetting(ctx context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[key] = value
	return s.commit()
}

// ---------- Fehlermeldungen ----------

const jsonReportsMax = 2000 // erledigte Meldungen werden darüber hinaus verworfen (älteste zuerst)

func (s *jsonStore) CreateReport(ctx context.Context, r Report) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = s.nextReport
	s.nextReport++
	r.Status = "open"
	r.ResolvedBy, r.ResolvedAt, r.Note = "", nil, ""
	s.reports = append(s.reports, r)
	if len(s.reports) > jsonReportsMax {
		for i, x := range s.reports {
			if x.Status != "open" {
				s.reports = append(s.reports[:i], s.reports[i+1:]...)
				break
			}
		}
	}
	return r.ID, s.commit()
}

func (s *jsonStore) ListReports(ctx context.Context, status string, limit int) ([]Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Report{}
	for i := len(s.reports) - 1; i >= 0; i-- { // neueste zuerst
		r := s.reports[i]
		if status != "" && r.Status != status {
			continue
		}
		out = append(out, r)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *jsonStore) UpdateReport(ctx context.Context, id int64, status, by, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.reports {
		r := &s.reports[i]
		if r.ID != id {
			continue
		}
		r.Status, r.Note = status, note
		if status == "open" {
			r.ResolvedBy, r.ResolvedAt = "", nil
		} else {
			now := time.Now().UTC()
			r.ResolvedBy, r.ResolvedAt = by, &now
		}
		return s.commit()
	}
	return ErrNotFound
}

func (s *jsonStore) CountReports(ctx context.Context, status, reporter string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, r := range s.reports {
		if r.Status == status && (reporter == "" || r.Reporter == reporter) {
			n++
		}
	}
	return n, nil
}

// ---------- Kontaktwege ----------

func (s *jsonStore) ListChannels(ctx context.Context) ([]ContactChannel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ContactChannel, 0, len(s.channels))
	for _, c := range s.channels {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *jsonStore) CreateChannel(ctx context.Context, c ContactChannel) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.ID = s.nextChan
	s.nextChan++
	s.channels[c.ID] = &c
	return c.ID, s.commit()
}

func (s *jsonStore) UpdateChannel(ctx context.Context, c ContactChannel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channels[c.ID] == nil {
		return ErrNotFound
	}
	s.channels[c.ID] = &c
	return s.commit()
}

func (s *jsonStore) DeleteChannel(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channels[id] == nil {
		return ErrNotFound
	}
	delete(s.channels, id)
	for _, e := range s.emp {
		if e.Channel == id {
			e.Channel = 0
		}
	}
	return s.commit()
}

func (s *jsonStore) SetChannel(ctx context.Context, ids []int64, channel int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, id := range ids {
		if e := s.emp[id]; e != nil {
			e.Channel = channel
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	return n, s.commit()
}

// ---------- Persönliche Einstellungen ----------

func (s *jsonStore) GetPrefs(ctx context.Context, user string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.prefs[user], nil
}

func (s *jsonStore) SetPrefs(ctx context.Context, user, prefs string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefs[user] = prefs
	return s.commit()
}

func (s *jsonStore) ListPrefs(ctx context.Context) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.prefs))
	for k, v := range s.prefs {
		out[k] = v
	}
	return out, nil
}

// ---------- Zuständigkeitsfinder ----------

func (s *jsonStore) ListTasks(ctx context.Context) ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		out = append(out, t.clone())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sort != out[j].Sort {
			return out[i].Sort < out[j].Sort
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *jsonStore) CreateTask(ctx context.Context, t Task) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := t.clone()
	c.ID = s.nextTask
	s.nextTask++
	s.tasks[c.ID] = &c
	return c.ID, s.commit()
}

func (s *jsonStore) UpdateTask(ctx context.Context, t Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks[t.ID] == nil {
		return ErrNotFound
	}
	c := t.clone()
	s.tasks[t.ID] = &c
	return s.commit()
}

func (s *jsonStore) DeleteTask(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks[id] == nil {
		return ErrNotFound
	}
	delete(s.tasks, id)
	return s.commit()
}
