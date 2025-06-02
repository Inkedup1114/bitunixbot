#!/bin/bash

# Complete ML Pipeline Setup and Test Script
# This script sets up the entire ML pipeline from scratch and runs validation tests

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
MODELS_DIR="$PROJECT_ROOT/models"
DATA_DIR="$PROJECT_ROOT/data"
LOGS_DIR="$PROJECT_ROOT/logs"

log() {
    echo -e "${GREEN}[$(date +'%H:%M:%S')]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[$(date +'%H:%M:%S')] WARNING:${NC} $1"
}

error() {
    echo -e "${RED}[$(date +'%H:%M:%S')] ERROR:${NC} $1"
    exit 1
}

info() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')] INFO:${NC} $1"
}

# Check dependencies
check_dependencies() {
    log "Checking dependencies..."
    
    # Check Python
    if ! command -v python3 &> /dev/null; then
        error "Python3 is not installed"
    fi
    
    # Check Python version
    PYTHON_VERSION=$(python3 -c 'import sys; print(".".join(map(str, sys.version_info[:2])))')
    log "Python version: $PYTHON_VERSION"
    
    # Check if version is suitable for ONNX Runtime
    if ! python3 -c "import sys; exit(0 if 3.8 <= sys.version_info[:2] <= (3, 12) else 1)"; then
        warn "Python version $PYTHON_VERSION may not be fully compatible with ONNX Runtime"
        warn "Recommended versions: 3.8, 3.9, 3.10, 3.11, or 3.12"
    fi
    
    # Check pip
    if ! command -v pip3 &> /dev/null; then
        error "pip3 is not installed"
    else
        PIP_VERSION=$(pip3 --version | awk '{print $2}')
        log "pip3 version: $PIP_VERSION"
    fi
}

# Setup Python environment
setup_python_env() {
    if [ -z "$VIRTUAL_ENV" ]; then
        # Try to find existing venv
        if [ -d "$PROJECT_ROOT/venv" ]; then
            log "Found existing virtual environment at $PROJECT_ROOT/venv"
            log "Activating virtual environment..."
            source "$PROJECT_ROOT/venv/bin/activate"
        elif [ -d "$PROJECT_ROOT/.venv" ]; then
            log "Found existing virtual environment at $PROJECT_ROOT/.venv"
            source "$PROJECT_ROOT/.venv/bin/activate"
        else
            log "No virtual environment found. Creating new one..."
            python3 -m venv "$PROJECT_ROOT/venv" || {
                error "Failed to create virtual environment. Installing python3-venv..."
                sudo apt-get update && sudo apt-get install -y python3-venv
                python3 -m venv "$PROJECT_ROOT/venv"
            }
            source "$PROJECT_ROOT/venv/bin/activate"
        fi
    else
        log "Virtual environment already active: $VIRTUAL_ENV"
    fi
    
    # Verify activation
    if [ -z "$VIRTUAL_ENV" ]; then
        error "Failed to activate virtual environment"
    fi
    
    # Upgrade pip and install wheel
    log "Upgrading pip and setuptools..."
    pip install --upgrade pip setuptools wheel
}

create_venv() {
    log "Creating new virtual environment..."
    python3 -m venv "$PROJECT_ROOT/venv" || error "Failed to create virtual environment"
    source "$PROJECT_ROOT/venv/bin/activate" || error "Failed to activate virtual environment"
    log "Virtual environment created and activated"
}

# Create directories
create_directories() {
    log "Creating required directories..."
    
    mkdir -p "$MODELS_DIR"
    mkdir -p "$DATA_DIR"
    mkdir -p "$LOGS_DIR"
    
    log "Directories created"
}

# Run tests
run_tests() {
    log "Running ML pipeline tests..."
    
    # Ensure we're in the virtual environment
    if [ -z "$VIRTUAL_ENV" ]; then
        warn "Not in virtual environment, activating..."
        source "$PROJECT_ROOT/venv/bin/activate" || warn "Failed to activate venv"
    fi
    
    # Test data export
    if [ -f "$DATA_DIR/features.db" ]; then
        go run "$SCRIPT_DIR/export_data.go" -db "$DATA_DIR/features.db" -days 1 -output "$SCRIPT_DIR/test_export.json" || warn "Data export test failed"
    fi
    
    # Test model training with sample data
    cd "$SCRIPT_DIR"
    python3 label_and_train.py --test-mode || error "Model training test failed"
    
    # Test model integration
    if [ -f "$MODELS_DIR/model.onnx" ]; then
        go run "$SCRIPT_DIR/test_model.go" "$MODELS_DIR/model.onnx" || warn "Model integration test failed"
    fi
    
    log "Tests completed"
}

# Main setup
main() {
    log "Starting ML pipeline setup..."
    
    check_dependencies
    create_directories
    setup_python_env
    
    if [ "${1:-}" = "--test" ]; then
        run_tests
    fi
    
    log "ML pipeline setup completed successfully!"
    info "Next steps:"
    info "1. Export training data: go run scripts/export_data.go -days 30"
    info "2. Train model: cd scripts && python3 label_and_train.py"
    info "3. Deploy model: ./scripts/deploy_model.sh"
    
    # Show activation command if not in venv
    if [ -z "$VIRTUAL_ENV" ]; then
        info ""
        info "To activate the virtual environment for future use:"
        info "  source $PROJECT_ROOT/venv/bin/activate"
    fi
}

# Run main
main "$@"
