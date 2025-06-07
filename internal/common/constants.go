package common

// Trading symbols
const (
	BTCUSDTSymbol = "BTCUSDT"
	ETHUSDTSymbol = "ETHUSDT"
	ADAUSDTSymbol = "ADAUSDT"
	BNBUSDTSymbol = "BNBUSDT"
	DOTUSDTSymbol = "DOTUSDT"
)

// Environment variable keys
const (
	EnvBitunixAPIKey       = "BITUNIX_API_KEY"
	EnvBitunixSecretKey    = "BITUNIX_SECRET_KEY"
	EnvForceLiveTrading    = "FORCE_LIVE_TRADING"
	EnvSymbols             = "SYMBOLS"
	EnvBaseURL             = "BASE_URL"
	EnvWsURL               = "WS_URL"
	EnvDataPath            = "DATA_PATH"
	EnvModelPath           = "MODEL_PATH"
	EnvVWAPSize            = "VWAP_SIZE"
	EnvTickSize            = "TICK_SIZE"
	EnvBaseSizeRatio       = "BASE_SIZE_RATIO"
	EnvProbThreshold       = "PROB_THRESHOLD"
	EnvDryRun              = "DRY_RUN"
	EnvMaxDailyLoss        = "MAX_DAILY_LOSS"
	EnvMetricsPort         = "METRICS_PORT"
	EnvMaxPositionSize     = "MAX_POSITION_SIZE"
	EnvMaxPositionExposure = "MAX_POSITION_EXPOSURE"
	EnvMaxPriceDistance    = "MAX_PRICE_DISTANCE"
	EnvLeverage            = "LEVERAGE"
	EnvMarginMode          = "MARGIN_MODE"
	EnvRiskUSD             = "RISK_USD"
	EnvRESTTimeout         = "REST_TIMEOUT"
	EnvPingInterval        = "PING_INTERVAL"
	EnvVWAPWindow          = "VWAP_WINDOW"
	EnvMLServerPort        = "ML_SERVER_PORT"
)

// Configuration defaults
const (
	DefaultBaseURL             = "https://api.bitunix.com"
	DefaultWsURL               = "wss://fapi.bitunix.com/public"
	DefaultModelPath           = "models/model.onnx"
	DefaultMarginMode          = "ISOLATION"
	DefaultMetricsPort         = 8080
	DefaultLeverage            = 20
	DefaultRiskUSD             = 25.0
	DefaultMaxPositionSize     = 0.01 // 1%
	DefaultMaxPositionExposure = 0.1  // 10%
	DefaultMaxDailyLoss        = 0.05 // 5%
	DefaultBaseSizeRatio       = 0.002
	DefaultProbThreshold       = 0.65
	DefaultMaxPriceDistance    = 3.0
	DefaultVWAPSize            = 600
	DefaultTickSize            = 50
)

// Circuit breaker environment keys
const (
	EnvCircuitBreakerVolatility = "CIRCUIT_BREAKER_VOLATILITY"
	EnvCircuitBreakerImbalance  = "CIRCUIT_BREAKER_IMBALANCE"
	EnvCircuitBreakerVolume     = "CIRCUIT_BREAKER_VOLUME"
	EnvCircuitBreakerErrorRate  = "CIRCUIT_BREAKER_ERROR_RATE"
	EnvCircuitBreakerRecovery   = "CIRCUIT_BREAKER_RECOVERY"
)

// Order execution timeout environment keys
const (
	EnvOrderExecutionTimeout    = "ORDER_EXECUTION_TIMEOUT"
	EnvOrderStatusCheckInterval = "ORDER_STATUS_CHECK_INTERVAL"
	EnvMaxOrderRetries          = "MAX_ORDER_RETRIES"
)

// Maximum drawdown protection environment keys
const (
	EnvMaxDrawdownProtection = "MAX_DRAWDOWN_PROTECTION"
)

// Circuit breaker defaults
const (
	DefaultCircuitBreakerVolatility = 2.0 // 2 std devs
	DefaultCircuitBreakerImbalance  = 0.8 // 80% imbalance
	DefaultCircuitBreakerVolume     = 5.0 // 5x normal volume
	DefaultCircuitBreakerErrorRate  = 0.2 // 20% error rate
)

// Maximum drawdown protection defaults
const (
	DefaultMaxDrawdownProtection = 0.1 // 10% maximum drawdown from peak
)

// Common error messages
const (
	ErrMsgAPIKeyRequired           = "API key and secret are required"
	ErrMsgBaseURLRequired          = "baseURL is required"
	ErrMsgWsURLRequired            = "wsURL is required"
	ErrMsgSymbolRequired           = "at least one trading symbol is required"
	ErrMsgForceLiveTradingRequired = "live trading requires FORCE_LIVE_TRADING=true environment variable"
)

// Validation constants
const (
	MaxBaseSizeRatio     = 0.1
	MaxPositionSizeLimit = 1.0
	MaxDailyLossLimit    = 1.0
	MinProbThreshold     = 0.5
	MaxProbThreshold     = 0.99
	MaxPositionSizeLive  = 0.1  // 10% for live trading
	MaxDailyLossLive     = 0.05 // 5% for live trading
	MinMetricsPort       = 1024
	MaxMetricsPort       = 65535
	MinVWAPSize          = 10
	MaxVWAPSize          = 10000
)
