# Go Microbench 基准（protocol + game）

> 由 `make bench` 或 CI 生成，勿手改。
> 最近运行：2026-07-06（go1.26.4, 12th Gen Intel i5-12450H, Windows 11）

## protocol 包

```
goos: windows
goarch: amd64
pkg: github.com/uppy-clone/backend/internal/protocol
cpu: 12th Gen Intel(R) Core(TM) i5-12450H

BenchmarkEncodeSnapshot-12              	 3727566	       424.8 ns/op	     112 B/op	       1 allocs/op
BenchmarkEncodeSnapshot_NoPlayers-12    	 6799419	       273.2 ns/op	      48 B/op	       1 allocs/op
BenchmarkDecodeTap-12                   	1000000000	         0.7920 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeSetNickname-12           	100000000	        13.23 ns/op	       0 B/op	       0 allocs/op
BenchmarkEncodeTapAccepted-12           	11723763	       112.1 ns/op	      64 B/op	       1 allocs/op
BenchmarkDecodeMessage-12               	1000000000	         0.5380 ns/op	       0 B/op	       0 allocs/op
```

**关键指标：** `EncodeSnapshot` 424.8 ns/op 1 allocs（手写二进制 + sync.Pool）；`EncodeSnapshot_NoPlayers` 273.2 ns/op（预分配最优路径）；`DecodeTap` 0.79 ns/op 0 allocs（类型断言零开销）；`DecodeMessage` 0.54 ns/op 0 allocs（分发热路径）。

## game 包

```
goos: windows
goarch: amd64
pkg: github.com/uppy-clone/backend/internal/game
cpu: 12th Gen Intel(R) Core(TM) i5-12450H

BenchmarkApplyTapForce-12                	68920362	        16.65 ns/op	       0 B/op	       0 allocs/op
BenchmarkUpdateGhostAI-12                	 5669860	       213.1 ns/op	      40 B/op	       3 allocs/op
BenchmarkUpdateBirdAI-12                 	 5543654	       225.2 ns/op	      40 B/op	       3 allocs/op
BenchmarkCheckGhostCollision-12          	174123126	         7.300 ns/op	       0 B/op	       0 allocs/op
BenchmarkCheckBirdCollision-12           	161750532	         7.762 ns/op	       0 B/op	       0 allocs/op
BenchmarkCalculateCooldown-12            	32819256	        37.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkGenerateRoomCode-12             	 5563286	       209.1 ns/op	      45 B/op	       4 allocs/op
BenchmarkRoom_BuildSnapshot-12           	 1016462	      1341 ns/op	     256 B/op	       1 allocs/op
BenchmarkSerializeState-12               	  175393	      6372 ns/op	    1329 B/op	       7 allocs/op
BenchmarkDeserializeState-12             	   48932	     25795 ns/op	    1288 B/op	      24 allocs/op
BenchmarkNewGameState-12                 	 1479570	       722.8 ns/op	     424 B/op	       6 allocs/op
```

**关键指标：** `ApplyTapForce` 16.65 ns/op 0 allocs（物理 tick 热路径）；`CheckGhost/BirdCollision` ~7.5 ns/op 0 allocs（碰撞检测）；`GenerateRoomCode` 209.1 ns/op 4 allocs；`Room_BuildSnapshot` 1341 ns/op 1 allocs；`SerializeState` 6372 ns/op 7 allocs（持久化路径）；`DeserializeState` 25795 ns/op 24 allocs（恢复路径）。

## 容量推算

基于 15Hz tick（66.7ms/tick）和单核 i5-12450H：

| 操作 | ns/op | 单 tick 可执行次数 | 瓶颈分析 |
|------|-------|------------------|---------|
| ApplyTapForce | 16.65 | ~4M | 非瓶颈 |
| UpdateGhostAI | 213.1 | ~313k | 50 玩家 × 10 ghost = 500 次/tick，占 0.1ms |
| CheckCollision | 7.5 | ~8.9M | 非瓶颈 |
| BuildSnapshot | 1341 | ~50k | 50 房间/tick = 0.07ms |
| EncodeSnapshot | 424.8 | ~157k | 50 房间广播 = 0.02ms |
| SerializeState | 6372 | ~10k | 异步不阻塞 tick |

**推算**：单核可支撑 ~2000-5000 活跃房间（CPU bound），受限于 GC 压力和内存带宽。WS 连接和 PG 连接池是更早的瓶颈（10k WS 舱壁 / 25 PG 连接）。

## 优化历史

| 优化项 | 前 | 后 | 提升 |
|--------|-----|-----|------|
| EncodeSnapshot (sync.Pool + 手写编码) | binary.Write 反射 + 每次分配 | 预分配 + PutUint32/16 | ~3-5x |
| GenerateRoomCode (缓存 big.Int) | 每次 big.NewInt | 包级缓存 | 0 allocs/op（历史最优） |
| buildSnapshot (复用 players 切片) | 每次新切片 | 字段复用 | 减少 GC 压力 |
