#!/usr/bin/env python3

"""
ML Setup Verification Script
Verifies that the ONNX ML pipeline is properly configured
"""

import os
import sys
import subprocess
import importlib
from pathlib import Path

# Colors for terminal output
class Colors:
    GREEN = '\033[0;32m'
    RED = '\033[0;31m'
    YELLOW = '\033[1;33m'
    NC = '\033[0m'  # No Color

def print_colored(message, color=Colors.NC):
    """Print message with color"""
    print(f"{color}{message}{Colors.NC}")

def check_command(command):
    """Check if a command exists"""
    try:
        subprocess.run([command, '--version'], capture_output=True, check=True)
        return True
    except (subprocess.CalledProcessError, FileNotFoundError):
        return False

def main():
    script_dir = Path(__file__).parent.absolute()
    
    print("🔍 Verifying Bitunix Bot ML Setup...")
    print("====================================")
    
    # Check virtual environment
    virtual_env = os.environ.get('VIRTUAL_ENV')
    if virtual_env:
        print_colored(f"✅ Virtual environment active: {virtual_env}", Colors.GREEN)
        print(f"   Python: {sys.executable}")
        try:
            pip_path = subprocess.check_output(['which', 'pip'], text=True).strip()
            print(f"   Pip: {pip_path}")
        except subprocess.CalledProcessError:
            print("   Pip: Not found")
    else:
        print_colored("❌ Virtual environment not active", Colors.RED)
        venv_path = script_dir / "venv"
        if venv_path.exists():
            print_colored("   Virtual environment exists but not activated", Colors.YELLOW)
            print("   Run: source activate.sh")
        else:
            print_colored("   No virtual environment found", Colors.YELLOW)
            print("   Run: ./run_ml_pipeline.sh to create one")
    
    # Check Python packages
    print("\nChecking Python packages...")
    print(f"Python: {sys.version}")
    
    packages = {
        'onnxruntime': 'ONNX Runtime for model inference',
        'numpy': 'NumPy for numerical computing',
        'pandas': 'Pandas for data manipulation',
        'sklearn': 'Scikit-learn for machine learning',
        'joblib': 'Joblib for serialization',
        'matplotlib': 'Matplotlib for plotting',
        'seaborn': 'Seaborn for statistical visualization',
        'yaml': 'PyYAML for YAML parsing'
    }
    
    for module, name in packages.items():
        try:
            mod = importlib.import_module(module)
            version = getattr(mod, '__version__', 'unknown')
            print_colored(f"✅ {name}: {version}", Colors.GREEN)
        except ImportError:
            print_colored(f"❌ {name} not installed", Colors.RED)
    
    # Check directories
    print("\nChecking directories...")
    required_dirs = ['models', 'data', 'logs', 'scripts']
    for dir_name in required_dirs:
        dir_path = script_dir / dir_name
        if dir_path.exists():
            print_colored(f"✅ Directory exists: {dir_name}", Colors.GREEN)
        else:
            print_colored(f"❌ Directory missing: {dir_name}", Colors.RED)
    
    # Check required files
    print("\nChecking required files...")
    required_files = [
        "scripts/requirements.txt",
        "scripts/label_and_train.py",
        "scripts/deploy_model.sh",
        "scripts/setup_ml_pipeline.sh"
    ]
    
    for file_path in required_files:
        full_path = script_dir / file_path
        if full_path.exists():
            print_colored(f"✅ File exists: {file_path}", Colors.GREEN)
        else:
            print_colored(f"❌ File missing: {file_path}", Colors.RED)
    
    # Check ML model
    print("\nChecking ML model...")
    model_path = script_dir / "models" / "model.onnx"
    if model_path.exists():
        print_colored("✅ ONNX model found", Colors.GREEN)
        stat = model_path.stat()
        print(f"   Size: {stat.st_size / 1024 / 1024:.2f} MB")
        
        # Test model loading if onnxruntime is available
        try:
            import onnxruntime as ort
            session = ort.InferenceSession(str(model_path))
            inputs = [inp.name for inp in session.get_inputs()]
            outputs = [out.name for out in session.get_outputs()]
            print_colored("✅ Model loads successfully", Colors.GREEN)
            print(f"   Inputs: {inputs}")
            print(f"   Outputs: {outputs}")
        except ImportError:
            print_colored("⚠️  ONNX Runtime not available for model testing", Colors.YELLOW)
        except Exception as e:
            print_colored(f"❌ Model loading failed: {e}", Colors.RED)
    else:
        print_colored("⚠️  No model found - run training pipeline to create one", Colors.YELLOW)
    
    # Check Go environment
    print("\nChecking Go environment...")
    if check_command('go'):
        try:
            go_version = subprocess.check_output(['go', 'version'], text=True).strip()
            print_colored(f"✅ Go installed: {go_version}", Colors.GREEN)
            
            # Check if Go modules are ready
            try:
                subprocess.run(['go', 'mod', 'verify'], 
                              cwd=script_dir, 
                              capture_output=True, 
                              check=True)
                print_colored("✅ Go modules verified", Colors.GREEN)
            except subprocess.CalledProcessError:
                print_colored("⚠️  Go modules need updating (run: go mod tidy)", Colors.YELLOW)
        except subprocess.CalledProcessError as e:
            print_colored(f"❌ Go command failed: {e}", Colors.RED)
    else:
        print_colored("❌ Go not installed", Colors.RED)
    
    print("\n====================================")
    print("Verification complete!")
    
    # Show next steps
    if not virtual_env:
        print("\n📋 Next steps:")
        print("1. Activate virtual environment: source activate.sh")
        print("2. Run setup: ./run_ml_pipeline.sh")

if __name__ == "__main__":
    main()
