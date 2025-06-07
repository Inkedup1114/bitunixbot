package bitunix

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// MetricsInterface defines the metrics methods needed by the order tracker
type MetricsInterface interface {
	OrderTimeoutsInc()
	OrderRetriesInc()
	OrderExecutionDurationObserve(float64)
}

// PlaceOrderInterface defines the interface for placing orders
type PlaceOrderInterface interface {
	Place(OrderReq) error
}

// OrderStatus represents the status of an order
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "PENDING"
	OrderStatusFilled    OrderStatus = "FILLED"
	OrderStatusCancelled OrderStatus = "CANCELLED"
	OrderStatusRejected  OrderStatus = "REJECTED"
	OrderStatusTimeout   OrderStatus = "TIMEOUT"
)

// TrackedOrder represents an order being tracked
type TrackedOrder struct {
	ID              string
	ClientOrderID   string
	Symbol          string
	Side            string
	Quantity        string
	OrderType       string
	Status          OrderStatus
	SubmittedAt     time.Time
	LastCheckedAt   time.Time
	TimeoutAt       time.Time
	RetryCount      int
	OriginalRequest OrderReq
	ResponseData    interface{}
	Error           error
}

// OrderTracker manages order execution with timeout handling
type OrderTracker struct {
	mu                  sync.RWMutex
	orders              map[string]*TrackedOrder
	client              PlaceOrderInterface
	executionTimeout    time.Duration
	statusCheckInterval time.Duration
	maxRetries          int
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	metrics             MetricsInterface
}

// NewOrderTracker creates a new order tracker
func NewOrderTracker(client PlaceOrderInterface, executionTimeout, statusCheckInterval time.Duration, maxRetries int) *OrderTracker {
	// Safety checks for zero intervals
	if executionTimeout <= 0 {
		executionTimeout = 30 * time.Second // Default execution timeout
	}
	if statusCheckInterval <= 0 {
		statusCheckInterval = 5 * time.Second // Default status check interval
	}
	if maxRetries < 0 {
		maxRetries = 3 // Default max retries
	}

	ctx, cancel := context.WithCancel(context.Background())
	tracker := &OrderTracker{
		orders:              make(map[string]*TrackedOrder),
		client:              client,
		executionTimeout:    executionTimeout,
		statusCheckInterval: statusCheckInterval,
		maxRetries:          maxRetries,
		ctx:                 ctx,
		cancel:              cancel,
		metrics:             nil, // Will be set if available
	}

	// Start the monitoring goroutine
	tracker.wg.Add(1)
	go tracker.monitorOrders()

	return tracker
}

// SetMetrics sets the metrics interface for reporting
func (ot *OrderTracker) SetMetrics(metrics MetricsInterface) {
	ot.mu.Lock()
	defer ot.mu.Unlock()
	ot.metrics = metrics
}

// Stop gracefully stops the order tracker
func (ot *OrderTracker) Stop() {
	ot.cancel()
	ot.wg.Wait()
}

// PlaceOrderWithTimeout places an order and monitors it for timeout
func (ot *OrderTracker) PlaceOrderWithTimeout(req OrderReq) error {
	startTime := time.Now()

	// Generate client order ID
	clientOrderID := uuid.New().String()

	// Create tracked order
	tracked := &TrackedOrder{
		ClientOrderID:   clientOrderID,
		Symbol:          req.Symbol,
		Side:            req.Side,
		Quantity:        req.Qty,
		OrderType:       req.OrderType,
		Status:          OrderStatusPending,
		SubmittedAt:     startTime,
		LastCheckedAt:   startTime,
		TimeoutAt:       startTime.Add(ot.executionTimeout),
		RetryCount:      0,
		OriginalRequest: req,
	}

	// Store the tracked order
	ot.mu.Lock()
	ot.orders[clientOrderID] = tracked
	ot.mu.Unlock()

	// Place the order
	err := ot.placeOrderWithRetry(tracked)

	// Record execution duration
	duration := time.Since(startTime).Seconds()
	ot.mu.RLock()
	metrics := ot.metrics
	ot.mu.RUnlock()

	if metrics != nil {
		metrics.OrderExecutionDurationObserve(duration)
	}

	if err != nil {
		ot.updateOrderStatus(clientOrderID, OrderStatusRejected, err)
		return fmt.Errorf("failed to place order: %w", err)
	}

	log.Info().
		Str("client_order_id", clientOrderID).
		Str("symbol", req.Symbol).
		Str("side", req.Side).
		Str("quantity", req.Qty).
		Time("timeout_at", tracked.TimeoutAt).
		Float64("duration_seconds", duration).
		Msg("Order placed with timeout tracking")

	return nil
}

