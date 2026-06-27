import http from 'k6/http';
import { check, sleep } from 'k6';

// ADR-V2-030: HTTP load smoke — auth quickplay + health + room create (requires running server).
// Usage: k6 run scripts/load/k6-smoke.js -e BASE_URL=http://localhost:8080

export const options = {
  stages: [
    { duration: '30s', target: 50 },
    { duration: '30s', target: 100 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'],
    http_req_failed: ['rate<0.05'],
  },
};

const base = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
  const health = http.get(`${base}/health/live`);
  check(health, { 'health 200': (r) => r.status === 200 });

  const qp = http.post(`${base}/api/v1/auth/quickplay`, JSON.stringify({ nickname: `k6_${__VU}` }), {
    headers: { 'Content-Type': 'application/json' },
  });
  check(qp, { 'quickplay 200': (r) => r.status === 200 });

  sleep(0.5);
}
