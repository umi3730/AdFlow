param(
    [ValidateRange(30,120)][int]$Seconds=60,
    [ValidateSet('open','idle')][string]$Dimension='open',
    [string]$Output='',
    [switch]$PrepareOnly
)
$ErrorActionPreference='Stop'
$projectRoot=(Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if(-not $Output){$folder=if($Dimension -eq 'idle'){'idle-comparison'}else{'pool-comparison'};$Output=Join-Path $projectRoot ("work/$folder/run-"+(Get-Date -Format 'yyyyMMdd-HHmmss'))}
$outputDirectory=[IO.Path]::GetFullPath($Output,$projectRoot)
if(Test-Path -LiteralPath $outputDirectory){throw 'Use a new output directory to preserve previous experiments.'}
New-Item -ItemType Directory -Path $outputDirectory | Out-Null
$linuxRoot=(& wsl.exe -d Ubuntu -u root --exec wslpath -a $projectRoot.Replace('\','/')).Trim()
$linuxOutput=(& wsl.exe -d Ubuntu -u root --exec wslpath -a $outputDirectory.Replace('\','/')).Trim()
# Build and run from one private snapshot, so concurrent edits cannot change
# migrations, load scripts or the source fingerprints halfway through a series.
$snapshot=Join-Path $outputDirectory 'source'
New-Item -ItemType Directory -Path $snapshot | Out-Null
foreach($relative in @('cmd','internal','migrations','tests/bench/k6-lab','tests/load','go.mod','go.sum')){
 $target=Join-Path $snapshot $relative
 New-Item -ItemType Directory -Force -Path (Split-Path $target -Parent) | Out-Null
 Copy-Item -LiteralPath (Join-Path $projectRoot $relative) -Destination $target -Recurse
}
function Get-SnapshotHashes {
 $hashes=[ordered]@{}
 foreach($file in (Get-ChildItem -LiteralPath $snapshot -File -Recurse | Sort-Object FullName)){
  $relative=[IO.Path]::GetRelativePath($snapshot,$file.FullName).Replace('\','/')
  $hashes[$relative]=(Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLower()
 }
 return $hashes
}
$snapshotHashes=Get-SnapshotHashes
$snapshotJSON=$snapshotHashes | ConvertTo-Json -Depth 5 -Compress
$snapshotJSON | Set-Content -LiteralPath (Join-Path $outputDirectory 'source-sha256.json') -Encoding utf8
$linuxSnapshot="$linuxOutput/source"
$apiBinary=Join-Path $outputDirectory 'api-linux'
$runnerBinary=Join-Path $outputDirectory 'runner-linux'
$previousGOOS=$env:GOOS;$previousGOARCH=$env:GOARCH
Push-Location $snapshot
try{
 $env:GOOS='linux';$env:GOARCH='amd64'
 go build -o $apiBinary ./cmd/api
 if($LASTEXITCODE -ne 0){throw 'API build failed'}
 go build -o $runnerBinary ./tests/bench/k6-lab
 if($LASTEXITCODE -ne 0){throw 'Experiment runner build failed'}
}finally{$env:GOOS=$previousGOOS;$env:GOARCH=$previousGOARCH;Pop-Location}
if(((Get-SnapshotHashes) | ConvertTo-Json -Depth 5 -Compress) -cne $snapshotJSON){throw 'Source snapshot changed during build'}
if($PrepareOnly){Write-Host "Prepared isolated sources and binaries: $outputDirectory";return}
$order=if($Dimension -eq 'idle'){@(10,20,20,10,10,20)}else{@(30,60,100,60,100,30,100,30,60)}
$fixedPool=if($Dimension -eq 'idle'){@{maxOpen=30;maxLifetime='3m';maxIdleTime='1m'}}else{@{maxIdle=10;maxLifetime='3m';maxIdleTime='1m'}}
$design=[ordered]@{dimension=$Dimension;startedAt=(Get-Date).ToString('o');order=$order;overloadSeconds=$Seconds;baseline=@{rate=45;seconds=20};overload=@{rate=100;seconds=$Seconds};recovery=@{rate=45;seconds=30};fixedPool=$fixedPool;cpuProfiling='20 seconds during each overload phase';sourceSnapshot='source';apiSHA256=(Get-FileHash $apiBinary).Hash.ToLower();runnerSHA256=(Get-FileHash $runnerBinary).Hash.ToLower();trials=@()}
$design | ConvertTo-Json -Depth 7 | Set-Content -LiteralPath (Join-Path $outputDirectory 'design.json') -Encoding utf8
for($i=0;$i -lt $order.Count;$i++){
 $limit=if($Dimension -eq 'idle'){30}else{$order[$i]}
 $idle=if($Dimension -eq 'idle'){$order[$i]}else{10}
 $round=[math]::Floor($i/$(if($Dimension -eq 'idle'){2}else{3}))+1
 $name=if($Dimension -eq 'idle'){'trial-{0:D2}-idle-{1}' -f ($i+1),$idle}else{'trial-{0:D2}-pool-{1}' -f ($i+1),$limit}
 Write-Host "Trial $($i+1)/$($order.Count): max $limit / idle $idle, round $round"
 if(((Get-SnapshotHashes) | ConvertTo-Json -Depth 5 -Compress) -cne $snapshotJSON){throw 'Source snapshot changed before trial'}
 & wsl.exe -d Ubuntu -u root --cd $linuxSnapshot --exec "$linuxOutput/runner-linux" -api "$linuxOutput/api-linux" -k6 "$linuxRoot/work/k6-lab/k6-v2.2.0-linux-amd64/k6" -backpressure -confirm-seconds $Seconds -pool-max-open $limit -pool-max-idle $idle -output "$linuxOutput/$name" 2>&1 | Tee-Object -FilePath (Join-Path $outputDirectory "$name.log")
 $runExit=$LASTEXITCODE
 $proofPath=Join-Path (Join-Path $outputDirectory $name) 'proof.json'
 if(-not(Test-Path -LiteralPath $proofPath)){throw "Missing proof for $name"}
 $proof=Get-Content -LiteralPath $proofPath -Raw | ConvertFrom-Json
 if(((Get-SnapshotHashes) | ConvertTo-Json -Depth 5 -Compress) -cne $snapshotJSON){throw 'Source snapshot changed during trial'}
 if($proof.apiBinarySHA256 -ne $design.apiSHA256 -or (Get-FileHash $runnerBinary).Hash.ToLower() -ne $design.runnerSHA256){throw 'Experiment binary changed during trial'}
 if(@($proof.cleanup.PSObject.Properties | Where-Object {$_.Value -ne $true}).Count){throw "Cleanup failed for $name"}
 $design.trials+=@{name=$name;pool=$limit;idle=$idle;round=$round;exitCode=$runExit;passed=($proof.protectionPassed -eq $true);error=$proof.error}
 $design | ConvertTo-Json -Depth 7 | Set-Content -LiteralPath (Join-Path $outputDirectory 'design.json') -Encoding utf8
}
$design.finishedAt=(Get-Date).ToString('o')
$design | ConvertTo-Json -Depth 7 | Set-Content -LiteralPath (Join-Path $outputDirectory 'design.json') -Encoding utf8
Write-Host "Comparison evidence: $outputDirectory"
