# Telefonbuch

Internes, webbasiertes Telefonbuch mit Schlagworten für Tätigkeiten und Besonderheiten der Mitarbeitenden. Jede Person kann einen eigenen Pixel-Kopf als Avatar gestalten (im Minecraft-Stil). Der Server ist in Go geschrieben, die Daten liegen in **MySQL oder in einer JSON-Datei**, die Anmeldung läuft über LDAPS (Active Directory). Die Stammdaten können optional aus dem AD abgeglichen werden.

Das Frontend (HTML, CSS und JavaScript ohne Build-Schritt und ohne externe Bibliotheken) ist ins Binary eingebettet. Es entsteht also **eine einzige Datei**.

## Funktionen

**Zwei-Listen-Ansicht**
- **Links** stehen die Personen mit den Spalten *Name (Nachname, Vorname)*, *Telefon*, *Mobil*, *Raum* und *Amt*.
  - Name, Telefon und Mobil teilen sich **eine** Suchleiste. Mehrere Wörter werden UND-verknüpft. Umlaute werden tolerant behandelt („mueller“ findet „Müller“). Bei Nummern zählen nur die Ziffern („456 12“ findet „0123 456-12“).
  - Raum und Amt haben **eigene** Suchfelder.
  - Ein Klick auf eine Spaltenüberschrift sortiert die Liste.
- **Rechts** stehen die Schlagworte, farblich nach **Kategorie** unterschieden (Standard: Tätigkeit, Besonderheit, Sonstiges), jeweils mit der Anzahl der Personen. Die Kategorien pflegen Administratoren selbst (siehe unten).
- **Klick auf eine Person:** Rechts erscheinen nur noch die Schlagworte dieser Person, darüber eine Visitenkarte mit drehendem Avatar. *Strg+Klick* wählt mehrere Personen.
- **Klick auf ein Schlagwort:** Links erscheinen nur noch die Personen mit diesem Schlagwort. *Strg+Klick* kombiniert mehrere Schlagworte, dann müssen alle passen.
- **Klick in die Schlagwortsuche** (oder `Esc`) setzt links und rechts alles zurück.
- Telefonnummern sind `tel:`-Links. Damit wählt ein Klick direkt über das Softphone.
- **Hilfe** gibt es im Benutzermenü (*❔ Hilfe*), für Gäste über den ❔-Knopf oder mit der Taste `?`.
- **Jede Bildschirmbreite:** Die Personenliste richtet sich nach ihrer tatsächlichen Breite (Container Queries). Breit erscheinen 5 Spalten, mittel 3 Spalten in zwei Zeilen (Raum und Amt unter Telefon und Mobil), schmal Karten mit Symbolen. Keine Angabe fällt weg. Auf dem Handy gibt es die Reiter *Personen* und *Schlagworte*: Tippt man eine Person an, wechselt die Ansicht zu ihren Schlagworten, tippt man ein Schlagwort an, zurück zu den passenden Personen.

**Rechte**

| Wer | Darf |
|---|---|
| ohne Anmeldung (Gast) | Telefonbuch durchsuchen und ansehen (Namen, Nummern, Raum, Amt, E-Mail, Schlagworte, Avatare). Abschaltbar mit `auth.publicDirectory: false` – dann ist vorher eine Anmeldung nötig |
| jede angemeldete Person | suchen; **eigene** Schlagworte und **eigenen** Avatar bearbeiten; **private Kontakte** anlegen und gezielt **freigeben** |
| Administratoren (AD-Gruppe) | zusätzlich: Stammdaten aller Einträge anlegen, ändern und löschen; Schlagworte aller Personen bearbeiten; Schlagworte umbenennen, zusammenführen und löschen; **Kategorien** verwalten; Avatare aller Einträge bearbeiten, kopieren und übertragen; AD-Abgleich starten; Protokoll einsehen |

Allgemein sichtbare Einträge können nur Administratoren anlegen.

**Private Kontakte und Freigaben**
- Ein privater Kontakt ist zunächst nur für seinen Besitzer sichtbar (🔒 *privat*).
- Unter **„🔗 Freigeben“** im Kontakt-Dialog lässt er sich für einzelne Personen (Auswahl mit Avatar und Name, Suche nach Name oder Amt) und/oder für ein ganzes **Amt** freigeben. Er heißt dann **„🔗 Privat, geteilt“**. Die Empfänger sehen ihn mit „🔗 von <Besitzer>“, **nur lesend**.
- **Bearbeiten und löschen kann nur der Besitzer.** Das prüft der Server (`owner = Benutzer`). Gäste sehen keine privaten Kontakte.
- Freigaben an ein Amt gelten für alle Einträge, deren Amt (ohne Groß-/Kleinschreibung) passt, auch für später hinzukommende. Wird ein Eintrag gelöscht, verschwinden die Freigaben an ihn.

