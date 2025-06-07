# Terraform configuration for Bitunix Trading Bot infrastructure
# This file defines the AWS infrastructure required for the Bitunix Trading Bot

terraform {
  required_version = ">= 1.0.0"
  
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.0"
    }
  }
  
  backend "s3" {
    bucket         = "bitunix-terraform-state"
    key            = "bitunix-bot/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "bitunix-terraform-locks"
  }
}

provider "aws" {
  region = var.aws_region
  
  default_tags {
    tags = {
      Project     = "BitunixBot"
      Environment = var.environment
      ManagedBy   = "Terraform"
    }
  }
}

provider "kubernetes" {
  host                   = module.eks.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)
  
  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
    command     = "aws"
  }
}

provider "helm" {
  kubernetes {
    host                   = module.eks.cluster_endpoint
    cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)
    
    exec {
      api_version = "client.authentication.k8s.io/v1beta1"
      args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
      command     = "aws"
    }
  }
}

# VPC for EKS cluster
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.0"
  
  name = "bitunix-vpc-${var.environment}"
  cidr = "10.0.0.0/16"
  
  azs             = ["${var.aws_region}a", "${var.aws_region}b", "${var.aws_region}c"]
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
  
  enable_nat_gateway     = true
  single_nat_gateway     = var.environment != "production"
  one_nat_gateway_per_az = var.environment == "production"
  
  enable_dns_hostnames = true
  enable_dns_support   = true
  
  public_subnet_tags = {
    "kubernetes.io/cluster/${var.cluster_name}" = "shared"
    "kubernetes.io/role/elb"                    = "1"
  }
  
  private_subnet_tags = {
    "kubernetes.io/cluster/${var.cluster_name}" = "shared"
    "kubernetes.io/role/internal-elb"           = "1"
  }
}

# EKS Cluster
module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 19.0"
  
  cluster_name    = var.cluster_name
  cluster_version = var.kubernetes_version
  
  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets
  
  cluster_endpoint_private_access = true
  cluster_endpoint_public_access  = true
  
  # EKS Managed Node Group(s)
  eks_managed_node_group_defaults = {
    disk_size      = 50
    instance_types = ["t3.medium"]
  }
  
  eks_managed_node_groups = {
    general = {
      min_size     = var.environment == "production" ? 3 : 2
      max_size     = var.environment == "production" ? 10 : 5
      desired_size = var.environment == "production" ? 3 : 2
      
      instance_types = var.environment == "production" ? ["t3.large"] : ["t3.medium"]
      capacity_type  = "ON_DEMAND"
      
      update_config = {
        max_unavailable_percentage = 25
      }
      
      labels = {
        Environment = var.environment
        NodeGroup   = "general"
      }
      
      tags = {
        "k8s.io/cluster-autoscaler/enabled"                 = "true"
        "k8s.io/cluster-autoscaler/${var.cluster_name}" = "owned"
      }
    }
  }
  
  # Enable IRSA (IAM Roles for Service Accounts)
  enable_irsa = true
  
  # Add-ons
  cluster_addons = {
    coredns = {
      most_recent = true
    }
    kube-proxy = {
      most_recent = true
    }
    vpc-cni = {
      most_recent = true
    }
    aws-ebs-csi-driver = {
      most_recent = true
    }
  }
}

# IAM Role for Bitunix Bot
resource "aws_iam_role" "bitunix_bot_role" {
  name = "bitunix-bot-role-${var.environment}"
  
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRoleWithWebIdentity"
        Effect = "Allow"
        Principal = {
          Federated = module.eks.oidc_provider_arn
        }
        Condition = {
          StringEquals = {
            "${module.eks.oidc_provider}:sub" = "system:serviceaccount:bitunix-bot-${var.environment}:bitunix-bot-sa"
            "${module.eks.oidc_provider}:aud" = "sts.amazonaws.com"
          }
        }
      }
    ]
  })
}

