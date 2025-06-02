#!/usr/bin/env python3

"""
Test ML Model Predictions
Simple script to test that the ONNX model is working correctly
"""

import numpy as np
import onnxruntime as ort
from pathlib import Path
import sys
import json

def test_model():
    """Test the ONNX model with sample data"""
    script_dir = Path(__file__).parent.parent
    model_path = script_dir / "models" / "model.onnx"
    
    if not model_path.exists():
        print(f"❌ Model not found at: {model_path}")
        return False
    
    try:
        # Load the model
        print(f"Loading model from: {model_path}")
        session = ort.InferenceSession(str(model_path))
        
        # Get input/output names and shapes
        input_name = session.get_inputs()[0].name
        input_shape = session.get_inputs()[0].shape
        output_names = [out.name for out in session.get_outputs()]
        
        print(f"\nModel Information:")
        print(f"  Input: {input_name}, shape: {input_shape}")
        print(f"  Outputs: {output_names}")
        
        # Test with the exact format used by Go predictor
        # Go sends: [tickRatio, depthRatio, priceDist]
        test_features = {
            "normal_market": [0.2, 0.1, 0.5],      # Typical market conditions
            "buy_pressure": [0.8, 0.3, -1.0],      # Strong buy pressure
            "sell_pressure": [-0.8, -0.3, 1.0],    # Strong sell pressure
            "neutral": [0.0, 0.0, 0.0],            # Neutral conditions
        }
        
        print(f"\nTesting predictions:")
        print(f"Features: [tickRatio, depthRatio, priceDistance]")
        
        for scenario, features in test_features.items():
            # Create input matching Go's format
            test_data = np.array([features], dtype=np.float32)
            
            # Run inference
            outputs = session.run(output_names, {input_name: test_data})
            
            print(f"\n{scenario}:")
            print(f"  Input: {features}")
            
            if len(outputs) >= 2:
                label = outputs[0]
                probability = outputs[1]
                print(f"  Predicted action: {label[0] if hasattr(label, '__getitem__') else label}")
                print(f"  Confidence: {probability[0] if hasattr(probability, '__getitem__') else probability:.3f}")
                
                # Interpret the prediction
                if hasattr(probability, '__getitem__'):
                    conf = float(probability[0])
                else:
                    conf = float(probability)
                    
                if conf > 0.65:  # Match Go's default threshold
                    print(f"  Decision: EXECUTE TRADE (confidence > 0.65)")
                else:
                    print(f"  Decision: SKIP (confidence <= 0.65)")
        
        # Test JSON communication format (matching Go's predictor)
        print(f"\n\nTesting JSON communication format:")
        test_request = {
            "features": [[0.2, 0.1, 0.5]]
        }
        
        print(f"Request: {json.dumps(test_request)}")
        
        # Simulate the response format
        outputs = session.run(output_names, {input_name: np.array(test_request["features"], dtype=np.float32)})
        
        response = {
            "label": int(outputs[0][0]) if hasattr(outputs[0], '__getitem__') else int(outputs[0]),
            "probability": float(outputs[1][0]) if hasattr(outputs[1], '__getitem__') else float(outputs[1])
        }
        
        print(f"Response: {json.dumps(response)}")
        
        print("\n✅ Model inference successful!")
        return True
        
    except Exception as e:
        print(f"\n❌ Error during inference: {e}")
        import traceback
        traceback.print_exc()
        return False

def main():
    print("🧪 Testing ML Model Predictions")
    print("==============================\n")
    
    if test_model():
        print("\n✅ ML pipeline is working correctly!")
        print("\nIntegration checklist:")
        print("✓ Model loads successfully")
        print("✓ Accepts 3-feature input: [tickRatio, depthRatio, priceDistance]")
        print("✓ Returns label and probability")
        print("✓ JSON communication format works")
        print("\nNext steps:")
        print("1. Ensure model.onnx is in the models/ directory")
        print("2. Run the Go bot: go run cmd/bitrader/main.go")
        print("3. Monitor predictions in the logs")
    else:
        print("\n❌ Model testing failed")
        print("Please check the error messages above")

if __name__ == "__main__":
    main()
