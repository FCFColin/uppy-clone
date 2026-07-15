output "database_connection_name" {
  value = google_sql_database_instance.balloon_game_db.connection_name
}

output "redis_host" {
  value = google_redis_instance.balloon_game_redis.host
}

output "redis_port" {
  value = google_redis_instance.balloon_game_redis.port
}

output "balloon_game_service_account" {
  description = "GKE Workload Identity GSA for balloon-game pods (ADR-014)"
  value       = google_service_account.balloon_game.email
}

# infra-028: 细分 GSA 邮箱输出，供 ci-cd / K8s overlay 注解引用。
output "balloon_game_server_service_account" {
  description = "Dedicated GSA for the WebSocket/Game server pods (infra-028)"
  value       = google_service_account.balloon_game_server.email
}

output "balloon_game_worker_service_account" {
  description = "Dedicated GSA for async worker pods/jobs (infra-028)"
  value       = google_service_account.balloon_game_worker.email
}

output "balloon_game_migrator_service_account" {
  description = "Dedicated GSA for DB migration CI job (infra-028)"
  value       = google_service_account.balloon_game_migrator.email
}

# infra-025: 全局静态 IP + 托管证书输出，与 multicluster-ingress.yaml 注解名字对齐。
output "balloon_game_global_ip" {
  description = "Global Anycast IP reserved for MultiClusterIngress (infra-025)"
  value       = google_compute_global_address.balloon_game_global_ip.address
}

output "balloon_game_cert_name" {
  description = "Name of the Google-managed SSL certificate referenced by MCI pre-shared-certs annotation (infra-025)"
  value       = google_compute_managed_ssl_certificate.balloon_game_cert.name
}

# v2-C-01：GKE 集群引用输出（集群手动管理，Terraform 仅暴露属性供下游使用）。
output "gke_clusters" {
  description = "Map of region -> GKE cluster attributes (existing clusters referenced via data)."
  value = {
    for region, cluster in data.google_container_cluster.balloon_game :
    region => {
      name            = cluster.name
      location        = cluster.location
      endpoint        = cluster.endpoint
      master_version  = cluster.master_version
      network         = cluster.network
      subnetwork      = cluster.subnetwork
      workload_pool   = try(cluster.workload_identity_config[0].workload_pool, null)
      node_pool_count = length(cluster.node_pool)
    }
  }
}

# v2-R-148：trusted_proxy_cidrs 逗号拼接值，与 K8s ConfigMap trusted-proxy-cidrs 格式一致。
output "trusted_proxy_cidrs_csv" {
  description = "Comma-joined trusted_proxy_cidrs, matching K8s ConfigMap trusted-proxy-cidrs format."
  value       = join(",", var.trusted_proxy_cidrs)
}
