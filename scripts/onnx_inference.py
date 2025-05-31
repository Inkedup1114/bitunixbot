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
import logging
from pathlib import Path

# Configure logging for better debugging
logging.basicConfig(
    level=logging.WARNING,  # Only show warnings/errors
    format='%(asctime)s - %(levelname)s - %(message)s',
    stream=sys.stderr  # Log to stderr so stdout is clean for JSON
)

try:
    import onnxruntime as ort
except ImportError as e:
    error_response = {"error": f"onnxruntime not installed: {e}"}
    print(json.dumps(error_response))
    sys.exit(1)

def validate_model_path(model_path: str) -> str:
    """Validate and resolve model path"""
    path = Path(model_path)
    if not path.exists():
        raise FileNotFoundError(f"Model file not found: {model_path}")
    if not path.is_file():
        raise ValueError(f"Model path is not a file: {model_path}")
    if not str(path).endswith('.onnx'):
        logging.warning(f"Model file doesn't have .onnx extension: {model_path}")
    return str(path.resolve())

def validate_features(features: list) -> np.ndarray:
    """Validate and convert features to numpy array"""
    if not isinstance(features, list):
        raise ValueError(f"Features must be a list, got {type(features)}")
    
    if len(features) != 3:
        raise ValueError(f"Expected 3 features, got {len(features)}")
    
    # Convert to numpy array and validate
    try:
        features_array = np.array([features], dtype=np.float32)
    except (ValueError, TypeError) as e:
        raise ValueError(f"Invalid feature values: {e}")
    
    # Check for NaN/Inf values
    if not np.isfinite(features_array).all():
        raise ValueError("Features contain NaN or infinite values")
    
    # Check for extremely large values
    if np.abs(features_array).max() > 1e10:
        raise ValueError("Features contain extremely large values")
    
    return features_array

def create_onnx_session(model_path: str) -> ort.InferenceSession:
    """Create ONNX inference session with optimized settings"""
    try:
        # Configure session options for better performance
        session_options = ort.SessionOptions()
        session_options.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL
        session_options.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL
        
        # Create session
        session = ort.InferenceSession(
            model_path, 
            sess_options=session_options,
            providers=['CPUExecutionProvider']  # Explicitly use CPU
        )
        
        return session
        
    except Exception as e:
        raise RuntimeError(f"Failed to load ONNX model: {e}")

def run_inference(session: ort.InferenceSession, features: np.ndarray) -> dict:
    """Run inference and format response"""
    try:
        # Get input name
        input_name = session.get_inputs()[0].name
        
        # Run inference
        result = session.run(None, {input_name: features})
        
        # Format response based on model output structure
        if len(result) >= 2:
            # Standard sklearn->ONNX format: [labels, probabilities]
            probabilities = result[1][0].tolist()
            prediction = int(result[0][0])
        else:
            # Single output format
            probabilities = result[0][0].tolist()
            prediction = int(np.argmax(probabilities))
        
        # Validate probabilities
        if not isinstance(probabilities, list) or len(probabilities) != 2:
            raise ValueError(f"Expected 2 probabilities, got {len(probabilities) if isinstance(probabilities, list) else 'non-list'}")
        
        for i, prob in enumerate(probabilities):
            if not (0 <= prob <= 1):
                raise ValueError(f"Probability {i} out of range [0,1]: {prob}")
        
        return {
            "probabilities": probabilities,
            "prediction": prediction,
            "success": True
        }
        
    except Exception as e:
        raise RuntimeError(f"Inference failed: {e}")

def main():
    """Main inference function"""
    if len(sys.argv) != 2:
        error_response = {"error": "Usage: python onnx_inference.py <model_path>"}
        print(json.dumps(error_response))
        sys.exit(1)
    
    model_path = sys.argv[1]
    
    try:
        # Validate model path
        validated_model_path = validate_model_path(model_path)
        
        # Read input from stdin
        try:
            request = json.load(sys.stdin)
        except json.JSONDecodeError as e:
            raise ValueError(f"Invalid JSON input: {e}")
        
        # Validate request structure
        if not isinstance(request, dict):
            raise ValueError("Request must be a JSON object")
        
        if "features" not in request:
            raise ValueError("Request must contain 'features' field")
        
        # Validate and prepare features
        features = validate_features(request["features"])
        
        # Create ONNX session
        session = create_onnx_session(validated_model_path)
        
        # Run inference
        response = run_inference(session, features)
        
        # Output successful response
        print(json.dumps(response))
        
    except Exception as e:
        # Log error for debugging
        logging.error(f"Inference error: {e}")
        
        # Return error response
        error_response = {
            "error": str(e),
            "success": False
        }
        print(json.dumps(error_response))
        sys.exit(1)

if __name__ == "__main__":
    main()
