package main

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestClientZip(t *testing.T) {
	for _, u := range []string{"https://telefonbuch.firma.local/", "http://10.0.0.5:8080/tb/"} {
		if !reClientURL.MatchString(u) {
			t.Fatalf("gültige Adresse abgelehnt: %s", u)
		}
	}
	for _, u := range []string{`https://x"&calc`, "https://x/%2e", "https://x/a b", "ftp://x/", "https://x/?a=1&b=2"} {
		if reClientURL.MatchString(u) {
			t.Fatalf("unsichere Adresse akzeptiert: %s", u)
		}
	}
	b, err := buildClientZip("https://tb.example/")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		c, _ := io.ReadAll(rc)
		rc.Close()
		seen[f.Name] = string(c)
	}
	for _, n := range []string{"Telefonbuch/Telefonbuch-Tray.ps1", "Telefonbuch/Install-Telefonbuch.ps1", "Telefonbuch/install.cmd", "Telefonbuch/uninstall.cmd"} {
		if _, ok := seen[n]; !ok {
			t.Fatalf("%s fehlt", n)
		}
	}
	cmd := seen["Telefonbuch/install.cmd"]
	if !strings.Contains(cmd, `set "TB_URL=https://tb.example/"`) || strings.Contains(cmd, clientURLPlaceholder) {
		t.Fatalf("Adresse nicht eingetragen:\n%s", cmd)
	}
	if strings.Contains(strings.ReplaceAll(cmd, "\r\n", ""), "\n") {
		t.Fatal("install.cmd ohne CRLF")
	}
	if !strings.HasPrefix(seen["Telefonbuch/Telefonbuch-Tray.ps1"], "\xef\xbb\xbf") {
		t.Fatal("PowerShell-Skript ohne UTF-8-BOM")
	}
}
