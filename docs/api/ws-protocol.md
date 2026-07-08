# WebSocket 二进制协议

> 企业为何需要：实时游戏 API 同样需要契约文档，前后端并行开发依赖消息格式约定。

## 连接

- **URL**: `GET /api/v1/lobby/{code}/ws`
- **认证**: HttpOnly Cookie (`session` 或 `quickplay` JWT)
- **Origin**: 必须匹配 `ALLOWED_ORIGINS`（CSWSH 防护）

## 帧格式

所有消息为 **二进制 WebSocket 帧**，编解码见 `backend/internal/protocol/`。

| 消息类型 | 方向 | 说明 |
|---------|------|------|
| 输入帧 | Client → Server | 玩家方向输入（tap/移动） |
| 状态帧 | Server → Client | 15Hz 广播游戏状态快照 |
| 昵称设置 | Client → Server | 设置玩家昵称 |
| 阶段变更 | Server → Client | waiting/countdown/playing/ended |

### GAME_STATE_CHANGE（阶段变更）

二进制布局：

| 字段 | 类型 | 说明 |
|------|------|------|
| msgType | uint8 | 固定为 `MsgGameStateChange` |
| phaseCode | uint8 | 0=waiting, 1=playing, 2=ended, 3=countdown |
| countdownRemainingMs | uint32 LE | **仅 countdown 阶段**：距 playing 开始的剩余毫秒数 |

非 countdown 阶段消息长度为 **2 字节**；countdown 阶段为 **6 字节**。

> 注：`phase=ended` 时后端 `EncodeGameStateChangeEnded` 追加第 3 字节 `endReason`（0=none/1=ground/2=bird/3=ghost），消息长度 **3 字节**。

### SNAPSHOT（状态快照）

服务端 → 客户端，15 Hz 广播。权威编码：`backend/internal/protocol/encode.go::EncodeSnapshot`。**所有多字节字段为小端序（LE）**。

**固定头（10 字节）**：

| 偏移 | 字段 | 类型 | 字节 | 说明 |
|------|------|------|------|------|
| 0 | msgType | uint8 | 1 | 固定 `MsgSnapshot`=0x01 |
| 1 | tickCount | uint32 LE | 4 | 房间 tick 计数（前端用作序列号去重） |
| 5 | score | uint32 LE | 4 | 当前分数 |
| 9 | phaseCode | uint8 | 1 | 0=waiting/1=playing/2=ended/3=countdown |

**气球 balloon（16 字节，固定）**：`x(float32 LE)` + `y(float32 LE)` + `vy(float32 LE)` + `vx(float32 LE)`。
> 注意字段顺序为 `x, y, vy, vx`（`vy` 在 `vx` 之前），与 `BalloonState` 结构体字段顺序一致。

**鸟 bird（1 或 9 字节，条件变长）**：

| 字段 | 类型 | 条件 |
|------|------|------|
| active | uint8 | 1 字节，恒写（0/1） |
| x | float32 LE | 仅 active=1 |
| y | float32 LE | 仅 active=1 |

**幽灵 ghost（1 或 11 字节，条件变长）**：

| 字段 | 类型 | 条件 |
|------|------|------|
| active | uint8 | 1 字节，恒写（0/1） |
| x | float32 LE | 仅 active=1 |
| y | float32 LE | 仅 active=1 |
| repelTimer | uint16 LE | 仅 active=1 |

**玩家段（变长）**：`playerCount(uint8)` 后跟 `playerCount` 条玩家记录，每条布局：

| 字段 | 类型 | 字节 | 说明 |
|------|------|------|------|
| playerIndex | uint16 LE | 2 | 玩家索引 |
| cooldownMs | uint32 LE | 4 | 剩余冷却毫秒 |
| palette | uint32 LE | 4 | 调色板索引/颜色 |
| scoreContribution | uint32 LE | 4 | 该玩家分数贡献 |
| nickLen | uint8 | 1 | nickname UTF-8 字节数 |
| nickname | bytes | nickLen | UTF-8 昵称 |

**涟漪段（变长）**：`rippleCount(uint8)` 后跟 `rippleCount` 条涟漪记录，每条布局：

| 字段 | 类型 | 字节 |
|------|------|------|
| playerIndex | uint16 LE | 2 |
| x | float32 LE | 4 |
| y | float32 LE | 4 |

**风 wind（4 字节，固定，位于消息末尾）**：`wind(float32 LE)`。

**长度约束**：理论最小消息 = 10（头）+16（balloon）+1（bird flag）+1（ghost flag）+1（playerCount=0）+1（rippleCount=0）+4（wind）= **34 字节**。前端 `decodeSnapshot` 以 `byteLength < 37` 为早返回阈值（保守值），`handleSnapshot` 进一步以 `< 44` 为丢弃阈值。

