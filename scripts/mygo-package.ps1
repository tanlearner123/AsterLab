# Package the unsigned Pure-Go MyGo build into a portable zip.
param(
    [string]$Version = '0.1.0-unsigned'
)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$distDir = Join-Path $repoRoot 'prototypes\mygo-run\dist'
if (-not (Test-Path -LiteralPath $distDir)) { throw "missing dist; run scripts/mygo-build.ps1 first" }
$releaseDir = Join-Path $repoRoot 'release'
if (-not (Test-Path -LiteralPath $releaseDir)) { New-Item -ItemType Directory -Path $releaseDir | Out-Null }
$name = "AsterLab-MyGo-$Version-win-x64"
$pkg = Join-Path $releaseDir $name
$zip = Join-Path $releaseDir "$name.zip"
if (Test-Path -LiteralPath $pkg) { Remove-Item -LiteralPath $pkg -Recurse -Force }
New-Item -ItemType Directory -Path $pkg | Out-Null
Copy-Item -Path (Join-Path $distDir '*') -Destination $pkg -Recurse
$hasNL = (Test-Path -LiteralPath (Join-Path $distDir 'models\nl-router\router.int8.onnx')) -and (Test-Path -LiteralPath (Join-Path $distDir 'onnxruntime.dll'))
$nlNote = if ($hasNL) {
    "Includes optional offline NL-Router ONNX model bundle (models\nl-router + onnxruntime.dll)."
} else {
    "Core Pure-Go standalone build (mygo-run.exe + asterlab.exe). No external DLLs required; optional offline NL-Router model can be placed in %LOCALAPPDATA%\AsterLab\models\nl-router."
}
Set-Content -LiteralPath (Join-Path $pkg 'UNSIGNED.txt') -Value "UNSIGNED PRE-RELEASE ($Version). No code signing certificate.`n$nlNote" -Encoding utf8
if (Test-Path -LiteralPath $zip) { Remove-Item -LiteralPath $zip -Force }
Compress-Archive -LiteralPath $pkg -DestinationPath $zip -CompressionLevel Optimal
Remove-Item -LiteralPath $pkg -Recurse -Force
Write-Output $zip
