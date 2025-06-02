#!/usr/bin/env python3
"""
Comprehensive ML setup verification script for Bitunix Bot
Checks Python environment, dependencies, and ONNX runtime
"""
import sys
import os
import subprocess
from pathlib import Path
import json

def check_python_version():
    """Check if Python version is compatible"""
    version = sys.version_info
    print(f"Python version: {version.major}.{version.minor}.{version.micro}")
    
    if version.major < 3 or (version.major == 3 and version.minor < 8):
        print("❌ Python 3.8+ required")
        return False
    
    print("✅ Python version OK")
    return True

def check_virtual_env():
    """Check virtual environment status"""
    in_venv = hasattr(sys, 'real_prefix') or (hasattr(sys, 'base_prefix') and sys.base_prefix != sys.prefix)
    venv_path = os.environ.get('VIRTUAL_ENV')
    
    if in_venv and venv_path:
        print(f"✅ Virtual environment active: {venv_path}")
        return True
    else:
        # Check for common venv locations
        script_dir = Path(__file__).parent.parent
        possible_venvs = [
            script_dir / "venv",
            script_dir / ".venv",
            script_dir / "env"
        ]
        
        for venv in possible_venvs:
            if venv.exists():
                print(f"⚠️  Virtual environment found at {venv} but not activated")
                print(f"   Run: source {venv}/bin/activate")
                return False
        
        print("❌ No virtual environment found or activated")
        print("   Create one with: python3 -m venv venv")
        return False

def check_onnxruntime():
    """Check if onnxruntime is installed and working"""
    try:
        import onnxruntime as ort
        print(f"✅ ONNX Runtime installed: version {ort.__version__}")
        
        # Check if it can create a session
        providers = ort.get_available_providers()
        print(f"   Available providers: {', '.join(providers)}")
        return True
    except ImportError:
        print("❌ ONNX Runtime not installed")
        print("   Install with: pip install onnxruntime==1.17.3")
        return False
    except Exception as e:
        print(f"❌ ONNX Runtime error: {e}")
        return False

def check_ml_dependencies():
    """Check all ML dependencies"""
    dependencies = {
        'numpy': None,
        'pandas': None,
        'scikit-learn': 'sklearn',
        'onnx': None,
        'skl2onnx': None,
        'xgboost': None,
    }
    
    all_ok = True
    print("\nChecking ML dependencies:")
    
    for package, import_name in dependencies.items():
        module_name = import_name or package
        try:
            module = __import__(module_name)
            version = getattr(module, '__version__', 'unknown')
            print(f"  ✅ {package}: {version}")
        except ImportError:
            print(f"  ❌ {package}: not installed")
            all_ok = False
    
    return all_ok

def check_model_files():
    """Check if model files exist"""
    project_root = Path(__file__).parent.parent
    model_locations = [
        project_root / "models" / "model.onnx",
        project_root / "model.onnx",
    ]
    
    print("\nChecking model files:")
    for model_path in model_locations:
        if model_path.exists():
            size_mb = model_path.stat().st_size / (1024 * 1024)
            print(f"  ✅ Found model: {model_path} ({size_mb:.2f} MB)")
            return True
    
    print("  ⚠️  No ONNX model found")
    print("     Expected locations:")
    for path in model_locations:
        print(f"     - {path}")
    return False

def test_onnx_inference():
    """Test ONNX inference with dummy data"""
    try:
        import onnxruntime as ort
        import numpy as np
        
        # Find model
        project_root = Path(__file__).parent.parent
        model_path = project_root / "models" / "model.onnx"
        if not model_path.exists():
            model_path = project_root / "model.onnx"
        
        if not model_path.exists():
            print("\n⚠️  Cannot test inference - no model found")
            return False
        
        print(f"\nTesting ONNX inference with {model_path}:")
        
        # Create session
        session = ort.InferenceSession(str(model_path))
        
        # Get input details
        inputs = session.get_inputs()
        print(f"  Model inputs: {[inp.name for inp in inputs]}")
        print(f"  Input shape: {inputs[0].shape}")
        
        # Create dummy input
        dummy_features = np.array([[0.1, -0.2, 0.5]], dtype=np.float32)
        
        # Run inference
        outputs = session.run(None, {inputs[0].name: dummy_features})
        
        print(f"  ✅ Inference successful!")
        print(f"     Output shape: {outputs[0].shape}")
        print(f"     Sample prediction: {outputs[0]}")
        
        return True
        
    except Exception as e:
        print(f"  ❌ Inference test failed: {e}")
        return False

def suggest_fixes():
    """Suggest fixes for common issues"""
    print("\n🔧 Quick fixes:")
    print("1. Create and activate virtual environment:")
    print("   python3 -m venv venv")
    print("   source venv/bin/activate  # Linux/Mac")
    print("   venv\\Scripts\\activate     # Windows")
    print()
    print("2. Install all dependencies:")
    print("   pip install -r scripts/requirements.txt")
    print()
    print("3. Train a model (if missing):")
    print("   cd scripts && python label_and_train.py")
    print()
    print("4. For Docker environments, ensure python3-dev is installed:")
    print("   apt-get update && apt-get install -y python3-dev")

def main():
    """Run all checks"""
    print("🔍 Bitunix Bot ML Setup Verification")
    print("=" * 50)
    
    checks = [
        ("Python Version", check_python_version),
        ("Virtual Environment", check_virtual_env),
        ("ONNX Runtime", check_onnxruntime),
        ("ML Dependencies", check_ml_dependencies),
        ("Model Files", check_model_files),
        ("ONNX Inference", test_onnx_inference),
    ]
    
    results = {}
    for name, check_func in checks:
        print(f"\n{name}:")
        results[name] = check_func()
    
    # Summary
    print("\n" + "=" * 50)
    print("📊 Summary:")
    all_passed = all(results.values())
    
    for name, passed in results.items():
        status = "✅" if passed else "❌"
        print(f"  {status} {name}")
    
    if all_passed:
        print("\n✅ All checks passed! ML pipeline is ready.")
    else:
        print("\n❌ Some checks failed.")
        suggest_fixes()
    
    return 0 if all_passed else 1

if __name__ == "__main__":
    sys.exit(main())
