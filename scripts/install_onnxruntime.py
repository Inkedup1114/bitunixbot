#!/usr/bin/env python3
"""
ONNX Runtime Installation Helper
Handles platform-specific installation of onnxruntime
"""
import sys
import subprocess
import platform
import os

def get_python_version():
    """Get Python version as tuple"""
    return sys.version_info[:2]

def get_platform_info():
    """Get platform information"""
    return {
        'system': platform.system(),
        'machine': platform.machine(),
        'python_version': get_python_version(),
        'is_m1_mac': platform.system() == 'Darwin' and platform.machine() == 'arm64'
    }

def install_onnxruntime():
    """Install appropriate onnxruntime version"""
    info = get_platform_info()
    python_major, python_minor = info['python_version']
    
    # Check Python version compatibility
    if python_major < 3 or (python_major == 3 and python_minor < 8):
        print(f"❌ Python {python_major}.{python_minor} is not supported")
        print("   ONNX Runtime requires Python 3.8 or higher")
        return False
    
    if python_major == 3 and python_minor > 12:
        print(f"⚠️  Python {python_major}.{python_minor} might not be fully supported")
        print("   ONNX Runtime is tested up to Python 3.12")
    
    # Determine package to install
    package = "onnxruntime==1.17.3"
    
    if info['is_m1_mac']:
        print("🍎 Detected Apple Silicon Mac")
        # For M1/M2 Macs, try silicon-specific version first
        packages_to_try = [
            "onnxruntime-silicon==1.17.3",
            "onnxruntime==1.17.3"
        ]
    else:
        packages_to_try = [package]
    
    # Try to install
    for pkg in packages_to_try:
        print(f"\n📦 Attempting to install {pkg}...")
        try:
            subprocess.check_call([
                sys.executable, "-m", "pip", "install", 
                "--upgrade", pkg
            ])
            print(f"✅ Successfully installed {pkg}")
            
            # Test import
            try:
                import onnxruntime
                print(f"✅ ONNX Runtime {onnxruntime.__version__} imported successfully")
                return True
            except ImportError as e:
                print(f"❌ Failed to import onnxruntime: {e}")
                
        except subprocess.CalledProcessError as e:
            print(f"❌ Failed to install {pkg}: {e}")
            continue
    
    # If all attempts failed, provide platform-specific guidance
    print("\n❌ Failed to install ONNX Runtime")
    print("\n📋 Platform-specific troubleshooting:")
    
    if info['system'] == 'Linux':
        print("\nFor Linux systems:")
        print("1. Ensure system dependencies are installed:")
        print("   Ubuntu/Debian: sudo apt-get install python3-dev build-essential")
        print("   CentOS/RHEL: sudo yum install python3-devel gcc gcc-c++")
        print("2. Try installing with --no-binary flag:")
        print(f"   pip install --no-binary onnxruntime onnxruntime==1.17.3")
    
    elif info['system'] == 'Darwin':
        print("\nFor macOS:")
        print("1. Ensure Xcode Command Line Tools are installed:")
        print("   xcode-select --install")
        if info['is_m1_mac']:
            print("2. For Apple Silicon, try:")
            print("   pip install onnxruntime-silicon")
        print("3. Try installing from conda-forge:")
        print("   conda install -c conda-forge onnxruntime")
    
    elif info['system'] == 'Windows':
        print("\nFor Windows:")
        print("1. Ensure Visual C++ redistributables are installed")
        print("2. Try installing specific wheel:")
        print("   Visit: https://pypi.org/project/onnxruntime/#files")
        print("   Download appropriate .whl file and install with:")
        print("   pip install <downloaded_file.whl>")
    
    return False

def main():
    print("🔧 ONNX Runtime Installation Helper")
    print("=" * 40)
    
    # Show current environment
    info = get_platform_info()
    print(f"Platform: {info['system']} {info['machine']}")
    print(f"Python: {sys.version}")
    print(f"Pip: {subprocess.check_output([sys.executable, '-m', 'pip', '--version'], text=True).strip()}")
    
    # Check if already installed
    try:
        import onnxruntime as ort
        print(f"\n✅ ONNX Runtime {ort.__version__} is already installed")
        print("   Providers:", ort.get_available_providers())
        return 0
    except ImportError:
        print("\n❌ ONNX Runtime not found")
    
    # Try to install
    if install_onnxruntime():
        return 0
    else:
        return 1

if __name__ == "__main__":
    sys.exit(main())
