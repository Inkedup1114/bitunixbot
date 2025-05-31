#!/bin/bash
# Comprehensive deployment script for Bitunix Trading Bot

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ENVIRONMENT="${1:-staging}"
NAMESPACE="bitunix-bot-${ENVIRONMENT}"
IMAGE_TAG="${2:-latest}"
DRY_RUN="${DRY_RUN:-false}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Validation functions
validate_environment() {
    if [[ ! "$ENVIRONMENT" =~ ^(development|staging|production)$ ]]; then
        log_error "Invalid environment: $ENVIRONMENT. Must be one of: development, staging, production"
        exit 1
    fi
}

validate_prerequisites() {
    log_info "Validating prerequisites..."
    
    # Check required tools
    local required_tools=("kubectl" "helm" "docker")
    for tool in "${required_tools[@]}"; do
        if ! command -v "$tool" &> /dev/null; then
            log_error "$tool is required but not installed"
            exit 1
        fi
    done
    
    # Check Kubernetes connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    # Check if namespace exists or can be created
    if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
        log_warn "Namespace $NAMESPACE does not exist, will create it"
    fi
    
    log_info "Prerequisites validated successfully"
}

# Build and push Docker image
build_and_push_image() {
    log_info "Building and pushing Docker image..."
    
    cd "$PROJECT_ROOT"
    
    # Build image
    docker build -f deploy/Dockerfile -t "bitunix-bot:${IMAGE_TAG}" .
    
    # Tag for registry (adjust for your registry)
    local registry="${DOCKER_REGISTRY:-localhost:5000}"
    docker tag "bitunix-bot:${IMAGE_TAG}" "${registry}/bitunix-bot:${IMAGE_TAG}"
    
    # Push to registry
    if [[ "$DRY_RUN" != "true" ]]; then
        docker push "${registry}/bitunix-bot:${IMAGE_TAG}"
        log_info "Image pushed to ${registry}/bitunix-bot:${IMAGE_TAG}"
    else
        log_info "DRY RUN: Would push image to ${registry}/bitunix-bot:${IMAGE_TAG}"
    fi
}

# Create namespace and basic resources
setup_namespace() {
    log_info "Setting up namespace: $NAMESPACE"
    
    if [[ "$DRY_RUN" != "true" ]]; then
        kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
        
        # Add labels
        kubectl label namespace "$NAMESPACE" \
            app.kubernetes.io/name=bitunix-bot \
            app.kubernetes.io/instance="$ENVIRONMENT" \
            environment="$ENVIRONMENT" \
            --overwrite
            
        # Add security labels for pod security standards
        if [[ "$ENVIRONMENT" == "production" ]]; then
            kubectl label namespace "$NAMESPACE" \
                pod-security.kubernetes.io/enforce=restricted \
                pod-security.kubernetes.io/audit=restricted \
                pod-security.kubernetes.io/warn=restricted \
                --overwrite
        fi
    else
        log_info "DRY RUN: Would create/update namespace $NAMESPACE"
    fi
}

# Deploy secrets (expects secrets to be provided via environment or files)
deploy_secrets() {
    log_info "Deploying secrets..."
    
    # Check if secrets are provided
    if [[ -z "${BITUNIX_API_KEY:-}" ]] && [[ ! -f "${PROJECT_ROOT}/secrets/api_key.txt" ]]; then
        log_error "BITUNIX_API_KEY environment variable or secrets/api_key.txt file is required"
        exit 1
    fi
    
    if [[ -z "${BITUNIX_SECRET_KEY:-}" ]] && [[ ! -f "${PROJECT_ROOT}/secrets/secret_key.txt" ]]; then
        log_error "BITUNIX_SECRET_KEY environment variable or secrets/secret_key.txt file is required"
        exit 1
    fi
    
    # Get API credentials
    local api_key="${BITUNIX_API_KEY:-$(cat "${PROJECT_ROOT}/secrets/api_key.txt" 2>/dev/null || echo "")}"
    local secret_key="${BITUNIX_SECRET_KEY:-$(cat "${PROJECT_ROOT}/secrets/secret_key.txt" 2>/dev/null || echo "")}"
    
    if [[ "$DRY_RUN" != "true" ]]; then
        kubectl create secret generic bitunix-credentials \
            --from-literal=api-key="$api_key" \
            --from-literal=secret-key="$secret_key" \
            --namespace="$NAMESPACE" \
            --dry-run=client -o yaml | kubectl apply -f -
        log_info "Secrets deployed successfully"
    else
        log_info "DRY RUN: Would deploy secrets to namespace $NAMESPACE"
    fi
}

