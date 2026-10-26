<#
.SYNOPSIS
  Telefonbuch im Infobereich (Notify-Icon) für Windows-Clients.

.DESCRIPTION
  Reines PowerShell-Skript (Windows PowerShell 5.1, .NET Framework) – kein Kompilat, kein Installer.
  - Linksklick auf das Symbol: Telefonbuch öffnen bzw. ein offenes Fenster nach vorne holen
  - Rechtsklick: Öffnen, Suchen …, Mit Windows starten, Beenden
  - Ein zweiter Start (z. B. über das Startmenü) öffnet nur das Telefonbuch.

  Die Adresse kommt (in dieser Reihenfolge) aus -Url, aus der Richtlinie
  HKLM:\SOFTWARE\Policies\Telefonbuch (Wert "Url", z. B. per GPO) oder aus HKCU:\Software\Telefonbuch.
  "Mode" = "app" (eigenes Fenster über Edge/Chrome, Standard) oder "browser" (Standardbrowser).

.PARAMETER Background
  Nur das Symbol anlegen, das Telefonbuch nicht sofort öffnen (Autostart).
#>
[CmdletBinding()]
param(
  [string]$Url,
  [ValidateSet('', 'app', 'browser')][string]$Mode = '',
  [switch]$Background,
  [switch]$Welcome,
  [string]$ExportIcon
)

Set-StrictMode -Version 2
$ErrorActionPreference = 'Stop'
$TbVersion = '1.3'
$TbTitle = 'Telefonbuch'   # Seitentitel (<title>) – daran wird ein offenes Fenster erkannt

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

# ---------------------------------------------------------------- Symbol (Pixel-Kopf, zur Laufzeit gezeichnet)

$script:TbPattern = @(
  '................',
  '.HHHHHHHHHHHHHH.',
  '.HHHHHHHHHHHHHH.',
  '.HHHHHHHHHHHHHH.',
  '.HHSSSSSSSSSSHH.',
  '.HSSSSSSSSSSSSH.',
  '.SSSSSSSSSSSSSS.',
  '.SSWESSSSSSWESS.',
  '.SSWESSSSSSWESS.',
  '.SSSSSSSSSSSSSS.',
  '.SSSSSSSSSSSSSS.',
  '.SSSMSSSSSSMSSS.',
  '.SSSSMMMMMMSSSS.',
  '.SSSSSSSSSSSSSS.',
  '.DDDDDDDDDDDDDD.',
  '................'
)
$script:TbColors = @{
  H = '#4f5359'; S = '#f5c518'; W = '#ffffff'; E = '#2d2f33'; M = '#7a5a00'; D = '#d9a400'   # gelber Skin
}

