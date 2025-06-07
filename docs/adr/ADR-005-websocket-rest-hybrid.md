# ADR-005: WebSocket + REST API Hybrid Communication

## Status
Accepted

## Date
2025-01-31

## Context

The Bitunix Trading Bot requires efficient communication with the Bitunix exchange for:
- **Real-time Market Data**: Live price feeds, order book updates, trade streams
- **Order Management**: Placing, modifying, and canceling orders
- **Account Information**: Balance queries, position updates, order status
- **Historical Data**: Backtesting data and historical analysis

Communication requirements:
- **Low Latency**: Critical for high-frequency trading decisions
- **Real-time Updates**: Immediate notification of market changes
- **Reliability**: Guaranteed delivery for order operations
- **Efficiency**: Minimize bandwidth and connection overhead
- **Error Handling**: Robust error recovery and reconnection
- **Rate Limiting**: Respect exchange API limits

Communication patterns considered:
- **REST Only**: Simple but requires polling for real-time data
- **WebSocket Only**: Real-time but complex for request-response operations
- **Hybrid Approach**: WebSocket for streaming, REST for operations
- **gRPC**: High performance but not supported by most exchanges
- **Message Queue**: Additional infrastructure complexity

## Decision

We chose a **Hybrid WebSocket + REST API** approach for exchange communication.

### Communication Strategy:
- **WebSocket**: Real-time market data streaming (trades, depth, price updates)
- **REST API**: Order operations, account queries, and configuration
- **Connection Pooling**: Optimized HTTP connections for REST calls
- **Automatic Reconnection**: Robust WebSocket reconnection with exponential backoff

### Key Factors:

1. **Real-time Performance**: WebSocket provides immediate market data updates
2. **Operational Reliability**: REST API ensures reliable order placement
3. **Exchange Standards**: Most exchanges follow this pattern
4. **Bandwidth Efficiency**: WebSocket reduces polling overhead
5. **Error Isolation**: Separate channels for different types of operations
6. **Rate Limit Management**: Different limits for streaming vs operations

## Consequences

### Positive:
- **Low Latency**: Immediate market data updates via WebSocket
- **Reliability**: REST API provides guaranteed delivery for critical operations
- **Efficiency**: Reduced bandwidth usage compared to REST polling
- **Flexibility**: Can optimize each channel for its specific use case
- **Standard Pattern**: Follows industry best practices for trading systems
- **Error Isolation**: Market data issues don't affect order placement
- **Scalability**: Can handle high-frequency market data efficiently

### Negative:
- **Complexity**: Managing two different communication channels
- **Connection Management**: Need to handle WebSocket reconnections
- **State Synchronization**: Ensuring consistency between channels
- **Resource Usage**: Multiple connections consume more resources

### Mitigations:
- **Connection Pooling**: Reuse HTTP connections for REST calls
- **Health Monitoring**: Monitor both WebSocket and REST connectivity
- **Graceful Degradation**: Continue operations if one channel fails
- **Comprehensive Testing**: Test reconnection scenarios and edge cases

## Implementation Details

### WebSocket Implementation:
```go
// internal/exchange/bitunix/ws.go
type WSClient struct {
    conn        *websocket.Conn
    url         string
    symbols     []string
    reconnect   chan bool
    trades      chan *Trade
    depth       chan *Depth
    errors      chan error
    done        chan struct{}
    mu          sync.RWMutex
}

func (w *WSClient) Connect() error {
    conn, _, err := websocket.DefaultDialer.Dial(w.url, nil)
    if err != nil {
        return fmt.Errorf("websocket connection failed: %w", err)
    }
    
    w.mu.Lock()
    w.conn = conn
    w.mu.Unlock()
    
    // Start message processing
    go w.readMessages()
    go w.handleReconnection()
    
    return w.subscribe()
}

func (w *WSClient) readMessages() {
    defer w.conn.Close()
    
    for {
        var msg map[string]interface{}
        if err := w.conn.ReadJSON(&msg); err != nil {
            select {
            case w.errors <- err:
            case <-w.done:
                return
            }
            
            // Trigger reconnection
            select {
            case w.reconnect <- true:
            default:
            }
            return
        }
        
        w.processMessage(msg)
    }
}
```

### REST Implementation:
```go
// internal/exchange/bitunix/rest.go
type Client struct {
    key, secret, base string
    rest              *resty.Client
    orderTracker      *OrderTracker
}

func NewREST(key, secret, base string, timeout time.Duration) *Client {
    client := &Client{
        key:    key,
        secret: secret,
        base:   base,
    }
    
    // Configure HTTP client with optimizations
    client.rest = resty.New().
        SetTimeout(timeout).
        SetRetryCount(3).
        SetRetryWaitTime(1 * time.Second).
        SetRetryMaxWaitTime(5 * time.Second).
        SetHeader("Content-Type", "application/json")
    
    // Configure connection pooling
    client.rest.GetClient().Transport = &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
        DisableCompression:  false,
        ForceAttemptHTTP2:   true,
    }
    
    return client
}

func (c *Client) PlaceOrder(req *OrderRequest) (*OrderResponse, error) {
    // Sign request
    signature := c.signRequest(req)
    
    var resp OrderResponse
    response, err := c.rest.R().
        SetHeader("X-API-KEY", c.key).
        SetHeader("X-SIGNATURE", signature).
        SetBody(req).
        SetResult(&resp).
        Post(c.base + "/api/v1/order")
    
    if err != nil {
        return nil, fmt.Errorf("order placement failed: %w", err)
    }
    
    if response.StatusCode() != 200 {
        return nil, fmt.Errorf("order rejected: %s", response.String())
    }
    
    return &resp, nil
}
```

### Connection Management:
```go
// Hybrid client that manages both connections
type HybridClient struct {
    rest *Client
    ws   *WSClient
    
    // Channels for market data
    Trades chan *Trade
    Depth  chan *Depth
    Errors chan error
}

func (h *HybridClient) Start() error {
    // Start WebSocket for market data
    if err := h.ws.Connect(); err != nil {
        return fmt.Errorf("websocket connection failed: %w", err)
    }
    
    // Test REST connectivity
    if _, err := h.rest.GetServerTime(); err != nil {
        return fmt.Errorf("rest connection failed: %w", err)
    }
    
    // Bridge WebSocket data to main channels
    go h.bridgeMarketData()
    
    return nil
}
```

### Error Handling and Reconnection:
- **Exponential Backoff**: Increasing delays between reconnection attempts
- **Circuit Breaker**: Stop reconnecting after consecutive failures
- **Health Checks**: Regular ping/pong to detect connection issues
- **Graceful Degradation**: Continue with available connections

### Performance Optimizations:
- **Message Pooling**: Reuse message objects to reduce GC pressure
- **Batch Processing**: Group multiple market updates when possible
- **Connection Reuse**: HTTP connection pooling for REST calls
- **Compression**: Enable compression for large responses

## Monitoring and Metrics

### WebSocket Metrics:
- Connection uptime and reconnection frequency
- Message processing latency and throughput
- Error rates and types
- Sequence number gaps and recovery

### REST Metrics:
- Request latency and success rates
- Rate limit usage and throttling
- Connection pool utilization
- Order placement success rates

## Related ADRs
- ADR-001: Go as Primary Language
- ADR-004: Microservices Architecture with Internal Packages
- ADR-006: Prometheus for Metrics and Monitoring
