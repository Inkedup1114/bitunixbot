// Package exec provides the core trading execution engine for the Bitunix bot.
// It handles order placement, risk management, position tracking, circuit breakers,
// and ML-based trading decisions with comprehensive safety mechanisms.
//
// The package integrates all components including feature calculation, ML predictions,
// risk management, and exchange communication to execute trading strategies safely.
package exec

import (
	"fmt"
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

const (
	// Order sides for trading operations
	SideBuy  = "BUY"  // Buy order side
	SideSell = "SELL" // Sell order side
)

// Strategy defines the interface for trading strategies.
// Implementations should provide execution logic and strategy identification.
type Strategy interface {
	// Execute processes market data and executes trades based on strategy logic
	Execute(symbol string, price, vwap, std, tick, depth float64, bidVol, askVol float64) error
	// GetName returns the human-readable name of the strategy
	GetName() string
}

// OVIRXStrategy implements the original OVIR-X strategy with ML predictions.
// It uses machine learning models to make trading decisions based on market
// microstructure features including tick imbalance, depth imbalance, and price distance.
type OVIRXStrategy struct {
	exec *Exec // Reference to the execution engine
}

// GetName returns the strategy name for identification and logging.
func (s *OVIRXStrategy) GetName() string {
	return "OVIR-X"
}

func (s *OVIRXStrategy) Execute(symbol string, price, vwap, std, tick, depth float64, bidVol, askVol float64) error {
	// Calculate position size first to check exposure limits
	if std == 0 {
		return nil
	}
	dist := (price - vwap) / std
	f := []float32{float32(tick), float32(depth), float32(dist)}

	if !s.exec.predictor.Approve(f, 0.65) {
		return nil
	}

	side := SideBuy
	if dist > 0 {
		side = SideSell
	}

	// Calculate position size
	positionSize := s.exec.Size(symbol, price)

	// Apply position direction
	signedPositionSize := positionSize
	if side == SideSell {
		signedPositionSize = -positionSize
	}

	// Check if trading is allowed based on risk limits including position exposure
	if !s.exec.CanTradeSymbol(symbol, signedPositionSize, price) {
		log.Debug().
			Str("symbol", symbol).
			Float64("proposed_size", signedPositionSize).
			Float64("price", price).
			Msg("Trading suspended due to risk limits")
		return nil
	}

	// Store features for ML training if storage is available
	if s.exec.store != nil {
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
		if err := s.exec.store.StoreFeatures(featureRecord); err != nil {
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
		if err := s.exec.store.StorePrice(priceRecord); err != nil {
			log.Warn().Err(err).Msg("failed to store price record")
		}
	}

	// Calculate stop-loss and take-profit levels
	stopDistance := std * 1.5 // 1.5 standard deviations for stop-loss
	var stopLoss, takeProfit float64
	if side == SideBuy {
		stopLoss = price - stopDistance
		takeProfit = vwap // Target VWAP for mean reversion
	} else {
		stopLoss = price + stopDistance
		takeProfit = vwap
	}

	// Calculate trailing stop parameters
	trailingDistance := std * 1.0 // 1 standard deviation for trailing stop
	var trailingStop *TrailingStop
	if side == SideBuy {
		trailingStop = &TrailingStop{
			InitialPrice: price,
			StopPrice:    price - trailingDistance,
			Distance:     trailingDistance,
			Side:         side,
			LastUpdate:   time.Now(),
		}
	} else {
		trailingStop = &TrailingStop{
			InitialPrice: price,
			StopPrice:    price + trailingDistance,
			Distance:     trailingDistance,
			Side:         side,
			LastUpdate:   time.Now(),
		}
	}

	// Place main order
	req := bitunix.OrderReq{
		Symbol:    symbol,
		Side:      side,
		TradeSide: "OPEN",
		Qty:       strconv.FormatFloat(positionSize, 'f', -1, 64),
		OrderType: "MARKET",
	}

	// Log trading action for audit
	if s.exec.securityManager != nil {
		s.exec.securityManager.LogTradingAction(
			"order_placement",
			"system",          // userIP
			"OVIR-X Strategy", // userAgent
			TradingAuditData{
				Symbol:    symbol,
				Side:      side,
				Quantity:  req.Qty,
				Price:     price,
				OrderType: req.OrderType,
				Balance:   s.exec.currentBalance,
				PnL:       s.exec.dailyPnL,
			},
			true, // Will be updated if order fails
			"",
		)
	}

	if err := s.exec.rest.PlaceWithTimeout(req); err != nil {
		// Log failed order for audit
		if s.exec.securityManager != nil {
			s.exec.securityManager.LogTradingAction(
				"order_placement_failed",
				"system",
				"OVIR-X Strategy",
				TradingAuditData{
					Symbol:    symbol,
					Side:      side,
					Quantity:  req.Qty,
					Price:     price,
					OrderType: req.OrderType,
					Balance:   s.exec.currentBalance,
					PnL:       s.exec.dailyPnL,
				},
				false,
				err.Error(),
			)
		}
		return fmt.Errorf("order failed: %w", err)
	}

	// Place stop-loss order
	slReq := bitunix.OrderReq{
		Symbol: symbol,
		Side: func() string {
			if side == SideBuy {
				return SideSell
			} else {
				return SideBuy
			}
		}(),
		TradeSide: "CLOSE",
		Qty:       strconv.FormatFloat(positionSize, 'f', -1, 64),
		OrderType: "STOP_LOSS",
		StopPrice: strconv.FormatFloat(stopLoss, 'f', -1, 64),
	}

	if err := s.exec.rest.PlaceWithTimeout(slReq); err != nil {
		log.Warn().Err(err).Msg("stop-loss order failed")
	}

	// Place take-profit order
	tpReq := bitunix.OrderReq{
		Symbol: symbol,
		Side: func() string {
			if side == SideBuy {
				return SideSell
			} else {
				return SideBuy
			}
		}(),
		TradeSide: "CLOSE",
		Qty:       strconv.FormatFloat(positionSize, 'f', -1, 64),
		OrderType: "TAKE_PROFIT",
		StopPrice: strconv.FormatFloat(takeProfit, 'f', -1, 64),
	}

	if err := s.exec.rest.PlaceWithTimeout(tpReq); err != nil {
		log.Warn().Err(err).Msg("take-profit order failed")
	}

	// Store stop-loss, take-profit and trailing stop levels
	s.exec.mu.Lock()
	s.exec.stopLosses[symbol] = stopLoss
	s.exec.takeProfits[symbol] = takeProfit
	s.exec.trailingStops[symbol] = trailingStop
	s.exec.mu.Unlock()

	// Update metrics if available
	if s.exec.metrics != nil {
		s.exec.metrics.OrdersTotal().Inc()

		// Track position
		s.exec.mu.Lock()
		s.exec.positionSizes[symbol] += signedPositionSize
		positions := make(map[string]float64)
		for k, v := range s.exec.positionSizes {
			positions[k] = v
		}
		s.exec.mu.Unlock()

		s.exec.metrics.UpdatePositions(positions)
	}

	log.Info().
		Str("symbol", symbol).
		Str("side", side).
		Str("qty", req.Qty).
		Float64("price", price).
		Float64("dist", dist).
		Float64("stop_loss", stopLoss).
		Float64("take_profit", takeProfit).
		Float64("trailing_distance", trailingDistance).
		Msg("OVIR-X trade executed with stop-loss, take-profit and trailing stop")

	return nil
}

// MeanReversionStrategy implements a simple mean reversion strategy.
// It trades based on price deviations from VWAP, buying when price is significantly
// below VWAP and selling when significantly above, expecting mean reversion.
type MeanReversionStrategy struct {
	exec *Exec // Reference to the execution engine
}

// GetName returns the strategy name for identification and logging.
func (s *MeanReversionStrategy) GetName() string {
	return "Mean Reversion"
}

func (s *MeanReversionStrategy) Execute(symbol string, price, vwap, std, tick, depth float64, bidVol, askVol float64) error {
	if std == 0 {
		return nil
	}

	// Calculate price deviation from VWAP
	dist := (price - vwap) / std

	// Only trade if deviation is significant
	if math.Abs(dist) < 2.0 {
		return nil
	}

	// Mean reversion: buy when price is below VWAP, sell when above
	side := SideBuy
	if dist > 0 {
		side = SideSell
	}

	// Calculate position size
	positionSize := s.exec.Size(symbol, price)

	// Apply position direction
	signedPositionSize := positionSize
	if side == SideSell {
		signedPositionSize = -positionSize
	}

	// Check if trading is allowed based on risk limits including position exposure
	if !s.exec.CanTradeSymbol(symbol, signedPositionSize, price) {
		log.Debug().
			Str("symbol", symbol).
			Float64("proposed_size", signedPositionSize).
			Float64("price", price).
			Msg("Trading suspended due to risk limits")
		return nil
	}

	// Place order
	req := bitunix.OrderReq{
		Symbol:    symbol,
		Side:      side,
		TradeSide: "OPEN",
		Qty:       strconv.FormatFloat(positionSize, 'f', -1, 64),
		OrderType: "MARKET",
	}

	if err := s.exec.rest.PlaceWithTimeout(req); err != nil {
		return fmt.Errorf("order failed: %w", err)
	}

	// Update metrics
	if s.exec.metrics != nil {
		s.exec.metrics.OrdersTotal().Inc()

		s.exec.mu.Lock()
		s.exec.positionSizes[symbol] += signedPositionSize
		positions := make(map[string]float64)
		for k, v := range s.exec.positionSizes {
			positions[k] = v
		}
		s.exec.mu.Unlock()

		s.exec.metrics.UpdatePositions(positions)
	}

	log.Info().
		Str("symbol", symbol).
		Str("side", side).
		Str("qty", req.Qty).
		Float64("price", price).
		Float64("dist", dist).
		Msg("Mean reversion trade executed")

	return nil
}

// TrailingStop represents a trailing stop order configuration.
// It tracks the stop price that moves with favorable price movements
// to lock in profits while limiting losses.
type TrailingStop struct {
	InitialPrice float64   // Initial price when trailing stop was set
	StopPrice    float64   // Current stop price level
	Distance     float64   // Distance from current price to maintain
	Side         string    // Order side (BUY/SELL)
	LastUpdate   time.Time // Last time the stop was updated
}

// OrderReq represents an order request structure.
// This is a local copy of the exchange order request for internal use.
type OrderReq struct {
	Symbol    string `json:"symbol"`              // Trading symbol
	Side      string `json:"side"`                // Order side: BUY or SELL
	TradeSide string `json:"tradeSide"`           // Trade side: OPEN or CLOSE
	Qty       string `json:"qty"`                 // Order quantity
	OrderType string `json:"orderType"`           // Order type: MARKET, STOP_LOSS, TAKE_PROFIT
	StopPrice string `json:"stopPrice,omitempty"` // Stop price for stop orders
}

// CircuitBreakerState tracks the state of circuit breakers for risk management.
// It monitors market conditions and system health to automatically suspend
// trading when predefined thresholds are exceeded.
type CircuitBreakerState struct {
	mu sync.RWMutex // Mutex for thread-safe access

	// Market condition thresholds
	volatilityThreshold float64 // Maximum allowed volatility
	imbalanceThreshold  float64 // Maximum allowed order book imbalance
	volumeThreshold     float64 // Maximum allowed volume spike
	errorRateThreshold  float64 // Maximum allowed error rate

	// Current breaker states
	volatilityBreaker bool // Whether volatility breaker is active
	imbalanceBreaker  bool // Whether imbalance breaker is active
	volumeBreaker     bool // Whether volume breaker is active
	errorRateBreaker  bool // Whether error rate breaker is active

	// Recovery tracking
	lastTriggered time.Time     // Last time any breaker was triggered
	recoveryTime  time.Duration // Time to wait before resetting breakers
}

// NewCircuitBreakerState creates a new circuit breaker state with configuration.
// It initializes all thresholds and recovery settings from the provided config.
func NewCircuitBreakerState(config cfg.Settings) *CircuitBreakerState {
	return &CircuitBreakerState{
		volatilityThreshold: config.CircuitBreakerVolatility,
		imbalanceThreshold:  config.CircuitBreakerImbalance,
		volumeThreshold:     config.CircuitBreakerVolume,
		errorRateThreshold:  config.CircuitBreakerErrorRate,
		recoveryTime:        config.CircuitBreakerRecoveryTime,
	}
}

// UpdateMarketConditions updates circuit breaker state based on market conditions
func (cb *CircuitBreakerState) UpdateMarketConditions(stdDev, imbalance, volume float64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check volatility
	if stdDev > cb.volatilityThreshold {
		cb.volatilityBreaker = true
		cb.lastTriggered = time.Now()
	} else if time.Since(cb.lastTriggered) > cb.recoveryTime {
		cb.volatilityBreaker = false
	}

	// Check order book imbalance
	if math.Abs(imbalance) > cb.imbalanceThreshold {
		cb.imbalanceBreaker = true
		cb.lastTriggered = time.Now()
	} else if time.Since(cb.lastTriggered) > cb.recoveryTime {
		cb.imbalanceBreaker = false
	}

	// Check volume
	if volume > cb.volumeThreshold {
		cb.volumeBreaker = true
		cb.lastTriggered = time.Now()
	} else if time.Since(cb.lastTriggered) > cb.recoveryTime {
		cb.volumeBreaker = false
	}
}

// UpdateErrorRate updates circuit breaker state based on error rate
func (cb *CircuitBreakerState) UpdateErrorRate(errorRate float64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if errorRate > cb.errorRateThreshold {
		cb.errorRateBreaker = true
		cb.lastTriggered = time.Now()
	} else if time.Since(cb.lastTriggered) > cb.recoveryTime {
		cb.errorRateBreaker = false
	}
}

// IsTripped checks if any circuit breaker is tripped
func (cb *CircuitBreakerState) IsTripped() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.volatilityBreaker || cb.imbalanceBreaker || cb.volumeBreaker || cb.errorRateBreaker
}

// GetStatus returns the current status of all circuit breakers
func (cb *CircuitBreakerState) GetStatus() map[string]bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return map[string]bool{
		"volatility": cb.volatilityBreaker,
		"imbalance":  cb.imbalanceBreaker,
		"volume":     cb.volumeBreaker,
		"error_rate": cb.errorRateBreaker,
	}
}

