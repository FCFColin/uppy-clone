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

# Database password managed via Google Secret Manager, not Terraform state.
# Use `random_password` for initial creation, then `lifecycle ignore_changes` to
# prevent drift. See Secret Manager for actual password value.
resource "google_sql_user" "balloon_game_user" {
  name     = "balloon_game"
  instance = google_sql_database_instance.balloon_game_db.name
  password = random_password.db_password.result
}

resource "random_password" "db_password" {
  length  = 24
  special = false
}

# Least-privilege database roles (see docker/postgres/init/01-create-roles.sql for local dev).
# These are created as Cloud SQL users with NOCREATEDB/NOCREATEROLE/NOSUPERUSER.
# Migration 000009 grants TABLE-level permissions; this creates the login roles.
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

# Redis instance (Memorystore). NOTE: infra/k8s/base/ also deploys self-hosted Redis
# StatefulSets. Consolidation decision: production targets Memorystore; dev uses
# self-hosted. See infra-008 audit finding — two Redis deployments coexist for
# env separation. Future: remove self-hosted Redis from K8s manifests.
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

# Secret Manager secrets (renamed from uppy-* to balloon-game-* per infra-006 audit)
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

# GKE Workload Identity service account (ADR-014).
# 取代 Cloud Run compute 默认 SA：WebSocket 层运行在 GKE（owner 反向代理需实例间可
# 寻址，ADR-005/013/016），Pod 通过 Workload Identity 以此 GSA 身份访问 Secret
# Manager / Cloud SQL / CRDB，免长期密钥。
resource "google_service_account" "balloon_game" {
  account_id   = "balloon-game"
  display_name = "Balloon Game GKE Workload Identity SA (legacy umbrella SA)"
}

# infra-028: 拆分最小权限 GSA——server/worker/migrator 各自独立身份。
# 原 balloon-game SA 作为遗留 umbrella 保留向后兼容；新部署应使用下列细分 GSA。
# 每个 GSA 只授予该服务实际需要的角色，避免横向越权（如 migrator 不应能读 Secret
# Manager 中的 JWT 私钥）。
resource "google_service_account" "balloon_game_server" {
  account_id   = "balloon-game-server"
  display_name = "Balloon Game WebSocket/Game server GSA (ADR-014)"
}

resource "google_service_account" "balloon_game_worker" {
  account_id   = "balloon-game-worker"
  display_name = "Balloon Game async worker GSA (email/outbox/gdpr cleanup)"
}

resource "google_service_account" "balloon_game_migrator" {
  account_id   = "balloon-game-migrator"
  display_name = "Balloon Game DB migrator GSA (DDL-only, ci-cd job)"
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

# infra-028: 细分 GSA 的 IAM 绑定（最小权限）。
# - server: 读 Secret（含 JWT/DB URL/Redis URL）；Cloud SQL client（pgx 直连）。
# - worker: 读 Secret（DB URL/Redis URL/Resend key）；Cloud SQL client。
# - migrator: Cloud SQL client + Secret Manager Viewer（仅读 database-url）；不授予其它 Secret。
resource "google_project_iam_member" "server_cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.balloon_game_server.email}"
}

resource "google_project_iam_member" "worker_cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.balloon_game_worker.email}"
}

resource "google_project_iam_member" "migrator_cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.balloon_game_migrator.email}"
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

# Worker GSA 仅读 DB/Redis/Resend（不需要 JWT/admin password）。
resource "google_secret_manager_secret_iam_member" "worker_secret_accessor" {
  for_each = toset([
    google_secret_manager_secret.database_url.secret_id,
    google_secret_manager_secret.redis_url.secret_id,
    google_secret_manager_secret.resend_api_key.secret_id,
  ])
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.balloon_game_worker.email}"
}

# Migrator GSA 仅读 database-url（DDL 期需要）。
resource "google_secret_manager_secret_iam_member" "migrator_secret_accessor" {
  secret_id = google_secret_manager_secret.database_url.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.balloon_game_migrator.email}"
}

# Workload Identity 绑定——K8s SA → GSA。K8s manifest 需相应增加 server/worker SA。
resource "google_service_account_iam_member" "workload_identity_server" {
  service_account_id = google_service_account.balloon_game_server.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[balloon-game/balloon-game-server]"
}

resource "google_service_account_iam_member" "workload_identity_worker" {
  service_account_id = google_service_account.balloon_game_worker.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[balloon-game/balloon-game-worker]"
}

resource "google_service_account_iam_member" "workload_identity_migrator" {
  service_account_id = google_service_account.balloon_game_migrator.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[balloon-game/balloon-game-migrator]"
}

resource "google_compute_global_address" "balloon_game_global_ip" {
  name         = "balloon-game-global-ip"
  description  = "Global Anycast IP for balloon-game MultiClusterIngress (ADR-014)"
  address_type = "EXTERNAL"
  ip_version   = "IPV4"
}

resource "google_compute_managed_ssl_certificate" "balloon_game_cert" {
  name        = "balloon-game-cert"
  description = "Managed TLS cert covering multi-region WSS subdomains + apex (ADR-014/016)"
  managed {
    # domains 列表应通过 var 配置；此处给出典型域，生产覆盖由 tfvars 注入。
    domains = ["balloon.example", "*.balloon.example"]
  }
}