**Schlagwort-Kategorien** (Verwaltung → *🗂 Kategorien verwalten*)
- Kategorien anlegen, umbenennen, Farbe und Reihenfolge ändern. Die Standardkategorien sind *Tätigkeit*, *Besonderheit* und *Sonstiges*.
- **Löschen mit Folge-Kategorie:** Beim Löschen wählt man, in welche Kategorie die vorhandenen Schlagworte wandern. Die letzte Kategorie lässt sich nicht löschen.
- Filterknöpfe, Farben und die Auswahl im Schlagwort-Editor richten sich automatisch nach den Kategorien.

**Avatar-Editor** (Menü → *Mein Avatar*, oder Klick auf den eigenen Avatar)
- Der Kopf hat **16×16 Pixel je Seite** (Version 2, vorher 8×8), dazu eine Überzug-Ebene und Aufbauten wie Hutkrempe, Dutt, Zöpfe, Ohren und Kopfhörer. Er wird in 3D gezeichnet und lässt sich mit der Maus drehen. Alte Avatare werden automatisch übernommen, gemalte Pixel dabei auf 16×16 vergrößert.
- **Standard ist neutral grau.** Damit gibt es keine Vorgabe zu Geschlecht oder Hautfarbe. Grautöne, Hauttöne, Fantasiefarben und eine freie Farbe stehen zur Auswahl.
- **32 Frisuren** (vom Undercut über Afro-Puffs und Flechtzöpfe bis zum Kopftuch), **18 Augenformen**, 11 Brauen, 9 Nasen, **16 Münder**, 12 Bärte, **14 Brillen** (inkl. VR-Brille und 3D-Brille), **30 Kopfbedeckungen** (Cap, Zauberhut, Wikingerhelm, Katzenohren, Heiligenschein …), 5 Ohrformen und 16 Details wie Sommersprossen, Piercings, Creolen, Mund-Nasen-Schutz oder Stift hinterm Ohr.
- Farben: Haut, Haare, **Strähnen**, Bart, Augen (auch **zwei verschiedene Augenfarben**), **Lippen**, Brillengestell und eine Akzentfarbe für Hüte und Accessoires, jeweils mit Farbfeldern und freier Farbwahl. Jede Option zeigt eine Vorschau-Kachel, wie im Charaktereditor des IT-Dioramas.
- **Malen:** Pixel direkt auf den 3D-Kopf malen, oder pixelgenau in der Abwicklung des Würfels. Zur Verfügung stehen Stift, Radierer, Pipette, „Seite füllen“, die Ebenen Kopf und Überzug sowie Rückgängig (`Strg+Z`).
- **Datensparsam:** Gemalte Pixel werden kompakt gespeichert. Eine komplett einfarbige Seite ist genau **ein** Eintrag (`"*": Farbe`), bei einer vollständig bemalten Seite steht die häufigste Farbe einmal da, und nur die abweichenden Pixel folgen einzeln. Der Editor vergleicht zusätzlich mit dem erzeugten Kopf: Pixel, die nichts ändern (z. B. „durchsichtig“ auf einem ohnehin leeren Überzug), entfallen. Sind alle Haar-Pixel einer Überzug-Seite durchsichtig gemalt, bleibt nur `"*": ""`. Der Server verdichtet beim Speichern vollständig bemalte Seiten ebenfalls. Vorhandene Avatare verdichtet er beim Start einmalig, den Abgleich mit dem erzeugten Kopf gibt es aber nur im Editor, also nach einmal Öffnen und Speichern.
- **Durchsichtig:** Auf der Ebene *Überzug* gibt es die Malfarbe ▨ „durchsichtig“, die einzelne Pixel (auch von Haaren oder Mützen) unsichtbar macht. Der **Radierer** setzt gemalte Pixel auf beiden Ebenen auf das Original zurück.
- Mit „🎲 Zufall“ und „↺ Standard“ lässt sich schnell ein neues Aussehen ausprobieren bzw. zum grauen Standard zurückkehren. **„■ Block“** erzeugt einen einfarbigen Würfel ohne Gesicht, z. B. für Einrichtungs-Einträge wie „Schule xy“. Die Farbe wählt man danach unter *Kopf → Hautfarbe*. Augen und Mund haben dafür auch die Option „Keine“, Frisuren die Option „Ohne“.
- **Kopieren & Einfügen:** „📋 Kopieren“ legt den Avatar in die Zwischenablage des Browsers, „📥 Einfügen“ setzt ihn bei einem anderen Eintrag ein.
- **Für Administratoren:** „Von Eintrag übernehmen …“ (Suche nach Name/Amt) holt den Avatar eines anderen Eintrags. „📤 Auf andere übertragen …“ speichert den aktuellen Avatar bei beliebig vielen ausgewählten Einträgen auf einmal.