// SecurityManager interface for audit logging (to avoid circular imports)
type SecurityManager interface {
	LogTradingAction(eventType, userIP, userAgent string, tradingData TradingAuditData, success bool, errorMsg string)
}

// TradingAuditData contains specific trading-related audit information
type TradingAuditData struct {
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"`
	Quantity  string  `json:"quantity"`
	Price     float64 `json:"price,omitempty"`
	OrderType string  `json:"orderType"`
	OrderID   string  `json:"orderID,omitempty"`
	Balance   float64 `json:"balance,omitempty"`
	PnL       float64 `json:"pnL,omitempty"`
}

// Exec represents the core trading execution engine.
// It coordinates all trading operations including strategy execution, risk management,
// position tracking, and order placement with comprehensive safety mechanisms.
type Exec struct {
	rest            *bitunix.Client          // Exchange REST client for order placement
	predictor       ml.PredictorInterface    // ML predictor for trading decisions
	config          cfg.Settings             // Configuration settings
	dailyPnL        float64                  // Daily profit and loss tracking
	initialBalance  float64                  // Initial balance at start of trading day
	dayStartTime    time.Time                // When the trading day started
	positionSizes   map[string]float64       // Current position sizes per symbol
	metrics         *metrics.MetricsWrapper  // Metrics collection and reporting
	store           *storage.Store           // Storage for ML feature collection
	mu              sync.RWMutex             // Mutex for thread-safe access
	stopLosses      map[string]float64       // Stop-loss levels per symbol
	takeProfits     map[string]float64       // Take-profit levels per symbol
	trailingStops   map[string]*TrailingStop // Trailing stop configurations per symbol
	strategies      map[string]Strategy      // Registered trading strategies
	circuitBreaker  *CircuitBreakerState     // Circuit breaker for risk management
	securityManager SecurityManager          // Security manager for audit logging

	// Maximum drawdown protection
	peakBalance    float64 // Highest balance achieved since start
	currentBalance float64 // Current balance (initialBalance + total PnL)
}

