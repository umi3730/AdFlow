param([int] $Port = 18080, [ValidateRange(0,65535)][int] $PprofPort = 0)
$ErrorActionPreference = 'Stop'
$adflowProject = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
if (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue) {
    throw "Port $Port is already in use. Stop the previous AdFlow API before starting its replacement."
}
Get-Content -LiteralPath (Join-Path $adflowProject '.env') | ForEach-Object {
    if ($_ -match '^([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
    }
}
$env:ADFLOW_HTTP_ADDR = "127.0.0.1:$Port"
if($PSBoundParameters.ContainsKey('PprofPort')) {
    $env:ADFLOW_PPROF_ADDR = if($PprofPort -eq 0){''}else{"127.0.0.1:$PprofPort"}
}
$adflowOutput = Join-Path $adflowProject 'work/native-api'
New-Item -ItemType Directory -Force -Path $adflowOutput | Out-Null
$adflowStamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
$adflowBinary = Join-Path $adflowOutput "adflow-api-$adflowStamp.exe"
Push-Location $adflowProject
try {
    & go build -o $adflowBinary ./cmd/api
    if ($LASTEXITCODE -ne 0) { throw 'AdFlow build failed' }
} finally { Pop-Location }
$adflowProcess = Start-Process -FilePath $adflowBinary -WorkingDirectory $adflowProject -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput (Join-Path $adflowOutput "api-$Port-$adflowStamp.stdout.log") `
    -RedirectStandardError (Join-Path $adflowOutput "api-$Port-$adflowStamp.stderr.log")
[pscustomobject]@{ ProcessId = $adflowProcess.Id; Port = $Port; Binary = $adflowBinary }
