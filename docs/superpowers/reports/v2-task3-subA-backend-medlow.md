# 代码库自检 v2 — Task3 子代理 A：后端 Medium/Low 资产审查报告

**审查范围**：11 个后端 Medium/Low 资产
**审查轴**：正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 可观测性（A-026/A-027 用 5 轴：正确性 + 可读性 + 可维护性 + 可观测性 + 文档一致性）
**重点轴**：可维护性、可观测性（新轴优先）
**审查日期**：2026-07-08
**发现 ID 范围**：CRITICAL v2-C-16+，REQUIRED v2-R-83+，OPTIONAL v2-O-69+，FYI v2-F-53+

---

## 资产 A-001: apierror
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 5 | RFC 7807 实现完整，Content-Type 正确 |
| 可读性 | 5 | 命名清晰，每个构造器单一职责 |
| 可维护性 | 5 | 简洁封装，扩展只需添加构造器 |
| 安全 | 4 | Write 忽略编码错误（合理但未记录）|
| 可观测性 | 3 | 无 error 日志/metrics 钩子（轻量资产合理）|

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-F-53 | 可维护性 | FYI | `Write` 中 `json.NewEncoder(w).Encode(e)` 错误被忽略（`_ =`），若连接中断或客户端断开无法观测 | apierror.go:80 | 可记录到 slogctx 但保持轻量；当前行为合理 |
| v2-F-54 | 可维护性 | FYI | `Type` 字段使用外部 URL `https://httpstatuses.com/{status}`，该域名可能失效，导致 type URI 不可解析 | apierror.go:28 | 考虑改用内部文档 URI 或 `about:blank` 默认值 |

### 整体健康度: 🟢 4.6/5
简洁、单一职责、测试充分。RFC 7807 实现质量高。

---

## 资产 A-010: health
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | 探针逻辑正确，degraded/not-ready 区分清晰 |
| 可读性 | 5 | 注释详尽，企业级 rationale 充分 |
| 可维护性 | 4 | `poolPingForTest` 全局可变变量 |
| 安全 | 5 | 探针无敏感信息泄露 |
| 性能 | 4 | defer cancel 在 if 块中的资源释放时机 |
| 可观测性 | 3 | 探针结果未输出到 metrics/log（合理，避免循环）|

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-F-55 | 可维护性 | FYI | `poolPingForTest` 是包级全局可变变量，非并发安全；若与 `t.Parallel()` 测试共用可能竞态 | health.go:23 | 改为 Checker 字段或依赖注入；当前测试未并行故无实际风险 |
| v2-F-56 | 性能 | FYI | PG 和 Redis 检查分别 `defer cancel()`，两个 cancel 都在函数返回时执行；PG 的 context 在 Redis 检查期间虽已超时但未显式释放 | health.go:70-71, 88-89 | 可在每个 if 块内显式调用 cancel()；资源影响极小 |
| v2-O-69 | 可维护性 | OPTIONAL | `LiveHandler` 总是返回 200，缺少进程级健康信号（如 goroutine 泄漏检测、死锁检测）| health.go:49-52 | 企业级可考虑增加 process-level 健康信号；当前 liveness 语义符合 K8s 惯例 |

### 整体健康度: 🟢 4.3/5
探针设计合理，degraded/not-ready 区分体现企业级思考。测试覆盖完整（PG/Redis/WS/miniredis）。

---

## 资产 A-011: idgen
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | UUID v4 格式正确，测试覆盖优秀 |
| 可读性 | 5 | 单一函数，注释清晰 |
| 可维护性 | 4 | 注释提到"便于未来切换 google/uuid"，技术债标记 |
| 安全 | 4 | 使用 crypto/rand，但忽略错误 |
| 性能 | 5 | fmt.Sprintf 性能充足 |
| 可观测性 | 3 | 无 metrics（轻量资产合理）|

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-F-57 | 安全 | FYI | `rand.Read(b)` 错误被忽略（`_, _ =`）；若 crypto/rand 失败（极罕见，如熵耗尽），b 为零值，生成 `00000000-0000-4000-8000-000000000000` | uuid.go:14 | 可 panic 或 fallback；测试 `TestUUID_NotAllZeros` 已部分覆盖此场景 |
| v2-F-58 | 可维护性 | FYI | 注释提到"集中维护便于未来切换到标准库（如 google/uuid）"，是技术债标记但未在 docs 中追踪 | uuid.go:10-11 | 可在技术债看板记录此切换意图 |

