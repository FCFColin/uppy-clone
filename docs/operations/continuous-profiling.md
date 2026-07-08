# 持续 Profiling

> ⚠️ **状态：部分实现**
>
> - `ENABLE_PYROSCOPE` / `PYROSCOPE_SERVER_ADDRESS` 环境变量已被 `server/server_debug.go`
>   读取，但实际 Pyroscope SDK 集成为 TODO（`server_debug.go:18`：未引入
>   `github.com/grafana/pyroscope-go` 依赖）。当前仅打印日志，未推送 profile 数据。
> - `ENABLE_PPROF` / `DEBUG_PORT` 环境变量**未被代码读取**（已声明但未实现）。后端未 import
>   `net/http/pprof`，`/debug/pprof/*` 端点不可用。排障时需临时在代码中注册
>   `import _ "net/http/pprof"` 并监听 6060 端口。

## 概述

支持通过 Pyroscope 或 Parca 做 always-on 持续 profiling，并与 Grafana 集成。

## 配置

### 前置条件

- Pyroscope 服务（或 Grafana Cloud Profiles）
- `ENABLE_PYROSCOPE=true`
- `PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040`

### Profile 类型

- **CPU**：热点路径
- **Alloc Objects / Alloc Space**：分配压力
- **Inuse Objects / Inuse Space**：泄漏排查

## 使用

1. 启动 Pyroscope：`docker run -p 4040:4040 grafana/pyroscope:latest`
2. 设置环境变量并部署应用
3. Grafana → Explore → Pyroscope 查看火焰图

## pprof（按需，当前未接线）

> ⚠️ 当前后端未 import `net/http/pprof`，以下端点不可用。需先在代码中注册
> `import _ "net/http/pprof"` 并启动 HTTP 监听 6060 端口。

- `curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof`
- `go tool pprof cpu.prof`

详见 [Runbook](../operations/runbook.md) 性能章节。
