variable "project_id" {
  description = "GCP Project ID"
  type        = string
}

variable "region" {
  description = "GCP Region"
  type        = string
  default     = "us-central1"
}

variable "db_password" {
  description = "PostgreSQL password"
  type        = string
  sensitive   = true
}

variable "vpc_name" {
  description = "VPC network name for private Cloud SQL and Redis"
  type        = string
  default     = "default"
}
