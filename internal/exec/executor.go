package exec

import (
	"math"
	"strconv"
	"sync"
	"time"

	"bitunix-bot/internal/cfg"
	"bitunix-bot/internal/exchange/bitunix"
	"bitunix-bot/internal/metrics"
	"bitunix-bot/internal/ml"
	"bitunix-bot/internal/storage"

	"github.com/rs/zerolog/log"
)

type Exec struct {
	rest          *bitunix.Client
	predictor     ml.PredictorInterface
	config        cfg.Settings
	dailyPnL      float64
	positionSizes map[string]float64 // Current position sizes per symbol
	metrics       *metrics.MetricsWrapper
	store         *storage.Store // For ML feature collection
	mu            sync.RWMutex
}

func New(c cfg.Settings, p ml.PredictorInterface, m *metrics.MetricsWrapper) *Exec {
	return &Exec{
		rest:          bitunix.NewREST(c.Key, c.Secret, c.BaseURL, c.RESTTimeout),
		predictor:     p,
		config:        c,
		positionSizes: make(map[string]float64),
		metrics:       m,
		store:         nil, // ML feature collection will be optional
	}
}

// SetStorage sets the storage instance for ML feature collection
func (e *Exec) SetStorage(s *storage.Store) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.store = s
}

// Size calculates position size based on volatility and symbol configuration
func (e *Exec) Size(symbol string, price float64) float64 {
	qty := (e.config.RiskUSD * float64(e.config.Leverage)) / price
	return RoundStep(qty, lotSize(symbol))
}

func lotSize(sym string) float64 {
	switch sym {
	case "BTCUSDT":
		return 0.001
	case "ETHUSDT":
		return 0.01
	default:
		return 0.01
	}
}

func RoundStep(qty, step float64) float64 {
	return math.Floor(qty/step) * step
}

// Try implements the original OVIR-X strategy logic
func (e *Exec) Try(symbol string, price, vwap, std, tick, depth float64, bidVol, askVol float64) {
	if std == 0 {
		return
	}
	dist := (price - vwap) / std
	f := []float32{float32(tick), float32(depth), float32(dist)}

	// Store features for ML training if storage is available
	if e.store != nil {
		featureRecord := storage.FeatureRecord{
			Symbol:     symbol,
			Timestamp:  time.Now(),
			TickRatio:  tick,
			DepthRatio: depth,
			PriceDist:  dist,
			Price:      price,
			VWAP:       vwap,
			StdDev:     std,
			BidVol:     bidVol,
			AskVol:     askVol,
		}
		if err := e.store.StoreFeatures(featureRecord); err != nil {
			log.Warn().Err(err).Msg("failed to store feature record")
		}

		// Also store price data for labeling
		priceRecord := storage.PriceRecord{
			Symbol:    symbol,
			Timestamp: time.Now(),
			Price:     price,
			VWAP:      vwap,
			StdDev:    std,
		}
		if err := e.store.StorePrice(priceRecord); err != nil {
			log.Warn().Err(err).Msg("failed to store price record")
		}
	}

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
		Qty:       strconv.FormatFloat(e.Size(symbol, price), 'f', -1, 64),
		OrderType: "MARKET",
	}

	if err := e.rest.Place(req); err != nil {
		log.Warn().Err(err).Msg("order failed")
		return
	}

	// Update metrics if available
	if e.metrics != nil {
		e.metrics.OrdersTotal().Inc()

		// Track position
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

// GetPositions returns the current position sizes
func (e *Exec) GetPositions() map[string]float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	positions := make(map[string]float64)
	for k, v := range e.positionSizes {
		positions[k] = v
	}
	return positions
}

// UpdatePnL updates the daily P&L tracking
func (e *Exec) UpdatePnL(pnl float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.dailyPnL += pnl

	if e.metrics != nil {
		e.metrics.PnLTotal().Set(e.dailyPnL)
	}
}

// GetDailyPnL returns the current daily P&L
func (e *Exec) GetDailyPnL() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.dailyPnL
}
