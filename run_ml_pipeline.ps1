# ML Pipeline Runner with Virtual Environment for Windows
# This script ensures the virtual environment is active before running ML operations

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$VenvPath = Join-Path $ScriptDir "venv"

# Colors for output
$Green = "`e[32m"
$Red = "`e[31m"
$Yellow = "`e[33m"
$NoColor = "`e[0m"

function Write-Log {
    param($Message)
    Write-Host "$Green[ML Pipeline]$NoColor $Message"
}

function Write-Warning {
    param($Message)
    Write-Host "$Yellow[ML Pipeline]$NoColor $Message"
}

function Write-Error {
    param($Message)
    Write-Host "$Red[ERROR]$NoColor $Message"
    exit 1
}

# Check Python installation
try {
    $pythonVersion = python --version
    Write-Log "Found Python: $pythonVersion"
} catch {
    Write-Error "Python is not installed or not in PATH. Please install Python 3.9+ first."
}

# Check/create virtual environment
if (-not (Test-Path $VenvPath)) {
    Write-Log "Creating Python virtual environment..."
    python -m venv $VenvPath
}

# Activate virtual environment
Write-Log "Activating virtual environment..."
$ActivateScript = Join-Path $VenvPath "Scripts\Activate.ps1"
if (Test-Path $ActivateScript) {
    . $ActivateScript
} else {
    Write-Error "Failed to find virtual environment activation script"
}

# Verify activation
if (-not $env:VIRTUAL_ENV) {
    Write-Error "Failed to activate virtual environment"
}

Write-Log "Virtual environment active: $env:VIRTUAL_ENV"

# Forward to the actual script with all arguments
Write-Log "Running ML pipeline setup..."
$SetupScript = Join-Path $ScriptDir "scripts\setup_ml_pipeline.ps1"
if (Test-Path $SetupScript) {
    & $SetupScript $args
} else {
    Write-Error "Setup script not found at: $SetupScript"
} 