# IAM Policy for Bitunix Bot
resource "aws_iam_policy" "bitunix_bot_policy" {
  name        = "bitunix-bot-policy-${var.environment}"
  description = "Policy for Bitunix Bot to access AWS resources"
  
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:ListBucket"
        ]
        Effect   = "Allow"
        Resource = [
          "arn:aws:s3:::bitunix-data-${var.environment}",
          "arn:aws:s3:::bitunix-data-${var.environment}/*",
          "arn:aws:s3:::bitunix-models-${var.environment}",
          "arn:aws:s3:::bitunix-models-${var.environment}/*"
        ]
      },
      {
        Action = [
          "secretsmanager:GetSecretValue"
        ]
        Effect   = "Allow"
        Resource = "arn:aws:secretsmanager:${var.aws_region}:*:secret:bitunix-bot-${var.environment}*"
      }
    ]
  })
}

# Attach policy to role
resource "aws_iam_role_policy_attachment" "bitunix_bot_policy_attachment" {
  role       = aws_iam_role.bitunix_bot_role.name
  policy_arn = aws_iam_policy.bitunix_bot_policy.arn
}

# S3 Buckets for data and models
resource "aws_s3_bucket" "data_bucket" {
  bucket = "bitunix-data-${var.environment}"
}

resource "aws_s3_bucket" "models_bucket" {
  bucket = "bitunix-models-${var.environment}"
}

resource "aws_s3_bucket_versioning" "models_versioning" {
  bucket = aws_s3_bucket.models_bucket.id
  
  versioning_configuration {
    status = "Enabled"
  }
}

# Secrets in AWS Secrets Manager
resource "aws_secretsmanager_secret" "bitunix_api_credentials" {
  name        = "bitunix-bot-${var.environment}/api-credentials"
  description = "API credentials for Bitunix Trading Bot"
}

# CloudWatch Log Group
resource "aws_cloudwatch_log_group" "bitunix_logs" {
  name              = "/aws/eks/${var.cluster_name}/bitunix-bot"
  retention_in_days = var.environment == "production" ? 90 : 30
}

# Kubernetes Namespace
resource "kubernetes_namespace" "bitunix_namespace" {
  metadata {
    name = "bitunix-bot-${var.environment}"
    
    labels = {
      name        = "bitunix-bot-${var.environment}"
      environment = var.environment
    }
  }
  
  depends_on = [module.eks]
}

# Kubernetes Service Account
resource "kubernetes_service_account" "bitunix_sa" {
  metadata {
    name      = "bitunix-bot-sa"
    namespace = kubernetes_namespace.bitunix_namespace.metadata[0].name
    
    annotations = {
      "eks.amazonaws.com/role-arn" = aws_iam_role.bitunix_bot_role.arn
    }
  }
  
  depends_on = [kubernetes_namespace.bitunix_namespace]
}

# Monitoring stack (Prometheus & Grafana)
resource "helm_release" "prometheus_stack" {
  count      = var.install_monitoring ? 1 : 0
  name       = "prometheus"
  repository = "https://prometheus-community.github.io/helm-charts"
  chart      = "kube-prometheus-stack"
  namespace  = "monitoring"
  version    = "45.0.0"
  
  create_namespace = true
  
  set {
    name  = "grafana.enabled"
    value = "true"
  }
  
  set {
    name  = "grafana.adminPassword"
    value = var.grafana_password
  }
  
  set {
    name  = "prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues"
    value = "false"
  }
  
  depends_on = [module.eks]
}

# Outputs
output "cluster_endpoint" {
  description = "Endpoint for EKS control plane"
  value       = module.eks.cluster_endpoint
}

output "cluster_name" {
  description = "Kubernetes Cluster Name"
  value       = module.eks.cluster_name
}

output "region" {
  description = "AWS region"
  value       = var.aws_region
}

output "data_bucket_name" {
  description = "Name of the S3 bucket for data"
  value       = aws_s3_bucket.data_bucket.id
}

output "models_bucket_name" {
  description = "Name of the S3 bucket for ML models"
  value       = aws_s3_bucket.models_bucket.id
}