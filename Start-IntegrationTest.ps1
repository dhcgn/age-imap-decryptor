#Requires -Version 5.1
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$configPath   = Join-Path $PSScriptRoot 'config.yaml'
$outputFolder = Join-Path $PSScriptRoot '.debug\decrypted'

if (Test-Path $configPath) {
    Write-Host "Using config file: $configPath"
} else {
    Write-Error "Config file not found: $configPath"
    exit 1
}

if (-not (Test-Path $outputFolder)) {
    Write-Host "Creating output folder: $outputFolder"
    New-Item -ItemType Directory -Path $outputFolder | Out-Null
}

# Remove old decrypted files
Write-Host "Cleaning output folder: $outputFolder"
Get-ChildItem -Path $outputFolder | Remove-Item -Force -Recurse

Write-Host 'Building and starting age-imap-decryptor...'
& go run (Join-Path $PSScriptRoot 'cmd\age-imap-decryptor\main.go') --config $configPath --idle=false

Write-Host "Integration test completed. Check the output folder for decrypted files: $outputFolder"

Get-ChildItem -Path $outputFolder -Recurse -File | ForEach-Object {
    $relativePath = $_.FullName.Substring($outputFolder.Length).TrimStart('\')
    Write-Host "Decrypted file: $relativePath"
}