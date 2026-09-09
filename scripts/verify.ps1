param(
    [switch]$Race,
    [switch]$BackendOnly
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$goPackages = @('./cmd/...', './internal/...', './tests/...')

function Invoke-Check {
    param([string]$Program, [string[]]$Arguments)
    Write-Host "> $Program $($Arguments -join ' ')"
    & $Program @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Program failed with exit code $LASTEXITCODE"
    }
}

Push-Location -LiteralPath $repositoryRoot
try {
    $unformatted = @(gofmt -l cmd internal tests)
    if ($LASTEXITCODE -ne 0) { throw 'gofmt failed' }
    if ($unformatted.Count -gt 0) {
        $fileList = $unformatted -join [Environment]::NewLine
        throw "Run gofmt on these files before verification:`n$fileList"
    }
    Invoke-Check -Program go -Arguments (@('vet') + $goPackages)
    $testArguments = @('test')
    if ($Race) { $testArguments += '-race' }
    Invoke-Check -Program go -Arguments ($testArguments + $goPackages)

    if (-not $BackendOnly) {
        Push-Location -LiteralPath (Join-Path $repositoryRoot 'web')
        try {
            Invoke-Check -Program npm.cmd -Arguments @('run', 'lint')
            Invoke-Check -Program npm.cmd -Arguments @('test')
        } finally {
            Pop-Location
        }
    }
    Write-Host 'Verification passed.'
} finally {
    Pop-Location
}
