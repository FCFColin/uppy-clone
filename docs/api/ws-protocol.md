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

## 契约与校验

- 本文档 + `protocol/constants.go` 为 WebSocket 协议权威来源；机器可读规范见
  [`asyncapi.yaml`](./asyncapi.yaml)（CI 用 AsyncAPI CLI 校验，并校验消息常量与代码一致）。
- REST 端点见 [`openapi.yaml`](./openapi.yaml)。