**AD-Abgleich** (optional)
- Stammdaten (Name, Telefon, Mobil, Raum, Amt, E-Mail; einstellbar) werden aus dem AD übernommen. Der Abgleich läuft zeitgesteuert, per Knopf oder per `telefonbuch sync`.
- Einträge werden über die `objectGUID` zugeordnet. Umbenennungen bleiben dadurch erhalten, Schlagworte und Avatare auch.
- Felder, die aus dem AD kommen, sind bei AD-Einträgen in der Oberfläche gesperrt (🔒). Der Server setzt das ebenfalls durch.
- Konten, die im AD fehlen oder deaktiviert sind, werden deaktiviert statt gelöscht, damit ihre Schlagworte erhalten bleiben.
- **Schutz vor Fehlkonfiguration:** Würde ein Lauf mehr als `maxDeactivatePercent` der AD-Einträge deaktivieren, bricht er ab. Das fängt etwa einen falschen Filter oder eine falsche OU ab.

## Datenhaltung: MySQL oder JSON-Datei

Das Telefonbuch kennt zwei gleichwertige Datenhaltungen. Alle Funktionen gibt es in beiden: Rechte, Schlagworte, Avatare, private Kontakte, AD-Abgleich und Protokoll.

| `storage.mode` | Daten liegen in | Typischer Einsatz |
|---|---|---|
| `mysql` (Standard) | MySQL/MariaDB | Normalbetrieb |
| `json` | einer Datei (`storage.jsonPath`) | ohne Datenbank, Notbetrieb, Offline-Laptop, kleine Standorte |

**Die JSON-Datei**
- **Atomar:** Nach jeder Änderung wird die Datei komplett neu geschrieben, erst in eine temporäre Datei, dann `fsync` und Umbenennen. Ein Absturz hinterlässt also nie eine halbe Datei.
- **Rechte `0600`:** Die Datei enthält auch private Kontakte und die Passwort-Hashes lokaler Konten.
- **Lesbar:** Sie ist eingerückt und lässt sich notfalls von Hand prüfen. Beim Laden wird die Struktur geprüft, eine kaputte Datei wird abgelehnt statt überschrieben.
- **Sitzungen** liegen nur im Speicher. Nach einem Neustart meldet man sich neu an.
- **Das Protokoll liegt in einer eigenen Datei** (Standard: `telefonbuch.audit.json` neben der Datendatei, einstellbar mit `storage.jsonAuditPath`). Ein Protokolleintrag schreibt also nur diese kleine Datei, die Datendatei bleibt unberührt. Sie behält die letzten 5000 Einträge, die AD-Abgleiche die letzten 50 (diese stehen in der Datendatei). Ältere Dateien mit Protokoll in der Datendatei werden beim Start automatisch umgestellt. Kopien und Exporte aus MySQL schreiben ebenfalls beide Dateien.
- **Nur eine Instanz** darf gleichzeitig mit derselben Datei arbeiten.

### MySQL → JSON (zweite Datenhaltung / Offline-Reserve)

Es gibt drei Wege, die JSON-Datei mit den Daten aus MySQL zu befüllen:

1. **Automatische Kopie im MySQL-Betrieb:** `"jsonSnapshotInterval": "1h"` schreibt beim Start und dann stündlich eine vollständige Kopie nach `storage.jsonPath`. Fällt die Datenbank aus, greift automatisch der **Rückfall-Betrieb** (siehe unten). Auch hier gilt der Schutz: Enthält MySQL plötzlich deutlich weniger Einträge als die vorhandene Kopie (`mysqlImportMinPercent`), wird die Kopie **nicht** überschrieben.
2. **Befehl:** `telefonbuch export-json -config config.json [-out datei.json]` liest MySQL und schreibt die Datei. Die vorherige Datei wird dabei als `.bak` gesichert.
3. **Im JSON-Betrieb übernehmen:** Mit `"mysqlImport": "manual"` erscheint unter *Verwaltung → 💾 Datenhaltung* der Knopf **„Aus MySQL übernehmen“**. Mit `"startup"` passiert das zusätzlich bei jedem Start, mit `"mysqlImportInterval": "1h"` regelmäßig. Ist MySQL nicht erreichbar, läuft das Telefonbuch unverändert mit der vorhandenen Datei weiter. Genau das ist der Offline-Fall.

