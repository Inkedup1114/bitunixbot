# Troubleshooting Guide

This guide helps you diagnose and resolve common issues with the Bitunix Trading Bot.

## Table of Contents
- [Bot Won't Start](#bot-wont-start)
- [ML Model Issues](#ml-model-issues)
- [Trading Problems](#trading-problems)
- [Connection Issues](#connection-issues)
- [Performance Issues](#performance-issues)
- [Logging and Debugging](#logging-and-debugging)

## Bot Won't Start

### Configuration Issues

**Problem**: Bot exits immediately with configuration errors.

**Solutions**:
1. Check your configuration file syntax:
   ```bash
   # Validate YAML syntax
   python -c "import yaml; yaml.safe_load(open('config.yaml'))"
   ```

2. Verify required environment variables:
   ```bash
   echo $BITUNIX_API_KEY
   echo $BITUNIX_SECRET_KEY
   ```

3. Check configuration validation errors in logs:
   ```bash
   ./bitrader 2>&1 | grep -i "validation failed"
   ```

**Common validation errors**:
- `max daily loss must be between 0 and 0.5`: Set `MAX_DAILY_LOSS` between 0-50%
- `probability threshold must be between 0.5 and 0.99`: Adjust `PROB_THRESHOLD`
- `metrics port must be between 1024 and 65535`: Change `METRICS_PORT`

### Permission Issues

**Problem**: Permission denied errors when accessing files.

**Solutions**:
1. Check file permissions:
   ```bash
   ls -la model.onnx
   ls -la data/
   ```

2. Fix permissions:
   ```bash
   chmod 644 model.onnx
   chmod 755 data/
   ```

3. For Docker deployments, ensure user 65532 has access:
   ```bash
   chown -R 65532:65532 /srv/data
   ```

## ML Model Issues

### Model Not Found

**Problem**: "ONNX model not found, using fallback heuristics"

**Solutions**:
1. Check model file exists:
   ```bash
   ls -la model.onnx
   ```

2. Verify model path in configuration:
   ```yaml
   ml:
     modelPath: "model.onnx"
   ```

3. Train a new model:
   ```bash
   python scripts/label_and_train.py --data-file scripts/training_data.json
   ```

### Python/ONNX Runtime Issues

**Problem**: "Python not found" or "onnxruntime not installed"

**Solutions**:
1. Install Python 3:
   ```bash
   # Ubuntu/Debian
   sudo apt install python3 python3-pip
   
   # Alpine Linux
   apk add python3 py3-pip
   ```

2. Install ONNX Runtime:
   ```bash
   pip3 install onnxruntime
   ```

3. For Docker, rebuild the image with dependencies:
   ```bash
   docker build -f deploy/Dockerfile -t bitunix-bot .
   ```

### Model Performance Issues

**Problem**: Low prediction accuracy or frequent fallback usage.

**Diagnostics**:
1. Check ML metrics:
   ```bash
   curl http://localhost:8080/metrics | grep ml_
   ```

2. Review model metrics file:
   ```bash
   cat model-$(date +%Y%m%d)_metrics.json
   ```

**Solutions**:
1. Retrain with more data:
   ```bash
   python scripts/label_and_train.py --min-samples 5000
   ```

2. Adjust hyperparameters:
   ```bash
   python scripts/label_and_train.py --sigma-threshold 2.0 --lookhead-seconds 120
   ```

3. Collect more training data:
   ```bash
   go run scripts/export_data.go -db data/features.db -output scripts/training_data.json
   ```

## ML Model Debugging

### Model Performance Issues

**Problem**: Low prediction accuracy or poor trading performance.

**Diagnostic Steps**:
1. Check model metrics:
   ```bash
   # View model training metrics
   cat model-*_metrics.json | jq '.cv_results'
   
   # Check feature importance
   cat model-*_metrics.json | jq '.feature_importance'
   ```

2. Monitor prediction scores in real-time:
   ```bash
   # Check Prometheus metrics
   curl -s http://localhost:8080/metrics | grep ml_prediction_scores
   ```

3. Validate model age:
   ```bash
   # Check when model was last trained
   stat -c %Y model.onnx
   ```

**Solutions**:
- Retrain model with more recent data if model is >7 days old
- Check class imbalance in training data (`class_ratio` in metrics)
- Adjust `SIGMA_THRESHOLD` or `LOOKHEAD_SECONDS` in training pipeline

### Model Loading Issues

**Problem**: "ONNX runtime dependency missing" errors.

**Solutions**:
1. Install ONNX runtime:
   ```bash
   pip install onnxruntime==1.17.3
   ```

2. For Docker environments:
   ```dockerfile
   RUN pip install --no-cache-dir onnxruntime==1.17.3
   ```

3. Verify Python path:
   ```bash
   which python3
   python3 -c "import onnxruntime; print('OK')"
   ```

### Feature Calculation Errors

**Problem**: High `feature_errors_total` in metrics.

**Diagnostic Steps**:
1. Check feature error logs:
   ```bash
   journalctl -u bitunix-bot | grep "feature calculation error"
   ```

2. Monitor feature values:
   ```bash
   # Check for NaN/Inf values in logs
   grep -i "nan\|inf" /var/log/bitunix-bot.log
   ```

**Solutions**:
- Ensure sufficient price history for VWAP calculation
- Check for market data gaps during low-volume periods
- Increase `VWAP_WINDOW` if getting insufficient data

## Trading Problems

### Orders Not Being Placed

**Problem**: Bot runs but doesn't place any orders.

**Diagnostics**:
1. Check if predictions are being made:
   ```bash
   curl http://localhost:8080/metrics | grep ml_predictions_total
   ```

2. Verify feature calculation:
   ```bash
   curl http://localhost:8080/metrics | grep feature_errors_total
   ```

3. Check dry run mode:
   ```bash
   grep -i "dry.run" config.yaml
   ```

**Solutions**:
1. Disable dry run mode:
   ```yaml
   trading:
     dryRun: false
   ```

2. Lower prediction threshold:
   ```yaml
   ml:
     probThreshold: 0.6  # Lower from 0.65
   ```

3. Check position limits:
   ```yaml
   trading:
     maxPositionSize: 0.02  # Increase from 0.01
   ```

### Frequent Position Closures

**Problem**: Positions are closed too quickly.

**Solutions**:
1. Increase max price distance:
   ```yaml
   trading:
     maxPriceDistance: 5.0  # Increase from 3.0
   ```

2. Adjust stop-loss parameters in code
3. Review risk management settings

## Connection Issues

### WebSocket Disconnections

**Problem**: Frequent "WebSocket reconnection" messages.

**Diagnostics**:
```bash
curl http://localhost:8080/metrics | grep ws_reconnects_total
```

**Solutions**:
1. Check network stability
2. Increase ping interval:
   ```yaml
   system:
     pingInterval: "30s"  # Increase from 15s
   ```

3. Verify API endpoints:
   ```yaml
   api:
     wsURL: "wss://fapi.bitunix.com/public"
   ```

### API Rate Limiting

**Problem**: "Rate limit exceeded" errors.

**Solutions**:
1. Increase REST timeout:
   ```yaml
   system:
     restTimeout: "10s"  # Increase from 5s
   ```

2. Reduce trading frequency
3. Check API key permissions

## Performance Issues

### High Memory Usage

**Diagnostics**:
1. Monitor memory usage:
   ```bash
   docker stats bitunix-bot
   ```

2. Check for memory leaks in logs

**Solutions**:
1. Reduce VWAP window size:
   ```yaml
   features:
     vwapWindow: "15s"  # Reduce from 30s
     vwapSize: 300      # Reduce from 600
   ```

2. Restart the bot periodically
3. Update to latest version

### High CPU Usage

**Diagnostics**:
1. Check ML prediction frequency:
   ```bash
   curl http://localhost:8080/metrics | grep ml_latency
   ```

**Solutions**:
1. Reduce tick size:
   ```yaml
   features:
     tickSize: 25  # Reduce from 50
   ```

2. Increase ML timeout:
   ```yaml
   # In Go code or environment
   ML_TIMEOUT=10s
   ```

## Logging and Debugging

### Enable Debug Logging

1. Set environment variable:
   ```bash
   export LOG_LEVEL=debug
   ```

2. For Docker:
   ```bash
   docker run -e LOG_LEVEL=debug bitunix-bot
   ```

### Collect Diagnostics

1. Export logs:
   ```bash
   docker logs bitunix-bot > bot-logs.txt 2>&1
   ```

2. Export metrics:
   ```bash
   curl http://localhost:8080/metrics > metrics.txt
   ```

3. Export configuration:
   ```bash
   cp config.yaml config-backup.yaml
   ```

### Common Log Messages

| Message | Meaning | Action |
|---------|---------|--------|
| "Model health check failed" | ML model not responding | Check Python/ONNX setup |
| "Feature calculation error" | Data processing issue | Check market data quality |
| "Position size too large" | Risk management triggered | Adjust position limits |
| "API key validation failed" | Authentication issue | Check API credentials |

## Performance Monitoring

### Metrics Dashboard Setup

Set up Grafana dashboard for monitoring:

```yaml
# docker-compose.yml
version: '3.8'
services:
  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
  
  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
```

### Key Metrics to Monitor

1. **ML Performance**:
   - `ml_latency_seconds`: Prediction latency
   - `ml_prediction_scores`: Score distribution
   - `ml_failures_total`: Prediction failures
   - `ml_timeouts_total`: Timeout occurrences

2. **Trading Performance**:
   - `orders_total`: Order placement rate
   - `pnl_total`: Profit/loss tracking
   - `active_positions`: Position count

3. **System Health**:
   - `ws_reconnects_total`: Connection stability
   - `errors_total`: General error rate

### Log Analysis

**Find specific error patterns**:
```bash
# Model-related errors
journalctl -u bitunix-bot | grep -i "onnx\|python\|model"

# Trading errors
journalctl -u bitunix-bot | grep -i "order\|position\|trade"

# Connection errors
journalctl -u bitunix-bot | grep -i "websocket\|connection\|reconnect"
```

**Performance analysis**:
```bash
# Find slow operations
journalctl -u bitunix-bot | grep -i "timeout\|slow\|latency"

# Memory issues
dmesg | grep -i "killed process.*bitrader"
```

## Emergency Procedures

### Stop Trading Immediately

1. **Graceful shutdown**:
   ```bash
   systemctl stop bitunix-bot
   ```

2. **Emergency stop** (if unresponsive):
   ```bash
   pkill -9 bitrader
   ```

3. **Cancel all open orders** (manual):
   ```bash
   # Use exchange API or web interface
   curl -X DELETE "https://api.bitunix.com/v1/orders" \
     -H "Authorization: Bearer $API_TOKEN"
   ```

### Data Recovery

**Backup critical data**:
```bash
# Backup model and configuration
tar -czf backup-$(date +%Y%m%d).tar.gz \
  model.onnx config.yaml data/ logs/
```

**Restore from backup**:
```bash
# Extract backup
tar -xzf backup-20250530.tar.gz

# Verify model integrity
python3 -c "
import onnxruntime as ort
session = ort.InferenceSession('model.onnx')
print('Model OK')
"
```

## Getting Help

### Log Collection for Support

```bash
# Collect comprehensive logs
mkdir support-logs-$(date +%Y%m%d)
cd support-logs-$(date +%Y%m%d)

# System info
uname -a > system-info.txt
docker --version >> system-info.txt
python3 --version >> system-info.txt

# Application logs
journalctl -u bitunix-bot --since "24 hours ago" > app-logs.txt

# System metrics
curl -s http://localhost:8080/metrics > prometheus-metrics.txt

# Configuration (sanitize first!)
cp ../config.yaml config-sanitized.yaml
sed -i 's/key:.*/key: [REDACTED]/' config-sanitized.yaml
sed -i 's/secret:.*/secret: [REDACTED]/' config-sanitized.yaml

# Model info
ls -la ../model* > model-info.txt
cat ../model-*_metrics.json > model-metrics.json 2>/dev/null || echo "No metrics found"

# Create archive
cd ..
tar -czf support-logs-$(date +%Y%m%d).tar.gz support-logs-$(date +%Y%m%d)/
```

### Community Resources

- **GitHub Issues**: Report bugs and feature requests
- **Documentation**: Check README.md and DEPLOYMENT.md
- **Model Training**: See ML_PIPELINE.md for retraining

### Professional Support

For production deployments requiring professional support:
- Include the support logs archive
- Specify your deployment environment (Docker, Kubernetes, bare metal)
- Describe the trading strategy and risk parameters
- Provide timeline of when issues started occurring