// New creates a new trading execution engine with the provided configuration.
// It initializes all components including REST client, strategies, circuit breakers,
// and risk management systems. Returns a ready-to-use execution engine.
func New(c cfg.Settings, p ml.PredictorInterface, m *metrics.MetricsWrapper) *Exec {
	exec := &Exec{
		rest:           bitunix.NewRESTWithOrderTrackingAndMetrics(c.Key, c.Secret, c.BaseURL, c.RESTTimeout, c.OrderExecutionTimeout, c.OrderStatusCheckInterval, c.MaxOrderRetries, m),
		predictor:      p,
		config:         c,
		positionSizes:  make(map[string]float64),
		metrics:        m,
		store:          nil, // ML feature collection will be optional
		stopLosses:     make(map[string]float64),
		takeProfits:    make(map[string]float64),
		trailingStops:  make(map[string]*TrailingStop),
		strategies:     make(map[string]Strategy),
		initialBalance: c.InitialBalance, // Use configured initial balance
		dayStartTime:   time.Now(),       // Set current time as day start
		circuitBreaker: NewCircuitBreakerState(c),
		// Initialize drawdown protection tracking
		peakBalance:    c.InitialBalance,
		currentBalance: c.InitialBalance,
	}

	// Register default strategies
	exec.RegisterStrategy(&OVIRXStrategy{exec: exec})
	exec.RegisterStrategy(&MeanReversionStrategy{exec: exec})

	return exec
}

