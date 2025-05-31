package features

import (
	"math"
	"sync"
	"testing"
	"time"
)

// MockMetricsTracker is a mock implementation of MetricsTracker for testing.
type MockMetricsTracker struct {
	ErrorsInvoked          bool
	CalcDurationInvoked    bool
	SampleCountInvoked     bool
	LastCalcDuration       time.Duration
	LastSampleCount        int
	FeatureErrorsIncCalled int
}

func (m *MockMetricsTracker) FeatureErrorsInc() {
	m.ErrorsInvoked = true
	m.FeatureErrorsIncCalled++
}

func (m *MockMetricsTracker) FeatureCalcDuration(duration time.Duration) {
	m.CalcDurationInvoked = true
	m.LastCalcDuration = duration
}

func (m *MockMetricsTracker) FeatureSampleCount(count int) {
	m.SampleCountInvoked = true
	m.LastSampleCount = count
}

func TestVWAP_NewVWAP(t *testing.T) {
	t.Parallel()

	// Test with valid inputs
	v := NewVWAP(time.Minute, 10)
	if v == nil {
		t.Fatal("NewVWAP returned nil for valid inputs")
	}
	if v.win != time.Minute { // Corrected: interval to win
		t.Errorf("Expected window %v, got %v", time.Minute, v.win)
	}
	if v.maxSize != 10 { // Corrected: size to maxSize
		t.Errorf("Expected maxSize %d, got %d", 10, v.maxSize)
	}
	if v.currentSize != 0 {
		t.Errorf("Expected currentSize %d, got %d", 0, v.currentSize)
	}
	if v.ring.Len() != 10 {
		t.Errorf("Expected ring length %d, got %d", 10, v.ring.Len())
	}
	if v.samplePool == nil {
		t.Error("samplePool should not be nil")
	}

	// Test with zero size
	v = NewVWAP(time.Minute, 0)
	if v.maxSize != 1 { // Corrected: size to maxSize (assuming default behavior)
		t.Errorf("Expected maxSize %d for zero input, got %d", 1, v.maxSize)
	}

	// Test with negative size
	v = NewVWAP(time.Minute, -5)
	if v.maxSize != 1 { // Corrected: size to maxSize (assuming default behavior)
		t.Errorf("Expected maxSize %d for negative input, got %d", 1, v.maxSize)
	}

	// Test with zero time window
	v = NewVWAP(0, 10)
	if v.win != time.Minute { // Corrected: interval to win (assuming default behavior)
		t.Errorf("Expected window %v for zero input, got %v", time.Minute, v.win)
	}

	// Test with negative time window
	v = NewVWAP(-time.Second, 10)
	if v.win != time.Minute { // Corrected: interval to win (assuming default behavior)
		t.Errorf("Expected window %v for negative input, got %v", time.Minute, v.win)
	}
}

func TestVWAP_Add(t *testing.T) {
	t.Parallel()
	v := NewVWAP(time.Minute, 3)
	mockMetrics := &MockMetricsTracker{}

	// Add first sample
	v.AddWithMetrics(10.0, 1.0, mockMetrics)
	if v.GetCurrentSize() != 1 {
		t.Errorf("Expected currentSize 1, got %d", v.GetCurrentSize())
	}

	// Add more samples to fill the ring
	v.AddWithMetrics(11.0, 2.0, mockMetrics)
	v.AddWithMetrics(12.0, 3.0, mockMetrics)
	if v.GetCurrentSize() != 3 {
		t.Errorf("Expected currentSize 3, got %d", v.GetCurrentSize())
	}

	// Add another sample, should overwrite the oldest
	v.AddWithMetrics(13.0, 4.0, mockMetrics)
	if v.GetCurrentSize() != 3 {
		t.Errorf("Expected currentSize 3 after overwrite, got %d", v.GetCurrentSize())
	}

	// Test Add (without metrics)
	v.Add(14.0, 5.0)
	if v.GetCurrentSize() != 3 {
		t.Errorf("Expected currentSize 3 after Add, got %d", v.GetCurrentSize())
	}

	// Test invalid inputs
	initialErrors := mockMetrics.FeatureErrorsIncCalled
	v.AddWithMetrics(math.NaN(), 1.0, mockMetrics)
	if mockMetrics.FeatureErrorsIncCalled <= initialErrors {
		t.Error("Expected FeatureErrorsInc to be called for NaN price")
	}

	initialErrors = mockMetrics.FeatureErrorsIncCalled
	v.AddWithMetrics(10.0, math.Inf(1), mockMetrics)
	if mockMetrics.FeatureErrorsIncCalled <= initialErrors {
		t.Error("Expected FeatureErrorsInc to be called for Inf volume")
	}

	initialErrors = mockMetrics.FeatureErrorsIncCalled
	v.AddWithMetrics(-1.0, 1.0, mockMetrics)
	if mockMetrics.FeatureErrorsIncCalled <= initialErrors {
		t.Error("Expected FeatureErrorsInc to be called for negative price")
	}

	initialErrors = mockMetrics.FeatureErrorsIncCalled
	v.AddWithMetrics(1.0, -1.0, mockMetrics)
	if mockMetrics.FeatureErrorsIncCalled <= initialErrors {
		t.Error("Expected FeatureErrorsInc to be called for negative volume")
	}
}

