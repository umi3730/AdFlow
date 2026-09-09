param(
    [ValidateSet('cpu','mem','goroutine','block')][string]$ProfileType='cpu',
    [ValidateRange(1,300)][int]$Duration=30,
    [string]$BaseURL='http://127.0.0.1:6060',
    [ValidateRange(0,65535)][int]$ExpectedApiPort=0,
    [string]$Output='work/profiles'
)
$ErrorActionPreference='Stop'
$endpoint=[Uri]$BaseURL
if($endpoint.Scheme -notin @('http','https') -or -not $endpoint.IsLoopback -or $endpoint.UserInfo -or $endpoint.Query -or $endpoint.Fragment -or $endpoint.AbsolutePath -ne '/') {
    throw 'BaseURL must be a loopback HTTP(S) origin without credentials, path or query.'
}
$base=$BaseURL.TrimEnd('/')
try { $before=Invoke-RestMethod -Uri "$base/debug/instance" -TimeoutSec 5 }
catch { throw "Cannot reach profiling instance metadata at $base. Start the intended API with ADFLOW_PPROF_ADDR, then retry. No API was started by this script." }
if(-not $before.pid -or $before.apiAddress -notmatch ':(\d+)$') {throw 'Invalid profiling instance metadata'}
$apiPort=[int]$matches[1]
if($ExpectedApiPort -ne 0 -and $ExpectedApiPort -ne $apiPort) {throw "Profiler belongs to API port $apiPort, expected $ExpectedApiPort. Refusing to sample the wrong instance."}
if($ProfileType -eq 'block' -and $before.blockProfileRate -le 0) {throw 'Block sampling is disabled. Start this API with ADFLOW_PPROF_BLOCK_RATE=1 (or a chosen sampling rate) before collecting block profiles.'}
Write-Host "Profiling API $($before.apiAddress), PID $($before.pid), pool max $($before.pool.MaxOpenConnections) via $base"

New-Item -ItemType Directory -Force -Path $Output | Out-Null
$outputDirectory=(Resolve-Path -LiteralPath $Output).Path
$stamp=Get-Date -Format 'yyyyMMdd-HHmmss-fff'
$stem="$ProfileType-$stamp-$([Guid]::NewGuid().ToString('N').Substring(0,8))"
$profilePath=Join-Path $outputDirectory "$stem.pprof"
$topPath=Join-Path $outputDirectory "$stem.top.txt"
$metadataPath=Join-Path $outputDirectory "$stem.json"
$profileEndpoint=switch($ProfileType){'cpu'{"profile?seconds=$Duration"};'mem'{'heap'};'goroutine'{'goroutine'};'block'{'block'}}
$started=(Get-Date).ToUniversalTime().ToString('o')
Invoke-WebRequest -Uri "$base/debug/pprof/$profileEndpoint" -OutFile $profilePath -TimeoutSec ($Duration+10)
$after=Invoke-RestMethod -Uri "$base/debug/instance" -TimeoutSec 5
if($after.pid -ne $before.pid -or $after.startedAt -ne $before.startedAt) {throw "API restarted during collection. Raw file retained at $profilePath; do not attribute it to the new process."}
$top=& go tool pprof -top -nodecount=20 $profilePath 2>&1
if($LASTEXITCODE -ne 0) {throw "Profile analysis failed: $($top -join [Environment]::NewLine)"}
$top | Set-Content -LiteralPath $topPath -Encoding utf8
[ordered]@{
    startedAt=$started;finishedAt=(Get-Date).ToUniversalTime().ToString('o')
    profileType=$ProfileType;requestedDurationSeconds=$Duration;profilingURL=$base
    before=$before;after=$after
    profileFile=$profilePath;topFile=$topPath
    profileSHA256=(Get-FileHash -LiteralPath $profilePath -Algorithm SHA256).Hash.ToLower()
} | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $metadataPath -Encoding utf8
Write-Host "Raw profile: $profilePath"
Write-Host "Text summary: $topPath"
Write-Host "Instance metadata: $metadataPath"
Write-Host "Optional interactive viewer (runs until stopped): go tool pprof -http=127.0.0.1:8080 `"$profilePath`""
