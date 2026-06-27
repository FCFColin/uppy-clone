# Infrastructure

本目录包含**应用运行时**基础设施，不含可观测性栈（见 [`../deploy/`](../deploy/)）。

| 子目录 | 用途 |
|--------|------|
| [`terraform/`](terraform/) | GCP 资源：Cloud SQL、Memorystore Redis、GSA、Workload Identity |
| [`k8s/base/`](k8s/base/) | 应用 Kustomize base（StatefulSet、HPA、Redis、PDB） |
| [`k8s/overlays/<region>/`](k8s/overlays/) | 区域 overlay（`us-east1`、`europe-west1`、`asia-southeast1`） |
| [`k8s/global/`](k8s/global/) | 跨集群资源（MCI、NetworkPolicy） |

## 部署关系

- **应用**：`kubectl apply -k infra/k8s/overlays/<region>`
- **可观测性**：`kubectl apply -k deploy/`（Prometheus、Alertmanager、Grafana、Thanos）
- **本地 compose**：`docker compose --profile observability up` 使用 `deploy/local/`

> 禁止在 `infra/` 根目录放置 YAML/Terraform 文件；新资源必须归入 `terraform/` 或 `k8s/`。
