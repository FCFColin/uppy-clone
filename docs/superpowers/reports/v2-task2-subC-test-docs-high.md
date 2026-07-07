# v2 自检 Task2 子代理 C：测试 + 文档 High 资产审查报告

> 审查时间：2026-07-08
> 审查范围：A-056/057/058/060（测试）+ A-077/078/079/080（文档）共 8 个 High 资产
> 审查轴：正确性 / 可读性 / 可维护性 / 可观测性 / 文档一致性（按资产适用轴）
> v1 基线报告：**缺失**（`docs/superpowers/reports/2026-07-07-full-self-inspection-report.md` 不存在，无法对比，仅 `v2-task1-results.md` 与 `v2-asset-inventory.md` 存在）

---

## 资产 A-056: E2E 测试

### 基本信息
- 路径: `tests/e2e/*.spec.ts`（11 个 spec 文件）
- 关键性: High
- 适用轴: 正确性 + 可读性 + 可维护性 + 可观测性 + 文档一致性
- 文件清单: `cross_page.spec.ts`、`midgame-reconnect.spec.ts`、`reconnect.spec.ts`、`concurrency.spec.ts`、`gameplay.spec.ts`、`error_handling.spec.ts`、`network_boundary.spec.ts`、`security.spec.ts`、`admin.spec.ts`、`auth.spec.ts`、`multiplayer.spec.ts`
- 配置: `playwright.config.ts`（chromium only，retries=1，workers=4，webServer 自启 backend）

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | 断言基本准确，少量断言较弱（`error_handling.spec.ts:33-37` 接受 `ended` 或 `playing` 二选一，弱断言） |
| 可读性 | 4 | 描述清晰、helper 抽象合理；少量内联重复代码（`multiplayer.spec.ts` 三 test 重复"建房+加入"流程） |
| 可维护性 | 3 | 大量 `waitForTimeout`（≥12 处，如 `reconnect.spec.ts:19,41`、`network_boundary.spec.ts:12,25,28`）存在 flaky 风险；与 ADR-023 自承"12+ 测试文件使用 time.Sleep"呼应 |
| 可观测性 | 3 | 无日志断言、无 metrics/tracing 路径覆盖；仅靠 Playwright 默认 trace（on-first-retry）+ screenshot |
| 文档一致性 | 3 | 大量使用 `POST /api/v1/registry/match`（helpers.ts:17、multiplayer/concurrency/gameplay），但 openapi.yaml 标记此端点为 `deprecated: true` 并称"未实现"——文档与测试用例严重矛盾 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-52 | 可维护性 | REQUIRED | E2E 普遍使用 `page.waitForTimeout()` 等待固定时长（≥12 处），在 CI 慢机/资源争抢下易 flaky | `reconnect.spec.ts:19,41`、`network_boundary.spec.ts:12,25,28`、`midgame-reconnect.spec.ts:14` 等 | 改为基于状态/事件的 `expect(...).toBeVisible()` 或 `waitForFunction` 等待条件，减少固定 sleep |
| v2-R-53 | 文档一致性 | REQUIRED | E2E 大量依赖 `POST /api/v1/registry/match` 端点（helpers.ts + 5 个 spec），但 openapi.yaml:482-510 将其标记为 `deprecated: true` 且描述"未实现：此端点尚未实现"——文档与测试/代码实现严重矛盾，会误导 API 消费者 | `tests/e2e/helpers.ts:16-22`、`docs/api/openapi.yaml:482-510` | 修正 openapi.yaml：移除 deprecated 标记，补全 match 端点的完整响应定义（字段名 `lobbyCode`，限速、RBAC 等） |
| v2-R-54 | 正确性 | REQUIRED | `error_handling.spec.ts:31-37` 断言接受 `phase === 'ended' \|\| phase === 'playing'` 二选一即通过，无法有效验证"游戏结束"语义 | `tests/e2e/error_handling.spec.ts:31-37` | 收紧断言：明确预期阶段，或分别覆盖 ended/playing 两条路径 |
| v2-O-35 | 可维护性 | OPTIONAL | `multiplayer.spec.ts` 三个 test 重复"建房+玩家2加入+提交昵称"约 20 行流程 | `tests/e2e/multiplayer.spec.ts:5-35,37-68,70-130` | 抽取到 helpers.ts 的 `createTwoPlayerRoom(page1, page2)` 复用 |
| v2-O-36 | 可观测性 | OPTIONAL | 无任何针对 `/metrics`、`/health/degraded`、OTel span、审计日志路径的 E2E 覆盖 | 全部 E2E | 补充可观测性端点的烟雾测试（至少验证 /metrics 200 与 /health/degraded 200） |
| v2-F-30 | 可维护性 | FYI | `admin.spec.ts:3` 硬编码默认密码 `'DevAdmin2024!Secure'`，仅作 dev fallback，生产应通过 `ADMIN_PASSWORD` env 覆盖 | `tests/e2e/admin.spec.ts:3` | 信息性备注；建议加注释说明此为 dev-only fallback |
| v2-F-31 | 文档一致性 | FYI | `playwright.config.ts:17-22` 注释了 Firefox/Safari 项目，仅启用 chromium；README/coverage 文档未明确说明 E2E 仅单浏览器覆盖 | `playwright.config.ts:23-28` | 在 docs/development/coverage-policy.md 中标注 E2E 浏览器覆盖范围 |

