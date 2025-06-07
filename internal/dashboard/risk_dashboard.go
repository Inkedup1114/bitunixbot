// Package dashboard provides real-time risk monitoring and visualization for the trading bot.
// It includes comprehensive risk metrics calculation, circuit breaker monitoring,
// and web-based dashboard interfaces for live trading oversight.
//
// The package provides both REST API endpoints and WebSocket streaming for
// real-time risk monitoring and alerting capabilities.
package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

	"bitunix-bot/internal/exec"
	"bitunix-bot/internal/metrics"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// RiskMetrics represents all risk-related metrics for the dashboard.
// It provides comprehensive risk assessment including P&L tracking,
// drawdown monitoring, position exposure, and performance statistics.
type RiskMetrics struct {
	Timestamp    time.Time `json:"timestamp"` // Timestamp of metrics collection

	// Account metrics
	InitialBalance  float64 `json:"initialBalance"`  // Starting account balance
	CurrentBalance  float64 `json:"currentBalance"`  // Current account balance
	PeakBalance     float64 `json:"peakBalance"`     // Peak account balance achieved
	DailyPnL        float64 `json:"dailyPnL"`        // Daily profit and loss

	// Risk protection status
	CurrentDrawdown        float64 `json:"currentDrawdown"`        // Current drawdown percentage
	MaxDrawdownProtection  float64 `json:"maxDrawdownProtection"`  // Maximum allowed drawdown
	DailyLossLimit         float64 `json:"dailyLossLimit"`         // Daily loss limit percentage
	DrawdownProtectionHit  bool    `json:"drawdownProtectionHit"`  // Whether drawdown protection triggered
	DailyLossLimitHit      bool    `json:"dailyLossLimitHit"`      // Whether daily loss limit hit

	// Position metrics
	ActivePositions     map[string]float64 `json:"activePositions"`     // Current positions by symbol
	TotalExposure       float64            `json:"totalExposure"`       // Total position exposure
	MaxPositionExposure float64            `json:"maxPositionExposure"` // Maximum allowed position exposure

	// Circuit breaker status
	CircuitBreakerStatus map[string]bool `json:"circuitBreakerStatus"` // Status of each circuit breaker
	CircuitBreakerActive bool            `json:"circuitBreakerActive"` // Whether any circuit breaker is active

	// Trading status
	CanTrade           bool   `json:"canTrade"`           // Whether trading is currently allowed
	TradingSuspendedBy string `json:"tradingSuspendedBy"` // Reason for trading suspension

	// Performance metrics
	TotalTrades    int     `json:"totalTrades"`    // Total number of trades executed
	WinRate        float64 `json:"winRate"`        // Win rate percentage
	ProfitFactor   float64 `json:"profitFactor"`   // Profit factor ratio
	SharpeRatio    float64 `json:"sharpeRatio"`    // Sharpe ratio
	MaxDrawdownHit float64 `json:"maxDrawdownHit"` // Maximum drawdown experienced
}

// RiskDashboard provides real-time risk monitoring and visualization.
// It serves a web-based dashboard with WebSocket streaming for live updates
// of trading metrics, risk parameters, and system status.
type RiskDashboard struct {
	executor         *exec.Exec                   // Trading executor for metrics access
	metricsWrapper   *metrics.MetricsWrapper      // Metrics wrapper for system stats
	server           *http.Server                 // HTTP server for dashboard
	upgrader         websocket.Upgrader           // WebSocket upgrader for real-time updates
	clients          map[*websocket.Conn]bool     // Connected WebSocket clients
	clientsMu        sync.RWMutex                 // Mutex for client map access
	broadcastChannel chan RiskMetrics             // Channel for broadcasting metrics
	stopChannel      chan struct{}                // Channel for shutdown signaling
	isRunning        bool                         // Whether the dashboard is running
	mu               sync.RWMutex                 // Mutex for dashboard state
}

