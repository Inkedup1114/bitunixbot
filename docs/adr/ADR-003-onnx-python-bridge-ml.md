# ADR-003: ONNX Runtime with Python Bridge for ML

## Status
Accepted

## Date
2025-01-31

## Context

The Bitunix Trading Bot requires machine learning capabilities for:
- Market signal prediction and validation
- Trading decision gate mechanism
- Feature importance analysis and model drift detection
- Online learning and model adaptation
- A/B testing of different models

ML Requirements:
- **Real-time Inference**: Low-latency predictions during trading
- **Model Portability**: Standard format for model deployment
- **Performance**: Fast inference without blocking trading operations
- **Fallback Mechanisms**: Graceful degradation when ML is unavailable
- **Integration**: Seamless integration with Go-based trading system
- **Training Pipeline**: Separate training environment with rich ML ecosystem

Options considered:
- **TensorFlow Lite**: Good performance, but limited Go support
- **ONNX Runtime**: Cross-platform, good Go bindings via CGO
- **PyTorch Mobile**: Good for mobile, limited server deployment
- **Native Go ML**: Limited ecosystem and performance
- **Python Subprocess**: Flexible but with IPC overhead
- **gRPC ML Service**: Network overhead and complexity

## Decision

We chose **ONNX Runtime with Python Bridge** for machine learning integration.

### Architecture:
1. **Training**: Python-based training pipeline with scikit-learn/XGBoost
2. **Model Export**: ONNX format with quantization for production
3. **Inference**: Python subprocess calls from Go with timeout protection
4. **Fallback**: Heuristic-based trading when ML is unavailable

### Key Factors:

1. **Standard Format**: ONNX provides cross-platform model portability
2. **Performance**: Optimized runtime with quantization support
3. **Ecosystem**: Rich Python ML ecosystem for training and experimentation
4. **Simplicity**: Avoid CGO complexity while maintaining performance
5. **Isolation**: Python subprocess isolation prevents crashes
6. **Flexibility**: Easy model updates without recompiling Go binary

## Consequences

### Positive:
- **Rich ML Ecosystem**: Access to full Python ML stack for training
- **Model Portability**: ONNX models work across different runtimes
- **Performance**: Quantized models provide fast inference
- **Isolation**: Subprocess isolation prevents ML crashes from affecting trading
- **Easy Updates**: Model updates without recompiling the main application
- **Fallback Support**: Graceful degradation to heuristic-based trading
- **Development Speed**: Rapid ML experimentation and deployment

### Negative:
- **Subprocess Overhead**: IPC latency for each prediction call
- **Dependency Management**: Python environment setup complexity
- **Memory Usage**: Additional memory for Python runtime
- **Deployment Complexity**: Need to manage Python dependencies

### Mitigations:
- **Prediction Caching**: Cache recent predictions to reduce subprocess calls
- **Batch Predictions**: Group multiple predictions when possible
- **Health Checks**: Monitor Python subprocess health and restart if needed
- **Timeout Protection**: 5-second timeout prevents hanging predictions
- **Containerization**: Docker containers for consistent Python environments

## Implementation Details

### Training Pipeline:
```python
# scripts/label_and_train.py
def train_model(features, labels):
    model = XGBClassifier(
        n_estimators=100,
        max_depth=6,
        learning_rate=0.1,
        random_state=42
    )
    model.fit(features, labels)
    
    # Export to ONNX with quantization
    onnx_model = convert_sklearn(
        model, 
        initial_types=[('float_input', FloatTensorType([None, len(feature_names)]))],
        target_opset=11
    )
    
    # Quantize for production
    quantized_model = quantize_dynamic(onnx_model, weight_type=QuantType.QUInt8)
    return quantized_model
```

### Go Integration:
```go
// internal/ml/predictor.go
type Predictor struct {
    modelPath string
    timeout   time.Duration
    cache     *PredictionCache
}

func (p *Predictor) Predict(features []float32) ([]float32, error) {
    // Check cache first
    if cached := p.cache.Get(features); cached != nil {
        return cached, nil
    }
    
    // Create Python subprocess
    ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, "python3", "scripts/onnx_inference.py", p.modelPath)
    cmd.Stdin = strings.NewReader(formatFeatures(features))
    
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("prediction failed: %w", err)
    }
    
    predictions := parseOutput(output)
    p.cache.Set(features, predictions)
    return predictions, nil
}
```

### Fallback Mechanism:
```go
func (p *Predictor) PredictWithFallback(features []float32) (float32, error) {
    // Try ML prediction first
    if predictions, err := p.Predict(features); err == nil {
        return predictions[1], nil // Binary classification probability
    }
    
    // Fall back to heuristic
    return p.heuristicPredict(features), nil
}

func (p *Predictor) heuristicPredict(features []float32) float32 {
    // Simple heuristic based on VWAP and imbalance
    vwap := features[0]
    imbalance := features[1]
    volume := features[2]
    
    if imbalance > 0.1 && volume > 1000 {
        return 0.7 // High confidence
    }
    return 0.3 // Low confidence
}
```

### Model Management:
- **Versioning**: Models include timestamp and performance metrics
- **A/B Testing**: Support for multiple model variants
- **Health Monitoring**: Track prediction latency and error rates
- **Automatic Rollback**: Revert to previous model if performance degrades

## Performance Characteristics

- **Prediction Latency**: ~50-100ms including subprocess overhead
- **Cache Hit Rate**: ~80% for similar market conditions
- **Memory Usage**: ~100MB for Python runtime + model
- **Throughput**: ~20 predictions/second with caching

## Related ADRs
- ADR-001: Go as Primary Language
- ADR-009: Circuit Breaker Pattern for Risk Management