### 整体健康度: 🟡 3.4/5

---

## 资产 A-057: E2E helpers

### 基本信息
- 路径: `tests/e2e/helpers.ts`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 可维护性 + 可观测性 + 文档一致性

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | 房间码正则 `/^[A-Z2-9]{5}$/` 与 `domain/room_code.go:11` 一致（5 位） |
| 可读性 | 5 | 每个 helper 单一职责、JSDoc 注释完整、命名清晰 |
| 可维护性 | 4 | 抽象合理；`createRoomViaUI` 与 `createTestUser` 略有重叠（line 73-75 仅做转发） |
| 可观测性 | 3 | helper 内部无日志输出，失败时仅靠 Playwright 默认 trace |
| 文档一致性 | 3 | helper 用 `lobbyCode` 字段名（line 19），与 openapi.yaml:502 声明的 `code` 字段名不一致——但 helper 实际匹配后端代码（`lobby_registry.go:204`） |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-55 | 文档一致性 | REQUIRED | `matchRoom` 解构 `lobbyCode` 字段（line 19），与 openapi.yaml:502 声明的响应字段 `code` 不一致；helper 与后端实际实现一致（`lobby_registry.go:204` 用 `lobbyCode`），说明 openapi.yaml 错误 | `tests/e2e/helpers.ts:19` vs `docs/api/openapi.yaml:502` | 修正 openapi.yaml match 响应字段名为 `lobbyCode` |
| v2-O-37 | 可维护性 | OPTIONAL | `createRoomViaUI`（line 73-75）仅转发 `createTestUser`，无额外逻辑，存在无意义中间层 | `tests/e2e/helpers.ts:73-75` | 直接调用 `createTestUser`，或移除此包装函数 |
| v2-O-38 | 可观测性 | OPTIONAL | helpers 无失败诊断信息（如 quickplayAuth 失败时不打印 response body） | `tests/e2e/helpers.ts:4-13` | 在 `expect(res.ok()).toBeTruthy()` 失败前先 `console.error(await res.text())` 辅助排查 |

### 整体健康度: 🟢 4.0/5

---

## 资产 A-058: 后端 property 测试

### 基本信息
- 路径: `backend/internal/protocol/property_test.go`、`backend/internal/game/physics_property_test.go`、`backend/internal/game/state_property_test.go`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 可维护性 + 可观测性 + 文档一致性
- 框架: `pgregory.net/rapid`

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | 不变量断言清晰（重力方向、位置边界、序列化 roundtrip、wind 范围）；少量 guard 过宽导致 coverage 下降 |
| 可读性 | 4 | 测试名表达意图清晰（`TestPhysics_ApplyPhysicsGravityAlwaysDownward`）；rapid 用法规范 |
| 可维护性 | 4 | 纯函数测试，无外部依赖，无 flaky 风险；`physics_property_test.go` 存在已归档的 fail 文件（`testdata/rapid/TestPhysics_ApplyPhysicsPositionInBounds-...fail`） |
| 可观测性 | 3 | 失败信息仅 `t.Fatal` 字符串，无 rapid shrink 后的输入回放细节（rapid 默认会输出，但断言信息偏简） |
| 文档一致性 | 4 | 与 ADR-023 混合测试策略一致；与 ADR-002 二进制协议描述一致 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-56 | 可维护性 | REQUIRED | `backend/internal/game/testdata/rapid/TestPhysics_ApplyPhysicsPositionInBounds-20260707054413-12908.fail` 为 rapid 失败归档文件，已被提交到仓库——可能是历史 flaky 失败未清理 | `backend/internal/game/testdata/rapid/` | 确认是否已修复；若已修复则删除此 fail 文件并在 `.gitignore` 排除 `testdata/rapid/*.fail` |
| v2-O-39 | 正确性 | OPTIONAL | `physics_property_test.go:18-20,36-38` 用 `if y <= 0 || y >= 1 ... return` 提前退出，导致边界外的输入不参与断言，property 覆盖率下降 | `backend/internal/game/physics_property_test.go:18-20,36-38` | 改用 `rapid.Float64Min(0.01).Max(0.99)` 等约束生成器，让所有生成的输入都参与断言 |
| v2-O-40 | 可观测性 | OPTIONAL | `state_property_test.go:124-134`（TestState_DeserializeStateNilMaps）中 `rapid.Int64().Draw(t, "seed")` 的返回值未被使用，rapid 生成无意义 | `backend/internal/game/state_property_test.go:125` | 删除无用的 `Draw` 调用，或真正用 seed 驱动随机化输入 |
| v2-F-32 | 文档一致性 | FYI | `protocol/property_test.go:18-26` 中 `BirdState{X: 0.3, Y: 0.4}`、`GhostState{X: 0.6, Y: 0.5}` 为硬编码常量，未与 protocol 文档中的字段布局示例对应 | `backend/internal/protocol/property_test.go:18-26` | 信息性备注；可考虑抽取为测试夹具常量 |

