package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"

	"bitunix-bot/internal/ml"
)

func main() {
	// Test ML predictor integration
	fmt.Println("🧪 Testing ML Model Integration")
	fmt.Println("==============================")

	// Get model path from args or use default
	modelPath := "models/model.onnx"
	if len(os.Args) > 1 {
		modelPath = os.Args[1]
	}

	absPath, err := filepath.Abs(modelPath)
	if err != nil {
		log.Fatalf("❌ Failed to get absolute path: %v", err)
	}

	fmt.Printf("📁 Model path: %s\n", absPath)

	// Test 1: Create predictor
	fmt.Println("\n🔧 Test 1: Creating ML Predictor...")
	predictor, err := ml.New(absPath)
	if err != nil {
		log.Fatalf("❌ Failed to create predictor: %v", err)
	}
	fmt.Println("✅ Predictor created successfully")

	// Test 2: Test with sample features
	fmt.Println("\n🔧 Test 2: Testing prediction with sample features...")
	testCases := []struct {
		name     string
		features []float32
		expected string
	}{
		{
			name:     "Strong reversal signal",
			features: []float32{0.8, -0.6, 1.2}, // high tick imbalance, depth imbalance, price deviation
			expected: "Should likely approve trade",
		},
		{
			name:     "Weak signal",
			features: []float32{0.1, 0.05, 0.2}, // low imbalances
			expected: "Should likely reject trade",
		},
		{
			name:     "High price deviation",
			features: []float32{0.3, -0.4, 3.0}, // price too far from VWAP
			expected: "Should reject due to high price deviation",
		},
		{
			name:     "Mixed signals",
			features: []float32{-0.2, 0.7, -0.8}, // conflicting signals
			expected: "Mixed result",
		},
	}

	for i, tc := range testCases {
		fmt.Printf("\n  Test 2.%d: %s\n", i+1, tc.name)
		fmt.Printf("    Features: [%.3f, %.3f, %.3f]\n", tc.features[0], tc.features[1], tc.features[2])

		// Test Approve method
		thresholds := []float64{0.5, 0.65, 0.8}
		for _, threshold := range thresholds {
			approved := predictor.Approve(tc.features, threshold)
			status := "❌ REJECT"
			if approved {
				status = "✅ APPROVE"
			}
			fmt.Printf("    Threshold %.2f: %s\n", threshold, status)
		}

		// Test Predict method
		predictions, err := predictor.Predict(tc.features)
		if err != nil {
			fmt.Printf("    ❌ Prediction failed: %v\n", err)
		} else {
			fmt.Printf("    📊 Predictions: [%.4f, %.4f]\n", predictions[0], predictions[1])
			fmt.Printf("    📈 Reversal probability: %.4f\n", predictions[1])
		}

		fmt.Printf("    💡 Expected: %s\n", tc.expected)
	}

	// Test 3: Stress test with many predictions
	fmt.Println("\n🔧 Test 3: Stress testing with multiple predictions...")

	approvalCount := 0
	totalTests := 100

	for i := 0; i < totalTests; i++ {
		// Generate random-ish features
		features := []float32{
			float32((i%20 - 10)) / 10.0, // tick ratio: -1.0 to 1.0
			float32((i%15 - 7)) / 7.0,   // depth ratio: -1.0 to 1.0
			float32((i%10 - 5)) / 2.5,   // price dist: -2.0 to 2.0
		}

		if predictor.Approve(features, 0.65) {
			approvalCount++
		}
	}

	approvalRate := float64(approvalCount) / float64(totalTests) * 100
	fmt.Printf("  📊 Approval rate: %.1f%% (%d/%d)\n", approvalRate, approvalCount, totalTests)

	if approvalRate > 80 {
		fmt.Println("  ⚠️  Warning: Very high approval rate - model may be too permissive")
	} else if approvalRate < 10 {
		fmt.Println("  ⚠️  Warning: Very low approval rate - model may be too restrictive")
	} else {
		fmt.Println("  ✅ Approval rate looks reasonable")
	}

	// Test 4: Edge cases
	fmt.Println("\n🔧 Test 4: Testing edge cases...")

	edgeCases := []struct {
		name     string
		features []float32
	}{
		{"Empty features", []float32{}},
		{"Too few features", []float32{0.5}},
		{"Too many features", []float32{0.1, 0.2, 0.3, 0.4, 0.5}},
		{"NaN values", []float32{float32(math.NaN()), 0.5, 0.3}},
		{"Extreme values", []float32{1000.0, -1000.0, 999.9}},
	}

	for i, tc := range edgeCases {
		fmt.Printf("  Test 4.%d: %s\n", i+1, tc.name)

		// Test should not crash
		approved := predictor.Approve(tc.features, 0.65)
		predictions, err := predictor.Predict(tc.features)

		if err != nil {
			fmt.Printf("    ❌ Error (expected): %v\n", err)
		} else {
			fmt.Printf("    ✅ No error for edge case %d, approved: %v, predictions: %v\n", i, approved, predictions)
		}
	}

	fmt.Println("\n🎉 All tests completed!")
	fmt.Println("========================")
	fmt.Println("💡 Integration tips:")
	fmt.Println("  - Monitor prediction latency in production")
	fmt.Println("  - Set appropriate approval thresholds based on risk tolerance")
	fmt.Println("  - Regularly retrain model with fresh data")
	fmt.Println("  - Consider fallback behavior when model is unavailable")
}
