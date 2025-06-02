package backtest

import (
	"bitunix-bot/internal/cfg"
	"bitunix-bot/internal/features"
	"bitunix-bot/internal/ml"
	"math"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Engine represents the backtesting engine
type Engine struct {
	config         *cfg.Settings
	predictor      ml.PredictorInterface
	data           *DataLoader
	results        *Results
	positions      map[string]*Position
	balance        float64
	initialBalance float64
	mu             sync.RWMutex

	// Feature calculators
	vwapMap   map[string]*features.VWAP
	ticksMap  map[string]*features.TickImb
	lastPrice map[string]float64
}

// Position represents an open position
type Position struct {
	Symbol     string
	Side       string // "long" or "short"
	EntryPrice float64
	Size       float64
	EntryTime  time.Time
	StopLoss   float64
	TakeProfit float64
}

// Trade represents a completed trade
type Trade struct {
	Symbol     string
	Side       string
	EntryPrice float64
	ExitPrice  float64
	Size       float64
	EntryTime  time.Time
	ExitTime   time.Time
	PnL        float64
	PnLPercent float64
	Commission float64
	ExitReason string // "stop_loss", "take_profit", "signal", "end_of_data"
}

// Results holds backtesting results
type Results struct {
	Trades          []Trade
	TotalTrades     int
	WinningTrades   int
	LosingTrades    int
	TotalPnL        float64
	TotalCommission float64
	MaxDrawdown     float64
	SharpeRatio     float64
	WinRate         float64
	ProfitFactor    float64
	StartTime       time.Time
	EndTime         time.Time
	InitialBalance  float64
	FinalBalance    float64
	mu              sync.RWMutex
}

// NewEngine creates a new backtesting engine
func NewEngine(config *cfg.Settings, predictor ml.PredictorInterface, data *DataLoader) *Engine {
	initialBalance := 10000.0 // Default starting balance
	if envBalance := config.InitialBalance; envBalance > 0 {
		initialBalance = envBalance
	}

	e := &Engine{
		config:         config,
		predictor:      predictor,
		data:           data,
		balance:        initialBalance,
		initialBalance: initialBalance,
		positions:      make(map[string]*Position),
		results: &Results{
			Trades:         make([]Trade, 0),
			InitialBalance: initialBalance,
		},
		vwapMap:   make(map[string]*features.VWAP),
		ticksMap:  make(map[string]*features.TickImb),
		lastPrice: make(map[string]float64),
	}

	// Initialize feature calculators for each symbol
	for _, symbol := range config.Symbols {
		e.vwapMap[symbol] = features.NewVWAP(config.VWAPWindow, config.VWAPSize)
		e.ticksMap[symbol] = features.NewTickImb(config.TickSize)
		e.lastPrice[symbol] = 0.0
	}

	return e
}

// Run executes the backtest
func (e *Engine) Run() error {
	log.Info().
		Time("start", e.data.StartTime).
		Time("end", e.data.EndTime).
		Strs("symbols", e.config.Symbols).
		Msg("Starting backtest")

	// Process data chronologically
	for e.data.HasNext() {
		tick := e.data.Next()

		// Update price tracking
		if tick.Type == "trade" {
			e.updatePriceData(tick)
		} else if tick.Type == "depth" {
			e.processDepthUpdate(tick)
		}

		// Check existing positions
		e.checkPositions(tick)
	}

	// Close any remaining positions at end of backtest
	e.closeAllPositions("end_of_data")

	// Calculate final metrics
	e.calculateMetrics()

	return nil
}

// updatePriceData updates price tracking and VWAP
func (e *Engine) updatePriceData(tick DataPoint) {
	e.mu.Lock()
	defer e.mu.Unlock()

	symbol := tick.Symbol
	price := tick.Price
	volume := tick.Volume

	// Update VWAP
	e.vwapMap[symbol].Add(price, volume)

	// Update tick imbalance
	oldPrice := e.lastPrice[symbol]
	if oldPrice > 0 {
		sign := int8(0)
		if price > oldPrice {
			sign = 1
		} else if price < oldPrice {
			sign = -1
		}
		e.ticksMap[symbol].Add(sign)
	}

	e.lastPrice[symbol] = price
}

// processDepthUpdate processes order book updates and generates signals
func (e *Engine) processDepthUpdate(tick DataPoint) {
	e.mu.RLock()
	symbol := tick.Symbol
	price := e.lastPrice[symbol]
	e.mu.RUnlock()

	if price == 0 {
		return // No price data yet
	}

	// Calculate features
	vwap, stdDev := e.vwapMap[symbol].Calc()
	if stdDev == 0 {
		return // Not enough data
	}

	tickRatio := e.ticksMap[symbol].Ratio()
	depthRatio := features.DepthImb(tick.BidVolume, tick.AskVolume)
	priceDist := (price - vwap) / stdDev

	// Prepare features for ML model
	mlFeatures := []float32{
		float32(tickRatio),
		float32(depthRatio),
		float32(priceDist),
	}

	// Get ML prediction
	if e.predictor.Approve(mlFeatures, e.config.ProbThreshold) {
		e.executeSignal(symbol, price, vwap, stdDev, priceDist, tick.Timestamp)
	}
}

// executeSignal executes a trading signal
func (e *Engine) executeSignal(symbol string, price, vwap, stdDev, priceDist float64, timestamp time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check if we already have a position
	if _, exists := e.positions[symbol]; exists {
		return // Already in position
	}

	// Determine trade direction (mean reversion)
	var side string
	if priceDist > 0 {
		side = "short" // Price above VWAP, expect reversion down
	} else {
		side = "long" // Price below VWAP, expect reversion up
	}

	// Calculate position size (risk-based)
	accountRisk := 0.01          // Risk 1% of account per trade
	stopDistance := stdDev * 1.5 // Stop loss at 1.5 standard deviations

	positionSize := (e.balance * accountRisk) / stopDistance

	// Apply position limits
	maxPositionValue := e.balance * e.config.MaxPositionSize
	if positionSize*price > maxPositionValue {
		positionSize = maxPositionValue / price
	}

	// Calculate stop loss and take profit
	var stopLoss, takeProfit float64
	if side == "long" {
		stopLoss = price - stopDistance
		takeProfit = vwap // Target VWAP for mean reversion
	} else {
		stopLoss = price + stopDistance
		takeProfit = vwap
	}

	// Create position
	position := &Position{
		Symbol:     symbol,
		Side:       side,
		EntryPrice: price,
		Size:       positionSize,
		EntryTime:  timestamp,
		StopLoss:   stopLoss,
		TakeProfit: takeProfit,
	}

	// Deduct commission AND position capital
	commission := price * positionSize * 0.001 // 0.1% commission
	positionCapital := price * positionSize
	e.balance -= (commission + positionCapital)

	e.positions[symbol] = position

	log.Debug().
		Str("symbol", symbol).
		Str("side", side).
		Float64("price", price).
		Float64("size", positionSize).
		Float64("stop_loss", stopLoss).
		Float64("take_profit", takeProfit).
		Msg("Opened position")
}

// checkPositions checks and manages existing positions
func (e *Engine) checkPositions(tick DataPoint) {
	e.mu.Lock()
	defer e.mu.Unlock()

	position, exists := e.positions[tick.Symbol]
	if !exists {
		return
	}

	currentPrice := tick.Price

	// Check stop loss
	if (position.Side == "long" && currentPrice <= position.StopLoss) ||
		(position.Side == "short" && currentPrice >= position.StopLoss) {
		e.closePosition(tick.Symbol, currentPrice, "stop_loss", tick.Timestamp)
		return
	}

	// Check take profit
	if (position.Side == "long" && currentPrice >= position.TakeProfit) ||
		(position.Side == "short" && currentPrice <= position.TakeProfit) {
		e.closePosition(tick.Symbol, currentPrice, "take_profit", tick.Timestamp)
		return
	}

	// Check max holding time (optional)
	holdingTime := tick.Timestamp.Sub(position.EntryTime)
	if holdingTime > 1*time.Hour {
		e.closePosition(tick.Symbol, currentPrice, "timeout", tick.Timestamp)
	}
}

// closePosition closes a position and records the trade
func (e *Engine) closePosition(symbol string, exitPrice float64, reason string, exitTime time.Time) {
	position, exists := e.positions[symbol]
	if !exists {
		return
	}

	// Calculate PnL
	var pnl float64
	if position.Side == "long" {
		pnl = (exitPrice - position.EntryPrice) * position.Size
	} else {
		pnl = (position.EntryPrice - exitPrice) * position.Size
	}

	// Deduct exit commission
	commission := exitPrice * position.Size * 0.001
	pnl -= commission

	// Update balance with position close proceeds
	positionCloseValue := exitPrice * position.Size
	e.balance += positionCloseValue - commission

	// Record trade
	trade := Trade{
		Symbol:     symbol,
		Side:       position.Side,
		EntryPrice: position.EntryPrice,
		ExitPrice:  exitPrice,
		Size:       position.Size,
		EntryTime:  position.EntryTime,
		ExitTime:   exitTime,
		PnL:        pnl,
		PnLPercent: (pnl / (position.EntryPrice * position.Size)) * 100,
		Commission: commission * 2, // Entry + exit commission
		ExitReason: reason,
	}

	e.results.mu.Lock()
	e.results.Trades = append(e.results.Trades, trade)
	e.results.mu.Unlock()

	delete(e.positions, symbol)

	log.Debug().
		Str("symbol", symbol).
		Str("side", position.Side).
		Float64("entry", position.EntryPrice).
		Float64("exit", exitPrice).
		Float64("pnl", pnl).
		Str("reason", reason).
		Msg("Closed position")
}

// closeAllPositions closes all open positions
func (e *Engine) closeAllPositions(reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for symbol, position := range e.positions {
		// Use last known price
		exitPrice := e.lastPrice[symbol]
		if exitPrice == 0 {
			exitPrice = position.EntryPrice // Fallback to entry price
		}

		e.closePosition(symbol, exitPrice, reason, time.Now())
	}
}

// calculateMetrics calculates final performance metrics
func (e *Engine) calculateMetrics() {
	e.results.mu.Lock()
	defer e.results.mu.Unlock()

	e.results.FinalBalance = e.balance
	e.results.TotalTrades = len(e.results.Trades)

	if e.results.TotalTrades == 0 {
		return
	}

	// Calculate basic metrics
	var totalProfit, totalLoss float64
	var returns []float64

	for _, trade := range e.results.Trades {
		e.results.TotalPnL += trade.PnL
		e.results.TotalCommission += trade.Commission

		if trade.PnL > 0 {
			e.results.WinningTrades++
			totalProfit += trade.PnL
		} else {
			e.results.LosingTrades++
			totalLoss += math.Abs(trade.PnL)
		}

		returns = append(returns, trade.PnLPercent)
	}

	// Win rate
	e.results.WinRate = float64(e.results.WinningTrades) / float64(e.results.TotalTrades)

	// Profit factor
	if totalLoss > 0 {
		e.results.ProfitFactor = totalProfit / totalLoss
	}

	// Max drawdown
	e.results.MaxDrawdown = e.calculateMaxDrawdown()

	// Sharpe ratio (simplified - assumes 0% risk-free rate)
	e.results.SharpeRatio = e.calculateSharpeRatio(returns)

	// Set time range
	if len(e.results.Trades) > 0 {
		e.results.StartTime = e.results.Trades[0].EntryTime
		e.results.EndTime = e.results.Trades[len(e.results.Trades)-1].ExitTime
	}
}

// calculateMaxDrawdown calculates maximum drawdown
func (e *Engine) calculateMaxDrawdown() float64 {
	if len(e.results.Trades) == 0 {
		return 0
	}

	peak := e.initialBalance
	maxDrawdown := 0.0
	runningBalance := e.initialBalance

	for _, trade := range e.results.Trades {
		runningBalance += trade.PnL
		if runningBalance > peak {
			peak = runningBalance
		}

		drawdown := (peak - runningBalance) / peak
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
	}

	return maxDrawdown * 100 // Return as percentage
}

// calculateSharpeRatio calculates Sharpe ratio
func (e *Engine) calculateSharpeRatio(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}

	// Calculate mean return
	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	// Calculate standard deviation
	var variance float64
	for _, r := range returns {
		variance += math.Pow(r-mean, 2)
	}
	stdDev := math.Sqrt(variance / float64(len(returns)-1))

	if stdDev == 0 {
		return 0
	}

	// Annualized Sharpe ratio (assuming daily returns and 252 trading days)
	return (mean / stdDev) * math.Sqrt(252)
}

// GetResults returns the backtesting results
func (e *Engine) GetResults() *Results {
	return e.results
}