function New-TbBitmap([int]$Size) {
  $bmp = New-Object System.Drawing.Bitmap($Size, $Size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  try {
    $g.Clear([System.Drawing.Color]::Transparent)
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::None
    $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::Half
    $brushes = @{}
    foreach ($k in $script:TbColors.Keys) { $brushes[$k] = New-Object System.Drawing.SolidBrush([System.Drawing.ColorTranslator]::FromHtml($script:TbColors[$k])) }
    for ($y = 0; $y -lt 16; $y++) {
      $row = $script:TbPattern[$y]
      for ($x = 0; $x -lt 16; $x++) {
        $c = [string]$row[$x]
        if ($c -eq '.') { continue }
        # ganzzahlige Kanten, damit auch 20/24 px scharf bleiben
        $x0 = [int][Math]::Floor($x * $Size / 16); $x1 = [int][Math]::Floor(($x + 1) * $Size / 16)
        $y0 = [int][Math]::Floor($y * $Size / 16); $y1 = [int][Math]::Floor(($y + 1) * $Size / 16)
        $g.FillRectangle($brushes[$c], $x0, $y0, [Math]::Max(1, $x1 - $x0), [Math]::Max(1, $y1 - $y0))
      }
    }
    foreach ($b in $brushes.Values) { $b.Dispose() }
  } finally { $g.Dispose() }
  return $bmp
}

# .ico mit PNG-Bildern (16–256 px) – für Verknüpfungen
function Save-TbIco([string]$Path) {
  $sizes = @(16, 20, 24, 32, 40, 48, 64, 256)
  $data = New-Object 'System.Collections.Generic.List[byte[]]'
  foreach ($s in $sizes) {
    $bmp = New-TbBitmap $s
    $ms = New-Object System.IO.MemoryStream
    $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
    $bmp.Dispose()
    $data.Add($ms.ToArray())
  }
  $fs = [System.IO.File]::Create($Path)
  $w = New-Object System.IO.BinaryWriter($fs)
  try {
    $w.Write([uint16]0); $w.Write([uint16]1); $w.Write([uint16]$sizes.Count)
    $offset = 6 + 16 * $sizes.Count
    for ($i = 0; $i -lt $sizes.Count; $i++) {
      $dim = $sizes[$i]
      if ($dim -ge 256) { $dim = 0 }
      $w.Write([byte]$dim); $w.Write([byte]$dim); $w.Write([byte]0); $w.Write([byte]0)
      $w.Write([uint16]1); $w.Write([uint16]32)
      $w.Write([uint32]$data[$i].Length); $w.Write([uint32]$offset)
      $offset += $data[$i].Length
    }
    foreach ($d in $data) { $w.Write($d) }
  } finally { $w.Close() }
}

if ($ExportIcon) { Save-TbIco $ExportIcon; return }

# ---------------------------------------------------------------- Einstellungen

function Get-TbSetting([string]$Name) {
  foreach ($key in @('HKLM:\SOFTWARE\Policies\Telefonbuch', 'HKCU:\Software\Telefonbuch')) {
    try {
      $v = (Get-ItemProperty -Path $key -Name $Name -ErrorAction Stop).$Name
      if ($v) { return [string]$v }
    } catch { }
  }
  return ''
}

function Show-TbMessage([string]$Text, [string]$Icon = 'Warning') {
  [void][System.Windows.Forms.MessageBox]::Show($Text, 'Telefonbuch', 'OK', $Icon)
}

if (-not $Url) { $Url = Get-TbSetting 'Url' }
if (-not $Mode) { $Mode = Get-TbSetting 'Mode' }
if ($Mode -ne 'browser') { $Mode = 'app' }
$Url = ([string]$Url).Trim()
if ($Url -notmatch '^https?://[^\s"''<>]+$') {
  Show-TbMessage "Keine gültige Adresse des Telefonbuchs eingestellt.`n`nBitte das Installationsprogramm erneut ausführen oder die Adresse unter HKCU:\Software\Telefonbuch (Wert ""Url"") eintragen."
  exit 1
}

# Edge oder Chrome für das eigene App-Fenster (--app=…)
function Find-TbAppBrowser {
  foreach ($exe in @('msedge.exe', 'chrome.exe')) {
    foreach ($root in @('HKLM:', 'HKCU:')) {
      try {
        $p = (Get-ItemProperty -Path "$root\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\$exe" -ErrorAction Stop).'(default)'
        if ($p) { $p = $p.Trim('"') }
        if ($p -and (Test-Path -LiteralPath $p)) { return $p }
      } catch { }
    }
  }
  $p = Join-Path ${env:ProgramFiles(x86)} 'Microsoft\Edge\Application\msedge.exe'
  if (Test-Path -LiteralPath $p) { return $p }
  return $null
}
$script:AppBrowser = $null
if ($Mode -eq 'app') { $script:AppBrowser = Find-TbAppBrowser }

# ---------------------------------------------------------------- Offenes Fenster nach vorne holen

$script:CanActivate = $false
try {
  if (-not ('TbWin' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Text;
using System.Diagnostics;
using System.Runtime.InteropServices;
public static class TbWin {
  delegate bool EnumProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc cb, IntPtr l);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] static extern bool IsIconic(IntPtr h);
  [DllImport("user32.dll")] static extern bool ShowWindow(IntPtr h, int cmd);
  [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr h);
  // Sichtbares Browserfenster, dessen Titel mit prefix beginnt (z. B. "Telefonbuch" oder "Telefonbuch – Microsoft Edge")
  public static bool Activate(string prefix, string[] procs) {
    IntPtr found = IntPtr.Zero;
    EnumWindows(delegate (IntPtr h, IntPtr l) {
      if (!IsWindowVisible(h)) return true;
      StringBuilder sb = new StringBuilder(512);
      GetWindowText(h, sb, 512);
      string t = sb.ToString();
      if (!(t.Equals(prefix, StringComparison.OrdinalIgnoreCase) || t.StartsWith(prefix + " ", StringComparison.OrdinalIgnoreCase))) return true;
      uint pid;
      GetWindowThreadProcessId(h, out pid);
      string name;
      try { name = Process.GetProcessById((int)pid).ProcessName; } catch { return true; }
      foreach (string p in procs) {
        if (string.Equals(p, name, StringComparison.OrdinalIgnoreCase)) { found = h; return false; }
      }
      return true;
    }, IntPtr.Zero);
    if (found == IntPtr.Zero) return false;
    if (IsIconic(found)) ShowWindow(found, 9);
    SetForegroundWindow(found);
    return true;
  }
}
'@
  }
  $script:CanActivate = $true
} catch {
  # z. B. im ConstrainedLanguage-Modus: dann wird immer ein neues Fenster geöffnet
}

