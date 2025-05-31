package features

import (
	"math"
	"testing"
)

func TestDepthImb_ValidInputs(t *testing.T) {
	testCases := []struct {
		name     string
		bid      float64
		ask      float64
		expected float64
	}{
		{"balanced book", 100.0, 100.0, 0.0},
		{"bid heavy", 150.0, 100.0, 0.2},  // (150-100)/(150+100) = 50/250 = 0.2
		{"ask heavy", 100.0, 150.0, -0.2}, // (100-150)/(100+150) = -50/250 = -0.2
		{"zero ask", 100.0, 0.0, 1.0},
		{"zero bid", 0.0, 100.0, -1.0},
		{"small values", 0.001, 0.002, -0.3333333333333333},
		{"large values", 1e6, 2e6, -0.3333333333333333},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := DepthImb(tc.bid, tc.ask)
			if math.Abs(result-tc.expected) > 1e-10 {
				t.Errorf("Expected %.10f, got %.10f", tc.expected, result)
			}
		})
	}
}

func TestDepthImb_ZeroBidAndAsk(t *testing.T) {
	result := DepthImb(0.0, 0.0)
	expected := 0.0

	if result != expected {
		t.Errorf("Expected %f for zero bid and ask, got %f", expected, result)
	}
}

func TestDepthImb_InvalidInputs(t *testing.T) {
	metrics := &MockMetricsTracker{}

	testCases := []struct {
		name string
		bid  float64
		ask  float64
	}{
		{"NaN bid", math.NaN(), 100.0},
		{"NaN ask", 100.0, math.NaN()},
		{"Inf bid", math.Inf(1), 100.0},
		{"Inf ask", 100.0, math.Inf(-1)},
		{"negative bid", -50.0, 100.0},
		{"negative ask", 100.0, -50.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initialErrors := metrics.FeatureErrorsIncCalled
			result := DepthImbWithMetrics(tc.bid, tc.ask, metrics)

			// Should return 0 for invalid inputs
			if result != 0.0 {
				t.Errorf("Expected 0 for invalid input, got %f", result)
			}

			// Should track error
			if metrics.FeatureErrorsIncCalled <= initialErrors {
				t.Error("Expected error to be tracked for invalid input")
			}
		})
	}
}

func TestDepthImbWithMetrics_ValidInputs(t *testing.T) {
	metrics := &MockMetricsTracker{}

	result := DepthImbWithMetrics(150.0, 100.0, metrics)
	expected := 0.2

	if math.Abs(result-expected) > 1e-10 {
		t.Errorf("Expected %.10f, got %.10f", expected, result)
	}

	// No errors should be tracked for valid inputs
	if metrics.FeatureErrorsIncCalled != 0 {
		t.Errorf("Expected no errors for valid input, got %d", metrics.FeatureErrorsIncCalled)
	}
}

func TestDepthImbWithMetrics_NilMetrics(t *testing.T) {
	// Should not panic with nil metrics
	result := DepthImbWithMetrics(150.0, 100.0, nil)
	expected := 0.2

	if math.Abs(result-expected) > 1e-10 {
		t.Errorf("Expected %.10f, got %.10f", expected, result)
	}

	// Test with invalid input and nil metrics (should not panic)
	result = DepthImbWithMetrics(math.NaN(), 100.0, nil)
	if result != 0.0 {
		t.Errorf("Expected 0 for invalid input with nil metrics, got %f", result)
	}
}

func TestTickImb_BasicFunctionality(t *testing.T) {
	tickImb := NewTickImb(5)

	// Add some ticks
	tickImb.Add(1)  // buy
	tickImb.Add(1)  // buy
	tickImb.Add(-1) // sell
	tickImb.Add(1)  // buy

	ratio := tickImb.Ratio()
	expected := float64(1+1-1+1) / 4.0 // 2/4 = 0.5

	if math.Abs(ratio-expected) > 1e-10 {
		t.Errorf("Expected ratio %.10f, got %.10f", expected, ratio)
	}
}

func TestTickImb_EmptyBuffer(t *testing.T) {
	tickImb := NewTickImb(5)

	ratio := tickImb.Ratio()
	expected := 0.0

	if ratio != expected {
		t.Errorf("Expected ratio %f for empty buffer, got %f", expected, ratio)
	}
}

func TestTickImb_MaxSize(t *testing.T) {
	maxSize := 3
	tickImb := NewTickImb(maxSize)

	// Add more ticks than max size
	ticks := []int8{1, -1, 1, -1, 1} // 5 ticks, but only last 3 should be kept
	for _, tick := range ticks {
		tickImb.Add(tick)
	}

	ratio := tickImb.Ratio()
	// Should only consider last 3: [1, -1, 1] = (1-1+1)/3 = 1/3
	expected := float64(1) / 3.0

	if math.Abs(ratio-expected) > 1e-10 {
		t.Errorf("Expected ratio %.10f (last %d ticks), got %.10f", expected, maxSize, ratio)
	}
}

