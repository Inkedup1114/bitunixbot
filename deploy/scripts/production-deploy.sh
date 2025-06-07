#!/bin/bash
# Production Deployment Script for Bitunix Trading Bot
# This script handles the complete deployment process for the Bitunix Trading Bot
# including environment setup, model deployment, and application deployment.

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ENVIRONMENT="${1:-production}"
NAMESPACE="bitunix-bot-${ENVIRONMENT}"
IMAGE_TAG="${2:-latest}"
DRY_RUN="${DRY_RUN:-false}"
SKIP_MODEL="${SKIP_MODEL:-false}"
SKIP_BACKUP="${SKIP_BACKUP:-false}"
SKIP_VALIDATION="${SKIP_VALIDATION:-false}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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

log_step() {
    echo -e "\n${BLUE}[STEP]${NC} $1"
}

# Validation functions
validate_environment() {
    if [[ ! "$ENVIRONMENT" =~ ^(development|staging|production)$ ]]; then
        log_error "Invalid environment: $ENVIRONMENT. Must be one of: development, staging, production"
        exit 1
    fi
}

validate_prerequisites() {
    log_step "Validating prerequisites..."
    
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
    
    # Check if namespace exists, create if not
    if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
        log_info "Namespace $NAMESPACE does not exist, creating..."
        kubectl create namespace "$NAMESPACE"
    fi
    
    log_info "Prerequisites validated successfully"
}

# Build and push Docker image
build_and_push_image() {
    log_step "Building Docker image..."
    
    cd "$PROJECT_ROOT"
    
    # Set image name based on environment
    local image_name="bitunix-bot:${ENVIRONMENT}-${IMAGE_TAG}"
    
    # Build image
    docker build -f deploy/Dockerfile -t "$image_name" . || {
        log_error "Docker build failed"
        exit 1
    }
    
    # Push to registry if not dry run
    if [[ "$DRY_RUN" != "true" ]]; then
        log_info "Pushing image to registry..."
        
        # Tag with registry
        if [[ -n "${DOCKER_REGISTRY:-}" ]]; then
            docker tag "$image_name" "${DOCKER_REGISTRY}/$image_name"
            docker push "${DOCKER_REGISTRY}/$image_name" || {
                log_error "Docker push failed"
                exit 1
            }
            image_name="${DOCKER_REGISTRY}/$image_name"
        else
            docker push "$image_name" || {
                log_error "Docker push failed"
                exit 1
            }
        fi
    fi
    
    log_info "Docker image built successfully: $image_name"
    echo "$image_name"
}

# Deploy ML model
deploy_model() {
    if [[ "$SKIP_MODEL" == "true" ]]; then
        log_info "Skipping model deployment as requested"
        return
    fi
    
    log_step "Deploying ML model..."
    
    # Backup existing model if it exists
    if [[ "$SKIP_BACKUP" != "true" ]]; then
        if kubectl get configmap ml-model -n "$NAMESPACE" &> /dev/null; then
            log_info "Backing up existing model..."
            kubectl get configmap ml-model -n "$NAMESPACE" -o yaml > "$PROJECT_ROOT/models/ml-model-backup-$(date +%Y%m%d%H%M%S).yaml"
        fi
    fi
    
    # Check if model file exists
    local model_path="$PROJECT_ROOT/models/model.onnx"
    local feature_info_path="$PROJECT_ROOT/models/model_feature_info.json"
    
    if [[ ! -f "$model_path" ]]; then
        log_error "Model file not found: $model_path"
        exit 1
    fi
    
    if [[ ! -f "$feature_info_path" ]]; then
        log_warn "Feature info file not found: $feature_info_path"
        log_warn "Continuing without feature info..."
    fi
    
    # Validate model if requested
    if [[ "$SKIP_VALIDATION" != "true" ]]; then
        log_info "Validating model..."
        if ! python "$PROJECT_ROOT/scripts/model_validation.py" --model "$model_path"; then
            log_error "Model validation failed"
            exit 1
        fi
    fi
    
    # Create or update ConfigMap with model
    log_info "Creating ConfigMap with model..."
    
    if [[ -f "$feature_info_path" ]]; then
        kubectl create configmap ml-model \
            --from-file=model.onnx="$model_path" \
            --from-file=model_feature_info.json="$feature_info_path" \
            -n "$NAMESPACE" \
            --dry-run=client -o yaml | kubectl apply -f -
    else
        kubectl create configmap ml-model \
            --from-file=model.onnx="$model_path" \
            -n "$NAMESPACE" \
            --dry-run=client -o yaml | kubectl apply -f -
    fi
    
    log_info "ML model deployed successfully"
}

