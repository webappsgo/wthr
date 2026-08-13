# install-windows.ps1 - Windows installer for Weather Service
# Requires: PowerShell 5.0+, Administrator privileges for service installation
# Installs as Windows Service using NSSM

param(
    [switch]$UserInstall = $false
)

$ErrorActionPreference = "Stop"

$ProjectName = "Weather"
$ProjectNameLower = "weather"
$GitHubRepo = "webappsgo/wthr"
$Version = "latest"

Write-Host "=== Weather Service Installer for Windows ===" -ForegroundColor Green

# Detect architecture
$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
Write-Host "Architecture: $Arch"

# Check if running as administrator
$IsAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $UserInstall -and -not $IsAdmin) {
    Write-Host "Error: Administrator privileges required for system installation" -ForegroundColor Red
    Write-Host "Run PowerShell as Administrator or use -UserInstall flag" -ForegroundColor Yellow
    exit 1
}

# Determine installation paths
if ($UserInstall -or -not $IsAdmin) {
    $InstallMode = "User"
    $BinDir = "$env:LOCALAPPDATA\Programs\$ProjectName"
    $ConfigDir = "$env:APPDATA\$ProjectName\config"
    $DataDir = "$env:APPDATA\$ProjectName\data"
    $LogDir = "$env:APPDATA\$ProjectName\logs"
} else {
    $InstallMode = "System"
    $BinDir = "$env:ProgramFiles\$ProjectName"
    $ConfigDir = "$env:ProgramData\$ProjectName\config"
    $DataDir = "$env:ProgramData\$ProjectName\data"
    $LogDir = "$env:ProgramData\$ProjectName\logs"
}

Write-Host "Install mode: $InstallMode"

# Create directories
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
New-Item -ItemType Directory -Force -Path "$DataDir\db" | Out-Null
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null

# Download binary
Write-Host "Downloading ${ProjectNameLower}-windows-${Arch}.exe..."
$DownloadUrl = "https://github.com/$GitHubRepo/releases/$Version/download/${ProjectNameLower}-windows-${Arch}.exe"
$BinaryPath = "$BinDir\${ProjectNameLower}.exe"

try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $BinaryPath
    Write-Host "✓ Binary downloaded to $BinaryPath" -ForegroundColor Green
} catch {
    Write-Host "Error downloading binary: $_" -ForegroundColor Red
    exit 1
}

# Add to PATH if user installation
if ($UserInstall -or -not $IsAdmin) {
    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($UserPath -notlike "*$BinDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$UserPath;$BinDir", "User")
        Write-Host "✓ Added to user PATH" -ForegroundColor Green
    }
}

# Install as Windows Service (system installation only)
if (-not $UserInstall -and $IsAdmin) {
    Write-Host "Installing as Windows Service..."

    # Check if NSSM is installed
    $NssmPath = (Get-Command nssm -ErrorAction SilentlyContinue).Source

    if (-not $NssmPath) {
        Write-Host "NSSM not found. Attempting to download..." -ForegroundColor Yellow

        # Download NSSM
        $NssmUrl = "https://nssm.cc/release/nssm-2.24.zip"
        $NssmZip = "$env:TEMP\nssm.zip"
        $NssmExtract = "$env:TEMP\nssm"

        Invoke-WebRequest -Uri $NssmUrl -OutFile $NssmZip
        Expand-Archive -Path $NssmZip -DestinationPath $NssmExtract -Force

        # Copy appropriate architecture
        $NssmExe = "$NssmExtract\nssm-2.24\win64\nssm.exe"
        Copy-Item $NssmExe -Destination "$env:SystemRoot\System32\nssm.exe"

        Write-Host "✓ NSSM installed" -ForegroundColor Green
        $NssmPath = "$env:SystemRoot\System32\nssm.exe"
    }

    # Stop and remove existing service if present
    & $NssmPath stop $ProjectName 2>$null
    & $NssmPath remove $ProjectName confirm 2>$null

    # Install service
    & $NssmPath install $ProjectName "$BinaryPath"
    & $NssmPath set $ProjectName AppDirectory $DataDir
    & $NssmPath set $ProjectName AppStdout "$LogDir\output.log"
    & $NssmPath set $ProjectName AppStderr "$LogDir\error.log"
    & $NssmPath set $ProjectName AppRotateFiles 1
    & $NssmPath set $ProjectName AppRotateBytes 10485760  # 10MB
    & $NssmPath set $ProjectName AppEnvironmentExtra "PORT=80" "CONFIG_DIR=$ConfigDir" "DATA_DIR=$DataDir" "LOG_DIR=$LogDir"
    & $NssmPath set $ProjectName DisplayName "Weather Service"
    & $NssmPath set $ProjectName Description "Production-grade weather API service"
    & $NssmPath set $ProjectName Start SERVICE_AUTO_START

    # Start service
    & $NssmPath start $ProjectName

    Write-Host "✓ Service installed and started" -ForegroundColor Green
    Write-Host ""
    Write-Host "Service Commands:"
    Write-Host "  nssm status $ProjectName"
    Write-Host "  nssm stop $ProjectName"
    Write-Host "  nssm start $ProjectName"
    Write-Host "  nssm restart $ProjectName"
    Write-Host "  nssm remove $ProjectName confirm"
}

# Print summary
Write-Host ""
Write-Host "════════════════════════════════════════" -ForegroundColor Green
Write-Host "✅ Installation Complete!" -ForegroundColor Green
Write-Host "════════════════════════════════════════" -ForegroundColor Green
Write-Host ""
Write-Host "Binary:  $BinaryPath"
Write-Host "Config:  $ConfigDir"
Write-Host "Data:    $DataDir"
Write-Host "Logs:    $LogDir"
Write-Host ""
Write-Host "To access the service:"
Write-Host "  http://localhost"
Write-Host ""
Write-Host "For more information:"
Write-Host "  ${ProjectNameLower} --help"
Write-Host "  ${ProjectNameLower} --version"
Write-Host ""
