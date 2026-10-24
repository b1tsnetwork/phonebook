<#
.SYNOPSIS
  Installiert das Telefonbuch-Symbol für den angemeldeten Benutzer (ohne Administratorrechte).

.DESCRIPTION
  - kopiert Telefonbuch-Tray.ps1 nach %LOCALAPPDATA%\Programs\Telefonbuch
  - legt eine Startmenü-Verknüpfung „Telefonbuch“ und (optional) eine Autostart-Verknüpfung an
  - speichert Adresse und Modus unter HKCU:\Software\Telefonbuch
  - startet das Symbol im Infobereich

  Verteilung per GPO/Intune: Adresse zentral als Richtlinie setzen
  (HKLM:\SOFTWARE\Policies\Telefonbuch, Wert "Url") und dieses Skript mit -Quiet im Anmeldeskript aufrufen.

.EXAMPLE
  powershell -NoProfile -ExecutionPolicy Bypass -File Install-Telefonbuch.ps1 -Url https://telefonbuch.firma.local
.EXAMPLE
  powershell -NoProfile -ExecutionPolicy Bypass -File Install-Telefonbuch.ps1 -Uninstall
#>
[CmdletBinding()]
param(
  [string]$Url,
  [ValidateSet('app', 'browser')][string]$Mode = 'app',
  [ValidateSet('headless', 'powershell')][string]$Launcher = 'headless',
  [switch]$NoAutostart,
  [switch]$NoStart,
  [switch]$Uninstall,
  [switch]$Quiet
)

Set-StrictMode -Version 2
$ErrorActionPreference = 'Stop'

$dest = Join-Path $env:LOCALAPPDATA 'Programs\Telefonbuch'
$tray = Join-Path $dest 'Telefonbuch-Tray.ps1'
$ico = Join-Path $dest 'Telefonbuch.ico'
$regKey = 'HKCU:\Software\Telefonbuch'
$startMenuLnk = Join-Path ([Environment]::GetFolderPath('Programs')) 'Telefonbuch.lnk'
$startupLnk = Join-Path ([Environment]::GetFolderPath('Startup')) 'Telefonbuch.lnk'

function Say([string]$Text) { if (-not $Quiet) { Write-Host $Text } }

function Stop-TbTray {
  try {
    Get-CimInstance Win32_Process -Filter "Name='powershell.exe'" -ErrorAction Stop |
      Where-Object { $_.ProcessId -ne $PID -and $_.CommandLine -and $_.CommandLine -like '*Telefonbuch-Tray.ps1*' -and $_.CommandLine -notlike '*-ExportIcon*' } |
      ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
  } catch { }
}

# ---------------------------------------------------------------- Deinstallation

if ($Uninstall) {
  Stop-TbTray
  Remove-Item -LiteralPath $startMenuLnk, $startupLnk -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $dest -Recurse -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $regKey -Recurse -ErrorAction SilentlyContinue
  Say 'Telefonbuch-Symbol entfernt.'
  exit 0
}

# ---------------------------------------------------------------- Adresse prüfen

$Url = ([string]$Url).Trim()
if ($Url -like '__*__') { $Url = '' }   # Platzhalter aus install.cmd nicht ersetzt
$policyUrl = ''
try { $policyUrl = [string](Get-ItemProperty -Path 'HKLM:\SOFTWARE\Policies\Telefonbuch' -Name Url -ErrorAction Stop).Url } catch { }
if (-not $Url) {
  try { $Url = [string](Get-ItemProperty -Path $regKey -Name Url -ErrorAction Stop).Url } catch { }
}
if (-not $Url -and -not $policyUrl) {
  if ($Quiet) { Write-Error 'Keine Adresse angegeben (-Url) und keine Richtlinie gesetzt.'; exit 1 }
  $Url = (Read-Host 'Adresse des Telefonbuchs (z. B. https://telefonbuch.firma.local)').Trim()
}
if ($Url -and $Url -notmatch '^https?://[^\s"''<>]+$') { Write-Error "Ungültige Adresse: $Url"; exit 1 }

# ---------------------------------------------------------------- Dateien

$src = Join-Path $PSScriptRoot 'Telefonbuch-Tray.ps1'
if (-not (Test-Path -LiteralPath $src)) { Write-Error "Telefonbuch-Tray.ps1 fehlt neben dem Installationsskript ($PSScriptRoot)."; exit 1 }

# Anmeldeskript (-Quiet): nichts tun, wenn dieselbe Version schon installiert ist
if ($Quiet -and (Test-Path -LiteralPath $tray) -and (Test-Path -LiteralPath $startMenuLnk) -and
    ((Get-FileHash -LiteralPath $src).Hash -eq (Get-FileHash -LiteralPath $tray).Hash)) {
  if ($Url) { New-Item -Path $regKey -Force | Out-Null; Set-ItemProperty -Path $regKey -Name Url -Value $Url }
  exit 0
}

Stop-TbTray
New-Item -ItemType Directory -Path $dest -Force | Out-Null
Copy-Item -LiteralPath $src -Destination $tray -Force
# Download-Markierung (Zone.Identifier) entfernen, sonst verweigert RemoteSigned die Ausführung
Unblock-File -LiteralPath $tray -ErrorAction SilentlyContinue

$ps = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
& $ps -NoProfile -ExecutionPolicy Bypass -File $tray -ExportIcon $ico

New-Item -Path $regKey -Force | Out-Null
if ($Url) { Set-ItemProperty -Path $regKey -Name Url -Value $Url }
Set-ItemProperty -Path $regKey -Name Mode -Value $Mode

# ---------------------------------------------------------------- Verknüpfungen

# Ohne sichtbares Konsolenfenster: conhost --headless (ab Windows 10 1809); sonst PowerShell versteckt
$conhost = Join-Path $env:SystemRoot 'System32\conhost.exe'
if ($Launcher -eq 'headless' -and (([Environment]::OSVersion.Version.Build -lt 17763) -or -not (Test-Path -LiteralPath $conhost))) { $Launcher = 'powershell' }
$psArgs = "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$tray`""
if ($Launcher -eq 'headless') {
  $target = $conhost
  $arguments = "--headless `"$ps`" $psArgs"
} else {
  $target = $ps
  $arguments = $psArgs
}

$sh = New-Object -ComObject WScript.Shell
foreach ($item in @(@{ Path = $startMenuLnk; Extra = '' }, @{ Path = $startupLnk; Extra = ' -Background' })) {
  if ($item.Path -eq $startupLnk -and $NoAutostart) { Remove-Item -LiteralPath $startupLnk -ErrorAction SilentlyContinue; continue }
  $l = $sh.CreateShortcut($item.Path)
  $l.TargetPath = $target
  $l.Arguments = $arguments + $item.Extra
  $l.WorkingDirectory = $dest
  $l.IconLocation = "$ico,0"
  $l.Description = 'Internes Telefonbuch'
  $l.WindowStyle = 7   # minimiert
  $l.Save()
}

Say "Installiert nach $dest"
if ($Url) { Say "Adresse: $Url" } else { Say "Adresse aus Richtlinie: $policyUrl" }
if (-not $NoAutostart) { Say 'Startet automatisch mit Windows.' }

if (-not $NoStart) {
  Start-Process -FilePath $target -ArgumentList ($arguments + ' -Background -Welcome') -WindowStyle Hidden
  Say 'Das Symbol erscheint im Infobereich (ggf. unter ^ ausgeblendete Symbole – dort in die Taskleiste ziehen).'
}
exit 0
