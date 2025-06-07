package features

import (
	"math"
	"sync"
)

// ImbalanceMetricsTracker interface for error tracking in imbalance calculations.
// Implementations should provide methods to track calculation errors.
type ImbalanceMetricsTracker interface {
	FeatureErrorsInc() // Increment error counter for invalid calculations
}

// DepthImb calculates the order book depth imbalance ratio.
// This is a convenience function that calls DepthImbWithMetrics with nil metrics.
// Returns a value between -1 and 1, where positive values indicate bid dominance.
func DepthImb(bid, ask float64) float64 {
	return DepthImbWithMetrics(bid, ask, nil)
}

// DepthImbWithMetrics calculates the order book depth imbalance ratio with metrics tracking.
// The imbalance is calculated as (bid - ask) / (bid + ask), providing a normalized
// measure of order book pressure. Returns 0 for invalid inputs and tracks errors if metrics provided.
func DepthImbWithMetrics(bid, ask float64, metrics ImbalanceMetricsTracker) float64 {
	// Sanity checks for invalid data
	if math.IsNaN(bid) || math.IsInf(bid, 0) || bid < 0 {
		if metrics != nil {
			metrics.FeatureErrorsInc()
		}
		return 0
	}
	if math.IsNaN(ask) || math.IsInf(ask, 0) || ask < 0 {
		if metrics != nil {
			metrics.FeatureErrorsInc()
		}
		return 0
	}

	if bid+ask == 0 {
		return 0
	}

	result := (bid - ask) / (bid + ask)

	// Check result validity
	if math.IsNaN(result) || math.IsInf(result, 0) {
		if metrics != nil {
			metrics.FeatureErrorsInc()
		}
		return 0
	}

	return result
}

// TickImb tracks tick-by-tick price movement imbalance over a sliding window.
// It maintains a buffer of trade direction signs (1 for uptick, -1 for downtick)
// and calculates the imbalance ratio. All methods are thread-safe.
type TickImb struct {
	buf []int8       // Buffer storing trade direction signs
	max int          // Maximum buffer size
	mu  sync.RWMutex // Mutex for thread-safe access
}

// NewTickImb creates a new tick imbalance tracker with the specified buffer size.
// The buffer size determines how many recent ticks are considered in the calculation.
func NewTickImb(n int) *TickImb { return &TickImb{max: n} }

// Add adds a new trade direction sign to the tick imbalance tracker.
// The sign should be 1 for upticks (price increase) and -1 for downticks (price decrease).
// The method is thread-safe and maintains a sliding window of the specified size.
func (t *TickImb) Add(sign int8) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Handle zero or negative max size
	if t.max <= 0 {
		return
	}

	if len(t.buf) == t.max {
		t.buf = t.buf[1:]
	}
	t.buf = append(t.buf, sign)
}

// Ratio calculates the current tick imbalance ratio.
// Returns a value between -1 and 1, where positive values indicate more upticks
// than downticks. Returns 0 if no ticks are recorded. The method is thread-safe.
func (t *TickImb) Ratio() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if len(t.buf) == 0 {
		return 0
	}
	var s int
	for _, v := range t.buf {
		s += int(v)
	}
	return float64(s) / float64(len(t.buf))
}
