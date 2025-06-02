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
    local required_tools=("kubectl" "docker")
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
    
    log_info "Prerequisites validated successfully"
}

# Build and push Docker image
build_and_push_image() {
    log_info "Building Docker image..."
    
    cd "$PROJECT_ROOT"
    
    # Build image
    docker build -f deploy/Dockerfile -t "bitunix-bot:${IMAGE_TAG}" . || {
        log_error "Docker build failed"
        exit 1
    }
    
    log_info "Docker image built successfully"
}

# Deploy application
deploy_application() {
    log_info "Deploying application to $ENVIRONMENT..."
    
    # Apply Kubernetes manifests
    kubectl apply -f "$PROJECT_ROOT/deploy/k8s/" -n "$NAMESPACE" || {
        log_error "Kubernetes deployment failed"
        exit 1
    }
    
    # Wait for deployment
    kubectl rollout status deployment/bitunix-bot -n "$NAMESPACE" --timeout=300s || {
        log_error "Deployment rollout failed"
        exit 1
    }
    
    log_info "Application deployed successfully"
}

# Main function
main() {
    log_info "Starting deployment to $ENVIRONMENT"
    
    validate_environment
    validate_prerequisites
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN mode - no actual deployment will occur"
    fi
    
    build_and_push_image
    
    if [[ "$DRY_RUN" != "true" ]]; then
        deploy_application
    fi
    
    log_info "Deployment completed successfully!"
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
