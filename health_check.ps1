# Bitunix Bot Health Check Script for Windows
# This script validates all the issues mentioned in TROUBLESHOOTING.md

$ErrorActionPreference = "Stop"

Write-Host "=== Bitunix Bot Health Check ==="
Write-Host "Timestamp: $(Get-Date)"
Write-Host ""

# Colors for output
$Red = "`e[31m"
$Green = "`e[32m"
$Yellow = "`e[33m"
$NoColor = "`e[0m"

# Helper functions
function Write-Success {
    param($Message)
    Write-Host "$Green✓ $Message$NoColor"
}

function Write-Warning {
    param($Message)
    Write-Host "$Yellow⚠ $Message$NoColor"
}

function Write-Error {
    param($Message)
    Write-Host "$Red✗ $Message$NoColor"
}

function Test-Command {
    param($Command)
    try {
        $null = Get-Command $Command -ErrorAction Stop
        Write-Success "$Command is available"
        return $true
    }
    catch {
        Write-Error "$Command is not available"
        return $false
    }
}

Write-Host "1. SYSTEM REQUIREMENTS CHECK"
Write-Host "================================"

# Check Go installation
if (Test-Command "go") {
    $goVersion = go version
    Write-Host "   $goVersion"
}

# Check Python installation
if (Test-Command "python") {
    $pythonVersion = python --version
    Write-Host "   $pythonVersion"
}

Write-Host ""

Write-Host "2. WINDOWS FIREWALL CHECK"
Write-Host "====================="

# Check Windows Firewall status
$firewallStatus = Get-NetFirewallProfile
$firewallEnabled = $false

foreach ($profile in $firewallStatus) {
    if ($profile.Enabled) {
        $firewallEnabled = $true
        Write-Warning "Windows Firewall is active for profile: $($profile.Name)"
    }
}

if (-not $firewallEnabled) {
    Write-Success "Windows Firewall is inactive - no firewall restrictions"
}

# Check if port 8080 is open
$port8080 = Get-NetFirewallRule | Where-Object { $_.LocalPort -eq 8080 }
if ($port8080) {
    Write-Success "Port 8080 is configured in Windows Firewall"
}
else {
    Write-Warning "Port 8080 not explicitly allowed (may still work)"
}

Write-Host ""

Write-Host "3. NETWORK CONNECTIVITY CHECK"
Write-Host "=============================="

# Test DNS resolution
try {
    $dnsResult = Resolve-DnsName -Name api.bitunix.com -ErrorAction Stop
    Write-Success "DNS resolution for api.bitunix.com works"
}
catch {
    Write-Error "DNS resolution for api.bitunix.com failed"
}

# Test HTTPS connectivity
try {
    $response = Invoke-WebRequest -Uri https://api.bitunix.com -UseBasicParsing -TimeoutSec 10
    Write-Success "HTTPS connectivity to api.bitunix.com works"
}
catch {
    Write-Error "HTTPS connectivity to api.bitunix.com failed"
}

Write-Host ""

Write-Host "4. ENVIRONMENT VARIABLES CHECK"
Write-Host "==============================="

# Check for API credentials
$apiKeySet = $false
$secretKeySet = $false
$forceLiveTrading = $false

if ($env:BITUNIX_API_KEY) {
    Write-Success "BITUNIX_API_KEY is set"
    $apiKeySet = $true
}
else {
    Write-Error "BITUNIX_API_KEY is not set"
}

if ($env:BITUNIX_SECRET_KEY) {
    Write-Success "BITUNIX_SECRET_KEY is set"
    $secretKeySet = $true
}
else {
    Write-Error "BITUNIX_SECRET_KEY is not set"
}

if ($env:FORCE_LIVE_TRADING -eq "true") {
    Write-Warning "FORCE_LIVE_TRADING is set to true - LIVE TRADING ENABLED"
    $forceLiveTrading = $true
}
else {
    Write-Success "FORCE_LIVE_TRADING is not set - Live trading disabled"
}

Write-Host ""

Write-Host "5. CONFIGURATION FILES CHECK"
Write-Host "============================="

# Check config file
if (Test-Path "config.yaml") {
    Write-Success "config.yaml exists"
    
    # Validate YAML syntax and content
    try {
        $config = python -c "import yaml; yaml.safe_load(open('config.yaml'))"
        Write-Success "config.yaml has valid YAML syntax"
        
        # Check for empty API keys
        if ($config.api.key -eq "" -and $config.api.secret -eq "") {
            Write-Success "API keys are empty in config (should be set via env vars)"
        }
        else {
            Write-Error "API keys should not be set in config.yaml - use environment variables instead"
        }
        
        # Check dry run setting
        if ($config.trading.dryRun -eq $true) {
            Write-Success "dryRun is enabled (safe mode)"
        }
        else {
            if ($forceLiveTrading) {
                Write-Warning "Live trading is enabled (dryRun=false and FORCE_LIVE_TRADING=true)"
            }
            else {
                Write-Error "Live trading attempted without FORCE_LIVE_TRADING=true"
            }
        }
        
        # Validate trading parameters
        if ($config.trading.maxPositionSize -gt 0.1) {
            Write-Error "maxPositionSize too high (>10%)"
        }
        if ($config.trading.maxDailyLoss -gt 0.05) {
            Write-Error "maxDailyLoss too high (>5%)"
        }
        
    }
    catch {
        Write-Error "config.yaml has invalid YAML syntax or content"
    }
}
else {
    Write-Warning "config.yaml not found"
    
    if (Test-Path "config.yaml.example") {
        Write-Warning "Use: Copy-Item config.yaml.example config.yaml"
    }
}

Write-Host ""

Write-Host "6. ML MODEL CHECK"
Write-Host "================"

# Check model file
if (Test-Path "models/model.onnx") {
    Write-Success "ML model file exists at models/model.onnx"
    
    # Check if it's a symlink
    $item = Get-Item "models/model.onnx"
    if ($item.LinkType) {
        $target = $item.Target
        if (Test-Path "models/$target") {
            Write-Success "Model symlink points to valid file: $target"
        }
        else {
            Write-Error "Model symlink is broken: $target"
        }
    }
}
else {
    Write-Warning "ML model file not found at models/model.onnx"
}

# Check Python dependencies for ML
try {
    $onnxVersion = python -c "import onnxruntime; print('ONNX Runtime version:', onnxruntime.__version__)"
    Write-Success "ONNX Runtime is available: $onnxVersion"
}
catch {
    Write-Error "ONNX Runtime is not available"
    Write-Host "   Install with: pip install onnxruntime==1.16.1"
}

try {
    $numpyVersion = python -c "import numpy; print('NumPy version:', numpy.__version__)"
    Write-Success "NumPy is available: $numpyVersion"
}
catch {
    Write-Error "NumPy is not available"
}

Write-Host ""

Write-Host "7. DATABASE CHECK"
Write-Host "================"

# Check data directory
if (Test-Path "data") {
    Write-Success "data directory exists"
}
else {
    Write-Warning "data directory not found - will be created on startup"
}

# Check database file
if (Test-Path "data/bitunix-data.db") {
    Write-Success "Database file exists"
    $dbSize = (Get-Item "data/bitunix-data.db").Length
    Write-Host "   Database size: $([math]::Round($dbSize/1MB, 2)) MB"
}
else {
    Write-Warning "Database file not found - will be created on startup"
} 