**Wichtig:** Eine Übernahme **ersetzt** den Inhalt der JSON-Datei vollständig: Einträge, Schlagworte, Avatare, private Kontakte, lokale Konten und Protokoll. Änderungen, die nur in der JSON-Datei gemacht wurden, landen in `<datei>.bak`. Als Schutz bricht die Übernahme ab, wenn MySQL **leer** ist oder weniger als `mysqlImportMinPercent` (Standard 50 %) der Einträge der Datei enthält. Das fängt etwa eine neu aufgesetzte oder falsch konfigurierte Datenbank ab. Im Dialog lässt sich der zweite Fall nach Rückfrage bewusst übergehen.

### Rückfall-Betrieb: MySQL fällt aus

Im MySQL-Modus mit vorhandener JSON-Kopie (`storage.fallbackToJSON`, Standard `true`) gilt:
- Ist MySQL **beim Start** nicht erreichbar, startet das Telefonbuch trotzdem. Es zeigt die JSON-Kopie **nur lesend** an, die Datei selbst bleibt unverändert.
- Oben erscheint dann das Label **„📄 Offline-Bestand“** mit dem Stand der Kopie. **Nur in diesem Fall.** Im normal konfigurierten JSON-Modus (`storage.mode: "json"`) erscheint es nicht.
- Anmelden geht weiterhin. Alle Änderungen werden mit einem klaren Hinweis abgelehnt (HTTP 503).
- Alle `fallbackRetry` (Standard `30s`) wird MySQL erneut versucht. Ist die Datenbank wieder da, schaltet das Telefonbuch **ohne Neustart** zurück. Angemeldete Benutzer bleiben dabei angemeldet.

`"fallbackToJSON": false` schaltet das ab. Dann bricht der Start wie bisher ab, wenn die Datenbank fehlt.

### JSON → MySQL (zurück nach dem Offline-Betrieb)

```bash
./telefonbuch import-json -config config.json -in telefonbuch.json -force
```

Dieser Befehl ersetzt **alle** Daten in MySQL durch den Inhalt der Datei. Ohne `-force` zeigt er nur an, was er tun würde. Den Server dabei am besten anhalten.

Beispiel für den Offline-Betrieb:

```json
"storage": { "mode": "json", "jsonPath": "/var/lib/telefonbuch/telefonbuch.json", "mysqlImport": "startup" }
```

Die `db`-Einstellungen bleiben für die Übernahme stehen. Im JSON-Modus braucht das Telefonbuch die Datenbank ansonsten nicht.

## Schnell ausprobieren (Demo)

Voraussetzung: Go ≥ 1.24. Für den Demo-Modus genügt eine JSON-Datei, eine Datenbank ist nicht nötig:

```bash
go mod tidy
TB_STORAGE=json TB_JSON_PATH=./demo.json TB_DEMO_PASSWORD=demo-passwort-123 go run . -config "" -demo
# oder mit MySQL (Datenbank und Benutzer anlegen, siehe unten):
TB_DB_PASSWORD=... TB_DEMO_PASSWORD=demo-passwort-123 go run . -config "" -demo
```

`-demo` legt 64 Beispieleinträge mit Schlagworten an und schaltet die lokale Anmeldung ein. Es gibt zwei Konten: `admin` (Administrator) und `mmuster` (normaler Benutzer, mit dem Eintrag „Muster, Max“ verknüpft). Das Passwort kommt aus `TB_DEMO_PASSWORD`. Ist die Variable nicht gesetzt, wird ein zufälliges Passwort im Log ausgegeben.

## Betrieb

### 1. Datenbank

```sql
CREATE DATABASE telefonbuch CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'telefonbuch'@'%' IDENTIFIED BY '…langes-zufälliges-passwort…';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, REFERENCES ON telefonbuch.* TO 'telefonbuch'@'%';
```

Die Tabellen legt das Programm beim Start selbst an (eingebettete Migrationen). Eine verschlüsselte Verbindung zur Datenbank aktivierst du mit `db.tls` (`"true"`, `"skip-verify"` oder `"custom"` zusammen mit `db.caFile`).

### 2. Konfiguration

Kopiere `config.example.json` nach `config.json` und passe sie an. **Passwörter gehören nicht in die Datei**, sondern in die Umgebung:

| Variable | Bedeutung |
|---|---|
| `TB_DB_PASSWORD` bzw. `TB_DB_PASSWORD_FILE` | Passwort der Datenbank |
| `TB_LDAP_BIND_PASSWORD` bzw. `TB_LDAP_BIND_PASSWORD_FILE` | Passwort des LDAP-Dienstkontos |
| `TB_DEV_ADMIN_PASSWORD` | Passwort des DEV-Admins (nur mit `devAdmin`) |
| `TB_DB_DSN` | alternativ eine vollständige DSN (`user:pw@tcp(host:3306)/telefonbuch`) |
| `TB_ADDR`, `TB_DB_HOST`, `TB_DB_USER`, `TB_DB_NAME`, `TB_LDAP_URL`, `TB_LDAP_BIND_DN`, `TB_AUTH_MODE`, `TB_STORAGE`, `TB_JSON_PATH`, `TB_PUBLIC_DIRECTORY`, `TB_DEV_ADMIN`, `TB_STORAGE_FALLBACK` | überschreiben die Werte aus der Datei |