// NewRiskDashboard creates a new risk dashboard with the specified configuration.
// It sets up HTTP routes, WebSocket handling, and initializes the server
// on the specified port. Returns a ready-to-start dashboard instance.
func NewRiskDashboard(executor *exec.Exec, metricsWrapper *metrics.MetricsWrapper, port int) *RiskDashboard {
	dashboard := &RiskDashboard{
		executor:         executor,
		metricsWrapper:   metricsWrapper,
		upgrader:         websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		clients:          make(map[*websocket.Conn]bool),
		broadcastChannel: make(chan RiskMetrics, 100),
		stopChannel:      make(chan struct{}),
	}

	// Setup HTTP server
	r := mux.NewRouter()
	r.HandleFunc("/", dashboard.handleDashboard).Methods("GET")
	r.HandleFunc("/api/metrics", dashboard.handleMetricsAPI).Methods("GET")
	r.HandleFunc("/ws", dashboard.handleWebSocket).Methods("GET")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static/"))))

	dashboard.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return dashboard
}

// Start starts the risk dashboard server
func (rd *RiskDashboard) Start() error {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	if rd.isRunning {
		return fmt.Errorf("risk dashboard is already running")
	}

	// Start metrics collection and broadcasting
	go rd.metricsCollector()
	go rd.clientBroadcaster()

	// Start HTTP server
	go func() {
		log.Info().
			Str("address", rd.server.Addr).
			Msg("Starting risk dashboard server")

		if err := rd.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Risk dashboard server failed")
		}
	}()

	rd.isRunning = true
	log.Info().Msg("Risk dashboard started successfully")
	return nil
}

// Stop stops the risk dashboard server
func (rd *RiskDashboard) Stop() error {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	if !rd.isRunning {
		return nil
	}

	// Signal stop
	close(rd.stopChannel)

	// Close all WebSocket connections
	rd.clientsMu.Lock()
	for client := range rd.clients {
		client.Close()
	}
	rd.clients = make(map[*websocket.Conn]bool)
	rd.clientsMu.Unlock()

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rd.server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to shutdown risk dashboard server")
		return err
	}

	rd.isRunning = false
	log.Info().Msg("Risk dashboard stopped")
	return nil
}

// metricsCollector collects metrics every second and broadcasts to clients
func (rd *RiskDashboard) metricsCollector() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := rd.collectMetrics()
			select {
			case rd.broadcastChannel <- metrics:
			default:
				// Channel full, skip this update
			}
		case <-rd.stopChannel:
			return
		}
	}
}

// clientBroadcaster broadcasts metrics to all connected WebSocket clients
func (rd *RiskDashboard) clientBroadcaster() {
	for {
		select {
		case metrics := <-rd.broadcastChannel:
			rd.broadcastToClients(metrics)
		case <-rd.stopChannel:
			return
		}
	}
}

// collectMetrics gathers all risk metrics from the executor and metrics wrapper
func (rd *RiskDashboard) collectMetrics() RiskMetrics {
	positions := rd.executor.GetPositions()
	
	// Calculate total exposure
	totalExposure := 0.0
	for symbol, position := range positions {
		// Approximate with current position value (would need current prices for exact calculation)
		totalExposure += position * 50000 // Rough estimate, should use actual prices
	}

	// Determine why trading might be suspended
	tradingSuspendedBy := ""
	canTrade := rd.executor.CanTrade()
	if !canTrade {
		if rd.executor.CheckDailyLossLimit() {
			tradingSuspendedBy = "Daily Loss Limit"
		} else if rd.executor.CheckMaxDrawdownProtection() {
			tradingSuspendedBy = "Maximum Drawdown Protection"
		} else if rd.executor.GetCircuitBreakerStatus()["volatility"] ||
			rd.executor.GetCircuitBreakerStatus()["imbalance"] ||
			rd.executor.GetCircuitBreakerStatus()["volume"] ||
			rd.executor.GetCircuitBreakerStatus()["error_rate"] {
			tradingSuspendedBy = "Circuit Breaker"
		}
	}

	circuitBreakerStatus := rd.executor.GetCircuitBreakerStatus()
	circuitBreakerActive := false
	for _, active := range circuitBreakerStatus {
		if active {
			circuitBreakerActive = true
			break
		}
	}

	return RiskMetrics{
		Timestamp:               time.Now(),
		InitialBalance:          rd.executor.GetInitialBalance(),
		CurrentBalance:          rd.executor.GetCurrentBalance(),
		PeakBalance:             rd.executor.GetPeakBalance(),
		DailyPnL:                rd.executor.GetDailyPnL(),
		CurrentDrawdown:         rd.executor.GetCurrentDrawdown(),
		MaxDrawdownProtection:   rd.executor.GetMaxDrawdownProtection(),
		DailyLossLimit:          rd.executor.GetDailyLossLimit(),
		DrawdownProtectionHit:   rd.executor.CheckMaxDrawdownProtection(),
		DailyLossLimitHit:       rd.executor.CheckDailyLossLimit(),
		ActivePositions:         positions,
		TotalExposure:           totalExposure,
		MaxPositionExposure:     rd.executor.GetMaxPositionExposure(),
		CircuitBreakerStatus:    circuitBreakerStatus,
		CircuitBreakerActive:    circuitBreakerActive,
		CanTrade:                canTrade,
		TradingSuspendedBy:      tradingSuspendedBy,
		TotalTrades:             rd.getTotalTrades(),
		WinRate:                 rd.getWinRate(),
		ProfitFactor:            rd.getProfitFactor(),
		SharpeRatio:             rd.getSharpeRatio(),
		MaxDrawdownHit:          rd.getMaxDrawdownHit(),
	}
}

