terraform {
  # 企业为何需要：远程状态存储 + 锁防止团队协作时状态文件冲突和并发修改。
  backend "gcs" {
    bucket = "balloon-game-terraform-state"
    prefix = "terraform/state"
  }
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# PostgreSQL instance
resource "google_sql_database_instance" "uppy_db" {
  name             = "uppy-db"
  database_version = "POSTGRES_16"
  region           = var.region
  settings {
    tier = "db-f1-micro"
    backup_configuration {
      enabled = true
    }
  }
  deletion_protection = true
}

resource "google_sql_database" "uppy_database" {
  name     = "uppy"
  instance = google_sql_database_instance.uppy_db.name
}

resource "google_sql_user" "uppy_user" {
  name     = "uppy"
  instance = google_sql_database_instance.uppy_db.name
  password = var.db_password
}

# Redis instance
resource "google_redis_instance" "uppy_redis" {
  name           = "uppy-redis"
  tier           = "STANDARD_HA"
  memory_size_gb = 1
  region         = var.region
  redis_version  = "REDIS_7_0"
}

# Secret Manager secrets
resource "google_secret_manager_secret" "jwt_secret" {
  secret_id = "uppy-jwt-secret"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "database_url" {
  secret_id = "uppy-database-url"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "redis_url" {
  secret_id = "uppy-redis-url"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "resend_api_key" {
  secret_id = "uppy-resend-api-key"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "admin_password" {
  secret_id = "uppy-admin-password"
  replication {
    auto {}
  }
}

# Cloud Run service IAM
resource "google_secret_manager_secret_iam_member" "secret_accessor" {
  for_each = toset([
    google_secret_manager_secret.jwt_secret.secret_id,
    google_secret_manager_secret.database_url.secret_id,
    google_secret_manager_secret.redis_url.secret_id,
    google_secret_manager_secret.resend_api_key.secret_id,
    google_secret_manager_secret.admin_password.secret_id,
  ])
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${data.google_project.current.number}-compute@developer.gserviceaccount.com"
}

data "google_project" "current" {}
