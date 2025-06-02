#!/bin/bash
# Quick activation script for Bitunix Bot ML environment

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Check if script is being sourced
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    echo ""
    echo -e "${RED}⚠️  This script should be sourced, not executed directly!${NC}"
    echo "Use: source activate.sh"
    echo "Or:  . activate.sh"
    exit 1
fi

if [[ ! -d "$SCRIPT_DIR/venv" ]]; then
    echo -e "${YELLOW}Creating virtual environment...${NC}"
    python3 -m venv "$SCRIPT_DIR/venv" || {
        echo -e "${RED}Failed to create virtual environment${NC}"
        echo "Try: sudo apt-get install python3-venv"
        return 1
    }
fi

echo -e "${GREEN}Activating virtual environment...${NC}"
source "$SCRIPT_DIR/venv/bin/activate"

# Verify activation worked
if [[ -z "$VIRTUAL_ENV" ]]; then
    echo -e "${RED}Failed to activate virtual environment${NC}"
    return 1
fi

# Check for onnxruntime
if ! python3 -c "import onnxruntime" 2>/dev/null; then
    echo -e "${YELLOW}ONNX Runtime not installed. Installing...${NC}"
    pip install --upgrade pip
    pip install onnxruntime==1.17.3
fi

# Show status
echo ""
echo "✅ Virtual environment activated"
echo "Python: $(which python3)"
echo "Pip: $(which pip)"
echo "Virtual Env: $VIRTUAL_ENV"

# Quick health check
if python3 -c "import onnxruntime; print('ONNX Runtime:', onnxruntime.__version__)" 2>/dev/null; then
    echo -e "${GREEN}✅ ONNX Runtime ready${NC}"
else
    echo -e "${YELLOW}⚠️  ONNX Runtime not available${NC}"
    echo "Run: pip install -r scripts/requirements.txt"
fi

echo ""
echo "Now you can run:"
echo "  python scripts/label_and_train.py"
echo "  ./scripts/deploy_model.sh"
echo ""
