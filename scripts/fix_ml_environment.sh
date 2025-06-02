#!/bin/bash
# One-click ML environment fix for Bitunix Bot

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[FIX]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Step 1: Check Python
log "Checking Python installation..."
if ! command -v python3 &> /dev/null; then
    error "Python 3 not found. Please install Python 3.8+"
fi

PYTHON_VERSION=$(python3 -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')
log "Found Python $PYTHON_VERSION"

# Step 2: Install system dependencies
if command -v apt-get &> /dev/null; then
    log "Installing system dependencies..."
    sudo apt-get update
    sudo apt-get install -y python3-dev python3-venv python3-pip build-essential
elif command -v yum &> /dev/null; then
    sudo yum install -y python3-devel gcc gcc-c++ make
fi

# Step 3: Create/activate virtual environment
cd "$PROJECT_ROOT"
if [ ! -d "venv" ]; then
    log "Creating virtual environment..."
    python3 -m venv venv
fi

log "Activating virtual environment..."
source venv/bin/activate

# Step 4: Upgrade pip
log "Upgrading pip..."
pip install --upgrade pip setuptools wheel

# Step 5: Install ONNX Runtime with retry
log "Installing ONNX Runtime..."
for i in {1..3}; do
    if pip install onnxruntime==1.17.3; then
        break
    else
        warn "Attempt $i failed, retrying..."
        sleep 2
    fi
done

# Step 6: Install all dependencies
log "Installing ML dependencies..."
cd "$SCRIPT_DIR"
pip install -r requirements.txt

# Step 7: Verify installation
log "Verifying installation..."
cd "$PROJECT_ROOT"
python3 scripts/verify_ml_setup.py

log "✅ ML environment setup complete!"
log "To activate the environment in the future, run:"
log "  source venv/bin/activate"
