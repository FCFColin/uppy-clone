# Deploy — Observability

本目录为 **Prometheus / Alertmanager / Grafana / Thanos** 的 Kustomize 与本地开发配置。应用部署清单在 [`../infra/k8s/`](../infra/k8s/)。

| 路径 | 用途 |
|------|------|
| [`local/`](local/) | `docker-compose.yml` observability profile 专用（prometheus、alertmanager 本地配置） |
| [`prometheus/`](prometheus/) | K8s Prometheus Deployment + 抓取配置 |
| [`alertmanager/`](alertmanager/) | K8s Alertmanager + 告警规则 |
| [`grafana/`](grafana/) | Dashboard 与 provisioning |
| [`thanos/`](thanos/) | 跨区域 Thanos Querier |
| [`kustomization.yaml`](kustomization.yaml) | 一键 apply 可观测性栈 |

## K8s vs 本地 Compose 配置对照

| 组件 | K8s（`deploy/prometheus/` 等） | 本地（`deploy/local/*.local.yml` + compose observability profile） |
|------|----------------------------------|----------------------------------------------------------------------|
| Prometheus | `deploy/prometheus/prometheus.yml` + Deployment | `deploy/local/prometheus.local.yml` 挂载到 compose |
| Alertmanager | `deploy/alertmanager/config.yaml` ConfigMap | `deploy/local/alertmanager.local.yaml` |
| 告警规则 | `rules.yml` → `make sync-alert-rules` → ConfigMap | 同一 `rules.yml` 挂载到 Prometheus |
| Grafana | K8s Deployment + provisioning | compose `grafana` 服务 |

## Alert rules 单源

- **源文件**：[`alertmanager/rules.yml`](alertmanager/rules.yml)（docker-compose 与文档引用此文件）
- **K8s ConfigMap**：[`alertmanager/rules-configmap.yaml`](alertmanager/rules-configmap.yaml) — 由 `make sync-alert-rules` 生成，勿手改

```bash
make sync-alert-rules   # 重新生成 rules-configmap.yaml
```
