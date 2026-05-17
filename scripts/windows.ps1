# Weather Service - Windows Installer
# PowerShell installation script with Windows Service support

param(
    [string]$Version = "latest",
    [string]$InstallDir = "$env:ProgramFiles\Weather",
    [string]$DataDir = "$env:ProgramData\Weather",
    [switch]$InstallService
)

$ErrorActionPreference = "Stop"

Write-Host "🌤️  Weather Service - Windows Installer" -ForegroundColor Cyan
Write-Host ""

# Detect architecture
$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
$IsARM = $false

# Check if ARM64
try {
    $ProcessorInfo = Get-WmiObject -Class Win32_Processor
    if ($ProcessorInfo.Architecture -eq 12) {
        $Arch = "arm64"
        $IsARM = $true
    }
} catch {}

Write-Host "✓ Detected: windows/$Arch" -ForegroundColor Green

# Get latest version
$Repo = "casapps/wthr"
if ($Version -eq "latest") {
    Write-Host "🔍 Fetching latest version..."
    try {
        $LatestRelease = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
        $Version = $LatestRelease.tag_name
    } catch {
        Write-Host "❌ Failed to fetch latest version" -ForegroundColor Red
        exit 1
    }
}

Write-Host "✓ Version: $Version" -ForegroundColor Green

# Construct download URL
$BinaryFile = "weather-windows-$Arch.exe"
$DownloadUrl = "https://github.com/$Repo/releases/download/$Version/$BinaryFile"

Write-Host "📥 Downloading from: $DownloadUrl"

# Create temp directory
$TempDir = [System.IO.Path]::GetTempPath() + [System.Guid]::NewGuid().ToString()
New-Item -ItemType Directory -Path $TempDir | Out-Null
$TempFile = Join-Path $TempDir $BinaryFile

try {
    # Download binary
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $TempFile
    Write-Host "✓ Downloaded successfully" -ForegroundColor Green

    # Create directories
    Write-Host "📁 Creating directories..."
    if (!(Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }
    if (!(Test-Path $DataDir)) {
        New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
    }
    # Create subdirectories
    New-Item -ItemType Directory -Path "$DataDir\db" -Force | Out-Null
    New-Item -ItemType Directory -Path "$DataDir\backups" -Force | Out-Null
    New-Item -ItemType Directory -Path "$env:APPDATA\weather\certs" -Force | Out-Null
    New-Item -ItemType Directory -Path "$env:APPDATA\weather\databases" -Force | Out-Null
    New-Item -ItemType Directory -Path "$env:LOCALAPPDATA\weather\logs" -Force | Out-Null
    New-Item -ItemType Directory -Path "$env:LOCALAPPDATA\weather\cache\weather" -Force | Out-Null

    # Install binary
    $DestFile = Join-Path $InstallDir "weather.exe"
    Write-Host "📦 Installing to $DestFile..."
    Copy-Item $TempFile $DestFile -Force

    # Add to PATH if not already there
    $CurrentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    if ($CurrentPath -notlike "*$InstallDir*") {
        Write-Host "🔧 Adding to system PATH..."
        [Environment]::SetEnvironmentVariable(
            "Path",
            "$CurrentPath;$InstallDir",
            "Machine"
        )
    }

    Write-Host ""
    Write-Host "✅ Installation complete!" -ForegroundColor Green
    Write-Host ""

    if ($InstallService) {
        Write-Host "⚙️  Installing Windows Service..."

        # Create NSSM configuration (if NSSM is available)
        if (Get-Command nssm -ErrorAction SilentlyContinue) {
            nssm install Weather "$DestFile"
            nssm set Weather AppDirectory "$DataDir"
            nssm set Weather AppEnvironmentExtra "PORT=3000" "DATABASE_PATH=$DataDir\weather.db" "GIN_MODE=release"
            nssm set Weather Description "Weather Service - Beautiful weather API"
            nssm set Weather Start SERVICE_AUTO_START

            Write-Host "✓ Service installed. Start with: nssm start Weather" -ForegroundColor Green
        } else {
            Write-Host "⚠️  NSSM not found. To install as a service:" -ForegroundColor Yellow
            Write-Host "   1. Install NSSM: choco install nssm"
            Write-Host "   2. Re-run with -InstallService flag"
        }
    } else {
        Write-Host "Run: weather.exe"
        Write-Host "Or:  weather.exe --help"
        Write-Host ""
        Write-Host "To install as a Windows Service:"
        Write-Host "  Install NSSM: choco install nssm"
        Write-Host "  Then run: .\windows.ps1 -InstallService"
    }

} finally {
    # Cleanup
    if (Test-Path $TempDir) {
        Remove-Item -Recurse -Force $TempDir
    }
}