**LDAPS:**
- `ldap.url` muss `ldaps://` sein (oder `ldap://` mit `startTLS: true`). Unverschlüsseltes LDAP lehnt das Programm ab.
- `ldap.caFile` zeigt auf das Root-Zertifikat eurer internen CA, mit der die Domänencontroller-Zertifikate ausgestellt sind. `insecureSkipVerify` ist nur zum Testen gedacht und erzeugt beim Start eine deutliche Warnung.
- Das **Dienstkonto** (`bindDN`) braucht nur Leserechte, normale Domänenbenutzer genügen. Ohne Dienstkonto geht es auch: Dann setzt du `userBindTemplate` (z. B. `"{username}@firma.local"`). In diesem Fall ist kein AD-Abgleich möglich.
- **Administratoren** sind die Mitglieder von `adminGroupDN`, mit `nestedGroups: true` auch über verschachtelte Gruppen (`LDAP_MATCHING_RULE_IN_CHAIN`). Mit `userGroupDN` kannst du die Anmeldung auf eine Gruppe beschränken.
- Angemeldet wird man mit `mmuster`, `FIRMA\mmuster` oder `mmuster@firma.local`.
- Ist `autoCreateOnLogin` aktiv, wird beim ersten Login ein Eintrag aus den AD-Attributen angelegt, falls noch keiner existiert. So kann jede Person sofort ihre Schlagworte pflegen.

**Ohne Anmeldung lesbar:** Standardmäßig (`auth.publicDirectory: true`) öffnet sich das Telefonbuch direkt. Oben rechts steht „🔑 Anmelden“, und erst zum Bearbeiten meldet man sich an. Gäste bekommen keine Benutzernamen, keine privaten Kontakte und keine ausgeblendeten oder inaktiven Einträge zu sehen. Jede ändernde Anfrage verlangt weiterhin eine Anmeldung. Das prüft der Server.

**DEV-Admin (AD umgehen, nur Entwicklung/Test):** Mit `auth.devAdmin: true`, dem Schalter `-dev-admin` oder `TB_DEV_ADMIN=1` legt das Programm beim Start ein lokales Admin-Konto `devadmin` an (Name über `auth.devAdminUser` änderbar).
- **Kein AD-Kontakt:** Für dieses Konto wird nie das AD gefragt. Fehlversuche führen dort also auch nicht zu Sperren.
- **Passwort:** kommt aus `TB_DEV_ADMIN_PASSWORD` (mindestens 12 Zeichen). Fehlt die Variable, wird bei jedem Start ein neues zufälliges Passwort erzeugt und im Log ausgegeben.
- **Nur dieses Konto:** Andere lokale Konten bleiben gesperrt, solange `auth.mode` nicht `local` enthält. Das AD für alle anderen läuft normal weiter.
- **Ohne LDAP-Konfiguration** schaltet das Programm automatisch auf rein lokale Anmeldung.
- **Unübersehbar:** Solange der DEV-Admin aktiv ist, zeigen Log, Kopfzeile und Anmeldedialog eine deutliche Warnung. **Im Betrieb ausschalten.**

```bash
TB_DEV_ADMIN_PASSWORD='mein-dev-passwort' ./telefonbuch -config config.json -dev-admin
```

**Notfallzugang:** Mit `auth.mode: "ldap+local"` funktionieren zusätzlich lokale Konten, z. B. falls das AD nicht erreichbar ist:

```bash
TB_PASSWORD='mindestens-12-zeichen' ./telefonbuch useradd -config config.json -admin -name "Notfall-Admin" notfalladmin
```

### Docker-Image aus GitHub (Workflow)

`.github/workflows/docker.yml` baut bei jedem Push auf `main`, bei Tags `v*.*.*` und bei Pull Requests:

1. **Tests:** `gofmt`, `go vet` und `go test -race`, auch gegen einen echten MySQL-8.4-Dienst (`TB_TEST_DSN`).
2. **Image** für `linux/amd64` und `linux/arm64`. Go baut dabei quer, ohne Emulation. Ergebnis ist ein schlankes Distroless-Image ohne Shell, als Benutzer `nonroot`, mit eingebautem `HEALTHCHECK` (`telefonbuch healthcheck`).
3. **Veröffentlichung** in der GitHub Container Registry: `ghcr.io/<owner>/<repo>` mit den Tags `latest`, `main`, `sha-…` bzw. `1.2.3`, `1.2`, `1`. Dazu kommen SBOM und Herkunftsnachweis (Attestation). Pull Requests werden nur gebaut, nicht veröffentlicht.

