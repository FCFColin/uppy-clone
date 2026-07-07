import { refreshAccessToken } from './auth.js';
import { fetchWithRetry } from './fetch.js';

export type SessionResult =
  | { ok: true }
  | { ok: false; status?: number; reason: 'rate_limit' | 'network' | 'server' };

export function normalizeAuthHost(): void {
  // Redirect 127.0.0.1 to localhost to match cookie domain expectations.
  // Skip redirect when the hostname is already localhost or a non-loopback address,
  // to support headless browsers and CI environments that connect via 127.0.0.1 directly.
  if (window.location.hostname === '127.0.0.1' && !navigator.webdriver && !window.__CI) {
    const port = window.location.port ? `:${window.location.port}` : '';
    window.location.replace(
      `http://localhost${port}${window.location.pathname}${window.location.search}${window.location.hash}`,
    );
  }
}

// Allow CI environments to suppress the 127.0.0.1 → localhost redirect.
declare global {
  interface Window { __CI?: boolean; }
}

export async function establishGameSession(): Promise<SessionResult> {
  try {
    if (sessionStorage.getItem('uppy-auth-ready') === '1') {
      sessionStorage.removeItem('uppy-auth-ready');
      const quickCheck: Response = await fetch('/api/v1/auth/check', { credentials: 'include' });
      if (quickCheck.ok) return { ok: true };
    }

    const checkRes: Response = await fetch('/api/v1/auth/check', { credentials: 'include' });
    if (checkRes.ok) return { ok: true };

    const refreshed = await refreshAccessToken();
    if (refreshed) {
      const recheck: Response = await fetch('/api/v1/auth/check', { credentials: 'include' });
      if (recheck.ok) return { ok: true };
    }

    const savedNick = localStorage.getItem('uppy-nickname') || '';
    const body = savedNick ? { nickname: savedNick } : {};
    const res = await fetchWithRetry('/api/v1/auth/quickplay', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(body),
    }, 2);

    if (!res.ok) {
      if (res.status === 429) return { ok: false, status: 429, reason: 'rate_limit' };
      return { ok: false, status: res.status, reason: 'server' };
    }
    return { ok: true };
  } catch {
    return { ok: false, reason: 'network' };
  }
}

export function sessionErrorMessage(session: SessionResult & { ok: false }): string {
  const reasons: Record<string, string> = {
    rate_limit: '操作太频繁，请稍后再试',
    network: '网络连接失败，请检查网络',
    server: '认证失败，请刷新后重试',
  };
  return reasons[session.reason] || '连接失败，请稍后重试';
}
