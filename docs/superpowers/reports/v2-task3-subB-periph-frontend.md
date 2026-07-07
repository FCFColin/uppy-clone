# 代码库自检 v2 — Task3 子代理 B：后端外围 cmd/* + 前端 Medium/Low 资产审查报告

**审查范围**：9 个 Medium/Low 资产（后端外围 cmd/* 4 个 + 前端 Medium/Low 5 个）
**审查日期**：2026-07-08
**发现 ID 范围**：CRITICAL v2-C-20+，REQUIRED v2-R-95+，OPTIONAL v2-O-80+，FYI v2-F-63+
**审查性质**：纯诊断，未修改任何业务代码

---

## 资产 A-029: cmd/seed
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | runSeed 吞没逐行插入错误并仍报告"Seed completed" |
| 安全 | 2 | sslmode=disable 子串守卫可被绕过（已有测试将弱点固化为"已接受"）|
| 可维护性 | 4 | 结构清晰，seedUsers/Sessions/Results 职责分离 |
| 弹性 | 3 | 无事务包裹，部分失败留不一致状态 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-R-95 | 安全 | REQUIRED | `runSeed` 守卫 `strings.Contains(dbURL, "sslmode=disable")` 可被 `?sslmode=disable&sslmode=require` 绕过：pgx 取最后一个 sslmode=require 连生产库，但子串检查通过，seed 将对生产数据执行。`TestRunSeed_WeakGuardSubstringBypass` 明确将此弱点固化为"已接受"行为 | main.go:32-34; run_seed_adversarial_test.go:54-58 | 改用 URL 解析后检查 sslmode 键的最终值，而非子串匹配；或要求 dbURL 主机为 localhost/127.0.0.1 |
| v2-R-96 | 正确性 | REQUIRED | `seedUsers/Sessions/Results` 对每个插入错误仅 `log.Printf("...may already exist")` 后继续，不返回错误；`main()` 无条件打印 `Seed completed: 3 users, 5 game sessions, 10 game results`，即使全部插入失败也报成功 | main.go:22, 62-64, 85-87, 111-113 | runSeed 应聚合错误并按实际成功计数报告；或至少在任一插入失败时返回错误 |
| v2-O-80 | 弹性 | OPTIONAL | seed 操作未包裹事务，部分失败（如 users 成功但 sessions 失败）留下不一致种子数据 | main.go:46-51 | 可将 seedUsers+seedSessions+seedResults 包在单事务中，失败整体回滚 |

### 整体健康度: 🟡 3.0/5
结构清晰但安全守卫形同虚设、成功报告不可信。

---

## 资产 A-030: cmd/backfill-emails
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | UPDATE 带 `email_hash IS NULL` 守卫，幂等性好 |
| 安全 | 4 | 加密后再写回，明文不在日志中出现 |
| 可维护性 | 4 | backfillDB 接口抽象良好，可测试 |
| 弹性 | 3 | 全量加载到内存；单条加密失败中断整个回填 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-R-97 | 弹性 | REQUIRED | `run()` 将所有 pending 用户一次性加载到 `var pending []pendingUser` 内存切片后再逐条处理。用户表量大时内存无上限，且 5 分钟超时内可能处理不完 | main.go:64-75 | 改用游标/分页流式处理（每次 SELECT LIMIT N + OFFSET 或 keyset pagination），避免全量驻留内存 |
| v2-O-81 | 弹性 | OPTIONAL | 单条 `encryptEmailFn` 失败即 `return err` 中断整个回填（集成测试 `TestBackfillEmails_StopOnEncryptError` 确认为预期行为）。一次性迁移中一条坏数据会阻塞所有后续用户 | main.go:85-87 | 可改为 skip-and-continue：记录失败 ID 到日志/文件，继续处理其余用户，最后汇总失败列表 |
| v2-O-82 | 可维护性 | OPTIONAL | 该迁移直接原地改写 `email` 列（明文→密文），无 `--dry-run` 模式。误操作不可逆 | main.go:89-94 | 增加 `--dry-run` 标志：只统计将处理的用户数和采样加密结果，不执行 UPDATE |

### 整体健康度: 🟡 3.8/5
加密逻辑与幂等性扎实，但内存弹性与中断策略有改进空间。

---

## 资产 A-031: cmd/migrate-passwords
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | bcrypt 检测短路幂等，迁移逻辑完整 |
| 安全 | 4 | bcrypt cost=12 合理 |
| 可维护性 | 4 | 函数拆分清晰，connectDBFn 可替换 |
| 弹性 | 3 | 读-改-写无事务；RowsAffected 错误被忽略 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-O-83 | 弹性 | OPTIONAL | `loadStoredConfig` + `migratePasswords` 是读-改-写 `admin_config`，未包裹事务。并发执行两次会 last-write-wins | main.go:80-127 | 可在单事务中完成 SELECT...FOR UPDATE + UPDATE；一次性脚本风险低但值得注意 |
| v2-F-63 | 正确性 | FYI | `rowsAffected, _ := result.RowsAffected()` 忽略错误；若驱动不支持该能力，rowsAffected=0 会触发误报"no rows updated" | main.go:123 | lib/pq 支持 RowsAffected，实际无影响；可注释说明假设 |
| v2-F-64 | 可维护性 | FYI | `connectDB` 将 `sql.Open` 和 `PingContext` 错误都包装为 `"failed to connect"`，诊断时无法区分配置错误 vs 网络错误 | main.go:71-76 | 分别包装为 "open: %w" 和 "ping: %w" |

### 整体健康度: 🟢 4.0/5
简洁的一次性迁移脚本，幂等设计到位，发现均为次要。

---

## 资产 A-032: cmd/gen-frontend-constants
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | TickRate 硬编码为 "15"，BinaryExpr 特例脆弱 |
| 可维护性 | 2 | 无任何测试，os.Exit 不可测，特殊用例遍布 |
| 弹性 | 3 | WriteFile 不创建父目录 |

> 注：v2-C-08（输出路径 `frontend/src/shared/constants.ts` 错误，实际应为 `frontend/src/shared/game/constants.ts`）已由 Task 2 记录，此处不重复。

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-R-98 | 正确性 | REQUIRED | `exprValue` 对 `*ast.Ident` 硬编码 `if v.Name == "TickRate" { return "15" }`，且 BinaryExpr 特判 `1000.0 / float64(TickRate)` → `"1000 / 15"`。若 constants.go 中 TickRate 值变更，生成器静默产出陈旧输出，前端物理同步错误 | main.go:134-136, 147-149 | 改为通过 `go/constant` 包评估常量表达式的实际值，或至少从同一 source of truth 读取 TickRate 值而非字面量 |
| v2-R-99 | 可维护性 | REQUIRED | 整个 cmd 无 `*_test.go`；`exprValue`/`isFloat`/`unFloat` 及 BinaryExpr 特例逻辑完全未测试。代码生成一致性是 Medium 资产的核心要求 | (整个目录) | 增加 table-driven 测试覆盖 exprValue 对各种 ast.Expr 类型的输出，包括 BasicLit/Ident/BinaryExpr/CallExpr/ParenExpr |
| v2-O-84 | 正确性 | OPTIONAL | `seen` map 按 tsPath 去重：若两个 Go const 共享同一 `@ts` 路径，第二个被静默丢弃。当前 constants.go 所有路径唯一，无实际丢失，但属潜在风险 | main.go:81-84 | 去重时若遇重复 tsPath 应报错或警告，而非静默忽略 |

### 整体健康度: 🟡 2.7/5
代码生成器缺乏测试且依赖硬编码魔法值，一致性保障薄弱。

---

## 资产 A-043: game/其他（main.ts, local_constants.ts, reducer.ts, visual_helpers.ts, waiting_tips.ts, window_events.ts, ui_cooldown.ts, tutorial.ts, ui_utils.ts, ui_elements.ts）
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | drawDangerVignettes 中幽灵 alpha 设置后无绘制即重置，属死代码 |
| 可读性 | 4 | 大部分文件简洁清晰 |
| 架构 | 3 | generateRandomNickname 是无价值透传包装；PALETTE_COLORS/END_REASON 与后端手动重复 |
| 性能 | 4 | 节流/防抖合理 |
| 可维护性 | 3 | RESET_ROUND 手动枚举字段易遗漏；visual_helpers 全局可变状态 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-R-100 | 正确性 | REQUIRED | `drawDangerVignettes` 在幽灵临近时设置 `globalAlpha = 0.85 + 0.15 * sin(...)`（行 80-82），但随后无任何绘制操作即 `getCtx().globalAlpha = 1`（行 84）重置。幽灵危险视觉效果为死代码/未完成实现 | visual_helpers.ts:74-84 | 补全幽灵危险 vignette 绘制（如 fillRect 半透明覆盖），或删除无效的 alpha 设置 |
| v2-O-85 | 可维护性 | OPTIONAL | `RESET_ROUND` 手动枚举需重置的字段（ripples/explosionEffect/myCooldownEnd/...）。新增 round 级字段若遗漏此处将静默携带旧值 | reducer.ts:70-86 | 改为基于 createDefaultState() 选择性覆盖 round 级字段，或用字段级重置清单加 lint 校验 |
| v2-O-86 | 架构 | OPTIONAL | `generateRandomNickname`（ui_utils.ts:53-55）是 `pickRandomNickname`（ui_elements.ts:26-31）的纯透传包装，增加间接层无任何价值 | ui_utils.ts:53-55 | 删除包装，调用方直接 import pickRandomNickname；或合并到 ui_elements |
| v2-F-65 | 架构 | FYI | `local_constants.ts` 中 `PALETTE_COLORS`（10 色）和 `END_REASON`（4 值）与后端 `protocol/constants.go` 的 `PaletteColors`/`EndReason*` 手动重复，无 codegen 同步（PHYSICS/COOLDOWN 已有 codegen 但调色板/结束原因没有）| local_constants.ts:1-26 | 可扩展 gen-frontend-constants 也生成 PALETTE_COLORS 和 END_REASON，消除漂移风险 |
| v2-F-66 | 可读性 | FYI | `mutateFloatingTexts` 包装器仅调用 `mutate(floatingTexts)`，是无意义间接层 | visual_helpers.ts:24-26 | 直接内联操作 floatingTexts 数组 |

### 整体健康度: 🟡 3.4/5
多数文件简洁可用，但存在死代码、透传包装和手动常量重复。

---

## 资产 A-046: shared/ui（audio.ts, toast.ts）
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | audio.ts 将音频创建绑定到 prefers-reduced-motion，语义错误 |
| 安全 | 5 | toast 用 textContent 无 XSS；无敏感信息 |
| 可维护性 | 4 | 两个文件简洁，测试覆盖充分 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-R-101 | 正确性 | REQUIRED | `audio.ts` 的 `ctx()` 和 `vibrate()` 均以 `prefers-reduced-motion: reduce` 为由返回 null/跳过。减少动效（动画）与音频/振动是两个无关的辅助功能维度：用户希望减少动画但仍想要音效时，此处静默禁用全部音频。vibrate 用 reduced-motion 尚可理解（振动算动效），但音频不应受此约束 | audio.ts:5, 36 | 音频创建不应绑定 reduced-motion；若需音频开关应提供独立设置或用 `prefers-reduced-data`。vibrate 绑定 reduced-motion 可保留 |
| v2-O-87 | 可维护性 | OPTIONAL | `toast.ts` 的 setTimeout 闭包捕获 `el` 引用；若 toast 元素被外部移除 DOM，定时器到期前持有分离节点引用。影响极小 | toast.ts:14-17 | 可在回调内重新 getElementById 而非依赖闭包捕获 |
| v2-F-67 | 可维护性 | FYI | `audio.ts` 模块级 `audioCtx` 单例从不关闭；测试中 `delete window.AudioContext` 但已创建的 `audioCtx` 跨测试持久，可能影响测试隔离 | audio.ts:1 | 测试可增加 afterEach 重置模块状态，或导出 reset 函数 |

### 整体健康度: 🟡 4.0/5
安全实践良好，但音频与动效偏好绑定的语义错误需修正。

---

## 资产 A-047: shared/data（best_score_cookie.ts, tutorial_cookie.ts）
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | cookie 解析健壮，API 失败有 cookie 回退 |
| 安全 | 4 | samesite=lax + 条件 Secure 标志 |
| 可维护性 | 4 | 两文件 cookie 解析逻辑重复 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-O-88 | 可维护性 | OPTIONAL | `best_score_cookie.ts` 和 `tutorial_cookie.ts` 各自手动拼接/正则解析 `document.cookie` 字符串，逻辑重复且易出解析 bug | best_score_cookie.ts:5,13; tutorial_cookie.ts:5,10 | 提取共享 cookie 读写工具（如 `shared/data/cookie_utils.ts`），两文件复用 |
| v2-F-68 | 性能 | FYI | `fetchUserBestScore` 和 `shouldShowTutorial` 各自独立调用 `/api/v1/user/stats`；若两个功能在同一会话启动时都被调用，产生两次相同请求 | best_score_cookie.ts:18; tutorial_cookie.ts:16 | 可在 shared 层缓存 user/stats 响应或合并调用 |

### 整体健康度: 🟢 4.3/5
安全实践扎实、回退合理，仅 cookie 解析重复值得收敛。

---

## 资产 A-048: shared/assets（nickname_pools_gen.ts）
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 5 | 代码生成，数据静态正确 |
| 安全 | 5 | 纯静态字符串数组，无注入面 |
| 可维护性 | 4 | 形容词/类别拆分未在文件内说明 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-F-69 | 可维护性 | FYI | `NICKNAME_ADJECTIVES` 被导出且被 `ui_elements.ts` 用作前缀，但未纳入 `NICKNAME_CATEGORIES` 数组（后者仅含 ANIMALS/JOBS/NATURE/SCIFI 作名词池）。文件内无注释说明"形容词+名词"的组合规则，读者看到 CATEGORIES 可能误以为它是昵称池全貌 | nickname_pools_gen.ts:3-12, 42-46 | 在 CATEGORIES 定义上方加注释说明：昵称由 NICKNAME_ADJECTIVES 前缀 + CATEGORIES 中随机名词组合而成 |
| v2-F-70 | 可维护性 | FYI | 文件头声明 "generated by scripts/codegen/generate_nicknames.go"，已验证该路径存在且文件标注 DO NOT EDIT。代码生成链一致 | nickname_pools_gen.ts:1 | 无需操作，记录一致性 |

### 整体健康度: 🟢 4.7/5
静态生成数据，质量高，仅文档可微调。

---

## 资产 A-049: test_fixtures（snapshot.ts）
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | 44 字节 buffer 布局用魔法偏移，无字段映射注释 |
| 可读性 | 2 | 魔法数 `42` 无解释，offset 递增无语义 |
| 可维护性 | 3 | 协议布局变更时静默产出畸形数据 |
| 可观测性 | 3 | 无对实际解码器的布局一致性校验 |
| 文档一致性 | 2 | 无字段映射文档 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-R-102 | 正确性 | REQUIRED | `buildMinimalSnapshot` 用硬编码 44 字节 buffer + 顺序 `o` 偏移构建快照，无任何注释将偏移映射到 SNAPSHOT 协议字段（timestamp/score/phase/balloon/bird/ghost/players/wind）。若二进制快照布局变更（增删字段、改顺序），此 fixture 静默产出畸形数据，相关 WS handler 测试会通过或失败得不可预测 | snapshot.ts:4-24 | 为每行 setXxx 添加字段名注释（如 `// timestamp`）；并增加一个断言 fixture 能被 `decodeSnapshot` 正确解析的回归测试 |
| v2-O-89 | 可读性 | OPTIONAL | 行 10 `dv.setUint32(o, 42, true)` 中的 `42` 是魔法数（实为 score 默认值，与 snapshot_decode.test.ts 的 `score = 42` 默认一致），但无命名/注释 | snapshot.ts:10 | 提取为具名参数 `score = 42` 或加注释 `// score` |
| v2-F-71 | 文档一致性 | FYI | `test_fixtures/` 目录仅含 `snapshot.ts` 单文件，目录名暗示更广的 fixture 集合。当前规模下可考虑扁平化或补充更多 fixture | (目录结构) | 随测试增长自然演进，暂无需操作 |

### 整体健康度: 🟡 2.6/5
测试基础设施缺乏文档与布局校验，协议变更时脆弱性高。

---

## 汇总统计

| 资产 | 整体评分 | CRITICAL | REQUIRED | OPTIONAL | FYI |
|------|---------|----------|----------|----------|-----|
| A-029 cmd/seed | 🟡 3.0/5 | 0 | 2 | 1 | 0 |
| A-030 cmd/backfill-emails | 🟡 3.8/5 | 0 | 1 | 2 | 0 |
| A-031 cmd/migrate-passwords | 🟢 4.0/5 | 0 | 0 | 1 | 2 |
| A-032 cmd/gen-frontend-constants | 🟡 2.7/5 | 0 | 2 | 1 | 0 |
| A-043 game/其他 | 🟡 3.4/5 | 0 | 1 | 2 | 2 |
| A-046 shared/ui | 🟡 4.0/5 | 0 | 1 | 1 | 1 |
| A-047 shared/data | 🟢 4.3/5 | 0 | 0 | 1 | 1 |
| A-048 shared/assets | 🟢 4.7/5 | 0 | 0 | 0 | 2 |
| A-049 test_fixtures | 🟡 2.6/5 | 0 | 1 | 1 | 1 |
| **合计** | — | **0** | **8** | **10** | **9** |

**发现 ID 分配**：v2-R-95~v2-R-102（8 个 REQUIRED），v2-O-80~v2-O-89（10 个 OPTIONAL），v2-F-63~v2-F-71（9 个 FYI）。

**最需优先处理**：
1. v2-R-95（seed 安全守卫可绕过）— 生产数据风险
2. v2-R-100（visual_helpers 幽灵 vignette 死代码）— 功能缺失
3. v2-R-98/v2-R-99（gen-frontend-constants 硬编码+无测试）— 代码生成一致性
4. v2-R-101（audio 绑定错误的媒体查询）— 辅助功能语义错误
5. v2-R-102（snapshot fixture 魔法偏移无文档）— 测试可靠性