// RegisterStrategy registers a new trading strategy
func (e *Exec) RegisterStrategy(s Strategy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.strategies[s.GetName()] = s
}

// SetSecurityManager sets the security manager for audit logging
func (e *Exec) SetSecurityManager(sm SecurityManager) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.securityManager = sm
}

// Try executes all registered strategies
func (e *Exec) Try(symbol string, price, vwap, std, tick, depth float64, bidVol, askVol float64) {
	// Update circuit breaker with market conditions
	e.circuitBreaker.UpdateMarketConditions(std, depth, bidVol+askVol)

	e.mu.RLock()
	strategies := make([]Strategy, 0, len(e.strategies))
	for _, s := range e.strategies {
		strategies = append(strategies, s)
	}
	e.mu.RUnlock()

	for _, strategy := range strategies {
		if err := strategy.Execute(symbol, price, vwap, std, tick, depth, bidVol, askVol); err != nil {
			log.Warn().Err(err).
				Str("strategy", strategy.GetName()).
				Str("symbol", symbol).
				Msg("Strategy execution failed")

			// Update error rate in circuit breaker
			if e.metrics != nil {
				errorRate := e.metrics.GetErrorRate()
				e.circuitBreaker.UpdateErrorRate(errorRate)
			}
		}
	}
}