### 整体健康度: 🟢 4.4/5
实现简洁正确，测试质量极高（格式/版本/变体/唯一性/并发/分布/边界）。crypto/rand 错误处理是唯一改进点。

---

## 资产 A-012: metrics
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 5 | 指标定义规范，SLO buckets 合理 |
| 可读性 | 5 | 每个指标有 Help 文本和企业级 rationale |
| 可维护性 | 4 | init() 忽略注册错误（已注释）|
| 安全 | 4 | /metrics 暴露内部状态（注释提示网络策略限制）|
| 性能 | 5 | promauto 自动注册，无运行时开销 |
| 可观测性 | 5 | Golden Signals + 业务 SLI 覆盖全面 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-O-70 | 可维护性 | OPTIONAL | `RoomCreationDuration` 定义为 `HistogramVec` 但标签维度为 `[]string{}`（空标签），等同于无标签 Histogram，设计冗余 | metrics.go:171-175 | 改为 `promauto.NewHistogram` 或补充有意义的标签（如 `mode`/`source`）|
| v2-F-59 | 可维护性 | FYI | `init()` 中 `prometheus.Register` 错误被 `_ = err` 忽略，注释说明"already registered in tests"；生产环境若注册失败将无 Go runtime 指标 | metrics.go:21-26 | 可改为 `prometheus.MustCompile` 或在启动时校验；当前测试场景下合理 |
| v2-F-60 | 可观测性 | FYI | `WSMessagesDroppedTotal` 用 `room_code` 作为标签，高频房间会导致标签基数增长；长生命周期下可能推高 Prometheus 内存 | metrics.go:107-110 | 考虑用房间阶段或固定桶替代；当前规模下可接受 |

### 整体健康度: 🟢 4.6/5
可观测性覆盖优秀（HTTP/DB/Redis/WS/熔断器/Outbox/Room lock），SLO buckets 体现企业级思考。指标命名遵循 Prometheus 规范。

---

## 资产 A-014: nicknames
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 3 | **`len()` 字节长度 vs rune 长度 bug** |
| 可读性 | 4 | 逻辑清晰，fallback 链明确 |
| 可维护性 | 4 | `randIntFn` 可注入，测试友好 |
| 安全 | 4 | crypto/rand 使用正确 |
| 性能 | 4 | 重试 10 次合理 |
| 可观测性 | 2 | 无 metrics 记录 fallback 触发频率 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-R-83 | 正确性 | REQUIRED | **`GenerateRandom` 后缀分支用 `len(candidate) <= maxNicknameLength`（字节长度）判断，而 `maxNicknameLength=12`。中文名如 `"快乐的气球"` 已 15 字节，导致后缀 `"快乐的气球#2"`（17 字节）总被拒绝，直接跳到 `PlayerXXXX` fallback。**`validate.Nickname` 用 rune 长度（`utf8.RuneCountInString`），两包语义不一致。测试 `TestGenerateRandom_ReturnsValidSuffix` 因 `len(want) > maxNicknameLength` 而 `t.Skip`，掩盖了此 bug | generator.go:43 | 改为 `utf8.RuneCountInString(candidate) <= maxNicknameLength`；移除测试中的 Skip |
| v2-O-71 | 正确性 | OPTIONAL | `PlayerXXXX` fallback 不检查 `usedNames`，可能返回已占用的名字（概率 1/10000，且仅在 10 次组合 + 98 后缀全冲突后触发）| generator.go:48 | 可循环重试或检查 usedNames；当前概率极低 |
| v2-F-61 | 可观测性 | FYI | 无 metrics 记录 fallback 触发（`#N` 后缀 / `PlayerXXXX`）频率，无法监控昵称生成压力 | generator.go:48 | 可暴露 counter `nickname_fallback_total{type="suffix|player"}` |
| v2-F-62 | 可维护性 | FYI | `randomIndex` 在 `randIntFn` 失败时返回 0，导致所有选择指向第一个元素，可预测 | generator.go:17-20 | 失败时可 panic 或记录；当前兜底避免崩溃 |

