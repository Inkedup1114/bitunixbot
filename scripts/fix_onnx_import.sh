#!/bin/bash

# ONNX Runtime Installation Fix Script
# Handles common installation issues

set -e

echo "🔧 ONNX Runtime Installation Fix"
echo "================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Detect OS and architecture
OS=$(uname -s)
ARCH=$(uname -m)
PYTHON_VERSION=$(python3 -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')")

echo "System: $OS $ARCH"
echo "Python: $PYTHON_VERSION"

# Function to test onnxruntime import
test_onnx() {
    python3 -c "import onnxruntime; print(f'✅ ONNX Runtime {onnxruntime.__version__} working')" 2>/dev/null
}

# Check if already working
if test_onnx; then
    echo -e "${GREEN}ONNX Runtime is already installed and working${NC}"
    exit 0
fi

echo -e "${YELLOW}ONNX Runtime not found or not working${NC}"

# Ensure pip is updated
echo "Updating pip..."
python3 -m pip install --upgrade pip

# Platform-specific installation
case "$OS" in
    Linux)
        echo "Installing for Linux..."
        # Install system dependencies if needed
        if command -v apt-get &> /dev/null; then
            echo "Installing system dependencies (may require sudo)..."
            sudo apt-get update && sudo apt-get install -y python3-dev build-essential
        elif command -v yum &> /dev/null; then
            sudo yum install -y python3-devel gcc gcc-c++
        fi
        
        # Try standard installation
        python3 -m pip install onnxruntime==1.17.3
        ;;
        
    Darwin)
        echo "Installing for macOS..."
        
        if [[ "$ARCH" == "arm64" ]]; then
            echo "Detected Apple Silicon (M1/M2)"
            # Try silicon-specific version first
            if ! python3 -m pip install onnxruntime-silicon==1.17.3; then
                echo "Silicon version failed, trying standard version..."
                python3 -m pip install onnxruntime==1.17.3
            fi
        else
            # Intel Mac
            python3 -m pip install onnxruntime==1.17.3
        fi
        ;;
        
    *)
        echo "Installing for $OS..."
        python3 -m pip install onnxruntime==1.17.3
        ;;
esac

# Test installation
echo ""
echo "Testing installation..."
if test_onnx; then
    echo -e "${GREEN}✅ ONNX Runtime successfully installed!${NC}"
    
    # Show providers
    python3 -c "import onnxruntime as ort; print('Available providers:', ort.get_available_providers())"
else
    echo -e "${RED}❌ ONNX Runtime installation failed${NC}"
    echo ""
    echo "Please try manual installation:"
    echo "1. Check Python version compatibility (3.8-3.12 recommended)"
    echo "2. Visit https://pypi.org/project/onnxruntime/#files"
    echo "3. Download the appropriate wheel file for your platform"
    echo "4. Install with: pip install <downloaded_file.whl>"
    exit 1
fi