### 整体健康度: 🟢 4.0/5

---

## 资产 A-060: 前端 property 测试

### 基本信息
- 路径: `frontend/src/game/message_codec.property.test.ts`、`frontend/src/game/reducer.property.test.ts`、`frontend/src/game/snapshot_decode.property.test.ts`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 可维护性 + 可观测性 + 文档一致性
- 框架: `vitest` + `fast-check`

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | 不变量覆盖完整（昵称截断≤12 runes、cooldown 单调、reducer 不可变、decode 不 panic） |
| 可读性 | 4 | describe/it 结构清晰，断言语义化 |
| 可维护性 | 3 | 多处 `try { ... } catch { return; }` 静默吞错（snapshot_decode.property.test.ts:37-53,65-72 等≥6处），并标注"Known limitation: oversized nickLen can throw"——已知缺陷未修复 |
| 可观测性 | 3 | catch 块仅注释"Known limitation"，未记录输入，失败时难复现 |
| 文档一致性 | 3 | 测试承认 `decodeSnapshot` 对"超长 nickLen"会 throw 是 known limitation，但 ws-protocol.md 与 asyncapi.yaml 未声明此限制 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-57 | 可维护性 | REQUIRED | `snapshot_decode.property.test.ts` 与 `message_codec.property.test.ts` 共≥6 处 `try { decodeSnapshot(...) } catch { return; }` 静默吞错，注释"Known limitation: very short buffers with oversized nickLen can throw"——这是真实的解码缺陷被测试"合法化"，而非测试捕获 | `frontend/src/game/snapshot_decode.property.test.ts:37-53,65-72,87-90,108-111,127-130,145-148`、`message_codec.property.test.ts:78-82,101-106,127-130` | 修复 `decodeSnapshot` 实现，使其对所有二进制输入不抛异常（返回 null 或截断），而非在测试中 catch；或将"已知限制"明确写入 ws-protocol.md 的错误处理章节 |
| v2-O-41 | 可维护性 | OPTIONAL | `message_codec.property.test.ts:59` 硬编码"37 字节"作为最短 snapshot 长度阈值，与后端 `calcSnapshotSize` 计算逻辑耦合——若后端加字段会静默失效 | `frontend/src/game/message_codec.property.test.ts:59-69` | 抽取为共享常量（与 `protocol/constants.go` 同步），或在测试中动态计算 |
| v2-O-42 | 可观测性 | OPTIONAL | catch 块无任何日志或输入记录，property 失败时无法回放触发输入 | `frontend/src/game/snapshot_decode.property.test.ts:51-53` 等 | 在 catch 中调用 `fc.log()` 或记录 buffer 长度供 shrink 报告 |
| v2-F-33 | 文档一致性 | FYI | `reducer.property.test.ts:5` 硬编码 `VALID_PHASES = ['waiting', 'countdown', 'playing', 'ended']`，与后端 `domain.Phase*` 常量重复定义 | `frontend/src/game/reducer.property.test.ts:5` | 信息性备注；可从 `shared/game/types.ts` 导入共享类型 |

### 整体健康度: 🟡 3.4/5

---

## 资产 A-077: ADR（Architecture Decision Records）

