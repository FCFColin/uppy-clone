# Environment Configuration

Comparison of configuration across local, staging, and production environments.

| Setting | Local | Staging | Production |
|---------|-------|---------|------------|
| **Secret Management** | `.env` file | Cloud Secret Manager | Cloud Secret Manager + KMS |
| **TLS** | Disabled (http) | Let's Encrypt | Managed certificate (Cloudflare/ALB) |
| **CORS Origins** | `localhost:*` | `staging.example.com` | `example.com` |
| **Log Level** | `debug` | `info` | `warn` |
| **OpenTelemetry** | Disabled | Enabled (OTLP gRPC) | Enabled (OTLP gRPC) |
| **Rate Limiting** | Relaxed (5x normal) | Normal | Normal + WAF |
| **DB Pool Size** | 5 | 20 | 50 |
| **Redis Pool Size** | 5 | 20 | 50 |
| **Max WS Connections** | 100 | 500 | 1000+ |
| **pprof** | Enabled (`:6060`) | Disabled | Disabled (enable on-demand) |
| **Metrics Auth** | None | Basic Auth | Basic Auth + IP whitelist |
| **Admin JWT TTL** | 30 min | 30 min | 30 min |
| **Shutdown Timeout** | 10s | 20s | 30s |
| **Backup Frequency** | Manual | Daily | Continuous (PITR) |
| **Error Budget Policy** | N/A | Alert only | Alert + freeze deploys |
