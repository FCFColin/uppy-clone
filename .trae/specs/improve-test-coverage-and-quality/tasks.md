# Tasks

## P0：修复阻塞性构建错误（阻塞所有后续测试）

- [x] Task 1: 修复 `internal/handler/auth.go` 缺失导入
  - [x] 在 import 块中添加 `"context"` 和 `"github.com/uppy-clone/backend/internal/domain"`
  - [x] 验证 `go build ./internal/handler/` 通过
  - [x] 验证 `go build ./...` 全量通过
  - [x] 验证 `go vet ./...` 零警告

## P1：补齐零覆盖模块单元测试（可并行，按包分组）

- [ ] Task 2: 为 `internal/domain/room_code.go` 编写测试
  - [ ] 测试合法 RoomCode 构造（[A-Z2-9] 字符集）
  - [ ] 测试非法长度（0、4、6、超长）
  - [ ] 测试非法字符（0、1、I、O、小写、特殊字符、Unicode）
  - [ ] 测试 String() 方法
  - [ ] 覆盖率 >90%

- [ ] Task 3: 为 `internal/domain/nickname.go` 编写测试
  - [ ] 测试昵称生成与校验逻辑
  - [ ] 测试边界条件（空、最大长度、Unicode）
  - [ ] 覆盖率 >90%

- [ ] Task 4: 为 `internal/domain/domain.go` 编写测试
  - [ ] 测试领域模型方法（如有）
  - [ ] 测试事件构造方法
  - [ ] 覆盖率 >60%

- [ ] Task 5: 为 `internal/domain/events.go` 编写测试
  - [ ] 测试事件类型构造
  - [ ] 测试事件序列化
  - [ ] 覆盖率 >60%

- [ ] Task 6: 为 `internal/validate/nickname.go` 编写测试（重要文件，>90%）
  - [ ] 测试控制字符过滤（U+0000-U+001F、U+007F-U+009F）
  - [ ] 测试零宽字符过滤（U+200B-U+200F、U+FEFF）
  - [ ] 测试 HTML 特殊字符过滤（<、>、"、'、`、&）— XSS 防护
  - [ ] 测试长度截断（>20 字符，含多字节 Unicode）
  - [ ] 测试空白折叠与 trim
  - [ ] 测试空字符串输入
  - [ ] 测试纯恶意输入（`<script>alert(1)</script>`）
  - [ ] 覆盖率 >90%

- [ ] Task 7: 为 `internal/auth/revoke.go` 编写测试（重要文件，>90%）
  - [ ] 测试 RevokeAllTokens 撤销 session cookie
  - [ ] 测试 RevokeAllTokens 撤销 quickplay cookie
  - [ ] 测试无 cookie 时不报错
  - [ ] 测试 cookie 存在但 token 无效时不报错
  - [ ] 测试 redis 为 nil 时不 panic
  - [ ] 测试 jti 为空时不调用 RevokeJWT
  - [ ] 覆盖率 >90%

- [ ] Task 8: 为 `internal/handler/degradation.go` 编写测试
  - [ ] 测试 WriteDegradedJSON 写入正确 JSON 结构
  - [ ] 测试 Content-Type 头设置
  - [ ] 测试状态码传递
  - [ ] 测试 message 字段 omitempty 行为
  - [ ] 覆盖率 >90%

- [ ] Task 9: 为 `internal/worker/game_result_worker.go` 编写测试（重要文件，>90%）
  - [ ] 测试 processBatch 正常批处理
  - [ ] 测试 processBatch 畸形 payload（非 string、非法 JSON）
  - [ ] 测试 processBatch 数据库失败时回滚
  - [ ] 测试 processBatch 幂等性（重复消息不重复插入）
  - [ ] 测试 Start 上下文取消时 flush 剩余批次
  - [ ] 测试 XAck 在 commit 后执行
  - [ ] 覆盖率 >90%

- [ ] Task 10: 为 `internal/service/auth_service.go` 编写测试
  - [ ] 测试 DeleteUserData 撤销令牌 + 匿名化
  - [ ] 测试 DeleteUserData redis 为 nil 时仍匿名化
  - [ ] 测试 DeleteUserData 数据库失败时返回错误
  - [ ] 测试 ExportUserData 返回用户数据
  - [ ] 覆盖率 >90%

- [ ] Task 11: 为 `internal/service/admin_service.go` 编写测试
  - [ ] 测试管理服务方法
  - [ ] 覆盖率 >60%

- [ ] Task 12: 为 `internal/service/lobby_service.go` 编写测试
  - [ ] 测试大厅服务方法
  - [ ] 覆盖率 >60%

