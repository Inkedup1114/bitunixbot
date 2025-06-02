#!/usr/bin/env python3
"""
ONNX Inference Script for Bitunix Trading Bot
Reads JSON from stdin, returns prediction JSON to stdout

This is a standalone script that can be pre-built into the Docker image
to avoid runtime script generation.
"""
import sys
import json
import numpy as np

# Try to import onnxruntime with proper error handling
try:
    import onnxruntime as ort
    ONNX_AVAILABLE = True
except ImportError as e:
    ONNX_AVAILABLE = False
    IMPORT_ERROR = str(e)

def main():
    if len(sys.argv) != 2:
        print(json.dumps({"error": "Usage: python onnx_inference.py <model_path>"}))
        sys.exit(1)
    
    # Check if onnxruntime is available
    if not ONNX_AVAILABLE:
        error_msg = (
            f"onnxruntime not installed. Error: {IMPORT_ERROR}\n"
            "Please install with: pip install onnxruntime==1.17.3"
        )
        print(json.dumps({"error": error_msg}))
        sys.exit(1)
    
    model_path = sys.argv[1]
    
    try:
        # Read input from stdin
        request = json.load(sys.stdin)
        features = np.array([request["features"]], dtype=np.float32)
        
        # Load ONNX model
        session = ort.InferenceSession(model_path)
        input_name = session.get_inputs()[0].name
        
        # Run inference
        outputs = session.run(None, {input_name: features})
        
        # Handle different output formats
        # GradientBoostingClassifier typically outputs:
        # - Output 0: predicted class (shape: [1])
        # - Output 1: probabilities (shape: [1, 2])
        
        if len(outputs) == 2:
            # Standard sklearn classifier format
            prediction_output = outputs[0]
            probabilities_output = outputs[1]
            
            # Handle both array and dict outputs
            if hasattr(prediction_output, 'tolist'):
                prediction = int(prediction_output[0])
            else:
                prediction = int(prediction_output)
                
            # Handle different probability output formats
            if isinstance(probabilities_output, dict):
                # Extract probabilities from dict format
                prob_values = list(probabilities_output.values())[0]
                if hasattr(prob_values, 'tolist'):
                    probabilities = prob_values.tolist() if len(prob_values.shape) == 1 else prob_values[0].tolist()
                else:
                    probabilities = list(prob_values) if len(prob_values.shape) == 1 else list(prob_values[0])
            elif hasattr(probabilities_output, 'tolist'):
                probabilities = probabilities_output[0].tolist()
            else:
                probabilities = list(probabilities_output[0])
                    
        elif len(outputs) == 1:
            # Single output - could be probabilities or prediction
            output = outputs[0]
            
            if hasattr(output, 'shape') and len(output.shape) > 1 and output.shape[-1] == 2:
                # Probabilities only
                probabilities = output[0].tolist() if hasattr(output, 'tolist') else list(output[0])
                prediction = int(np.argmax(probabilities))
            else:
                # Single prediction value
                if hasattr(output, 'tolist'):
                    pred_val = output[0]
                else:
                    pred_val = output
                prediction = int(pred_val)
                # Assume binary classification with threshold
                prob_positive = float(pred_val)
                probabilities = [1.0 - prob_positive, prob_positive]
        else:
            raise ValueError(f"Unexpected number of outputs: {len(outputs)}")
        
        # Ensure probabilities sum to 1 (handle numerical issues)
        prob_sum = sum(probabilities)
        if abs(prob_sum - 1.0) > 0.01:
            probabilities = [p / prob_sum for p in probabilities]
        
        response = {
            "probabilities": probabilities,
            "prediction": prediction
        }
        
        print(json.dumps(response))
        
    except Exception as e:
        error_response = {"error": str(e)}
        print(json.dumps(error_response))
        sys.exit(1)

if __name__ == "__main__":
    main()
