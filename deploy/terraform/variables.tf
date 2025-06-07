# Variables for Bitunix Trading Bot Terraform configuration

variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment name (development, staging, production)"
  type        = string
  default     = "production"
  
  validation {
    condition     = contains(["development", "staging", "production"], var.environment)
    error_message = "Environment must be one of: development, staging, production."
  }
}

variable "cluster_name" {
  description = "Name of the EKS cluster"
  type        = string
  default     = "bitunix-bot-cluster"
}

variable "kubernetes_version" {
  description = "Kubernetes version for the EKS cluster"
  type        = string
  default     = "1.28"
}

variable "install_monitoring" {
  description = "Whether to install the monitoring stack (Prometheus & Grafana)"
  type        = bool
  default     = true
}

variable "grafana_password" {
  description = "Password for Grafana admin user"
  type        = string
  sensitive   = true
  default     = "changeme"  # Should be overridden in tfvars file or via environment variable
}

variable "node_instance_types" {
  description = "Instance types for the EKS node groups"
  type = object({
    development = list(string)
    staging     = list(string)
    production  = list(string)
  })
  default = {
    development = ["t3.medium"]
    staging     = ["t3.large"]
    production  = ["t3.xlarge", "m5.large"]
  }
}

variable "node_group_scaling" {
  description = "Scaling configuration for the EKS node groups"
  type = object({
    development = object({
      min_size     = number
      max_size     = number
      desired_size = number
    })
    staging = object({
      min_size     = number
      max_size     = number
      desired_size = number
    })
    production = object({
      min_size     = number
      max_size     = number
      desired_size = number
    })
  })
  default = {
    development = {
      min_size     = 1
      max_size     = 3
      desired_size = 1
    }
    staging = {
      min_size     = 2
      max_size     = 5
      desired_size = 2
    }
    production = {
      min_size     = 3
      max_size     = 10
      desired_size = 3
    }
  }
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "enable_vpc_flow_logs" {
  description = "Whether to enable VPC flow logs"
  type        = bool
  default     = true
}

variable "log_retention_days" {
  description = "Number of days to retain logs"
  type = object({
    development = number
    staging     = number
    production  = number
  })
  default = {
    development = 7
    staging     = 30
    production  = 90
  }
}

variable "tags" {
  description = "Additional tags for all resources"
  type        = map(string)
  default     = {}
}

variable "enable_waf" {
  description = "Whether to enable AWS WAF for the ALB"
  type        = bool
  default     = true
}

variable "enable_shield" {
  description = "Whether to enable AWS Shield Advanced for DDoS protection"
  type        = bool
  default     = false  # Disabled by default due to cost
}

variable "backup_retention_days" {
  description = "Number of days to retain backups"
  type = object({
    development = number
    staging     = number
    production  = number
  })
  default = {
    development = 1
    staging     = 7
    production  = 30
  }
}

variable "enable_cross_region_backup" {
  description = "Whether to enable cross-region backup replication"
  type        = bool
  default     = false
}

variable "cross_region_backup_region" {
  description = "Region to replicate backups to"
  type        = string
  default     = "us-west-2"
}

variable "domain_name" {
  description = "Domain name for the application"
  type        = string
  default     = "bitunix-bot.example.com"
}

variable "create_route53_records" {
  description = "Whether to create Route53 records"
  type        = bool
  default     = true
}

variable "route53_zone_id" {
  description = "Route53 hosted zone ID"
  type        = string
  default     = ""  # Should be provided in tfvars file
}

variable "enable_cloudfront" {
  description = "Whether to enable CloudFront distribution"
  type        = bool
  default     = false
}

variable "cloudfront_price_class" {
  description = "CloudFront price class"
  type        = string
  default     = "PriceClass_100"  # Use PriceClass_100 for North America and Europe
}

variable "enable_s3_versioning" {
  description = "Whether to enable versioning for S3 buckets"
  type        = bool
  default     = true
}

variable "s3_lifecycle_rules" {
  description = "Lifecycle rules for S3 buckets"
  type = object({
    data_bucket = object({
      enabled                   = bool
      noncurrent_version_expiry = number
      abort_incomplete_days     = number
    })
    models_bucket = object({
      enabled                   = bool
      noncurrent_version_expiry = number
      abort_incomplete_days     = number
    })
  })
  default = {
    data_bucket = {
      enabled                   = true
      noncurrent_version_expiry = 30
      abort_incomplete_days     = 7
    }
    models_bucket = {
      enabled                   = true
      noncurrent_version_expiry = 90
      abort_incomplete_days     = 7
    }
  }
}

variable "enable_kms_encryption" {
  description = "Whether to enable KMS encryption for S3 buckets and Secrets Manager"
  type        = bool
  default     = true
}

variable "kms_deletion_window_in_days" {
  description = "Number of days to wait before deleting a KMS key that has been scheduled for deletion"
  type        = number
  default     = 30
}

variable "enable_cloudtrail" {
  description = "Whether to enable CloudTrail"
  type        = bool
  default     = true
}

variable "enable_guardduty" {
  description = "Whether to enable GuardDuty"
  type        = bool
  default     = true
}

variable "enable_config" {
  description = "Whether to enable AWS Config"
  type        = bool
  default     = true
}

variable "enable_security_hub" {
  description = "Whether to enable Security Hub"
  type        = bool
  default     = false  # Disabled by default due to cost
}