func TestTickImb_AllBuys(t *testing.T) {
	tickImb := NewTickImb(5)

	for i := 0; i < 4; i++ {
		tickImb.Add(1)
	}

	ratio := tickImb.Ratio()
	expected := 1.0 // All buys

	if ratio != expected {
		t.Errorf("Expected ratio %f for all buys, got %f", expected, ratio)
	}
}

func TestTickImb_AllSells(t *testing.T) {
	tickImb := NewTickImb(5)

	for i := 0; i < 4; i++ {
		tickImb.Add(-1)
	}

	ratio := tickImb.Ratio()
	expected := -1.0 // All sells

	if ratio != expected {
		t.Errorf("Expected ratio %f for all sells, got %f", expected, ratio)
	}
}

func TestTickImb_ZeroTicks(t *testing.T) {
	tickImb := NewTickImb(5)

	// Add some zero ticks (neutral)
	tickImb.Add(0)
	tickImb.Add(0)
	tickImb.Add(0)

	ratio := tickImb.Ratio()
	expected := 0.0

	if ratio != expected {
		t.Errorf("Expected ratio %f for neutral ticks, got %f", expected, ratio)
	}
}

func TestTickImb_MixedTicks(t *testing.T) {
	tickImb := NewTickImb(10)

	// Add mixed pattern: 3 buys, 2 sells, 1 neutral
	ticks := []int8{1, 1, 1, -1, -1, 0}
	for _, tick := range ticks {
		tickImb.Add(tick)
	}

	ratio := tickImb.Ratio()
	expected := float64(1+1+1-1-1+0) / 6.0 // 1/6

	if math.Abs(ratio-expected) > 1e-10 {
		t.Errorf("Expected ratio %.10f, got %.10f", expected, ratio)
	}
}

func TestTickImb_Concurrency(t *testing.T) {
	tickImb := NewTickImb(1000)

	// Test concurrent access
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				tick := int8((j % 3) - 1) // -1, 0, 1
				tickImb.Add(tick)

				// Occasionally read ratio
				if j%10 == 0 {
					tickImb.Ratio()
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic and should produce reasonable results
	ratio := tickImb.Ratio()
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		t.Errorf("Concurrent access produced invalid ratio: %f", ratio)
	}
	if ratio < -1.0 || ratio > 1.0 {
		t.Errorf("Ratio should be between -1 and 1, got %f", ratio)
	}
}

func TestTickImb_ZeroMaxSize(t *testing.T) {
	tickImb := NewTickImb(0)

	tickImb.Add(1)
	ratio := tickImb.Ratio()

	// With zero max size, buffer should remain empty
	if ratio != 0.0 {
		t.Errorf("Expected ratio 0 for zero max size, got %f", ratio)
	}
}

func TestTickImb_SingleElement(t *testing.T) {
	tickImb := NewTickImb(1)

	tickImb.Add(1)
	ratio1 := tickImb.Ratio()
	if ratio1 != 1.0 {
		t.Errorf("Expected ratio 1.0 for single buy tick, got %f", ratio1)
	}

	// Add another tick, should replace the first
	tickImb.Add(-1)
	ratio2 := tickImb.Ratio()
	if ratio2 != -1.0 {
		t.Errorf("Expected ratio -1.0 for single sell tick, got %f", ratio2)
	}
}

func BenchmarkDepthImb(b *testing.B) {
	bid := 100.0
	ask := 95.0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DepthImb(bid+float64(i%10)*0.1, ask+float64(i%5)*0.1)
	}
}

func BenchmarkDepthImbWithMetrics(b *testing.B) {
	metrics := &MockMetricsTracker{}
	bid := 100.0
	ask := 95.0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DepthImbWithMetrics(bid+float64(i%10)*0.1, ask+float64(i%5)*0.1, metrics)
	}
}

func BenchmarkTickImb_Add(b *testing.B) {
	tickImb := NewTickImb(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tick := int8((i % 3) - 1) // -1, 0, 1
		tickImb.Add(tick)
	}
}

func BenchmarkTickImb_Ratio(b *testing.B) {
	tickImb := NewTickImb(1000)

	// Prepopulate
	for i := 0; i < 500; i++ {
		tick := int8((i % 3) - 1)
		tickImb.Add(tick)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tickImb.Ratio()
	}
}

func BenchmarkTickImb_AddConcurrent(b *testing.B) {
	tickImb := NewTickImb(1000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := int8(0)
		for pb.Next() {
			tick := (i % 3) - 1 // -1, 0, 1
			tickImb.Add(tick)
			i++
		}
	})
}

func BenchmarkTickImb_RatioConcurrent(b *testing.B) {
	tickImb := NewTickImb(1000)

	// Pre-populate
	for i := 0; i < 500; i++ {
		tick := int8((i % 3) - 1)
		tickImb.Add(tick)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tickImb.Ratio()
		}
	})
}

func BenchmarkDepthImb_VariousInputs(b *testing.B) {
	// Pre-generate varied inputs to avoid allocation in hot loop
	bids := make([]float64, b.N)
	asks := make([]float64, b.N)
	for i := 0; i < b.N; i++ {
		bids[i] = 100.0 + float64(i%1000)*0.01
		asks[i] = 95.0 + float64(i%800)*0.01
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DepthImb(bids[i], asks[i])
	}
}
