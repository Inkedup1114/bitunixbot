// Package features provides technical indicator calculations for trading analysis.
// It includes Volume Weighted Average Price (VWAP) calculations, order book
// imbalance metrics, and other market microstructure features used for
// algorithmic trading decisions.
//
// All calculations are thread-safe and optimized for high-frequency data processing.
package features

import (
	"container/ring"
	"math"
	"sync"
	"time"

	"bitunix-bot/internal/metrics"
)

// sample represents a single price-volume observation with timestamp.
type sample struct {
	p, v float64   // price and volume
	t    time.Time // timestamp
}

// VWAP calculates Volume Weighted Average Price over a sliding time window.
// It maintains a ring buffer of price-volume samples and provides thread-safe
// calculations of VWAP and standard deviation. All methods are thread-safe
// and can be called concurrently.
type VWAP struct {
	win         time.Duration // Time window for VWAP calculation
	ring        *ring.Ring    // Ring buffer for storing samples
	mu          sync.RWMutex  // Mutex for thread-safe access
	maxSize     int           // Maximum number of samples in the ring
	currentSize int           // Current number of samples in the ring
	samplePool  *sync.Pool    // Object pool for sample reuse to reduce allocations
}

// MetricsTracker interface for error tracking and performance monitoring.
// Implementations should provide methods to track feature calculation errors,
// duration metrics, and sample count statistics.
type MetricsTracker interface {
	FeatureErrorsInc()                          // Increment error counter
	FeatureCalcDuration(duration time.Duration) // Record calculation duration
	FeatureSampleCount(count int)               // Record number of samples processed
}

// NewVWAP creates a new VWAP calculator with the specified time window and sample size.
// The time window determines how far back in time to include samples, while size
// limits the maximum number of samples stored. Returns a thread-safe VWAP instance.
func NewVWAP(win time.Duration, size int) *VWAP {
	if size <= 0 {
		size = 1
	}
	// Validate time window
	if win <= 0 {
		// Default to 1 minute, or handle as an error (e.g., return an error or panic)
		// For this example, defaulting to 1 minute.
		win = time.Minute
	}
	return &VWAP{
		win:         win,
		ring:        ring.New(size),
		maxSize:     size,
		currentSize: 0,
		samplePool: &sync.Pool{ // Initialize samplePool
			New: func() interface{} {
				return &sample{}
			},
		},
	}
}

// Add adds a new price-volume sample to the VWAP calculation.
// This is a convenience method that calls AddWithMetrics with nil metrics.
func (v *VWAP) Add(price, volume float64) {
	v.AddWithMetrics(price, volume, nil)
}

// AddWithMetrics adds a new price-volume sample to the VWAP calculation with metrics tracking.
// It validates the input values and updates the ring buffer. Invalid values (NaN, infinity,
// negative) are rejected and tracked as errors if metrics are provided.
func (v *VWAP) AddWithMetrics(price, volume float64, metrics MetricsTracker) {
	// Fast path: validate inputs before acquiring lock
	if math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
		if metrics != nil {
			metrics.FeatureErrorsInc()
		}
		return
	}
	if math.IsNaN(volume) || math.IsInf(volume, 0) || volume < 0 {
		if metrics != nil {
			metrics.FeatureErrorsInc()
		}
		return
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// Get a sample from the pool
	s := v.samplePool.Get().(*sample)
	s.p = price
	s.v = volume
	s.t = time.Now()

	// If the ring is full and has an old sample, put it back to the pool
	if v.ring.Value != nil {
		oldSample, ok := v.ring.Value.(*sample)
		if ok && oldSample != nil {
			v.samplePool.Put(oldSample)
		}
	}

	v.ring.Value = s
	v.ring = v.ring.Next()

	// Track the current size up to maxSize
	if v.currentSize < v.maxSize {
		v.currentSize++
	}
}

// Calc calculates the current VWAP and standard deviation.
// This is a convenience method that calls CalcWithMetrics with nil metrics.
// Returns the VWAP value and standard deviation.
func (v *VWAP) Calc() (value, std float64) {
	return v.CalcWithMetrics(nil)
}

