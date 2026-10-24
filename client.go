package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

// Windows-Client (Symbol im Infobereich): reine PowerShell-Skripte, als ZIP zum Herunterladen.
// Die Adresse des Telefonbuchs wird beim Download in install.cmd eingetragen.

//go:embed client/windows
var clientFS embed.FS

const clientURLPlaceholder = "__TELEFONBUCH_URL__"

// Nur unkritische Zeichen – die Adresse landet in einer .cmd-Datei
var reClientURL = regexp.MustCompile(`^https?://[A-Za-z0-9.\-]+(:[0-9]{1,5})?(/[A-Za-z0-9._~\-/]*)?$`)

// baseURL: konfigurierte publicURL, sonst aus der Anfrage abgeleitet
func (a *App) baseURL(r *http.Request) string {
	if a.cfg.PublicURL != "" {
		return strings.TrimRight(a.cfg.PublicURL, "/") + "/"
	}
	scheme := "http"
	if a.isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/"
}

func buildClientZip(url string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	now := time.Now()
	err := fs.WalkDir(clientFS, "client/windows", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := clientFS.ReadFile(p)
		if err != nil {
			return err
		}
		name := path.Base(p)
		// Zeilenenden für Windows sicherstellen (falls Git sie umgewandelt hat)
		b = bytes.ReplaceAll(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))
		if strings.HasSuffix(name, ".cmd") {
			b = bytes.ReplaceAll(b, []byte(clientURLPlaceholder), []byte(url))
		}
		// Windows PowerShell 5.1 liest Skripte ohne BOM als ANSI – Umlaute brauchen das UTF-8-BOM
		if (strings.HasSuffix(name, ".ps1") || strings.HasSuffix(name, ".txt")) && !bytes.HasPrefix(b, []byte("\xef\xbb\xbf")) {
			b = append([]byte("\xef\xbb\xbf"), b...)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: "Telefonbuch/" + name, Method: zip.Deflate, Modified: now})
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (a *App) handleClientZip(w http.ResponseWriter, r *http.Request) {
	url := a.baseURL(r)
	if !reClientURL.MatchString(url) {
		url = "" // Installationsskript fragt dann nach der Adresse
	}
	b, err := buildClientZip(url)
	if err != nil {
		http.Error(w, "Fehler", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="Telefonbuch-Windows.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}