### 基本信息
- 路径: `docs/adr/`（000~029 共 30 个 ADR + README.md）
- 关键性: High
- 适用轴: 正确性 + 可读性 + 文档一致性
- 状态分布（据 README）：已接受 24、提议中 3（014/015/016）、已废弃 1（013）、部分落地 2（022/023）

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | 多处代码引用行号过时（ADR-022 的 `aes.go:162-165` 等）；ADR-022 "RotateKey 未实现"已过时；ADR-018 "仅 2 个 vitest 文件"已过时 |
| 可读性 | 4 | 模板统一（状态/上下文/决策/后果），关联引用规范 |
| 文档一致性 | 2 | ADR-014 README 状态与文件不一致；ADR-013 引用 ADR-028 错误；ADR-025 README 标题与文件标题不一致；ADR-018/025 关于"可变单例"vs"受控状态"矛盾 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-C-10 | 文档一致性 | CRITICAL | ADR-013 line 4 引用"详见 ADR-028 (GKE multi-region)"，但 ADR-028 实际标题为"Clean Architecture Interface-Driven Decoupling"，与 GKE/多区域部署无关——**错误的 ADR 交叉引用**会误导读者寻找不存在的 GKE 决策 | `docs/adr/013-cloud-run-deployment.md:4` | 修正引用：GKE 多区域终态应指向 ADR-014（多区域拓扑）或保留 ADR-013 自身终态章节 |
| v2-C-11 | 文档一致性 | CRITICAL | ADR-022 "负面"章节声明 `RotateKey() 未实现（aes.go:162-165）`，但实际代码 `backend/internal/crypto/aes.go:212-219` 已完整实现 `RotateKey(oldKey, newKey []byte) error`；同时引用的行号 `aes.go:162-165` 实际是 `ReEncryptWithKey` 内的 hex decode 逻辑，非"未实现"占位——**文档声称的缺陷已不存在**，会误导评审者将其列为待修债务 | `docs/adr/022-field-level-pii-encryption.md:39` vs `backend/internal/crypto/aes.go:212-219` | 更新 ADR-022：移除"RotateKey 未实现"负面项，或在"实施现状"章节说明已于何时实现 |
| v2-R-58 | 文档一致性 | REQUIRED | ADR-014 README 表格标注状态"提议中"（README line 26），但 ADR-014 文件内部标注"已接受"（file line 5）——索引与文件状态不一致 | `docs/adr/README.md:26` vs `docs/adr/014-multi-region-topology.md:5` | 同步状态：以 ADR-014 文件为准更新 README 为"已接受"，或反向修正 |
| v2-R-59 | 文档一致性 | REQUIRED | ADR-013 标注"已废弃（Cloud Run 部署路径已被 ADR-014/016 的 GKE 多区域终态取代）"，但被引用为取代方的 ADR-016 状态仍为"提议中"——已废弃 ADR 不应被"提议中"的 ADR 取代，逻辑矛盾 | `docs/adr/013-cloud-run-deployment.md:8` vs `docs/adr/016-region-local-rooms.md:5` | 明确多区域终态的实际落地状态：若 ADR-016 未落地，则 ADR-013 的"废弃"应改为"部分废弃（GKE 已落地，多区域未落地）" |
| v2-R-60 | 文档一致性 | REQUIRED | ADR-025 README 标题为"前端可变单例状态管理"（README line 37），但 ADR-025 文件实际标题为"前端受控状态管理 (GameStore)"，内容描述 dispatch/reducer 不可变更新模式——**README 标题与文件标题语义相反**（可变 vs 受控/不可变） | `docs/adr/README.md:37` vs `docs/adr/025-frontend-mutable-singleton-state.md:1,3` | 更新 README 标题为"前端受控状态管理 (GameStore)"；考虑重命名 ADR-025 文件名（`025-frontend-mutable-singleton-state.md` → `025-frontend-controlled-state-store.md`） |
| v2-R-61 | 文档一致性 | REQUIRED | ADR-022 声明 `EncryptEmailForStorage 在 encKey == nil 时回退明文（aes.go:146-149）`，行号错误：该函数实际在 `aes_email.go:38-43`（不在 aes.go），且 `aes.go:146-149` 实际是 `ReEncryptWithKey` 的 hex decode 中段——代码引用行号过时 | `docs/adr/022-field-level-pii-encryption.md:40` | 修正行号引用为 `aes_email.go:38-43`；同时澄清"回退明文仅 dev 安全"的语义（生产 `InitFromEnv` 会 fail-fast，不会 encKey==nil） |
| v2-R-62 | 文档一致性 | REQUIRED | ADR-018 line 21 声明"状态管理：可变单例 `state` 对象（`frontend/src/game/state.ts:71-92`），无 Redux/Zustang"，但 ADR-025（2026-07-03，晚于 ADR-018）已将其改为 dispatch/reducer 受控状态——ADR-018 未标注此决策已被 ADR-025 取代；同时 "Zustang" 是 "Zustand" 的拼写错误 | `docs/adr/018-frontend-vanilla-ts-mpa.md:21` | 在 ADR-018 line 21 加注"（已被 ADR-025 取代为受控状态管理）"；修正拼写 "Zustang" → "Zustand" |
| v2-R-63 | 文档一致性 | REQUIRED | ADR-018 line 34 声明"前端测试覆盖极低（仅 2 个 vitest 文件 vs ~22 源文件）"，但当前 `frontend/src/game/` 已有 30+ 个 `.test.ts` 文件（含 3 个 property test）——负面描述已过时 | `docs/adr/018-frontend-vanilla-ts-mpa.md:34` | 更新为当前测试文件数（约 30+），或改为"前端测试已大幅扩展，详见 docs/development/coverage-policy.md" |
| v2-R-64 | 文档一致性 | REQUIRED | ADR-022 line 42 声明 `AUDIT_SECRET 当前可回退到 JWT_SECRET（main.go:113），审计完整性与签名密钥耦合`，行号 `main.go:113` 需核对是否仍准确（cmd/server/main.go 已演进） | `docs/adr/022-field-level-pii-encryption.md:42` | 核对 `cmd/server/main.go` 当前行号，更新引用；若已解耦则移除此负面项 |
| v2-O-43 | 可读性 | OPTIONAL | ADR-000 "附录 A 清理"章节声明"ADR 编号重复（两个 011）→ 去重（bounded-contexts 改为 017）"，ADR-017 line 15 也确认此历史——但 README 索引未标注此编号变迁史 | `docs/adr/000-project-charter.md:89`、`docs/adr/017-bounded-contexts.md:15` | 信息性备注；可在 README 加"编号变迁说明"脚注 |
| v2-F-34 | 文档一致性 | FYI | ADR-019 line 32 声明"`postgres.go` 已达 954 行，成为 god-file"——具体行数会随代码演进失效 | `docs/adr/019-no-orm-raw-sql-pgx.md:32` | 改为"postgres.go 行数偏多（>900 行），需考虑拆分"等相对描述 |

