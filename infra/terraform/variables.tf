variable "project_id" {
  description = "GCP Project ID"
  type        = string
  validation {
    condition     = length(var.project_id) > 0
    error_message = "project_id must not be empty."
  }
}

variable "region" {
  description = "GCP primary region for regional resources (Cloud SQL / Redis). Multi-region GKE clusters see var.gke_regions."
  type        = string
  default     = "us-central1"
  validation {
    condition     = can(regex("^[a-z]+[0-9]?-[a-z]+[0-9]+$", var.region))
    error_message = "region must look like a GCP region (e.g. us-central1, europe-west1)."
  }
}

variable "app_user_password" {
  description = "PostgreSQL password for app_user (least-privilege application user)"
  type        = string
  sensitive   = true
  validation {
    condition     = length(var.app_user_password) >= 16
    error_message = "app_user_password must be at least 16 characters."
  }
}

variable "migrator_password" {
  description = "PostgreSQL password for migrator role (DDL-only migration user)"
  type        = string
  sensitive   = true
  validation {
    condition     = length(var.migrator_password) >= 16
    error_message = "migrator_password must be at least 16 characters."
  }
}

variable "vpc_name" {
  description = "Existing VPC network name for private Cloud SQL and Redis (referenced via data, not created here)."
  type        = string
  default     = "default"
  validation {
    condition     = length(var.vpc_name) > 0
    error_message = "vpc_name must not be empty."
  }
}

# v2-R-148: trusted_proxy_cidrs 与 K8s region-config.yaml 对齐，ci-cd.yml 注入 overlay，backend/internal/middleware/proxy.go 解析 X-Forwarded-For。
variable "trusted_proxy_cidrs" {
  description = "Trusted proxy CIDRs (GKE Ingress / L7 LB) allowed to set X-Forwarded-* headers. Maps to K8s ConfigMap trusted-proxy-cidrs (comma-joined)."
  type        = list(string)
  default     = []
  validation {
    condition = alltrue([
      for cidr in var.trusted_proxy_cidrs : can(regex("^(?:[0-9]{1,3}\\.){3}[0-9]{1,3}/[0-9]{1,2}$", cidr))
    ])
    error_message = "Each entry in trusted_proxy_cidrs must be a valid IPv4 CIDR (e.g. 10.0.0.0/8)."
  }
}

# v2-R-23：多区域 GKE 集群（ADR-014），与 ci-cd.yml deploy matrix 对齐。
variable "gke_regions" {
  description = "GCP regions where balloon-game GKE clusters exist (managed manually, referenced via data). Must match ci-cd.yml deploy matrix."
  type        = list(string)
  default     = ["us-east1", "europe-west1", "asia-southeast1"]
  validation {
    condition     = length(var.gke_regions) > 0
    error_message = "gke_regions must contain at least one region."
  }
}

variable "gke_cluster_name_prefix" {
  description = "Name prefix for existing GKE clusters; actual cluster name is <prefix>-<region> (e.g. balloon-game-us-east1)."
  type        = string
  default     = "balloon-game"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*$", var.gke_cluster_name_prefix))
    error_message = "gke_cluster_name_prefix must be lowercase alphanumeric with hyphens."
  }
}
