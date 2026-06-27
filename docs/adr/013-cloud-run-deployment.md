# ADR-013: 部署平台 — 从 Cloud Run 收敛到 GKE（多区域）

## 状态

已接受（已被 ADR-014/016 演进为多区域终态；Cloud Run 双平台已废弃）

## 背景

需要将容器化 Go 服务部署到生产，兼顾自动扩缩、HTTPS、运维成本，以及**有状态实时
WebSocket** 的特殊要求。

## 决策演进

### 初始决策（已废弃）：Google Cloud Run

最初采用 Cloud Run + GCR 不可变镜像（git SHA tag），看重无服务器扩缩、内置 HTTPS、
低运维成本。

### 终态决策：GKE（单一平台，多区域）

**全部服务统一部署到 GKE StatefulSet + headless Service**，废弃 Cloud Run / 双平台。

## 理由（为何离开 Cloud Run）

ADR-005 最终采用 **owner 反向代理** 实现多实例水平扩展：非 owner 实例需把 WebSocket
转发到拥有该房间的实例，这要求**实例之间网络可寻址**。Cloud Run 实例彼此不可寻址
（无稳定 Pod IP/DNS、无法直接实例间拨号），与按房间分片路由根本冲突。

继续维护「无状态 REST 跑 Cloud Run + WebSocket 跑 GKE」的双平台会带来：
- 两套部署/可观测性/IAM 心智与故障域；
- 同进程内 REST 与 WS 共享 Hub/连接池，强行拆分得不偿失。

因此**收敛为单一 GKE 平台**：REST 与 WebSocket 同镜像、同 StatefulSet。

## 终态形态（ADR-014/016）

| 维度 | 方案 |
|------|------|
| 计算 | 每区域 GKE StatefulSet（`infra/base/service.yaml`）+ HPA（`infra/base/hpa.yaml`） |
| 入口 | 全局 Multi-Cluster Ingress / Anycast（`infra/global/multicluster-ingress.yaml`） |
| 多区域 | 每区域 overlay（`infra/overlays/<region>/`）注入 region 配置 + Workload Identity |
| 身份 | GKE Workload Identity GSA（`infra/main.tf`），免长期密钥 |
| 实例寻址 | downward API 注入 `POD_IP`/`INSTANCE_ID`，owner 反向代理（ADR-005） |
| 镜像 | GCR 不可变 git SHA tag + Cosign 签名（CI build-push 不变） |

## 备选方案

| 方案 | 放弃原因 |
|------|---------|
| Cloud Run（含双平台） | 实例不可寻址，无法做 owner 反向代理（核心原因） |
| Cloudflare Workers | 有状态 WebSocket + PG 不适合 DO 模型（见 ADR-001） |
| 裸 VM + systemd | 无自动扩缩与滚动更新、无多区域编排 |

## 权衡

- GKE 运维复杂度高于 Cloud Run，但换来实例可寻址、多区域编排、统一平台。
- 失去 scale-to-zero；以 HPA minReplicas + 区域内排空（ADR-005）平衡成本与可用性。
- CI deploy 改为逐区域 `kubectl apply -k`（见 `.github/workflows/ci-cd.yml`）。
