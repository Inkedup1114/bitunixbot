#!/bin/bash

# ML Model Deployment Script for Bitunix Trading Bot
# This script handles the complete ML pipeline: data export, training, and deployment

set -e  # Exit on any error

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
MODELS_DIR="$PROJECT_ROOT/models"
DATA_DIR="$PROJECT_ROOT/data"
LOGS_DIR="$PROJECT_ROOT/logs"

# Create directories if they don't exist
mkdir -p "$MODELS_DIR" "$LOGS_DIR"

# Default values
DB_PATH="$DATA_DIR/features.db"
TRAINING_DAYS=30
SYMBOL=""
RESTART_BOT=true
BACKUP_MODEL=true

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging function
log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING:${NC} $1"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR:${NC} $1"
    exit 1
}

# Help function
show_help() {
    cat << EOF
ML Model Deployment Script for Bitunix Trading Bot

Usage: $0 [OPTIONS]

Options:
    -d, --days DAYS         Number of days of data to use for training (default: 30)
    -s, --symbol SYMBOL     Train model for specific symbol only
    -b, --db-path PATH      Path to BoltDB database (default: data/features.db)
    --no-restart           Don't restart the bot after deployment
    --no-backup            Don't backup existing model
    -h, --help             Show this help message

Examples:
    $0                      # Train with default settings (30 days, all symbols)
    $0 -d 7 -s BTCUSDT     # Train with 7 days of BTCUSDT data
    $0 --no-restart        # Train but don't restart bot
EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--days)
            TRAINING_DAYS="$2"
            shift 2
            ;;
        -s|--symbol)
            SYMBOL="$2"
            shift 2
            ;;
        -b|--db-path)
            DB_PATH="$2"
            shift 2
            ;;
        --no-restart)
            RESTART_BOT=false
            shift
            ;;
        --no-backup)
            BACKUP_MODEL=false
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

# Validate inputs
if [[ ! -f "$DB_PATH" ]]; then
    error "Database file not found: $DB_PATH"
fi

if [[ $TRAINING_DAYS -lt 1 ]]; then
    error "Training days must be at least 1"
fi

# Start deployment
log "Starting ML model deployment..."
log "Configuration:"
log "  Database: $DB_PATH"
log "  Training days: $TRAINING_DAYS"
log "  Symbol filter: ${SYMBOL:-'all'}"
log "  Restart bot: $RESTART_BOT"
log "  Backup model: $BACKUP_MODEL"

# Step 1: Export data from BoltDB
log "Step 1: Exporting training data..."
export_cmd="go run $SCRIPT_DIR/export_data.go -db '$DB_PATH' -output '$SCRIPT_DIR/training_data.json' -days $TRAINING_DAYS"
if [[ -n "$SYMBOL" ]]; then
    export_cmd="$export_cmd -symbol '$SYMBOL'"
fi

cd "$PROJECT_ROOT"
if ! eval "$export_cmd" > "$LOGS_DIR/export.log" 2>&1; then
    error "Data export failed. Check $LOGS_DIR/export.log for details."
fi
log "Data export completed successfully"

# Step 2: Check if training data exists and has content
if [[ ! -f "$SCRIPT_DIR/training_data.json" ]]; then
    error "Training data file not created: $SCRIPT_DIR/training_data.json"
fi

# Check if JSON file has data
if ! python3 -c "import json; data=json.load(open('$SCRIPT_DIR/training_data.json')); exit(0 if len(data) > 100 else 1)" 2>/dev/null; then
    warn "Training data has fewer than 100 records. Model quality may be poor."
fi

# Step 3: Backup existing model
if [[ "$BACKUP_MODEL" == true && -f "$MODELS_DIR/model.onnx" ]]; then
    backup_name="model_backup_$(date +%Y%m%d_%H%M%S).onnx"
    log "Step 2: Backing up existing model to $backup_name..."
    cp "$MODELS_DIR/model.onnx" "$MODELS_DIR/$backup_name"
fi

# Step 4: Install Python dependencies
log "Step 3: Installing Python dependencies..."
cd "$SCRIPT_DIR"
if ! pip3 install -r requirements.txt > "$LOGS_DIR/pip_install.log" 2>&1; then
    error "Failed to install Python dependencies. Check $LOGS_DIR/pip_install.log for details."
fi

# Step 5: Train the model
log "Step 4: Training ML model..."
if ! python3 label_and_train.py > "$LOGS_DIR/training.log" 2>&1; then
    error "Model training failed. Check $LOGS_DIR/training.log for details."
fi

# Step 6: Verify model was created
if [[ ! -f "$MODELS_DIR/model.onnx" ]]; then
    error "Model file not created: $MODELS_DIR/model.onnx"
fi

log "Model training completed successfully"

# Step 7: Validate model file
log "Step 5: Validating ONNX model..."
if ! python3 -c "
import onnx
import onnxruntime as ort
try:
    model = onnx.load('$MODELS_DIR/model.onnx')
    onnx.checker.check_model(model)
    session = ort.InferenceSession('$MODELS_DIR/model.onnx')
    print(f'Model inputs: {[inp.name for inp in session.get_inputs()]}')
    print(f'Model outputs: {[out.name for out in session.get_outputs()]}')
    print('Model validation successful')
except Exception as e:
    print(f'Model validation failed: {e}')
    exit(1)
" > "$LOGS_DIR/validation.log" 2>&1; then
    error "Model validation failed. Check $LOGS_DIR/validation.log for details."
fi

log "Model validation passed"

# Step 8: Show training metrics if available
if [[ -f "$MODELS_DIR/training_metrics.json" ]]; then
    log "Training metrics:"
    python3 -c "
import json
try:
    with open('$MODELS_DIR/training_metrics.json') as f:
        metrics = json.load(f)
    print(f\"  AUC Score: {metrics.get('auc_score', 'N/A'):.4f}\")
    print(f\"  F1 Score: {metrics.get('f1_score', 'N/A'):.4f}\")
    print(f\"  Training samples: {metrics.get('n_samples', 'N/A')}\")
    print(f\"  Positive ratio: {metrics.get('positive_ratio', 'N/A'):.3f}\")
except:
    pass
"
fi

# Step 9: Restart bot if requested
if [[ "$RESTART_BOT" == true ]]; then
    log "Step 6: Restarting trading bot..."
    
    # Check if bot is running (adjust process name as needed)
    if pgrep -f "bitrader" > /dev/null; then
        log "Stopping existing bot process..."
        pkill -f "bitrader" || true
        sleep 2
    fi
    
    # Start bot in background (adjust command as needed)
    log "Starting bot with new model..."
    cd "$PROJECT_ROOT"
    nohup ./bin/bitrader > "$LOGS_DIR/bot.log" 2>&1 &
    
    # Wait a moment and check if it started successfully
    sleep 3
    if pgrep -f "bitrader" > /dev/null; then
        log "Bot restarted successfully (PID: $(pgrep -f bitrader))"
    else
        warn "Bot may not have started correctly. Check $LOGS_DIR/bot.log"
    fi
fi

# Clean up temporary files
rm -f "$SCRIPT_DIR/training_data.json"

log "ML model deployment completed successfully!"
log "Model location: $MODELS_DIR/model.onnx"
log "Logs available in: $LOGS_DIR/"

# Show final summary
echo
echo "=== Deployment Summary ==="
echo "✓ Data exported from BoltDB"
echo "✓ Model trained and validated"
echo "✓ ONNX model deployed"
if [[ "$RESTART_BOT" == true ]]; then
    echo "✓ Trading bot restarted"
fi
echo "=========================="
