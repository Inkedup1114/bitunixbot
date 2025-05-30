package exec

import (
"bitunix-bot/internal/cfg"
"bitunix-bot/internal/exchange/bitunix"
"bitunix-bot/internal/metrics"
"bitunix-bot/internal/ml"
"fmt"
"math"
"strconv"
"sync"

"github.com/rs/zerolog/log"
)

type Exec struct {
rest          *bitunix.Client
predictor     *ml.Predictor
config        cfg.Settings
dailyPnL      float64
positionSizes map[string]float64
metrics       *metrics.MetricsWrapper
mu            sync.RWMutex
}

func New(c cfg.Settings, p *ml.Predictor, m *metrics.MetricsWrapper) *Exec {
return &Exec{
rest:          bitunix.NewREST(c.Key, c.Secret, c.BaseURL, c.RESTTimeout),
predictor:     p,
config:        c,
positionSizes: make(map[string]float64),
metrics:       m,
}
}

func (e *Exec) Size(symbol string, zDist float64) string {
sc := e.config.GetSymbolConfig(symbol)
base := sc.BaseSizeRatio
if base == 0 {
base = e.config.BaseSizeRatio
}
if base == 0 {
base = 0.002
}
q := base / (1 + math.Abs(zDist))
return fmt.Sprintf("%.4f", q)
}

func (e *Exec) Try(symbol string, price, vwap, std, tick, depth float64) {
if std == 0 {
return
}
dist := (price - vwap) / std
f := []float32{float32(tick), float32(depth), float32(dist)}

if !e.predictor.Approve(f, 0.65) {
return
}

side := "BUY"
if dist > 0 {
side = "SELL"
}

req := bitunix.OrderReq{
Symbol:    symbol,
Side:      side,
TradeSide: "OPEN",
Qty:       e.Size(symbol, dist),
OrderType: "MARKET",
}

if err := e.rest.Place(req); err != nil {
log.Warn().Err(err).Msg("order failed")
return
}

if e.metrics != nil {
e.metrics.OrdersTotal().Inc()

sizeFloat, _ := strconv.ParseFloat(req.Qty, 64)
if side == "SELL" {
sizeFloat = -sizeFloat
}

e.mu.Lock()
e.positionSizes[symbol] += sizeFloat
positions := make(map[string]float64)
for k, v := range e.positionSizes {
positions[k] = v
}
e.mu.Unlock()

e.metrics.UpdatePositions(positions)
}

log.Info().
Str("symbol", symbol).
Str("side", side).
Str("qty", req.Qty).
Float64("price", price).
Float64("dist", dist).
Msg("OVIR-X trade executed")
}

func (e *Exec) GetPositions() map[string]float64 {
e.mu.RLock()
defer e.mu.RUnlock()

positions := make(map[string]float64)
for k, v := range e.positionSizes {
positions[k] = v
}
return positions
}

func (e *Exec) UpdatePnL(pnl float64) {
e.mu.Lock()
defer e.mu.Unlock()

e.dailyPnL += pnl

if e.metrics != nil {
e.metrics.PnLTotal().Set(e.dailyPnL)
}
}

func (e *Exec) GetDailyPnL() float64 {
e.mu.RLock()
defer e.mu.RUnlock()
return e.dailyPnL
}
