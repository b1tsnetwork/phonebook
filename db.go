package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// attempts: Anzahl Verbindungsversuche im Abstand von 2 s (Start in docker compose → 30, Import → 1)
func openDB(c *Config, attempts int) (*sql.DB, error) {
	var mc *mysql.Config
	if c.DB.DSN != "" {
		var err error
		mc, err = mysql.ParseDSN(c.DB.DSN)
		if err != nil {
			return nil, fmt.Errorf("db.dsn: %w", err)
		}
	} else {
		mc = mysql.NewConfig()
		mc.User = c.DB.User
		mc.Passwd = c.DB.Password
		mc.Net = "tcp"
		mc.Addr = fmt.Sprintf("%s:%d", c.DB.Host, c.DB.Port)
		mc.DBName = c.DB.Name
		switch c.DB.TLS {
		case "", "false":
		case "custom":
			pem, err := os.ReadFile(c.DB.CAFile)
			if err != nil {
				return nil, fmt.Errorf("db.caFile: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, errors.New("db.caFile enthält kein gültiges Zertifikat")
			}
			if err := mysql.RegisterTLSConfig("custom", &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12, ServerName: c.DB.Host}); err != nil {
				return nil, err
			}
			mc.TLSConfig = "custom"
		default:
			mc.TLSConfig = c.DB.TLS
		}
	}
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.Collation = "utf8mb4_unicode_ci"
	mc.MultiStatements = false
	mc.InterpolateParams = false
	if mc.Params == nil {
		mc.Params = map[string]string{}
	}
	mc.Params["time_zone"] = "'+00:00'"
	mc.Timeout = 10 * time.Second
	mc.ReadTimeout = 30 * time.Second
	mc.WriteTimeout = 30 * time.Second

	db, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	// Beim Start (z. B. in docker compose) etwas auf die Datenbank warten
	var lastErr error
	if attempts < 1 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		lastErr = db.PingContext(ctx)
		cancel()
		if lastErr == nil {
			return db, nil
		}
		if i == attempts-1 {
			break
		}
		if i == 0 {
			log.Printf("Warte auf Datenbank: %v", lastErr)
		}
		time.Sleep(2 * time.Second)
	}
	db.Close()
	return nil, fmt.Errorf("Datenbank nicht erreichbar: %w", lastErr)
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INT NOT NULL PRIMARY KEY, applied_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)) ENGINE=InnoDB`); err != nil {
		return err
	}
	applied := map[int]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		base := strings.TrimPrefix(f, "migrations/")
		v, err := strconv.Atoi(strings.SplitN(base, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("Migration %s: Dateiname muss mit Nummer beginnen", f)
		}
		if applied[v] {
			continue
		}
		b, _ := migrationFS.ReadFile(f)
		for _, stmt := range splitSQL(string(b)) {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("Migration %s: %w\n%s", f, err, stmt)
			}
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, v); err != nil {
			return err
		}
		log.Printf("Datenbank-Migration %s angewendet", base)
	}
	return nil
}

// Einfaches Aufteilen an ";" am Zeilenende (die Migrationen enthalten keine Prozeduren).
func splitSQL(s string) []string {
	var out []string
	var cur strings.Builder
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "--") || t == "" {
			continue
		}
		cur.WriteString(line)
		cur.WriteString("\n")
		if strings.HasSuffix(t, ";") {
			stmt := strings.TrimSuffix(strings.TrimSpace(cur.String()), ";")
			out = append(out, stmt)
			cur.Reset()
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, cur.String())
	}
	return out
}

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}