### 整体健康度: 🟡 3.0/5

---

## 资产 A-078: 架构文档

### 基本信息
- 路径: `docs/architecture/architecture.md`、`docs/architecture/multi-region-topology.md`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 文档一致性

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | "提议 vs 已实现"章节明确区分，状态标注准确；mermaid 图与代码结构基本一致 |
| 可读性 | 4 | mermaid 图清晰，表格化容量规划；分层描述简洁 |
| 文档一致性 | 3 | line 34 引用"前端可变单例状态（ADR-025）"与 ADR-025 实际内容矛盾；line 213 用删除线标注演进历史，良好实践 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-65 | 文档一致性 | REQUIRED | architecture.md line 34 声明"前端可变单例状态（ADR-025）"，但 ADR-025 已更新为"前端受控状态管理 (GameStore)"，dispatch/reducer 不可变模式——与"可变单例"语义相反 | `docs/architecture/architecture.md:34` | 更新为"前端受控状态管理（ADR-025）" |
| v2-R-66 | 文档一致性 | REQUIRED | architecture.md line 121 数据流图标注 `P1->>S: POST /api/v1/registry/create` 与 `S-->>P1: {code: "ABC123"}`，但"ABC123"是 6 位房间码示例，实际房间码为 5 位（`domain/room_code.go:11`）——示例与代码不一致 | `docs/architecture/architecture.md:121,123,127` | 修正示例为 5 位房间码（如 "ABC23"），与 `domain/room_code.go` 一致 |
| v2-O-44 | 可读性 | OPTIONAL | multi-region-topology.md line 36 引用 `infra/k8s/global/multicluster-ingress.yaml`，已验证文件存在——引用准确，良好 | `docs/architecture/multi-region-topology.md:36` | FYI 正面备注 |
| v2-O-45 | 文档一致性 | OPTIONAL | architecture.md line 75 数据流图标注 `GW["cmd/game-worker 独立进程"]`，但 `backend/cmd/` 目录下仅 `backfill-emails`、`gen-frontend-constants`、`migrate-passwords`、`seed`、`server` 五个——无 `game-worker` 子目录 | `docs/architecture/architecture.md:75` | 核对 `cmd/game-worker` 是否存在；若未实现则标注"（计划中）"或移除该节点 |
| v2-F-35 | 文档一致性 | FYI | architecture.md line 3 标注"最后更新: 2026-06-26"，与 ADR-028（2026-07-03）、ADR-029（2026-07-06）等新决策存在时间差，未反映 ADR-028/029 的接口解耦与 Redis 域拆分 | `docs/architecture/architecture.md:3` | 更新最后更新日期，并在分层章节补充 ADR-028 接口解耦、ADR-029 Redis 域拆分的现状 |

### 整体健康度: 🟡 3.7/5

---

## 资产 A-079: API 文档

