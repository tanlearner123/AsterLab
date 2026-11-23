# scripts/mygo-build.ps1
# Build Pure-Go AsterLab desktop app + CLI into a clean dist folder.
# Optional -IncludeNLModel bundles the offline INT8 ONNX NL-Router + onnxruntime.dll with SHA256 verification.
# Unsigned: this script does not sign anything.
param(
    [string]$Version = '0.1.0-dev',
    [switch]$IncludeNLModel,
    [string]$NLModelDir = '',
    [string]$OnnxRuntimeDll = ''
)
$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$protoDir = Join-Path $repoRoot 'prototypes\mygo-run'
$distDir = Join-Path $protoDir 'dist'

if (-not (Test-Path -LiteralPath $protoDir)) { throw "prototype dir missing: $protoDir" }

# Clean dist
if (Test-Path -LiteralPath $distDir) { Remove-Item -LiteralPath $distDir -Recurse -Force }
New-Item -ItemType Directory -Path $distDir | Out-Null

Push-Location $protoDir
try {
    $env:CGO_ENABLED = '0'
    $commit = (git -C $repoRoot rev-parse --short HEAD).Trim()
    $verFlags = "-X 'asterlab-mygo-prototype/internal/updater.Version=$Version' -X 'asterlab-mygo-prototype/internal/updater.Commit=$commit'"

    # 1. Pure-Go Desktop app (windowsgui hides the console window when double-clicked)
    go build -tags=e2e -trimpath -ldflags="-s -w -H=windowsgui $verFlags" -o (Join-Path $distDir 'mygo-run.exe') .
    if ($LASTEXITCODE -ne 0) { throw "go build desktop failed (exit $LASTEXITCODE)" }

    # 2. Pure-Go Vault CLI
    go build -trimpath -ldflags="-s -w $verFlags" -o (Join-Path $distDir 'asterlab.exe') ./cmd/asterlab
    if ($LASTEXITCODE -ne 0) { throw "go build cli failed (exit $LASTEXITCODE)" }

    $icoSrc = Join-Path $protoDir 'assets\asterlab.ico'
    if (Test-Path -LiteralPath $icoSrc) {
        Copy-Item -LiteralPath $icoSrc -Destination (Join-Path $distDir 'asterlab.ico')
    }

    # 3. Optional Offline AI Semantic Router bundle (models/nl-router + onnxruntime.dll)
    $nlBundleNote = "nl-router: NOT bundled (graceful fallback to deterministic local rules + user hot-upgrade dir %LOCALAPPDATA%\AsterLab\models\nl-router)"
    if ($IncludeNLModel) {
        if ([string]::IsNullOrWhiteSpace($NLModelDir)) {
            $NLModelDir = Join-Path $protoDir 'models\nl-router'
        }
        $manifestPath = Join-Path $NLModelDir 'manifest.json'
        $onnxPath = Join-Path $NLModelDir 'router.int8.onnx'
        $vocabPath = Join-Path $NLModelDir 'vocab.txt'
        if (-not (Test-Path -LiteralPath $manifestPath)) { throw "NL-Router manifest missing: $manifestPath" }
        if (-not (Test-Path -LiteralPath $onnxPath)) { throw "NL-Router ONNX model missing: $onnxPath" }
        if (-not (Test-Path -LiteralPath $vocabPath)) { throw "NL-Router vocab missing: $vocabPath" }

        $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
        $actualOnnxSha = (Get-FileHash -LiteralPath $onnxPath -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($manifest.onnx_sha256 -and ($manifest.onnx_sha256.ToLowerInvariant() -ne $actualOnnxSha)) {
            throw "NL-Router SHA256 mismatch! manifest=$($manifest.onnx_sha256) actual=$actualOnnxSha"
        }

        if ([string]::IsNullOrWhiteSpace($OnnxRuntimeDll)) {
            $candidates = @(
                (Join-Path $protoDir 'onnxruntime.dll'),
                (Join-Path $NLModelDir 'onnxruntime.dll'),
                "$env:LOCALAPPDATA\AsterLab\onnxruntime.dll"
            )
            foreach ($cand in $candidates) {
                if (Test-Path -LiteralPath $cand) { $OnnxRuntimeDll = $cand; break }
            }
            if ([string]::IsNullOrWhiteSpace($OnnxRuntimeDll)) {
                $pyOrt = python -c "import onnxruntime, os; print(os.path.join(os.path.dirname(onnxruntime.__file__), 'capi', 'onnxruntime.dll'))" 2>$null
                if ($pyOrt -and (Test-Path -LiteralPath $pyOrt.Trim())) {
                    $OnnxRuntimeDll = $pyOrt.Trim()
                }
            }
        }
        if ([string]::IsNullOrWhiteSpace($OnnxRuntimeDll) -or -not (Test-Path -LiteralPath $OnnxRuntimeDll)) {
            throw "onnxruntime.dll not found; pass -OnnxRuntimeDll <path> when using -IncludeNLModel"
        }

        $dstModelDir = Join-Path $distDir 'models\nl-router'
        New-Item -ItemType Directory -Path $dstModelDir -Force | Out-Null
        foreach ($f in @('manifest.json', 'router.int8.onnx', 'vocab.txt', 'schemas.json')) {
            $srcFile = Join-Path $NLModelDir $f
            if (Test-Path -LiteralPath $srcFile) {
                Copy-Item -LiteralPath $srcFile -Destination (Join-Path $dstModelDir $f) -Force
            }
        }
        Copy-Item -LiteralPath $OnnxRuntimeDll -Destination (Join-Path $distDir 'onnxruntime.dll') -Force
        $shaOrt = (Get-FileHash -LiteralPath (Join-Path $distDir 'onnxruntime.dll') -Algorithm SHA256).Hash
        $nlBundleNote = "nl-router: v$($manifest.version) bundled`nrouter.int8.onnx SHA256 $actualOnnxSha`nonnxruntime.dll SHA256 $shaOrt"
    }

    # Evidence
    $shaExe = (Get-FileHash -LiteralPath (Join-Path $distDir 'mygo-run.exe') -Algorithm SHA256).Hash
    $shaCli = (Get-FileHash -LiteralPath (Join-Path $distDir 'asterlab.exe') -Algorithm SHA256).Hash
    $info = "AsterLab Pure-Go Build (UNSIGNED)`nversion $Version`ncommit $commit`nmygo-run.exe SHA256 $shaExe`nasterlab.exe SHA256 $shaCli`n$nlBundleNote`n"
    Set-Content -LiteralPath (Join-Path $distDir 'BUILD_INFO.txt') -Value $info -Encoding utf8
    Write-Output "dist: $distDir"
    Write-Output $info
} finally {
    Pop-Location
}
