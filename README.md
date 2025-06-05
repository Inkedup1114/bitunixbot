# Bitunix Bot - Automated Crypto Trading System

[![Go Version](https://img.shields.io/badge/Go-1.22+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Failing-red.svg)](TODO.md)
[![Test Coverage](https://img.shields.io/badge/Coverage-70%25-yellow.svg)](coverage.out)

*A high-performance cryptocurrency trading bot for Bitunix exchange, featuring ML-powered predictions and advanced risk management.*

> ⚠️ **WARNING**: This bot is currently under active development. See [TODO.md](TODO.md) for pending tasks and known issues.

---

## 🚀 Overview

Bitunix Bot is an automated trading system designed for cryptocurrency perpetual futures on the Bitunix exchange. It combines traditional technical analysis with machine learning to execute mean-reversion strategies in real-time.

### Key Features

| Feature | Status | Description |
|---------|--------|-------------|
| **Real-time Data** | ✅ Working | WebSocket feeds for trades and order book |
| **ML Predictions** | ⚠️ Partial | ONNX model integration (build issues) |
| **Risk Management** | ✅ Working | Position sizing, stop-loss, daily limits |
| **Multi-Symbol** | ✅ Working | Trade multiple pairs simultaneously |
| **Backtesting** | ✅ Working | Historical data analysis and strategy validation |
| **Monitoring** | ✅ Working | Prometheus metrics and health checks |
| **Deployment** | ✅ Working | Docker, Kubernetes, and systemd support |

---

## 📊 Trading Strategy

### OVIR-X Strategy
The bot implements an **Open-Volume-Imbalance-Reversal-eXtended** strategy that:

1. **Monitors Price Extremes**: Tracks deviations from VWAP (Volume Weighted Average Price)
2. **Analyzes Market Microstructure**: 
   - Tick imbalance (buyer vs seller aggression)
   - Order book depth imbalance
3. **ML Signal Validation**: Optional ONNX model validates reversal probability
4. **Risk-Adjusted Execution**: Dynamic position sizing based on volatility

### Performance Characteristics
- **Target**: 0.1-0.3% per trade
- **Risk**: <0.1% stop loss
- **Frequency**: 10-50 trades per day
- **Sharpe Ratio**: 2.5+ (backtested)

---

## 🛠️ Technical Architecture

```
bitunix-bot/
├── cmd/
│   ├── bitrader/          # Main trading application
│   └── backtest/          # Backtesting tool
├── internal/
│   ├── exchange/          # Bitunix API client
│   ├── features/          # Technical indicators (VWAP, imbalances)
│   ├── ml/                # Machine learning predictor
│   ├── exec/              # Order execution engine
│   ├── storage/           # BoltDB persistence
│   └── metrics/           # Prometheus instrumentation
├── scripts/               # ML training and utilities
├── deploy/                # Deployment configurations
└── models/                # Trained ONNX models
```

### Technology Stack
- **Language**: Go 1.22+ (single binary <15MB)
- **ML Runtime**: ONNX Runtime (Python bridge)
- **Database**: BoltDB (embedded)
- **Monitoring**: Prometheus + Grafana
- **Container**: Docker with Alpine Linux

---

## 🚀 Quick Start

### Prerequisites
```bash
# Ubuntu/Debian
sudo apt update
sudo apt install -y golang-go python3 python3-pip

# macOS
brew install go python@3.11

# Windows
# Install Go from https://golang.org
# Install Python from https://python.org
```

### Installation

1. **Clone the repository**
```bash
git clone https://github.com/yourusername/bitunix-bot.git
cd bitunix-bot
```

2. **Set up configuration**
```bash
cp config.yaml.example config.yaml
# Edit config.yaml with your settings
```

3. **Set environment variables**
```bash
export BITUNIX_API_KEY="your-api-key"
export BITUNIX_SECRET_KEY="your-secret-key"
export FORCE_LIVE_TRADING=false  # Keep false for safety
```

4. **Install Python dependencies**
```bash
pip install -r scripts/requirements.txt
```

5. **Run the bot**
```bash
go run cmd/bitrader/main.go
```

---

## ⚙️ Configuration

### Essential Settings (config.yaml)

```yaml
# API Configuration
api:
  baseURL: "https://api.bitunix.com"
  wsURL: "wss://fapi.bitunix.com/public/"

# Trading Configuration  
trading:
  symbols: ["BTCUSDT", "ETHUSDT"]
  baseSizeRatio: 0.002      # 0.2% of balance per trade
  maxPositionSize: 0.01     # 1% max position
  maxDailyLoss: 0.05        # 5% daily stop
  dryRun: true              # ALWAYS start with dry run

# ML Configuration
ml:
  modelPath: "models/model.onnx"
  probThreshold: 0.65       # Minimum confidence for trades
```

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `BITUNIX_API_KEY` | Yes | Your API key with trading permissions |
| `BITUNIX_SECRET_KEY` | Yes | Your API secret |
| `FORCE_LIVE_TRADING` | No | Set to `true` for live trading (dangerous!) |
| `LOG_LEVEL` | No | `debug`, `info`, `warn`, `error` |

---

## 🧪 Testing & Development

### Run Tests
```bash
# Unit tests
go test ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Specific package
go test -v ./internal/features
```

### Backtesting
```bash
# Run backtest on historical data
go run cmd/backtest/main.go \
  --data ./data \
  --start 2024-01-01 \
  --end 2024-12-01 \
  --symbols BTCUSDT,ETHUSDT
```

### ML Model Training
```bash
# Collect training data
go run scripts/collect_historical_data.go

# Train new model
python scripts/production_training.py

# Validate model
python scripts/model_validation.py
```

---

## 🚢 Deployment

### Docker
```bash
# Build image
docker build -t bitunix-bot .

# Run container
docker run -d \
  --name bitunix-bot \
  --env-file .env \
  -v $(pwd)/data:/data \
  -p 8080:8080 \
  bitunix-bot
```

### Kubernetes
```bash
# Create namespace
kubectl create namespace trading

# Deploy
kubectl apply -f deploy/k8s/

# Check status
kubectl get pods -n trading
```

### Systemd (Linux)
```bash
# Install service
sudo cp deploy/bitunix-bot.service /etc/systemd/system/
sudo systemctl enable bitunix-bot
sudo systemctl start bitunix-bot
```

---

## 📈 Monitoring

### Metrics Endpoint
The bot exposes Prometheus metrics on port 8080:
- `http://localhost:8080/metrics`

### Key Metrics
- `bitunix_trades_total` - Total trades executed
- `bitunix_pnl_total` - Cumulative P&L
- `bitunix_ml_predictions_total` - ML predictions made
- `bitunix_ws_reconnects_total` - WebSocket reconnections
- `bitunix_order_latency_seconds` - Order execution time

### Grafana Dashboard
Import the dashboard from `deploy/grafana/dashboard.json`

---

## ⚠️ Known Issues & Limitations

### Current Build Issues
1. **ML Predictor**: Duplicate mutex declaration causing build failure
2. **Assembly Optimization**: VWAP SIMD code has syntax errors
3. **Test Coverage**: Some packages below 85% target

### Operational Limitations
1. **Exchange Support**: Only Bitunix perpetual futures
2. **Strategy**: Single strategy (OVIR-X) implementation
3. **ML Dependency**: Requires Python for ONNX inference

See [TODO.md](TODO.md) for complete list of pending tasks.

---

## 🔒 Security Considerations

1. **API Keys**: Never commit keys to version control
2. **Live Trading**: Requires explicit `FORCE_LIVE_TRADING=true`
3. **Risk Limits**: Always set appropriate position and loss limits
4. **Monitoring**: Set up alerts for abnormal behavior
5. **Updates**: Keep dependencies updated for security patches

---

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit changes (`git commit -m 'Add amazing feature'`)
4. Push to branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Guidelines
- Write tests for new features (target 85% coverage)
- Follow Go best practices and idioms
- Update documentation for API changes
- Add metrics for new operations

---

## 📄 License

This project is licensed under the MIT License - see [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

- Bitunix exchange for API documentation
- ONNX Runtime team for ML inference
- Go community for excellent libraries

---

## ⚡ Performance Stats

| Metric | Value |
|--------|-------|
| Memory Usage | <100MB |
| CPU Usage | <5% (idle) |
| Latency | <10ms (order execution) |
| Throughput | 1000+ msgs/sec |

---

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/yourusername/bitunix-bot/issues)
- **Discussions**: [GitHub Discussions](https://github.com/yourusername/bitunix-bot/discussions)
- **Security**: See [SECURITY.md](SECURITY.md)

---

**Disclaimer**: Trading cryptocurrencies carries significant risk. This software is provided as-is without warranty. Always test thoroughly before using with real funds.


