# WebSocket 二进制协议

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

**长度约束**：理论最小消息 = 10（头）+16（balloon）+1（bird flag）+1（ghost flag）+1（playerCount=0）+1（rippleCount=0）+4（wind）= **34 字节**。前端 `decodeSnapshot` 以 `byteLength < 37` 早返回，`handleSnapshot` 以 `< 44` 丢弃。

### RESTART_STATUS（重启投票状态）

服务端 → 客户端。权威编码：`backend/internal/protocol/encode.go::EncodeRestartStatus`。**定长 7 字节，小端序**。

| 偏移 | 字段 | 类型 | 字节 | 说明 |
|------|------|------|------|------|
| 0 | msgType | uint8 | 1 | 固定 `MsgRestartStatus`=0x07 |
| 1 | yesVotes | uint8 | 1 | 已投赞成票数 |
| 2 | totalPlayers | uint8 | 1 | 房间总玩家数 |
| 3 | countdownMs | uint32 LE | 4 | 距重启开始的剩余毫秒数 |

前端解码见 `ws_handlers_phase.ts::handleRestartStatus`（`getUint8(1)` / `getUint8(2)` / `getUint32(3, true)`），与后端逐字段一致。

### NICKNAME_REJECTED（昵称拒绝反馈）

服务端 → 客户端。权威编码：`backend/internal/protocol/encode.go::EncodeNicknameRejected`。**定长 2 字节**。

| 偏移 | 字段 | 类型 | 字节 | 说明 |
|------|------|------|------|------|
| 0 | msgType | uint8 | 1 | 固定 `MsgNicknameRejected`=0x08 |
| 1 | reason | uint8 | 1 | 拒绝原因码，见下表 |

**拒绝原因码（`NICKNAME_REJECT_REASON`）**：

| 名称 | 值 | 说明 |
|------|-----|------|
| EMPTY | 0x01 | 昵称为空或 sanitize 后为空 |
| DUPLICATE | 0x02 | 昵称已被房间内其他玩家占用 |
| COOLDOWN | 0x03 | 昵称处于冷却期（玩家断线重连后短时间内重复提交同名） |
| DECODE_ERROR | 0x04 | `DecodeNicknamePayload` 解码失败（payload 长度非法） |

**触发场景**：`room_tick.go::handleSetNicknameMsg` 三条拒绝路径（sanitize 为空 / 解码失败 / 冷却或重复）在 `return` 前发送此消息，避免客户端静默卡在 `entryStep='waiting'`。

前端解码见 `ws_handlers.ts::handleNicknameRejected`（`getUint8(1)`），收到后重置 `nicknameSubmitted=false`、`pendingNickname=null`，回退到昵称输入步骤，清除倒计时并显示对应中文文案。

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
| MsgNicknameRejected | 0x08 |
| MsgPong | 0x21 |

## 实例与区域路由（ADR-005 / ADR-014）

1. **就近接入预检**：连接前调 `GET /api/v1/lobby/{code}/resolve` 获取 home region 的 `ws_endpoint`（同源为空），再用该 endpoint 打开 WebSocket。
2. **区域内**：房间由 owner 实例 tick；连接到非 owner 实例时透明反向代理到 owner。
3. **跨区域**：错误区域返回 **421 Misdirected Request** + `{ ws_endpoint, region }`，客户端就近重连（绝不跨区域转发游戏帧）。
4. **迁移中**：owner 失联且未完成接管时返回 **503 Service Unavailable**（可重试）。

## 错误处理与已知限制

### decodeSnapshot 已知限制（known limitation，v2-R-143）

前端 `decodeSnapshot`（`message_codec.ts`）对**非法/超长输入**的容错行为：

- **短缓冲**：`byteLength < 37` 返回 `null`，`handleSnapshot` 静默丢弃。
- **变长段越界**：≥ 37 字节但计数/长度字段超出剩余缓冲的非法输入，解码器**抛 `RangeError` 而非返回 null**。
  - `nickLen` 已 `Math.min(nickLen, remaining)` 钳制，昵称读取不越界；
  - **涟漪段缺乏循环边界检查**：`rippleCount` 超容量时 `getUint16`/`getFloat32` 抛 `RangeError`——主要抛错路径。
- **调用方兜底**：`handleSnapshot`（`ws_handlers_snapshot.ts`）`try/catch` 包裹，仅记 `[snapshot] parse error` 日志，不影响连接。

**契约**：超长 `nickLen` / `rippleCount` 均属非法输入（后端 `EncodeSnapshot` 保证长度自洽），解码器对此类输入的行为契约是"抛异常而非返回 null"。合法 SNAPSHOT 由后端按上述布局编码，前端无需对正常帧做防御兜底。

## 契约与校验

- 本文档 + `protocol/constants.go` 为 WebSocket 协议权威来源；机器可读规范见 [`asyncapi.yaml`](./asyncapi.yaml)（CI 用 AsyncAPI CLI 校验消息常量与代码一致）。
- REST 端点见 [`openapi.yaml`](./openapi.yaml)。
