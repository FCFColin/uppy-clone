# 持续 Profiling

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

## pprof（按需）

- `ENABLE_PPROF=true`
- `curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof`
- `go tool pprof cpu.prof`

详见 [Runbook](../operations/runbook.md) 性能章节。
