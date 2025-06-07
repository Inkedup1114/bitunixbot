# ADR-007: Multi-Environment Deployment Strategy

## Status
Accepted

## Date
2025-01-31

## Context

The Bitunix Trading Bot requires deployment across multiple environments to support:
- **Development**: Local testing and feature development
- **Staging**: Pre-production validation and integration testing
- **Production**: Live trading operations with real money

Deployment requirements:
- **Environment Isolation**: Clear separation between environments
- **Configuration Management**: Environment-specific settings and secrets
- **Deployment Automation**: Consistent and repeatable deployments
- **Rollback Capability**: Quick recovery from failed deployments
- **Monitoring**: Environment-specific monitoring and alerting
- **Security**: Production-grade security for live trading
- **Scalability**: Support for different resource requirements per environment

Deployment strategies considered:
- **Manual Deployment**: Simple but error-prone and inconsistent
- **Docker Containers**: Consistent environments but requires orchestration
- **Kubernetes**: Full orchestration but complex for simple applications
- **Cloud Functions**: Serverless but limited for stateful trading applications
- **Hybrid Approach**: Different strategies per environment based on needs

## Decision

We chose a **Multi-Platform Deployment Strategy** with environment-specific approaches:

### Deployment Matrix:
- **Development**: Local binary + Docker Compose for dependencies
- **Staging**: Kubernetes with Helm charts for production-like testing
- **Production**: Kubernetes + Terraform for infrastructure as code

### Key Components:
1. **Containerization**: Docker for consistent runtime environments
2. **Orchestration**: Kubernetes for staging and production
3. **Infrastructure as Code**: Terraform for cloud resource management
4. **Configuration Management**: Environment-specific configs and secrets
5. **CI/CD Pipeline**: Automated testing, building, and deployment

## Consequences

### Positive:
- **Environment Consistency**: Docker ensures identical runtime across environments
- **Scalability**: Kubernetes provides auto-scaling and resource management
- **Infrastructure Automation**: Terraform enables reproducible infrastructure
- **Deployment Safety**: Staged deployments with validation at each level
- **Operational Excellence**: Standardized deployment and monitoring practices
- **Developer Productivity**: Local development closely matches production
- **Disaster Recovery**: Infrastructure as code enables quick environment recreation

### Negative:
- **Complexity**: Multiple deployment methods require different expertise
- **Resource Overhead**: Kubernetes adds operational complexity
- **Learning Curve**: Team needs to understand Docker, Kubernetes, and Terraform
- **Infrastructure Costs**: Cloud resources for staging and production environments

### Mitigations:
- **Documentation**: Comprehensive deployment guides and runbooks
- **Training**: Team training on containerization and orchestration
- **Automation**: Extensive automation to reduce manual operations
- **Monitoring**: Comprehensive monitoring to detect issues early

## Implementation Details

### 1. Development Environment

#### Local Development:
```bash
# Quick start for development
git clone https://github.com/your-repo/bitunix-bot.git
cd bitunix-bot

# Copy and configure
cp config.yaml.example config.yaml
cp .env.example .env.dev

# Install dependencies
go mod download
pip install -r scripts/requirements.txt

# Run locally
go run cmd/bitrader/main.go
```

#### Docker Compose for Dependencies:
```yaml
# docker-compose.dev.yml
version: '3.8'
services:
  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./deploy/prometheus.yml:/etc/prometheus/prometheus.yml

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
```

### 2. Staging Environment

#### Kubernetes Deployment:
```yaml
# deploy/k8s/staging/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bitunix-bot-staging
  namespace: bitunix-staging
spec:
  replicas: 1
  selector:
    matchLabels:
      app: bitunix-bot
      env: staging
  template:
    metadata:
      labels:
        app: bitunix-bot
        env: staging
    spec:
      containers:
      - name: bitunix-bot
        image: bitunix-bot:staging
        env:
        - name: ENVIRONMENT
          value: "staging"
        - name: DRY_RUN
          value: "false"
        - name: LOG_LEVEL
          value: "info"
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
```

#### Helm Chart Values:
```yaml
# deploy/helm/values-staging.yaml
image:
  repository: bitunix-bot
  tag: staging
  pullPolicy: Always

environment: staging

config:
  dryRun: false
  logLevel: info
  maxDailyLoss: 0.02
  baseSizeRatio: 0.001

resources:
  requests:
    memory: 512Mi
    cpu: 250m
  limits:
    memory: 1Gi
    cpu: 500m

monitoring:
  enabled: true
  serviceMonitor:
    enabled: true
```

### 3. Production Environment