### 基本信息
- 路径: `docs/api/openapi.yaml`、`docs/api/asyncapi.yaml`、`docs/api/ws-protocol.md`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 文档一致性
- 一致性测试: `backend/internal/handler/openapi_consistency_test.go`（仅覆盖 QuickPlay 的 `userId` 字段，覆盖极薄）

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 2 | 房间码长度错（6 vs 5）、match 端点状态错（deprecated vs 已实现）、match 响应字段错（code vs lobbyCode）、JWT 算法错（HMAC-SHA256 vs ES256） |
| 可读性 | 4 | OpenAPI 结构规范，RFC 7807 错误响应统一，示例完整 |
| 文档一致性 | 1 | 与后端代码多处严重不一致；缺失端点（POST /verify、/health/degraded、/metrics）；引用不存在的 /resolve 端点 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-C-12 | 文档一致性 | CRITICAL | openapi.yaml 全文声明房间码为"6 位房间代码"（line 461,522,905,1014,1039）且 `minLength: 6, maxLength: 6`（line 525-526），但实际代码 `domain/room_code.go:11` 强制 `len(code) != 5`（5 位），E2E helpers.ts:20 正则也是 `{5}`——**API 契约文档与代码的字段长度约束直接冲突**，客户端按 openapi 生成 6 位 code 会被后端拒绝 | `docs/api/openapi.yaml:461,522-526,905,1014,1039` vs `backend/internal/domain/room_code.go:11` | 全文将"6 位"改为"5 位"，`minLength: 6, maxLength: 6` 改为 `minLength: 5, maxLength: 5` |
| v2-C-13 | 文档一致性 | CRITICAL | openapi.yaml line 482-510 将 `POST /api/v1/registry/match` 标记为 `deprecated: true` 且描述"未实现：此端点尚未实现，仅作为接口预留，不应在生产环境中调用。原计划匹配一个可用房间，当前实现与 create 相同"，但实际代码 `routes_public.go:116-120` 注册了完整中间件链（限流+幂等+auth+RBAC）的 `lobbyHandler.MatchRoom`，`lobby_registry.go:196-208` 实现了独立的 `MatchRoom` handler（调用 `h.hub.MatchRoom(ctx)`，与 CreateRoom 调用 `h.hub.CreateRoom(ctx)` 不同）——**文档声明"未实现/deprecated"但端点实际生产可用且被 E2E 大量调用**，会误导客户端避免使用此端点 | `docs/api/openapi.yaml:482-510` vs `backend/internal/server/routes_public.go:116-120`、`backend/internal/handler/lobby_registry.go:196-208` | 移除 `deprecated: true`，重写 description 为实际行为；补全 200/401/429/503 响应定义；响应字段名用 `lobbyCode`（与代码一致） |
| v2-C-14 | 文档一致性 | CRITICAL | openapi.yaml line 502 声明 match 响应字段为 `code`，但实际 `lobby_registry.go:204` 用 `responseField: "lobbyCode"`，E2E helpers.ts:19 也用 `lobbyCode`——**响应字段名不一致**，按 openapi 生成的客户端会取不到房间码 | `docs/api/openapi.yaml:502` vs `backend/internal/handler/lobby_registry.go:204` | 将 openapi.yaml match 响应字段 `code` 改为 `lobbyCode` |
| v2-R-67 | 文档一致性 | REQUIRED | openapi.yaml 缺失以下已注册路由的文档：`POST /api/v1/auth/verify`（routes_public.go:69，VerifyMagicLinkPost）、`GET /health/degraded`（routes_public.go:58）、`GET /metrics`（routes_public.go:48）、`GET /health`（routes_public.go:46，/health/ready 的别名）——4 个端点无契约文档 | `docs/api/openapi.yaml`（缺失） vs `backend/internal/server/routes_public.go:46,48,58,69` | 补充这 4 个端点的 openapi 定义；/metrics 需说明 basic auth |
| v2-R-68 | 文档一致性 | REQUIRED | ws-protocol.md line 66 与 asyncapi.yaml line 16 均引用 `GET /api/v1/lobby/{code}/resolve` 端点（"客户端连接前先调...得到房间 home region 的 ws_endpoint"），但 `routes_public.go` 未注册此路由，openapi.yaml 也未定义——**3 份文档引用了代码中不存在的 REST 端点** | `docs/api/ws-protocol.md:66`、`docs/api/asyncapi.yaml:16` vs `backend/internal/server/routes_public.go`（无 /resolve） | 实现 `/api/v1/lobby/{code}/resolve` 端点；或在文档中标注"（计划中，ADR-016 提议中）"并说明当前单区域不需要 |
| v2-R-69 | 文档一致性 | REQUIRED | openapi.yaml line 485-489 描述 match 端点"原计划匹配一个可用房间，当前实现与 create 相同"，但实际 `MatchRoom` 调用 `h.hub.MatchRoom(ctx)`（lobby_registry.go:206），`CreateRoom` 调用 `h.hub.CreateRoom(ctx)`（line 75）——两者调用不同的 Hub 方法，"实现与 create 相同"的描述错误 | `docs/api/openapi.yaml:488` vs `backend/internal/handler/lobby_registry.go:75,206` | 重写 description，准确描述 MatchRoom 的实际语义（匹配现有房间或创建新房间） |
| v2-R-70 | 文档一致性 | REQUIRED | `openapi_consistency_test.go` 仅校验 QuickPlay 响应的 `userId` 字段存在（line 47-52），未覆盖房间码长度、match 端点状态、响应字段名等关键契约——CI 守门不足，导致上述 CRITICAL 不一致长期未被发现 | `backend/internal/handler/openapi_consistency_test.go:47-52` | 扩展一致性测试：校验 openapi 路径与 chi 路由注册一致、关键字段长度约束与 domain 校验一致、响应字段名与 handler 实际写入一致 |
| v2-R-71 | 正确性 | REQUIRED | `openapi_consistency_test.go:58` 引用未在函数内声明的 `nickname` 变量（`if nickname != "SchemaTest"`），grep 全 handler 包未发现 `var nickname` 或 `nickname =` 的包级声明——此测试仅在 `//go:build integration` tag 下编译，可能存在编译错误未被常规 `go test ./...` 捕获 | `backend/internal/handler/openapi_consistency_test.go:58-60` | 核对 `nickname` 变量来源：若为包级变量需显式声明并加注释；若为笔误应改为 `body["nickname"]`；并在 CI 增加 `go build -tags integration ./...` 守门 |
| v2-O-46 | 可读性 | OPTIONAL | openapi.yaml line 14 `servers` 仅列 `http://localhost:8080` 与 `https://balloon-game.example.com`，未列 `http://localhost:57266`（playwright.config.ts:3 的 E2E 默认端口） | `docs/api/openapi.yaml:13-17` vs `playwright.config.ts:3` | 补充 E2E/dev 端口说明，或在 servers 加 description 区分 |
| v2-F-36 | 文档一致性 | FYI | asyncapi.yaml line 12 `host: '{region}.balloon.example'` 为多区域目标态，当前单区域未落地——文档未标注"（目标态）" | `docs/api/asyncapi.yaml:11-20` | 加注"（多区域目标态，当前单区域见 openapi.yaml）" |

