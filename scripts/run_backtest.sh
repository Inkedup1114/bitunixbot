#!/bin/bash

# Backtest runner script for Bitunix Trading Bot

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
RESULTS_DIR="$PROJECT_ROOT/backtest_results"

# Default values
CONFIG_FILE="$PROJECT_ROOT/config.yaml"
DATA_PATH="$PROJECT_ROOT/data"
MODEL_PATH="$PROJECT_ROOT/models/model.onnx"
LOG_LEVEL="info"
OUTPUT_DIR=""

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -c|--config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        -d|--data)
            DATA_PATH="$2"
            shift 2
            ;;
        -m|--model)
            MODEL_PATH="$2"
            shift 2
            ;;
        -s|--start)
            START_DATE="$2"
            shift 2
            ;;
        -e|--end)
            END_DATE="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --symbols)
            SYMBOLS="$2"
            shift 2
            ;;
        --debug)
            LOG_LEVEL="debug"
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Function to show help
show_help() {
    cat << EOF
Usage: $0 [OPTIONS]

Run backtests for the Bitunix Trading Bot

Options:
    -c, --config FILE      Config file path (default: config.yaml)
    -d, --data PATH        Data path (CSV/JSON file or BoltDB directory)
    -m, --model PATH       Model path (default: models/model.onnx)
    -s, --start DATE       Start date (YYYY-MM-DD)
    -e, --end DATE         End date (YYYY-MM-DD)
    -o, --output DIR       Output directory for results
    --symbols SYMBOLS      Comma-separated symbols to test
    --debug                Enable debug logging
    -h, --help             Show this help message

Examples:
    # Run backtest with default settings
    $0

    # Run backtest for specific date range
    $0 -s 2024-01-01 -e 2024-03-01

    # Run backtest with specific data file
    $0 -d historical_data.csv -s 2024-01-01 -e 2024-02-01

    # Run backtest for specific symbols
    $0 --symbols "BTCUSDT,ETHUSDT" -s 2024-01-01 -e 2024-03-01
EOF
}

# Create results directory if not specified
if [[ -z "$OUTPUT_DIR" ]]; then
    OUTPUT_DIR="$RESULTS_DIR/backtest_$(date +%Y%m%d_%H%M%S)"
fi

# Build backtest command
CMD="go run $PROJECT_ROOT/cmd/backtest/main.go"
CMD="$CMD -config $CONFIG_FILE"
CMD="$CMD -data $DATA_PATH"
CMD="$CMD -model $MODEL_PATH"
CMD="$CMD -output $OUTPUT_DIR"
CMD="$CMD -log $LOG_LEVEL"

# Add optional parameters
if [[ -n "${START_DATE:-}" ]]; then
    CMD="$CMD -start $START_DATE"
fi

if [[ -n "${END_DATE:-}" ]]; then
    CMD="$CMD -end $END_DATE"
fi

if [[ -n "${SYMBOLS:-}" ]]; then
    CMD="$CMD -symbols $SYMBOLS"
fi

# Print configuration
echo "=== Backtest Configuration ==="
echo "Config File: $CONFIG_FILE"
echo "Data Path: $DATA_PATH"
echo "Model Path: $MODEL_PATH"
echo "Output Directory: $OUTPUT_DIR"
echo "Log Level: $LOG_LEVEL"
[[ -n "${START_DATE:-}" ]] && echo "Start Date: $START_DATE"
[[ -n "${END_DATE:-}" ]] && echo "End Date: $END_DATE"
[[ -n "${SYMBOLS:-}" ]] && echo "Symbols: $SYMBOLS"
echo "=============================="
echo

# Run backtest
echo "Starting backtest..."
eval $CMD

# Check if backtest was successful
if [[ $? -eq 0 ]]; then
    echo
    echo "✅ Backtest completed successfully!"
    echo "📊 Results saved to: $OUTPUT_DIR"
    echo
    echo "View reports:"
    echo "  - Summary: $OUTPUT_DIR/backtest_summary.txt"
    echo "  - Trade Log: $OUTPUT_DIR/trade_log.csv"
    echo "  - Metrics: $OUTPUT_DIR/metrics_report.csv"
    echo "  - JSON Report: $OUTPUT_DIR/backtest_results.json"
else
    echo
    echo "❌ Backtest failed!"
    exit 1
fi