// SetStorage sets the storage instance for ML feature collection
func (e *Exec) SetStorage(s *storage.Store) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.store = s
}

// Size calculates position size based on Kelly Criterion
func (e *Exec) Size(symbol string, price float64) float64 {
	// Get historical win rate and average win/loss ratios
	winRate := e.getHistoricalWinRate(symbol)
	avgWin := e.getAverageWin(symbol)
	avgLoss := e.getAverageLoss(symbol)

	// Calculate Kelly Criterion
	kelly := e.calculateKelly(winRate, avgWin, avgLoss)

	// Apply safety factor (half Kelly)
	kelly *= 0.5

	// Calculate position size based on Kelly and account risk
	accountRisk := e.config.RiskUSD * kelly
	qty := (accountRisk * float64(e.config.Leverage)) / price

	// Apply position limits
	maxPositionValue := e.config.MaxPositionSize * e.config.RiskUSD
	if qty*price > maxPositionValue {
		qty = maxPositionValue / price
	}

	return RoundStep(qty, lotSize(symbol))
}

// getHistoricalWinRate returns the historical win rate for a symbol
func (e *Exec) getHistoricalWinRate(symbol string) float64 {
	// TODO: Implement actual historical win rate calculation
	// For now, return a conservative default
	return 0.55
}