### 整体健康度: 🔴 2.3/5

---

## 资产 A-080: 安全文档

### 基本信息
- 路径: `docs/security/threat-model.md`、`docs/security/self-check-checklist.md`、`docs/security/logging-policy.md`、`docs/security/self-check-baseline.txt`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 文档一致性

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | threat-model.md JWT 算法描述错误（HMAC-SHA256 vs 实际 ES256）；registry:match 限流标注"配置预留"与代码不符 |
| 可读性 | 4 | STRIDE 分类规范，限流配额表清晰，self-check-checklist 分层合理 |
| 文档一致性 | 3 | threat-model.md 与 jwt.go 算法不一致；与 routes_public.go 的 match 端点状态不一致 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-C-15 | 文档一致性 | CRITICAL | threat-model.md line 11 声明"JWT 使用 HMAC-SHA256 签名，密钥仅服务端持有"，但实际代码 `backend/internal/auth/jwt.go:78` 使用 `jwt.SigningMethodES256`（ECDSA P-256），`jwt.go:23-24` 持有 `*ecdsa.PrivateKey`/`*ecdsa.PublicKey`——**算法族错误**（对称 HMAC vs 非对称 ECDSA），密钥管理模型完全不同，影响威胁建模的 S（欺骗）维度分析准确性 | `docs/security/threat-model.md:11` vs `backend/internal/auth/jwt.go:23-24,78` | 修正为"JWT 使用 ECDSA P-256（ES256）签名，私钥仅服务端持有，公钥可分发验证"；并复核威胁分析中"密钥仅服务端持有"的语义是否仍成立 |
| v2-R-72 | 文档一致性 | REQUIRED | threat-model.md line 63 限流配额表标注 `registry:match | （配置预留） | 10 | 1 分钟 | 否`，但 `routes_public.go:117` 实际为 match 端点注册了 `EndpointRateLimit(cluster.Ephemeral, "registry:match", jwtMgr)`——**端点已上线限流，非"配置预留"** | `docs/security/threat-model.md:63` vs `backend/internal/server/routes_public.go:117` | 更新表格：路径填 `POST /api/v1/registry/match`，移除"配置预留"标注 |
| v2-R-73 | 文档一致性 | REQUIRED | threat-model.md line 36 声明"邮箱存储在 PostgreSQL，传输使用 TLS"，但未提及 ADR-022 的字段级 AES-256-GCM 加密——PII 保护措施描述不完整，与 ADR-022 矛盾 | `docs/security/threat-model.md:36` vs `docs/adr/022-field-level-pii-encryption.md` | 补充"邮箱使用 AES-256-GCM 字段级加密存储（ADR-022），email_hash HMAC 索引查找" |
| v2-R-74 | 文档一致性 | REQUIRED | self-check-checklist.md line 62 声明"POST `/api/v1/auth/verify` 可用于避免 URL token 泄露"，但 openapi.yaml 仅文档化了 `GET /api/v1/auth/verify`（line 133-171），未文档化 POST 变体——checklist 引用了未文档化的端点 | `docs/security/self-check-checklist.md:62` vs `docs/api/openapi.yaml:133-171` | 补充 openapi.yaml 对 POST /api/v1/auth/verify 的定义（与 v2-R-67 关联） |
| v2-O-47 | 可读性 | OPTIONAL | self-check-baseline.txt line 2-3 含 `BASE_SHA=07c34ac...` 与 `HEAD_SHA=265ce1a...`，为 2026-06-27 的快照——未说明当前 HEAD 是否已推进 | `docs/security/self-check-baseline.txt:2-3` | 加注"基线快照日期"，或在自检流程中更新此文件 |
| v2-O-48 | 文档一致性 | OPTIONAL | logging-policy.md 未提及 ADR-029 的 Redis 域拆分对日志字段的影响（stateful vs ephemeral Redis 的熔断器日志应可区分） | `docs/security/logging-policy.md` | 补充 Redis 域标识字段（如 `redis_domain=stateful|ephemeral`） |
| v2-F-37 | 文档一致性 | FYI | self-check-checklist.md line 103-113 "子 agent 结论"章节记录 2026-06-27 的审查状态，引用多个子 agent UUID——信息性备注，需定期更新 | `docs/security/self-check-checklist.md:103-113` | FYI；建议加"最后更新"日期 |

