#!/usr/bin/env python3

"""
Fix ONNX Model IR Version
Converts ONNX model to compatible IR version for ONNX Runtime 1.17.3
"""

import sys
from pathlib import Path

def fix_model_ir_version(model_path, output_path=None):
    """Convert ONNX model to compatible IR version"""
    try:
        import onnx
        
        print(f"Loading model from: {model_path}")
        model = onnx.load(str(model_path))
        
        print(f"Current IR version: {model.ir_version}")
        print(f"Opset version: {model.opset_import[0].version if model.opset_import else 'Unknown'}")
        
        # Set IR version to 9 (compatible with ONNX Runtime 1.17.3)
        model.ir_version = 9
        
        # Save the model
        if output_path is None:
            output_path = model_path.parent / f"{model_path.stem}_fixed{model_path.suffix}"
        
        onnx.save(model, str(output_path))
        print(f"✅ Model saved with IR version 9 to: {output_path}")
        
        # Verify the model
        onnx.checker.check_model(str(output_path))
        print("✅ Model verification passed")
        
        return True
        
    except Exception as e:
        print(f"❌ Error fixing model: {e}")
        return False

def main():
    script_dir = Path(__file__).parent.parent
    model_path = script_dir / "models" / "model.onnx"
    
    print("🔧 ONNX Model IR Version Fix")
    print("===========================")
    print()
    
    if not model_path.exists():
        print(f"❌ Model not found at: {model_path}")
        sys.exit(1)
    
    # First, let's check if we need to install onnx package
    try:
        import onnx
    except ImportError:
        print("📦 Installing onnx package...")
        import subprocess
        subprocess.check_call([sys.executable, "-m", "pip", "install", "onnx"])
        print("✅ ONNX package installed")
    
    # Create backup
    backup_path = model_path.parent / f"{model_path.stem}_backup{model_path.suffix}"
    if not backup_path.exists():
        import shutil
        shutil.copy(model_path, backup_path)
        print(f"📋 Created backup at: {backup_path}")
    
    # Fix the model (overwrite original)
    if fix_model_ir_version(model_path, model_path):
        print("\n✅ Model IR version fixed successfully!")
        print("\nNext steps:")
        print("1. Run verification: python verify_setup.py")
        print("2. Test model loading with ONNX Runtime")
    else:
        print("\n❌ Failed to fix model IR version")
        print("The model might be corrupted or in an incompatible format.")
        print("You may need to retrain the model with compatible versions.")

if __name__ == "__main__":
    main()
