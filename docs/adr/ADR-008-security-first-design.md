# ADR-008: Security-First Design with Multiple Layers

## Status
Accepted

## Date
2025-01-31

## Context

The Bitunix Trading Bot handles sensitive financial operations requiring comprehensive security:
- **API Credentials**: Exchange API keys and secrets for trading operations
- **Financial Data**: Real-time trading data and position information
- **Trading Decisions**: Automated order placement with real money
- **Audit Requirements**: Compliance and regulatory audit trails
- **Network Security**: Protection against external attacks and unauthorized access
- **Data Protection**: Encryption of sensitive data at rest and in transit

Security threats considered:
- **Credential Theft**: Unauthorized access to trading accounts
- **API Abuse**: Rate limiting bypass and DoS attacks
- **Data Breaches**: Unauthorized access to trading data and strategies
- **Man-in-the-Middle**: Interception of trading communications
- **Insider Threats**: Unauthorized access by team members
- **Compliance Violations**: Inadequate audit trails and data protection

Security approaches evaluated:
- **Basic Authentication**: Simple but insufficient for financial applications
- **Defense in Depth**: Multiple security layers for comprehensive protection
- **Zero Trust**: Assume no implicit trust and verify everything
- **Compliance-First**: Design for regulatory requirements from the start

## Decision

We chose a **Security-First Design with Multiple Layers** approach implementing comprehensive security controls.

### Security Architecture:
1. **API Security**: HMAC signature verification and rate limiting
2. **Network Security**: IP whitelisting and TLS encryption
3. **Data Protection**: Encryption at rest and secure configuration management
4. **Access Control**: Role-based access and audit logging
5. **Monitoring**: Security event detection and alerting

### Key Principles:
- **Defense in Depth**: Multiple independent security layers
- **Least Privilege**: Minimal required permissions for each component
- **Fail Secure**: Default to secure state when errors occur
- **Audit Everything**: Comprehensive logging of all security-relevant events

## Consequences

### Positive:
- **Comprehensive Protection**: Multiple layers protect against various attack vectors
- **Regulatory Compliance**: Audit trails and data protection meet compliance requirements
- **Incident Response**: Detailed logging enables quick incident investigation
- **Trust and Confidence**: Strong security builds user and stakeholder confidence
- **Risk Mitigation**: Reduces financial and reputational risks
- **Future-Proof**: Extensible security framework for new requirements

### Negative:
- **Implementation Complexity**: Multiple security layers increase development effort
- **Performance Overhead**: Security checks add latency to operations
- **Operational Complexity**: More components to monitor and maintain
- **Development Friction**: Security requirements may slow feature development

### Mitigations:
- **Security Automation**: Automated security testing and deployment
- **Performance Optimization**: Efficient implementation of security controls
- **Developer Training**: Security awareness and secure coding practices
- **Security Tools**: Leverage existing security libraries and frameworks

## Implementation Details

### 1. API Security

#### HMAC Signature Verification:
```go
// internal/security/signature.go
type SignatureValidator struct {
    secretKey    string
    timeWindow   time.Duration
    exemptPaths  map[string]bool
}

func (sv *SignatureValidator) ValidateRequest(r *http.Request) error {
    // Skip validation for public endpoints
    if sv.exemptPaths[r.URL.Path] {
        return nil
    }
    
    // Extract signature and timestamp
    signature := r.Header.Get("X-Signature")
    timestamp := r.Header.Get("X-Timestamp")
    
    if signature == "" || timestamp == "" {
        return fmt.Errorf("missing required headers")
    }
    
    // Validate timestamp window
    reqTime, err := time.Parse(time.RFC3339, timestamp)
    if err != nil {
        return fmt.Errorf("invalid timestamp format")
    }
    
    if time.Since(reqTime) > sv.timeWindow {
        return fmt.Errorf("request timestamp outside valid window")
    }
    
    // Verify HMAC signature
    body, _ := io.ReadAll(r.Body)
    r.Body = io.NopCloser(bytes.NewReader(body))
    
    payload := fmt.Sprintf("%s%s%s%s", r.Method, r.URL.Path, timestamp, string(body))
    expectedSignature := sv.generateHMAC(payload)
    
    if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
        return fmt.Errorf("invalid signature")
    }
    
    return nil
}

func (sv *SignatureValidator) generateHMAC(payload string) string {
    h := hmac.New(sha256.New, []byte(sv.secretKey))
    h.Write([]byte(payload))
    return hex.EncodeToString(h.Sum(nil))
}
```

