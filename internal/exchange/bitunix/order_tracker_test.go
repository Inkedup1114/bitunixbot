package bitunix

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockMetrics implements MetricsInterface for testing
type MockMetrics struct {
	mu                 sync.RWMutex
	timeouts           int
	retries            int
	executionDurations []float64
}

func (m *MockMetrics) OrderTimeoutsInc() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timeouts++
}

func (m *MockMetrics) OrderRetriesInc() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retries++
}

func (m *MockMetrics) OrderExecutionDurationObserve(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executionDurations = append(m.executionDurations, v)
}

func (m *MockMetrics) GetTimeouts() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.timeouts
}

func (m *MockMetrics) GetRetries() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.retries
}

func (m *MockMetrics) GetExecutionDurationsCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.executionDurations)
}

// MockClient implements a failing client for testing
type MockClient struct {
	shouldFail  bool
	failCount   int
	maxFailures int
	placeCalls  int
	mu          sync.RWMutex
}

func (m *MockClient) Place(req OrderReq) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.placeCalls++

	if m.shouldFail && m.failCount < m.maxFailures {
		m.failCount++
		return fmt.Errorf("mock error %d", m.failCount)
	}

	return nil
}

func (m *MockClient) GetPlaceCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.placeCalls
}

func TestOrderTracker_BasicFunctionality(t *testing.T) {
	mockClient := &MockClient{}
	metrics := &MockMetrics{}

	tracker := NewOrderTracker(mockClient, 30*time.Second, 1*time.Second, 3)
	tracker.SetMetrics(metrics)
	defer tracker.Stop()

	req := OrderReq{
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		TradeSide: "OPEN",
		Qty:       "0.1",
		OrderType: "MARKET",
	}

	err := tracker.PlaceOrderWithTimeout(req)
	assert.NoError(t, err)

	// Check that metrics were recorded
	assert.Equal(t, 1, metrics.GetExecutionDurationsCount())
	assert.Equal(t, 0, metrics.GetTimeouts())
	assert.Equal(t, 0, metrics.GetRetries())

	// Check that the order was placed
	assert.Equal(t, 1, mockClient.GetPlaceCalls())
}

func TestOrderTracker_OrderRetries(t *testing.T) {
	mockClient := &MockClient{
		shouldFail:  true,
		maxFailures: 2, // Fail twice, then succeed
	}
	metrics := &MockMetrics{}

	tracker := NewOrderTracker(mockClient, 30*time.Second, 1*time.Second, 3)
	tracker.SetMetrics(metrics)
	defer tracker.Stop()

	req := OrderReq{
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		TradeSide: "OPEN",
		Qty:       "0.1",
		OrderType: "MARKET",
	}

	err := tracker.PlaceOrderWithTimeout(req)
	assert.NoError(t, err)

	// Check that retries were recorded
	assert.Equal(t, 2, metrics.GetRetries())       // 2 retries before success
	assert.Equal(t, 3, mockClient.GetPlaceCalls()) // 1 initial + 2 retries
	assert.Equal(t, 1, metrics.GetExecutionDurationsCount())
}