### RESTART_STATUS（重启投票状态）

服务端 → 客户端。权威编码：`backend/internal/protocol/encode.go::EncodeRestartStatus`。**定长 7 字节，小端序**。

| 偏移 | 字段 | 类型 | 字节 | 说明 |
|------|------|------|------|------|
| 0 | msgType | uint8 | 1 | 固定 `MsgRestartStatus`=0x07 |
| 1 | yesVotes | uint8 | 1 | 已投赞成票数 |
| 2 | totalPlayers | uint8 | 1 | 房间总玩家数 |
| 3 | countdownMs | uint32 LE | 4 | 距重启开始的剩余毫秒数 |

前端解码见 `frontend/src/game/ws_handlers_phase.ts::handleRestartStatus`（`getUint8(1)` / `getUint8(2)` / `getUint32(3, true)`），与后端逐字段一致。

## 频率与限制

- Tick 率: **15 Hz**（66.67ms）
- 读限制: `config.WSReadLimit` 字节/消息
- 慢客户端: 连续丢弃达阈值断开（见 `room.go` consecutiveDrops）

## 消息类型常量（权威：`backend/internal/protocol/constants.go`）

客户端 → 服务端：

| 名称 | 值 |
|------|-----|
| MsgTap | 0x10 |
| MsgSetNickname | 0x11 |
| MsgRestartVote | 0x12 |
| MsgPing | 0x20 |

服务端 → 客户端：

| 名称 | 值 |
|------|-----|
| MsgSnapshot | 0x01 |
| MsgPlayerJoin | 0x02 |
| MsgPlayerLeave | 0x03 |
| MsgTapAccepted | 0x04 |
| MsgTapRejected | 0x05 |
| MsgGameStateChange | 0x06 |
| MsgRestartStatus | 0x07 |
| MsgPong | 0x21 |

## 实例与区域路由（ADR-005 / ADR-016）

1. **就近接入预检**：客户端连接前先调 `GET /api/v1/lobby/{code}/resolve`，得到房间
   home region 的 `ws_endpoint`（同源则为空），再用该 endpoint 打开 WebSocket。
2. **区域内**：房间由其 owner 实例 tick；连接到本区域非 owner 实例时透明反向代理到 owner。
3. **跨区域**：连接到错误区域时返回 **421 Misdirected Request** + `{ ws_endpoint, region }`，
   客户端就近重连房间 home region（绝不跨区域转发游戏帧）。
4. **迁移中**：区域内 owner 失联且暂未完成接管时返回 **503 Service Unavailable**（可重试）。

## 错误处理与已知限制

### decodeSnapshot 已知限制（known limitation，v2-R-143）

前端 `decodeSnapshot`（`frontend/src/game/message_codec.ts`）对**非法/超长输入的容错行为**：

- **短缓冲**：`view.byteLength < 37` 时返回 `null`，调用方 `handleSnapshot` 静默丢弃。
- **变长段越界**：对于 ≥ 37 字节但变长段的计数/长度字段超出剩余缓冲的**非法输入**，解码器**不会返回 null，而是抛出 `RangeError`**（DataView 越界读取）。
  - 玩家昵称长度 `nickLen` 已通过 `Math.min(nickLen, remaining)` 钳制，昵称读取本身**不会越界**；
  - **涟漪（ripple）段缺乏逐次循环边界检查**：读取 `rippleCount` 后无条件按 10 字节/条循环读取，当 `rippleCount` 超过剩余缓冲可容纳条数时 `getUint16`/`getFloat32` 抛 `RangeError` —— 这是主要的抛错路径。
- **调用方兜底**：`handleSnapshot`（`ws_handlers_snapshot.ts`）以 `try/catch` 包裹 `decodeSnapshot`，捕获异常后仅记录 `[snapshot] parse error` 日志，不影响 WebSocket 连接。
- **测试记录**：`snapshot_decode.property.test.ts` 将此列为 `Known limitation`，对 ≥ 37 字节的任意输入用 `try/catch` 吞掉异常（解码器实现修复见 Task 6.1）。

**契约说明**：超长 `nickLen` / `rippleCount` 均属非法输入（后端 `EncodeSnapshot` 保证长度自洽），解码器对这类输入的行为契约是"**抛异常而非返回 null**"。合法 SNAPSHOT 由后端按上述布局编码，长度自洽；前端无需对服务端正常帧做防御性兜底。

## 契约与校验

- 本文档 + `protocol/constants.go` 为 WebSocket 协议权威来源；机器可读规范见
  [`asyncapi.yaml`](./asyncapi.yaml)（CI 用 AsyncAPI CLI 校验，并校验消息常量与代码一致）。
- REST 端点见 [`openapi.yaml`](./openapi.yaml)。