// CalcWithMetrics calculates the current VWAP and standard deviation with metrics tracking.
// It processes all samples within the time window, calculates volume-weighted statistics,
// and tracks performance metrics if provided. Returns VWAP value and standard deviation.
func (v *VWAP) CalcWithMetrics(metrics MetricsTracker) (value, std float64) {
	startTime := time.Now() // For calc duration metric

	v.mu.RLock()
	defer v.mu.RUnlock()

	// Fast path: early return for empty ring
	if v.currentSize == 0 {
		if metrics != nil {
			metrics.FeatureCalcDuration(time.Since(startTime))
			metrics.FeatureSampleCount(0)
		}
		return 0, 0
	}

	// Collect valid samples within time window
	validSamples, pv, vv, count := v.collectValidSamples(metrics)

	if metrics != nil {
		metrics.FeatureSampleCount(count)
	}

	// Fast path: empty data or zero volume
	if vv == 0 || count == 0 {
		if metrics != nil {
			metrics.FeatureCalcDuration(time.Since(startTime))
		}
		return 0, 0
	}

	value = pv / vv

	// Fast path: single sample, variance is 0
	if count == 1 {
		if metrics != nil {
			metrics.FeatureCalcDuration(time.Since(startTime))
		}
		return value, 0
	}

	// Calculate volume-weighted standard deviation
	std = v.calculateWeightedStd(validSamples, value, vv, metrics)

	// Final validation
	value, std = v.validateOutputs(value, std, metrics)

	if metrics != nil {
		metrics.FeatureCalcDuration(time.Since(startTime))
	}

	return value, std
}

// collectValidSamples collects samples within the time window and calculates basic statistics
func (v *VWAP) collectValidSamples(metrics MetricsTracker) ([]sample, float64, float64, int) {
	var pv, vv float64 // price*volume sum, volume sum
	var count int
	cutoff := time.Now().Add(-v.win)

	// Pre-allocate validSamples slice to avoid allocations in hot path
	validSamples := make([]sample, 0, v.currentSize)

	v.ring.Do(func(x any) {
		if x == nil {
			return
		}

		sPtr, ok := x.(*sample)
		if !ok || sPtr == nil {
			return
		}
		s := *sPtr // Dereference to get the sample value

		if s.t.After(cutoff) {
			// Basic sanity checks for invalid data
			if v.isValidSample(s, metrics) {
				pv += s.p * s.v
				vv += s.v
				validSamples = append(validSamples, s)
				count++
			}
		}
	})

	return validSamples, pv, vv, count
}

// isValidSample validates a single sample for NaN, infinity, and negative values
func (v *VWAP) isValidSample(s sample, metrics MetricsTracker) bool {
	if math.IsNaN(s.p) || math.IsInf(s.p, 0) || s.p < 0 {
		if metrics != nil {
			metrics.FeatureErrorsInc()
		}
		return false
	}
	if math.IsNaN(s.v) || math.IsInf(s.v, 0) || s.v < 0 {
		if metrics != nil {
			metrics.FeatureErrorsInc()
		}
		return false
	}
	return true
}

// calculateWeightedStd calculates the volume-weighted standard deviation
func (v *VWAP) calculateWeightedStd(validSamples []sample, value, vv float64, metrics MetricsTracker) float64 {
	var weightedVariance float64
	for _, s := range validSamples {
		deviation := s.p - value
		weightedVariance += s.v * deviation * deviation
	}

	if vv > 0 {
		variance := weightedVariance / vv
		// Guard against negative variance (numerical precision issues)
		if variance > 0 {
			return math.Sqrt(variance)
		} else {
			if variance < 0 && metrics != nil {
				metrics.FeatureErrorsInc() // Log if variance is negative
			}
			return 0
		}
	}
	return 0
}

// validateOutputs performs final validation on the calculated value and standard deviation
func (v *VWAP) validateOutputs(value, std float64, metrics MetricsTracker) (float64, float64) {
	// Final sanity checks on output
	if math.IsNaN(value) || math.IsInf(value, 0) {
		if metrics != nil {
			metrics.FeatureErrorsInc()
		}
		return 0, 0
	}
	if math.IsNaN(std) || math.IsInf(std, 0) {
		if metrics != nil {
			metrics.FeatureErrorsInc()
		}
		std = 0
	}

	return value, std
}

// GetCurrentSize returns the current number of samples in the VWAP calculation window.
// It is thread-safe.
func (v *VWAP) GetCurrentSize() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.currentSize
}

// Reset clears all samples from the VWAP calculator.
// It is thread-safe.
func (v *VWAP) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Iterate over the ring and put samples back to the pool
	// and nil out the ring values
	current := v.ring
	for i := 0; i < v.maxSize; i++ {
		if current.Value != nil {
			s, ok := current.Value.(*sample)
			if ok && s != nil {
				v.samplePool.Put(s)
			}
			current.Value = nil
		}
		current = current.Next()
	}

	// Reset currentSize without recreating the ring
	v.currentSize = 0

	// No need to recreate the ring - just reuse the existing one
	// The existing logic in AddWithMetrics will properly handle nil values
}

// FastVWAP implements a high-performance VWAP calculator
type FastVWAP struct {
	samples    []sample
	head, tail int
	size       int
	mu         sync.RWMutex
	pool       *sync.Pool
	batchSize  int
	win        time.Duration
}