func TestOrderTracker_OrderRetriesExhausted(t *testing.T) {
	mockClient := &MockClient{
		shouldFail:  true,
		maxFailures: 10, // Always fail
	}
	metrics := &MockMetrics{}

	tracker := NewOrderTracker(mockClient, 30*time.Second, 1*time.Second, 3)
	tracker.SetMetrics(metrics)
	defer tracker.Stop()

	req := OrderReq{
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		TradeSide: "OPEN",
		Qty:       "0.1",
		OrderType: "MARKET",
	}

	err := tracker.PlaceOrderWithTimeout(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 3 retries")

	// Check that all retries were attempted
	assert.Equal(t, 3, metrics.GetRetries())       // 3 retries
	assert.Equal(t, 4, mockClient.GetPlaceCalls()) // 1 initial + 3 retries
}

func TestOrderTracker_OrderTimeout(t *testing.T) {
	mockClient := &MockClient{}
	metrics := &MockMetrics{}

	// Very short timeout for testing
	tracker := NewOrderTracker(mockClient, 100*time.Millisecond, 50*time.Millisecond, 3)
	tracker.SetMetrics(metrics)
	defer tracker.Stop()

	req := OrderReq{
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		TradeSide: "OPEN",
		Qty:       "0.1",
		OrderType: "MARKET",
	}

	// Place order successfully
	err := tracker.PlaceOrderWithTimeout(req)
	assert.NoError(t, err)

	// Wait for timeout to trigger
	time.Sleep(200 * time.Millisecond)

	// Check that timeout was recorded
	require.Eventually(t, func() bool {
		return metrics.GetTimeouts() > 0
	}, 1*time.Second, 50*time.Millisecond, "Expected timeout to be recorded")
}

func TestOrderTracker_GetPendingOrders(t *testing.T) {
	mockClient := &MockClient{}
	tracker := NewOrderTracker(mockClient, 30*time.Second, 1*time.Second, 3)
	defer tracker.Stop()

	req1 := OrderReq{
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		TradeSide: "OPEN",
		Qty:       "0.1",
		OrderType: "MARKET",
	}

	req2 := OrderReq{
		Symbol:    "ETHUSDT",
		Side:      "SELL",
		TradeSide: "OPEN",
		Qty:       "1.0",
		OrderType: "MARKET",
	}

	// Place two orders
	err1 := tracker.PlaceOrderWithTimeout(req1)
	assert.NoError(t, err1)

	err2 := tracker.PlaceOrderWithTimeout(req2)
	assert.NoError(t, err2)

	// Check pending orders
	pendingOrders := tracker.GetPendingOrders()
	assert.Len(t, pendingOrders, 2)

	// Verify order details
	symbols := make(map[string]bool)
	for _, order := range pendingOrders {
		symbols[order.Symbol] = true
		assert.Equal(t, OrderStatusPending, order.Status)
	}

	assert.True(t, symbols["BTCUSDT"])
	assert.True(t, symbols["ETHUSDT"])
}

func TestOrderTracker_GetAllOrders(t *testing.T) {
	mockClient := &MockClient{}
	tracker := NewOrderTracker(mockClient, 30*time.Second, 1*time.Second, 3)
	defer tracker.Stop()

	req := OrderReq{
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		TradeSide: "OPEN",
		Qty:       "0.1",
		OrderType: "MARKET",
	}

	err := tracker.PlaceOrderWithTimeout(req)
	assert.NoError(t, err)

	allOrders := tracker.GetAllOrders()
	assert.Len(t, allOrders, 1)

	for _, order := range allOrders {
		assert.Equal(t, "BTCUSDT", order.Symbol)
		assert.Equal(t, "BUY", order.Side)
		assert.Equal(t, "0.1", order.Quantity)
		assert.Equal(t, OrderStatusPending, order.Status)
	}
}

func TestOrderTracker_ConcurrentOrderPlacement(t *testing.T) {
	mockClient := &MockClient{}
	metrics := &MockMetrics{}

	tracker := NewOrderTracker(mockClient, 30*time.Second, 1*time.Second, 3)
	tracker.SetMetrics(metrics)
	defer tracker.Stop()

	var wg sync.WaitGroup
	numOrders := 10

	// Place multiple orders concurrently
	for i := 0; i < numOrders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			req := OrderReq{
				Symbol:    fmt.Sprintf("SYM%d", i),
				Side:      "BUY",
				TradeSide: "OPEN",
				Qty:       "0.1",
				OrderType: "MARKET",
			}

			err := tracker.PlaceOrderWithTimeout(req)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Check that all orders were placed
	assert.Equal(t, numOrders, mockClient.GetPlaceCalls())
	assert.Equal(t, numOrders, metrics.GetExecutionDurationsCount())

	// Check that all orders are tracked
	allOrders := tracker.GetAllOrders()
	assert.Len(t, allOrders, numOrders)
}

func TestOrderTracker_MetricsWithoutInterface(t *testing.T) {
	mockClient := &MockClient{}

	// Create tracker without metrics
	tracker := NewOrderTracker(mockClient, 30*time.Second, 1*time.Second, 3)
	defer tracker.Stop()

	req := OrderReq{
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		TradeSide: "OPEN",
		Qty:       "0.1",
		OrderType: "MARKET",
	}

	// Should not panic even without metrics
	err := tracker.PlaceOrderWithTimeout(req)
	assert.NoError(t, err)

	assert.Equal(t, 1, mockClient.GetPlaceCalls())
}

func TestOrderTracker_Stop(t *testing.T) {
	mockClient := &MockClient{}
	tracker := NewOrderTracker(mockClient, 30*time.Second, 1*time.Second, 3)

	// Place an order
	req := OrderReq{
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		TradeSide: "OPEN",
		Qty:       "0.1",
		OrderType: "MARKET",
	}

	err := tracker.PlaceOrderWithTimeout(req)
	assert.NoError(t, err)

	// Stop should complete without blocking
	done := make(chan struct{})
	go func() {
		tracker.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() blocked for too long")
	}
}
