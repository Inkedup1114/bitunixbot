package features

import (
	"math"
	"sync"
)

// MetricsTracker interface for error tracking
type ImbalanceMetricsTracker interface {
	FeatureErrorsInc()
}

func DepthImb(bid, ask float64) float64 {
	return DepthImbWithMetrics(bid, ask, nil)
}

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

type TickImb struct {
	buf []int8
	max int
	mu  sync.RWMutex
}

func NewTickImb(n int) *TickImb { return &TickImb{max: n} }

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