- [ ] Task 13: 为 `internal/service/command_service.go` 编写测试
  - [ ] 测试命令服务方法
  - [ ] 覆盖率 >60%

- [ ] Task 14: 为 `internal/service/query_service.go` 编写测试
  - [ ] 测试查询服务方法
  - [ ] 覆盖率 >60%

- [ ] Task 15: 为 `internal/game/repository.go` 编写测试
  - [ ] 测试仓储接口实现
  - [ ] 覆盖率 >60%

- [ ] Task 16: 为 `internal/idgen/uuid.go` 编写测试
  - [ ] 测试 UUID 生成唯一性
  - [ ] 测试 UUID 格式合规
  - [ ] 覆盖率 >90%

- [ ] Task 17: 为 `internal/slogctx/slogctx.go` 编写测试
  - [ ] 测试上下文日志字段存取
  - [ ] 覆盖率 >60%

- [ ] Task 18: 为 `internal/metrics/metrics.go` 编写测试
  - [ ] 测试指标注册与递增
  - [ ] 覆盖率 >60%

- [ ] Task 19: 为 `internal/telemetry/telemetry.go` 编写测试
  - [ ] 测试遥测初始化（使用 noop exporter）
  - [ ] 覆盖率 >60%

## P2：增强现有测试的对抗性（可并行，按模块分组）

- [ ] Task 20: 增强 `internal/auth/jwt_test.go` 对抗性
  - [ ] 测试 alg=none 攻击
  - [ ] 测试 HS256 vs RS256 混淆攻击
  - [ ] 测试空 subject/nickname
  - [ ] 测试超长 subject（DoS）
  - [ ] 测试伪造 jti
  - [ ] 测试 token 重放（同 jti 多次验证）

- [ ] Task 21: 增强 `internal/auth/magiclink_test.go` 对抗性
  - [ ] 测试过期 magic link
  - [ ] 测试已使用 magic link（重放）
  - [ ] 测试篡改 token
  - [ ] 测试跨用户 token 使用

- [ ] Task 22: 增强 `internal/auth/refresh_test.go` 对抗性
  - [ ] 测试 refresh token 轮换后旧 token 失效
  - [ ] 测试并发 refresh 竞态（同一 token 多次使用）
  - [ ] 测试已撤销 token 使用
  - [ ] 测试跨用户 token 使用

- [ ] Task 23: 增强 `internal/auth/secure_test.go` 对抗性
  - [ ] 测试密码强度校验边界
  - [ ] 测试常见弱密码
  - [ ] 测试超长密码（DoS）

- [ ] Task 24: 增强 `internal/crypto/aes_test.go` 对抗性
  - [ ] 测试空明文
  - [ ] 测试超长明文
  - [ ] 测试错误密钥解密
  - [ ] 测试篡改密文（认证失败）
  - [ ] 测试 nonce 重用风险

- [ ] Task 25: 增强 `internal/middleware/cors_test.go` 对抗性
  - [ ] 测试 Origin 伪造（null origin、file:// origin）
  - [ ] 测试子域名绕过尝试（evil.example.com）
  - [ ] 测试通配符 Origin 处理
  - [ ] 测试 CORS 头注入（CRLF 注入）

- [ ] Task 26: 增强 `internal/middleware/security_test.go` 对抗性
  - [ ] 测试每个安全头的存在性与正确值
  - [ ] 测试头注入攻击
  - [ ] 测试缺失头的场景

- [ ] Task 27: 增强 `internal/middleware/ratelimit_test.go` 对抗性
  - [ ] 测试并发请求竞态
  - [ ] 测试 IP 伪造（X-Forwarded-For）
  - [ ] 测试窗口边界精确性
  - [ ] 测试限流恢复

- [ ] Task 28: 增强 `internal/middleware/idempotency_test.go` 对抗性
  - [ ] 测试重复请求幂等性
  - [ ] 测试并发相同 key 竞态
  - [ ] 测试 key 过期后重放
  - [ ] 测试跨用户 key 使用

- [ ] Task 29: 增强 `internal/protocol/encode_decode_test.go` 对抗性
  - [ ] 测试畸形输入解码（截断、非法字段）
  - [ ] 测试超大消息（DoS）
  - [ ] 测试 round-trip 一致性（含边界值）
  - [ ] 测试未知消息类型处理

- [ ] Task 30: 增强 `internal/game/physics_test.go` 对抗性
  - [ ] 测试边界坐标（NaN、Inf、负值）
  - [ ] 测试超速碰撞
  - [ ] 测试并发物理更新竞态
  - [ ] 测试零质量/负质量对象