# Deploy application
deploy_application() {
    log_step "Deploying application to $ENVIRONMENT..."
    
    local values_file="$PROJECT_ROOT/deploy/helm/values-${ENVIRONMENT}.yaml"
    
    if [[ ! -f "$values_file" ]]; then
        log_error "Values file not found: $values_file"
        exit 1
    fi
    
    # Apply Helm chart
    log_info "Deploying with Helm..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        helm upgrade --install bitunix-bot "$PROJECT_ROOT/deploy/helm/bitunix-bot" \
            --namespace "$NAMESPACE" \
            --values "$values_file" \
            --set image.tag="$IMAGE_TAG" \
            --dry-run
    else
        helm upgrade --install bitunix-bot "$PROJECT_ROOT/deploy/helm/bitunix-bot" \
            --namespace "$NAMESPACE" \
            --values "$values_file" \
            --set image.tag="$IMAGE_TAG"
        
        # Wait for deployment
        kubectl rollout status deployment/bitunix-bot -n "$NAMESPACE" --timeout=300s || {
            log_error "Deployment rollout failed"
            exit 1
        }
    fi
    
    log_info "Application deployed successfully"
}

# Verify deployment
verify_deployment() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "Skipping verification in dry run mode"
        return
    fi
    
    log_step "Verifying deployment..."
    
    # Check pods
    log_info "Checking pod status..."
    kubectl get pods -n "$NAMESPACE" -l app=bitunix-bot
    
    # Check if all pods are running
    local running_pods=$(kubectl get pods -n "$NAMESPACE" -l app=bitunix-bot -o jsonpath='{.items[?(@.status.phase=="Running")].metadata.name}' | wc -w)
    local total_pods=$(kubectl get pods -n "$NAMESPACE" -l app=bitunix-bot -o jsonpath='{.items[*].metadata.name}' | wc -w)
    
    if [[ "$running_pods" -ne "$total_pods" ]]; then
        log_warn "Not all pods are running: $running_pods/$total_pods"
    else
        log_info "All pods are running: $running_pods/$total_pods"
    fi
    
    # Check health endpoint if available
    if kubectl get service bitunix-bot-service -n "$NAMESPACE" &> /dev/null; then
        log_info "Checking health endpoint..."
        
        # Port-forward to service
        kubectl port-forward service/bitunix-bot-service 8080:8080 -n "$NAMESPACE" &
        local port_forward_pid=$!
        
        # Wait for port-forward to establish
        sleep 5
        
        # Check health endpoint
        if curl -s http://localhost:8080/metrics > /dev/null; then
            log_info "Health endpoint is responding"
        else
            log_warn "Health endpoint is not responding"
        fi
        
        # Kill port-forward
        kill $port_forward_pid 2>/dev/null || true
    fi
    
    log_info "Deployment verification completed"
}

# Main function
main() {
    log_step "Starting deployment to $ENVIRONMENT"
    
    validate_environment
    validate_prerequisites
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN mode - no actual deployment will occur"
    fi
    
    # Build and push Docker image
    local image_name=$(build_and_push_image)
    
    if [[ "$DRY_RUN" != "true" ]]; then
        # Deploy ML model
        deploy_model
        
        # Deploy application
        deploy_application
        
        # Verify deployment
        verify_deployment
    fi
    
    log_step "Deployment completed successfully!"
    
    if [[ "$ENVIRONMENT" == "production" ]]; then
        log_info "Production deployment complete. Please monitor the application for any issues."
        log_info "Access the metrics dashboard at: https://grafana.example.com/d/bitunix-bot/bitunix-bot-dashboard"
    fi
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
    echo "  DRY_RUN        Set to 'true' for dry-run mode (default: false)"
    echo "  SKIP_MODEL     Set to 'true' to skip model deployment (default: false)"
    echo "  SKIP_BACKUP    Set to 'true' to skip model backup (default: false)"
    echo "  SKIP_VALIDATION Set to 'true' to skip model validation (default: false)"
    echo "  DOCKER_REGISTRY Docker registry URL (default: none)"
    echo ""
    echo "Examples:"
    echo "  $0 staging v1.2.3"
    echo "  DRY_RUN=true $0 production latest"
    echo "  SKIP_MODEL=true $0 development"
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