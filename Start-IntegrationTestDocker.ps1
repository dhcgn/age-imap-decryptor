#Requires -Version 5.1
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$scriptRoot       = $PSScriptRoot
$configPath       = Join-Path $scriptRoot 'config.yaml'
$debugDir         = Join-Path $scriptRoot '.debug'
$outputFolder     = Join-Path $debugDir 'decrypted_docker'
$dockerConfigPath = Join-Path $debugDir 'config.docker.yaml'
$dockerfilePath   = Join-Path $scriptRoot 'Dockerfile'
$imageName        = 'age-imap-decryptor:integration'
$containerName    = 'age-imap-decryptor-integration-test'
$keysHostPath     = Join-Path $HOME '.age'

function Invoke-NativeCommand {
	param(
		[Parameter(Mandatory = $true)]
		[string]$Description,
		[Parameter(Mandatory = $true)]
		[scriptblock]$Command
	)

	& $Command
	if ($LASTEXITCODE -ne 0) {
		throw "$Description failed with exit code $LASTEXITCODE."
	}
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
	Write-Error 'Docker CLI is not available in PATH.'
	exit 1
}

if (Test-Path $configPath) {
	Write-Host "Using config file: $configPath"
} else {
	Write-Error "Config file not found: $configPath"
	exit 1
}

if (Test-Path $keysHostPath) {
	Write-Host "Using age identities from: $keysHostPath"
} else {
	Write-Error "Age key directory not found: $keysHostPath"
	exit 1
}

if (-not (Test-Path $outputFolder)) {
	Write-Host "Creating output folder: $outputFolder"
	New-Item -ItemType Directory -Path $outputFolder -Force | Out-Null
}

# Remove old decrypted files but keep the output folder itself.
Write-Host "Cleaning output folder: $outputFolder"
Get-ChildItem -Path $outputFolder | Remove-Item -Force -Recurse

if (-not (Test-Path $debugDir)) {
	New-Item -ItemType Directory -Path $debugDir -Force | Out-Null
}

Write-Host 'Preparing container config...'
$configContent = Get-Content -Path $configPath -Raw
$dockerConfigContent = $configContent
$dockerConfigContent = $dockerConfigContent -replace '(?m)^([ \t]*path:[ \t]*).+$', '$1"/keys"'
$dockerConfigContent = $dockerConfigContent -replace '(?m)^([ \t]*base_dir:[ \t]*).+$', '$1"/output"'
Set-Content -Path $dockerConfigPath -Value $dockerConfigContent -NoNewline

Write-Host "Building Docker image: $imageName"
Invoke-NativeCommand -Description 'Docker build' -Command {
	& docker build --tag $imageName --file $dockerfilePath $scriptRoot
}

try {
	# Remove any previous test container if it exists.
	& docker rm --force $containerName 2>$null | Out-Null

	Write-Host "Running integration container: $containerName"
	Invoke-NativeCommand -Description 'Docker run' -Command {
		& docker run --rm --name $containerName `
			-e HOME=/root `
			--mount "type=bind,src=$dockerConfigPath,dst=/config.yaml,readonly" `
			--mount "type=bind,src=$outputFolder,dst=/output" `
			--mount "type=bind,src=$keysHostPath,dst=/keys,readonly" `
			$imageName --config /config.yaml --idle=false
	}
}
finally {
	if (Test-Path $dockerConfigPath) {
		Remove-Item -Path $dockerConfigPath -Force
	}
}

Write-Host "Integration test completed. Check the output folder for decrypted files: $outputFolder"

Get-ChildItem -Path $outputFolder -Recurse -File | ForEach-Object {
	$relativePath = $_.FullName.Substring($outputFolder.Length).TrimStart('\\')
	Write-Host "Decrypted file: $relativePath"
}
