package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Zweite Datenhaltung: JSON-Datei (Offline-Betrieb) und das Übertragen MySQL → JSON.

type storageState struct {
	mu            sync.Mutex
	busy          bool
	lastImport    *time.Time
	lastImportOK  bool
	lastImportMsg string
	lastSnap      *time.Time
	lastSnapMsg   string
}

func openStore(cfg *Config) (Store, error) {
	switch cfg.Storage.Mode {
	case "json":
		st, err := newJSONStore(cfg.Storage.JSONPath, cfg.auditPathFor(cfg.Storage.JSONPath))
		if err != nil {
			return nil, fmt.Errorf("JSON-Datenhaltung: %w", err)
		}
		log.Printf("Datenhaltung: JSON-Datei %s", cfg.Storage.JSONPath)
		return st, nil
	default:
		attempts := 30
		if cfg.fallbackEnabled() {
			if _, err := os.Stat(cfg.Storage.JSONPath); err == nil {
				attempts = 5 // Reserve vorhanden: nicht lange warten
			}
		}
		st, err := newMySQLStore(cfg, attempts)
		if err != nil {
			if !cfg.fallbackEnabled() {
				return nil, err
			}
			fb, ferr := openFallbackStore(cfg)
			if ferr != nil {
				return nil, fmt.Errorf("%w (Rückfall auf JSON nicht möglich: %v)", err, ferr)
			}
			log.Printf("WARNUNG: MySQL nicht erreichbar (%v) – Rückfall-Betrieb mit der JSON-Kopie %s (nur lesen, Stand %s)", err, cfg.Storage.JSONPath, fb.savedAt.Format("02.01.2006 15:04"))
			return fb, nil
		}
		log.Printf("Datenhaltung: MySQL")
		return st, nil
	}
}

func (a *App) lockStorage() bool {
	a.storage.mu.Lock()
	defer a.storage.mu.Unlock()
	if a.storage.busy {
		return false
	}
	a.storage.busy = true
	return true
}

func (a *App) unlockStorage() {
	a.storage.mu.Lock()
	a.storage.busy = false
	a.storage.mu.Unlock()
}

// importFromMySQL ersetzt die JSON-Daten durch den aktuellen Stand der MySQL-Datenbank.
// Ist MySQL nicht erreichbar, bleibt die JSON-Datei unverändert (Offline-Betrieb).
// errShrink: MySQL enthält deutlich weniger Einträge als die JSON-Datei – Schutz vor leerer/falscher DB
type errShrink struct{ have, got int }

func (e errShrink) Error() string {
	if e.got == 0 {
		return fmt.Sprintf("Abbruch: Die MySQL-Datenbank enthält keine Einträge (JSON-Datei: %d) – JSON-Daten bleiben unverändert", e.have)
	}
	return fmt.Sprintf("Abbruch: MySQL enthält nur %d Einträge, die JSON-Datei %d (Grenze storage.mysqlImportMinPercent) – JSON-Daten bleiben unverändert", e.got, e.have)
}

func (a *App) importFromMySQL(ctx context.Context, trigger string, force bool) (*Snapshot, error) {
	if a.st().Kind() != "json" {
		return nil, errors.New("Nur im JSON-Modus möglich")
	}
	if !a.lockStorage() {
		return nil, errors.New("Eine Übertragung läuft bereits")
	}
	defer a.unlockStorage()
	snap, err := func() (*Snapshot, error) {
		src, err := newMySQLStore(a.cfg, 1)
		if err != nil {
			return nil, fmt.Errorf("MySQL nicht erreichbar – JSON-Daten bleiben unverändert: %w", err)
		}
		defer src.Close()
		snap, err := src.Export(ctx)
		if err != nil {
			return nil, fmt.Errorf("Lesen aus MySQL: %w", err)
		}
		have, err := a.st().ListEmployees(ctx, true)
		if err != nil {
			return nil, err
		}
		got := len(snap.Employees)
		if len(have) > 0 && (got == 0 || (!force && got*100 < len(have)*a.cfg.Storage.MySQLImportMinPercent)) {
			return nil, errShrink{have: len(have), got: got}
		}
		if err := snap.validate(); err != nil {
			return nil, err
		}
		if err := a.st().Import(ctx, snap); err != nil {
			return nil, fmt.Errorf("Schreiben der JSON-Datei: %w", err)
		}
		if a.cfg.Auth.DevAdmin && a.devHash != "" {
			_ = a.st().UpsertLocalUser(ctx, LocalUser{Username: a.cfg.Auth.DevAdminUser, DisplayName: "DEV-Admin", PwHash: a.devHash, IsAdmin: true, CreatedAt: time.Now().UTC()})
		}
		return snap, nil
	}()
	now := time.Now().UTC()
	a.storage.mu.Lock()
	a.storage.lastImport, a.storage.lastImportOK = &now, err == nil
	if err != nil {
		a.storage.lastImportMsg = err.Error()
	} else {
		a.storage.lastImportMsg = fmt.Sprintf("%d Einträge, %d Schlagworte, %d private Kontakte übernommen", len(snap.Employees), len(snap.Tags), len(snap.Private))
	}
	msg := a.storage.lastImportMsg
	a.storage.mu.Unlock()
	status := "ok"
	if err != nil {
		status = "error"
		log.Printf("MySQL → JSON (%s): %v", trigger, err)
	} else {
		log.Printf("MySQL → JSON (%s): %s", trigger, msg)
	}
	a.audit(nil, trigger, "mysql_import", status, msg)
	return snap, err
}