function Invoke-TbActivate {
  if (-not $script:CanActivate) { return $false }
  try { return [TbWin]::Activate($TbTitle, [string[]]@('msedge', 'chrome', 'firefox', 'brave', 'opera', 'vivaldi')) } catch { return $false }
}

# ---------------------------------------------------------------- Öffnen / Suchen

function Open-Tb([string]$Query) {
  $target = $Url
  if ($Query) {
    $sep = '?'
    if ($Url.Contains('?')) { $sep = '&' }
    $target = $Url + $sep + 'q=' + [uri]::EscapeDataString($Query)
  } elseif (Invoke-TbActivate) {
    return
  }
  if ($script:AppBrowser) {
    # Größe/Position für das App-Fenster: bis 1280×900, mittig im Arbeitsbereich
    $wa = [System.Windows.Forms.Screen]::PrimaryScreen.WorkingArea
    $ww = [Math]::Min(1280, $wa.Width - 40); $wh = [Math]::Min(900, $wa.Height - 40)
    $wx = $wa.Left + [int](($wa.Width - $ww) / 2); $wy = $wa.Top + [int](($wa.Height - $wh) / 2)
    Start-Process -FilePath $script:AppBrowser -ArgumentList @(('--app=' + $target), "--window-size=$ww,$wh", "--window-position=$wx,$wy")
  } else {
    Start-Process -FilePath $target
  }
  if ($Query) {
    # Die Seite übergibt die Suche an ein schon offenes Fenster und schließt sich – das holen wir nach vorne.
    $t = New-Object System.Windows.Forms.Timer
    $t.Interval = 1300
    $t.add_Tick({ param($s, $e) $s.Stop(); $s.Dispose(); [void](Invoke-TbActivate) })
    $t.Start()
  }
}

$script:SearchForm = $null
function Show-TbSearch {
  if ($script:SearchForm -and -not $script:SearchForm.IsDisposed) { $script:SearchForm.Activate(); return }
  $f = New-Object System.Windows.Forms.Form
  $f.Text = 'Telefonbuch durchsuchen'
  $f.FormBorderStyle = 'FixedDialog'
  $f.MaximizeBox = $false
  $f.MinimizeBox = $false
  $f.TopMost = $true
  $f.KeyPreview = $true
  $f.StartPosition = 'Manual'
  $f.AutoScaleMode = 'Dpi'
  $f.AutoScaleDimensions = New-Object System.Drawing.SizeF(96, 96)
  $f.Font = New-Object System.Drawing.Font('Segoe UI', 9)
  $f.ClientSize = New-Object System.Drawing.Size(380, 78)
  $f.Icon = $script:TrayIcon

  $lbl = New-Object System.Windows.Forms.Label
  $lbl.Text = 'Name, Telefon oder Mobil:'
  $lbl.AutoSize = $true
  $lbl.Location = New-Object System.Drawing.Point(12, 10)

  $tb = New-Object System.Windows.Forms.TextBox
  $tb.Name = 'q'
  $tb.MaxLength = 100
  $tb.Location = New-Object System.Drawing.Point(12, 34)
  $tb.Size = New-Object System.Drawing.Size(268, 25)

  $btn = New-Object System.Windows.Forms.Button
  $btn.Text = 'Suchen'
  $btn.Location = New-Object System.Drawing.Point(288, 32)
  $btn.Size = New-Object System.Drawing.Size(80, 28)
  $btn.add_Click({
      param($s, $e)
      $form = $s.FindForm()
      $q = $form.Controls['q'].Text.Trim()
      if ($q) { $form.Close(); try { Open-Tb $q } catch { Show-TbMessage $_.Exception.Message } }
    })
  $f.AcceptButton = $btn
  $f.Controls.AddRange(@($lbl, $tb, $btn))
  $f.add_KeyDown({ param($s, $e) if ($e.KeyCode -eq [System.Windows.Forms.Keys]::Escape) { $s.Close() } })
  $f.add_Shown({
      param($s, $e)
      $wa = [System.Windows.Forms.Screen]::FromPoint([System.Windows.Forms.Cursor]::Position).WorkingArea
      $s.Location = New-Object System.Drawing.Point(($wa.Right - $s.Width - 12), ($wa.Bottom - $s.Height - 12))
      $s.Activate()
      [void]$s.Controls['q'].Focus()
    })
  $script:SearchForm = $f
  $f.Show()
}