// placeOrderWithRetry attempts to place an order with retry logic
func (ot *OrderTracker) placeOrderWithRetry(tracked *TrackedOrder) error {
	var lastErr error

	for i := 0; i <= ot.maxRetries; i++ {
		err := ot.client.Place(tracked.OriginalRequest)
		if err == nil {
			return nil
		}

		lastErr = err
		tracked.RetryCount = i + 1 // Track actual retry attempts

		if i < ot.maxRetries {
			// Record retry metric only for actual retries (not the initial attempt)
			ot.mu.RLock()
			metrics := ot.metrics
			ot.mu.RUnlock()

			if metrics != nil {
				metrics.OrderRetriesInc()
			}

			retryDelay := time.Duration(i+1) * time.Second
			log.Warn().
				Err(err).
				Str("client_order_id", tracked.ClientOrderID).
				Int("retry", i+1).
				Dur("delay", retryDelay).
				Msg("Order placement failed, retrying")

			time.Sleep(retryDelay)
		}
	}

	return fmt.Errorf("order placement failed after %d retries: %w", ot.maxRetries, lastErr)
}

// monitorOrders continuously monitors pending orders for timeout
func (ot *OrderTracker) monitorOrders() {
	defer ot.wg.Done()

	ticker := time.NewTicker(ot.statusCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ot.ctx.Done():
			return
		case <-ticker.C:
			ot.checkPendingOrders()
		}
	}
}

// checkPendingOrders checks all pending orders for timeout
func (ot *OrderTracker) checkPendingOrders() {
	ot.mu.RLock()
	pendingOrders := make([]*TrackedOrder, 0)
	for _, order := range ot.orders {
		if order.Status == OrderStatusPending {
			pendingOrders = append(pendingOrders, order)
		}
	}
	metrics := ot.metrics
	ot.mu.RUnlock()

	now := time.Now()
	for _, order := range pendingOrders {
		// Check if order has timed out
		if now.After(order.TimeoutAt) {
			log.Warn().
				Str("client_order_id", order.ClientOrderID).
				Str("symbol", order.Symbol).
				Dur("elapsed", now.Sub(order.SubmittedAt)).
				Msg("Order execution timeout reached")

			// Record timeout metric
			if metrics != nil {
				metrics.OrderTimeoutsInc()
			}

			// Attempt to cancel the order
			ot.cancelOrder(order)

			// Update status to timeout
			ot.updateOrderStatus(order.ClientOrderID, OrderStatusTimeout, fmt.Errorf("order execution timeout after %v", ot.executionTimeout))
		} else {
			// Check order status (would need API endpoint for this)
			// For now, we'll just update the last checked time
			order.LastCheckedAt = now
		}
	}
}

// cancelOrder attempts to cancel a pending order
func (ot *OrderTracker) cancelOrder(order *TrackedOrder) {
	// Note: This would require a cancel order API endpoint
	// For now, we'll just log the attempt
	log.Info().
		Str("client_order_id", order.ClientOrderID).
		Str("symbol", order.Symbol).
		Msg("Attempting to cancel timed-out order")

	// In a real implementation, you would call:
	// err := ot.client.CancelOrder(order.ClientOrderID)
}

// updateOrderStatus updates the status of a tracked order
func (ot *OrderTracker) updateOrderStatus(clientOrderID string, status OrderStatus, err error) {
	ot.mu.Lock()
	defer ot.mu.Unlock()

	if order, exists := ot.orders[clientOrderID]; exists {
		order.Status = status
		order.Error = err

		// Clean up completed orders after some time
		if status != OrderStatusPending {
			go func() {
				time.Sleep(5 * time.Minute)
				ot.mu.Lock()
				delete(ot.orders, clientOrderID)
				ot.mu.Unlock()
			}()
		}
	}
}

// GetOrderStatus returns the current status of an order
func (ot *OrderTracker) GetOrderStatus(clientOrderID string) (OrderStatus, error) {
	ot.mu.RLock()
	defer ot.mu.RUnlock()

	if order, exists := ot.orders[clientOrderID]; exists {
		return order.Status, order.Error
	}

	return "", fmt.Errorf("order not found: %s", clientOrderID)
}

// GetPendingOrders returns all pending orders
func (ot *OrderTracker) GetPendingOrders() []*TrackedOrder {
	ot.mu.RLock()
	defer ot.mu.RUnlock()

	pending := make([]*TrackedOrder, 0)
	for _, order := range ot.orders {
		if order.Status == OrderStatusPending {
			pending = append(pending, order)
		}
	}

	return pending
}

// GetAllOrders returns all tracked orders
func (ot *OrderTracker) GetAllOrders() map[string]*TrackedOrder {
	ot.mu.RLock()
	defer ot.mu.RUnlock()

	// Create a copy to avoid race conditions
	ordersCopy := make(map[string]*TrackedOrder)
	for k, v := range ot.orders {
		ordersCopy[k] = v
	}

	return ordersCopy
}