// broadcastToClients sends metrics to all connected WebSocket clients
func (rd *RiskDashboard) broadcastToClients(metrics RiskMetrics) {
	rd.clientsMu.RLock()
	defer rd.clientsMu.RUnlock()

	data, err := json.Marshal(metrics)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal metrics for broadcast")
		return
	}

	for client := range rd.clients {
		if err := client.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Error().Err(err).Msg("Failed to send message to WebSocket client")
			client.Close()
			delete(rd.clients, client)
		}
	}
}

// handleDashboard serves the main dashboard HTML page
func (rd *RiskDashboard) handleDashboard(w http.ResponseWriter, r *http.Request) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>Bitunix Bot - Risk Dashboard</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; padding: 20px; background-color: #f5f5f5; }
        .container { max-width: 1400px; margin: 0 auto; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 20px; border-radius: 10px; margin-bottom: 20px; }
        .header h1 { margin: 0; font-size: 2.5em; text-align: center; }
        .status-bar { display: flex; justify-content: space-between; align-items: center; background: white; padding: 15px; border-radius: 8px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .status-indicator { display: flex; align-items: center; font-weight: bold; }
        .status-dot { width: 12px; height: 12px; border-radius: 50%; margin-right: 8px; }
        .status-active { background-color: #28a745; }
        .status-warning { background-color: #ffc107; }
        .status-danger { background-color: #dc3545; }
        .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; }
        .card { background: white; border-radius: 10px; padding: 20px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
        .card h3 { margin-top: 0; color: #333; border-bottom: 2px solid #eee; padding-bottom: 10px; }
        .metric { display: flex; justify-content: space-between; align-items: center; padding: 8px 0; border-bottom: 1px solid #eee; }
        .metric:last-child { border-bottom: none; }
        .metric-label { font-weight: 500; color: #666; }
        .metric-value { font-weight: bold; color: #333; }
        .metric-positive { color: #28a745; }
        .metric-negative { color: #dc3545; }
        .metric-warning { color: #ffc107; }
        .positions-table { width: 100%; border-collapse: collapse; margin-top: 10px; }
        .positions-table th, .positions-table td { text-align: left; padding: 8px; border-bottom: 1px solid #eee; }
        .positions-table th { background-color: #f8f9fa; font-weight: 600; }
        .circuit-breaker { display: flex; justify-content: space-between; align-items: center; padding: 5px 0; }
        .circuit-status { padding: 2px 8px; border-radius: 4px; font-size: 0.8em; font-weight: bold; }
        .circuit-active { background-color: #dc3545; color: white; }
        .circuit-inactive { background-color: #28a745; color: white; }
        .large-metric { font-size: 1.5em; text-align: center; margin: 10px 0; }
        .progress-bar { width: 100%; height: 20px; background-color: #eee; border-radius: 10px; overflow: hidden; margin: 10px 0; }
        .progress-fill { height: 100%; transition: width 0.3s ease; }
        .progress-safe { background-color: #28a745; }
        .progress-warning { background-color: #ffc107; }
        .progress-danger { background-color: #dc3545; }
        @keyframes pulse { 0% { opacity: 1; } 50% { opacity: 0.5; } 100% { opacity: 1; } }
        .pulsing { animation: pulse 2s infinite; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🛡️ Risk Management Dashboard</h1>
        </div>
        
        <div class="status-bar">
            <div class="status-indicator">
                <div class="status-dot" id="trading-status"></div>
                <span id="trading-status-text">Checking...</span>
            </div>
            <div class="status-indicator">
                <span id="last-update">Last Updated: --</span>
            </div>
        </div>

        <div class="grid">
            <!-- Account Overview -->
            <div class="card">
                <h3>💰 Account Overview</h3>
                <div class="metric">
                    <span class="metric-label">Initial Balance</span>
                    <span class="metric-value" id="initial-balance">$0.00</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Current Balance</span>
                    <span class="metric-value" id="current-balance">$0.00</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Peak Balance</span>
                    <span class="metric-value" id="peak-balance">$0.00</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Daily P&L</span>
                    <span class="metric-value" id="daily-pnl">$0.00</span>
                </div>
            </div>

            <!-- Drawdown Protection -->
            <div class="card">
                <h3>📉 Drawdown Protection</h3>
                <div class="large-metric">
                    <span id="current-drawdown">0.00%</span>
                </div>
                <div class="progress-bar">
                    <div class="progress-fill" id="drawdown-progress"></div>
                </div>
                <div class="metric">
                    <span class="metric-label">Max Allowed</span>
                    <span class="metric-value" id="max-drawdown-protection">0.00%</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Status</span>
                    <span class="metric-value" id="drawdown-status">Safe</span>
                </div>
            </div>

            <!-- Daily Loss Limit -->
            <div class="card">
                <h3>⚠️ Daily Loss Limit</h3>
                <div class="large-metric">
                    <span id="daily-loss-percentage">0.00%</span>
                </div>
                <div class="progress-bar">
                    <div class="progress-fill" id="daily-loss-progress"></div>
                </div>
                <div class="metric">
                    <span class="metric-label">Max Allowed</span>
                    <span class="metric-value" id="daily-loss-limit">0.00%</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Status</span>
                    <span class="metric-value" id="daily-loss-status">Safe</span>
                </div>
            </div>

            <!-- Circuit Breakers -->
            <div class="card">
                <h3>🔌 Circuit Breakers</h3>
                <div class="circuit-breaker">
                    <span>Volatility</span>
                    <span class="circuit-status" id="circuit-volatility">INACTIVE</span>
                </div>
                <div class="circuit-breaker">
                    <span>Order Book Imbalance</span>
                    <span class="circuit-status" id="circuit-imbalance">INACTIVE</span>
                </div>
                <div class="circuit-breaker">
                    <span>Volume</span>
                    <span class="circuit-status" id="circuit-volume">INACTIVE</span>
                </div>
                <div class="circuit-breaker">
                    <span>Error Rate</span>
                    <span class="circuit-status" id="circuit-error-rate">INACTIVE</span>
                </div>
            </div>

            <!-- Active Positions -->
            <div class="card">
                <h3>📊 Active Positions</h3>
                <div class="metric">
                    <span class="metric-label">Total Exposure</span>
                    <span class="metric-value" id="total-exposure">$0.00</span>
                </div>
                <table class="positions-table">
                    <thead>
                        <tr>
                            <th>Symbol</th>
                            <th>Position</th>
                            <th>Exposure</th>
                        </tr>
                    </thead>
                    <tbody id="positions-table-body">
                        <tr>
                            <td colspan="3" style="text-align: center; color: #666;">No active positions</td>
                        </tr>
                    </tbody>
                </table>
            </div>

            <!-- Performance Metrics -->
            <div class="card">
                <h3>📈 Performance</h3>
                <div class="metric">
                    <span class="metric-label">Total Trades</span>
                    <span class="metric-value" id="total-trades">0</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Win Rate</span>
                    <span class="metric-value" id="win-rate">0.00%</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Profit Factor</span>
                    <span class="metric-value" id="profit-factor">0.00</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Sharpe Ratio</span>
                    <span class="metric-value" id="sharpe-ratio">0.00</span>
                </div>
            </div>
        </div>
    </div>

    <script>
        const ws = new WebSocket('ws://' + location.host + '/ws');
        
        ws.onmessage = function(event) {
            const data = JSON.parse(event.data);
            updateDashboard(data);
        };
        
        ws.onopen = function() {
            console.log('WebSocket connected');
        };
        
        ws.onclose = function() {
            console.log('WebSocket disconnected');
            setTimeout(() => location.reload(), 5000);
        };
        
        function updateDashboard(data) {
            // Update timestamp
            document.getElementById('last-update').textContent = 'Last Updated: ' + new Date(data.timestamp).toLocaleTimeString();
            
            // Update trading status
            const statusDot = document.getElementById('trading-status');
            const statusText = document.getElementById('trading-status-text');
            if (data.canTrade) {
                statusDot.className = 'status-dot status-active';
                statusText.textContent = 'Trading Active';
            } else {
                statusDot.className = 'status-dot status-danger';
                statusText.textContent = 'Trading Suspended: ' + data.tradingSuspendedBy;
            }
            
            // Update account overview
            document.getElementById('initial-balance').textContent = '$' + data.initialBalance.toFixed(2);
            document.getElementById('current-balance').textContent = '$' + data.currentBalance.toFixed(2);
            document.getElementById('peak-balance').textContent = '$' + data.peakBalance.toFixed(2);
            
            const dailyPnL = document.getElementById('daily-pnl');
            dailyPnL.textContent = '$' + data.dailyPnL.toFixed(2);
            dailyPnL.className = 'metric-value ' + (data.dailyPnL >= 0 ? 'metric-positive' : 'metric-negative');
            
            // Update drawdown protection
            const drawdownPercentage = (data.currentDrawdown * 100).toFixed(2);
            document.getElementById('current-drawdown').textContent = drawdownPercentage + '%';
            document.getElementById('max-drawdown-protection').textContent = (data.maxDrawdownProtection * 100).toFixed(2) + '%';
            
            const drawdownProgress = document.getElementById('drawdown-progress');
            const drawdownProgressValue = Math.min((data.currentDrawdown / data.maxDrawdownProtection) * 100, 100);
            drawdownProgress.style.width = drawdownProgressValue + '%';
            
            if (data.drawdownProtectionHit) {
                drawdownProgress.className = 'progress-fill progress-danger pulsing';
                document.getElementById('drawdown-status').textContent = 'LIMIT HIT';
                document.getElementById('drawdown-status').className = 'metric-value metric-negative';
            } else if (drawdownProgressValue > 80) {
                drawdownProgress.className = 'progress-fill progress-warning';
                document.getElementById('drawdown-status').textContent = 'Warning';
                document.getElementById('drawdown-status').className = 'metric-value metric-warning';
            } else {
                drawdownProgress.className = 'progress-fill progress-safe';
                document.getElementById('drawdown-status').textContent = 'Safe';
                document.getElementById('drawdown-status').className = 'metric-value metric-positive';
            }
            
            // Update daily loss limit
            const dailyLossPercentage = data.dailyPnL < 0 ? (Math.abs(data.dailyPnL) / data.initialBalance * 100).toFixed(2) : '0.00';
            document.getElementById('daily-loss-percentage').textContent = dailyLossPercentage + '%';
            document.getElementById('daily-loss-limit').textContent = (data.dailyLossLimit * 100).toFixed(2) + '%';
            
            const dailyLossProgress = document.getElementById('daily-loss-progress');
            const dailyLossProgressValue = Math.min((Math.abs(data.dailyPnL) / data.initialBalance) / data.dailyLossLimit * 100, 100);
            dailyLossProgress.style.width = dailyLossProgressValue + '%';
            
            if (data.dailyLossLimitHit) {
                dailyLossProgress.className = 'progress-fill progress-danger pulsing';
                document.getElementById('daily-loss-status').textContent = 'LIMIT HIT';
                document.getElementById('daily-loss-status').className = 'metric-value metric-negative';
            } else if (dailyLossProgressValue > 80) {
                dailyLossProgress.className = 'progress-fill progress-warning';
                document.getElementById('daily-loss-status').textContent = 'Warning';
                document.getElementById('daily-loss-status').className = 'metric-value metric-warning';
            } else {
                dailyLossProgress.className = 'progress-fill progress-safe';
                document.getElementById('daily-loss-status').textContent = 'Safe';
                document.getElementById('daily-loss-status').className = 'metric-value metric-positive';
            }
            
            // Update circuit breakers
            updateCircuitBreaker('circuit-volatility', data.circuitBreakerStatus.volatility);
            updateCircuitBreaker('circuit-imbalance', data.circuitBreakerStatus.imbalance);
            updateCircuitBreaker('circuit-volume', data.circuitBreakerStatus.volume);
            updateCircuitBreaker('circuit-error-rate', data.circuitBreakerStatus.error_rate);
            
            // Update positions
            updatePositionsTable(data.activePositions);
            document.getElementById('total-exposure').textContent = '$' + data.totalExposure.toFixed(2);
            
            // Update performance metrics
            document.getElementById('total-trades').textContent = data.totalTrades;
            document.getElementById('win-rate').textContent = (data.winRate * 100).toFixed(2) + '%';
            document.getElementById('profit-factor').textContent = data.profitFactor.toFixed(2);
            document.getElementById('sharpe-ratio').textContent = data.sharpeRatio.toFixed(2);
        }
        
        function updateCircuitBreaker(elementId, isActive) {
            const element = document.getElementById(elementId);
            if (isActive) {
                element.textContent = 'ACTIVE';
                element.className = 'circuit-status circuit-active pulsing';
            } else {
                element.textContent = 'INACTIVE';
                element.className = 'circuit-status circuit-inactive';
            }
        }
        
        function updatePositionsTable(positions) {
            const tbody = document.getElementById('positions-table-body');
            tbody.innerHTML = '';
            
            if (Object.keys(positions).length === 0) {
                tbody.innerHTML = '<tr><td colspan="3" style="text-align: center; color: #666;">No active positions</td></tr>';
                return;
            }
            
            for (const [symbol, position] of Object.entries(positions)) {
                const row = document.createElement('tr');
                const exposure = Math.abs(position) * 50000; // Rough estimate
                row.innerHTML = `
                    <td>${symbol}</td>
                    <td class="${position >= 0 ? 'metric-positive' : 'metric-negative'}">${position.toFixed(4)}</td>
                    <td>$${exposure.toFixed(2)}</td>
                `;
                tbody.appendChild(row);
            }
        }
    </script>
</body>
</html>
	`

	t, err := template.New("dashboard").Parse(tmpl)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	t.Execute(w, nil)
}

// handleMetricsAPI serves metrics data as JSON
func (rd *RiskDashboard) handleMetricsAPI(w http.ResponseWriter, r *http.Request) {
	metrics := rd.collectMetrics()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// handleWebSocket handles WebSocket connections for real-time updates
func (rd *RiskDashboard) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := rd.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upgrade WebSocket connection")
		return
	}
	defer conn.Close()

	// Add client to the list
	rd.clientsMu.Lock()
	rd.clients[conn] = true
	rd.clientsMu.Unlock()

	// Send initial metrics
	metrics := rd.collectMetrics()
	if data, err := json.Marshal(metrics); err == nil {
		conn.WriteMessage(websocket.TextMessage, data)
	}

	// Keep connection alive
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	// Remove client from the list
	rd.clientsMu.Lock()
	delete(rd.clients, conn)
	rd.clientsMu.Unlock()
}

// Helper methods to get performance metrics (placeholder implementations)
func (rd *RiskDashboard) getTotalTrades() int {
	// TODO: Implement actual total trades counting
	return 0
}

func (rd *RiskDashboard) getWinRate() float64 {
	// TODO: Implement actual win rate calculation
	return 0.0
}

func (rd *RiskDashboard) getProfitFactor() float64 {
	// TODO: Implement actual profit factor calculation
	return 0.0
}

func (rd *RiskDashboard) getSharpeRatio() float64 {
	// TODO: Implement actual Sharpe ratio calculation
	return 0.0
}

func (rd *RiskDashboard) getMaxDrawdownHit() float64 {
	// TODO: Implement actual max drawdown hit calculation
	return 0.0
} 