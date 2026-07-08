output "database_connection_name" {
  value = google_sql_database_instance.uppy_db.connection_name
}

output "redis_host" {
  value = google_redis_instance.uppy_redis.host
}

output "redis_port" {
  value = google_redis_instance.uppy_redis.port
}

output "balloon_game_service_account" {
  description = "GKE Workload Identity GSA for balloon-game pods (ADR-014)"
  value       = google_service_account.balloon_game.email
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
