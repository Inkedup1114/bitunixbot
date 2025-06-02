#!/bin/bash

# Quick start script for Bitunix Bot

set -e

echo "🚀 Bitunix Bot Quick Start"
echo "========================="
echo ""

# Check if .env exists
if [[ ! -f ".env" ]]; then
    echo "Creating .env file from template..."
    cp .env.example .env
    echo "⚠️  Please edit .env and add your API credentials"
    echo "   nano .env"
    exit 1
fi

# Source environment variables
set -a
source .env
set +a

# Check API credentials
if [[ -z "$BITUNIX_API_KEY" ]] || [[ "$BITUNIX_API_KEY" == "your_api_key_here" ]]; then
    echo "❌ Please set BITUNIX_API_KEY in .env file"
    exit 1
fi

if [[ -z "$BITUNIX_SECRET_KEY" ]] || [[ "$BITUNIX_SECRET_KEY" == "your_secret_key_here" ]]; then
    echo "❌ Please set BITUNIX_SECRET_KEY in .env file"
    exit 1
fi

# Activate virtual environment
if [[ -d "venv" ]]; then
    source venv/bin/activate
else
    echo "Setting up Python environment..."
    python3 -m venv venv
    source venv/bin/activate
    pip install -r scripts/requirements.txt
fi

# Check for model
if [[ ! -f "models/model.onnx" ]]; then
    echo "⚠️  No ML model found"
    echo "Would you like to create a sample model? (y/n)"
    read -r response
    if [[ "$response" == "y" ]]; then
        python scripts/label_and_train.py
    fi
fi

# Build and run
if command -v go &> /dev/null; then
    echo "Building Go binary..."
    go build -o bitunix-bot cmd/bitrader/main.go
    
    echo ""
    echo "✅ Ready to start!"
    echo ""
    echo "Starting Bitunix Bot..."
    echo "Metrics available at: http://localhost:${METRICS_PORT:-8080}/metrics"
    echo "Press Ctrl+C to stop"
    echo ""
    
    ./bitunix-bot
else
    echo "❌ Go is not installed"
    echo "Please install Go from: https://go.dev/dl/"
    exit 1
fi
