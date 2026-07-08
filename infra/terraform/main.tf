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
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# VPC：引用项目内现有 VPC（默认 default），用于 Cloud SQL/Redis 私网连接。
# 决策：VPC 由项目默认提供，不自创建，避免误删整张网络波及其他资源。
data "google_compute_network" "balloon_vpc" {
  name    = var.vpc_name
  project = var.project_id
}

# GKE 集群（v2-C-01 折中方案：data 块引用现有手动创建的集群，不自创建）。
#
# 决策：GKE 集群 + node pool 手动管理，Terraform 仅通过 data 引用。
#   1. 集群控制平面生命周期长、变更低频，误删代价极高（不可重建历史数据）；
#   2. 多区域集群（us-east1/europe-west1/asia-southeast1，ADR-014）跨 region
#      apply 复杂度高，与现有 ci-cd.yml 逐区域 kubectl 部署流程并存易冲突；
#   3. node pool 启用自动扩缩容，与 Terraform 生命周期管理冲突（drift 频繁）。
# 见 ADR-014（多区域拓扑）。若后续需全 IaC 化，应新建 ADR 评估迁移路径。
# v2-R-23：var.gke_regions 默认含 3 个区域，与 ci-cd.yml deploy matrix 对齐。
data "google_container_cluster" "balloon_game" {
  for_each = toset(var.gke_regions)
  name     = "${var.gke_cluster_name_prefix}-${each.value}"
  location = each.value
  project  = var.project_id
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
resource "google_sql_database_instance" "uppy_db" {
  name             = "uppy-db"
  database_version = "POSTGRES_16"
  region           = var.region
  depends_on = [
    google_service_networking_connection.private_vpc_connection
  ]
  settings {
    tier = "db-f1-micro"
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
  name                    = "uppy-redis"
  tier                    = "STANDARD_HA"
  memory_size_gb          = 1
  region                  = var.region
  redis_version           = "REDIS_7_0"
  auth_enabled            = true
  transit_encryption_mode = "SERVER_AUTHENTICATION"
  authorized_network      = data.google_compute_network.balloon_vpc.id
  connect_mode            = "DIRECT_PEERING"
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


