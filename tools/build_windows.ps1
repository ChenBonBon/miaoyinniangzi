#Requires -Version 5.1
[CmdletBinding()]
param(
    [switch]$Tests,
    [switch]$AllowMissingWebView2
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Invoke-Checked {
    param([string]$Program, [string[]]$Arguments)
    & $Program @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Program failed with exit code $LASTEXITCODE" }
}
function Get-WebView2Version {
    $id = '{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
    foreach ($path in @("HKCU:\SOFTWARE\Microsoft\EdgeUpdate\Clients\$id", "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\$id", "HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients\$id")) {
        $item = Get-ItemProperty -LiteralPath $path -ErrorAction SilentlyContinue
        if ($null -ne $item -and $null -ne $item.PSObject.Properties['pv'] -and $item.pv -and $item.pv -ne '0.0.0.0') { return [string]$item.pv }
    }
    return $null
}
if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT -or -not [Environment]::Is64BitOperatingSystem) { throw 'Run this script on Windows x64.' }
if (-not [Environment]::Is64BitProcess) { throw 'Open 64-bit PowerShell, not PowerShell (x86).' }
foreach ($program in @('go', 'bun')) { if (-not (Get-Command $program -ErrorAction SilentlyContinue)) { throw "Missing $program. Read docs/windows-testing.md first." } }
$root = Split-Path -Parent $PSScriptRoot
$originalDirectory = Get-Location
$names = @('GOOS', 'GOARCH', 'GOAMD64', 'CGO_ENABLED', 'GOTOOLCHAIN', 'MIAOYIN_MPV_LIBRARY')
$previous = @{}
foreach ($name in $names) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
try {
    Set-Location -LiteralPath $root
    $env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:GOAMD64 = 'v1'; $env:CGO_ENABLED = '0'; $env:GOTOOLCHAIN = 'auto'
    $env:MIAOYIN_MPV_LIBRARY = $null
    Invoke-Checked 'go' @('version')
    $hostArch = (& go env GOHOSTARCH).Trim()
    if ($LASTEXITCODE -ne 0 -or $hostArch -ne 'amd64') { throw 'Install the Windows amd64 Go toolchain, not ARM64 or 386.' }
    $bunArch = (& bun -p 'process.arch').Trim()
    if ($LASTEXITCODE -ne 0 -or $bunArch -ne 'x64') { throw 'Install Bun for Windows x64.' }
    $bunVersion = (& bun --version).Trim()
    if ($LASTEXITCODE -ne 0 -or [version]($bunVersion.Split('-')[0]) -lt [version]'1.4.0') { throw 'Bun 1.4.0 or newer is required. Run bun upgrade, then reopen PowerShell.' }
    $webview = Get-WebView2Version
    if (-not $webview) {
        if (-not $AllowMissingWebView2) { throw 'WebView2 Runtime not detected. Install Microsoft.EdgeWebView2Runtime with winget, then rerun. Use -AllowMissingWebView2 only to build without running.' }
        Write-Warning 'WebView2 is missing. The built app cannot run until it is installed.'
    } else { Write-Host "WebView2 Runtime: $webview" }
    Write-Warning 'Development acceptance build only. Run Windows native sandbox tests before testing a real source; isolation never falls back to an ordinary process.'
    Invoke-Checked 'go' @('run', './tools/windows_build', 'prepare')
    Invoke-Checked 'go' @('run', './tools/windows_build', 'native')
    Push-Location -LiteralPath (Join-Path $root 'frontend')
    try {
        Invoke-Checked 'bun' @('install', '--frozen-lockfile')
        if ($Tests) {
            Invoke-Checked 'bun' @('--bun', 'run', 'check')
            Invoke-Checked 'bun' @('test', './tests')
        }
        Invoke-Checked 'bun' @('--bun', 'run', 'build')
    } finally { Pop-Location }
    if ($Tests) {
        $env:MIAOYIN_MPV_LIBRARY = Join-Path $root 'build\deps\windows-x64\libmpv-2.dll'
        Invoke-Checked 'go' @('test', '-ldflags=-H=windowsgui', './...')
        $env:MIAOYIN_MPV_LIBRARY = $null
    }
    Invoke-Checked 'go' @('run', 'github.com/wailsapp/wails/v2/cmd/wails@v2.17.0', 'build', '-platform', 'windows/amd64', '-s', '-m', '-webview2', 'error')
    Invoke-Checked 'go' @('build', '-trimpath', '-ldflags=-s -w -H=windowsgui', '-o', (Join-Path $root 'build\bin\miaoyin-source-worker.exe'), './tools/source_worker')
    Invoke-Checked 'go' @('run', './tools/windows_build', 'sandbox', '--exe', (Join-Path $root 'build\bin\miaoyinniangzi.exe'))
    $name = 'windows-x64-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + [Guid]::NewGuid().ToString('N').Substring(0, 6)
    $output = Join-Path $root ('build\bin\' + $name)
    Invoke-Checked 'go' @('run', './tools/windows_build', 'package', '--exe', (Join-Path $root 'build\bin\miaoyinniangzi.exe'), '--out', $output)
    Write-Host "Portable folder: $output"
    Write-Host 'Keep both EXEs and both DLLs together. Read WINDOWS-TESTING.md before acceptance.'
} finally {
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
    Set-Location -LiteralPath $originalDirectory
}
