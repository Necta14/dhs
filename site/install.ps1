<#
.SYNOPSIS
    Installs the DHS command-line tool on Windows, for the current user.

.DESCRIPTION
    Downloads the release archive for this machine's architecture, checks it against the SHA-256
    sums published with the release, unpacks dhs.exe into a per-user directory and puts that
    directory on the user PATH. No administrator rights, no service, nothing running afterwards.

    The shortest form:

        irm https://dhs-suite.vercel.app/install.ps1 | iex

    To pass options, create a script block from it instead:

        & ([scriptblock]::Create((irm https://dhs-suite.vercel.app/install.ps1))) -Version 0.1.1

.PARAMETER Version
    A release to install, with or without the leading "v". Defaults to the most recent release,
    pre-releases included, which is what DHS publishes today.

.PARAMETER InstallDir
    Where to put dhs.exe. Defaults to %LOCALAPPDATA%\Programs\dhs.

.PARAMETER NoPathUpdate
    Unpack the binary but leave the PATH alone.

.LINK
    https://github.com/Necta14/dhs
#>
param(
    [string] $Version = 'latest',
    [string] $InstallDir,
    [switch] $NoPathUpdate
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'   # the progress bar makes downloads slower in 5.1

$Repo = 'Necta14/dhs'

# PowerShell 5.1 still defaults to TLS 1.0 against some hosts; GitHub needs 1.2 or better.
try {
    [Net.ServicePointManager]::SecurityProtocol =
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
} catch { }

# $IsWindows exists from PowerShell 6; on 5.1 there is only Windows.
$onWindows = if (Test-Path variable:IsWindows) { $IsWindows } else { $true }

function Say([string] $Text) { Write-Host $Text }
function Step([string] $Text) { Write-Host "  $Text" -ForegroundColor DarkGray }
function Good([string] $Text) { Write-Host "  $Text" -ForegroundColor Green }
function Die([string] $Text) { Write-Host "  $Text" -ForegroundColor Red; exit 1 }

Say ''
Say 'DHS — Direct Handoff Suite'
Say ''

# ── which build ────────────────────────────────────────────────────────────────────────────────
# On ARM64 Windows an x64 PowerShell reports AMD64 and puts the real answer in ARCHITEW6432.
$raw = $env:PROCESSOR_ARCHITEW6432
if (-not $raw) { $raw = $env:PROCESSOR_ARCHITECTURE }
if (-not $raw) { $raw = 'AMD64' }

switch ($raw.ToUpperInvariant()) {
    'AMD64' { $arch = 'amd64' }
    'ARM64' { $arch = 'arm64' }
    'X86'   { Die '32-bit Windows is not supported. DHS ships amd64 and arm64 builds.' }
    default { Die "Unrecognised processor architecture: $raw" }
}

# ── which release ──────────────────────────────────────────────────────────────────────────────
$headers = @{ 'Accept' = 'application/vnd.github+json'; 'User-Agent' = 'dhs-install' }

if ($Version -eq 'latest') {
    # Not /releases/latest: that endpoint skips pre-releases, and every DHS release so far is one.
    Step 'Looking up the most recent release…'
    $rel = (Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases?per_page=10" -Headers $headers) |
           Where-Object { -not $_.draft } | Select-Object -First 1
    if (-not $rel) { Die 'No release found. Check https://github.com/Necta14/dhs/releases' }
    $tag = $rel.tag_name
} else {
    $tag = if ($Version.StartsWith('v')) { $Version } else { "v$Version" }
}
$number = $tag.TrimStart('v')

if ($tag -match '^v0\.') {
    Say "  $tag is a pre-release. The file core is tested; migrating anything you care about is not"
    Say '  advisable yet, and the Windows build has had less exposure than the Linux one.'
    Say ''
}

$asset = "dhs_${number}_windows_${arch}.zip"
$base  = "https://github.com/$Repo/releases/download/$tag"

# ── download ───────────────────────────────────────────────────────────────────────────────────
$work = Join-Path ([IO.Path]::GetTempPath()) ("dhs-install-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work -Force | Out-Null

try {
    $zip = Join-Path $work $asset
    Step "Downloading $asset …"
    try {
        Invoke-WebRequest -Uri "$base/$asset" -OutFile $zip -Headers $headers -UseBasicParsing
    } catch {
        Die "Could not download $base/$asset — $($_.Exception.Message)"
    }

    # ── verify ─────────────────────────────────────────────────────────────────────────────────
    Step 'Checking SHA-256 against the sums published with the release…'
    $sumsFile = Join-Path $work 'SHA256SUMS'
    try {
        Invoke-WebRequest -Uri "$base/SHA256SUMS" -OutFile $sumsFile -Headers $headers -UseBasicParsing
    } catch {
        Die "The release has no SHA256SUMS to check against — refusing to install unverified. $($_.Exception.Message)"
    }

    $expected = $null
    foreach ($line in Get-Content -LiteralPath $sumsFile) {
        # "<64 hex>  ./dhs_0.1.1_windows_amd64.zip", the format sha256sum writes
        $parts = $line -split '\s+', 2
        if ($parts.Count -eq 2 -and ($parts[1].Trim() -replace '^\./', '') -eq $asset) {
            $expected = $parts[0].Trim().ToLowerInvariant()
            break
        }
    }
    if (-not $expected) { Die "SHA256SUMS does not mention $asset — refusing to install unverified." }

    $actual = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        Die "Checksum mismatch for $asset.`n  expected $expected`n  got      $actual"
    }
    Good 'Checksum matches.'

    # ── unpack ─────────────────────────────────────────────────────────────────────────────────
    if (-not $InstallDir) {
        $localAppData = $env:LOCALAPPDATA
        if (-not $localAppData) { $localAppData = Join-Path $HOME '.local' }
        $InstallDir = Join-Path (Join-Path $localAppData 'Programs') 'dhs'
    }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null

    $unpacked = Join-Path $work 'unpacked'
    Expand-Archive -LiteralPath $zip -DestinationPath $unpacked -Force

    $exe = Get-ChildItem -LiteralPath $unpacked -Filter 'dhs.exe' -Recurse | Select-Object -First 1
    if (-not $exe) { Die 'The archive contains no dhs.exe.' }

    $target = Join-Path $InstallDir 'dhs.exe'
    Copy-Item -LiteralPath $exe.FullName -Destination $target -Force
    foreach ($doc in @('LICENSE', 'NOTICE', 'README.md')) {
        $found = Get-ChildItem -LiteralPath $unpacked -Filter $doc -Recurse -ErrorAction SilentlyContinue |
                 Select-Object -First 1
        if ($found) { Copy-Item -LiteralPath $found.FullName -Destination (Join-Path $InstallDir $doc) -Force }
    }
    Good "Installed to $target"

    # ── PATH ───────────────────────────────────────────────────────────────────────────────────
    if ($NoPathUpdate) {
        Step 'Leaving PATH untouched, as asked.'
    } elseif (-not $onWindows) {
        Step 'Not on Windows, so PATH is left alone.'
    } else {
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        if (-not $userPath) { $userPath = '' }
        $already = ($userPath -split ';' | Where-Object { $_ } |
                    Where-Object { $_.TrimEnd('\') -ieq $InstallDir.TrimEnd('\') })
        if ($already) {
            Step 'Already on your PATH.'
        } else {
            $joined = if ($userPath.TrimEnd(';')) { $userPath.TrimEnd(';') + ';' + $InstallDir } else { $InstallDir }
            [Environment]::SetEnvironmentVariable('Path', $joined, 'User')
            Good 'Added to your PATH. Open a new terminal for it to take effect.'
        }
        # Usable in this session too, without waiting for a new terminal.
        if (($env:Path -split ';') -notcontains $InstallDir) { $env:Path = "$env:Path;$InstallDir" }
    }

    # ── prove it runs ──────────────────────────────────────────────────────────────────────────
    Say ''
    if ($onWindows) {
        try {
            $reported = (& $target version 2>&1 | Select-Object -First 1)
            Good "$reported"
        } catch {
            Step "Installed, but running it failed: $($_.Exception.Message)"
        }
    } else {
        Step 'Windows binary unpacked; not run, because this is not Windows.'
    }

    Say ''
    Say '  Next:'
    Say '    dhs scan --dest D:\           what would be saved, how big, whether it fits'
    Say '    dhs backup --dest D:\ --name laptop'
    Say '    dhs verify D:\laptop.dhs'
    Say '    dhs restore D:\laptop.dhs --dry-run'
    Say ''
    Say '  Documentation: https://dhs-suite.vercel.app'
    Say '  To remove DHS: delete the folder above and take it off your PATH. Nothing else was changed.'
    Say ''
}
finally {
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}
