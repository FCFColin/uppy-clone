output "database_connection_name" {
  value = google_sql_database_instance.uppy_db.connection_name
}

output "redis_host" {
  value = google_redis_instance.uppy_redis.host
}

output "redis_port" {
  value = google_redis_instance.uppy_redis.port
}