# ---------------------------------------------------------------- Favoriten (Kurzwahl)
# Persönlicher Favoriten-Code aus dem Telefonbuch (Menü → ★ Favoriten im Windows-Symbol),
# gespeichert unter HKCU:\Software\Telefonbuch (FavToken). Der Code erlaubt nur das Lesen der Favoriten.

$script:FavItems = $null
$script:FavError = ''
$script:FavLoaded = [DateTime]::MinValue

function Get-TbFavToken {
  try { return [string](Get-ItemProperty -Path 'HKCU:\Software\Telefonbuch' -Name FavToken -ErrorAction Stop).FavToken } catch { return '' }
}

function Update-TbFavorites {
  $tok = Get-TbFavToken
  if (-not $tok) { $script:FavItems = $null; $script:FavError = ''; return }
  try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    $feed = $Url.TrimEnd('/') + '/api/favorites/feed?t=' + [uri]::EscapeDataString($tok)
    $r = Invoke-RestMethod -Uri $feed -UseBasicParsing -TimeoutSec 6
    # leere Liste/fehlendes Feld sauber als 0 Einträge behandeln
    $script:FavItems = @(@(Get-TbField $r 'items') | Where-Object { $null -ne $_ -and $_ -isnot [string] })
    $script:FavError = ''
    $script:FavLoaded = [DateTime]::Now
  } catch {
    $script:FavError = $_.Exception.Message
  }
}

# Feld eines JSON-Objekts lesen – fehlende Felder (StrictMode!) ergeben ''
function Get-TbField($Obj, [string]$Name) {
  if ($null -eq $Obj) { return '' }
  $p = $Obj.PSObject.Properties[$Name]
  if ($null -eq $p -or $null -eq $p.Value) { return '' }
  return $p.Value
}

function Invoke-TbCall([string]$Number) {
  $n = ($Number -replace '[^0-9+]', '')
  if ($n) { Start-Process -FilePath ('tel:' + $n) }
}

function Connect-TbFavorites {
  Add-Type -AssemblyName Microsoft.VisualBasic
  $code = [Microsoft.VisualBasic.Interaction]::InputBox("Bitte den Favoriten-Code aus dem Telefonbuch einfügen`n(Menü → ★ Favoriten im Windows-Symbol → Code erzeugen):", 'Telefonbuch – Favoriten verbinden', '')
  $code = ([string]$code).Trim()
  if (-not $code) { return }
  if ($code -notmatch '^tbf_[A-Za-z0-9_-]{10,90}$') { Show-TbMessage 'Das sieht nicht wie ein Favoriten-Code aus (beginnt mit tbf_).'; return }
  New-Item -Path 'HKCU:\Software\Telefonbuch' -Force | Out-Null
  Set-ItemProperty -Path 'HKCU:\Software\Telefonbuch' -Name FavToken -Value $code
  Update-TbFavorites
  if ($script:FavError) { Show-TbMessage ('Verbindung fehlgeschlagen: ' + $script:FavError) }
  else { $script:Tray.ShowBalloonTip(4000, 'Telefonbuch', ('{0} Favorit(en) verbunden – Rechtsklick → ★ Favoriten.' -f @($script:FavItems).Count), [System.Windows.Forms.ToolTipIcon]::Info) }
}

