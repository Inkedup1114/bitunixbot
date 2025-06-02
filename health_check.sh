#!/bin/bash

# Bitunix Bot Health Check Script
# This script validates all the issues mentioned in TROUBLESHOOTING.md

set -e

echo "=== Bitunix Bot Health Check ==="
echo "Timestamp: $(date)"
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
success() {
    echo -e "${GREEN}✓ $1${NC}"
}

warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

error() {
    echo -e "${RED}✗ $1${NC}"
}

check_command() {
    if command -v "$1" >/dev/null 2>&1; then
        success "$1 is available"
        return 0
    else
        error "$1 is not available"
        return 1
    fi
}

echo "1. SYSTEM REQUIREMENTS CHECK"
echo "================================"

# Check Go installation
if check_command go; then
    echo "   Go version: $(go version)"
fi

# Check Python installation  
if check_command python3; then
    echo "   Python version: $(python3 --version)"
fi

echo

echo "2. UFW FIREWALL CHECK"
echo "====================="

# Check UFW status
if command -v ufw >/dev/null 2>&1; then
    UFW_STATUS=$(sudo ufw status | head -1)
    if [[ "$UFW_STATUS" == *"active"* ]]; then
        warning "UFW is active - checking rules"
        
        # Check if outgoing connections are allowed
        UFW_OUTGOING=$(sudo ufw status verbose | grep "Default:" | grep "allow (outgoing)")
        if [[ -n "$UFW_OUTGOING" ]]; then
            success "Outgoing connections allowed by default"
        else
            error "Outgoing connections may be blocked"
        fi
        
        # Check if port 8080 is open for metrics
        UFW_8080=$(sudo ufw status | grep "8080")
        if [[ -n "$UFW_8080" ]]; then
            success "Port 8080 is open for metrics"
        else
            warning "Port 8080 not explicitly allowed (may still work)"
        fi
    else
        success "UFW is inactive - no firewall restrictions"
    fi
else
    success "UFW not installed - no firewall restrictions"
fi

echo

echo "3. NETWORK CONNECTIVITY CHECK"
echo "=============================="

# Test DNS resolution
if nslookup api.bitunix.com >/dev/null 2>&1; then
    success "DNS resolution for api.bitunix.com works"
else
    error "DNS resolution for api.bitunix.com failed"
fi

# Test HTTPS connectivity
if curl -s --connect-timeout 10 https://api.bitunix.com >/dev/null 2>&1; then
    success "HTTPS connectivity to api.bitunix.com works"
else
    error "HTTPS connectivity to api.bitunix.com failed"
fi

echo

echo "4. ENVIRONMENT VARIABLES CHECK"
echo "==============================="

# Check for API credentials
if [[ -n "$BITUNIX_API_KEY" ]]; then
    success "BITUNIX_API_KEY is set"
else
    warning "BITUNIX_API_KEY is not set"
fi

if [[ -n "$BITUNIX_SECRET_KEY" ]]; then
    success "BITUNIX_SECRET_KEY is set"
else
    warning "BITUNIX_SECRET_KEY is not set"
fi

echo

echo "5. CONFIGURATION FILES CHECK"
echo "============================="

# Check config file
if [[ -f "config.yaml" ]]; then
    success "config.yaml exists"
    
    # Validate YAML syntax
    if python3 -c "import yaml; yaml.safe_load(open('config.yaml'))" 2>/dev/null; then
        success "config.yaml has valid YAML syntax"
    else
        error "config.yaml has invalid YAML syntax"
    fi
else
    warning "config.yaml not found"
    
    if [[ -f "config.yaml.example" ]]; then
        warning "Use: cp config.yaml.example config.yaml"
    fi
fi

echo

echo "6. ML MODEL CHECK"
echo "================"

# Check model file
if [[ -f "models/model.onnx" ]]; then
    success "ML model file exists at models/model.onnx"
    
    # Check if it's a symlink
    if [[ -L "models/model.onnx" ]]; then
        LINK_TARGET=$(readlink "models/model.onnx")
        if [[ -f "models/$LINK_TARGET" ]]; then
            success "Model symlink points to valid file: $LINK_TARGET"
        else
            error "Model symlink is broken: $LINK_TARGET"
        fi
    fi
else
    warning "ML model file not found at models/model.onnx"
fi

# Check Python dependencies for ML
if python3 -c "import onnxruntime; print('ONNX Runtime version:', onnxruntime.__version__)" 2>/dev/null; then
    success "ONNX Runtime is available"
else
    error "ONNX Runtime is not available"
    echo "   Install with: pip install onnxruntime==1.16.1"
fi

if python3 -c "import numpy; print('NumPy version:', numpy.__version__)" 2>/dev/null; then
    success "NumPy is available"
else
    error "NumPy is not available"
fi

echo

echo "7. DATABASE CHECK"
echo "================"

# Check data directory
if [[ -d "data" ]]; then
    success "data directory exists"
else
    warning "data directory not found - will be created on startup"
fi

# Check database file
if [[ -f "data/bitunix-data.db" ]]; then
    success "Database file exists"
    DB_SIZE=$(du -h "data/bitunix-data.db" | cut -f1)
    echo "   Database size: $DB_SIZE"
else
    warning "Database file not found - will be created on startup"
fi

echo

echo "8. BUILD CHECK"
echo "=============="

# Try to build the project
if go build -o /tmp/bitrader cmd/bitrader/main.go 2>/dev/null; then
    success "Project builds successfully"
    rm -f /tmp/bitrader
else
    error "Project build failed"
    echo "   Run: go mod tidy && go build cmd/bitrader/main.go"
fi

echo

echo "9. PORT AVAILABILITY CHECK"
echo "=========================="

# Check if port 8080 is in use
if lsof -i :8080 >/dev/null 2>&1; then
    warning "Port 8080 is already in use"
    echo "   Processes using port 8080:"
    lsof -i :8080 2>/dev/null | tail -n +2 | awk '{print "   " $1 " (PID: " $2 ")"}'
else
    success "Port 8080 is available"
fi

echo

echo "10. QUICK FUNCTIONAL TEST"
echo "========================="

# Test dry run mode
echo "Testing bot startup in dry-run mode..."
if timeout 10 go run cmd/bitrader/main.go -dry-run=true 2>&1 | grep -q "Starting.*dry.*run"; then
    success "Bot starts successfully in dry-run mode"
else
    warning "Bot startup test inconclusive (timeout or no dry-run message)"
fi

echo

echo "=== HEALTH CHECK COMPLETE ==="
echo

echo "NEXT STEPS:"
echo "==========="
echo "1. Fix any errors shown above"
echo "2. Set API credentials if missing:"
echo "   export BITUNIX_API_KEY='your_key'"
echo "   export BITUNIX_SECRET_KEY='your_secret'"
echo "3. Start the bot:"
echo "   go run cmd/bitrader/main.go"
echo "4. Monitor metrics:"
echo "   curl http://localhost:8080/metrics"
echo