// getAverageWin returns the average win ratio for a symbol
func (e *Exec) getAverageWin(symbol string) float64 {
	// TODO: Implement actual average win calculation
	// For now, return a conservative default
	return 1.5
}

// getAverageLoss returns the average loss ratio for a symbol
func (e *Exec) getAverageLoss(symbol string) float64 {
	// TODO: Implement actual average loss calculation
	// For now, return a conservative default
	return 1.0
}

// calculateKelly calculates the Kelly Criterion
func (e *Exec) calculateKelly(winRate, avgWin, avgLoss float64) float64 {
	if avgLoss == 0 {
		return 0
	}

	// Kelly Criterion formula: f* = (p(b+1)-1)/b
	// where p is win rate, b is win/loss ratio
	b := avgWin / avgLoss
	kelly := (winRate*(b+1) - 1) / b

	// Ensure Kelly is between 0 and 1
	if kelly < 0 {
		return 0
	}
	if kelly > 1 {
		return 1
	}
	return kelly
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

// GetPositionExposure returns the current exposure for a symbol in USD
func (e *Exec) GetPositionExposure(symbol string, price float64) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	position := e.positionSizes[symbol]
	return math.Abs(position) * price
}

// GetMaxAllowedExposure returns the maximum allowed exposure for a symbol in USD
func (e *Exec) GetMaxAllowedExposure(symbol string) float64 {
	symbolConfig := e.config.GetSymbolConfig(symbol)
	return e.initialBalance * symbolConfig.MaxPositionExposure
}

// UpdatePnL updates the daily P&L tracking
func (e *Exec) UpdatePnL(pnl float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.dailyPnL += pnl
	// Update current balance for drawdown tracking
	e.currentBalance = e.initialBalance + e.dailyPnL

	// Update peak balance if current balance is higher
	if e.currentBalance > e.peakBalance {
		e.peakBalance = e.currentBalance
	}

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

// GetCurrentBalance returns the current account balance
func (e *Exec) GetCurrentBalance() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentBalance
}

// GetPeakBalance returns the peak balance achieved
func (e *Exec) GetPeakBalance() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.peakBalance
}

// GetCurrentDrawdown returns the current drawdown percentage from peak
func (e *Exec) GetCurrentDrawdown() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.peakBalance <= 0 {
		return 0
	}

	return (e.peakBalance - e.currentBalance) / e.peakBalance
}

// GetInitialBalance returns the initial balance
func (e *Exec) GetInitialBalance() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.initialBalance
}

// GetMaxDrawdownProtection returns the maximum drawdown protection limit
func (e *Exec) GetMaxDrawdownProtection() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.MaxDrawdownProtection
}

// GetDailyLossLimit returns the daily loss limit
func (e *Exec) GetDailyLossLimit() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.MaxDailyLoss
}

// GetMaxPositionExposure returns the maximum position exposure limit
func (e *Exec) GetMaxPositionExposure() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.MaxPositionExposure
}

// GetCircuitBreakerStatus returns the current circuit breaker status
func (e *Exec) GetCircuitBreakerStatus() map[string]bool {
	return e.circuitBreaker.GetStatus()
}