# Deploy configuration
deploy_config() {
    log_info "Deploying configuration..."
    
    local config_file="${PROJECT_ROOT}/config-${ENVIRONMENT}.yaml"
    if [[ ! -f "$config_file" ]]; then
        config_file="${PROJECT_ROOT}/config.yaml"
    fi
    
    if [[ ! -f "$config_file" ]]; then
        log_error "Configuration file not found: $config_file"
        exit 1
    fi
    
    if [[ "$DRY_RUN" != "true" ]]; then
        kubectl create configmap bot-config \
            --from-file=config.yaml="$config_file" \
            --namespace="$NAMESPACE" \
            --dry-run=client -o yaml | kubectl apply -f -
        log_info "Configuration deployed successfully"
    else
        log_info "DRY RUN: Would deploy config from $config_file"
    fi
}

# Deploy ML model
deploy_model() {
    log_info "Deploying ML model..."
    
    local model_file="${PROJECT_ROOT}/model-${ENVIRONMENT}.onnx"
    if [[ ! -f "$model_file" ]]; then
        model_file="${PROJECT_ROOT}/model.onnx"
    fi
    
    if [[ ! -f "$model_file" ]]; then
        log_error "Model file not found: $model_file"
        exit 1
    fi
    
    # Validate model before deployment
    log_info "Validating model..."
    if command -v python3 &> /dev/null; then
        python3 -c "
import onnxruntime as ort
try:
    session = ort.InferenceSession('$model_file')
    print('Model validation successful')
except Exception as e:
    print(f'Model validation failed: {e}')
    exit(1)
" || {
            log_error "Model validation failed"
            exit 1
        }
    fi
    
    if [[ "$DRY_RUN" != "true" ]]; then
        kubectl create configmap ml-model \
            --from-file=model.onnx="$model_file" \
            --namespace="$NAMESPACE" \
            --dry-run=client -o yaml | kubectl apply -f -
        log_info "ML model deployed successfully"
    else
        log_info "DRY RUN: Would deploy model from $model_file"
    fi
}

# Deploy using Helm
deploy_application() {
    log_info "Deploying application using Helm..."
    
    local helm_chart="${PROJECT_ROOT}/deploy/helm/bitunix-bot"
    local values_file="${PROJECT_ROOT}/deploy/helm/values-${ENVIRONMENT}.yaml"
    
    if [[ ! -d "$helm_chart" ]]; then
        log_error "Helm chart not found: $helm_chart"
        exit 1
    fi
    
    if [[ ! -f "$values_file" ]]; then
        values_file="${PROJECT_ROOT}/deploy/helm/values.yaml"
    fi
    
    local helm_args=(
        "upgrade" "--install" "bitunix-bot-${ENVIRONMENT}"
        "$helm_chart"
        "--namespace" "$NAMESPACE"
        "--values" "$values_file"
        "--set" "image.tag=${IMAGE_TAG}"
        "--set" "environment=${ENVIRONMENT}"
        "--timeout" "10m"
        "--wait"
    )
    
    if [[ "$DRY_RUN" == "true" ]]; then
        helm_args+=("--dry-run")
    fi
    
    helm "${helm_args[@]}"
    
    if [[ "$DRY_RUN" != "true" ]]; then
        log_info "Application deployed successfully"
    else
        log_info "DRY RUN: Application deployment validated"
    fi
}

