param(
    [ValidateRange(5,120)][int]$Seconds = 20,
    [ValidateRange(10,100)][int]$Rate = 30,
    [switch]$Capacity,
    [switch]$Backpressure,
    [ValidateRange(30,120)][int]$ConfirmSeconds = 60,
    [ValidateRange(0,450)][int]$CapacityRate = 0
)
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$labDirectory = Join-Path $projectRoot 'work/k6-lab'
New-Item -ItemType Directory -Force $labDirectory | Out-Null
New-Item -ItemType Directory -Force (Join-Path $projectRoot 'work/end-to-end') | Out-Null
$linuxRoot = (& wsl.exe -d Ubuntu -u root --exec wslpath -a $projectRoot.Replace('\','/')).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Cannot resolve workspace inside WSL' }
$linuxK6 = Join-Path $labDirectory 'k6-v2.2.0-linux-amd64/k6'
if (!(Test-Path -LiteralPath $linuxK6)) {
    $archive = Join-Path $labDirectory 'k6-v2.2.0-linux-amd64.tar.gz'
    Invoke-WebRequest -Uri 'https://github.com/grafana/k6/releases/download/v2.2.0/k6-v2.2.0-linux-amd64.tar.gz' -OutFile $archive
    # SHA256 published in the official v2.2.0 checksums release asset.
    $expectedHash = 'B5A8003C86F35F5CD5CEEF1490312C48E587696C94D998CEFC6D7B3B4CB1597D'
    if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $expectedHash) { throw 'k6 archive checksum mismatch' }
    & wsl.exe -d Ubuntu -u root --cd "$linuxRoot/work/k6-lab" --exec tar -xzf k6-v2.2.0-linux-amd64.tar.gz
    if ($LASTEXITCODE -ne 0) { throw 'Cannot extract Linux k6' }
}
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
Push-Location $projectRoot
try {
    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    & go build -o work/end-to-end/api-linux ./cmd/api
    if ($LASTEXITCODE -ne 0) { throw 'API build failed' }
    & go build -o work/k6-lab/runner-linux ./tests/bench/k6-lab
    if ($LASTEXITCODE -ne 0) { throw 'k6 lab runner build failed' }
} finally {
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
    Pop-Location
}
$relativeOutput = 'work/k6-lab/run-' + (Get-Date -Format 'yyyyMMdd-HHmmss-fff')
$runnerArgs = @('-output', $relativeOutput, '-seconds', $Seconds, '-rate', $Rate)
if ($Capacity) { $runnerArgs += @('-capacity', '-confirm-seconds', $ConfirmSeconds, '-capacity-rate', $CapacityRate) }
if ($Backpressure) { $runnerArgs += @('-backpressure', '-confirm-seconds', $ConfirmSeconds) }
& wsl.exe -d Ubuntu -u root --cd $linuxRoot --exec ./work/k6-lab/runner-linux @runnerArgs
$labExitCode = $LASTEXITCODE
Write-Host "Reports: $(Join-Path $projectRoot $relativeOutput)"
if ($labExitCode -ne 0) { throw "k6 lab failed (exit $labExitCode); inspect k6-console.log and proof.json" }