Es sind keine Secrets nötig, der Workflow nutzt `GITHUB_TOKEN`. Bei privaten Repositories ist das Paket anfangs privat: unter *Packages → Package settings* die Sichtbarkeit bzw. den Zugriff freigeben. Die Versionsnummer kommt aus dem Git-Tag (`telefonbuch version`).

### 3. Starten

```bash
go build -o telefonbuch .
./telefonbuch -config /etc/telefonbuch/config.json
```

Oder mit Docker: `.env` mit `TB_DB_PASSWORD`, `MYSQL_ROOT_PASSWORD` und `TB_LDAP_BIND_PASSWORD` anlegen, dann `docker compose up -d --build`.

**HTTPS ist Pflicht.** Entweder bedient das Programm HTTPS selbst (`tls.certFile`/`tls.keyFile`), oder es läuft hinter einem Reverse-Proxy. Im zweiten Fall trägst du die Adresse des Proxys in `trustedProxies` ein, damit `X-Forwarded-Proto`/`-For` ausgewertet werden. Beispiel nginx:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 600s;   # manueller AD-Abgleich
}
```

Health-Check: `GET /healthz`.

### Befehle

| Befehl | Zweck |
|---|---|
| `telefonbuch -config config.json` | Server starten (`-dev`: Frontend aus `./web` statt aus dem Binary, `-demo`: Beispieldaten, `-dev-admin`: lokales Admin-Konto am AD vorbei) |
| `telefonbuch useradd [-admin] [-name …] BENUTZER` | lokales Konto anlegen bzw. Passwort setzen (Passwort aus `TB_PASSWORD` oder von stdin) |
| `telefonbuch sync -config config.json` | AD-Abgleich einmalig ausführen (z. B. per Cron statt Intervall) |
| `telefonbuch export-json [-out datei] [-force]` | MySQL → JSON-Datei + Protokolldatei (siehe *Datenhaltung*) |
| `telefonbuch healthcheck [-url …]` | Prüft `/healthz` (für Docker-HEALTHCHECK), Exit-Code 0/1 |
| `telefonbuch import-json [-in datei] -force` | JSON-Datei → MySQL (ersetzt alles in MySQL) |

## Windows-Client: Symbol im Infobereich

Der Server läuft zentral (Docker). Auf den Windows-Clients sorgt ein kleines Symbol im Infobereich dafür, dass das Telefonbuch jederzeit einen Klick entfernt ist:

- **Klick:** Das Telefonbuch öffnet sich in einem **eigenen Fenster** (Edge/Chrome im App-Modus, ohne Adressleiste). Ist es schon offen, wird es nach vorne geholt.
- **Rechtsklick → „Suchen …“:** Ein kleines Suchfeld unten rechts. Mit `Enter` wird gesucht. Ein schon offenes Telefonbuch-Fenster übernimmt die Suche, es entsteht kein zweites Fenster.
- **Rechtsklick → „Mit Windows starten“** schaltet den Autostart um, „Beenden“ schließt das Symbol.
- Ein Start über das Startmenü öffnet das Telefonbuch. Läuft das Symbol schon, wird kein zweites angelegt.

**Bewusst ohne Programm:** Der Client besteht nur aus **PowerShell-Skripten** (Windows PowerShell 5.1, ist auf jedem Windows 10/11 vorhanden). Es gibt nichts zu kompilieren, keine EXE und keinen MSI. Das Symbol wird zur Laufzeit gezeichnet. Installiert wird pro Benutzer nach `%LOCALAPPDATA%\Programs\Telefonbuch`, ohne Administratorrechte.

| Datei (in `client/windows/`) | Zweck |
|---|---|
| `Telefonbuch-Tray.ps1` | das Symbol selbst |
| `Install-Telefonbuch.ps1` | Installation/Deinstallation pro Benutzer: Verknüpfungen in Startmenü und Autostart, Einstellungen unter `HKCU:\Software\Telefonbuch` |
| `install.cmd` / `uninstall.cmd` | zum Doppelklicken |

### Verteilen

**Zum Herunterladen:** Der Server liefert unter `/client/telefonbuch-windows.zip` ein ZIP aus, in dessen `install.cmd` die Adresse des Telefonbuchs schon eingetragen ist. Grundlage ist `publicURL` aus der Konfiguration, sonst die aufgerufene Adresse. In der Oberfläche steht der Link unter *Benutzermenü → 💻 Windows-Symbol* und in der Hilfe. Benutzer entpacken das ZIP und doppelklicken `install.cmd`.

**Per Gruppenrichtlinie / Intune** (empfohlen):
1. Die Adresse als Richtlinie setzen (GPO → Computer- oder Benutzerkonfiguration → Einstellungen → Registrierung):
   `HKLM\SOFTWARE\Policies\Telefonbuch` → `Url` (REG_SZ) = `https://telefonbuch.firma.local/`, optional `Mode` = `app` oder `browser`.
