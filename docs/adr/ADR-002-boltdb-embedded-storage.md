# ADR-002: BoltDB for Embedded Storage

## Status
Accepted

## Date
2025-01-31

## Context

The Bitunix Trading Bot requires persistent storage for:
- Historical trade data for backtesting and analysis
- Market depth snapshots for feature calculation
- ML training data and feature records
- Configuration and state persistence
- Audit logs and trading history

Storage requirements:
- **Embedded**: No external database dependencies for simplified deployment
- **ACID Compliance**: Ensure data consistency for financial records
- **Performance**: Fast reads/writes for real-time data storage
- **Reliability**: Data durability and crash recovery
- **Simplicity**: Minimal operational overhead
- **Size Efficiency**: Compact storage for time-series data

Options considered:
- **BoltDB**: Pure Go, embedded, ACID-compliant key-value store
- **SQLite**: Mature, SQL support, but requires CGO
- **BadgerDB**: High performance, but more complex
- **LevelDB**: Good performance, but requires CGO
- **PostgreSQL**: Full SQL features, but external dependency
- **In-Memory**: Fast but no persistence

## Decision

We chose **BoltDB** as the embedded storage solution for the Bitunix Trading Bot.

### Key Factors:

1. **Pure Go**: No CGO dependencies, ensuring easy cross-compilation and deployment
2. **ACID Compliance**: Full ACID transactions ensure data consistency for financial records
3. **Embedded**: Single file database with no external dependencies
4. **Performance**: Excellent read performance with B+ tree structure
5. **Simplicity**: Simple key-value API that's easy to use and maintain
6. **Reliability**: Proven stability and data durability
7. **Size**: Compact binary size and efficient storage format

## Consequences

### Positive:
- **Zero Dependencies**: No external database server required
- **Easy Deployment**: Single file database travels with the application
- **ACID Guarantees**: Strong consistency for financial data
- **Cross-Platform**: Works identically across all platforms
- **Simple Operations**: No database administration required
- **Fast Reads**: Excellent performance for time-series queries
- **Crash Recovery**: Built-in recovery mechanisms

### Negative:
- **Write Performance**: Single writer limitation affects concurrent writes
- **No SQL**: Limited query capabilities compared to SQL databases
- **Memory Usage**: Entire database is memory-mapped
- **Concurrent Access**: Only one write transaction at a time

### Mitigations:
- **Batch Writes**: Group multiple operations into single transactions
- **Read Replicas**: Use read-only transactions for concurrent access
- **Data Modeling**: Design key structure for efficient range queries
- **Monitoring**: Track database size and performance metrics

## Implementation Details

### Database Structure:
```
bitunix-data.db
├── trades/          # Trade records by timestamp
├── depths/          # Market depth snapshots
├── features/        # ML feature records
└── config/          # Configuration and state
```

### Key Design Patterns:
- **Time-based Keys**: Use timestamp prefixes for efficient range queries
- **Bucket Organization**: Separate buckets for different data types
- **Batch Operations**: Group related writes into single transactions
- **Read-Only Access**: Use read-only transactions for concurrent queries

### Performance Optimizations:
- **Connection Pooling**: Reuse database connections
- **Batch Inserts**: Group multiple records into single transactions
- **Efficient Serialization**: Use JSON for human-readable data
- **Index Strategy**: Design keys for efficient range scans

## Usage Examples

```go
// Store trade data
func (s *Storage) StoreTrade(trade *bitunix.Trade) error {
    return s.db.Update(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("trades"))
        key := fmt.Sprintf("%d_%s", trade.Timestamp, trade.Symbol)
        data, _ := json.Marshal(trade)
        return bucket.Put([]byte(key), data)
    })
}

// Query trades in time range
func (s *Storage) GetTradesInRange(symbol string, start, end time.Time) ([]*bitunix.Trade, error) {
    var trades []*bitunix.Trade
    return trades, s.db.View(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("trades"))
        cursor := bucket.Cursor()
        
        startKey := fmt.Sprintf("%d_%s", start.Unix(), symbol)
        endKey := fmt.Sprintf("%d_%s", end.Unix(), symbol)
        
        for k, v := cursor.Seek([]byte(startKey)); k != nil && bytes.Compare(k, []byte(endKey)) <= 0; k, v = cursor.Next() {
            var trade bitunix.Trade
            if err := json.Unmarshal(v, &trade); err == nil {
                trades = append(trades, &trade)
            }
        }
        return nil
    })
}
```

## Related ADRs
- ADR-001: Go as Primary Language
- ADR-004: Microservices Architecture with Internal Packages