func TestVWAP_Calc(t *testing.T) {
	t.Parallel()
	v := NewVWAP(time.Minute, 5)
	mockMetrics := &MockMetricsTracker{}

	// Test empty VWAP
	val, std := v.CalcWithMetrics(mockMetrics)
	if val != 0 || std != 0 {
		t.Errorf("Expected 0, 0 for empty VWAP, got %.2f, %.2f", val, std)
	}
	if !mockMetrics.CalcDurationInvoked {
		t.Error("CalcDuration should be invoked for empty VWAP")
	}
	if !mockMetrics.SampleCountInvoked || mockMetrics.LastSampleCount != 0 {
		t.Errorf("SampleCount should be 0 for empty VWAP, got %d", mockMetrics.LastSampleCount)
	}

	// Add some data
	v.Add(10.0, 1.0)                  // t0
	time.Sleep(10 * time.Millisecond) // Ensure time difference
	v.Add(11.0, 2.0)                  // t1
	time.Sleep(10 * time.Millisecond)
	v.Add(12.0, 3.0) // t2

	mockMetrics = &MockMetricsTracker{} // Reset metrics
	val, std = v.CalcWithMetrics(mockMetrics)

	expectedVal := (10.0*1.0 + 11.0*2.0 + 12.0*3.0) / (1.0 + 2.0 + 3.0)
	if math.Abs(val-expectedVal) > 1e-9 {
		t.Errorf("Expected VWAP value %.2f, got %.2f", expectedVal, val)
	}

	// Expected standard deviation calculation
	mean := expectedVal
	weightedVariance := 1.0*math.Pow(10.0-mean, 2) + 2.0*math.Pow(11.0-mean, 2) + 3.0*math.Pow(12.0-mean, 2)
	expectedStd := math.Sqrt(weightedVariance / (1.0 + 2.0 + 3.0))
	if math.Abs(std-expectedStd) > 1e-9 {
		t.Errorf("Expected VWAP std %.2f, got %.2f", expectedStd, std)
	}
	if !mockMetrics.CalcDurationInvoked {
		t.Error("CalcDuration should be invoked")
	}
	if !mockMetrics.SampleCountInvoked || mockMetrics.LastSampleCount != 3 {
		t.Errorf("SampleCount should be 3, got %d", mockMetrics.LastSampleCount)
	}

	// Test Calc (without metrics)
	valNoMetrics, stdNoMetrics := v.Calc()
	if math.Abs(valNoMetrics-expectedVal) > 1e-9 {
		t.Errorf("Expected VWAP value %.2f (no metrics), got %.2f", expectedVal, valNoMetrics)
	}
	if math.Abs(stdNoMetrics-expectedStd) > 1e-9 {
		t.Errorf("Expected VWAP std %.2f (no metrics), got %.2f", expectedStd, stdNoMetrics)
	}

	// Test with single sample
	vSingle := NewVWAP(time.Minute, 1)
	vSingle.Add(100, 5)
	mockMetrics = &MockMetricsTracker{}
	valSingle, stdSingle := vSingle.CalcWithMetrics(mockMetrics)
	if valSingle != 100 || stdSingle != 0 {
		t.Errorf("Expected 100, 0 for single sample VWAP, got %.2f, %.2f", valSingle, stdSingle)
	}
	if !mockMetrics.CalcDurationInvoked {
		t.Error("CalcDuration should be invoked for single sample")
	}
	if !mockMetrics.SampleCountInvoked || mockMetrics.LastSampleCount != 1 {
		t.Errorf("SampleCount should be 1 for single sample, got %d", mockMetrics.LastSampleCount)
	}

	// Test with zero volume in samples (should result in 0,0 and not panic)
	vZeroVol := NewVWAP(time.Minute, 2)
	vZeroVol.Add(10, 0)
	vZeroVol.Add(20, 0)
	mockMetrics = &MockMetricsTracker{}
	valZero, stdZero := vZeroVol.CalcWithMetrics(mockMetrics)
	if valZero != 0 || stdZero != 0 {
		t.Errorf("Expected 0,0 for zero volume samples, got %.2f, %.2f", valZero, stdZero)
	}
	if !mockMetrics.CalcDurationInvoked {
		t.Error("CalcDuration should be invoked for zero volume")
	}
	if !mockMetrics.SampleCountInvoked || mockMetrics.LastSampleCount != 2 {
		t.Errorf("SampleCount should be 2 for zero volume, got %d", mockMetrics.LastSampleCount)
	}
}