function Build-TbFavMenu($Parent) {
  $Parent.DropDownItems.Clear()
  $add = { param($text, $action, $enabled = $true) $mi = New-Object System.Windows.Forms.ToolStripMenuItem($text); $mi.Enabled = $enabled; if ($action) { $mi.add_Click($action) }; [void]$Parent.DropDownItems.Add($mi); $mi }
  if (-not (Get-TbFavToken)) {
    [void](& $add 'Noch nicht verbunden' $null $false)
  } elseif ($script:FavError -and $null -eq $script:FavItems) {
    [void](& $add ('Nicht erreichbar: ' + $script:FavError) $null $false)
  } elseif (@($script:FavItems).Count -eq 0) {
    [void](& $add 'Keine Favoriten – im Telefonbuch mit ☆ markieren' $null $false)
  } else {
    foreach ($f in $script:FavItems) {
      $num = [string](Get-TbField $f 'phone')
      if (-not $num) { $num = [string](Get-TbField $f 'viaTel') }
      if (-not $num) { $num = [string](Get-TbField $f 'mobile') }
      $label = [string](Get-TbField $f 'name')
      if (Get-TbField $f 'private') { $label = '🔒 ' + $label }
      if ($num) { $label += '   ☎ ' + $num }
      $away = [string](Get-TbField $f 'away')
      if ($away) { try { $label += '   (abwesend bis ' + ([datetime]$away).ToString('dd.MM.') + ')' } catch { } }
      $mi = New-Object System.Windows.Forms.ToolStripMenuItem($label)
      $mi.Tag = $num
      $via = [string](Get-TbField $f 'via')
      $tip = @([string](Get-TbField $f 'amt'), $(if ($via) { 'bevorzugt: ' + $via } else { '' }), [string](Get-TbField $f 'email')) | Where-Object { $_ }
      $mi.ToolTipText = ($tip -join ' · ')
      if ($num) { $mi.add_Click({ param($s, $e) try { Invoke-TbCall ([string]$s.Tag) } catch { Show-TbMessage $_.Exception.Message } }) } else { $mi.Enabled = $false }
      [void]$Parent.DropDownItems.Add($mi)
    }
  }
  [void]$Parent.DropDownItems.Add((New-Object System.Windows.Forms.ToolStripSeparator))
  if (Get-TbFavToken) {
    [void](& $add 'Aktualisieren' { Update-TbFavorites })
    [void](& $add 'Trennen' { Remove-ItemProperty -Path 'HKCU:\Software\Telefonbuch' -Name FavToken -ErrorAction SilentlyContinue; $script:FavItems = $null })
  } else {
    [void](& $add 'Verbinden …' { try { Connect-TbFavorites } catch { Show-TbMessage $_.Exception.Message } })
  }
}

# ---------------------------------------------------------------- Autostart (Verknüpfung im Autostart-Ordner)

$script:StartMenuLnk = Join-Path ([Environment]::GetFolderPath('Programs')) 'Telefonbuch.lnk'
$script:StartupLnk = Join-Path ([Environment]::GetFolderPath('Startup')) 'Telefonbuch.lnk'

function Set-TbAutostart([bool]$On) {
  if (-not $On) { Remove-Item -LiteralPath $script:StartupLnk -ErrorAction SilentlyContinue; return }
  $sh = New-Object -ComObject WScript.Shell
  $src = $sh.CreateShortcut($script:StartMenuLnk)
  $dst = $sh.CreateShortcut($script:StartupLnk)
  $dst.TargetPath = $src.TargetPath
  $dst.Arguments = ($src.Arguments + ' -Background').Trim()
  $dst.WorkingDirectory = $src.WorkingDirectory
  $dst.IconLocation = $src.IconLocation
  $dst.Description = $src.Description
  $dst.WindowStyle = $src.WindowStyle
  $dst.Save()
}

# ---------------------------------------------------------------- Einzelinstanz

$created = $false
$mutex = New-Object System.Threading.Mutex($true, 'Local\Telefonbuch-Tray', [ref]$created)
if (-not $created) {
  # Läuft schon: nur das Telefonbuch öffnen (z. B. Klick auf die Startmenü-Verknüpfung)
  Open-Tb ''
  return
}

# ---------------------------------------------------------------- Infobereich

[System.Windows.Forms.Application]::EnableVisualStyles()
$size = [System.Windows.Forms.SystemInformation]::SmallIconSize.Width
$script:TrayBitmap = New-TbBitmap $size
$script:TrayIcon = [System.Drawing.Icon]::FromHandle($script:TrayBitmap.GetHicon())

