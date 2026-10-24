// Telefonbuch – internes, webbasiertes Telefonbuch mit Schlagworten, Pixel-Avataren,
// LDAPS-Anmeldung und optionalem Active-Directory-Abgleich.
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

// version wird beim Build gesetzt: -ldflags "-X main.version=1.2.3"
var version = "dev"

type App struct {
	cfg     *Config
	storeP  atomic.Pointer[storeBox] // austauschbar (Rückfall auf JSON und zurück)
	offline atomic.Bool              // läuft gerade auf der JSON-Reserve (MySQL nicht erreichbar)
	ctx     context.Context
	devHash string // Passwort-Hash des DEV-Admins (wird nach einer MySQL-Übernahme wieder eingetragen)
	ldap    *LDAPClient
	limiter *limiter
	sync    syncer
	storage storageState
}

func main() {
	log.SetFlags(log.LstdFlags)
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		switch os.Args[1] {
		case "useradd":
			cmdUserAdd(os.Args[2:])
		case "sync":
			cmdSync(os.Args[2:])
		case "healthcheck":
			cmdHealthcheck(os.Args[2:])
		case "export-json":
			cmdExportJSON(os.Args[2:])
		case "import-json":
			cmdImportJSON(os.Args[2:])
		case "version":
			fmt.Println("telefonbuch", version)
		default:
			fmt.Fprintf(os.Stderr, "Unbekannter Befehl %q. Befehle: useradd, sync, export-json, import-json, healthcheck, version\n", os.Args[1])
			os.Exit(2)
		}
		return
	}

	cfgPath := flag.String("config", "config.json", "Pfad zur Konfigurationsdatei (leer = nur Standardwerte/Umgebung)")
	demo := flag.Bool("demo", false, "Beispieldaten anlegen (nur wenn das Telefonbuch leer ist) und lokale Anmeldung aktivieren")
	dev := flag.Bool("dev", false, "Frontend aus ./web statt aus dem Binary ausliefern")
	devAdmin := flag.Bool("dev-admin", false, "NUR ENTWICKLUNG: lokales Admin-Konto am AD vorbei (wie auth.devAdmin)")
	flag.Parse()
	if *devAdmin {
		os.Setenv("TB_DEV_ADMIN", "1")
	}

	path := *cfgPath
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) && path == "config.json" {
		path = ""
	}
	cfg, err := loadConfigDemo(path, *demo)
	if err != nil {
		log.Fatalf("Konfiguration: %v", err)
	}
	app, err := newApp(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer app.st().Close()
	if cfg.Auth.DevAdmin {
		if err := ensureDevAdmin(app); err != nil {
			log.Fatalf("DEV-Admin: %v", err)
		}
	}
	if app.st().Kind() == "json" && cfg.Storage.MySQLImport == "startup" {
		ictx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		if _, err := app.importFromMySQL(ictx, "start", false); err != nil {
			log.Printf("Weiter mit vorhandener JSON-Datei (offline).")
		}
		cancel()
	}
	if *demo {
		if err := seedDemo(app); err != nil {
			log.Fatalf("Demo-Daten: %v", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go app.sessionGC(ctx)
	go app.syncLoop(ctx)
	app.ctx = ctx
	if !app.offline.Load() {
		compactStoredAvatars(ctx, app.st())
	}
	go app.storageLoop(ctx)
	go app.reconnectLoop(ctx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.routes(*dev),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      11 * time.Minute, // manueller AD-Sync kann dauern
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    32 << 10,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if cfg.TLS.CertFile != "" {
		log.Printf("Telefonbuch läuft auf https://%s", displayAddr(cfg.Addr))
		err = srv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	} else {
		log.Printf("Telefonbuch läuft auf http://%s (ohne TLS – im Betrieb hinter einem HTTPS-Reverse-Proxy oder mit tls.certFile betreiben)", displayAddr(cfg.Addr))
		err = srv.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func displayAddr(a string) string {
	if strings.HasPrefix(a, ":") {
		return "localhost" + a
	}
	return a
}

func loadConfigDemo(path string, demo bool) (*Config, error) {
	if demo && path == "" && os.Getenv("TB_AUTH_MODE") == "" {
		os.Setenv("TB_AUTH_MODE", "local")
	}
	return loadConfig(path)
}

func newApp(cfg *Config) (*App, error) {
	st, err := openStore(cfg)
	if err != nil {
		return nil, err
	}
	app := &App{cfg: cfg, ctx: context.Background(), limiter: newLimiter(cfg.Auth.MaxFailsPerIP, cfg.Auth.MaxFailsPerUsr, cfg.Auth.LockDuration.Duration)}
	app.setStore(st)
	if _, ok := st.(*readOnlyStore); ok {
		app.offline.Store(true)
	}
	if cfg.LDAP.URL != "" {
		app.ldap, err = newLDAPClient(&cfg.LDAP)
		if err != nil {
			return nil, err
		}
		if cfg.LDAP.InsecureSkipVerify {
			log.Printf("WARNUNG: ldap.insecureSkipVerify ist aktiv – das Zertifikat des Domänencontrollers wird NICHT geprüft!")
		}
	}
	return app, nil
}

// ---------- HTTP ----------

func (a *App) routes(dev bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", a.handleLogin)
	mux.HandleFunc("POST /api/logout", a.requireAuth(a.handleLogout))
	mux.HandleFunc("GET /api/me", a.requireAuth(a.handleMe))
	mux.HandleFunc("GET /api/directory", a.optionalAuth(a.handleDirectory))
	mux.HandleFunc("GET /api/site", a.handleSite)

	mux.HandleFunc("PUT /api/employees/{id}/tags", a.requireAuth(a.handleSetTags))
	mux.HandleFunc("PUT /api/employees/{id}/avatar", a.requireAuth(a.handleSetAvatar))
	mux.HandleFunc("POST /api/employees", a.requireAdmin(a.handleCreateEmployee))
	mux.HandleFunc("PUT /api/employees/{id}", a.requireAdmin(a.handleUpdateEmployee))
	mux.HandleFunc("DELETE /api/employees/{id}", a.requireAdmin(a.handleDeleteEmployee))

	mux.HandleFunc("GET /api/tags", a.requireAdmin(a.handleListTags))
	mux.HandleFunc("POST /api/tags", a.requireAdmin(a.handleCreateTag))
	mux.HandleFunc("PUT /api/tags/{id}", a.requireAdmin(a.handleUpdateTag))
	mux.HandleFunc("POST /api/tags/{id}/merge", a.requireAdmin(a.handleMergeTag))
	mux.HandleFunc("DELETE /api/tags/{id}", a.requireAdmin(a.handleDeleteTag))
	mux.HandleFunc("POST /api/tags/cleanup", a.requireAdmin(a.handleDeleteUnusedTags))
	mux.HandleFunc("POST /api/categories", a.requireAdmin(a.handleCreateCategory))
	mux.HandleFunc("PUT /api/categories/{key}", a.requireAdmin(a.handleUpdateCategory))
	mux.HandleFunc("POST /api/categories/{key}/delete", a.requireAdmin(a.handleDeleteCategory))

	mux.HandleFunc("POST /api/private", a.requireAuth(a.handleCreatePrivate))
	mux.HandleFunc("PUT /api/private/{id}", a.requireAuth(a.handleUpdatePrivate))
	mux.HandleFunc("DELETE /api/private/{id}", a.requireAuth(a.handleDeletePrivate))

	mux.HandleFunc("GET /api/admin/sync", a.requireAdmin(a.handleSyncStatus))
	mux.HandleFunc("POST /api/admin/sync", a.requireAdmin(a.handleSyncRun))
	mux.HandleFunc("POST /api/admin/ldap-test", a.requireAdmin(a.handleLDAPTest))
	mux.HandleFunc("GET /api/admin/audit", a.requireAdmin(a.handleAudit))
	mux.HandleFunc("GET /api/admin/storage", a.requireAdmin(a.handleStorageStatus))
	mux.HandleFunc("POST /api/admin/storage/import-mysql", a.requireAdmin(a.handleStorageImport))
	mux.HandleFunc("POST /api/admin/storage/snapshot", a.requireAdmin(a.handleStorageSnapshot))

	mux.HandleFunc("GET /client/telefonbuch-windows.zip", a.handleClientZip)

	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) {
		jsonError(w, http.StatusNotFound, "Unbekannter Endpunkt")
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.st().Ping(ctx); err != nil {
			http.Error(w, "storage", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})

	var static fs.FS
	if dev {
		static = os.DirFS("web")
	} else {
		static, _ = fs.Sub(webFS, "web")
	}
	files := http.FileServer(http.FS(static))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "no-cache") // kleine Dateien; ETag/Last-Modified reicht
		}
		if strings.HasSuffix(r.URL.Path, "/") && r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
	return a.securityHeaders(a.readOnlyGuard(mux))
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if a.isHTTPS(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- Befehle ----------

func cmdUserAdd(args []string) {
	fsx := flag.NewFlagSet("useradd", flag.ExitOnError)
	cfgPath := fsx.String("config", "config.json", "Konfigurationsdatei")
	admin := fsx.Bool("admin", false, "Administratorrechte")
	name := fsx.String("name", "", "Anzeigename")
	fsx.Usage = func() {
		fmt.Fprintln(os.Stderr, "Lokales Konto anlegen bzw. Passwort ändern (Notfallzugang oder auth.mode=local).")
		fmt.Fprintln(os.Stderr, "Aufruf: telefonbuch useradd [-config config.json] [-admin] [-name \"Max Muster\"] BENUTZERNAME")
		fmt.Fprintln(os.Stderr, "Passwort aus TB_PASSWORD oder einzeilig von der Standardeingabe.")
		fsx.PrintDefaults()
	}
	_ = fsx.Parse(args)
	if fsx.NArg() != 1 {
		fsx.Usage()
		os.Exit(2)
	}
	user := normalizeLoginName(fsx.Arg(0))
	if !reLoginName.MatchString(user) {
		log.Fatal("Benutzername ungültig")
	}
	pw := os.Getenv("TB_PASSWORD")
	if pw == "" {
		fmt.Fprint(os.Stderr, "Passwort: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			log.Fatal("Kein Passwort gelesen")
		}
		pw = strings.TrimRight(line, "\r\n")
	}
	if len(pw) < 12 {
		log.Fatal("Passwort muss mindestens 12 Zeichen haben")
	}
	path := *cfgPath
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) && path == "config.json" {
		path = ""
	}
	cfg, err := loadConfig(path)
	if err != nil {
		log.Fatal(err)
	}
	st, err := openStore(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	h, err := hashPassword(pw)
	if err != nil {
		log.Fatal(err)
	}
	if *name == "" {
		*name = user
	}
	ctx := context.Background()
	if err := st.UpsertLocalUser(ctx, LocalUser{Username: user, DisplayName: *name, PwHash: h, IsAdmin: *admin, CreatedAt: time.Now().UTC()}); err != nil {
		log.Fatal(err)
	}
	_ = st.DeleteUserSessions(ctx, user, "local")
	if st.Kind() == "json" {
		fmt.Println("Hinweis (JSON-Modus): Den Server vorher beenden, sonst überschreibt er die Datei mit seinem Stand.")
	}
	fmt.Printf("Lokales Konto %q gespeichert (Admin: %v). Hinweis: Anmeldung nur, wenn auth.mode \"local\" oder \"ldap+local\" ist.\n", user, *admin)
}

func cmdSync(args []string) {
	fsx := flag.NewFlagSet("sync", flag.ExitOnError)
	cfgPath := fsx.String("config", "config.json", "Konfigurationsdatei")
	_ = fsx.Parse(args)
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	if !cfg.Sync.Enabled {
		log.Fatal("adSync.enabled ist nicht gesetzt")
	}
	app, err := newApp(cfg)
	if err != nil {
		log.Fatal(err)
	}
	res, err := app.RunSync(context.Background(), "kommandozeile")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("OK: gelesen %d, neu %d, geändert %d, deaktiviert %d, reaktiviert %d\n", res.Seen, res.Created, res.Updated, res.Deactivated, res.Reactivated)
}

func cfgPathOrEmpty(p string) string {
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) && p == "config.json" {
		return ""
	}
	return p
}

// export-json: MySQL → JSON-Datei (z. B. als Offline-Reserve oder um den JSON-Modus zu befüllen)
func cmdExportJSON(args []string) {
	fsx := flag.NewFlagSet("export-json", flag.ExitOnError)
	cfgPath := fsx.String("config", "config.json", "Konfigurationsdatei (MySQL-Zugang)")
	out := fsx.String("out", "", "Zieldatei (Standard: storage.jsonPath)")
	force := fsx.Bool("force", false, "auch überschreiben, wenn MySQL deutlich weniger Einträge enthält")
	_ = fsx.Parse(args)
	cfg, err := loadConfig(cfgPathOrEmpty(*cfgPath))
	if err != nil {
		log.Fatal(err)
	}
	if *out == "" {
		*out = cfg.Storage.JSONPath
	}
	src, err := newMySQLStore(cfg, 3)
	if err != nil {
		log.Fatal(err)
	}
	defer src.Close()
	snap, err := src.Export(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	if err := snap.validate(); err != nil {
		log.Fatal(err)
	}
	auditOut := cfg.auditPathFor(*out)
	if old, _, err := readSnapshotFiles(*out, auditOut); err == nil && len(old.Employees) > 0 && len(snap.Employees)*100 < len(old.Employees)*cfg.Storage.MySQLImportMinPercent && !*force {
		log.Fatalf("Abbruch: MySQL enthält %d Einträge, %s aber %d. Mit -force trotzdem überschreiben.", len(snap.Employees), *out, len(old.Employees))
	}
	now := time.Now().UTC()
	snap.ImportedAt = &now
	for _, p := range []string{*out, auditOut} {
		if b, err := os.ReadFile(p); err == nil {
			if err := os.WriteFile(p+".bak", b, 0o600); err != nil {
				log.Fatal(err)
			}
		}
	}
	if err := writeSnapshotFiles(*out, auditOut, snap); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("OK: %d Einträge, %d Schlagworte, %d private Kontakte, %d lokale Konten → %s (Protokoll: %s)\n", len(snap.Employees), len(snap.Tags), len(snap.Private), len(snap.LocalUsers), *out, auditOut)
	fmt.Println("Hinweis: Läuft ein Server im JSON-Modus auf dieser Datei, stattdessen den Knopf „Aus MySQL übernehmen“ verwenden.")
}

// import-json: JSON-Datei → MySQL (ersetzt alle Daten in MySQL!)
func cmdImportJSON(args []string) {
	fsx := flag.NewFlagSet("import-json", flag.ExitOnError)
	cfgPath := fsx.String("config", "config.json", "Konfigurationsdatei (MySQL-Zugang)")
	in := fsx.String("in", "", "Quelldatei (Standard: storage.jsonPath)")
	force := fsx.Bool("force", false, "Bestätigung: ALLE Daten in MySQL werden ersetzt")
	_ = fsx.Parse(args)
	cfg, err := loadConfig(cfgPathOrEmpty(*cfgPath))
	if err != nil {
		log.Fatal(err)
	}
	if *in == "" {
		*in = cfg.Storage.JSONPath
	}
	snap, _, err := readSnapshotFiles(*in, cfg.auditPathFor(*in))
	if err != nil {
		log.Fatal(err)
	}
	if !*force {
		log.Fatalf("Würde %d Einträge, %d Schlagworte, %d private Kontakte nach MySQL schreiben und dabei ALLE vorhandenen Daten ersetzen. Mit -force bestätigen.", len(snap.Employees), len(snap.Tags), len(snap.Private))
	}
	dst, err := newMySQLStore(cfg, 3)
	if err != nil {
		log.Fatal(err)
	}
	defer dst.Close()
	if err := dst.Import(context.Background(), snap); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("OK: %d Einträge, %d Schlagworte, %d private Kontakte nach MySQL übertragen\n", len(snap.Employees), len(snap.Tags), len(snap.Private))
}

// ensureDevAdmin legt das lokale Entwickler-Admin-Konto an bzw. setzt dessen Passwort.
func ensureDevAdmin(a *App) error {
	pw := os.Getenv("TB_DEV_ADMIN_PASSWORD")
	gen := pw == ""
	if gen {
		pw = randomToken(12)
	} else if len(pw) < 12 {
		return errors.New("TB_DEV_ADMIN_PASSWORD muss mindestens 12 Zeichen haben")
	}
	h, err := hashPassword(pw)
	if err != nil {
		return err
	}
	user := a.cfg.Auth.DevAdminUser
	a.devHash = h
	if err := a.st().UpsertLocalUser(context.Background(), LocalUser{Username: user, DisplayName: "DEV-Admin", PwHash: h, IsAdmin: true, CreatedAt: time.Now().UTC()}); err != nil {
		return err
	}
	_ = a.st().DeleteUserSessions(context.Background(), user, "dev")
	log.Printf("╔══════════════════════════════════════════════════════════════════╗")
	log.Printf("║ WARNUNG: DEV-Admin aktiv – Anmeldung am AD vorbei möglich!       ║")
	log.Printf("║ Nur für Entwicklung/Test. Im Betrieb auth.devAdmin = false.      ║")
	if gen {
		log.Printf("║ Benutzer: %-20s Passwort: %-24s║", user, pw)
	} else {
		log.Printf("║ Benutzer: %-20s Passwort aus TB_DEV_ADMIN_PASSWORD       ║", user)
	}
	log.Printf("╚══════════════════════════════════════════════════════════════════╝")
	return nil
}

// healthcheck: für Docker-HEALTHCHECK (das Image enthält kein curl)
func cmdHealthcheck(args []string) {
	fsx := flag.NewFlagSet("healthcheck", flag.ExitOnError)
	url := fsx.String("url", "http://127.0.0.1:8080/healthz", "Adresse des Health-Endpunkts")
	_ = fsx.Parse(args)
	c := &http.Client{Timeout: 4 * time.Second}
	resp, err := c.Get(*url)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "Status", resp.StatusCode)
		os.Exit(1)
	}
}