- [ ] Task 31: 增强 `internal/game/state_test.go` 对抗性
  - [ ] 测试状态序列化/反序列化边界
  - [ ] 测试非法状态转移
  - [ ] 测试并发状态更新
  - [ ] 测试超大状态（DoS）

- [ ] Task 32: 增强 `internal/game/room_test.go` 与 `hub_test.go` 对抗性
  - [ ] 测试房间满员边界
  - [ ] 测试并发加入/离开竞态
  - [ ] 测试非法房间码
  - [ ] 测试玩家重复加入

- [ ] Task 33: 增强 `internal/handler/admin_password_test.go` 对抗性
  - [ ] 测试 bcrypt 哈希校验边界
  - [ ] 测试非 bcrypt 哈希拒绝
  - [ ] 测试时序攻击防护
  - [ ] 测试空密码/超长密码

- [ ] Task 34: 增强 `internal/handler/auth_test.go` 对抗性
  - [ ] 测试畸形 JSON 请求体
  - [ ] 测试超大请求体（DoS）
  - [ ] 测试缺失字段
  - [ ] 测试 SQL 注入尝试（email 字段）

- [ ] Task 35: 增强 `internal/handler/lobby_test.go` 与 `websocket_test.go` 对抗性
  - [ ] 测试 WebSocket 握手非法 Origin
  - [ ] 测试超大消息帧
  - [ ] 测试并发连接竞态
  - [ ] 测试连接断开清理

## P3：扩展集成测试（依赖 P0）

- [ ] Task 36: 扩展 `tests/integration/postgres_test.go`
  - [ ] 测试事务回滚（失败时数据不变）
  - [ ] 测试并发写入冲突
  - [ ] 测试软删除行为
  - [ ] 测试审计日志写入
  - [ ] 测试 outbox 事件写入
  - [ ] 测试外键约束违规
  - [ ] 测试连接池耗尽场景

- [ ] Task 37: 扩展 `tests/integration/redis_test.go`
  - [ ] 测试 TTL 过期行为
  - [ ] 测试原子操作（INCR + EXPIRE）
  - [ ] 测试并发 rate limit 精确性
  - [ ] 测试 magic token 删除后不可用
  - [ ] 测试大 value 读写

- [ ] Task 38: 新增 handler 集成测试
  - [ ] 测试完整 magic link 认证流程
  - [ ] 测试 JWT 签发 → 验证 → 刷新 → 撤销流程
  - [ ] 测试 admin 登录 → 修改密码 → 重新登录流程
  - [ ] 测试 WebSocket 房间创建 → 加入 → 游戏流程

## P4：覆盖率验证与缺陷修复（依赖 P0-P3）

- [ ] Task 39: 生成覆盖率报告并验证达标
  - [ ] 执行 `go test -race -coverprofile=coverage.out ./...`
  - [ ] 验证整体覆盖率 >85%
  - [ ] 验证每文件覆盖率 >60%
  - [ ] 验证重要文件覆盖率 >90%
  - [ ] 记录未达标文件并补充测试

- [ ] Task 40: 修复测试过程中发现的真实缺陷
  - [ ] 记录每个发现的缺陷（文件、行号、描述、修复方式）
  - [ ] 修复缺陷
  - [ ] 验证修复后测试仍通过

- [ ] Task 41: 测试质量审查
  - [ ] 审查所有测试文件，移除恒真断言
  - [ ] 审查重复测试，合并为表驱动
  - [ ] 审查无用测试（_ = err、空 t.Run），移除或增强
  - [ ] 验证每个测试有明确目的注释

- [ ] Task 42: 全量回归验证
  - [ ] `go build ./...` — 零错误
  - [ ] `go vet ./...` — 零警告
  - [ ] `go test -race -count=1 ./...` — 零失败
  - [ ] `go test -race -count=1 -short ./...` — 零失败（短模式）
  - [ ] 覆盖率报告达标

# Task Dependencies
- Task 1 阻塞所有后续任务（构建修复）
- Task 2-19 可并行（按包分组，互不依赖）
- Task 20-35 可并行（按模块分组，互不依赖）
- Task 36-38 依赖 Task 1（构建修复），可与 Task 2-35 并行
- Task 39 依赖 Task 1-38 全部完成
- Task 40 可在 Task 2-38 过程中持续进行
- Task 41 依赖 Task 2-40 完成
- Task 42 依赖 Task 39-41 完成
