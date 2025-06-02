#!/bin/bash

# Load environment variables from .env file if it exists
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Script to collect historical data from Bitunix exchange

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Default values
SYMBOLS=""
DAYS=30
INTERVAL="1h"
START_DATE=""
END_DATE=""

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -s|--symbols)
            SYMBOLS="$2"
            shift 2
            ;;
        -d|--days)
            DAYS="$2"
            shift 2
            ;;
        -i|--interval)
            INTERVAL="$2"
            shift 2
            ;;
        --start)
            START_DATE="$2"
            shift 2
            ;;
        --end)
            END_DATE="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  -s, --symbols    Comma-separated symbols (e.g., BTCUSDT,ETHUSDT)"
            echo "  -d, --days       Number of days to collect (default: 30)"
            echo "  -i, --interval   Kline interval: 1m, 5m, 15m, 1h, 4h, 1d (default: 1h)"
            echo "  --start         Start date (YYYY-MM-DD)"
            echo "  --end           End date (YYYY-MM-DD)"
            echo "  -h, --help       Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

cd "$PROJECT_ROOT"

# Build the collector if needed
echo "Building historical data collector..."
go build -o "$SCRIPT_DIR/collect_historical_data" "$SCRIPT_DIR/collect_historical_data.go"

if [ $? -ne 0 ]; then
    echo "Failed to build collector"
    exit 1
fi

# Prepare arguments
ARGS=""
if [ -n "$SYMBOLS" ]; then
    ARGS="$ARGS -symbols=$SYMBOLS"
fi
if [ -n "$START_DATE" ]; then
    ARGS="$ARGS -start=$START_DATE"
fi
if [ -n "$END_DATE" ]; then
    ARGS="$ARGS -end=$END_DATE"
fi
ARGS="$ARGS -days=$DAYS -interval=$INTERVAL"

# Run the collector
echo "Collecting historical data..."
echo "Interval: $INTERVAL"
echo "Days: $DAYS"
if [ -n "$SYMBOLS" ]; then
    echo "Symbols: $SYMBOLS"
fi

"$SCRIPT_DIR/collect_historical_data" $ARGS

# Cleanup
rm -f "$SCRIPT_DIR/collect_historical_data"

echo "Data collection complete!"
