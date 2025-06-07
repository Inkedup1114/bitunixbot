package bitunix

import (
	"runtime"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// MemoryStats tracks memory usage statistics for WebSocket operations
type MemoryStats struct {
	// Message processing stats
	MessagesProcessed      int64
	MessageProcessingBytes int64
	DroppedMessages        int64
	
	// Object pool stats
	TradePoolGets          int64
	TradePoolPuts          int64
	DepthPoolGets          int64
	DepthPoolPuts          int64
	MessagePoolGets        int64
	MessagePoolPuts        int64
	
	// Connection stats
	ActiveConnections      int32
	TotalConnections       int64
	
	// Worker pool stats
	ActiveWorkers          int32
	TotalWorkers           int64
	
	// Memory allocation stats
	LastReportedAlloc      uint64
	PeakAlloc              uint64
	
	// Monitoring state
	monitoringActive       int32
	monitoringInterval     time.Duration
	leakThresholdPercent   float64
}

// NewMemoryStats creates a new memory statistics tracker
func NewMemoryStats() *MemoryStats {
	return &MemoryStats{
		monitoringInterval:   30 * time.Second,
		leakThresholdPercent: 10.0, // 10% growth between checks is considered suspicious
	}
}

// StartMonitoring begins periodic memory usage monitoring
func (ms *MemoryStats) StartMonitoring() {
	if !atomic.CompareAndSwapInt32(&ms.monitoringActive, 0, 1) {
		// Already monitoring
		return
	}
	
	go func() {
		ticker := time.NewTicker(ms.monitoringInterval)
		defer ticker.Stop()
		
		// Initial memory snapshot
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		atomic.StoreUint64(&ms.LastReportedAlloc, m.Alloc)
		atomic.StoreUint64(&ms.PeakAlloc, m.Alloc)
		
		log.Info().
			Uint64("alloc_mb", m.Alloc/1024/1024).
			Uint64("sys_mb", m.Sys/1024/1024).
			Int("goroutines", runtime.NumGoroutine()).
			Msg("WebSocket memory monitoring started")
		
		for range ticker.C {
			if atomic.LoadInt32(&ms.monitoringActive) == 0 {
				return
			}
			
			runtime.ReadMemStats(&m)
			
			// Update peak allocation if current is higher
			for {
				peak := atomic.LoadUint64(&ms.PeakAlloc)
				if m.Alloc <= peak {
					break
				}
				if atomic.CompareAndSwapUint64(&ms.PeakAlloc, peak, m.Alloc) {
					break
				}
			}
			
			// Calculate growth since last check
			lastAlloc := atomic.LoadUint64(&ms.LastReportedAlloc)
			growthPercent := 0.0
			if lastAlloc > 0 {
				growthPercent = (float64(m.Alloc) - float64(lastAlloc)) / float64(lastAlloc) * 100.0
			}
			
			// Check for potential memory leaks
			leakDetected := false
			if growthPercent > ms.leakThresholdPercent {
				leakDetected = true
			}
			
			// Calculate pool balance (gets - puts)
			tradePoolBalance := atomic.LoadInt64(&ms.TradePoolGets) - atomic.LoadInt64(&ms.TradePoolPuts)
			depthPoolBalance := atomic.LoadInt64(&ms.DepthPoolGets) - atomic.LoadInt64(&ms.DepthPoolPuts)
			messagePoolBalance := atomic.LoadInt64(&ms.MessagePoolGets) - atomic.LoadInt64(&ms.MessagePoolPuts)
			
			// Log memory stats
			logEvent := log.Info()
			if leakDetected {
				logEvent = log.Warn().Bool("potential_leak", true)
			}
			
			logEvent.
				Uint64("alloc_mb", m.Alloc/1024/1024).
				Uint64("sys_mb", m.Sys/1024/1024).
				Uint64("peak_alloc_mb", atomic.LoadUint64(&ms.PeakAlloc)/1024/1024).
				Float64("growth_percent", growthPercent).
				Int64("messages_processed", atomic.LoadInt64(&ms.MessagesProcessed)).
				Int64("dropped_messages", atomic.LoadInt64(&ms.DroppedMessages)).
				Int64("trade_pool_balance", tradePoolBalance).
				Int64("depth_pool_balance", depthPoolBalance).
				Int64("message_pool_balance", messagePoolBalance).
				Int32("active_connections", atomic.LoadInt32(&ms.ActiveConnections)).
				Int32("active_workers", atomic.LoadInt32(&ms.ActiveWorkers)).
				Int("goroutines", runtime.NumGoroutine()).
				Msg("WebSocket memory usage stats")
			
			// Update last reported allocation
			atomic.StoreUint64(&ms.LastReportedAlloc, m.Alloc)
		}
	}()
}

// StopMonitoring stops the memory monitoring
func (ms *MemoryStats) StopMonitoring() {
	atomic.StoreInt32(&ms.monitoringActive, 0)
}

// TrackMessageProcessed updates message processing statistics
func (ms *MemoryStats) TrackMessageProcessed(size int) {
	atomic.AddInt64(&ms.MessagesProcessed, 1)
	atomic.AddInt64(&ms.MessageProcessingBytes, int64(size))
}

// TrackMessageDropped increments the dropped message counter
func (ms *MemoryStats) TrackMessageDropped() {
	atomic.AddInt64(&ms.DroppedMessages, 1)
}

// TrackTradePoolGet increments the trade pool get counter
func (ms *MemoryStats) TrackTradePoolGet() {
	atomic.AddInt64(&ms.TradePoolGets, 1)
}

// TrackTradePoolPut increments the trade pool put counter
func (ms *MemoryStats) TrackTradePoolPut() {
	atomic.AddInt64(&ms.TradePoolPuts, 1)
}

// TrackDepthPoolGet increments the depth pool get counter
func (ms *MemoryStats) TrackDepthPoolGet() {
	atomic.AddInt64(&ms.DepthPoolGets, 1)
}

// TrackDepthPoolPut increments the depth pool put counter
func (ms *MemoryStats) TrackDepthPoolPut() {
	atomic.AddInt64(&ms.DepthPoolPuts, 1)
}

// TrackMessagePoolGet increments the message pool get counter
func (ms *MemoryStats) TrackMessagePoolGet() {
	atomic.AddInt64(&ms.MessagePoolGets, 1)
}

// TrackMessagePoolPut increments the message pool put counter
func (ms *MemoryStats) TrackMessagePoolPut() {
	atomic.AddInt64(&ms.MessagePoolPuts, 1)
}

// TrackConnectionActive increments the active connection counter
func (ms *MemoryStats) TrackConnectionActive() {
	atomic.AddInt32(&ms.ActiveConnections, 1)
	atomic.AddInt64(&ms.TotalConnections, 1)
}

// TrackConnectionClosed decrements the active connection counter
func (ms *MemoryStats) TrackConnectionClosed() {
	atomic.AddInt32(&ms.ActiveConnections, -1)
}

// TrackWorkerActive increments the active worker counter
func (ms *MemoryStats) TrackWorkerActive() {
	atomic.AddInt32(&ms.ActiveWorkers, 1)
	atomic.AddInt64(&ms.TotalWorkers, 1)
}

// TrackWorkerInactive decrements the active worker counter
func (ms *MemoryStats) TrackWorkerInactive() {
	atomic.AddInt32(&ms.ActiveWorkers, -1)
}

// GetStats returns a map of current memory statistics
func (ms *MemoryStats) GetStats() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	return map[string]interface{}{
		"alloc_mb":            m.Alloc / 1024 / 1024,
		"sys_mb":              m.Sys / 1024 / 1024,
		"peak_alloc_mb":       atomic.LoadUint64(&ms.PeakAlloc) / 1024 / 1024,
		"messages_processed":  atomic.LoadInt64(&ms.MessagesProcessed),
		"dropped_messages":    atomic.LoadInt64(&ms.DroppedMessages),
		"trade_pool_balance":  atomic.LoadInt64(&ms.TradePoolGets) - atomic.LoadInt64(&ms.TradePoolPuts),
		"depth_pool_balance":  atomic.LoadInt64(&ms.DepthPoolGets) - atomic.LoadInt64(&ms.DepthPoolPuts),
		"message_pool_balance": atomic.LoadInt64(&ms.MessagePoolGets) - atomic.LoadInt64(&ms.MessagePoolPuts),
		"active_connections":  atomic.LoadInt32(&ms.ActiveConnections),
		"active_workers":      atomic.LoadInt32(&ms.ActiveWorkers),
		"goroutines":          runtime.NumGoroutine(),
	}
}