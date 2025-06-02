#!/usr/bin/env python3
"""
Model Health Check Script for Bitunix Trading Bot
Tests the ONNX model to ensure it's properly loaded and functional.
"""

import sys
import json
import os
import numpy as np
from pathlib import Path

def main():
    if len(sys.argv) != 2:
        print("Usage: python test_model_health.py <model_path>")
        sys.exit(1)
    
    model_path = sys.argv[1]
    
    # Check if model file exists
    if not os.path.exists(model_path):
        print(f"ERROR: Model file not found: {model_path}")
        sys.exit(1)
    
    try:
        import onnxruntime as ort
        print(f"✓ ONNX Runtime version: {ort.__version__}")
    except ImportError as e:
        print(f"ERROR: ONNX Runtime not installed: {e}")
        print("Install with: pip install onnxruntime")
        sys.exit(1)
    
    try:
        # Load the ONNX model
        print(f"Loading model from: {model_path}")
        session = ort.InferenceSession(model_path)
        
        # Get model info
        inputs = session.get_inputs()
        outputs = session.get_outputs()
        
        print(f"✓ Model loaded successfully")
        print(f"  - Inputs: {len(inputs)}")
        for i, inp in enumerate(inputs):
            print(f"    {i}: {inp.name} {inp.type} {inp.shape}")
        
        print(f"  - Outputs: {len(outputs)}")
        for i, out in enumerate(outputs):
            print(f"    {i}: {out.name} {out.type} {out.shape}")
        
        # Test prediction with sample data
        if len(inputs) > 0:
            input_name = inputs[0].name
            
            # Create test features [tickRatio, depthRatio, priceDist]
            test_cases = [
                [0.1, -0.2, 0.5],   # Normal case
                [0.0, 0.0, 0.0],    # Zero case
                [1.0, 1.0, 1.0],    # High values
                [-1.0, -1.0, -1.0], # Negative values
            ]
            
            print(f"\nTesting predictions:")
            for i, features in enumerate(test_cases):
                try:
                    features_array = np.array([features], dtype=np.float32)
                    result = session.run(None, {input_name: features_array})
                    
                    if len(result) >= 2:
                        prediction = int(result[0][0])
                        probabilities = result[1][0].tolist()
                        print(f"  Test {i+1}: features={features} -> prediction={prediction}, probs={probabilities}")
                    elif len(result) == 1:
                        output = result[0]
                        if len(output.shape) > 1 and output.shape[-1] == 2:
                            probabilities = output[0].tolist()
                            prediction = int(np.argmax(probabilities))
                        else:
                            prediction = int(output[0] > 0.5)
                            prob_positive = float(output[0]) if 0 <= output[0] <= 1 else 0.5
                            probabilities = [1.0 - prob_positive, prob_positive]
                        print(f"  Test {i+1}: features={features} -> prediction={prediction}, probs={probabilities}")
                    else:
                        print(f"  Test {i+1}: Unexpected output format: {len(result)} outputs")
                        
                except Exception as e:
                    print(f"  Test {i+1}: FAILED - {e}")
                    sys.exit(1)
        
        print(f"\n✓ All tests passed! Model is healthy.")
        
    except Exception as e:
        print(f"ERROR: Model validation failed: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
