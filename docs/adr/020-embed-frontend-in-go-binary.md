# ADR-020: 前端 dist 嵌入 Go 二进制单镜像部署

## 状态: 提议中（审计草稿，2026-06-26）

## 上下文

部署目标为 GKE 多区域 StatefulSet（ADR-013），需要：
- 最小化 K8s 部署单元数量
- 不可变容器镜像（SLSA L2，Dockerfile digest 锁定）
- 非 root 运行时（distroless nonroot）
- 开发时前后端分离（Vite dev server proxy 到 Go API）

## 决策

采用 **三阶段 Docker 构建，单二进制服务 API + 静态资源**：
1. Stage 1：Node 20 构建前端 `npm run build` → `dist/`
2. Stage 2：Go 编译 `cmd/server` 静态二进制
3. Stage 3：distroless 镜像包含 `/server` + `/dist` + `/migrations`

Go server 在运行时通过 `http.FileServer` 或等效机制服务 `dist/` 中的静态文件（`cmd/server/main.go` 静态路由段）。

开发模式保持分离：Vite `:5173` proxy `/api` 和 `/lobby` 到 Go `:8080`（`vite.config.ts:4-37`）。

## 后果

**正面**
- 单 Pod 单容器，简化 K8s 配置和 HPA
- 无 Nginx sidecar 依赖
- 前端版本与 API 版本原子发布（同一镜像 tag）
- distroless 攻击面最小

**负面**
- 静态资源无 CDN 边缘缓存（章程非目标，但真实流量需要）
- 前端变更需重建整个 Docker 镜像（含 Go 编译）
- CSP nonce 难以注入 Vite 构建产物（`security.go:48-58`）
- 无法独立扩缩静态资源层

**放弃的替代方案**
- Nginx 独立托管静态资源：多一个部署单元
- Cloudflare Pages / Vercel 托管前端：与 GKE 后端分离，增加 CORS/认证复杂度
- Go `embed` FS：当前选择 COPY dist 目录，等效但 Dockerfile 更清晰
