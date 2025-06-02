#!/bin/bash

# ML Pipeline Runner with Virtual Environment
# This script ensures the virtual environment is active before running ML operations

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_PATH="$SCRIPT_DIR/venv"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[ML Pipeline]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[ML Pipeline]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Check Python installation
if ! command -v python3 &> /dev/null; then
    error "Python 3 is not installed. Please install Python 3.9+ first."
fi

# Install python3-venv if not available
if ! python3 -c "import venv" 2>/dev/null; then
    log "Installing python3-venv..."
    if command -v apt &> /dev/null; then
        sudo apt update && sudo apt install -y python3-venv python3-full
    elif command -v apk &> /dev/null; then
        sudo apk add python3-dev
    else
        error "Could not install python3-venv. Please install manually."
    fi
fi

# Check/create virtual environment
if [[ ! -d "$VENV_PATH" ]]; then
    log "Creating Python virtual environment..."
    python3 -m venv "$VENV_PATH"
fi

# Activate virtual environment
log "Activating virtual environment..."
source "$VENV_PATH/bin/activate"

# Verify activation
if [[ -z "$VIRTUAL_ENV" ]]; then
    error "Failed to activate virtual environment"
fi

log "Virtual environment active: $VIRTUAL_ENV"

# Forward to the actual script with all arguments
log "Running ML pipeline setup..."
exec "$SCRIPT_DIR/scripts/setup_ml_pipeline.sh" "$@"
