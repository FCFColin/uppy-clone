# 企业级生产就绪自检审计（Prompt A 第二轮）Spec

## Why
本项目上一轮已覆盖基础安全修复、性能优化、代码重构和技术选型调整。本轮以"企业级工程实践学习"为核心目标——识别"个人项目写法"与"企业级写法"的差距，解释企业为何这样做（业务/协作/运维原因），按企业标准实施改造并以 ADR 记录决策依据。即使某些改造对当前规模"过度设计"，只要是行业标准实践就应实施并注明理由，以便向面试官解释时有据可查。

## What Changes
- 产出 `docs/audit-enterprise.md`：覆盖 10 个维度（A-J）的企业级差距矩阵，每项引用具体文件名+行号
- 产出 `tasks-enterprise.md`：基于审计结果的有序可验证实施任务清单
- 产出 `checklist-enterprise.md`：实施验证检查清单
- 审计报告须包含：差距矩阵（✅/⚠️/❌）、学习价值标注（🎓/💼/🔭/📋）、工具选型建议（开源/企业 SaaS/Go 集成）、优先级矩阵（就业价值 × 实施复杂度）
- 实施阶段每个改动须在代码注释或文档中记录"为什么企业需要这个"+"做了什么权衡"

**BREAKING**：本 Spec 阶段仅产出审计报告与实施规划文档，不修改业务代码；代码改造在 Phase 2 获用户明确批准后另行实施。

## Impact
- Affected code: 全项目作为审计对象（`backend/`、`frontend/`、`docs/`、`infra/`、`.github/`、`Dockerfile`、`docker-compose.yml` 等）
- Affected docs: 新增 `docs/audit-enterprise.md`；实施阶段可能新增/更新 `docs/architecture.md`、`docs/adr/`、`docs/runbook.md` 等
- 与并行进行的 plan2.md（垂直深度审计，8 维度）正交，两份报告后续可合并学习清单

## ADDED Requirements

### Requirement: 企业级差距矩阵审计
系统 SHALL 产出 `docs/audit-enterprise.md`，按 10 个维度（A-J）逐项评估，每项标注 ✅ 已达企业标准 / ⚠️ 部分达标 / ❌ 未达标，并引用具体文件名与行号作为证据。

#### Scenario: 覆盖全部 10 个维度
- **WHEN** 审计报告生成完成
- **THEN** 报告包含维度 A（系统设计与架构）、B（可观测性三支柱）、C（弹性工程）、D（CI/CD 与 DevSecOps）、E（测试策略）、F（API 设计成熟度）、G（安全深度）、H（云原生与容器化）、I（数据库工程）、J（工程文化与协作就绪）共 10 节
- **AND** 每节内每个检查项均标注达标状态与证据（文件名+行号），禁止基于猜测得出结论

### Requirement: 学习价值与工具选型标注
审计报告 SHALL 对每个改造项标注学习价值（🎓 面试高频 / 💼 工作每天用到 / 🔭 高级工程师技能 / 📋 行业标准规范），并给出开源方案、企业常用 SaaS/商业方案、Go 生态具体集成方式（库名+示例用法）。

#### Scenario: 改造项含完整元数据
- **WHEN** 报告列出任一改造项
- **THEN** 该项包含学习价值标注、开源方案、企业商业方案、Go 集成方式

### Requirement: 优先级矩阵
审计报告 SHALL 提供按"就业价值 × 实施复杂度（Low/Medium/High）"两轴排序的优先级矩阵，作为实施顺序依据。

### Requirement: 容量瓶颈分析
审计报告 SHALL 包含"流量增长 100 倍"场景下的瓶颈分析，从系统设计视角指出当前架构最先崩溃点，并给出具体应对方案（读写分离 / 缓存层 / 队列解耦 / 水平扩展）。

### Requirement: 两阶段交付与门控
审计 SHALL 分两阶段交付：Phase 1 产出 `docs/audit-enterprise.md` 并等待用户确认；Phase 2 在确认后产出 `tasks-enterprise.md` 与 `checklist-enterprise.md`，并等待用户明确批准后才进入代码实施。

#### Scenario: Phase 1 报告先行
- **WHEN** Phase 1 审计报告完成
- **THEN** 暂停并等待用户确认报告
- **AND** 在确认前不启动 Phase 2

#### Scenario: Phase 2 实施规划
- **WHEN** 用户确认 Phase 1 报告
- **THEN** 产出 `tasks-enterprise.md` 与 `checklist-enterprise.md`
- **AND** 在用户明确批准前不修改任何业务代码

### Requirement: 改造决策记录
实施阶段每个改动 SHALL 在代码注释或对应文档中说明"为什么企业需要这个"+"做了什么权衡"，形成可向面试官展示的决策依据。