### 整体健康度: 🟡 3.3/5

---

## 汇总统计

### 各资产 v2 整体评分
| 资产 ID | 名称 | 评分 | 健康度 |
|---------|------|------|--------|
| A-056 | E2E 测试 | 3.4/5 | 🟡 |
| A-057 | E2E helpers | 4.0/5 | 🟢 |
| A-058 | 后端 property 测试 | 4.0/5 | 🟢 |
| A-060 | 前端 property 测试 | 3.4/5 | 🟡 |
| A-077 | ADR | 3.0/5 | 🟡 |
| A-078 | 架构文档 | 3.7/5 | 🟡 |
| A-079 | API 文档 | 2.3/5 | 🔴 |
| A-080 | 安全文档 | 3.3/5 | 🟡 |

### 发现数量统计
| 严重级别 | 数量 | 发现 ID 范围 |
|---------|------|-------------|
| CRITICAL | 6 | v2-C-10 ~ v2-C-15 |
| REQUIRED | 23 | v2-R-52 ~ v2-R-74 |
| OPTIONAL | 14 | v2-O-35 ~ v2-O-48 |
| FYI | 8 | v2-F-30 ~ v2-F-37 |
| **合计** | **51** | |

### CRITICAL 发现摘要（按优先级处理）

1. **v2-C-10**: ADR-013 引用"ADR-028 (GKE multi-region)"错误，ADR-028 实际是 Clean Architecture
2. **v2-C-11**: ADR-022 声明"RotateKey 未实现"已过时，代码已完整实现
3. **v2-C-12**: openapi.yaml 房间码长度全部错误（6 位 vs 实际 5 位）
4. **v2-C-13**: openapi.yaml 将 `/api/v1/registry/match` 标记 deprecated/未实现，但端点完全可用
5. **v2-C-14**: openapi.yaml match 响应字段 `code` 与实际 `lobbyCode` 不一致
6. **v2-C-15**: threat-model.md 声明 JWT 用 HMAC-SHA256，实际用 ES256（ECDSA P-256）

### 跨资产重复主题
- **ADR-025 "可变单例 vs 受控状态"矛盾**：影响 A-077（ADR-025 自身 + README）、A-078（architecture.md line 34）、A-077（ADR-018 line 21 未更新）——需统一修正
- **房间码长度 5 vs 6**：影响 A-079（openapi.yaml 全文）、A-078（architecture.md 示例）——需统一为 5 位
- **`/api/v1/registry/match` 状态**：影响 A-079（openapi.yaml deprecated）、A-080（threat-model "配置预留"）、A-056（E2E 大量使用）——需统一为"已实现"
- **`/api/v1/lobby/{code}/resolve` 端点**：A-079 的 3 份文档（openapi/asyncapi/ws-protocol）引用但代码未实现——需实现或标注"计划中"

### v1 基线对比
- v1 基线报告 `docs/superpowers/reports/2026-07-07-full-self-inspection-report.md` **不存在**，无法对比新增/已修/残留。仅 `v2-task1-results.md` 与 `v2-asset-inventory.md` 存在。
