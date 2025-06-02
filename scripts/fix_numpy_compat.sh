#!/bin/bash

# Fix NumPy compatibility with ONNX Runtime
# ONNX Runtime 1.17.3 requires NumPy 1.x

set -e

echo "🔧 Fixing NumPy Compatibility Issue"
echo "==================================="
echo ""

# Check if we're in a virtual environment
if [[ -z "$VIRTUAL_ENV" ]]; then
    echo "❌ Virtual environment not active!"
    echo "Please activate your virtual environment first:"
    echo "  source venv/bin/activate"
    exit 1
fi

echo "📦 Current environment: $VIRTUAL_ENV"
echo ""

# Show current versions
echo "Current package versions:"
python -m pip show numpy | grep Version || echo "NumPy not installed"
python -m pip show onnxruntime | grep Version || echo "ONNX Runtime not installed"
echo ""

# Uninstall numpy 2.x
echo "🔄 Uninstalling NumPy 2.x..."
pip uninstall -y numpy || true

# Install compatible numpy version
echo "📥 Installing NumPy 1.26.4 (latest 1.x version)..."
pip install numpy==1.26.4

# Reinstall onnxruntime to ensure compatibility
echo "🔄 Reinstalling ONNX Runtime..."
pip uninstall -y onnxruntime || true
pip install onnxruntime==1.17.3

# Install other missing packages with compatible versions
echo "📥 Installing other required packages..."
pip install pandas scikit-learn joblib matplotlib seaborn PyYAML

echo ""
echo "✅ Testing installation..."
python -c "
import numpy as np
import onnxruntime as ort
print(f'NumPy version: {np.__version__}')
print(f'ONNX Runtime version: {ort.__version__}')
print('✅ Import successful!')
"

if [ $? -eq 0 ]; then
    echo ""
    echo "✅ NumPy compatibility issue fixed!"
    echo ""
    echo "📋 Next steps:"
    echo "1. Run verification: python verify_setup.py"
    echo "2. Continue with ML pipeline setup"
else
    echo ""
    echo "❌ Fix failed. Please check error messages above."
fi
