# Bitunix Bot Troubleshooting Guide

Last Updated: 2024-12-19

## Table of Contents
- [Quick Diagnosis](#quick-diagnosis)
- [Common Issues](#common-issues)
- [ML Pipeline Issues](#ml-pipeline-issues)
- [WebSocket Connection Issues](#websocket-connection-issues)
- [Configuration Issues](#configuration-issues)
- [Performance Issues](#performance-issues)
- [Deployment Issues](#deployment-issues)
- [Security Issues](#security-issues)
- [Emergency Procedures](#emergency-procedures)

## Quick Diagnosis

### Health Check Commands
```bash
# Check if bot is running
curl -s http://localhost:8080/metrics | grep -E "up|bitunix_"

# Check ML model status
python3 -c "import onnxruntime; print('ONNX Runtime OK')"

# Verify configuration
go run cmd/bitrader/main.go -dry-run=true

# Test WebSocket connectivity
wscat -c wss://fapi.bitunix.com/public
```

## Common Issues

### 1. Bot Won't Start

**Symptoms**: Application exits immediately or hangs on startup

**Diagnosis**:
```bash
# Check for missing environment variables
env | grep BITUNIX

# Verify config file exists
ls -la config.yaml

# Check for port conflicts
lsof -i :8080
```

**Solutions**:

1. **Missing API Credentials**:
   ```bash
   # Set required environment variables
   export BITUNIX_API_KEY="your_api_key"
   export BITUNIX_SECRET_KEY="your_secret_key"
   
   # Or use .env file
   cp .env.example .env
   nano .env  # Add your credentials
   source .env
   ```

2. **Config File Issues**:
   ```bash
   # Create config from example
   cp config.yaml.example config.yaml
   
   # Validate YAML syntax
   python3 -c "import yaml; yaml.safe_load(open('config.yaml'))"
   ```

3. **Port Already in Use**:
   ```bash
   # Find process using port 8080
   sudo lsof -i :8080
   
   # Kill the process or change port
   export METRICS_PORT=8081
   ```

### 2. No Trading Activity

**Symptoms**: Bot runs but doesn't place any trades

**Common Causes**:
- Dry run mode enabled
- ML model threshold too high
- Insufficient market volatility
- Symbol configuration issues

**Solutions**:

1. **Check Dry Run Mode**:
   ```yaml
   # In config.yaml, ensure:
   trading:
     dryRun: false  # Set to false for live trading
   ```

2. **Adjust ML Threshold**:
   ```yaml
   ml:
     probThreshold: 0.65  # Lower to 0.6 for more signals
   ```

3. **Verify Symbol Configuration**:
   ```yaml
   trading:
     symbols: ["BTCUSDT", "ETHUSDT"]  # Ensure symbols are valid
   ```

4. **Check Feature Calculations**:
   ```bash
   # Monitor feature values
   curl -s http://localhost:8080/metrics | grep -E "tick_ratio|depth_ratio|price_dist"
   ```

### 3. High Memory Usage

**Symptoms**: Memory consumption grows over time

**Solutions**:

1. **Reduce VWAP Window Size**:
   ```yaml
   features:
     vwapSize: 300  # Reduce from 600 to 300
   ```

2. **Enable GC Tuning**:
   ```bash
   export GOGC=20  # More aggressive garbage collection
   export GOMEMLIMIT=512MiB  # Set memory limit
   ```

3. **Monitor Memory Leaks**:
   ```bash
   # Generate heap profile
   curl http://localhost:8080/debug/pprof/heap > heap.prof
   go tool pprof heap.prof
   ```

## ML Pipeline Issues

### ML Model Not Loading

**Error**: `WARN ML model unavailable, using fallback`

**Solutions**:

1. **Check Model File**:
   ```bash
   # Verify model exists
   ls -la models/model.onnx
   
   # Validate model format
   python3 -c "
   import onnxruntime as ort
   session = ort.InferenceSession('models/model.onnx')
   print('Model OK, inputs:', [i.name for i in session.get_inputs()])
   "
   ```

2. **Train New Model**:
   ```bash
   # Quick training with sample data
   cd scripts/
   python3 label_and_train.py
   cd ..
   ```

3. **Fix Python Path**:
   ```bash
   # Ensure Python is available
   which python3 || alias python3=python
   
   # Install ONNX runtime
   pip3 install onnxruntime==1.16.1
   ```

### ML Predictions Timing Out

**Error**: `ML prediction timeout`

**Solutions**:

1. **Increase Timeout**:
   ```yaml
   ml:
     timeout: "10s"  # Increase from 5s
   ```

2. **Optimize Model**:
   ```python
   # Re-export with optimization
   python3 scripts/label_and_train.py --optimize
   ```

3. **Use Fallback Strategy**:
   ```yaml
   ml:
     fallbackStrategy: "conservative"  # Use heuristics on timeout
   ```

## WebSocket Connection Issues

### Connection Drops Frequently

**Symptoms**: Repeated reconnection messages in logs

**Solutions**:

1. **Check Network Stability**:
   ```bash
   # Test WebSocket endpoint
   wscat -c wss://fapi.bitunix.com/public
   
   # Monitor network latency
   ping api.bitunix.com
   ```

2. **Adjust Ping Interval**:
   ```yaml
   system:
     pingInterval: "10s"  # More frequent pings
   ```

3. **Enable Reconnection Backoff**:
   ```go
   // Already implemented with exponential backoff
   // Check logs for reconnection patterns
   grep -i "websocket.*reconnect" logs/*.log
   ```

### No Market Data Received

**Symptoms**: Empty order books, no trades

**Solutions**:

1. **Verify Subscription**:
   ```bash
   # Check WebSocket messages
   export LOG_LEVEL=debug
   go run cmd/bitrader/main.go 2>&1 | grep -i "subscribe"
   ```

2. **Check Symbol Format**:
   ```yaml
   trading:
     symbols: ["BTCUSDT"]  # Ensure correct format (no spaces, uppercase)
   ```

## Configuration Issues

### UFW Firewall Issues

**Symptoms**: WebSocket connections fail, API requests timeout, or "connection refused" errors

**UFW Configuration Check**:
```bash
# Check UFW status
sudo ufw status verbose

# Expected output should show:
# - Default: allow (outgoing) - Required for API connections
# - Port 8080 open (for metrics endpoint)
```

**Required UFW Rules**:
```bash
# Allow outgoing HTTPS (port 443) - Usually allowed by default
sudo ufw allow out 443/tcp

# Allow outgoing HTTP (port 80) - Usually allowed by default  
sudo ufw allow out 80/tcp

# Allow incoming on metrics port (if accessing from other machines)
sudo ufw allow 8080/tcp

# Allow outgoing WebSocket connections (port 443 for WSS)
sudo ufw allow out on any to any port 443 proto tcp
```

**Test Network Connectivity**:
```bash
# Test HTTPS API connectivity
curl -v --connect-timeout 10 https://api.bitunix.com

# Test if outgoing connections work
curl -v --connect-timeout 10 https://httpbin.org/ip

# Check if DNS resolution works
nslookup api.bitunix.com
nslookup fapi.bitunix.com
```

**If connections still fail**:
```bash
# Temporarily disable UFW for testing (CAUTION: Only for troubleshooting)
sudo ufw --force disable

# Test bot connectivity
go run cmd/bitrader/main.go -dry-run=true

# Re-enable UFW after testing
sudo ufw --force enable
```

**UFW Logs for Debugging**:
```bash
# Check UFW logs for blocked connections
sudo tail -f /var/log/ufw.log | grep -i "BLOCK"

# Look for blocked connections to Bitunix
sudo grep -i "bitunix\|104.18" /var/log/ufw.log
```

### Environment vs Config File Conflicts

**Issue**: Settings not taking effect

**Resolution Priority**:
1. Command-line flags (highest)
2. Environment variables
3. Config file
4. Defaults (lowest)

**Debug Configuration**:
```bash
# Show effective configuration
go run cmd/bitrader/main.go -show-config

# Test with specific config
go run cmd/bitrader/main.go -config=test-config.yaml
```

### Invalid Configuration Values

**Common Errors**:

1. **Invalid Size Ratios**:
   ```yaml
   trading:
     baseSizeRatio: 0.001  # Must be between 0.0001 and 0.1
     maxPositionSize: 0.01  # Must be less than 0.1
   ```

2. **Invalid Timeouts**:
   ```yaml
   api:
     timeout: "5s"  # Format: number + s/m/h
   ```

## Performance Issues

### Slow Order Execution

**Symptoms**: High latency between signal and order placement

**Solutions**:

1. **Reduce API Timeout**:
   ```yaml
   api:
     timeout: "3s"  # Reduce from 5s
   ```

2. **Optimize Feature Calculations**:
   ```yaml
   features:
     tickSize: 25  # Reduce from 50 for faster processing
   ```

3. **Use Market Orders**:
   ```go
   // Already implemented - ensure using MARKET orders
   // Check order type in logs
   ```

### High CPU Usage

**Solutions**:

1. **Limit Symbols**:
   ```yaml
   trading:
     symbols: ["BTCUSDT"]  # Trade fewer symbols
   ```

2. **Adjust Processing Intervals**:
   ```bash
   # Set CPU limits
   export GOMAXPROCS=2  # Limit to 2 cores
   ```

3. **Profile CPU Usage**:
   ```bash
   # Generate CPU profile
   curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof
   go tool pprof cpu.prof
   ```

## Deployment Issues

### Docker Container Exits

**Common Causes**:

1. **Missing Environment Variables**:
   ```bash
   # Use env-file
   docker run --env-file .env bitunix-bot
   ```

2. **Permission Issues**:
   ```bash
   # Fix data directory permissions
   docker run -v $(pwd)/data:/srv/data:Z bitunix-bot
   ```

3. **Resource Limits**:
   ```bash
   # Increase memory limit
   docker run -m 1g bitunix-bot
   ```

### Kubernetes Pod CrashLoopBackOff

**Diagnosis**:
```bash
# Check pod logs
kubectl logs -n bitunix-bot <pod-name> --previous

# Check events
kubectl describe pod -n bitunix-bot <pod-name>
```

**Common Fixes**:

1. **Secret Not Found**:
   ```bash
   kubectl create secret generic bitunix-credentials \
     --from-literal=api-key="your_key" \
     --from-literal=secret-key="your_secret" \
     -n bitunix-bot
   ```

2. **Liveness Probe Failing**:
   ```yaml
   # Increase probe delays
   livenessProbe:
     initialDelaySeconds: 60  # Increase from 30
   ```

## Security Issues

### API Credentials Exposed

**CRITICAL**: If credentials were committed to Git

**Immediate Actions**:

1. **Revoke Compromised Keys**:
   - Log into Bitunix account
   - Generate new API keys immediately
   - Update all deployments

2. **Clean Git History**:
   ```bash
   # Remove from git history
   git filter-branch --force --index-filter \
     'git rm --cached --ignore-unmatch .env' \
     --prune-empty --tag-name-filter cat -- --all
   ```

3. **Prevent Future Exposure**:
   ```bash
   # Ensure .env is gitignored
   echo ".env" >> .gitignore
   git add .gitignore
   git commit -m "Add .env to gitignore"
   ```

## Emergency Procedures

### 1. Stop All Trading Immediately

```bash
# Method 1: Enable dry run
export DRY_RUN=true
pkill -SIGHUP bitrader  # Reload config

# Method 2: Scale down deployment
kubectl scale deployment bitunix-bot --replicas=0 -n bitunix-bot

# Method 3: Kill process
pkill -9 bitrader
```

### 2. Recover from Bad State

```bash
# 1. Stop the bot
systemctl stop bitunix-bot

# 2. Backup current state
cp data/bitunix-data.db data/bitunix-data.db.backup

# 3. Reset state (optional)
rm data/bitunix-data.db

# 4. Restart with dry run
DRY_RUN=true systemctl start bitunix-bot

# 5. Verify operation before enabling trading
```

### 3. Debug Production Issues

```bash
# Enable debug logging temporarily
export LOG_LEVEL=debug

# Capture debug output
timeout 60 go run cmd/bitrader/main.go 2>&1 | tee debug.log

# Analyze specific issues
grep -i error debug.log
grep -i "ml prediction" debug.log
grep -i "order.*failed" debug.log
```

## Monitoring Commands

### Real-time Monitoring
```bash
# Watch metrics
watch -n 1 'curl -s http://localhost:8080/metrics | grep -E "trades_total|daily_pnl|errors_total"'

# Follow logs
tail -f logs/bitunix-bot.log | grep -E "ERROR|WARN|order"

# System resources
htop -p $(pgrep bitrader)
```

### Performance Analysis
```bash
# Check goroutines
curl -s http://localhost:8080/debug/pprof/goroutine?debug=1

# Memory stats
curl -s http://localhost:8080/debug/pprof/heap?debug=1

# Execution trace (30 seconds)
curl -s http://localhost:8080/debug/pprof/trace?seconds=30 > trace.out
go tool trace trace.out
```

## Getting Help

1. **Check Logs First**:
   ```bash
   tail -n 100 logs/bitunix-bot.log | grep -i error
   ```

2. **Enable Debug Mode**:
   ```bash
   export LOG_LEVEL=debug
   ```

3. **Collect Diagnostics**:
   ```bash
   # Create diagnostic bundle
   mkdir -p diagnostics
   cp logs/*.log diagnostics/
   curl -s http://localhost:8080/metrics > diagnostics/metrics.txt
   go version > diagnostics/version.txt
   cp config.yaml diagnostics/
   tar -czf diagnostics-$(date +%Y%m%d-%H%M%S).tar.gz diagnostics/
   ```

4. **Common Log Patterns**:
   - `"ML model unavailable"` - ML model not loaded
   - `"order placement failed"` - API or permission issue  
   - `"websocket disconnected"` - Network or API issue
   - `"context deadline exceeded"` - Timeout issue
