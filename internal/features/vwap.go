package features

import (
	"container/ring"
	"math"
	"sync"
	"time"

	"bitunix-bot/internal/metrics"
)

type sample struct {
	p, v float64
	t    time.Time
}

// VWAP calculates Volume Weighted Average Price over a sliding time window.
// All methods are thread-safe and can be called concurrently.
type VWAP struct {
	win         time.Duration
	ring        *ring.Ring
	mu          sync.RWMutex
	maxSize     int
	currentSize int
	samplePool  *sync.Pool // Added for sample object pooling
}

// MetricsTracker interface for error tracking and performance monitoring
type MetricsTracker interface {
	FeatureErrorsInc()
	FeatureCalcDuration(duration time.Duration) // Added for performance monitoring
	FeatureSampleCount(count int)               // Added for performance monitoring
}

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

func (v *VWAP) Add(price, volume float64) {
	v.AddWithMetrics(price, volume, nil)
}

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

func (v *VWAP) Calc() (value, std float64) {
	return v.CalcWithMetrics(nil)
}

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

	var pv, vv float64 // price*volume sum, volume sum
	var count int
	cutoff := time.Now().Add(-v.win)

	// Pre-allocate validSamples slice to avoid allocations in hot path
	// Maximum size is currentSize, but typically it will be smaller
	validSamples := make([]sample, 0, v.currentSize)

	v.ring.Do(func(x any) {
		if x == nil {
			return
		}

		sPtr, ok := x.(*sample)
		if !ok || sPtr == nil { // Check if it's a pointer and not nil
			return
		}
		s := *sPtr // Dereference to get the sample value

		if s.t.After(cutoff) {
			// Basic sanity checks for invalid data
			if math.IsNaN(s.p) || math.IsInf(s.p, 0) || s.p < 0 {
				if metrics != nil {
					metrics.FeatureErrorsInc()
				}
				return
			}
			if math.IsNaN(s.v) || math.IsInf(s.v, 0) || s.v < 0 {
				if metrics != nil {
					metrics.FeatureErrorsInc()
				}
				return
			}

			pv += s.p * s.v
			vv += s.v
			validSamples = append(validSamples, s) // Collect valid samples
			count++
		}
	})

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
	var weightedVariance float64
	for _, s := range validSamples {
		deviation := s.p - value
		weightedVariance += s.v * deviation * deviation
	}

	// Denominator for weighted variance should be sum of weights (volumes)
	// However, standard formula for weighted sample variance can be complex.
	// A common approach for weighted variance: sum(w_i * (x_i - mu_w)^2) / sum(w_i)
	// Or, if considering Bessel's correction: sum(w_i * (x_i - mu_w)^2) / ( (M-1)/M * sum(w_i) ) where M is number of non-zero weights.
	// For simplicity, using sum(w_i * (x_i - mu_w)^2) / sum(w_i)
	if vv > 0 { // vv is sum of volumes (weights)
		variance := weightedVariance / vv
		// Guard against negative variance (numerical precision issues)
		if variance > 0 {
			std = math.Sqrt(variance)
		} else {
			if variance < 0 && metrics != nil {
				metrics.FeatureErrorsInc() // Log if variance is negative
			}
			std = 0
		}
	} else {
		std = 0 // Should not happen if vv > 0 check passed earlier, but as a safeguard
	}

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

	if metrics != nil {
		metrics.FeatureCalcDuration(time.Since(startTime))
	}

	return
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
	// Reset ring pointer to the beginning (optional, but good practice)
	// v.ring = v.ring.Move(-v.ring.Len() +1) // This might be tricky if ring is not fully populated.
	// Simpler: just reset currentSize. The existing logic in AddWithMetrics
	// will overwrite old values or nil values correctly.
	v.currentSize = 0
	// To be absolutely sure the ring is clean for Do iteration:
	v.ring = ring.New(v.maxSize) // Re-initialize the ring
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
