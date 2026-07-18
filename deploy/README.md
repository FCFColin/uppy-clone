# Deploy — Observability

本目录为 **Prometheus / Alertmanager / Grafana** 的 Kustomize 与本地开发配置。应用部署清单在 [`../infra/k8s/`](../infra/k8s/)。

| 路径 | 用途 |
|------|------|
| [`local/`](local/) | `docker-compose.yml` observability profile 专用（prometheus、alertmanager 本地配置） |
| [`prometheus/`](prometheus/) | K8s Prometheus Deployment + 抓取配置 + 告警规则 |
| [`alertmanager/`](alertmanager/) | K8s Alertmanager |
| [`grafana/`](grafana/) | Dashboard 与 provisioning |
| [`kustomization.yaml`](kustomization.yaml) | 一键 apply 可观测性栈 |

## K8s vs 本地 Compose 配置对照

| 组件 | K8s（`deploy/prometheus/` 等） | 本地（`deploy/local/*.local.yml` + compose observability profile） |
|------|----------------------------------|----------------------------------------------------------------------|
| Prometheus | `deploy/prometheus/prometheus.yml` + Deployment | `deploy/local/prometheus.local.yml` 挂载到 compose |
| Alertmanager | `deploy/alertmanager/config.yaml` ConfigMap | `deploy/local/alertmanager.local.yaml` |
| 告警规则 | `deploy/prometheus/alerts.yml` → kustomize configMapGenerator | 同一 `alerts.yml` 挂载到 Prometheus |
| Grafana | K8s Deployment + provisioning | compose `grafana` 服务 |

## Alert rules 单源

- **源文件**：[`prometheus/alerts.yml`](prometheus/alerts.yml)（docker-compose 与 K8s 均引用此文件）
- **K8s ConfigMap**：由 `deploy/kustomization.yaml` 的 `configMapGenerator` 生成（name=`alertmanager-rules`），勿手改