#### Rate Limiting:
```go
// internal/security/rate_limiter.go
type RateLimiter struct {
    limiter *rate.Limiter
    mu      sync.RWMutex
}

func NewRateLimiter(requestsPerSecond float64, burstSize int) *RateLimiter {
    return &RateLimiter{
        limiter: rate.NewLimiter(rate.Limit(requestsPerSecond), burstSize),
    }
}

func (rl *RateLimiter) Allow() bool {
    rl.mu.RLock()
    defer rl.mu.RUnlock()
    return rl.limiter.Allow()
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !rl.Allow() {
            w.Header().Set("Retry-After", "1")
            http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### 2. Network Security

#### IP Whitelisting:
```go
// internal/security/ip_whitelist.go
type IPWhitelist struct {
    allowedIPs   map[string]bool
    allowedCIDRs []*net.IPNet
    mu           sync.RWMutex
}

func (ipw *IPWhitelist) IsAllowed(clientIP string) bool {
    ipw.mu.RLock()
    defer ipw.mu.RUnlock()
    
    // Check direct IP match
    if ipw.allowedIPs[clientIP] {
        return true
    }
    
    // Check CIDR ranges
    ip := net.ParseIP(clientIP)
    if ip == nil {
        return false
    }
    
    for _, cidr := range ipw.allowedCIDRs {
        if cidr.Contains(ip) {
            return true
        }
    }
    
    return false
}

func (ipw *IPWhitelist) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        clientIP := ipw.getClientIP(r)
        
        if !ipw.IsAllowed(clientIP) {
            log.Warn().Str("client_ip", clientIP).Msg("Access denied: IP not whitelisted")
            http.Error(w, "Access denied", http.StatusForbidden)
            return
        }
        
        next.ServeHTTP(w, r)
    })
}

func (ipw *IPWhitelist) getClientIP(r *http.Request) string {
    // Check X-Forwarded-For header (for load balancers)
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        ips := strings.Split(xff, ",")
        return strings.TrimSpace(ips[0])
    }
    
    // Check X-Real-IP header
    if xri := r.Header.Get("X-Real-IP"); xri != "" {
        return xri
    }
    
    // Fall back to RemoteAddr
    host, _, _ := net.SplitHostPort(r.RemoteAddr)
    return host
}
```

### 3. Data Protection

#### Configuration Encryption:
```go
// internal/security/encryption.go
type ConfigEncryption struct {
    cipher cipher.AEAD
}

func NewConfigEncryption(key []byte) (*ConfigEncryption, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    
    aead, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    
    return &ConfigEncryption{cipher: aead}, nil
}

