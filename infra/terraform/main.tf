terraform {
  # 企业为何需要：远程状态存储 + 锁防止团队协作时状态文件冲突和并发修改。
  backend "gcs" {
    bucket = "balloon-game-terraform-state"
    prefix = "terraform/state"
  }
  # v2-R-21：固定 Terraform CLI 最低版本，避免团队成员用旧版本引入不兼容语法。
  required_version = ">= 1.5"
  required_providers {
    google = {
      source = "hashicorp/google"
      # v2-R-26：provider version pinning（major version 锁定，允许 minor 修复）。
      version = "~> 5.0"
    }
    random = {
      source = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# VPC：引用项目内现有 VPC（默认 default）用于 Cloud SQL/Redis 私网连接；不自创建避免误删网络。
data "google_compute_network" "balloon_vpc" {
  name    = var.vpc_name
  project = var.project_id
}

# Reserved IP range for private services (VPC peering with Google services).
resource "google_compute_global_address" "private_ip_range" {
  name          = "balloon-game-private-ip-range"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = data.google_compute_network.balloon_vpc.id
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = data.google_compute_network.balloon_vpc.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_ip_range.name]
}

# PostgreSQL instance
resource "google_sql_database_instance" "balloon_game_db" {
  name             = "balloon-game-db"
  database_version = "POSTGRES_16"
  region           = var.region
  depends_on = [
    google_service_networking_connection.private_vpc_connection
  ]
  settings {
    tier = "db-custom-2-7680"
    availability_type = "REGIONAL_AVAILABILITY"
    backup_configuration {
      enabled = true
    }
    ip_configuration {
      ipv4_enabled    = false
      private_network = data.google_compute_network.balloon_vpc.id
      require_ssl     = true
    }
  }
  deletion_protection = true
}

resource "google_sql_database" "balloon_game_database" {
  name     = "balloon_game"
  instance = google_sql_database_instance.balloon_game_db.name
}

# Database password managed via random_password + Secret Manager (not TF state).
resource "google_sql_user" "balloon_game_user" {
  name     = "balloon_game"
  instance = google_sql_database_instance.balloon_game_db.name
  password = random_password.db_password.result
}

resource "random_password" "db_password" {
  length  = 24
  special = false
}

# Least-privilege DB roles (NOCREATEDB/NOCREATEROLE/NOSUPERUSER); migration 000009 grants TABLE-level permissions.
resource "google_sql_user" "app_user" {
  name     = "app_user"
  instance = google_sql_database_instance.balloon_game_db.name
  password = var.app_user_password
  type     = "BUILT_IN"
}

resource "google_sql_user" "migrator" {
  name     = "migrator"
  instance = google_sql_database_instance.balloon_game_db.name
  password = var.migrator_password
  type     = "BUILT_IN"
}

# Redis (Memorystore). 生产用此; dev 用 infra/k8s/base/redis.yaml 自托管 (infra-008).
resource "google_redis_instance" "balloon_game_redis" {
  name                    = "balloon-game-redis"
  tier                    = "STANDARD_HA"
  memory_size_gb          = 1
  region                  = var.region
  redis_version           = "REDIS_7_0"
  auth_enabled            = true
  transit_encryption_mode = "SERVER_AUTHENTICATION"
  authorized_network      = data.google_compute_network.balloon_vpc.id
  connect_mode            = "DIRECT_PEERING"
}

# Secret Manager secrets (renamed from uppy-* to balloon-game-* per infra-006).
resource "google_secret_manager_secret" "jwt_secret" {
  secret_id = "balloon-game-jwt-secret"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "database_url" {
  secret_id = "balloon-game-database-url"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "redis_url" {
  secret_id = "balloon-game-redis-url"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "resend_api_key" {
  secret_id = "balloon-game-resend-api-key"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "admin_password" {
  secret_id = "balloon-game-admin-password"
  replication {
    auto {}
  }
}

# GKE Workload Identity SA: Pod 以此 GSA 身份访问 Secret Manager / Cloud SQL，免长期密钥。
resource "google_service_account" "balloon_game" {
  account_id   = "balloon-game"
  display_name = "Balloon Game GKE Workload Identity SA (legacy umbrella SA)"
}

# infra-028: 拆分最小权限 GSA——server 独立身份（worker/migrator 已删），原 balloon-game SA 作为遗留 umbrella 保留。
resource "google_service_account" "balloon_game_server" {
  account_id   = "balloon-game-server"
  display_name = "Balloon Game WebSocket/Game server GSA"
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

# Bind K8s SA (balloon-game) to GSA via Workload Identity; overlay 注解 iam.gke.io/gcp-service-account 指向它。
resource "google_service_account_iam_member" "workload_identity" {
  service_account_id = google_service_account.balloon_game.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[balloon-game/balloon-game]"
}

# Cloud SQL client access for the GKE Workload Identity GSA (single-region PostgreSQL).
resource "google_project_iam_member" "cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.balloon_game.email}"
}

# infra-028: server GSA IAM 绑定（worker/migrator 已随 Batch 1 worker.yaml 删除）。
resource "google_project_iam_member" "server_cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.balloon_game_server.email}"
}

# Server GSA 读所有应用 Secret（与遗留 umbrella SA 一致，便于逐步迁移）。
resource "google_secret_manager_secret_iam_member" "server_secret_accessor" {
  for_each = toset([
    google_secret_manager_secret.jwt_secret.secret_id,
    google_secret_manager_secret.database_url.secret_id,
    google_secret_manager_secret.redis_url.secret_id,
    google_secret_manager_secret.resend_api_key.secret_id,
    google_secret_manager_secret.admin_password.secret_id,
  ])
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.balloon_game_server.email}"
}

# Workload Identity 绑定——K8s SA → GSA。
resource "google_service_account_iam_member" "workload_identity_server" {
  service_account_id = google_service_account.balloon_game_server.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[balloon-game/balloon-game-server]"
}