// UpdateTrailingStop updates the trailing stop for a position
func (e *Exec) UpdateTrailingStop(symbol string, currentPrice float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ts, exists := e.trailingStops[symbol]
	if !exists {
		return
	}

	// Update trailing stop based on price movement
	if ts.Side == SideBuy {
		// For long positions, move stop up if price increases
		if currentPrice > ts.InitialPrice {
			newStop := currentPrice - ts.Distance
			if newStop > ts.StopPrice {
				ts.StopPrice = newStop
				ts.LastUpdate = time.Now()
			}
		}
	} else {
		// For short positions, move stop down if price decreases
		if currentPrice < ts.InitialPrice {
			newStop := currentPrice + ts.Distance
			if newStop < ts.StopPrice {
				ts.StopPrice = newStop
				ts.LastUpdate = time.Now()
			}
		}
	}

	// Check if stop is hit
	if (ts.Side == SideBuy && currentPrice <= ts.StopPrice) ||
		(ts.Side == SideSell && currentPrice >= ts.StopPrice) {
		// Close position
		req := bitunix.OrderReq{
			Symbol: symbol,
			Side: func() string {
				if ts.Side == SideBuy {
					return SideSell
				} else {
					return SideBuy
				}
			}(),
			TradeSide: "CLOSE",
			Qty:       strconv.FormatFloat(e.positionSizes[symbol], 'f', -1, 64),
			OrderType: "MARKET",
		}

		if err := e.rest.PlaceWithTimeout(req); err != nil {
			log.Warn().Err(err).Msg("trailing stop order failed")
			return
		}

		// Clear position tracking
		delete(e.positionSizes, symbol)
		delete(e.trailingStops, symbol)
		delete(e.stopLosses, symbol)
		delete(e.takeProfits, symbol)

		log.Info().
			Str("symbol", symbol).
			Str("side", ts.Side).
			Float64("exit_price", currentPrice).
			Float64("stop_price", ts.StopPrice).
			Msg("Position closed by trailing stop")
	}
}

// IsNewTradingDay checks if we need to reset daily tracking
func (e *Exec) IsNewTradingDay() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	now := time.Now()
	// Consider it a new trading day if:
	// 1. It's a different calendar day (UTC)
	// 2. Or it's been more than 24 hours since day start
	return now.Day() != e.dayStartTime.Day() ||
		now.Sub(e.dayStartTime) > 24*time.Hour
}

// ResetDailyTracking resets daily P&L tracking for a new trading day
func (e *Exec) ResetDailyTracking() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.dailyPnL = 0
	e.dayStartTime = time.Now()
	// Reset drawdown tracking for new trading day
	e.currentBalance = e.initialBalance
	e.peakBalance = e.initialBalance

	if e.metrics != nil {
		e.metrics.PnLTotal().Set(0)
	}

	log.Info().
		Time("new_day_start", e.dayStartTime).
		Float64("initial_balance", e.initialBalance).
		Msg("Daily P&L tracking and drawdown protection reset for new trading day")
}

// CheckDailyLossLimit checks if daily loss limit has been reached
func (e *Exec) CheckDailyLossLimit() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check if it's a new trading day first
	if e.IsNewTradingDay() {
		// Unlock to avoid deadlock when calling ResetDailyTracking
		e.mu.RUnlock()
		e.ResetDailyTracking()
		e.mu.RLock()
	}

	// If MaxDailyLoss is 0 or negative, no limit is enforced
	if e.config.MaxDailyLoss <= 0 {
		return false
	}

	// If we're not in a loss, no limit is reached
	if e.dailyPnL >= 0 {
		return false
	}

	// Calculate loss as percentage of initial balance
	lossPercentage := -e.dailyPnL / e.initialBalance

	// Check if loss exceeds the configured limit
	limitReached := lossPercentage >= e.config.MaxDailyLoss

	if limitReached {
		log.Warn().
			Float64("daily_pnl", e.dailyPnL).
			Float64("loss_percentage", lossPercentage*100).
			Float64("max_daily_loss_percentage", e.config.MaxDailyLoss*100).
			Float64("initial_balance", e.initialBalance).
			Msg("Daily loss limit reached - trading suspended")
	}

	return limitReached
}