func TestVWAP_TimeWindowExpiration(t *testing.T) {
	t.Parallel()
	win := 50 * time.Millisecond
	v := NewVWAP(win, 3)
	mockMetrics := &MockMetricsTracker{}

	v.AddWithMetrics(10.0, 1.0, mockMetrics) // s1
	time.Sleep(win / 2)                      // Wait for half the window
	v.AddWithMetrics(11.0, 2.0, mockMetrics) // s2

	val, std := v.CalcWithMetrics(mockMetrics)
	expectedVal := (10.0*1.0 + 11.0*2.0) / (1.0 + 2.0)
	if math.Abs(val-expectedVal) > 1e-9 {
		t.Errorf("Expected VWAP value %.2f before expiration, got %.2f", expectedVal, val)
	}
	if mockMetrics.LastSampleCount != 2 {
		t.Errorf("Expected 2 samples before expiration, got %d", mockMetrics.LastSampleCount)
	}

	time.Sleep(win/2 + 10*time.Millisecond)  // Wait for s1 to expire
	v.AddWithMetrics(12.0, 3.0, mockMetrics) // s3, s1 should be out

	mockMetrics = &MockMetricsTracker{} // Reset metrics
	val, std = v.CalcWithMetrics(mockMetrics)
	expectedVal = (11.0*2.0 + 12.0*3.0) / (2.0 + 3.0) // Only s2 and s3
	if math.Abs(val-expectedVal) > 1e-9 {
		t.Errorf("Expected VWAP value %.2f after expiration, got %.2f. Std: %.2f", expectedVal, val, std)
	}
	if mockMetrics.LastSampleCount != 2 {
		t.Errorf("Expected 2 samples after s1 expiration, got %d", mockMetrics.LastSampleCount)
	}
}

func TestVWAP_GetCurrentSize(t *testing.T) {
	t.Parallel()
	v := NewVWAP(time.Minute, 3)

	if v.GetCurrentSize() != 0 {
		t.Errorf("Expected currentSize 0 for new VWAP, got %d", v.GetCurrentSize())
	}

	v.Add(10.0, 1.0)
	if v.GetCurrentSize() != 1 {
		t.Errorf("Expected currentSize 1 after one add, got %d", v.GetCurrentSize())
	}

	v.Add(20.0, 2.0)
	v.Add(30.0, 3.0)
	if v.GetCurrentSize() != 3 {
		t.Errorf("Expected currentSize 3 after adding 3 samples, got %d", v.GetCurrentSize())
	}

	v.Add(40.0, 4.0) // This should overwrite the oldest sample
	if v.GetCurrentSize() != 3 {
		t.Errorf("Expected currentSize 3 after overwrite, got %d", v.GetCurrentSize())
	}
}