2. Den Ordner `client/windows` auf eine Freigabe legen, z. B. `\\firma.local\NETLOGON\Telefonbuch`.
3. Als Anmeldeskript eintragen:
   `powershell.exe -NoProfile -ExecutionPolicy Bypass -File \\firma.local\NETLOGON\Telefonbuch\Install-Telefonbuch.ps1 -Quiet`
   Ist dieselbe Version schon installiert, beendet sich das Skript sofort. Ein Update geschieht durch Austauschen der Dateien auf der Freigabe.

Weitere Parameter von `Install-Telefonbuch.ps1`: `-Url`, `-Mode app|browser`, `-NoAutostart`, `-NoStart`, `-Launcher headless|powershell`, `-Uninstall`.

### Ausführungsrichtlinie, Signatur und Virenscanner

- Die Verknüpfungen starten PowerShell mit `-ExecutionPolicy Bypass`, und der Installer entfernt die Download-Markierung (`Unblock-File`). Unter den Windows-Standardeinstellungen und bei `RemoteSigned` ist **keine Signatur nötig**.
- **Erzwingt eine GPO `AllSigned`** (Richtlinie *„Skriptausführung aktivieren“*), hat sie Vorrang vor `-ExecutionPolicy`. Dann müssen die beiden `.ps1` einmalig mit dem Codesignatur-Zertifikat der internen CA signiert werden:
  `Set-AuthenticodeSignature .\Telefonbuch-Tray.ps1 -Certificate (Get-ChildItem Cert:\CurrentUser\My -CodeSigningCert)`. Dasselbe gilt für `Install-Telefonbuch.ps1`. Das ist kein Kompilieren, nur eine Signaturzeile am Dateiende.
- **AppLocker/WDAC mit „Constrained Language Mode“** verhindert Windows-Forms-Skripte grundsätzlich. Dort brauchen die Skripte eine Freigabe über Pfad- oder Herausgeberregel bzw. eine Signatur.
- **Kein Konsolenfenster:** Gestartet wird über `conhost.exe --headless powershell.exe …` (ab Windows 10 1809). Das funktioniert auch, wenn Windows Terminal als Standardterminal eingestellt ist. Manche EDR-Produkte stufen `conhost --headless` als auffällig ein. Dann mit `-Launcher powershell` installieren: Dabei kann beim Start kurz ein Fenster aufblitzen.

**Technik:** Eine Suche aus dem Infobereich öffnet `…/?q=<Suchbegriff>`. Die Seite meldet sich per `BroadcastChannel` bei einem schon offenen Telefonbuch-Fenster. Übernimmt dieses die Suche, schließt sich das neue Fenster wieder, und das Symbol holt das vorhandene nach vorne. Erkannt wird es am Fenstertitel „Telefonbuch“ eines Browserprozesses. Der Link `…/?q=Müller` funktioniert auch so, etwa als Lesezeichen oder aus anderen Programmen.

## Sicherheit

