# Deployment Guide

This guide covers deploying the Bitunix Trading Bot across different environments and platforms.

## Table of Contents
- [Environment Overview](#environment-overview)
- [Prerequisites](#prerequisites)
- [Local Development](#local-development)
- [Staging Environment](#staging-environment)
- [Docker Deployment](#docker-deployment)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Cloud Platforms](#cloud-platforms)
- [Production Deployment](#production-deployment)
- [Monitoring and Observability](#monitoring-and-observability)
- [Security Considerations](#security-considerations)
- [Backup and Recovery](#backup-and-recovery)
- [Troubleshooting](#troubleshooting)

## Environment Overview

The Bitunix Trading Bot supports deployment across multiple environments with different configurations:

| Environment | Purpose | Configuration | Monitoring Level |
|-------------|---------|---------------|------------------|
| **Development** | Local testing and feature development | Debug logging, dry-run mode | Basic |
| **Staging** | Pre-production testing and validation | Production-like setup with test data | Full monitoring |
| **Production** | Live trading operations | Optimized performance, security hardened | Enterprise-grade |

### Environment-Specific Features

- **Development**: Hot-reload, verbose logging, mock data support
- **Staging**: Blue-green deployments, performance testing, integration validation
- **Production**: High availability, auto-scaling, disaster recovery

## Prerequisites

### System Requirements

| Component | Minimum | Recommended | Production |
|-----------|---------|-------------|------------|
| **CPU** | 2 cores | 4 cores | 8+ cores |
| **Memory** | 4GB RAM | 8GB RAM | 16+ GB RAM |
| **Storage** | 10GB | 50GB SSD | 200+ GB NVMe |
| **Network** | 10 Mbps | 100 Mbps | 1+ Gbps |

### Software Dependencies

- **Go**: 1.24+ (exact version in `go.mod`)
- **Python**: 3.12+ with pip
- **Docker**: 20.10+ (for containerized deployments)
- **Kubernetes**: 1.20+ (for K8s deployments)
- **Git**: For source code management

## Local Development

### Quick Setup

1. **Clone and setup**:
   ```bash
   git clone https://github.com/your-repo/bitunix-bot.git
   cd bitunix-bot
   
   # Copy and configure settings
   cp config.yaml.example config.yaml
   nano config.yaml  # Edit with your preferences
   ```

2. **Install dependencies**:
   ```bash
   # Go dependencies
   go mod download && go mod verify
   
   # Python ML dependencies
   pip3 install -r scripts/requirements.txt
   
   # Verify installations
   go version
   python3 --version
   python3 -c "import onnxruntime; print('ONNX Runtime:', onnxruntime.__version__)"
   ```

3. **Development configuration**:
   ```bash
   # Create development environment file
   cat > .env.dev << EOF
   BITUNIX_API_KEY="your_development_api_key"
   BITUNIX_SECRET_KEY="your_development_secret_key"
   DRY_RUN=true
   LOG_LEVEL=debug
   CONFIG_FILE="config.yaml"
   ML_TIMEOUT=30s
   METRICS_PORT=8080
   EOF
   
   # Load environment
   source .env.dev
   ```

4. **Prepare ML model**:
   ```bash
   # Option 1: Train a new model with sample data
   python scripts/label_and_train.py --data-file scripts/training_data.json
   
   # Option 2: Use pre-trained model (if available)
   cp model-20250530.onnx model.onnx
   
   # Validate model
   python scripts/model_validation.py --model model.onnx --test-data scripts/test_data.json
   ```

5. **Run development server**:
   ```bash
   # Method 1: Direct Go execution
   go run cmd/bitrader/main.go
   
   # Method 2: Build and run binary
   go build -o bitrader cmd/bitrader/main.go
   ./bitrader
   
   # Method 3: Using air for hot-reload (install with: go install github.com/cosmtrek/air@latest)
   air
   ```

### Development Tools

1. **Testing**:
   ```bash
   # Run all tests
   go test ./...
   
   # Run with coverage
   go test -v -race -coverprofile=coverage.out ./...
   go tool cover -html=coverage.out -o coverage.html
   
   # Integration tests
   go test -v -tags=integration ./...
   
   # ML model tests
   python scripts/model_validation.py --model model.onnx --test-data test_data.json
   ```

2. **Linting and formatting**:
   ```bash
   # Format code
   go fmt ./...
   
   # Run linter (install golangci-lint first)
   golangci-lint run
   
   # Python code formatting
   python -m black scripts/
   python -m flake8 scripts/
   ```

3. **Local monitoring**:
   ```bash
   # Start monitoring in background
   python scripts/enhanced_monitoring.py --daemon --interval 30 &
   
   # View metrics
   curl http://localhost:8080/metrics
   
   # View logs
   tail -f logs/bitrader.log
   ```

### Development Configuration

Create a comprehensive `config.yaml` for development:

```yaml
# Development configuration
api:
  baseURL: "https://testnet-api.bitunix.com"  # Use testnet for development
  wsURL: "wss://testnet-fapi.bitunix.com/public"
  timeout: "10s"
  retryAttempts: 3

trading:
  symbols: ["BTCUSDT"]  # Start with single symbol
  baseSizeRatio: 0.001  # Smaller position sizes for testing
  dryRun: true          # Always true in development
  maxDailyLoss: 0.01    # 1% max loss for testing
  maxPositionSize: 0.005
  maxPriceDistance: 2.0

ml:
  modelPath: "model.onnx"
  probThreshold: 0.6    # Lower threshold for more signals in testing
  timeout: "30s"        # Longer timeout for debugging

features:
  vwapWindow: "30s"
  vwapSize: 300         # Smaller buffer for faster testing
  tickSize: 25

system:
  pingInterval: "30s"   # Less frequent pings in development
  dataPath: "./data"
  metricsPort: 8080
  restTimeout: "10s"
  logLevel: "debug"     # Verbose logging
```

## Staging Environment

The staging environment replicates production settings with test data and reduced scale.

### Staging Setup

1. **Infrastructure preparation**:
   ```bash
   # Create staging namespace
   kubectl create namespace bitunix-staging
   
   # Set up staging secrets
   kubectl create secret generic bitunix-staging-credentials \
     --from-literal=api-key="staging_api_key" \
     --from-literal=secret-key="staging_secret_key" \
     -n bitunix-staging
   
   # Create staging config
   kubectl create configmap staging-config \
     --from-file=config.yaml=staging-config.yaml \
     -n bitunix-staging
   ```

2. **Staging-specific configuration** (`staging-config.yaml`):
   ```yaml
   api:
     baseURL: "https://staging-api.bitunix.com"
     wsURL: "wss://staging-fapi.bitunix.com/public"
   
   trading:
     symbols: ["BTCUSDT", "ETHUSDT"]
     baseSizeRatio: 0.002
     dryRun: false  # Live trading with small amounts
     maxDailyLoss: 0.02
     maxPositionSize: 0.01
   
   ml:
     probThreshold: 0.65  # Production threshold
     timeout: "15s"
   
   system:
     logLevel: "info"     # Production logging level
     metricsPort: 8080
   ```

3. **Deploy to staging**:
   ```bash
   # Build staging image
   docker build -f deploy/Dockerfile -t bitunix-bot:staging .
   
   # Deploy with Helm
   helm upgrade --install bitunix-staging ./deploy/helm/bitunix-bot \
     --namespace bitunix-staging \
     --values staging-values.yaml \
     --set image.tag=staging
   ```

### Staging Validation

1. **Automated testing**:
   ```bash
   # Run staging test suite
   python scripts/staging_tests.py --endpoint http://staging-bitunix-bot:8080
   
   # Performance testing
   python scripts/load_test.py --target staging --duration 300s
   
   # ML model validation
   python scripts/model_validation.py \
     --model staging-model.onnx \
     --test-data staging-test-data.json \
     --baseline baseline-metrics.json
   ```

2. **Manual validation checklist**:
   - [ ] API connectivity and authentication
   - [ ] WebSocket connections stable
   - [ ] ML predictions generating reasonable outputs
   - [ ] Metrics collection working
   - [ ] Alerts firing correctly
   - [ ] Resource usage within expected bounds

## Docker Deployment

### Quick Start

1. Build the image:
   ```bash
   docker build -f deploy/Dockerfile -t bitunix-bot .
   ```

2. Run with environment variables:
   ```bash
   docker run -d \
     --name bitunix-bot \
     -e BITUNIX_API_KEY="your_key" \
     -e BITUNIX_SECRET_KEY="your_secret" \
     -e DRY_RUN=false \
     -p 8080:8080 \
     -v $(pwd)/data:/srv/data \
     -v $(pwd)/model.onnx:/srv/data/model.onnx:ro \
     bitunix-bot
   ```

### Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  bitunix-bot:
    build:
      context: .
      dockerfile: deploy/Dockerfile
    container_name: bitunix-bot
    restart: unless-stopped
    environment:
      - BITUNIX_API_KEY=${BITUNIX_API_KEY}
      - BITUNIX_SECRET_KEY=${BITUNIX_SECRET_KEY}
      - DRY_RUN=false
      - LOG_LEVEL=info
      - METRICS_PORT=8080
    ports:
      - "8080:8080"
    volumes:
      - ./data:/srv/data
      - ./model.onnx:/srv/data/model.onnx:ro
      - ./config.yaml:/srv/data/config.yaml:ro
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/metrics"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s

  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    restart: unless-stopped
    ports:
      - "9090:9090"
    volumes:
      - ./deploy/prometheus.yml:/etc/prometheus/prometheus.yml:ro
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/etc/prometheus/console_libraries'
      - '--web.console.templates=/etc/prometheus/consoles'

  grafana:
    image: grafana/grafana:latest
    container_name: grafana
    restart: unless-stopped
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-storage:/var/lib/grafana
      - ./deploy/grafana/dashboards:/etc/grafana/provisioning/dashboards:ro
      - ./deploy/grafana/datasources:/etc/grafana/provisioning/datasources:ro

volumes:
  grafana-storage:
```

Deploy with:
```bash
docker-compose up -d
```

### Production Docker Configuration

For production, use a more secure configuration:

```yaml
version: '3.8'

services:
  bitunix-bot:
    build:
      context: .
      dockerfile: deploy/Dockerfile
    container_name: bitunix-bot
    restart: unless-stopped
    read_only: true
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
    environment:
      - BITUNIX_API_KEY_FILE=/run/secrets/api_key
      - BITUNIX_SECRET_KEY_FILE=/run/secrets/secret_key
      - DRY_RUN=false
      - LOG_LEVEL=warn
    secrets:
      - api_key
      - secret_key
    volumes:
      - ./data:/srv/data
      - ./model.onnx:/srv/data/model.onnx:ro
      - tmpfs:/tmp
    tmpfs:
      - /tmp:noexec,nosuid,size=100m
    ulimits:
      nofile:
        soft: 1024
        hard: 1024
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"

secrets:
  api_key:
    file: ./secrets/api_key.txt
  secret_key:
    file: ./secrets/secret_key.txt
```

## Kubernetes Deployment

### Prerequisites

- Kubernetes cluster (1.20+)
- kubectl configured
- Helm (optional)

### Basic Deployment

1. Create namespace:
   ```bash
   kubectl create namespace bitunix-bot
   ```

2. Create secrets:
   ```bash
   kubectl create secret generic bitunix-credentials \
     --from-literal=api-key="your_api_key" \
     --from-literal=secret-key="your_secret_key" \
     -n bitunix-bot
   ```

3. Deploy the application:

```yaml
# deploy/k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bitunix-bot
  namespace: bitunix-bot
  labels:
    app: bitunix-bot
spec:
  replicas: 1
  selector:
    matchLabels:
      app: bitunix-bot
  template:
    metadata:
      labels:
        app: bitunix-bot
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
      containers:
      - name: bitunix-bot
        image: bitunix-bot:latest
        imagePullPolicy: Always
        ports:
        - containerPort: 8080
          name: metrics
        env:
        - name: BITUNIX_API_KEY
          valueFrom:
            secretKeyRef:
              name: bitunix-credentials
              key: api-key
        - name: BITUNIX_SECRET_KEY
          valueFrom:
            secretKeyRef:
              name: bitunix-credentials
              key: secret-key
        - name: DRY_RUN
          value: "false"
        - name: LOG_LEVEL
          value: "info"
        - name: METRICS_PORT
          value: "8080"
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /metrics
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /metrics
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
        volumeMounts:
        - name: model-volume
          mountPath: /srv/data/model.onnx
          subPath: model.onnx
          readOnly: true
        - name: config-volume
          mountPath: /srv/data/config.yaml
          subPath: config.yaml
          readOnly: true
        - name: data-volume
          mountPath: /srv/data
      volumes:
      - name: model-volume
        configMap:
          name: ml-model
      - name: config-volume
        configMap:
          name: bot-config
      - name: data-volume
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: bitunix-bot-service
  namespace: bitunix-bot
  labels:
    app: bitunix-bot
spec:
  selector:
    app: bitunix-bot
  ports:
  - port: 8080
    targetPort: 8080
    name: metrics
  type: ClusterIP
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: bot-config
  namespace: bitunix-bot
data:
  config.yaml: |
    api:
      baseURL: "https://api.bitunix.com"
      wsURL: "wss://fapi.bitunix.com/public"
    trading:
      symbols: ["BTCUSDT", "ETHUSDT"]
      baseSizeRatio: 0.002
      dryRun: false
      maxDailyLoss: 0.05
      maxPositionSize: 0.01
      maxPriceDistance: 3.0
    ml:
      modelPath: "model.onnx"
      probThreshold: 0.65
    features:
      vwapWindow: "30s"
      vwapSize: 600
      tickSize: 50
    system:
      pingInterval: "15s"
      dataPath: ""
      metricsPort: 8080
      restTimeout: "5s"
```

Apply the configuration:
```bash
kubectl apply -f deploy/k8s/deployment.yaml
```

### Helm Chart

Create a Helm chart for more flexible deployments:

```bash
# Create chart structure
helm create bitunix-bot-chart
cd bitunix-bot-chart
```

`values.yaml`:
```yaml
replicaCount: 1

image:
  repository: bitunix-bot
  pullPolicy: Always
  tag: "latest"

nameOverride: ""
fullnameOverride: ""

serviceAccount:
  create: true
  annotations: {}
  name: ""

podAnnotations: {}

podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  fsGroup: 65532

securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop:
    - ALL
  readOnlyRootFilesystem: true

service:
  type: ClusterIP
  port: 8080

resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi

autoscaling:
  enabled: false
  minReplicas: 1
  maxReplicas: 3
  targetCPUUtilizationPercentage: 80

nodeSelector: {}

tolerations: []

affinity: {}

# Bot-specific configuration
bot:
  dryRun: false
  logLevel: info
  config:
    api:
      baseURL: "https://api.bitunix.com"
      wsURL: "wss://fapi.bitunix.com/public"
    trading:
      symbols: ["BTCUSDT"]
      baseSizeRatio: 0.002
      maxDailyLoss: 0.05
      maxPositionSize: 0.01
      maxPriceDistance: 3.0
    ml:
      probThreshold: 0.65
    features:
      vwapWindow: "30s"
      vwapSize: 600
      tickSize: 50
    system:
      pingInterval: "15s"
      metricsPort: 8080
      restTimeout: "5s"

# Secrets (these should be set via --set during install)
secrets:
  apiKey: ""
  secretKey: ""
```

Deploy with Helm:
```bash
helm install bitunix-bot ./bitunix-bot-chart \
  --set secrets.apiKey="your_api_key" \
  --set secrets.secretKey="your_secret_key" \
  --namespace bitunix-bot \
  --create-namespace
```

## Cloud Platforms

### AWS EKS

#### Complete EKS Setup

1. **Prerequisites**:
   ```bash
   # Install required tools
   curl --silent --location "https://github.com/weaveworks/eksctl/releases/latest/download/eksctl_$(uname -s)_amd64.tar.gz" | tar xz -C /tmp
   sudo mv /tmp/eksctl /usr/local/bin
   
   # Install AWS CLI v2
   curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
   unzip awscliv2.zip && sudo ./aws/install
   
   # Configure AWS credentials
   aws configure
   ```

2. **Create EKS cluster**:
   ```bash
   # Create cluster with optimized configuration
   eksctl create cluster \
     --name bitunix-bot-prod \
     --region us-west-2 \
     --version 1.28 \
     --nodegroup-name standard-workers \
     --node-type t3.medium \
     --nodes 2 \
     --nodes-min 1 \
     --nodes-max 4 \
     --managed \
     --with-oidc \
     --ssh-access \
     --ssh-public-key ~/.ssh/id_rsa.pub
   ```

3. **Install AWS Load Balancer Controller**:
   ```bash
   # Add the EKS chart repo
   helm repo add eks https://aws.github.io/eks-charts
   
   # Install AWS Load Balancer Controller
   helm install aws-load-balancer-controller eks/aws-load-balancer-controller \
     -n kube-system \
     --set clusterName=bitunix-bot-prod \
     --set serviceAccount.create=false \
     --set serviceAccount.name=aws-load-balancer-controller
   ```

4. **Set up AWS Secrets Manager integration**:
   ```bash
   # Install Secrets Store CSI Driver
   helm repo add secrets-store-csi-driver https://kubernetes-sigs.github.io/secrets-store-csi-driver/charts
   helm install csi-secrets-store secrets-store-csi-driver/secrets-store-csi-driver \
     --namespace kube-system
   
   # Install AWS provider
   kubectl apply -f https://raw.githubusercontent.com/aws/secrets-store-csi-driver-provider-aws/main/deployment/aws-provider-installer.yaml
   ```

5. **Create secrets in AWS Secrets Manager**:
   ```bash
   # Store API credentials
   aws secretsmanager create-secret \
     --name bitunix-bot/api-credentials \
     --description "Bitunix API credentials for trading bot" \
     --secret-string '{"api_key":"your_api_key","secret_key":"your_secret_key"}'
   ```

6. **Deploy with Secrets Manager integration**:
   ```yaml
   # aws-secretproviderclass.yaml
   apiVersion: secrets-store.csi.x-k8s.io/v1
   kind: SecretProviderClass
   metadata:
     name: bitunix-credentials
     namespace: bitunix-bot
   spec:
     provider: aws
     parameters:
       objects: |
         - objectName: "bitunix-bot/api-credentials"
           objectType: "secretsmanager"
           jmesPath:
             - path: "api_key"
               objectAlias: "api-key"
             - path: "secret_key"
               objectAlias: "secret-key"
     secretObjects:
     - secretName: bitunix-credentials
       type: Opaque
       data:
       - objectName: "api-key"
         key: "api-key"
       - objectName: "secret-key"
         key: "secret-key"
   ```

7. **CloudWatch logging setup**:
   ```bash
   # Enable CloudWatch Container Insights
   curl https://raw.githubusercontent.com/aws-samples/amazon-cloudwatch-container-insights/latest/k8s-deployment-manifest-templates/deployment-mode/daemonset/container-insights-monitoring/quickstart/cwagent-fluentd-quickstart.yaml | sed "s/{{cluster_name}}/bitunix-bot-prod/;s/{{region_name}}/us-west-2/" | kubectl apply -f -
   ```

### Google GKE

#### Complete GKE Setup

1. **Prerequisites**:
   ```bash
   # Install Google Cloud SDK
   curl https://sdk.cloud.google.com | bash
   exec -l $SHELL
   gcloud init
   
   # Enable required APIs
   gcloud services enable container.googleapis.com
   gcloud services enable secretmanager.googleapis.com
   ```

2. **Create GKE cluster with security best practices**:
   ```bash
   gcloud container clusters create bitunix-bot-prod \
     --zone us-central1-a \
     --machine-type e2-standard-2 \
     --num-nodes 2 \
     --enable-autoscaling \
     --min-nodes 1 \
     --max-nodes 4 \
     --enable-autorepair \
     --enable-autoupgrade \
     --enable-network-policy \
     --enable-ip-alias \
     --enable-shielded-nodes \
     --workload-pool=$(gcloud config get-value project).svc.id.goog \
     --addons HorizontalPodAutoscaling,HttpLoadBalancing,NetworkPolicy
   ```

3. **Set up Workload Identity**:
   ```bash
   # Create Google Service Account
   gcloud iam service-accounts create bitunix-bot-gsa \
     --display-name="Bitunix Bot Service Account"
   
   # Grant Secret Manager access
   gcloud projects add-iam-policy-binding $(gcloud config get-value project) \
     --member="serviceAccount:bitunix-bot-gsa@$(gcloud config get-value project).iam.gserviceaccount.com" \
     --role="roles/secretmanager.secretAccessor"
   
   # Create Kubernetes Service Account
   kubectl create serviceaccount bitunix-bot-ksa \
     --namespace bitunix-bot
   
   # Bind the accounts
   gcloud iam service-accounts add-iam-policy-binding \
     bitunix-bot-gsa@$(gcloud config get-value project).iam.gserviceaccount.com \
     --role roles/iam.workloadIdentityUser \
     --member "serviceAccount:$(gcloud config get-value project).svc.id.goog[bitunix-bot/bitunix-bot-ksa]"
   
   kubectl annotate serviceaccount bitunix-bot-ksa \
     --namespace bitunix-bot \
     iam.gke.io/gcp-service-account=bitunix-bot-gsa@$(gcloud config get-value project).iam.gserviceaccount.com
   ```

4. **Create secrets in Secret Manager**:
   ```bash
   # Store API credentials
   echo -n "your_api_key" | gcloud secrets create bitunix-api-key --data-file=-
   echo -n "your_secret_key" | gcloud secrets create bitunix-secret-key --data-file=-
   ```

5. **Deploy with Secret Manager integration**:
   ```yaml
   # gke-deployment.yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: bitunix-bot
     namespace: bitunix-bot
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: bitunix-bot
     template:
       metadata:
         labels:
           app: bitunix-bot
       spec:
         serviceAccountName: bitunix-bot-ksa
         containers:
         - name: bitunix-bot
           image: gcr.io/$(gcloud config get-value project)/bitunix-bot:latest
           env:
           - name: BITUNIX_API_KEY
             valueFrom:
               secretKeyRef:
                 name: bitunix-gcp-secrets
                 key: api-key
           - name: BITUNIX_SECRET_KEY
             valueFrom:
               secretKeyRef:
                 name: bitunix-gcp-secrets
                 key: secret-key
           volumeMounts:
           - name: secrets-store
             mountPath: "/mnt/secrets"
             readOnly: true
         volumes:
         - name: secrets-store
           csi:
             driver: secrets-store.csi.k8s.io
             readOnly: true
             volumeAttributes:
               secretProviderClass: "bitunix-gcp-secrets"
   ---
   apiVersion: secrets-store.csi.x-k8s.io/v1
   kind: SecretProviderClass
   metadata:
     name: bitunix-gcp-secrets
     namespace: bitunix-bot
   spec:
     provider: gcp
     parameters:
       secrets: |
         - resourceName: "projects/$(gcloud config get-value project)/secrets/bitunix-api-key/versions/latest"
           path: "api-key"
         - resourceName: "projects/$(gcloud config get-value project)/secrets/bitunix-secret-key/versions/latest"
           path: "secret-key"
     secretObjects:
     - secretName: bitunix-gcp-secrets
       type: Opaque
       data:
       - objectName: "api-key"
         key: "api-key"
       - objectName: "secret-key"
         key: "secret-key"
   ```

6. **Set up Google Cloud Operations (Stackdriver)**:
   ```bash
   # Install Google Cloud Operations for GKE
   kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/master/k8s-stackdriver-logging.yaml
   kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/master/k8s-stackdriver-monitoring.yaml
   ```

### Azure AKS

#### Complete AKS Setup

1. **Prerequisites**:
   ```bash
   # Install Azure CLI
   curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
   
   # Login to Azure
   az login
   
   # Create resource group
   az group create --name bitunix-bot-rg --location eastus
   ```

2. **Create AKS cluster with security features**:
   ```bash
   az aks create \
     --resource-group bitunix-bot-rg \
     --name bitunix-bot-prod \
     --node-count 2 \
     --node-vm-size Standard_D2s_v3 \
     --enable-addons monitoring,azure-keyvault-secrets-provider \
     --enable-managed-identity \
     --enable-cluster-autoscaler \
     --min-count 1 \
     --max-count 4 \
     --network-plugin azure \
     --network-policy azure \
     --generate-ssh-keys \
     --kubernetes-version 1.28.0
   ```

3. **Get AKS credentials**:
   ```bash
   az aks get-credentials --resource-group bitunix-bot-rg --name bitunix-bot-prod
   ```

4. **Create Azure Key Vault**:
   ```bash
   # Create Key Vault
   az keyvault create \
     --name bitunix-bot-kv \
     --resource-group bitunix-bot-rg \
     --location eastus \
     --enable-soft-delete \
     --enable-purge-protection
   
   # Add secrets
   az keyvault secret set --vault-name bitunix-bot-kv --name api-key --value "your_api_key"
   az keyvault secret set --vault-name bitunix-bot-kv --name secret-key --value "your_secret_key"
   ```

5. **Set up Managed Identity for Key Vault access**:
   ```bash
   # Get the managed identity
   IDENTITY_CLIENT_ID=$(az aks show --resource-group bitunix-bot-rg --name bitunix-bot-prod --query addonProfiles.azureKeyvaultSecretsProvider.identity.clientId -o tsv)
   
   # Grant Key Vault access
   az keyvault set-policy \
     --name bitunix-bot-kv \
     --secret-permissions get \
     --spn $IDENTITY_CLIENT_ID
   ```

6. **Deploy with Azure Key Vault integration**:
   ```yaml
   # azure-deployment.yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: bitunix-bot
     namespace: bitunix-bot
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: bitunix-bot
     template:
       metadata:
         labels:
           app: bitunix-bot
       spec:
         containers:
         - name: bitunix-bot
           image: bitunixbot.azurecr.io/bitunix-bot:latest
           env:
           - name: BITUNIX_API_KEY
             valueFrom:
               secretKeyRef:
                 name: bitunix-azure-secrets
                 key: api-key
           - name: BITUNIX_SECRET_KEY
             valueFrom:
               secretKeyRef:
                 name: bitunix-azure-secrets
                 key: secret-key
           volumeMounts:
           - name: secrets-store
             mountPath: "/mnt/secrets"
             readOnly: true
         volumes:
         - name: secrets-store
           csi:
             driver: secrets-store.csi.k8s.io
             readOnly: true
             volumeAttributes:
               secretProviderClass: "bitunix-azure-secrets"
   ---
   apiVersion: secrets-store.csi.x-k8s.io/v1
   kind: SecretProviderClass
   metadata:
     name: bitunix-azure-secrets
     namespace: bitunix-bot
   spec:
     provider: azure
     parameters:
       usePodIdentity: "false"
       useVMManagedIdentity: "true"
       userAssignedIdentityID: "$IDENTITY_CLIENT_ID"
       keyvaultName: "bitunix-bot-kv"
       tenantId: "$(az account show --query tenantId -o tsv)"
       objects: |
         array:
           - |
             objectName: api-key
             objectType: secret
           - |
             objectName: secret-key
             objectType: secret
     secretObjects:
     - secretName: bitunix-azure-secrets
       type: Opaque
       data:
       - objectName: "api-key"
         key: "api-key"
       - objectName: "secret-key"
         key: "secret-key"
   ```

7. **Set up Azure Monitor**:
   ```bash
   # Create Log Analytics workspace
   az monitor log-analytics workspace create \
     --resource-group bitunix-bot-rg \
     --workspace-name bitunix-bot-logs
   
   # Get workspace ID
   WORKSPACE_ID=$(az monitor log-analytics workspace show \
     --resource-group bitunix-bot-rg \
     --workspace-name bitunix-bot-logs \
     --query customerId -o tsv)
   
   # Enable Container Insights
   az aks enable-addons \
     --resource-group bitunix-bot-rg \
     --name bitunix-bot-prod \
     --addons monitoring \
     --workspace-resource-id /subscriptions/$(az account show --query id -o tsv)/resourcegroups/bitunix-bot-rg/providers/microsoft.operationalinsights/workspaces/bitunix-bot-logs
   ```

## Production Deployment

### Production-Ready Configuration

#### High Availability Setup

1. **Multi-replica deployment**:
   ```yaml
   # production-deployment.yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: bitunix-bot
     namespace: bitunix-bot
   spec:
     replicas: 3  # Multiple replicas for HA
     strategy:
       type: RollingUpdate
       rollingUpdate:
         maxSurge: 1
         maxUnavailable: 0  # Ensure zero downtime
     selector:
       matchLabels:
         app: bitunix-bot
     template:
       metadata:
         labels:
           app: bitunix-bot
         annotations:
           prometheus.io/scrape: "true"
           prometheus.io/port: "8080"
           prometheus.io/path: "/metrics"
       spec:
         securityContext:
           runAsNonRoot: true
           runAsUser: 65532
           runAsGroup: 65532
           fsGroup: 65532
         affinity:
           podAntiAffinity:
             preferredDuringSchedulingIgnoredDuringExecution:
             - weight: 100
               podAffinityTerm:
                 labelSelector:
                   matchExpressions:
                   - key: app
                     operator: In
                     values:
                     - bitunix-bot
                 topologyKey: kubernetes.io/hostname
         containers:
         - name: bitunix-bot
           image: bitunix-bot:production
           imagePullPolicy: Always
           ports:
           - containerPort: 8080
             name: metrics
           env:
           - name: BITUNIX_API_KEY
             valueFrom:
               secretKeyRef:
                 name: bitunix-credentials
                 key: api-key
           - name: BITUNIX_SECRET_KEY
             valueFrom:
               secretKeyRef:
                 name: bitunix-credentials
                 key: secret-key
           - name: DRY_RUN
             value: "false"
           - name: LOG_LEVEL
             value: "warn"  # Production logging level
           - name: METRICS_PORT
             value: "8080"
           - name: GOMAXPROCS
             valueFrom:
               resourceFieldRef:
                 resource: limits.cpu
           resources:
             requests:
               memory: "256Mi"
               cpu: "200m"
             limits:
               memory: "1Gi"
               cpu: "1000m"
           livenessProbe:
             httpGet:
               path: /metrics
               port: 8080
             initialDelaySeconds: 60
             periodSeconds: 30
             timeoutSeconds: 10
             failureThreshold: 3
           readinessProbe:
             httpGet:
               path: /metrics
               port: 8080
             initialDelaySeconds: 30
             periodSeconds: 10
             timeoutSeconds: 5
             failureThreshold: 2
           startupProbe:
             httpGet:
               path: /metrics
               port: 8080
             initialDelaySeconds: 30
             periodSeconds: 10
             timeoutSeconds: 5
             failureThreshold: 10
           securityContext:
             allowPrivilegeEscalation: false
             capabilities:
               drop:
               - ALL
             readOnlyRootFilesystem: true
           volumeMounts:
           - name: model-volume
             mountPath: /srv/data/model.onnx
             subPath: model.onnx
             readOnly: true
           - name: config-volume
             mountPath: /srv/data/config.yaml
             subPath: config.yaml
             readOnly: true
           - name: data-volume
             mountPath: /srv/data
           - name: tmp-volume
             mountPath: /tmp
         volumes:
         - name: model-volume
           configMap:
             name: ml-model
         - name: config-volume
           configMap:
             name: bot-config
         - name: data-volume
           emptyDir: {}
         - name: tmp-volume
           emptyDir:
             sizeLimit: 100Mi
   ---
   apiVersion: policy/v1
   kind: PodDisruptionBudget
   metadata:
     name: bitunix-bot-pdb
     namespace: bitunix-bot
   spec:
     minAvailable: 2
     selector:
       matchLabels:
         app: bitunix-bot
   ```

2. **Horizontal Pod Autoscaler**:
   ```yaml
   apiVersion: autoscaling/v2
   kind: HorizontalPodAutoscaler
   metadata:
     name: bitunix-bot-hpa
     namespace: bitunix-bot
   spec:
     scaleTargetRef:
       apiVersion: apps/v1
       kind: Deployment
       name: bitunix-bot
     minReplicas: 3
     maxReplicas: 10
     metrics:
     - type: Resource
       resource:
         name: cpu
         target:
           type: Utilization
           averageUtilization: 70
     - type: Resource
       resource:
         name: memory
         target:
           type: Utilization
           averageUtilization: 80
     behavior:
       scaleDown:
         stabilizationWindowSeconds: 300
         policies:
         - type: Percent
           value: 10
           periodSeconds: 60
       scaleUp:
         stabilizationWindowSeconds: 60
         policies:
         - type: Percent
           value: 50
           periodSeconds: 30
   ```

#### Production Configuration Template

```yaml
# production-config.yaml
api:
  baseURL: "https://api.bitunix.com"
  wsURL: "wss://fapi.bitunix.com/public"
  timeout: "5s"
  retryAttempts: 3
  rateLimitPerSecond: 10

trading:
  symbols: ["BTCUSDT", "ETHUSDT", "ADAUSDT", "DOTUSDT"]
  baseSizeRatio: 0.001  # Conservative position sizing
  dryRun: false
  maxDailyLoss: 0.02    # 2% max daily loss
  maxPositionSize: 0.005
  maxPriceDistance: 2.5
  riskManagement:
    enabled: true
    stopLoss: 0.02      # 2% stop loss
    takeProfit: 0.04    # 4% take profit
    maxOpenPositions: 3

ml:
  modelPath: "model.onnx"
  probThreshold: 0.7    # Higher threshold for production
  timeout: "10s"
  fallbackStrategy: "conservative"  # Conservative when ML fails
  validationEnabled: true
  minConfidence: 0.6

features:
  vwapWindow: "30s"
  vwapSize: 1200        # Larger buffer for stability
  tickSize: 100
  technicalIndicators:
    rsi: true
    macd: true
    bollinger: true

system:
  pingInterval: "10s"
  dataPath: "/srv/data"
  metricsPort: 8080
  restTimeout: "3s"
  logLevel: "warn"      # Production logging
  maxMemoryUsage: 512   # MB
  gcPercent: 20         # Conservative GC

monitoring:
  enabled: true
  alerting:
    enabled: true
    channels: ["slack", "email"]
    thresholds:
      errorRate: 0.05
      latency: 1000  # ms
      memoryUsage: 80  # percent
```

## Monitoring and Observability

### Comprehensive Monitoring Stack

#### Prometheus Configuration

```yaml
# prometheus-config.yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "bitunix-bot-rules.yml"

alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - alertmanager:9093

scrape_configs:
  - job_name: 'bitunix-bot'
    static_configs:
      - targets: ['bitunix-bot-service:8080']
    scrape_interval: 10s
    metrics_path: /metrics
    
  - job_name: 'kubernetes-apiservers'
    kubernetes_sd_configs:
      - role: endpoints
    scheme: https
    tls_config:
      ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
    relabel_configs:
      - source_labels: [__meta_kubernetes_namespace, __meta_kubernetes_service_name, __meta_kubernetes_endpoint_port_name]
        action: keep
        regex: default;kubernetes;https

  - job_name: 'kubernetes-nodes'
    kubernetes_sd_configs:
      - role: node
    scheme: https
    tls_config:
      ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
```

#### Alerting Rules

```yaml
# bitunix-bot-rules.yml
groups:
  - name: bitunix-bot
    rules:
      # Application Health
      - alert: BitunixBotDown
        expr: up{job="bitunix-bot"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Bitunix Bot is down"
          description: "Bitunix Bot has been down for more than 1 minute"

      # Trading Performance
      - alert: HighErrorRate
        expr: rate(bitunix_errors_total[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High error rate detected"
          description: "Error rate is {{ $value }} errors per second"

      - alert: MLModelTimeout
        expr: increase(bitunix_ml_timeouts_total[5m]) > 5
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "ML model timeouts increasing"
          description: "{{ $value }} ML timeouts in the last 5 minutes"

      # Resource Usage
      - alert: HighMemoryUsage
        expr: container_memory_usage_bytes{pod=~"bitunix-bot-.*"} / container_spec_memory_limit_bytes > 0.9
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage"
          description: "Memory usage is above 90% for {{ $labels.pod }}"

      - alert: HighCPUUsage
        expr: rate(container_cpu_usage_seconds_total{pod=~"bitunix-bot-.*"}[5m]) / (container_spec_cpu_quota / container_spec_cpu_period) > 0.8
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High CPU usage"
          description: "CPU usage is above 80% for {{ $labels.pod }}"

      # Trading Specific
      - alert: ExcessiveLosses
        expr: bitunix_daily_pnl < -0.05
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Excessive daily losses"
          description: "Daily P&L is {{ $value }}, exceeding loss threshold"

      - alert: LowMLAccuracy
        expr: bitunix_ml_accuracy < 0.6
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "ML model accuracy degraded"
          description: "ML accuracy is {{ $value }}, below acceptable threshold"
```

#### Grafana Dashboards

Create comprehensive dashboards:

1. **Trading Performance Dashboard**:
   ```json
   {
     "dashboard": {
       "title": "Bitunix Bot - Trading Performance",
       "panels": [
         {
           "title": "Total Trades",
           "type": "stat",
           "targets": [
             {
               "expr": "increase(bitunix_trades_total[24h])"
             }
           ]
         },
         {
           "title": "Daily P&L",
           "type": "stat",
           "targets": [
             {
               "expr": "bitunix_daily_pnl"
             }
           ]
         },
         {
           "title": "ML Model Accuracy",
           "type": "gauge",
           "targets": [
             {
               "expr": "bitunix_ml_accuracy"
             }
           ]
         },
         {
           "title": "Trade Volume",
           "type": "graph",
           "targets": [
             {
               "expr": "rate(bitunix_trade_volume_total[5m])"
             }
           ]
         }
       ]
     }
   }
   ```

2. **System Health Dashboard**:
   ```json
   {
     "dashboard": {
       "title": "Bitunix Bot - System Health",
       "panels": [
         {
           "title": "Pod Status",
           "type": "stat",
           "targets": [
             {
               "expr": "kube_pod_status_phase{pod=~\"bitunix-bot-.*\"}"
             }
           ]
         },
         {
           "title": "Memory Usage",
           "type": "graph",
           "targets": [
             {
               "expr": "container_memory_usage_bytes{pod=~\"bitunix-bot-.*\"}"
             }
           ]
         },
         {
           "title": "CPU Usage",
           "type": "graph",
           "targets": [
             {
               "expr": "rate(container_cpu_usage_seconds_total{pod=~\"bitunix-bot-.*\"}[5m])"
             }
           ]
         },
         {
           "title": "Network I/O",
           "type": "graph",
           "targets": [
             {
               "expr": "rate(container_network_receive_bytes_total{pod=~\"bitunix-bot-.*\"}[5m])"
             }
           ]
         }
       ]
     }
   }
   ```

### Advanced Logging

#### Structured Logging Configuration

```yaml
# fluent-bit-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluent-bit-config
  namespace: bitunix-bot
data:
  fluent-bit.conf: |
    [SERVICE]
        Flush         1
        Log_Level     info
        Daemon        off
        Parsers_File  parsers.conf
        HTTP_Server   On
        HTTP_Listen   0.0.0.0
        HTTP_Port     2020

    [INPUT]
        Name              tail
        Path              /var/log/containers/bitunix-bot*.log
        Parser            cri
        Tag               kube.*
        Refresh_Interval  5
        Mem_Buf_Limit     5MB
        Skip_Long_Lines   On

    [FILTER]
        Name                kubernetes
        Match               kube.*
        Kube_URL            https://kubernetes.default.svc:443
        Kube_CA_File        /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
        Kube_Token_File     /var/run/secrets/kubernetes.io/serviceaccount/token
        Kube_Tag_Prefix     kube.var.log.containers.
        Merge_Log           On
        Keep_Log            Off
        K8S-Logging.Parser  On
        K8S-Logging.Exclude On

    [OUTPUT]
        Name  es
        Match *
        Host  elasticsearch
        Port  9200
        Index bitunix-bot-logs
        Type  _doc

  parsers.conf: |
    [PARSER]
        Name   cri
        Format regex
        Regex  ^(?<time>[^ ]+) (?<stream>stdout|stderr) (?<logtag>[^ ]*) (?<message>.*)$
        Time_Key    time
        Time_Format %Y-%m-%dT%H:%M:%S.%L%z
```

## Security Considerations

### Container Security Hardening

1. **Security Context**:
   ```yaml
   securityContext:
     runAsNonRoot: true
     runAsUser: 65532
     runAsGroup: 65532
     fsGroup: 65532
     seccompProfile:
       type: RuntimeDefault
     capabilities:
       drop:
       - ALL
     allowPrivilegeEscalation: false
     readOnlyRootFilesystem: true
   ```

2. **Network Policies**:
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
         port: 53   # DNS
       - protocol: UDP
         port: 53   # DNS
     - to:
       - namespaceSelector:
           matchLabels:
             name: kube-system
   ```

3. **Pod Security Standards**:
   ```yaml
   apiVersion: v1
   kind: Namespace
   metadata:
     name: bitunix-bot
     labels:
       pod-security.kubernetes.io/enforce: restricted
       pod-security.kubernetes.io/audit: restricted
       pod-security.kubernetes.io/warn: restricted
   ```

### API Security

1. **API Key Management**:
   ```bash
   # Rotate API keys regularly (monthly)
   # Use different keys for different environments
   # Implement key rotation automation
   
   # Example rotation script
   #!/bin/bash
   NEW_API_KEY=$(generate_new_api_key)
   kubectl patch secret bitunix-credentials \
     -p='{"data":{"api-key":"'$(echo -n $NEW_API_KEY | base64)'"}}' \
     -n bitunix-bot
   
   # Rolling restart to pick up new keys
   kubectl rollout restart deployment/bitunix-bot -n bitunix-bot
   ```

2. **TLS Configuration**:
   ```yaml
   # Ensure all communications use TLS
   spec:
     template:
       spec:
         containers:
         - name: bitunix-bot
           env:
           - name: BITUNIX_API_BASE_URL
             value: "https://api.bitunix.com"  # Always HTTPS
           - name: BITUNIX_WS_URL
             value: "wss://fapi.bitunix.com/public"  # Always WSS
   ```

### Vulnerability Scanning

1. **Container Image Scanning**:
   ```bash
   # Scan images for vulnerabilities
   trivy image bitunix-bot:latest
   
   # Fail CI/CD pipeline on high severity vulnerabilities
   trivy image --exit-code 1 --severity HIGH,CRITICAL bitunix-bot:latest
   ```

2. **Kubernetes Security Scanning**:
   ```bash
   # Scan Kubernetes manifests
   kubesec scan deployment.yaml
   
   # Use kube-bench for CIS benchmarks
   kubectl apply -f https://raw.githubusercontent.com/aquasecurity/kube-bench/main/job.yaml
   ```

## Backup and Recovery

### Data Backup Strategy

1. **Configuration Backup**:
   ```bash
   #!/bin/bash
   # backup-config.sh
   
   BACKUP_DIR="/backups/$(date +%Y%m%d)"
   mkdir -p $BACKUP_DIR
   
   # Backup Kubernetes configurations
   kubectl get all -n bitunix-bot -o yaml > $BACKUP_DIR/k8s-resources.yaml
   kubectl get secrets -n bitunix-bot -o yaml > $BACKUP_DIR/secrets.yaml
   kubectl get configmaps -n bitunix-bot -o yaml > $BACKUP_DIR/configmaps.yaml
   
   # Backup application configuration
   cp config.yaml $BACKUP_DIR/
   cp model.onnx $BACKUP_DIR/
   
   # Backup to cloud storage
   aws s3 sync $BACKUP_DIR s3://bitunix-bot-backups/$(date +%Y%m%d)/
   ```

2. **ML Model Versioning**:
   ```bash
   #!/bin/bash
   # model-backup.sh
   
   MODEL_VERSION=$(date +%Y%m%d_%H%M%S)
   MODEL_BACKUP_PATH="s3://bitunix-models/backups/model-${MODEL_VERSION}.onnx"
   
   # Backup current model
   aws s3 cp model.onnx $MODEL_BACKUP_PATH
   
   # Update model registry
   echo "${MODEL_VERSION},${MODEL_BACKUP_PATH},$(date)" >> model_registry.csv
   aws s3 cp model_registry.csv s3://bitunix-models/registry/
   ```

3. **Database Backup (if applicable)**:
   ```bash
   #!/bin/bash
   # database-backup.sh
   
   BACKUP_FILE="bitunix_db_$(date +%Y%m%d_%H%M%S).sql"
   
   # Create database dump
   pg_dump -h $DB_HOST -U $DB_USER -d $DB_NAME > $BACKUP_FILE
   
   # Encrypt and upload
   gpg --encrypt --recipient backup@company.com $BACKUP_FILE
   aws s3 cp ${BACKUP_FILE}.gpg s3://bitunix-db-backups/
   
   # Cleanup local files
   rm $BACKUP_FILE ${BACKUP_FILE}.gpg
   ```

### Disaster Recovery Plan

1. **Recovery Procedures**:
   ```bash
   #!/bin/bash
   # disaster-recovery.sh
   
   echo "Starting disaster recovery process..."
   
   # 1. Create new namespace
   kubectl create namespace bitunix-bot-recovery
   
   # 2. Restore secrets
   kubectl apply -f backups/latest/secrets.yaml -n bitunix-bot-recovery
   
   # 3. Restore configurations
   kubectl apply -f backups/latest/configmaps.yaml -n bitunix-bot-recovery
   
   # 4. Deploy application
   kubectl apply -f backups/latest/k8s-resources.yaml -n bitunix-bot-recovery
   
   # 5. Update DNS/load balancer to point to recovery instance
   # 6. Verify functionality
   
   echo "Disaster recovery process completed"
   ```

2. **RTO/RPO Targets**:
   - **Recovery Time Objective (RTO)**: 15 minutes
   - **Recovery Point Objective (RPO)**: 5 minutes
   - **Backup Frequency**: Every 4 hours
   - **Backup Retention**: 30 days

### High Availability Setup

1. **Multi-Region Deployment**:
   ```yaml
   # regions: us-east-1, us-west-2, eu-west-1
   apiVersion: argoproj.io/v1alpha1
   kind: Application
   metadata:
     name: bitunix-bot-multiregion
   spec:
     source:
       repoURL: https://github.com/your-repo/bitunix-bot
       path: deploy/helm
       helm:
         valueFiles:
         - values-multiregion.yaml
     destinations:
     - server: https://eks-us-east-1.cluster
       namespace: bitunix-bot
     - server: https://eks-us-west-2.cluster
       namespace: bitunix-bot
     - server: https://eks-eu-west-1.cluster
       namespace: bitunix-bot
   ```

2. **Cross-Region Failover**:
   ```bash
   #!/bin/bash
   # failover.sh
   
   PRIMARY_REGION="us-east-1"
   SECONDARY_REGION="us-west-2"
   
   # Check primary region health
   if ! kubectl get pods -n bitunix-bot --context $PRIMARY_REGION | grep -q Running; then
     echo "Primary region unhealthy, initiating failover"
     
     # Scale up secondary region
     kubectl scale deployment bitunix-bot --replicas=3 \
       --context $SECONDARY_REGION -n bitunix-bot
     
     # Update DNS to point to secondary region
     aws route53 change-resource-record-sets \
       --hosted-zone-id Z123456789 \
       --change-batch file://failover-changeset.json
     
     echo "Failover completed"
   fi
   ```

## Performance Optimization

### Resource Optimization

1. **CPU and Memory Tuning**:
   ```yaml
   resources:
     requests:
       memory: "256Mi"
       cpu: "200m"
     limits:
       memory: "1Gi"
       cpu: "1000m"
   env:
   - name: GOMAXPROCS
     valueFrom:
       resourceFieldRef:
         resource: limits.cpu
   - name: GOMEMLIMIT
     valueFrom:
       resourceFieldRef:
         resource: limits.memory
   ```

2. **JVM Tuning (if using Java components)**:
   ```yaml
   env:
   - name: JAVA_OPTS
     value: "-Xmx512m -Xms256m -XX:+UseG1GC -XX:MaxGCPauseMillis=200"
   ```

### Caching Strategy

1. **Redis Cache Setup**:
   ```yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: redis-cache
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: redis-cache
     template:
       metadata:
         labels:
           app: redis-cache
       spec:
         containers:
         - name: redis
           image: redis:7-alpine
           ports:
           - containerPort: 6379
           args:
           - redis-server
           - --maxmemory 256mb
           - --maxmemory-policy allkeys-lru
           - --save ""
           - --appendonly no
           resources:
             requests:
               memory: "128Mi"
               cpu: "100m"
             limits:
               memory: "256Mi"
               cpu: "200m"
   ```

2. **Application Cache Configuration**:
   ```yaml
   env:
   - name: REDIS_URL
     value: "redis://redis-cache:6379"
   - name: CACHE_TTL
     value: "300"  # 5 minutes
   - name: ENABLE_CACHING
     value: "true"
   ```

### Database Optimization

1. **Connection Pooling**:
   ```yaml
   env:
   - name: DB_MAX_OPEN_CONNS
     value: "25"
   - name: DB_MAX_IDLE_CONNS
     value: "5"
   - name: DB_CONN_MAX_LIFETIME
     value: "5m"
   ```

2. **Database Performance Monitoring**:
   ```bash
   # Monitor slow queries
   kubectl exec -it postgres-pod -- psql -c "
   SELECT query, mean_time, calls 
   FROM pg_stat_statements 
   ORDER BY mean_time DESC 
   LIMIT 10;"
   ```

## Troubleshooting

### Common Issues and Solutions

1. **Pod Startup Issues**:
   ```bash
   # Check pod status
   kubectl get pods -n bitunix-bot
   
   # Check pod events
   kubectl describe pod <pod-name> -n bitunix-bot
   
   # Check logs
   kubectl logs <pod-name> -n bitunix-bot --previous
   
   # Check resource constraints
   kubectl top pods -n bitunix-bot
   ```

2. **Memory Issues**:
   ```bash
   # Check memory usage
   kubectl exec -it <pod-name> -n bitunix-bot -- free -h
   
   # Check for memory leaks
   kubectl exec -it <pod-name> -n bitunix-bot -- cat /proc/meminfo
   
   # Generate heap dump (if Go)
   kubectl exec -it <pod-name> -n bitunix-bot -- kill -SIGUSR1 1
   ```

3. **Network Connectivity Issues**:
   ```bash
   # Test external connectivity
   kubectl exec -it <pod-name> -n bitunix-bot -- wget -O- https://api.bitunix.com/health
   
   # Test internal service connectivity
   kubectl exec -it <pod-name> -n bitunix-bot -- nslookup redis-cache
   
   # Check network policies
   kubectl get networkpolicy -n bitunix-bot
   ```

4. **ML Model Issues**:
   ```bash
   # Validate model file
   kubectl exec -it <pod-name> -n bitunix-bot -- python3 -c "
   import onnxruntime as ort
   session = ort.InferenceSession('/srv/data/model.onnx')
   print('Model loaded successfully')
   print('Input shapes:', [inp.shape for inp in session.get_inputs()])
   "
   
   # Check model permissions
   kubectl exec -it <pod-name> -n bitunix-bot -- ls -la /srv/data/model.onnx
   ```

### Emergency Procedures

1. **Circuit Breaker Activation**:
   ```bash
   # Immediately stop trading
   kubectl patch deployment bitunix-bot -n bitunix-bot \
     -p='{"spec":{"template":{"spec":{"containers":[{"name":"bitunix-bot","env":[{"name":"DRY_RUN","value":"true"}]}]}}}}'
   
   # Scale down to single replica
   kubectl scale deployment bitunix-bot --replicas=1 -n bitunix-bot
   ```

2. **Emergency Rollback**:
   ```bash
   # Rollback to previous version
   kubectl rollout undo deployment/bitunix-bot -n bitunix-bot
   
   # Check rollback status
   kubectl rollout status deployment/bitunix-bot -n bitunix-bot
   ```

3. **Force Pod Restart**:
   ```bash
   # Delete problematic pods
   kubectl delete pod -l app=bitunix-bot -n bitunix-bot
   
   # Force recreation
   kubectl patch deployment bitunix-bot -n bitunix-bot \
     -p='{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"'$(date +%Y%m%dT%H%M%S)'"}}}}}'
   ```

### Debugging Tools

1. **Debug Container**:
   ```bash
   # Run debug container in same namespace
   kubectl run debug --image=nicolaka/netshoot -i --tty --rm -n bitunix-bot
   ```

2. **Port Forwarding for Local Debug**:
   ```bash
   # Forward metrics port
   kubectl port-forward svc/bitunix-bot-service 8080:8080 -n bitunix-bot
   
   # Access metrics locally
   curl http://localhost:8080/metrics
   ```

3. **Log Analysis**:
   ```bash
   # Search for errors in logs
   kubectl logs -l app=bitunix-bot -n bitunix-bot | grep -i error
   
   # Follow logs in real-time
   kubectl logs -f deployment/bitunix-bot -n bitunix-bot
   
   # Export logs for analysis
   kubectl logs -l app=bitunix-bot -n bitunix-bot --since=1h > bitunix-bot-logs.txt
   ```

This comprehensive deployment guide covers all aspects of deploying and maintaining the Bitunix Trading Bot in production environments with enterprise-grade security, monitoring, and reliability features.
