# Cross-compiles yt-import for Windows, macOS, and Linux
param (
    [string]$Version = "v1.0.0"
)

$ErrorActionPreference = "Stop"

$targets = @(
    @{ OS = "windows"; Arch = "amd64"; Ext = ".exe" },
    @{ OS = "windows"; Arch = "arm64"; Ext = ".exe" },
    @{ OS = "darwin";  Arch = "amd64"; Ext = "" },
    @{ OS = "darwin";  Arch = "arm64"; Ext = "" },
    @{ OS = "linux";   Arch = "amd64"; Ext = "" },
    @{ OS = "linux";   Arch = "arm64"; Ext = "" }
)

$distDir = Join-Path $PSScriptRoot "..\dist"
if (Test-Path $distDir) {
    Remove-Item -Recurse -Force $distDir
}
New-Item -ItemType Directory -Path $distDir | Out-Null

$binDir = Join-Path $PSScriptRoot "..\bin"
if (Test-Path $binDir) {
    Remove-Item -Recurse -Force $binDir
}
New-Item -ItemType Directory -Path $binDir | Out-Null

Write-Host "Building yt-import $Version for all platforms..." -ForegroundColor Cyan

foreach ($target in $targets) {
    $os = $target.OS
    $arch = $target.Arch
    $ext = $target.Ext
    $outBinary = Join-Path $binDir ("yt-import" + $ext)
    $assetName = "yt-import_${Version}_${os}_${arch}"

    Write-Host "  -> Building $os/$arch..." -ForegroundColor Yellow
    $env:GOOS = $os
    $env:GOARCH = $arch
    $env:CGO_ENABLED = "0"

    & go build -ldflags="-s -w" -trimpath -o $outBinary ./cmd/yt-import
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to build for $os/$arch"
        exit 1
    }

    $archivePath = ""
    if ($os -eq "windows") {
        $archivePath = Join-Path $distDir ($assetName + ".zip")
        Compress-Archive -Path $outBinary, "README.md", "LICENSE" -DestinationPath $archivePath -Force
    } else {
        $archivePath = Join-Path $distDir ($assetName + ".tar.gz")
        tar -czf $archivePath -C $binDir ("yt-import" + $ext) -C $PSScriptRoot\.. "README.md" "LICENSE"
    }

    if (Test-Path $outBinary) {
        Remove-Item -Force $outBinary
    }
}

if (Test-Path $binDir) {
    Remove-Item -Recurse -Force $binDir
}

# Generate SHA256 Checksums
Write-Host "Generating checksums..." -ForegroundColor Cyan
$checksumFile = Join-Path $distDir "checksums.txt"
$files = Get-ChildItem -Path $distDir -File | Where-Object { $_.Name -ne "checksums.txt" }
$lines = foreach ($file in $files) {
    $hash = (Get-FileHash -Path $file.FullName -Algorithm SHA256).Hash.ToLower()
    "$hash  $($file.Name)"
}
$lines | Out-File -FilePath $checksumFile -Encoding utf8

Write-Host "Build complete! All archives and checksums ready in ./dist:" -ForegroundColor Green
Get-ChildItem -Path $distDir | Format-Table Name, Length