$ni = New-Object System.Windows.Forms.NotifyIcon
$ni.Icon = $script:TrayIcon
$ni.Text = 'Telefonbuch'
$script:Tray = $ni

$menu = New-Object System.Windows.Forms.ContextMenuStrip
$miOpen = New-Object System.Windows.Forms.ToolStripMenuItem('Telefonbuch öffnen')
$miOpen.Font = New-Object System.Drawing.Font($miOpen.Font, [System.Drawing.FontStyle]::Bold)
$miOpen.add_Click({ try { Open-Tb '' } catch { Show-TbMessage $_.Exception.Message } })
$miSearch = New-Object System.Windows.Forms.ToolStripMenuItem('Suchen …')
$miSearch.add_Click({ try { Show-TbSearch } catch { Show-TbMessage $_.Exception.Message } })
$miFav = New-Object System.Windows.Forms.ToolStripMenuItem('★ Favoriten')
[void]$miFav.DropDownItems.Add('…')
$miAuto = New-Object System.Windows.Forms.ToolStripMenuItem('Mit Windows starten')
$miAuto.add_Click({
    param($s, $e)
    try { Set-TbAutostart (-not $s.Checked); $s.Checked = Test-Path -LiteralPath $script:StartupLnk } catch { Show-TbMessage $_.Exception.Message }
  })
$miInfo = New-Object System.Windows.Forms.ToolStripMenuItem(('Adresse: ' + ([uri]$Url).Host))
$miInfo.Enabled = $false
$miExit = New-Object System.Windows.Forms.ToolStripMenuItem('Beenden')
$miExit.add_Click({ $script:Tray.Visible = $false; [System.Windows.Forms.Application]::Exit() })
[void]$menu.Items.Add($miOpen)
[void]$menu.Items.Add($miSearch)
[void]$menu.Items.Add($miFav)
[void]$menu.Items.Add((New-Object System.Windows.Forms.ToolStripSeparator))
[void]$menu.Items.Add($miAuto)
[void]$menu.Items.Add($miInfo)
[void]$menu.Items.Add((New-Object System.Windows.Forms.ToolStripSeparator))
[void]$menu.Items.Add($miExit)
$menu.add_Opening({
    try {
      if ((Get-TbFavToken) -and ([DateTime]::Now - $script:FavLoaded).TotalMinutes -gt 10) { Update-TbFavorites }
      Build-TbFavMenu $miFav
    } catch {
      # Ein Fehler bei den Favoriten darf das Menü nie verhindern
      $miFav.DropDownItems.Clear()
      $err = New-Object System.Windows.Forms.ToolStripMenuItem(('Favoriten: ' + $_.Exception.Message))
      $err.Enabled = $false
      [void]$miFav.DropDownItems.Add($err)
    }
    $miAuto.Checked = Test-Path -LiteralPath $script:StartupLnk
    $miAuto.Enabled = (Test-Path -LiteralPath $script:StartMenuLnk) -or $miAuto.Checked
  })
$ni.ContextMenuStrip = $menu

$script:LastClick = [DateTime]::MinValue
$ni.add_MouseClick({
    param($s, $e)
    if ($e.Button -ne [System.Windows.Forms.MouseButtons]::Left) { return }
    $now = [DateTime]::Now
    if (($now - $script:LastClick).TotalMilliseconds -lt 800) { return }   # Doppelklick nicht doppelt öffnen
    $script:LastClick = $now
    try { Open-Tb '' } catch { Show-TbMessage $_.Exception.Message }
  })

$ni.Visible = $true
if ($Welcome) {
  $ni.ShowBalloonTip(6000, 'Telefonbuch', 'Das Telefonbuch ist jetzt im Infobereich. Klick: öffnen · Rechtsklick: suchen.', [System.Windows.Forms.ToolTipIcon]::Info)
}
if (-not $Background) {
  try { Open-Tb '' } catch { Show-TbMessage $_.Exception.Message }
}

# Favoriten im Hintergrund aktuell halten (alle 5 Minuten)
Update-TbFavorites
$favTimer = New-Object System.Windows.Forms.Timer
$favTimer.Interval = 5 * 60 * 1000
$favTimer.add_Tick({ Update-TbFavorites })
$favTimer.Start()

try {
  [System.Windows.Forms.Application]::Run()
} finally {
  $ni.Visible = $false
  $ni.Dispose()
  $mutex.ReleaseMutex()
  $mutex.Dispose()
}
