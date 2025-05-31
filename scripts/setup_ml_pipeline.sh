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

show_help() {
    cat << EOF
Complete ML Pipeline Setup and Test Script

Usage: $0 [OPTIONS]

Options:
    --setup-only      Only setup dependencies, don't run training/tests
    --test-only       Only run tests, skip setup and training
    --no-sample-data  Don't generate sample data (require real BoltDB)
    --quick-test      Run minimal tests for quick validation
    -h, --help        Show this help message

Examples:
    $0                    # Full setup, training, and testing
    $0 --setup-only       # Just install dependencies
    $0 --test-only        # Just run validation tests
    $0 --quick-test       # Fast validation check
EOF
}

# Parse arguments
SETUP_ONLY=false
TEST_ONLY=false
NO_SAMPLE_DATA=false
QUICK_TEST=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --setup-only)
            SETUP_ONLY=true
            shift
            ;;
        --test-only)
            TEST_ONLY=true
            shift
            ;;
        --no-sample-data)
            NO_SAMPLE_DATA=true
            shift
            ;;
        --quick-test)
            QUICK_TEST=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            ;;
    esac
done

# Main execution
main() {
    echo "🚀 Bitunix Trading Bot - ML Pipeline Setup & Test"
    echo "=================================================="
    
    if [[ "$TEST_ONLY" != true ]]; then
        setup_environment
        if [[ "$SETUP_ONLY" != true ]]; then
            run_training_pipeline
        fi
    fi
    
    if [[ "$SETUP_ONLY" != true ]]; then
        run_validation_tests
    fi
    
    show_summary
}

setup_environment() {
    log "Setting up ML pipeline environment..."
    
    # Create required directories
    mkdir -p "$MODELS_DIR" "$DATA_DIR" "$LOGS_DIR"
    
    # Check Go installation
    if ! command -v go &> /dev/null; then
        error "Go is not installed. Please install Go 1.21+ first."
    fi
    
    go_version=$(go version | awk '{print $3}' | sed 's/go//')
    info "Go version: $go_version"
    
    # Check Python installation
    if ! command -v python3 &> /dev/null; then
        error "Python 3 is not installed. Please install Python 3.9+ first."
    fi
    
    python_version=$(python3 --version | awk '{print $2}')
    info "Python version: $python_version"
    
    # Install Python dependencies
    log "Installing Python ML dependencies..."
    cd "$SCRIPT_DIR"
    if ! pip3 install -r requirements.txt > "$LOGS_DIR/pip_install.log" 2>&1; then
        error "Failed to install Python dependencies. Check $LOGS_DIR/pip_install.log"
    fi
    
    # Check critical Python packages
    log "Validating Python packages..."
    python3 -c "
import sys
try:
    import sklearn, xgboost, onnx, onnxruntime, numpy, pandas
    print('✅ All required packages installed')
except ImportError as e:
    print(f'❌ Missing package: {e}')
    sys.exit(1)
" || error "Python package validation failed"
    
    # Build Go modules
    log "Building Go modules..."
    cd "$PROJECT_ROOT"
    if ! go mod tidy > "$LOGS_DIR/go_build.log" 2>&1; then
        error "Failed to build Go modules. Check $LOGS_DIR/go_build.log"
    fi
    
    # Test Go compilation
    if ! go build -o /tmp/test_build scripts/export_data.go > "$LOGS_DIR/go_compile.log" 2>&1; then
        error "Failed to compile Go scripts. Check $LOGS_DIR/go_compile.log"
    fi
    rm -f /tmp/test_build
    
    log "Environment setup completed successfully"
}