### 整体健康度: 🟡 3.8/5
**核心 bug（v2-R-83）**：字节 vs rune 长度不一致导致中文昵称后缀逻辑失效。测试存在 Skip 掩盖问题。`randIntFn` 注入设计优秀。

---

## 资产 A-018: requestctx
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 5 | XFF 解析逻辑正确，trusted-proxy 语义清晰 |
| 可读性 | 5 | 简洁，函数职责单一 |
| 可维护性 | 5 | context key 模式规范 |
| 安全 | 5 | 仅在 trusted proxy 时信任 XFF，防 spoofing |
| 性能 | 5 | 字符串操作轻量 |
| 可观测性 | 3 | 无 IP 提取日志（合理）|

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-F-63 | 安全 | FYI | `ExtractClientIP` 不验证 XFF 首 IP 格式（如 IPv4/IPv6 合法性），直接返回；若 XFF 被注入畸形值，下游消费方需自行处理 | proxy.go:38 | 当前设计合理（轻量），由调用方负责格式校验 |

### 整体健康度: 🟢 4.8/5
安全设计优秀（trusted-proxy 显式标记），测试覆盖完整（含畸形 RemoteAddr、空 XFF、blank XFF 等边界）。

---

## 资产 A-021: slogctx
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | 实现正确，但注释与实现不一致 |
| 可读性 | 5 | 简洁清晰 |
| 可维护性 | 3 | **导出类型但用未导出变量，注释误导** |
| 安全 | 5 | 无敏感信息处理 |
| 性能 | 5 | context.Value 单次查找 |
| 可观测性 | 4 | 上下文日志传播正确 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-R-84 | 可维护性 | REQUIRED | **`CtxKey` 类型已导出，注释称"Exported so multiple packages (middleware, auth) can inject/retrieve the contextual logger without circular imports"，但实际存储用未导出的 `var ctxKey = CtxKey{}`。其他包无法用 `context.WithValue(ctx, slogctx.CtxKey{}, logger)` 注入（键不匹配），只能通过 `WithLogger` 函数。注释与实现不一致，易误导调用方** | slogctx.go:9-14 | 要么移除 CtxKey 导出（仅暴露 WithLogger/LoggerFromContext），要么改用 `CtxKey{}` 作为键并更新注释 |
| v2-F-64 | 可观测性 | FYI | `LoggerFromContext` fallback 到 `slog.Default()`，若调用方未注入 logger，将丢失 request-scoped 字段（如 trace_id、request_id）| slogctx.go:22 | 可记录 warning 或在中间件层强制注入；当前兜底合理 |

### 整体健康度: 🟡 4.0/5
实现简洁，但注释误导（v2-R-84）可能导致跨包集成错误。测试覆盖类型安全（wrong-type fallback）。

---

## 资产 A-023: telemetry
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 5 | OTel 初始化逻辑正确，sampling 合理 |
| 可读性 | 5 | 企业级 rationale 详尽 |
| 可维护性 | 4 | 工厂函数可注入，测试友好 |
| 安全 | 4 | OTLP insecure 默认 true（dev-friendly 但需生产校验）|
| 性能 | 4 | ParentBased(TraceIDRatioBased) 避免过载 |
| 可观测性 | 5 | trace context + baggage 传播配置完整 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-F-65 | 可维护性 | FYI | `tracer` 全局变量在 `init()` 中通过 `otel.Tracer(...)` 创建（基于 noop provider），之后 `InitTracer` 调用 `otel.SetTracerProvider` 更换 provider。`tracer` 仍有效依赖 OTel SDK 的 delegate 代理行为，未在注释中说明 | telemetry.go:28, 36-38 | 可在 `Tracer()` 中改为 `return otel.Tracer("...")` 动态获取，或注释说明 delegate 行为 |
| v2-O-72 | 可维护性 | OPTIONAL | `TestTracer_ReturnsConsistentInstance` 无实际断言（`_ = tr1; _ = tr2`），测试形同虚设 | telemetry_test.go:79-87 | 删除或补充断言（如 `reflect.DeepEqual(tr1, tr2)`）|
| v2-F-66 | 安全 | FYI | `isOTLPInsecure()` 默认返回 true（dev-friendly），生产环境若忘记设置 `OTLP_INSECURE=false` 将用明文 gRPC 传输 trace 数据 | telemetry.go:113-116 | 可根据 `ENV=production` 自动强制 TLS，或在启动日志中告警 |

