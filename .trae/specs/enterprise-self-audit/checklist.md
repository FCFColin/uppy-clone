# Checklist

## 审计报告完整性
- [x] `docs/audit-enterprise.md` 已生成
- [x] 报告覆盖维度 A-J 全部 10 个维度
- [x] 每个检查项标注 ✅/⚠️/❌ 达标状态
- [x] 每个发现引用具体文件名与行号（证据优先，禁止猜测）

## 维度 A：系统设计与架构
- [x] 含 Mermaid 架构图（组件关系+数据流）
- [x] 含所有关键技术选型的 ADR 条目
- [x] 含已知局限性与未来扩展点
- [x] 含流量增长 100 倍的瓶颈分析与应对方案（读写分离/缓存/队列/水平扩展）

## 维度 B：可观测性三支柱
- [x] Logs：结构化 JSON、request_id/trace_id、耗时、用户上下文
- [x] Logs：error 日志含完整调用链（不泄露敏感信息）、audit log、日志级别策略
- [x] Metrics：/metrics 端点（Prometheus 格式）
- [x] Metrics：黄金信号（Latency P50/P95/P99、Traffic RPS、Errors 4xx/5xx、Saturation）
- [x] Metrics：业务指标（非仅系统指标）
- [x] Traces：OpenTelemetry 集成、HTTP 请求完整 span 覆盖、trace_id 贯穿日志

## 维度 C：弹性工程
- [x] 熔断器（DB/第三方 API，雪崩防御方案）
- [x] 退避重试+Jitter，区分幂等/非幂等操作
- [x] 分层超时（连接/读/整体请求各自独立配置）
- [x] 优雅降级（功能不可用时返回降级响应）
- [x] 舱壁模式（不同请求类型独立资源池）

## 维度 D：CI/CD 与 DevSecOps
- [x] lint / test（含 go test -race）/ security scan（govulncheck 或 trivy）/ container scan / build&push
- [x] 镜像 tag 为 git commit SHA（非 latest）
- [x] 自动生成变更日志或部署摘要
- [x] shift-left security 理念说明
- [x] branch protection 规则建议（PR review、强制检查通过才可合并）

## 维度 E：测试策略
- [x] 测试金字塔分布评估（单元/集成/契约/E2E）
- [x] Table-Driven Tests 覆盖边界与异常路径
- [x] 竞态测试（go test -race）
- [x] 性能基准测试（go test -bench）防回退
- [x] 测试欠债地图（核心无测试路径按风险排序）

## 维度 F：API 设计成熟度
- [x] OpenAPI/Swagger 规格文档完整性（端点/schema/错误码）
- [x] API 版本化（/v1/）评估与企业必要性说明
- [x] 统一错误 envelope（RFC 7807 Problem Details）
- [x] 幂等性保障（Idempotency Key）
- [x] 分页策略权衡（cursor vs offset）与切换时机
- [x] HTTP 语义（GET 幂等 / PUT 全量 vs PATCH 部分 / 202/409/422 使用）

## 维度 G：安全深度
- [x] 认证（JWT+Refresh Token、token 轮换、撤销列表）
- [x] 授权（RBAC 或 ABAC，每端点明确声明所需权限）
- [x] STRIDE 威胁建模（每类威胁具体表现+缓解措施）
- [x] 数据分类（PII 识别）与 GDPR/CCPA 合规含义
- [x] 安全 HTTP 响应头完备（HSTS/X-Frame-Options/CSP/X-Content-Type-Options）
- [x] Rate Limiting 按用户/IP/端点维度分别实施

## 维度 H：云原生与容器化
- [x] Dockerfile 多阶段构建 + distroless/alpine 基础镜像
- [x] 非 root 用户运行（USER nonroot）
- [x] 基础镜像 pin digest（防供应链攻击）
- [x] .dockerignore 完备
- [x] K8s 就绪：liveness/readiness/startup probe 区分与配置
- [x] 资源 requests/limits 基于实测
- [x] 水平扩展（无状态设计、会话/缓存外移）
- [x] PodDisruptionBudget 考虑
- [x] 12-Factor 配置管理（环境变量，无硬编码/提交 VCS）

## 维度 I：数据库工程
- [x] 迁移管理（版本化、可回滚、Up/Down 双向）
- [x] 迁移在 CI 中被测试（含回滚测试）
- [x] 索引策略（EXPLAIN ANALYZE 报告、复合索引列顺序、冗余索引排查）
- [x] 数据完整性（外键 DB 层声明、CHECK 约束、事务边界）
- [x] 连接池配置经压测验证（MaxOpenConns/MaxIdleConns/ConnMaxLifetime）

## 维度 J：工程文化与协作就绪
- [x] CONTRIBUTING.md（本地搭建/代码风格/PR 规范/Conventional Commits）
- [x] .editorconfig + golangci-lint 配置
- [x] pre-commit hook（提交前自动格式化与 lint）
- [x] CHANGELOG.md（Keep a Changelog 规范）
- [x] API deprecation 策略
- [x] docs/runbook.md（5 类故障+排查步骤）

## 学习价值与工具选型
- [x] 每个改造项标注学习价值（🎓/💼/🔭/📋）
- [x] 每个改造项给出开源方案
- [x] 每个改造项给出企业常用 SaaS/商业方案
- [x] 每个改造项给出 Go 生态集成方式（库名+示例用法）

## 优先级矩阵
- [x] 按就业价值 × 实施复杂度（Low/Medium/High）两轴排序

## 阶段门控
- [x] Phase 1 审计报告输出后暂停，等待用户确认
- [ ] Phase 2 `tasks-enterprise.md` 与 `checklist-enterprise.md` 在确认后产出
- [ ] 用户明确批准前不修改任何业务代码
