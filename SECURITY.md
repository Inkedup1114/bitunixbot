# Security Guide

This document outlines security best practices and configurations for the Bitunix Trading Bot.

## Table of Contents
- [Security Architecture](#security-architecture)
- [Authentication & Authorization](#authentication--authorization)
- [Network Security](#network-security)
- [Container Security](#container-security)
- [API Security](#api-security)
- [Monitoring & Alerting](#monitoring--alerting)
- [Incident Response](#incident-response)

## Security Architecture

### Defense in Depth

The bot implements multiple layers of security:

1. **Application Layer**: Input validation, secure coding practices
2. **Runtime Layer**: Non-root execution, read-only filesystems
3. **Network Layer**: TLS encryption, network policies
4. **Infrastructure Layer**: Container scanning, secrets management

### Security Controls

```yaml
# Security configuration in config.yaml
security:
  # API rate limiting
    enabled: true
    retentionDays: 90
```

## Authentication & Authorization

### API Key Security

1. **Key Management**:
   ```bash
   # NEVER hardcode credentials in code or config files
   # Use environment variables
   export BITUNIX_API_KEY="$(cat /run/secrets/api_key)"
   export BITUNIX_SECRET_KEY="$(cat /run/secrets/secret_key)"
   
   # Or use secure files with proper permissions
   chmod 600 secrets/api_key.txt secrets/secret_key.txt
   ```

2. **Environment Variables**:
   ```bash
   # Use secure environment variable patterns
   # Option 1: From files (recommended for production)
   export BITUNIX_API_KEY="$(cat /run/secrets/api_key)"
   export BITUNIX_SECRET_KEY="$(cat /run/secrets/secret_key)"
   
   # Option 2: Direct environment variables (development only)
   export BITUNIX_API_KEY="your_development_key"
   export BITUNIX_SECRET_KEY="your_development_secret"
   ```

3. **Secrets Rotation**:
   ```bash
   # Automated rotation script
   #!/bin/bash
   NEW_KEY=$(openssl rand -hex 32)
   kubectl create secret generic bitunix-credentials-new \
     --from-literal=api-key="$NEW_KEY" \
     --dry-run=client -o yaml | kubectl apply -f -
   
   # Rolling update deployment
   kubectl patch deployment bitunix-bot \
     -p '{"spec":{"template":{"metadata":{"annotations":{"date":"'$(date +'%s')'"}}}}}' \
     -n bitunix-bot
   ```

### Access Control

1. **RBAC Configuration** (Kubernetes):
   ```yaml
   apiVersion: rbac.authorization.k8s.io/v1
   kind: Role
   metadata:
     namespace: bitunix-bot
     name: bitunix-bot-role
   rules:
   - apiGroups: [""]
     resources: ["secrets", "configmaps"]
     verbs: ["get", "list"]
   - apiGroups: [""]
     resources: ["pods"]
     verbs: ["get", "list", "watch"]
   ---
   apiVersion: rbac.authorization.k8s.io/v1
   kind: RoleBinding
   metadata:
     name: bitunix-bot-binding
     namespace: bitunix-bot
   subjects:
   - kind: ServiceAccount
     name: bitunix-bot-sa
     namespace: bitunix-bot
   roleRef:
     kind: Role
     name: bitunix-bot-role
     apiGroup: rbac.authorization.k8s.io
   ```

## Network Security

### TLS Configuration

1. **Certificate Management**:
   ```bash
   # Generate self-signed certificate for development
   openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem \
     -days 365 -nodes -subj "/C=US/ST=CA/L=SF/O=BitunixBot/CN=localhost"
   
   # For production, use Let's Encrypt or enterprise CA
   certbot certonly --dns-cloudflare \
     --dns-cloudflare-credentials ~/.secrets/certbot/cloudflare.ini \
     -d bitunix-bot.yourdomain.com
   ```

2. **Network Policies** (Kubernetes):
   ```yaml
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: bitunix-bot-netpol
     namespace: bitunix-bot
   spec:
     podSelector:
       matchLabels:
         app: bitunix-bot
     policyTypes:
     - Ingress
     - Egress
     ingress:
     - from:
       - namespaceSelector:
           matchLabels:
             name: monitoring
       ports:
       - protocol: TCP
         port: 8080
     egress:
     - to: []
       ports:
       - protocol: TCP
         port: 443  # HTTPS to Bitunix API
       - protocol: TCP
         port: 80   # HTTP for health checks
     ```

### Firewall Rules

1. **UFW Configuration**:
   ```bash
   # Basic UFW setup
   sudo ufw default deny incoming
   sudo ufw default allow outgoing
   sudo ufw allow 22/tcp    # SSH
   sudo ufw allow 8080/tcp  # Metrics (restrict to monitoring network)
   sudo ufw allow out 443/tcp  # HTTPS
   sudo ufw enable
   
   # Restrict metrics endpoint
   sudo ufw allow from 10.0.0.0/8 to any port 8080
   sudo ufw allow from 172.16.0.0/12 to any port 8080
   sudo ufw allow from 192.168.0.0/16 to any port 8080
   ```

## Container Security

### Secure Base Images

1. **Dockerfile Security Best Practices**:
   ```dockerfile
   # Use specific version tags
   FROM golang:1.24.2-alpine3.20 AS build
   
   # Create non-root user
   RUN addgroup -g 65532 -S appgroup && \
       adduser -u 65532 -S appuser -G appgroup
   
   # Install specific package versions
   RUN apk add --no-cache \
       ca-certificates=20240705-r0 \
       python3=3.12.8-r1 \
       py3-pip=24.0-r2
   
   # Final stage
   FROM alpine:3.20
   RUN apk add --no-cache ca-certificates=20240705-r0
   
   # Security hardening
   RUN rm -rf /var/cache/apk/* \
       && rm -rf /tmp/* \
       && rm -rf /var/tmp/*
   
   # Use non-root user
   USER 65532:65532
   ```

2. **Security Scanning**:
   ```bash
   # Trivy scanning
   trivy image --severity HIGH,CRITICAL bitunix-bot:latest
   
   # Snyk scanning
   snyk test --docker bitunix-bot:latest
   
   # Docker Scout (if available)
   docker scout cves bitunix-bot:latest
   ```

### Runtime Security

1. **Security Context**:
   ```yaml
   securityContext:
     runAsNonRoot: true
     runAsUser: 65532
     runAsGroup: 65532
     allowPrivilegeEscalation: false
     readOnlyRootFilesystem: true
     capabilities:
       drop:
       - ALL
     seccompProfile:
       type: RuntimeDefault
   ```

2. **Resource Limits**:
   ```yaml
   resources:
     limits:
       cpu: "500m"
       memory: "512Mi"
       ephemeral-storage: "1Gi"
     requests:
       cpu: "100m"
       memory: "128Mi"
       ephemeral-storage: "500Mi"
   ```

## API Security

### Input Validation

1. **Go Implementation**:
   ```go
   package security
   
   import (
       "errors"
       "regexp"
       "strings"
   )
   
   // ValidateSymbol ensures trading symbols are valid
   func ValidateSymbol(symbol string) error {
       if len(symbol) < 3 || len(symbol) > 20 {
           return errors.New("invalid symbol length")
       }
       
       matched, _ := regexp.MatchString(`^[A-Z0-9]+$`, symbol)
       if !matched {
           return errors.New("invalid symbol format")
       }
       
       return nil
   }
   
   // SanitizeInput removes potentially dangerous characters
   func SanitizeInput(input string) string {
       // Remove control characters
       input = regexp.MustCompile(`[\x00-\x1f\x7f-\x9f]`).ReplaceAllString(input, "")
       
       // Limit length
       if len(input) > 1000 {
           input = input[:1000]
       }
       
       return strings.TrimSpace(input)
   }
   ```

### Rate Limiting

1. **Implementation**:
   ```go
   package middleware
   
   import (
       "net/http"
       "sync"
       "time"
       
       "golang.org/x/time/rate"
   )
   
   type RateLimiter struct {
       visitors map[string]*rate.Limiter
       mu       sync.RWMutex
       rate     rate.Limit
       burst    int
   }
   
   func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
       return &RateLimiter{
           visitors: make(map[string]*rate.Limiter),
           rate:     r,
           burst:    b,
       }
   }
   
   func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
       rl.mu.Lock()
       defer rl.mu.Unlock()
       
       limiter, exists := rl.visitors[ip]
       if !exists {
           limiter = rate.NewLimiter(rl.rate, rl.burst)
           rl.visitors[ip] = limiter
       }
       
       return limiter
   }
   
   func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
       return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           ip := r.RemoteAddr
           limiter := rl.getLimiter(ip)
           
           if !limiter.Allow() {
               http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
               return
           }
           
           next.ServeHTTP(w, r)
       })
   }
   ```

## Monitoring & Alerting

### Security Metrics

1. **Custom Metrics**:
   ```go
   var (
       securityEventsTotal = prometheus.NewCounterVec(
           prometheus.CounterOpts{
               Name: "security_events_total",
               Help: "Total number of security events",
           },
           []string{"type", "severity"},
       )
       
       authFailuresTotal = prometheus.NewCounterVec(
           prometheus.CounterOpts{
               Name: "auth_failures_total",
               Help: "Total number of authentication failures",
           },
           []string{"source"},
       )
       
       rateLimitHitsTotal = prometheus.NewCounter(
           prometheus.CounterOpts{
               Name: "rate_limit_hits_total",
               Help: "Total number of rate limit hits",
           },
       )
   )
   ```

2. **Alert Rules**:
   ```yaml
   groups:
   - name: security_alerts
     rules:
     - alert: HighAuthFailureRate
       expr: rate(auth_failures_total[5m]) > 10
       for: 2m
       labels:
         severity: warning
       annotations:
         summary: "High authentication failure rate detected"
         description: "Authentication failure rate is {{ $value }} per second"
     
     - alert: RateLimitExceeded
       expr: rate(rate_limit_hits_total[1m]) > 5
       for: 1m
       labels:
         severity: critical
       annotations:
         summary: "Rate limiting activated"
         description: "Rate limit exceeded with {{ $value }} hits per second"
     
     - alert: UnauthorizedAccess
       expr: increase(security_events_total{type="unauthorized"}[5m]) > 5
       for: 1m
       labels:
         severity: critical
       annotations:
         summary: "Multiple unauthorized access attempts"
         description: "{{ $value }} unauthorized access attempts in 5 minutes"
   ```

### Security Logging

1. **Structured Logging**:
   ```go
   package security
   
   import (
       "github.com/rs/zerolog"
       "github.com/rs/zerolog/log"
   )
   
   type SecurityEvent struct {
       Type        string `json:"type"`
       Severity    string `json:"severity"`
       Source      string `json:"source"`
       Description string `json:"description"`
       UserAgent   string `json:"user_agent,omitempty"`
       IP          string `json:"ip,omitempty"`
   }
   
   func LogSecurityEvent(event SecurityEvent) {
       log.Warn().
           Str("event_type", event.Type).
           Str("severity", event.Severity).
           Str("source", event.Source).
           Str("description", event.Description).
           Str("user_agent", event.UserAgent).
           Str("ip", event.IP).
           Msg("Security event detected")
   }
   ```

## Incident Response

### Response Procedures

1. **Security Incident Playbook**:
   ```bash
   #!/bin/bash
   # security_incident_response.sh
   
   set -e
   
   INCIDENT_TYPE=$1
   SEVERITY=$2
   
   case $INCIDENT_TYPE in
       "unauthorized_access")
           echo "Responding to unauthorized access incident..."
           # Block suspicious IPs
           kubectl patch networkpolicy bitunix-bot-netpol \
             --patch '{"spec":{"ingress":[]}}'
           ;;
       "api_compromise")
           echo "Responding to API key compromise..."
           # Rotate API keys immediately
           ./scripts/rotate_api_keys.sh
           # Scale down deployment
           kubectl scale deployment bitunix-bot --replicas=0
           ;;
       "data_breach")
           echo "Responding to data breach..."
           # Enable audit logging
           kubectl patch configmap bot-config \
             --patch '{"data":{"audit.enabled":"true"}}'
           # Notify stakeholders
           ./scripts/notify_incident.sh "$INCIDENT_TYPE" "$SEVERITY"
           ;;
   esac
   ```

2. **Emergency Shutdown**:
   ```bash
   #!/bin/bash
   # emergency_shutdown.sh
   
   echo "Initiating emergency shutdown..."
   
   # Stop trading immediately
   kubectl set env deployment/bitunix-bot DRY_RUN=true
   
   # Scale down to zero
   kubectl scale deployment bitunix-bot --replicas=0
   
   # Revoke API access
   kubectl delete secret bitunix-credentials
   
   # Alert team
   curl -X POST "$SLACK_WEBHOOK" \
     -H 'Content-type: application/json' \
     --data '{"text":"🚨 EMERGENCY SHUTDOWN: Bitunix bot stopped due to security incident"}'
   
   echo "Emergency shutdown complete"
   ```

### Recovery Procedures

1. **Post-Incident Recovery**:
   ```bash
   #!/bin/bash
   # recovery.sh
   
   echo "Starting post-incident recovery..."
   
   # Verify system integrity
   ./scripts/verify_integrity.sh
   
   # Update configurations
   kubectl apply -f deploy/k8s/
   
   # Restart with new credentials
   kubectl create secret generic bitunix-credentials \
     --from-file=api-key=./secrets/new_api_key.txt \
     --from-file=secret-key=./secrets/new_secret_key.txt
   
   # Scale back up
   kubectl scale deployment bitunix-bot --replicas=1
   
   # Monitor for 24 hours
   ./scripts/enhanced_monitoring.sh
   
   echo "Recovery complete"
   ```

This security guide provides comprehensive protection for the Bitunix Trading Bot across all deployment scenarios and threat vectors.
