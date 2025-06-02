# Bitunix O| **Language**      | Go 1.24 (single static binary < 10 MB)                               |IR‑X Trading Bot

*A lightweight, production‑ready crypto futures bot written in Go. Trades BTCUSDT / ETHUSDT (or any Bitunix perp symbol) using an adaptive micro‑structure mean‑reversion strategy.*

---

## ✨ Features

| Capability        | Details                                                              |
| ----------------- | -------------------------------------------------------------------- |
| **Exchange**      | Bitunix Perpetual Futures (`/api/v1/futures`)                        |
| **Language**      | Go 1.22 (single static binary < 10 MB)                               |
| **Data Feeds**    | Public WebSocket – tick trades & depth, optional ONNX model gate     |
| **Strategy**      | **OVIR‑X** → Open‑interest‑proxy, VWAP deviation, Imbalance Reversal |
| **Execution**     | REST `place_order` (MARKET) with volatility‑weighted sizing          |
| **Persistence**   | BoltDB state file, crash‑safe auto‑resume                            |
| **Observability** | Zerolog structured logs, optional Prometheus metrics `/metrics`      |
| **Deployment**    | Run via `go run`, Docker, or systemd unit                            |
| **Security**      | Environment-based secrets, encrypted storage, secure deployment      |

---

## 🔍 Strategy in 60 seconds

> **OVIR‑X (Open‑interest‑Volume‑Imbalance Reversal‑eXtended)**
>
> A short‑term mean‑reversion scalp that detects *over‑leveraged fake breakouts* and fades them.

1. **Price bands** Compute real‑time **VWAP** and rolling **σ** over a 30 s window.
2. **Over‑extension** Score the distance `(price − vwap) / σ` (|z| ≥ 2 triggers attention).
3. **Micro‑structure hints**  

   * **Tick‑imbalance** (buyer vs seller aggressor ratio, 50 ticks).
   * **Depth‑imbalance** `(ΣBid − ΣAsk)/(ΣBid + ΣAsk)` from last order‑book snapshot.
4. **Composite score** `score = α·tick + β·depth + γ·|z|`
5. **ML gate (optional)** ONNX Gradient Boost model (features: tick, depth, z) must predict **“reversal” > 65 %**.
6. **Action** Place a **MARKET** order opposite the spike with size `baseQty / (1 + |z|)` and tight internal TP/SL (user‑configurable).

Why it works:

* Liquidations & breakout traps leave **long wicks** + **volume bursts** + **depth empties**.
* Fading the exhaustion often yields a quick 0.1 – 0.3 % mean move while capping risk at < 0.1 %.

---

## 📂 Repository Layout

```
bitunix-bot/
├── cmd/bitrader/          # main.go (starts WS, features, executor)
├── internal/
│   ├── cfg/               # env loader
│   ├── exchange/bitunix/  # REST + WS client & signer
│   ├── features/          # VWAP, tick/depth imbalance
│   ├── ml/                # ONNX predictor (optional)
│   └── exec/              # order sizing & placement
├── deploy/
│   ├── Dockerfile         # multi‑stage Alpine build
│   └── bitunix-bot.service# systemd unit
├── .env.example           # copy → .env and fill keys
└── README.md              # document of project
```

---

## 🚀 Quick Start

```bash
# Prerequisites (Ubuntu 24.04)
sudo snap install go --classic
sudo apt install -y docker.io

# Clone & configure
mkdir -p ~/projects && cd ~/projects
git clone https://github.com/<you>/bitunix-bot.git
cd bitunix-bot && cp .env.example .env
edit .env   # add BITUNIX_API_KEY + BITUNIX_SECRET_KEY

# Run (foreground)
go run ./cmd/bitrader
```

### Docker

```bash
docker build -t bitunix-bot ./deploy
docker run --env-file .env -v ~/bitunix-data:/srv/data -d --restart unless-stopped bitunix-bot
```

**Security features:**
- Runs as non-root user (65532:65532)
- Includes CA certificates for secure outbound connections
- Uses minimal Alpine base image
- Binary is stripped of debug info with `-buildvcs=false`

### systemd

```bash
sudo install -Dm755 $(go env GOPATH)/bin/bitrader /usr/local/bin/
sudo install -Dm600 .env /etc/bitrader.env
sudo install -Dm644 deploy/bitunix-bot.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now bitunix-bot@<youruser>.service
```

---

## ⚙️ Configuration (`.env`)

| Var                  | Example                    | Description                                    |
| -------------------- | -------------------------- | ---------------------------------------------- |
| `BITUNIX_API_KEY`    | `ak_xxx`                   | Your Bitunix futures API key (trading enabled) |
| `BITUNIX_SECRET_KEY` | `sk_xxx`                   | Secret key for signing REST calls              |
| `SYMBOLS`            | `BTCUSDT,ETHUSDT`          | Comma‑list of perp symbols to trade            |
| `DATA_PATH`          | `/home/alice/bitunix-data` | Folder for BoltDB and logs                     |

---

## 🛡️ Risk Controls

* **Vol‑scaled sizing** — position size ↓ when volatility ↑
* **Hard stop‑loss** inside executor (customizable)
* **Daily loss cap** easy to add by reading BoltDB PnL stats
* Logs stored outside repo in `DATA_PATH` (avoids filling `/`)