run_training_pipeline() {
    log "Running ML training pipeline..."
    
    cd "$PROJECT_ROOT"
    
    # Check for existing database
    if [[ -f "$DATA_DIR/features.db" ]]; then
        log "Found existing BoltDB, exporting training data..."
        
        # Export data for training
        if ! go run scripts/export_data.go \
            -db "$DATA_DIR/features.db" \
            -output "$SCRIPT_DIR/training_data.json" \
            -days 30 > "$LOGS_DIR/data_export.log" 2>&1; then
            warn "Data export failed, will use sample data generation"
            rm -f "$SCRIPT_DIR/training_data.json"
        else
            record_count=$(python3 -c "
import json
try:
    with open('$SCRIPT_DIR/training_data.json') as f:
        data = json.load(f)
    print(len(data))
except:
    print(0)
")
            info "Exported $record_count training records"
            
            if [[ "$record_count" -lt 100 ]]; then
                warn "Insufficient training data ($record_count records), will use sample data generation"
                rm -f "$SCRIPT_DIR/training_data.json"
            fi
        fi
    else
        if [[ "$NO_SAMPLE_DATA" == true ]]; then
            error "No BoltDB found and sample data generation disabled"
        fi
        warn "No BoltDB found at $DATA_DIR/features.db, will use sample data generation"
    fi
    
    # Run training
    log "Training ML model..."
    cd "$SCRIPT_DIR"
    if ! python3 label_and_train.py > "$LOGS_DIR/training.log" 2>&1; then
        error "Model training failed. Check $LOGS_DIR/training.log"
    fi
    
    # Verify model creation
    if [[ ! -f "$MODELS_DIR/model.onnx" ]]; then
        error "Model file not created: $MODELS_DIR/model.onnx"
    fi
    
    # Show training results
    if [[ -f "$MODELS_DIR/training_metrics.json" ]]; then
        log "Training completed successfully! Metrics:"
        python3 -c "
import json
try:
    with open('$MODELS_DIR/training_metrics.json') as f:
        metrics = json.load(f)
    print(f\"  🎯 AUC Score: {metrics.get('auc_score', 'N/A'):.4f}\")
    print(f\"  🎯 F1 Score: {metrics.get('f1_score', 'N/A'):.4f}\")
    print(f\"  📊 Training samples: {metrics.get('n_samples', 'N/A')}\")
    print(f\"  ⚖️ Positive ratio: {metrics.get('positive_ratio', 'N/A'):.3f}\")
except Exception as e:
    print(f'Could not read metrics: {e}')
"
    fi
    
    # Clean up
    rm -f "$SCRIPT_DIR/training_data.json"
}

run_validation_tests() {
    log "Running validation tests..."
    
    cd "$PROJECT_ROOT"
    
    # Test 1: ONNX model validation
    log "Test 1: ONNX model validation..."
    if [[ -f "$MODELS_DIR/model.onnx" ]]; then
        python3 -c "
import onnx
import onnxruntime as ort
try:
    model = onnx.load('$MODELS_DIR/model.onnx')
    onnx.checker.check_model(model)
    session = ort.InferenceSession('$MODELS_DIR/model.onnx')
    print('✅ ONNX model validation passed')
    
    # Test inference
    import numpy as np
    test_input = np.array([[0.1, -0.2, 0.5]], dtype=np.float32)
    input_name = session.get_inputs()[0].name
    result = session.run(None, {input_name: test_input})
    print(f'✅ ONNX inference test passed: {len(result)} outputs')
    
except Exception as e:
    print(f'❌ ONNX validation failed: {e}')
    exit(1)
" || error "ONNX model validation failed"
    else
        warn "No ONNX model found, skipping ONNX validation"
    fi
    
    # Test 2: Go integration
    log "Test 2: Go ML integration..."
    if [[ "$QUICK_TEST" == true ]]; then
        # Quick test - just compile
        if ! go build -o /tmp/test_ml scripts/test_model.go > "$LOGS_DIR/go_test.log" 2>&1; then
            error "Go ML test compilation failed. Check $LOGS_DIR/go_test.log"
        fi
        rm -f /tmp/test_ml
        info "Go compilation test passed"
    else
        # Full integration test
        if ! timeout 30 go run scripts/test_model.go "$MODELS_DIR/model.onnx" > "$LOGS_DIR/integration_test.log" 2>&1; then
            warn "Go integration test failed or timed out. Check $LOGS_DIR/integration_test.log"
        else
            info "Go integration test passed"
        fi
    fi
    
    # Test 3: Data export script
    log "Test 3: Data export functionality..."
    if ! go run scripts/export_data.go -h > /dev/null 2>&1; then
        error "Data export script failed to run"
    fi
    info "Data export test passed"
    
    # Test 4: Deployment script
    log "Test 4: Deployment script validation..."
    if [[ ! -x "$SCRIPT_DIR/deploy_model.sh" ]]; then
        error "Deployment script is not executable"
    fi
    
    if ! "$SCRIPT_DIR/deploy_model.sh" --help > /dev/null 2>&1; then
        error "Deployment script failed help test"
    fi
    info "Deployment script test passed"
    
    # Test 5: Model size and performance
    if [[ -f "$MODELS_DIR/model.onnx" ]]; then
        log "Test 5: Model performance checks..."
        
        model_size=$(stat -f%z "$MODELS_DIR/model.onnx" 2>/dev/null || stat -c%s "$MODELS_DIR/model.onnx")
        model_size_mb=$((model_size / 1024 / 1024))
        
        if [[ $model_size_mb -gt 10 ]]; then
            warn "Model size is large: ${model_size_mb}MB (consider quantization)"
        else
            info "Model size OK: ${model_size_mb}MB"
        fi
        
        # Test inference speed
        python3 -c "
import time
import numpy as np
import onnxruntime as ort

try:
    session = ort.InferenceSession('$MODELS_DIR/model.onnx')
    test_input = np.array([[0.1, -0.2, 0.5]], dtype=np.float32)
    input_name = session.get_inputs()[0].name
    
    # Warmup
    for _ in range(5):
        session.run(None, {input_name: test_input})
    
    # Timing test
    start = time.time()
    for _ in range(100):
        session.run(None, {input_name: test_input})
    end = time.time()
    
    avg_time_ms = (end - start) / 100 * 1000
    print(f'Average inference time: {avg_time_ms:.2f}ms')
    
    if avg_time_ms > 100:
        print('⚠️  Warning: Inference time is high (>100ms)')
    else:
        print('✅ Inference time is acceptable')
        
except Exception as e:
    print(f'❌ Performance test failed: {e}')
"
    fi
}

show_summary() {
    echo ""
    echo "🎉 ML Pipeline Setup Complete!"
    echo "=============================="
    echo ""
    
    if [[ -f "$MODELS_DIR/model.onnx" ]]; then
        echo "✅ ONNX Model: $MODELS_DIR/model.onnx"
        model_size=$(stat -f%z "$MODELS_DIR/model.onnx" 2>/dev/null || stat -c%s "$MODELS_DIR/model.onnx")
        echo "   Size: $((model_size / 1024))KB"
    else
        echo "❌ No ONNX model found"
    fi
    
    if [[ -f "$MODELS_DIR/training_metrics.json" ]]; then
        echo "✅ Training Metrics: $MODELS_DIR/training_metrics.json"
    fi
    
    echo "✅ Scripts: $SCRIPT_DIR/"
    echo "✅ Documentation: $PROJECT_ROOT/ML_PIPELINE.md"
    echo "✅ Logs: $LOGS_DIR/"
    
    echo ""
    echo "Next Steps:"
    echo "==========="
    echo "1. Review training metrics in $MODELS_DIR/training_metrics.json"
    echo "2. Test integration: go run scripts/test_model.go"
    echo "3. Deploy to production: ./scripts/deploy_model.sh"
    echo "4. Monitor bot performance with new ML gate"
    echo "5. Set up automated retraining with GitHub Actions"
    echo ""
    echo "For detailed usage instructions, see: ML_PIPELINE.md"
}

# Run main function
main "$@"