// CheckMaxDrawdownProtection checks if maximum drawdown protection has been triggered
func (e *Exec) CheckMaxDrawdownProtection() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// If MaxDrawdownProtection is 0 or negative, no protection is enforced
	if e.config.MaxDrawdownProtection <= 0 {
		return false
	}

	// Calculate current drawdown from peak
	drawdown := (e.peakBalance - e.currentBalance) / e.peakBalance

	// Check if drawdown exceeds the configured limit
	limitReached := drawdown >= e.config.MaxDrawdownProtection

	if limitReached {
		log.Warn().
			Float64("peak_balance", e.peakBalance).
			Float64("current_balance", e.currentBalance).
			Float64("drawdown_percentage", drawdown*100).
			Float64("max_drawdown_percentage", e.config.MaxDrawdownProtection*100).
			Float64("initial_balance", e.initialBalance).
			Msg("Maximum drawdown protection triggered - trading suspended")
	}

	return limitReached
}

// CheckPositionExposureLimit checks if position exposure limit has been reached for a symbol
func (e *Exec) CheckPositionExposureLimit(symbol string, proposedSize float64, price float64) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Get symbol-specific config
	symbolConfig := e.config.GetSymbolConfig(symbol)
	maxExposure := symbolConfig.MaxPositionExposure

	// If MaxPositionExposure is 0 or negative, no limit is enforced
	if maxExposure <= 0 {
		return false
	}

	// Calculate current exposure for this symbol (absolute value of all positions)
	currentPosition := e.positionSizes[symbol]
	currentExposure := math.Abs(currentPosition) * price

	// Calculate proposed new position after trade
	newPosition := currentPosition + proposedSize
	newExposure := math.Abs(newPosition) * price

	// Calculate maximum allowed exposure based on initial balance
	maxAllowedExposure := e.initialBalance * maxExposure

	// Check if proposed exposure exceeds limit
	limitReached := newExposure > maxAllowedExposure

	if limitReached {
		log.Warn().
			Str("symbol", symbol).
			Float64("current_position", currentPosition).
			Float64("current_exposure", currentExposure).
			Float64("proposed_position", newPosition).
			Float64("proposed_exposure", newExposure).
			Float64("max_allowed_exposure", maxAllowedExposure).
			Float64("max_exposure_percentage", maxExposure*100).
			Float64("initial_balance", e.initialBalance).
			Msg("Position exposure limit reached - trade rejected")
	}

	return limitReached
}

// CanTrade checks if trading is allowed based on risk limits
func (e *Exec) CanTrade() bool {
	// Check daily loss limit
	if e.CheckDailyLossLimit() {
		return false
	}

	// Check maximum drawdown protection
	if e.CheckMaxDrawdownProtection() {
		return false
	}

	// Check circuit breaker
	if e.circuitBreaker.IsTripped() {
		log.Warn().
			Interface("circuit_breaker_status", e.circuitBreaker.GetStatus()).
			Msg("Trading suspended due to circuit breaker")
		return false
	}

	return true
}

// CanTradeSymbol checks if trading is allowed for a specific symbol and trade size
func (e *Exec) CanTradeSymbol(symbol string, proposedSize float64, price float64) bool {
	// Check general trading limits
	if !e.CanTrade() {
		return false
	}

	// Check position exposure limit for this symbol
	if e.CheckPositionExposureLimit(symbol, proposedSize, price) {
		return false
	}

	return true
}

// Close gracefully shuts down the executor and cleans up resources
func (e *Exec) Close() {
	// Stop the order tracker
	if e.rest != nil {
		e.rest.Close()
	}
}

// GetOrderTracker returns the order tracker for monitoring purposes
func (e *Exec) GetOrderTracker() *bitunix.OrderTracker {
	if e.rest != nil {
		return e.rest.GetOrderTracker()
	}
	return nil
}
