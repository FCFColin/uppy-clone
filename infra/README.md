# Infrastructure

本目录包含**应用运行时**基础设施，不含可观测性栈（见 [`../deploy/`](../deploy/)）。

| 子目录 | 用途 |
|--------|------|
| [`terraform/`](terraform/) | GCP 资源：Cloud SQL、Memorystore Redis、GSA、Workload Identity |
| [`k8s/base/`](k8s/base/) | 应用 Kustomize base（StatefulSet、HPA、Redis、PDB） |
| [`k8s/overlays/<region>/`](k8s/overlays/) | 区域 overlay 模板（`region/`，CI/CD sed 替换 __REGION__） |

> 禁止在 `infra/` 根目录放置 YAML/Terraform 文件；新资源必须归入 `terraform/` 或 `k8s/`。