### 整体健康度: 🟢 4.4/5
OTel 集成质量高（采样策略、工厂注入、并发测试、错误路径测试）。insecure 默认值需生产注意。

---

## 资产 A-024: validate
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 4 | 清理逻辑正确，但与 nicknames 包长度语义不一致 |
| 可读性 | 5 | 正则命名清晰，函数职责单一 |
| 可维护性 | 4 | 正则预编译，adapter 模式清晰 |
| 安全 | 5 | XSS/control char/zero-width 清理全面 |
| 性能 | 5 | 正则预编译，单次清理 |
| 可观测性 | 3 | 无 rejected 计数（合理）|

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-F-67 | 安全 | FYI | `nicknameInputRejectedRegex`（`[\x00-\x1f<>"'&]`）与 `htmlCharsRegex`（`[<>"'\x60&]`）不一致：后者包含反引号 `` ` ``，前者不包含。`NicknameInputRejected` 不检测反引号，但 `Nickname` 会移除反引号。两个函数语义不同（拒绝 vs 清理）但易混淆 | nickname.go:14, 25 | 可统一字符集或在注释中明确差异；当前行为有意（预检宽松，后处理严格）|
| v2-F-68 | 可维护性 | FYI | `Nickname` 先移除 control chars 再 `TrimSpace`，最后才移除 HTML chars；顺序导致 `<a> <b>` 先变 `a b` 再变 `ab`（中间态含空格）。最终结果正确，但顺序对边界 case 有微妙影响 | nickname.go:34-38 | 当前实现可接受；可添加顺序注释 |

### 整体健康度: 🟢 4.4/5
安全清理全面（control/zero-width/HTML/whitespace），测试覆盖 XSS 攻击向量和 CJK truncation。adapter 模式便于替换。

---

## 资产 A-026: testsecrets
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 5 | 密钥格式有效，明确标注"not production" |
| 可读性 | 5 | 注释清晰 |
| 可维护性 | 3 | 硬编码密钥，无 build 约束 |
| 可观测性 | 3 | N/A（测试辅助）|
| 文档一致性 | 4 | 包注释与常量注释一致 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-O-73 | 可维护性 | OPTIONAL | 包含硬编码 EC 私钥，即使标注"test only"，仍可能被误用到生产。无 `//go:build` 约束防止生产引入 | secrets.go:5-9 | 添加 build tag（如 `//go:build test`）或移至 `testdata/` 目录；当前依赖团队纪律 |
| v2-F-69 | 安全 | FYI | `TestEncryptionKeyHex` 为固定值 `cccc...`，若测试覆盖不足，可能掩盖弱密钥检测 | secrets.go:8 | 可接受（测试用途）；确保生产密钥生成路径有独立测试 |

### 整体健康度: 🟢 4.2/5
明确标注非生产用途，密钥格式有效。建议加 build tag 防误用。

---

