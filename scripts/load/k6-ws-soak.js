import ws from 'k6/ws';
import http from 'k6/http';
import { check } from 'k6';
import { Trend, Counter } from 'k6/metrics';

// WebSocket soak / horizontal-scaling load test (P1/P4).
// 验证目标：N 实例下并发 WS 连接与活跃房间线性扩容，p99 game-message 延迟达标。
//
// Usage:
//   k6 run scripts/load/k6-ws-soak.js -e BASE_URL=http://localhost:8080 -e WS_URL=ws://localhost:8080
//   分布式高并发（P4）：k6 cloud 或多 runner，配合 --vus / stages 调高 target。

const wsConnectTime = new Trend('ws_connect_time', true);
const wsFirstSnapshot = new Trend('ws_first_snapshot_ms', true);
const wsMessages = new Counter('ws_messages_received');

export const options = {
  scenarios: {
    rooms: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: Number(__ENV.VUS || 200) },
        { duration: '3m', target: Number(__ENV.VUS || 200) },
        { duration: '1m', target: 0 },
      ],
    },
  },
  thresholds: {
    ws_connect_time: ['p(95)<1000'],
    ws_first_snapshot_ms: ['p(99)<500'],
    checks: ['rate>0.95'],
  },
};

const base = __ENV.BASE_URL || 'http://localhost:8080';
const wsBase = __ENV.WS_URL || base.replace(/^http/, 'ws');

// CLIENT_MSG.PING from frontend/src/game/constants.ts; keepalive only.
const CLIENT_PING = 0x09;

export default function () {
  // 1) Quickplay auth (cookie-based) + create a room via REST.
  const jar = http.cookieJar();
  const qp = http.post(`${base}/api/v1/auth/quickplay`, JSON.stringify({ nickname: `k6_${__VU}` }), {
    headers: { 'Content-Type': 'application/json' },
  });
  check(qp, { 'quickplay 200': (r) => r.status === 200 });

  const create = http.post(`${base}/api/v1/registry/create`, null, { jar });
  check(create, { 'create 200': (r) => r.status === 200 });
  let code = '';
  try {
    code = JSON.parse(create.body).code;
  } catch (_) {
    return;
  }
  if (!code) return;

  // 2) Open the game WebSocket and measure connect + first snapshot latency.
  const start = Date.now();
  const url = `${wsBase}/api/v1/lobby/${code}/ws`;
  const res = ws.connect(url, { jar }, (socket) => {
    let firstSnapshotSeen = false;
    socket.on('open', () => {
      wsConnectTime.add(Date.now() - start);
      // Heartbeat to keep the connection alive during the soak window.
      socket.setInterval(() => socket.sendBinary(new Uint8Array([CLIENT_PING]).buffer), 5000);
    });
    socket.on('binaryMessage', () => {
      wsMessages.add(1);
      if (!firstSnapshotSeen) {
        firstSnapshotSeen = true;
        wsFirstSnapshot.add(Date.now() - start);
      }
    });
    socket.setTimeout(() => socket.close(), 30000);
  });
  check(res, { 'ws 101': (r) => r && r.status === 101 });
}
