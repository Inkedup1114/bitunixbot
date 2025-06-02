#!/usr/bin/env python3

"""Quick test to verify ML setup is working"""

import sys

print("🧪 Quick ML Setup Test")
print("====================")

try:
    import numpy as np
    print(f"✅ NumPy {np.__version__}")
except ImportError as e:
    print(f"❌ NumPy import failed: {e}")
    sys.exit(1)

try:
    import onnxruntime as ort
    print(f"✅ ONNX Runtime {ort.__version__}")
except ImportError as e:
    print(f"❌ ONNX Runtime import failed: {e}")
    sys.exit(1)

try:
    import pandas as pd
    print(f"✅ Pandas {pd.__version__}")
except ImportError as e:
    print(f"❌ Pandas import failed: {e}")

try:
    import sklearn
    print(f"✅ Scikit-learn {sklearn.__version__}")
except ImportError as e:
    print(f"❌ Scikit-learn import failed: {e}")

# Test model loading
from pathlib import Path
model_path = Path(__file__).parent.parent / "models" / "model.onnx"

if model_path.exists():
    try:
        session = ort.InferenceSession(str(model_path))
        print(f"✅ Model loads successfully")
        print(f"   Inputs: {[i.name for i in session.get_inputs()]}")
        print(f"   Outputs: {[o.name for o in session.get_outputs()]}")
    except Exception as e:
        print(f"❌ Model loading failed: {e}")
else:
    print("⚠️  No model found")

print("\n✅ ML setup is working!")