func TestVWAP_Reset(t *testing.T) {
	t.Parallel()
	v := NewVWAP(time.Minute, 3)
	mockMetrics := &MockMetricsTracker{}

	v.AddWithMetrics(10.0, 1.0, mockMetrics)
	v.AddWithMetrics(11.0, 2.0, mockMetrics)
	v.Reset()

	if v.GetCurrentSize() != 0 {
		t.Errorf("Expected currentSize 0 after reset, got %d", v.GetCurrentSize())
	}

	val, std := v.CalcWithMetrics(mockMetrics)
	if val != 0 || std != 0 {
		t.Errorf("Expected 0, 0 from Calc after reset, got %.2f, %.2f", val, std)
	}

	// Ensure ring is actually cleared and samples are put back to pool
	// This is harder to test directly without exposing pool internals or ring state
	// But we can check if adding new items works as expected
	v.AddWithMetrics(100.0, 1.0, mockMetrics)
	if v.GetCurrentSize() != 1 {
		t.Errorf("Expected currentSize 1 after reset and add, got %d", v.GetCurrentSize())
	}
	val, std = v.CalcWithMetrics(mockMetrics)
	if val != 100 || std != 0 {
		t.Errorf("Expected 100, 0 from Calc after reset and add, got %.2f, %.2f", val, std)
	}

	// Test resetting an empty VWAP
	vEmpty := NewVWAP(time.Minute, 3)
	vEmpty.Reset()
	if vEmpty.GetCurrentSize() != 0 {
		t.Errorf("Expected currentSize 0 after resetting empty VWAP, got %d", vEmpty.GetCurrentSize())
	}
}

func TestVWAP_ConcurrentAccess(t *testing.T) {
	vwap := NewVWAP(10*time.Second, 1000)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				price := 100.0 + float64(j%10)
				volume := 10.0
				vwap.Add(price, volume)

				// Occasionally calculate VWAP
				if j%10 == 0 {
					vwap.Calc()
				}
			}
		}(i)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Final calculation should not panic and produce a valid result
	value, std := vwap.Calc()
	if math.IsNaN(value) || math.IsInf(value, 0) {
		t.Errorf("Concurrent access produced invalid VWAP: %.4f", value)
	}
	if math.IsNaN(std) || math.IsInf(std, 0) {
		t.Errorf("Concurrent access produced invalid std: %.4f", std)
	}
}

func TestVWAP_InvalidDataInRing(t *testing.T) {
	t.Parallel()
	v := NewVWAP(time.Minute, 3)
	mockMetrics := &MockMetricsTracker{}

	// Manually insert a nil into the ring to simulate a corrupted state (not normally possible with current Add)
	// This test is more about the robustness of Calc if the ring somehow gets bad data.
	sGood := v.samplePool.Get().(*sample)
	sGood.p = 10
	sGood.v = 1
	sGood.t = time.Now()
	v.ring.Value = sGood
	v.ring = v.ring.Next()
	v.currentSize++

	v.ring.Value = nil // Simulate a nil entry
	v.ring = v.ring.Next()
	// v.currentSize++ // If we increment currentSize, Calc might try to process it.
	// For this test, let's assume currentSize reflects actual valid items, but a nil is in the ring path.

	sGood2 := v.samplePool.Get().(*sample)
	sGood2.p = 20
	sGood2.v = 2
	sGood2.t = time.Now()
	v.ring.Value = sGood2
	v.ring = v.ring.Next()
	v.currentSize++

	val, _ := v.CalcWithMetrics(mockMetrics) // Corrected: Ignored std as it's not used

	expectedVal := (10.0*1.0 + 20.0*2.0) / (1.0 + 2.0)
	if math.Abs(val-expectedVal) > 1e-9 {
		t.Errorf("Expected VWAP value %.2f with nil in ring, got %.2f", expectedVal, val)
	}
	if mockMetrics.LastSampleCount != 2 {
		t.Errorf("Expected sample count 2 with nil in ring, got %d", mockMetrics.LastSampleCount)
	}
	if mockMetrics.FeatureErrorsIncCalled > 0 {
		t.Errorf("Expected 0 errors for nil in ring (should be skipped), got %d", mockMetrics.FeatureErrorsIncCalled)
	}

	// Test with non-sample type in ring (also should not happen with current Add)
	vRobust := NewVWAP(time.Minute, 3)
	mockMetricsRobust := &MockMetricsTracker{}
	sGoodRobust := vRobust.samplePool.Get().(*sample)
	sGoodRobust.p = 30
	sGoodRobust.v = 3
	sGoodRobust.t = time.Now()
	vRobust.ring.Value = sGoodRobust
	vRobust.ring = vRobust.ring.Next()
	vRobust.currentSize++

	vRobust.ring.Value = "not a sample ptr" // Put a string instead of *sample
	vRobust.ring = vRobust.ring.Next()
	// vRobust.currentSize++ // Again, assume currentSize is accurate for valid items.

	valRobust, _ := vRobust.CalcWithMetrics(mockMetricsRobust) // Corrected: Ignored std as it's not used
	if math.Abs(valRobust-30.0) > 1e-9 {
		t.Errorf("Expected VWAP value 30.0 with non-sample in ring, got %.2f", valRobust)
	}
	if mockMetricsRobust.LastSampleCount != 1 {
		t.Errorf("Expected sample count 1 with non-sample in ring, got %d", mockMetricsRobust.LastSampleCount)
	}
	if mockMetricsRobust.FeatureErrorsIncCalled > 0 {
		t.Errorf("Expected 0 errors for non-sample in ring (should be skipped), got %d", mockMetricsRobust.FeatureErrorsIncCalled)
	}
}

