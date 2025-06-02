#!/bin/bash

# Complete ML Pipeline and Go Integration Setup

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}🚀 Bitunix Bot Complete Setup${NC}"
echo "=================================="

# Step 1: Check Python environment
echo -e "\n${YELLOW}Step 1: Checking Python environment${NC}"
if ! command -v python3 &> /dev/null; then
    echo -e "${RED}❌ Python 3 not found${NC}"
    exit 1
fi

PYTHON_VERSION=$(python3 -c 'import sys; print(".".join(map(str, sys.version_info[:2])))')
echo "Python version: $PYTHON_VERSION"

# Step 2: Set up virtual environment
echo -e "\n${YELLOW}Step 2: Setting up virtual environment${NC}"
cd "$PROJECT_ROOT"

if [[ ! -d "venv" ]]; then
    python3 -m venv venv
fi

# Activate virtual environment
source venv/bin/activate

# Step 3: Install Python dependencies
echo -e "\n${YELLOW}Step 3: Installing Python dependencies${NC}"
pip install --upgrade pip
pip install -r scripts/requirements.txt

# Step 4: Verify ONNX Runtime
echo -e "\n${YELLOW}Step 4: Verifying ONNX Runtime${NC}"
if python3 -c "import onnxruntime; print('ONNX Runtime version:', onnxruntime.__version__)"; then
    echo -e "${GREEN}✅ ONNX Runtime installed successfully${NC}"
else
    echo -e "${RED}❌ ONNX Runtime installation failed${NC}"
    exit 1
fi

# Step 5: Check for model
echo -e "\n${YELLOW}Step 5: Checking ML model${NC}"
if [[ -f "$PROJECT_ROOT/models/model.onnx" ]]; then
    echo -e "${GREEN}✅ Model found at models/model.onnx${NC}"
    # Test the model
    echo "Testing model..."
    python3 scripts/test_prediction.py
else
    echo -e "${YELLOW}⚠️  No model found${NC}"
    echo "To create a model, run: python scripts/label_and_train.py"
fi

# Step 6: Check Go environment
echo -e "\n${YELLOW}Step 6: Checking Go environment${NC}"
if command -v go &> /dev/null; then
    GO_VERSION=$(go version)
    echo -e "${GREEN}✅ $GO_VERSION${NC}"
    
    # Download Go dependencies
    echo "Downloading Go dependencies..."
    go mod download
    go mod verify
else
    echo -e "${YELLOW}⚠️  Go not installed${NC}"
    echo "Install Go from: https://go.dev/dl/"
fi

# Step 7: Create necessary directories
echo -e "\n${YELLOW}Step 7: Creating directories${NC}"
mkdir -p "$PROJECT_ROOT/models"
mkdir -p "$PROJECT_ROOT/data"
mkdir -p "$PROJECT_ROOT/logs"
echo -e "${GREEN}✅ Directories created${NC}"

# Step 8: Check configuration
echo -e "\n${YELLOW}Step 8: Checking configuration${NC}"
if [[ -f "$PROJECT_ROOT/config.yaml" ]]; then
    echo -e "${GREEN}✅ config.yaml found${NC}"
else
    echo -e "${YELLOW}Creating default config.yaml${NC}"
    cp "$PROJECT_ROOT/config.yaml.example" "$PROJECT_ROOT/config.yaml" 2>/dev/null || \
    cat > "$PROJECT_ROOT/config.yaml" << 'EOF'
api:
  key: ""
  secret: ""
  baseURL: "https://api.bitunix.com"
  wsURL: "wss://fapi.bitunix.com/public"

trading:
  symbols: ["BTCUSDT"]
  baseSizeRatio: 0.002
  maxPositionSize: 0.01
  maxDailyLoss: 0.05
  maxPriceDistance: 3.0
  dryRun: true

ml:
  modelPath: "models/model.onnx"
  probThreshold: 0.65

features:
  vwapWindow: "30s"
  vwapSize: 600
  tickSize: 50

system:
  dataPath: "./data"
  pingInterval: "15s"
  metricsPort: 8080
  restTimeout: "5s"
EOF
fi

# Step 9: Build Go binary
echo -e "\n${YELLOW}Step 9: Building Go binary${NC}"
if command -v go &> /dev/null; then
    echo "Building bitunix-bot..."
    go build -o bitunix-bot cmd/bitrader/main.go
    if [[ -f "bitunix-bot" ]]; then
        echo -e "${GREEN}✅ Binary built successfully${NC}"
    else
        echo -e "${RED}❌ Build failed${NC}"
    fi
else
    echo -e "${YELLOW}Skipping Go build (Go not installed)${NC}"
fi

# Step 10: Final verification
echo -e "\n${YELLOW}Step 10: Running final verification${NC}"
python3 verify_setup.py

echo -e "\n${BLUE}Setup Summary${NC}"
echo "=============="
echo -e "Python Environment: ${GREEN}✅${NC}"
echo -e "ONNX Runtime: ${GREEN}✅${NC}"
if [[ -f "$PROJECT_ROOT/models/model.onnx" ]]; then
    echo -e "ML Model: ${GREEN}✅${NC}"
else
    echo -e "ML Model: ${YELLOW}⚠️  Not found${NC}"
fi
if command -v go &> /dev/null; then
    echo -e "Go Environment: ${GREEN}✅${NC}"
else
    echo -e "Go Environment: ${YELLOW}⚠️  Not installed${NC}"
fi

echo -e "\n${BLUE}Next Steps:${NC}"
echo "1. Set environment variables:"
echo "   export BITUNIX_API_KEY='your_api_key'"
echo "   export BITUNIX_SECRET_KEY='your_secret_key'"
echo ""
echo "2. If no model exists, create one:"
echo "   python scripts/label_and_train.py"
echo ""
echo "3. Run the bot:"
echo "   ./bitunix-bot"
echo ""
echo "4. Monitor metrics:"
echo "   http://localhost:8080/metrics"

echo -e "\n${GREEN}✅ Setup complete!${NC}"
