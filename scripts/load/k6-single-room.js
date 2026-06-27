import ws from 'k6/ws';
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

// Single-room soak: many players in one lobby, verify no mass disconnect under tick load.
//
// Usage:
//   k6 run scripts/load/k6-single-room.js -e BASE_URL=http://localhost:8080 -e PLAYERS=50

const wsFirstSnapshot = new Trend('ws_first_snapshot_ms', true);
const wsDisconnects = new Counter('ws_unexpected_disconnects');

export const options = {
  scenarios: {
    single_room: {
      executor: 'shared-iterations',
      vus: Number(__ENV.PLAYERS || 50),
      iterations: Number(__ENV.PLAYERS || 50),
      maxDuration: '5m',
    },
  },
  thresholds: {
    ws_first_snapshot_ms: ['p(99)<500'],
    ws_unexpected_disconnects: ['count<1'],
    checks: ['rate>0.95'],
  },
};

const base = __ENV.BASE_URL || 'http://localhost:8080';
const wsBase = __ENV.WS_URL || base.replace(/^http/, 'ws');
const CLIENT_PING = 0x09;

export function setup() {
  const jar = http.cookieJar();
  const qp = http.post(`${base}/api/v1/auth/quickplay`, JSON.stringify({ nickname: 'host_k6' }), {
    headers: { 'Content-Type': 'application/json' },
  });
  check(qp, { 'host quickplay': (r) => r.status === 200 });

  const create = http.post(`${base}/api/v1/registry/create`, null, { jar });
  check(create, { 'create room': (r) => r.status === 200 || r.status === 201 });
  const body = create.json();
  return { code: body.code || body.lobbyCode || body.data?.code };
}

export default function (data) {
  const code = data.code;
  if (!code) return;

  const jar = http.cookieJar();
  const qp = http.post(`${base}/api/v1/auth/quickplay`, JSON.stringify({ nickname: `k6_${__VU}` }), {
    headers: { 'Content-Type': 'application/json' },
  });
  check(qp, { 'quickplay': (r) => r.status === 200 });

  const url = `${wsBase}/api/v1/lobby/${code}/ws`;
  const connectStart = Date.now();
  let gotSnapshot = false;

  const res = ws.connect(url, { jar }, (socket) => {
    socket.on('open', () => {
      socket.setInterval(() => {
        socket.sendBinary(new Uint8Array([CLIENT_PING]).buffer);
      }, 5000);
    });

    socket.on('binaryMessage', () => {
      if (!gotSnapshot) {
        gotSnapshot = true;
        wsFirstSnapshot.add(Date.now() - connectStart);
      }
    });

    socket.on('close', () => {
      if (!gotSnapshot) {
        wsDisconnects.add(1);
      }
    });

    socket.setTimeout(() => {
      socket.close();
    }, 180000);
  });

  check(res, { 'ws connected': (r) => r && r.status === 101 });
  sleep(1);
}
