# Bitunix Trading Bot - Production Deployment Pipeline

This directory contains the complete production deployment pipeline for the Bitunix Trading Bot. The pipeline automates the build, test, and deployment process across different environments (development, staging, and production).

## Architecture

The deployment pipeline consists of the following components:

1. **CI/CD Workflows**: GitHub Actions workflows for continuous integration and deployment
2. **Infrastructure as Code**: Terraform configurations for provisioning cloud resources
3. **Container Orchestration**: Kubernetes manifests and Helm charts for deployment
4. **Deployment Scripts**: Shell scripts for manual and automated deployments
5. **ML Pipeline Integration**: Automated model training and deployment

## Directory Structure

```
deploy/
├── README.md                   # This file
├── Dockerfile                  # Container image definition
├── scripts/                    # Deployment scripts
│   ├── deploy.sh               # Basic deployment script
│   ├── production-deploy.sh    # Production deployment script
│   └── deploy_model.sh         # ML model deployment script
├── helm/                       # Helm charts for Kubernetes deployment
│   ├── bitunix-bot/            # Main Helm chart
│   ├── values-development.yaml # Development environment values
│   ├── values-staging.yaml     # Staging environment values
│   └── values-production.yaml  # Production environment values
├── terraform/                  # Infrastructure as Code
│   ├── main.tf                 # Main Terraform configuration
│   ├── variables.tf            # Terraform variables
│   └── outputs.tf              # Terraform outputs
└── k8s/                        # Kubernetes manifests
    ├── deployment.yaml         # Deployment configuration
    ├── service.yaml            # Service configuration
    └── configmap.yaml          # ConfigMap configuration
```

## CI/CD Pipeline

The CI/CD pipeline is implemented using GitHub Actions and consists of the following workflows:

1. **CI/CD Workflow** (`.github/workflows/ci-cd.yml`):
   - Triggered on push to main branch, pull requests, or manual dispatch
   - Builds and tests the application
   - Builds and pushes Docker images
   - Deploys to the specified environment (development, staging, or production)

2. **ML Retraining Workflow** (`.github/workflows/ml-retrain.yml`):
   - Scheduled to run daily at 2 AM UTC
   - Can also be triggered manually
   - Retrains the ML model with the latest data
   - Validates the model performance
   - Deploys the model to the specified environment

## Environment Setup

### Prerequisites

- AWS account with appropriate permissions
- Kubernetes cluster (EKS)
- Docker registry (ECR or other)
- GitHub repository with Actions enabled
- Terraform Cloud account (optional)

### Initial Setup

1. **Set up GitHub Secrets**:
   - `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`: AWS credentials
   - `DOCKER_USERNAME` and `DOCKER_PASSWORD`: Docker registry credentials
   - `KUBE_CONFIG_DEV`, `KUBE_CONFIG_STAGING`, `KUBE_CONFIG_PROD`: Kubernetes configs
   - `BITUNIX_API_KEY` and `BITUNIX_SECRET_KEY`: API credentials for the bot

2. **Initialize Terraform**:
   ```bash
   cd deploy/terraform
   terraform init
   ```

3. **Create Infrastructure**:
   ```bash
   terraform apply -var-file=production.tfvars
   ```

4. **Set up Helm**:
   ```bash
   helm repo add bitunix-bot ./deploy/helm/bitunix-bot
   helm repo update
   ```

## Deployment Process

### Automated Deployment

The CI/CD pipeline automatically deploys the application when changes are pushed to the main branch. You can also trigger a manual deployment using the GitHub Actions workflow dispatch.

1. Go to the GitHub repository
2. Navigate to Actions > CI/CD Pipeline
3. Click "Run workflow"
4. Select the target environment (development, staging, or production)
5. Click "Run workflow"

### Manual Deployment

You can also deploy the application manually using the provided scripts:

```bash
# Deploy to development
./deploy/scripts/production-deploy.sh development latest

# Deploy to staging
./deploy/scripts/production-deploy.sh staging v1.2.3

# Deploy to production
./deploy/scripts/production-deploy.sh production v1.2.3
```

### ML Model Deployment

The ML model can be deployed separately from the application:

```bash
# Deploy model to development
./deploy/scripts/deploy_model.sh --environment development

# Deploy model to staging
./deploy/scripts/deploy_model.sh --environment staging

# Deploy model to production
./deploy/scripts/deploy_model.sh --environment production
```

## Environment Configuration

Each environment (development, staging, and production) has its own configuration:

### Development

- Single-node Kubernetes cluster
- Dry run mode enabled (no real trading)
- Debug logging
- Local Docker registry
- Minimal resource requirements

### Staging

- Multi-node Kubernetes cluster
- Live trading with small amounts
- Standard logging
- Remote Docker registry
- Moderate resource requirements
- Monitoring enabled

### Production

- High-availability Kubernetes cluster
- Live trading with full amounts
- Minimal logging (warnings and errors only)
- Remote Docker registry with image scanning
- High resource requirements
- Full monitoring and alerting
- Backup and disaster recovery

## Monitoring and Observability

The deployment pipeline includes comprehensive monitoring and observability:

1. **Prometheus**: Metrics collection
2. **Grafana**: Dashboards and visualization
3. **Loki**: Log aggregation
4. **Alertmanager**: Alerting and notifications

Access the dashboards at:
- Development: http://grafana.dev.example.com
- Staging: https://grafana.staging.example.com
- Production: https://grafana.prod.example.com

## Backup and Recovery

The deployment pipeline includes automated backup and recovery:

1. **Database Backups**: Daily backups of the database
2. **Configuration Backups**: Backups of all configuration files
3. **ML Model Backups**: Versioned backups of all ML models

Backups are stored in S3 and retained according to the environment:
- Development: 7 days
- Staging: 30 days
- Production: 90 days

## Security

The deployment pipeline includes several security measures:

1. **Secret Management**: All secrets are stored in AWS Secrets Manager
2. **Network Policies**: Strict network policies to limit communication
3. **Pod Security Policies**: Enforce security best practices
4. **Image Scanning**: Scan container images for vulnerabilities
5. **RBAC**: Role-based access control for Kubernetes resources

## Troubleshooting

### Common Issues

1. **Deployment Failures**:
   - Check the GitHub Actions logs for errors
   - Verify Kubernetes resources with `kubectl get pods -n bitunix-bot-<environment>`
   - Check pod logs with `kubectl logs <pod-name> -n bitunix-bot-<environment>`

2. **ML Model Issues**:
   - Check model metrics in `models/model_metrics.json`
   - Verify model loading with `kubectl logs <pod-name> -n bitunix-bot-<environment> | grep "model loaded"`
   - Test model manually with `python scripts/test_model.py models/model.onnx`

3. **Infrastructure Issues**:
   - Check Terraform state with `terraform state list`
   - Verify AWS resources in the AWS Console
   - Check CloudWatch logs for errors

### Support

For additional support, contact the DevOps team at devops@example.com or open an issue in the GitHub repository.