# Verify deployment
verify_deployment() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "Skipping verification in dry-run mode"
        return
    fi
    
    log_info "Verifying deployment..."
    
    # Wait for rollout to complete
    kubectl rollout status deployment/bitunix-bot -n "$NAMESPACE" --timeout=300s
    
    # Check pod status
    local pod_count
    pod_count=$(kubectl get pods -n "$NAMESPACE" -l app=bitunix-bot --field-selector=status.phase=Running --no-headers | wc -l)
    
    if [[ "$pod_count" -eq 0 ]]; then
        log_error "No running pods found"
        exit 1
    fi
    
    log_info "Found $pod_count running pod(s)"
    
    # Check service health
    local service_port
    service_port=$(kubectl get svc bitunix-bot-service -n "$NAMESPACE" -o jsonpath='{.spec.ports[0].port}')
    
    if command -v curl &> /dev/null; then
        log_info "Testing service health..."
        kubectl port-forward svc/bitunix-bot-service "$service_port:$service_port" -n "$NAMESPACE" &
        local pf_pid=$!
        sleep 5
        
        if curl -s "http://localhost:$service_port/metrics" > /dev/null; then
            log_info "Service health check passed"
        else
            log_warn "Service health check failed"
        fi
        
        kill $pf_pid 2>/dev/null || true
    fi
    
    log_info "Deployment verification completed"
}

# Cleanup function
cleanup() {
    log_info "Performing cleanup..."
    # Kill any background processes
    jobs -p | xargs -r kill 2>/dev/null || true
}

# Main deployment flow
main() {
    log_info "Starting deployment of Bitunix Trading Bot"
    log_info "Environment: $ENVIRONMENT"
    log_info "Image Tag: $IMAGE_TAG"
    log_info "Namespace: $NAMESPACE"
    log_info "Dry Run: $DRY_RUN"
    
    # Set up cleanup trap
    trap cleanup EXIT
    
    # Validation
    validate_environment
    validate_prerequisites
    
    # Deployment steps
    if [[ "${SKIP_BUILD:-false}" != "true" ]]; then
        build_and_push_image
    fi
    
    setup_namespace
    deploy_secrets
    deploy_config
    deploy_model
    deploy_application
    verify_deployment
    
    log_info "Deployment completed successfully!"
    
    # Print useful information
    echo ""
    log_info "Useful commands:"
    echo "  View pods:        kubectl get pods -n $NAMESPACE"
    echo "  View logs:        kubectl logs -f deployment/bitunix-bot -n $NAMESPACE"
    echo "  View metrics:     kubectl port-forward svc/bitunix-bot-service 8080:8080 -n $NAMESPACE"
    echo "  Scale deployment: kubectl scale deployment bitunix-bot --replicas=N -n $NAMESPACE"
}

# Script usage
usage() {
    echo "Usage: $0 <environment> [image_tag]"
    echo ""
    echo "Arguments:"
    echo "  environment    Deployment environment (development|staging|production)"
    echo "  image_tag      Docker image tag (default: latest)"
    echo ""
    echo "Environment variables:"
    echo "  DRY_RUN             Set to 'true' for dry-run mode (default: false)"
    echo "  SKIP_BUILD          Set to 'true' to skip image build (default: false)"
    echo "  DOCKER_REGISTRY     Docker registry URL (default: localhost:5000)"
    echo "  BITUNIX_API_KEY     Bitunix API key (can also be in secrets/api_key.txt)"
    echo "  BITUNIX_SECRET_KEY  Bitunix secret key (can also be in secrets/secret_key.txt)"
    echo ""
    echo "Examples:"
    echo "  $0 staging v1.2.3"
    echo "  DRY_RUN=true $0 production latest"
    echo "  SKIP_BUILD=true $0 development"
}

# Check if help is requested
if [[ "${1:-}" == "-h" ]] || [[ "${1:-}" == "--help" ]]; then
    usage
    exit 0
fi

# Check if environment is provided
if [[ $# -eq 0 ]]; then
    log_error "Environment argument is required"
    usage
    exit 1
fi

# Run main function
main "$@"
