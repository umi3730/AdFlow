$ErrorActionPreference = 'Stop'

# systemd services alone do not keep a WSL instance alive after wsl.exe exits.
$adflowKeeper = Get-CimInstance Win32_Process -Filter "Name='wsl.exe'" | Where-Object { $_.CommandLine -match 'ADFLOW_WSL_KEEPALIVE=1' }
if (-not $adflowKeeper) {
    Start-Process -FilePath 'wsl.exe' -ArgumentList @('-d', 'Ubuntu', '--exec', 'env', 'ADFLOW_WSL_KEEPALIVE=1', 'sleep', 'infinity') -WindowStyle Hidden | Out-Null
}

function Invoke-AdFlowWSL {
    param([Parameter(Position=0, ValueFromRemainingArguments=$true)][string[]] $Arguments)
    & wsl.exe -d Ubuntu -u root -- @Arguments
    if ($LASTEXITCODE -ne 0) { throw "WSL command failed: $($Arguments[0])" }
}

Invoke-AdFlowWSL @('test', '-f', '/opt/adflow-runtime/kafka_2.13-4.3.1/bin/kafka-server-start.sh')
Invoke-AdFlowWSL @('systemctl', 'start', 'mysql', 'redis-server', 'adflow-kafka')
$adflowNativePath = & wsl.exe -d Ubuntu --exec wslpath -a $PSScriptRoot.Replace('\', '/')
if ($LASTEXITCODE -ne 0) { throw 'Cannot resolve native runtime scripts in WSL' }
$adflowNativePath = $adflowNativePath.Trim()
Invoke-AdFlowWSL @('install', '-m', '644', "$adflowNativePath/adflow-mysql-proxy.socket", '/etc/systemd/system/adflow-mysql-proxy.socket')
Invoke-AdFlowWSL @('install', '-m', '644', "$adflowNativePath/adflow-mysql-proxy.service", '/etc/systemd/system/adflow-mysql-proxy.service')
Invoke-AdFlowWSL @('systemctl', 'daemon-reload')
Invoke-AdFlowWSL @('systemctl', 'enable', '--now', 'adflow-mysql-proxy.socket')
Invoke-AdFlowWSL @('mysqladmin', 'ping')
Invoke-AdFlowWSL @('redis-cli', 'ping')
Invoke-AdFlowWSL @('systemctl', '--no-pager', 'is-active', 'mysql', 'redis-server', 'adflow-kafka')
Write-Output 'Native WSL components started. MySQL:13306 Redis:6379 Kafka:9092. No Docker containers are used.'