func (ce *ConfigEncryption) Encrypt(plaintext []byte) (string, error) {
    nonce := make([]byte, ce.cipher.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }
    
    ciphertext := ce.cipher.Seal(nonce, nonce, plaintext, nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (ce *ConfigEncryption) Decrypt(ciphertext string) ([]byte, error) {
    data, err := base64.StdEncoding.DecodeString(ciphertext)
    if err != nil {
        return nil, err
    }
    
    nonceSize := ce.cipher.NonceSize()
    if len(data) < nonceSize {
        return nil, fmt.Errorf("ciphertext too short")
    }
    
    nonce, ciphertext := data[:nonceSize], data[nonceSize:]
    return ce.cipher.Open(nil, nonce, ciphertext, nil)
}
```

### 4. Audit Logging

#### Comprehensive Audit Trail:
```go
// internal/security/audit_logger.go
type AuditLogger struct {
    file   *os.File
    logger *log.Logger
    mu     sync.Mutex
}

type AuditEvent struct {
    Timestamp   time.Time `json:"timestamp"`
    EventType   string    `json:"event_type"`
    UserID      string    `json:"user_id,omitempty"`
    Action      string    `json:"action"`
    Resource    string    `json:"resource,omitempty"`
    Result      string    `json:"result"`
    ClientIP    string    `json:"client_ip,omitempty"`
    UserAgent   string    `json:"user_agent,omitempty"`
    Details     map[string]interface{} `json:"details,omitempty"`
}

func (al *AuditLogger) LogEvent(event AuditEvent) error {
    al.mu.Lock()
    defer al.mu.Unlock()
    
    event.Timestamp = time.Now().UTC()
    
    eventJSON, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("failed to marshal audit event: %w", err)
    }
    
    al.logger.Println(string(eventJSON))
    
    // Force sync for critical events
    if event.EventType == "SECURITY_VIOLATION" || event.EventType == "TRADING_ACTION" {
        al.file.Sync()
    }
    
    return nil
}

// Trading-specific audit events
func (al *AuditLogger) LogTradingAction(action, symbol string, details map[string]interface{}) {
    al.LogEvent(AuditEvent{
        EventType: "TRADING_ACTION",
        Action:    action,
        Resource:  symbol,
        Result:    "SUCCESS",
        Details:   details,
    })
}

func (al *AuditLogger) LogSecurityViolation(violation, clientIP string, details map[string]interface{}) {
    al.LogEvent(AuditEvent{
        EventType: "SECURITY_VIOLATION",
        Action:    violation,
        Result:    "BLOCKED",
        ClientIP:  clientIP,
        Details:   details,
    })
}
```

### 5. Security Manager Integration

#### Unified Security Management:
```go
// internal/security/security_manager.go
type SecurityManager struct {
    config           SecurityConfig
    rateLimiter      *RateLimiter
    ipWhitelist      *IPWhitelist
    auditLogger      *AuditLogger
    encryptionCipher cipher.AEAD
    signatureValidator *SignatureValidator
    mu               sync.RWMutex
}

func NewSecurityManager(config SecurityConfig) (*SecurityManager, error) {
    // Initialize all security components
    rateLimiter := NewRateLimiter(config.RateLimit.RequestsPerSecond, config.RateLimit.BurstSize)
    
    ipWhitelist := NewIPWhitelist(config.IPWhitelist.AllowedIPs, config.IPWhitelist.AllowedCIDRs)
    
    auditLogger, err := NewAuditLogger(config.AuditLog.FilePath)
    if err != nil {
        return nil, fmt.Errorf("failed to create audit logger: %w", err)
    }
    
    encryption, err := NewConfigEncryption([]byte(config.Encryption.Key))
    if err != nil {
        return nil, fmt.Errorf("failed to create encryption: %w", err)
    }
    
    signatureValidator := NewSignatureValidator(config.APISignature.SecretKey, config.APISignature.TimeWindow)
    
    return &SecurityManager{
        config:             config,
        rateLimiter:        rateLimiter,
        ipWhitelist:        ipWhitelist,
        auditLogger:        auditLogger,
        encryptionCipher:   encryption.cipher,
        signatureValidator: signatureValidator,
    }, nil
}

func (sm *SecurityManager) SecurityMiddleware(next http.Handler) http.Handler {
    return sm.ipWhitelist.Middleware(
        sm.rateLimiter.Middleware(
            sm.signatureValidator.Middleware(next),
        ),
    )
}
```

## Security Configuration

### Environment Variables:
```bash
# Security configuration
SECURITY_ENABLED=true
API_SIGNATURE_SECRET_KEY="your-secret-key"
API_SIGNATURE_TIME_WINDOW="300s"
RATE_LIMIT_REQUESTS_PER_SECOND="10"
RATE_LIMIT_BURST_SIZE="20"
IP_WHITELIST_ENABLED=true
IP_WHITELIST_ALLOWED_IPS="192.168.1.100,10.0.0.0/8"
AUDIT_LOG_ENABLED=true
AUDIT_LOG_FILE_PATH="/var/log/bitunix/audit.log"
ENCRYPTION_KEY="32-byte-encryption-key-here"
```

### Security Monitoring:
- Failed authentication attempts
- Rate limit violations
- IP whitelist violations
- Unusual trading patterns
- Configuration changes
- System access events

## Related ADRs
- ADR-006: Prometheus for Metrics and Monitoring
- ADR-007: Multi-Environment Deployment Strategy
- ADR-009: Circuit Breaker Pattern for Risk Management