func TestVWAP_CalcWithMetrics_NegativeVariance(t *testing.T) {
	vwap := NewVWAP(10*time.Second, 100)
	metrics := &MockMetricsTracker{}

	// Create a scenario that might cause numerical precision issues
	// by adding very large numbers with small differences
	basePrice := 1e10
	for i := 0; i < 3; i++ {
		price := basePrice + float64(i)*1e-6
		vwap.Add(price, 1.0)
	}

	value, std := vwap.CalcWithMetrics(metrics)

	// Should handle numerical precision gracefully
	if math.IsNaN(value) || math.IsInf(value, 0) {
		t.Errorf("Numerical precision issues caused invalid VWAP: %.4f", value)
	}
	if math.IsNaN(std) || math.IsInf(std, 0) {
		t.Errorf("Numerical precision issues caused invalid std: %.4f", std)
	}

	// If variance calculation had issues, std should be 0 and errors should be tracked
	if std < 0 {
		t.Errorf("Standard deviation cannot be negative: %.4f", std)
	}
}

// Benchmarks for VWAP performance testing
func BenchmarkVWAP_Add(b *testing.B) {
	v := NewVWAP(time.Minute, 1000)

	// Pre-generate test data to avoid allocation in hot loop
	prices := make([]float64, b.N)
	volumes := make([]float64, b.N)
	for i := 0; i < b.N; i++ {
		prices[i] = 100.0 + float64(i%100)*0.01
		volumes[i] = 1.0 + float64(i%50)*0.1
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Add(prices[i], volumes[i])
	}
}

func BenchmarkVWAP_AddWithMetrics(b *testing.B) {
	v := NewVWAP(time.Minute, 1000)
	metrics := &MockMetricsTracker{}

	// Pre-generate test data to avoid allocation in hot loop
	prices := make([]float64, b.N)
	volumes := make([]float64, b.N)
	for i := 0; i < b.N; i++ {
		prices[i] = 100.0 + float64(i%100)*0.01
		volumes[i] = 1.0 + float64(i%50)*0.1
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.AddWithMetrics(prices[i], volumes[i], metrics)
	}
}

func BenchmarkVWAP_Calc(b *testing.B) {
	v := NewVWAP(time.Minute, 100)

	// Pre-populate with some data
	for i := 0; i < 50; i++ {
		v.Add(100.0+float64(i)*0.1, 1.0+float64(i)*0.05)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Calc()
	}
}

func BenchmarkVWAP_CalcWithMetrics(b *testing.B) {
	v := NewVWAP(time.Minute, 100)
	metrics := &MockMetricsTracker{}

	// Pre-populate with some data
	for i := 0; i < 50; i++ {
		v.Add(100.0+float64(i)*0.1, 1.0+float64(i)*0.05)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.CalcWithMetrics(metrics)
	}
}

func BenchmarkVWAP_ConcurrentAddCalc(b *testing.B) {
	v := NewVWAP(time.Minute, 1000)

	// Pre-populate with some data
	for i := 0; i < 100; i++ {
		v.Add(100.0+float64(i)*0.1, 1.0+float64(i)*0.05)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				// 10% calc operations
				v.Calc()
			} else {
				// 90% add operations
				v.Add(100.0+float64(i%100)*0.01, 1.0+float64(i%50)*0.1)
			}
			i++
		}
	})
}

func BenchmarkVWAP_Reset(b *testing.B) {
	v := NewVWAP(time.Minute, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Add some data then reset
		for j := 0; j < 10; j++ {
			v.Add(100.0+float64(j)*0.1, 1.0)
		}
		v.Reset()
	}
}
