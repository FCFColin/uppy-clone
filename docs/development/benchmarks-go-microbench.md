# Go Microbench 基准（protocol + game）

> 由 `make bench` 或 CI 生成，勿手改。

## protocol 包（P1-6/P1-7 优化后）

```
goos: windows
goarch: amd64
pkg: github.com/uppy-clone/backend/internal/protocol
cpu: 12th Gen Intel(R) Core(TM) i5-12450H

BenchmarkEncodeSnapshot-12                       5731400               184.7 ns/op           112 B/op          1 allocs/op
BenchmarkEncodeSnapshot_NoPlayers-12            12231392               128.1 ns/op            48 B/op          1 allocs/op
```

**关键指标：**
- `EncodeSnapshot`：184.7 ns/op，1 allocs/op — 手写二进制编码 + sync.Pool 复用 bytes.Buffer
- `EncodeSnapshot_NoPlayers`：128.1 ns/op，1 allocs/op — 预分配 buffer 最优路径

## game 包

```
goos: windows
goarch: amd64
pkg: github.com/uppy-clone/backend/internal/game
cpu: 12th Gen Intel(R) Core(TM) i5-12450H

BenchmarkGenerateRoomCode-12                   105929522               10.27 ns/op            0 B/op          0 allocs/op
BenchmarkSerializeState-12                        211993              5545 ns/op    1201 B/op          7 allocs/op
BenchmarkDeserializeState-12                       54816             25976 ns/op    1288 B/op         24 allocs/op
BenchmarkNewGameState-12                          553848              2249 ns/op     624 B/op         18 allocs/op
```

**关键指标：**
- `GenerateRoomCode`：10.27 ns/op，0 allocs/op — 缓存 `alphabetLen` big.Int 后零分配
- `SerializeState`：5545 ns/op，7 allocs/op
- `DeserializeState`：25976 ns/op，24 allocs/op
- `NewGameState`：2249 ns/op，18 allocs/op

## 优化总结

| 优化项 | 前 | 后 | 提升 |
|--------|-----|-----|------|
| EncodeSnapshot (sync.Pool + 手写编码) | binary.Write 反射 + 每次分配 | 预分配 + PutUint32/16 | ~3-5x |
| GenerateRoomCode (缓存 big.Int) | 每次 big.NewInt | 包级缓存 | 0 allocs/op |
| buildSnapshot (复用 players 切片) | 每次新切片 | 字段复用 | 减少 GC 压力 |
