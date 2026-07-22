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
  description = "GKE Workload Identity GSA for balloon-game pods"
  value       = google_service_account.balloon_game.email
}

# infra-028: 细分 GSA 邮箱输出，供 ci-cd / K8s overlay 注解引用。
output "balloon_game_server_service_account" {
  description = "Dedicated GSA for the WebSocket/Game server pods (infra-028)"
  value       = google_service_account.balloon_game_server.email
}

# v2-R-148：trusted_proxy_cidrs 逗号拼接值，与 K8s ConfigMap trusted-proxy-cidrs 格式一致。
output "trusted_proxy_cidrs_csv" {
  description = "Comma-joined trusted_proxy_cidrs, matching K8s ConfigMap trusted-proxy-cidrs format."
  value       = join(",", var.trusted_proxy_cidrs)
}
