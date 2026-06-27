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