## 资产 A-027: testutil
### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 正确性 | 5 | testcontainers 启动逻辑正确 |
| 可读性 | 4 | 函数命名清晰，但重复代码降低可读性 |
| 可维护性 | 3 | 多处 testcontainers 启动逻辑重复 |
| 可观测性 | 3 | 跳过原因用 t.Skipf 记录（合理）|
| 文档一致性 | 4 | 函数注释完整 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
|---------|-----|---------|------|------|------|
| v2-O-74 | 可维护性 | OPTIONAL | `SetupRedisClient`（miniredis.go）与 `SetupRedisStore`（redis.go）的 testcontainers Redis 启动逻辑高度重复（容器镜像、wait strategy、endpoint 获取、ping）| miniredis.go:17-45, redis.go:15-49 | 抽取内部 `startRedisContainer(t) (*tcredis.RedisContainer, context.Context)` 共享 |
| v2-O-75 | 可维护性 | OPTIONAL | `SetupPostgresConn`/`SetupPostgresPool`/`SetupPostgresStore` 三处重复 postgres testcontainers 启动（镜像、用户名密码、wait strategy）| postgres.go:76-106, 117-149, 167-203 | 抽取 `startPostgresContainer(t) (*postgres.PostgresContainer, context.Context)` 共享 |
| v2-F-70 | 可读性 | FYI | `Contains` 函数仅包装 `strings.Contains`，无附加价值；可能是历史遗留 | assert_helpers.go:7-9 | 可移除并直接用 `strings.Contains`；保留若作为断言语义入口 |
| v2-F-71 | 可维护性 | FYI | `SetupPostgresPoolMigrated` 硬编码跳过 `"000009"` 迁移，注释说明"database roles require superuser"，但跳过逻辑分散在调用方 | postgres.go:153-164 | 可在迁移文件命名或 metadata 中标记 superuser-only，集中管理跳过规则 |

### 整体健康度: 🟡 4.0/5
功能完整（testcontainers PG/Redis + miniredis + 迁移），但重复代码较多（v2-O-74, v2-O-75）增加维护成本。

---

## 汇总统计

### 发现数量统计
| 严重级别 | 数量 | 发现 ID 范围 |
|---------|------|-------------|
| CRITICAL | 0 | — |
| REQUIRED | 2 | v2-R-83, v2-R-84 |
| OPTIONAL | 6 | v2-O-69 ~ v2-O-75 |
| FYI | 19 | v2-F-53 ~ v2-F-71 |
| **合计** | **27** | |

### 各资产 v2 整体评分
| 资产 ID | 名称 | 评分 | 健康度 | 关键问题 |
|---------|------|------|--------|---------|
| A-001 | apierror | 4.6 | 🟢 | 无 REQUIRED |
| A-010 | health | 4.3 | 🟢 | 无 REQUIRED |
| A-011 | idgen | 4.4 | 🟢 | 无 REQUIRED |
| A-012 | metrics | 4.6 | 🟢 | 无 REQUIRED |
| A-014 | nicknames | 3.8 | 🟡 | **v2-R-83 字节 vs rune 长度 bug** |
| A-018 | requestctx | 4.8 | 🟢 | 无 REQUIRED |
| A-021 | slogctx | 4.0 | 🟡 | **v2-R-84 注释与实现不一致** |
| A-023 | telemetry | 4.4 | 🟢 | 无 REQUIRED |
| A-024 | validate | 4.4 | 🟢 | 无 REQUIRED |
| A-026 | testsecrets | 4.2 | 🟢 | 无 REQUIRED |
| A-027 | testutil | 4.0 | 🟡 | 无 REQUIRED（重复代码 OPTIONAL）|

### 重点 REQUIRED 发现
1. **v2-R-83（A-014 nicknames）**：`GenerateRandom` 用 `len()`（字节）而非 `utf8.RuneCountInString()` 判断长度，导致中文昵称后缀分支失效，直接 fallback 到 `PlayerXXXX`。测试 `TestGenerateRandom_ReturnsValidSuffix` 因 `t.Skip` 掩盖了此 bug。**与 `validate.Nickname` 的 rune 长度语义不一致**。
2. **v2-R-84（A-021 slogctx）**：`CtxKey` 类型导出但实际用未导出的 `ctxKey` 变量存储，注释"Exported so multiple packages can inject/retrieve"具有误导性，可能导致跨包键不匹配。

### 整体评价
11 个 Medium/Low 资产整体质量良好（平均 4.3/5）。可观测性覆盖优秀（metrics 资产 SLI 完整，telemetry OTel 集成规范）。可维护性问题集中在：注释与实现不一致（slogctx）、字节/rune 长度语义不一致（nicknames vs validate）、测试辅助代码重复（testutil）。无 CRITICAL 级别问题。