- **Anmeldung:** LDAP-Bind über TLS mit Zertifikatsprüfung. Leere Passwörter werden abgelehnt (kein „unauthenticated bind“). Benutzernamen werden geprüft und für LDAP-Filter escaped.
- **Brute-Force-Schutz:** Nach `maxFailsPerUser` Fehlversuchen pro Benutzer bzw. `maxFailsPerIP` pro IP wird `lockDuration` lang gesperrt. Fehlversuche landen im Protokoll.
- **Sitzungen:** 256-Bit-Zufallstoken. In der Datenbank liegt nur der SHA-256-Hash. Das Cookie ist `HttpOnly`, `Secure` und `SameSite=Strict` (unter HTTPS `__Host-`-Präfix), mit Leerlauf-Timeout (`sessionIdle`) und Höchstdauer (`sessionMax`). Beim Login wird eine neue Sitzung erzeugt.
- **CSRF:** Jede ändernde Anfrage braucht das Sitzungs-CSRF-Token im Header, `Content-Type: application/json` und eine passende `Origin`.
- **Rechte** prüft immer der Server: eigene Schlagworte und eigener Avatar nur über die Verknüpfung Konto ↔ Eintrag, private Kontakte nur über `owner = Benutzer` (Freigaben sind nur lesend, Empfänger werden gegen den Bestand geprüft), Stammdaten und Kategorien nur für Admins, AD-Felder gesperrt.
- **Eingaben:** Alle SQL-Abfragen sind parametrisiert. Längen, Zeichen, Telefonnummern und E-Mail werden geprüft. Die Avatar-Daten prüft der Server streng (nur bekannte Felder, Hex-Farben, Pixelindizes 0–63, Größenlimit).
- **Browser:** Strikte Content-Security-Policy (kein Inline-Script, kein `eval`, keine fremden Quellen), dazu `X-Frame-Options: DENY`, `nosniff` und HSTS unter HTTPS. Das Frontend baut das DOM ohne `innerHTML`, damit ist XSS über Namen oder Schlagworte ausgeschlossen.
- **Protokoll** (Admin): Anmeldungen, Fehlversuche, Änderungen an Stammdaten und Schlagworten sowie AD-Abgleiche. Private Kontakte werden bewusst nicht protokolliert.
- **Datenbank:** eigener Benutzer mit minimalen Rechten, optional TLS.
- **JSON-Datei:** Rechte `0600`, Verzeichnis `0700`. Die Datei (und ihre `.bak`) gehört zu den schützenswerten Daten und ist z. B. im Backup verschlüsselt abzulegen.

## Aufbau

```
main.go        Start, Routen, Sicherheits-Header, Befehle useradd/sync
config.go      Konfiguration (JSON + Umgebungsvariablen) und Prüfung
store.go       Schnittstelle der Datenhaltung + Datenformat (Snapshot)
store_mysql.go Datenhaltung MySQL
store_json.go  Datenhaltung JSON-Datei + separate Protokolldatei (atomares Schreiben)
storage.go     MySQL → JSON (Kopie, Übernahme, Schutzgrenze), Admin-Dialog
fallback.go    Rückfall-Betrieb: JSON-Kopie nur lesend, automatisches Zurückschalten
db.go          MySQL-Verbindung, eingebettete Migrationen (migrations/*.sql)
auth.go        Sitzungen, CSRF, Login/Logout, Rate-Limit, lokale Konten (PBKDF2)
ldap.go        LDAPS: Bind, Benutzersuche, Gruppenprüfung, seitenweise Suche
adsync.go      AD-Abgleich
api.go         REST-API: Verzeichnis, Schlagworte, Stammdaten, private Kontakte, Protokoll
avatar.go      Prüfung der Avatar-Daten
seed.go        Demo-Daten
client.go      Download des Windows-Clients (ZIP mit eingetragener Adresse)
client/windows Windows-Client: Symbol im Infobereich (PowerShell, ohne Kompilat)
web/           Frontend (index.html, css/app.css, js/*.js)
  js/avatar.js   Pixel-Kopf 16×16: Textur-Generator, 3D-Renderer (Texturen, tiefensortierte Kacheln), Vorschaubilder
  js/editor.js   Avatar-Editor
  js/app.js      Zwei-Listen-Ansicht, Dialoge, Verwaltung (Kategorien, Freigaben)
  js/picker.js   Auswahl von Personen (Avatar + Name) und Ämtern
```

### API (Auszug)

| Methode & Pfad | Recht |
|---|---|
| `POST /api/login`, `GET /api/site`, `GET /client/telefonbuch-windows.zip` | – |
| `POST /api/logout`, `GET /api/me` | angemeldet |
| `GET /api/directory` (Einträge, Schlagworte, Kategorien; eigene und für mich freigegebene private Kontakte nur angemeldet) | Gast (wenn `publicDirectory`) / angemeldet |
| `PUT /api/employees/{id}/tags`, `PUT /api/employees/{id}/avatar` | eigener Eintrag oder Admin |
| `POST/PUT/DELETE /api/employees[/{id}]` | Admin |
| `POST/PUT/DELETE /api/private[/{id}]` (mit `shares: {employees: [IDs], amts: [Ämter]}`) | angemeldet (nur eigene) |
| `POST /api/categories`, `PUT /api/categories/{key}`, `POST /api/categories/{key}/delete` (`{moveTo}`) | Admin |
| `GET/POST /api/tags`, `PUT/DELETE /api/tags/{id}`, `POST /api/tags/{id}/merge`, `POST /api/tags/cleanup` | Admin |
| `GET/POST /api/admin/sync`, `POST /api/admin/ldap-test`, `GET /api/admin/audit` | Admin |
| `GET /api/admin/storage`, `POST /api/admin/storage/import-mysql`, `POST /api/admin/storage/snapshot` | Admin |
