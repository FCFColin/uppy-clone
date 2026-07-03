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

# GKE Workload Identity service account (ADR-014).
# 取代 Cloud Run compute 默认 SA：WebSocket 层运行在 GKE（owner 反向代理需实例间可
# 寻址，ADR-005/013/016），Pod 通过 Workload Identity 以此 GSA 身份访问 Secret
# Manager / Cloud SQL / CRDB，免长期密钥。
resource "google_service_account" "balloon_game" {
  account_id   = "balloon-game"
  display_name = "Balloon Game GKE Workload Identity SA"
}

# Secret Manager access granted to the GKE Workload Identity GSA (not Cloud Run).
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
  member    = "serviceAccount:${google_service_account.balloon_game.email}"
}

# Bind the Kubernetes ServiceAccount (balloon-game in namespace balloon-game,
# created by infra/k8s/base/service.yaml) to the GSA via Workload Identity.
# 每区域集群共用同一 GSA；overlay 的 SA 注解 iam.gke.io/gcp-service-account 指向它。
resource "google_service_account_iam_member" "workload_identity" {
  service_account_id = google_service_account.balloon_game.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[balloon-game/balloon-game]"
}

# Cloud SQL client access for the Workload Identity GSA (single-region PostgreSQL;
# multi-region CRDB uses its own client certs, see docs/data/cockroachdb-migration.md).
resource "google_project_iam_member" "cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.balloon_game.email}"
}


