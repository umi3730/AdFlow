param(
    [ValidateRange(3,60)][int]$Seconds = 15,
    [ValidateRange(1,100)][int]$Threads = 20,
    [ValidateRange(1,5)][int]$Rounds = 3
)
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$jmeterDirectory = Join-Path $projectRoot 'work/jmeter'
New-Item -ItemType Directory -Force $jmeterDirectory | Out-Null
New-Item -ItemType Directory -Force (Join-Path $projectRoot 'work/end-to-end') | Out-Null
$linuxRoot = (& wsl.exe -d Ubuntu -u root --exec wslpath -a $projectRoot.Replace('\','/')).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Cannot resolve project in WSL' }
if (!(Test-Path -LiteralPath (Join-Path $jmeterDirectory 'apache-jmeter-5.6.3/bin/ApacheJMeter.jar'))) {
    $archive = Join-Path $jmeterDirectory 'apache-jmeter-5.6.3.tgz'
    Invoke-WebRequest -Uri 'https://dlcdn.apache.org/jmeter/binaries/apache-jmeter-5.6.3.tgz' -OutFile $archive
    $expected = '5978a1a35edb5a7d428e270564ff49d2b1b257a65e17a759d259a9283fc17093e522fe46f474a043864aea6910683486340706d745fcdf3db1505fd71e689083'
    if ((Get-FileHash -LiteralPath $archive -Algorithm SHA512).Hash -ne $expected) { throw 'JMeter SHA512 mismatch' }
    & wsl.exe -d Ubuntu -u root --cd "$linuxRoot/work/jmeter" --exec tar -xzf apache-jmeter-5.6.3.tgz
    if ($LASTEXITCODE -ne 0) { throw 'JMeter extraction failed' }
}
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
Push-Location $projectRoot
try {
    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    & go build -o work/end-to-end/api-linux ./cmd/api
    if ($LASTEXITCODE -ne 0) { throw 'API build failed' }
    & go build -o work/jmeter/profile-runner-linux ./tests/bench/jmeter-profile
    if ($LASTEXITCODE -ne 0) { throw 'Runner build failed' }
} finally {
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
    Pop-Location
}
$relativeOutput = 'work/jmeter/run-' + (Get-Date -Format 'yyyyMMdd-HHmmss-fff')
& wsl.exe -d Ubuntu -u root --cd $linuxRoot --exec ./work/jmeter/profile-runner-linux -output $relativeOutput -seconds $Seconds -threads $Threads -rounds $Rounds
$runExitCode = $LASTEXITCODE
Write-Host "Results and JTL files: $(Join-Path $projectRoot $relativeOutput)"
if ($runExitCode -ne 0) { throw "JMeter comparison failed (exit $runExitCode); inspect results.json and JTL assertions" }