// NewFastVWAP creates a new optimized VWAP calculator
func NewFastVWAP(win time.Duration, size int) *FastVWAP {
	if size <= 0 {
		size = 1
	}
	if win <= 0 {
		win = time.Minute
	}
	return &FastVWAP{
		samples:   make([]sample, size),
		size:      size,
		batchSize: 32, // Optimal batch size for cache lines
		win:       win,
		pool: &sync.Pool{
			New: func() interface{} {
				return &sample{}
			},
		},
	}
}

// BatchUpdate processes multiple price updates efficiently
func (v *FastVWAP) BatchUpdate(prices, volumes []float64) {
	if len(prices) != len(volumes) {
		return
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	for i := 0; i < len(prices); i += v.batchSize {
		end := i + v.batchSize
		if end > len(prices) {
			end = len(prices)
		}

		// Process batch
		for j := i; j < end; j++ {
			if math.IsNaN(prices[j]) || math.IsInf(prices[j], 0) || prices[j] < 0 {
				continue
			}
			if math.IsNaN(volumes[j]) || math.IsInf(volumes[j], 0) || volumes[j] < 0 {
				continue
			}

			// Get sample from pool
			s := v.pool.Get().(*sample)
			s.p = prices[j]
			s.v = volumes[j]
			s.t = now

			// Update circular buffer
			v.samples[v.tail] = *s
			v.tail = (v.tail + 1) % v.size
			if v.tail == v.head {
				v.head = (v.head + 1) % v.size
			}

			// Return sample to pool
			v.pool.Put(s)
		}
	}
}

// FastCalc performs optimized VWAP calculation
func (v *FastVWAP) FastCalc() (value, std float64) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.head == v.tail {
		return 0, 0
	}

	var pv, vv float64
	count := 0
	cutoff := time.Now().Add(-v.win)

	// Pre-allocate slice for valid samples
	validSamples := make([]sample, 0, v.size)

	// Process samples in batches
	for i := v.head; i != v.tail; i = (i + 1) % v.size {
		s := v.samples[i]
		if s.t.After(cutoff) {
			pv += s.p * s.v
			vv += s.v
			validSamples = append(validSamples, s)
			count++
		}
	}

	if vv == 0 || count == 0 {
		return 0, 0
	}

	value = pv / vv

	if count == 1 {
		return value, 0
	}

	// Calculate variance using Welford's online algorithm
	var m2 float64
	for _, s := range validSamples {
		delta := s.p - value
		m2 += s.v * delta * delta
	}

	if vv > 0 {
		variance := m2 / vv
		if variance > 0 {
			std = math.Sqrt(variance)
		}
	}

	return value, std
}

//go:noescape
func simdVWAPCalc(samples []sample, size int) (sum, sumSq, volSum float64)

// CalcWithMetrics calculates VWAP and standard deviation with metrics tracking
func (v *FastVWAP) CalcWithMetrics(m *metrics.MetricsWrapper) (float64, float64) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.size == 0 {
		return 0, 0
	}

	// Use SIMD-optimized calculation if available
	if v.size >= 4 { // Only use SIMD for larger batches
		sum, sumSq, volSum := simdVWAPCalc(v.samples[v.head:v.head+v.size], v.size)
		if volSum > 0 {
			vwap := sum / volSum
			variance := (sumSq/volSum - vwap*vwap)
			std := math.Sqrt(variance)

			// Track metrics
			if m != nil {
				m.FeatureSampleCount(1)
			}

			return vwap, std
		}
	}

	// Fallback to scalar calculation for small batches
	var sum, sumSq, volSum float64
	for i := 0; i < v.size; i++ {
		idx := (v.head + i) % len(v.samples)
		sample := v.samples[idx]
		sum += sample.p * sample.v
		sumSq += sample.p * sample.p * sample.v
		volSum += sample.v
	}

	if volSum == 0 {
		return 0, 0
	}

	vwap := sum / volSum
	variance := (sumSq/volSum - vwap*vwap)
	std := math.Sqrt(variance)

	// Track metrics
	if m != nil {
		m.FeatureSampleCount(1)
	}

	return vwap, std
}

// Add adds a new price and volume sample to the VWAP calculation
func (v *FastVWAP) Add(price, volume float64) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Add new sample
	v.samples[v.tail] = sample{
		p: price,
		v: volume,
		t: time.Now(),
	}

	// Update indices
	v.tail = (v.tail + 1) % len(v.samples)
	if v.size < len(v.samples) {
		v.size++
	} else {
		v.head = (v.head + 1) % len(v.samples)
	}

	// Evict old samples
	now := time.Now()
	for v.size > 0 {
		oldest := v.samples[v.head]
		if now.Sub(oldest.t) <= v.win {
			break
		}
		v.head = (v.head + 1) % len(v.samples)
		v.size--
	}
}