#### Terraform Infrastructure:
```hcl
# deploy/terraform/main.tf
resource "aws_eks_cluster" "bitunix_cluster" {
  name     = "bitunix-bot-${var.environment}"
  role_arn = aws_iam_role.cluster_role.arn
  version  = var.kubernetes_version

  vpc_config {
    subnet_ids              = aws_subnet.private[*].id
    endpoint_private_access = true
    endpoint_public_access  = true
    public_access_cidrs     = var.allowed_cidr_blocks
  }

  encryption_config {
    provider {
      key_arn = aws_kms_key.cluster_encryption.arn
    }
    resources = ["secrets"]
  }

  depends_on = [
    aws_iam_role_policy_attachment.cluster_policy,
    aws_iam_role_policy_attachment.service_policy,
  ]
}

resource "aws_eks_node_group" "bitunix_nodes" {
  cluster_name    = aws_eks_cluster.bitunix_cluster.name
  node_group_name = "bitunix-nodes"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = aws_subnet.private[*].id

  scaling_config {
    desired_size = var.node_desired_size
    max_size     = var.node_max_size
    min_size     = var.node_min_size
  }

  instance_types = [var.node_instance_type]
  capacity_type  = "ON_DEMAND"

  update_config {
    max_unavailable = 1
  }
}
```

#### Production Helm Values:
```yaml
# deploy/helm/values-production.yaml
image:
  repository: bitunix-bot
  tag: latest
  pullPolicy: IfNotPresent

environment: production

config:
  dryRun: false
  logLevel: warn
  maxDailyLoss: 0.05
  baseSizeRatio: 0.002

resources:
  requests:
    memory: 2Gi
    cpu: 1000m
  limits:
    memory: 4Gi
    cpu: 2000m

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 5
  targetCPUUtilizationPercentage: 70

security:
  podSecurityPolicy:
    enabled: true
  networkPolicy:
    enabled: true
  serviceAccount:
    create: true
    annotations:
      eks.amazonaws.com/role-arn: arn:aws:iam::ACCOUNT:role/bitunix-bot-role
```

### 4. CI/CD Pipeline

#### GitHub Actions Workflow:
```yaml
# .github/workflows/ci-cd.yml
name: CI/CD Pipeline

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version: '1.22'
    - name: Run tests
      run: go test -v -race -coverprofile=coverage.out ./...

  build:
    needs: test
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
    - name: Build Docker image
      run: docker build -t bitunix-bot:${{ github.sha }} .
    - name: Push to registry
      run: |
        echo ${{ secrets.DOCKER_PASSWORD }} | docker login -u ${{ secrets.DOCKER_USERNAME }} --password-stdin
        docker push bitunix-bot:${{ github.sha }}

  deploy-staging:
    needs: build
    if: github.ref == 'refs/heads/develop'
    runs-on: ubuntu-latest
    steps:
    - name: Deploy to staging
      run: |
        helm upgrade --install bitunix-staging ./deploy/helm/bitunix-bot \
          --namespace bitunix-staging \
          --values deploy/helm/values-staging.yaml \
          --set image.tag=${{ github.sha }}

  deploy-production:
    needs: build
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    environment: production
    steps:
    - name: Deploy to production
      run: |
        helm upgrade --install bitunix-production ./deploy/helm/bitunix-bot \
          --namespace bitunix-production \
          --values deploy/helm/values-production.yaml \
          --set image.tag=${{ github.sha }}
```

### 5. Configuration Management

#### Environment-Specific Configs:
```yaml
# config/development.yaml
api:
  baseURL: "https://testnet-api.bitunix.com"
  wsURL: "wss://testnet-fapi.bitunix.com/public"

trading:
  dryRun: true
  maxDailyLoss: 0.01
  baseSizeRatio: 0.001

system:
  logLevel: "debug"
  metricsPort: 8080

# config/production.yaml
api:
  baseURL: "https://api.bitunix.com"
  wsURL: "wss://fapi.bitunix.com/public"

trading:
  dryRun: false
  maxDailyLoss: 0.05
  baseSizeRatio: 0.002

system:
  logLevel: "warn"
  metricsPort: 8080
```

### 6. Secrets Management

#### Kubernetes Secrets:
```bash
# Create secrets for each environment
kubectl create secret generic bitunix-credentials \
  --from-literal=api-key="${API_KEY}" \
  --from-literal=secret-key="${SECRET_KEY}" \
  --namespace bitunix-production

# Use external secrets operator for production
kubectl apply -f - <<EOF
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-secrets-manager
  namespace: bitunix-production
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-east-1
EOF
```

## Deployment Validation

### Staging Validation:
- Automated integration tests
- Performance benchmarks
- Security scans
- Configuration validation

### Production Deployment:
- Blue-green deployment strategy
- Canary releases for major changes
- Automated rollback on failure
- Comprehensive monitoring

## Related ADRs
- ADR-004: Microservices Architecture with Internal Packages
- ADR-006: Prometheus for Metrics and Monitoring
- ADR-008: Security-First Design with Multiple Layers