// writeJSONSnapshot schreibt im MySQL-Modus eine vollständige Kopie nach storage.jsonPath.
// existingSnapshotCount: Anzahl Einträge in einer vorhandenen JSON-Kopie (-1 = keine/unlesbar)
func existingSnapshotCount(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	var x struct {
		Employees []json.RawMessage `json:"employees"`
	}
	if json.Unmarshal(b, &x) != nil {
		return -1
	}
	return len(x.Employees)
}

func (a *App) writeJSONSnapshot(ctx context.Context, trigger string, force bool) error {
	if a.st().Kind() != "mysql" {
		return errors.New("Nur im MySQL-Modus möglich")
	}
	if !a.lockStorage() {
		return errors.New("Eine Übertragung läuft bereits")
	}
	defer a.unlockStorage()
	snap, err := a.st().Export(ctx)
	if err == nil {
		// Schutz der Offline-Reserve: eine leere/falsche Datenbank überschreibt keine gute Kopie
		if have := existingSnapshotCount(a.cfg.Storage.JSONPath); have > 0 && !force {
			got := len(snap.Employees)
			if got == 0 || got*100 < have*a.cfg.Storage.MySQLImportMinPercent {
				err = errShrinkCopy{have: have, got: got}
			}
		}
	}
	if err == nil {
		if err = snap.validate(); err == nil {
			err = writeSnapshotFiles(a.cfg.Storage.JSONPath, a.cfg.auditPathFor(a.cfg.Storage.JSONPath), snap)
		}
	}
	now := time.Now().UTC()
	a.storage.mu.Lock()
	a.storage.lastSnap = &now
	if err != nil {
		a.storage.lastSnapMsg = err.Error()
		log.Printf("JSON-Kopie (%s): %v", trigger, err)
	} else {
		a.storage.lastSnapMsg = fmt.Sprintf("%d Einträge geschrieben", len(snap.Employees))
	}
	a.storage.mu.Unlock()
	return err
}

type errShrinkCopy struct{ have, got int }

func (e errShrinkCopy) Error() string {
	return fmt.Sprintf("Abbruch: MySQL enthält nur %d Einträge, die vorhandene JSON-Kopie %d (Grenze storage.mysqlImportMinPercent) – Kopie bleibt unverändert", e.got, e.have)
}

func (a *App) storageLoop(ctx context.Context) {
	if a.offline.Load() {
		return // Rückfall-Betrieb: reconnectLoop startet die Schleife nach dem Zurückschalten neu
	}
	var iv time.Duration
	var job func()
	switch {
	case a.st().Kind() == "json" && a.cfg.Storage.MySQLImportInterval.Duration > 0:
		iv = a.cfg.Storage.MySQLImportInterval.Duration
		job = func() {
			c, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			_, _ = a.importFromMySQL(c, "zeitgesteuert", false)
		}
	case a.st().Kind() == "mysql" && a.cfg.Storage.JSONSnapshotInterval.Duration > 0:
		iv = a.cfg.Storage.JSONSnapshotInterval.Duration
		job = func() {
			c, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			_ = a.writeJSONSnapshot(c, "zeitgesteuert", false)
		}
	default:
		return
	}
	t := time.NewTicker(iv)
	defer t.Stop()
	if a.st().Kind() == "mysql" {
		job() // Kopie gleich beim Start
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			job()
		}
	}
}

// ---------- Admin-Oberfläche ----------

func (a *App) handleStorageStatus(w http.ResponseWriter, r *http.Request) {
	a.storage.mu.Lock()
	out := map[string]any{
		"mode": a.st().Kind(), "jsonPath": a.cfg.Storage.JSONPath, "busy": a.storage.busy,
		"mysqlImport": a.cfg.Storage.MySQLImport, "mysqlImportInterval": a.cfg.Storage.MySQLImportInterval.String(),
		"snapshotInterval": a.cfg.Storage.JSONSnapshotInterval.String(),
		"lastImport":       a.storage.lastImport, "lastImportOK": a.storage.lastImportOK, "lastImportMsg": a.storage.lastImportMsg,
		"lastSnapshot": a.storage.lastSnap, "lastSnapshotMsg": a.storage.lastSnapMsg,
	}
	a.storage.mu.Unlock()
	out["offline"] = a.offline.Load()
	switch js := a.st().(type) {
	case *jsonStore:
		out["json"] = js.info()
	case *readOnlyStore:
		out["json"] = js.info()
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) handleStorageImport(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	if a.st().Kind() != "json" || a.cfg.Storage.MySQLImport == "off" {
		jsonError(w, http.StatusBadRequest, "Übernahme aus MySQL ist nicht aktiviert (storage.mode = json und storage.mysqlImport ≠ off)")
		return
	}
	var req struct {
		Force bool `json:"force"`
	}
	if !decodeJSON(w, r, &req, 1<<10) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	snap, err := a.importFromMySQL(ctx, s.Username, req.Force)
	if err != nil {
		var sh errShrink
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "canForce": errors.As(err, &sh) && sh.got > 0})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "employees": len(snap.Employees), "tags": len(snap.Tags), "private": len(snap.Private)})
}

func (a *App) handleStorageSnapshot(w http.ResponseWriter, r *http.Request) {
	if a.st().Kind() != "mysql" {
		jsonError(w, http.StatusBadRequest, "Nur im MySQL-Modus")
		return
	}
	var req struct {
		Force bool `json:"force"`
	}
	if !decodeJSON(w, r, &req, 1<<10) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := a.writeJSONSnapshot(ctx, sessionFrom(r).Username, req.Force); err != nil {
		var sh errShrinkCopy
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "canForce": errors.As(err, &sh)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": a.cfg.Storage.JSONPath})
}
