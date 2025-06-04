# Bitunix Bot Environment Activation Script for Windows
# This script sets up the development environment

$ErrorActionPreference = "Stop"

# Colors for output
$Green = "`e[32m"
$Red = "`e[31m"
$Yellow = "`e[33m"
$NoColor = "`e[0m"

function Write-Log {
    param($Message)
    Write-Host "$Green[Setup]$NoColor $Message"
}

function Write-Warning {
    param($Message)
    Write-Host "$Yellow[Setup]$NoColor $Message"
}

function Write-Error {
    param($Message)
    Write-Host "$Red[ERROR]$NoColor $Message"
    exit 1
}

# Check if running as administrator
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Warning "This script should be run as Administrator for full functionality"
    Write-Warning "Some features may not work correctly"
}

# Check Go installation
try {
    $goVersion = go version
    Write-Log "Found Go: $goVersion"
} catch {
    Write-Error "Go is not installed or not in PATH. Please install Go 1.21+ first."
}

# Check Python installation
try {
    $pythonVersion = python --version
    Write-Log "Found Python: $pythonVersion"
} catch {
    Write-Error "Python is not installed or not in PATH. Please install Python 3.9+ first."
}

# Create and activate virtual environment
$venvPath = "venv"
if (-not (Test-Path $venvPath)) {
    Write-Log "Creating Python virtual environment..."
    python -m venv $venvPath
}

# Activate virtual environment
$activateScript = Join-Path $venvPath "Scripts\Activate.ps1"
if (Test-Path $activateScript) {
    Write-Log "Activating virtual environment..."
    . $activateScript
} else {
    Write-Error "Failed to find virtual environment activation script"
}

# Install Python dependencies
Write-Log "Installing Python dependencies..."
pip install -r requirements.txt

# Install Go dependencies
Write-Log "Installing Go dependencies..."
go mod download

# Create necessary directories
$directories = @("data", "logs", "models")
foreach ($dir in $directories) {
    if (-not (Test-Path $dir)) {
        Write-Log "Creating $dir directory..."
        New-Item -ItemType Directory -Path $dir | Out-Null
    }
}

# Check if config.yaml exists
if (-not (Test-Path "config.yaml")) {
    if (Test-Path "config.yaml.example") {
        Write-Log "Creating config.yaml from example..."
        Copy-Item "config.yaml.example" "config.yaml"
    } else {
        Write-Warning "config.yaml.example not found. Please create config.yaml manually."
    }
}

# Set up Windows Firewall rules
if ($isAdmin) {
    Write-Log "Setting up Windows Firewall rules..."
    
    # Allow incoming connections on port 8080 for metrics
    $ruleName = "BitunixBot-Metrics"
    $existingRule = Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue
    
    if (-not $existingRule) {
        New-NetFirewallRule -DisplayName $ruleName `
            -Direction Inbound `
            -LocalPort 8080 `
            -Protocol TCP `
            -Action Allow `
            -Profile Any
        Write-Log "Created firewall rule for metrics port 8080"
    } else {
        Write-Log "Firewall rule for metrics port 8080 already exists"
    }
} else {
    Write-Warning "Skipping firewall setup - run as Administrator to configure firewall rules"
}

Write-Log "Environment setup complete!"
Write-Log "To activate the environment in a new PowerShell session, run:"
Write-Host "    . .\activate.ps1" 