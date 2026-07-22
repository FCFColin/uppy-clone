import { refreshAccessToken } from './auth.js';
import { apiFetch } from './api_fetch.js';

function isAbortError(e: unknown): boolean {
  return e instanceof DOMException && e.name === 'AbortError';
}

export type SessionResult = { ok: true } | { ok: false; status?: number; reason: 'rate_limit' | 'network' | 'server' };

export function normalizeAuthHost(): void {
  // Redirect 127.0.0.1 to localhost to match cookie domain expectations.
  // Skip redirect when the hostname is already localhost or a non-loopback address,
  // to support headless browsers and CI environments that connect via 127.0.0.1 directly.
  // shared-021: Removed navigator.webdriver check — it's fragile (not supported in all
  // browsers, can be spoofed) and redundant with window.__CI which is the explicit
  // CI suppression mechanism.
  if (window.location.hostname === '127.0.0.1' && !window.__CI) {
    const port = window.location.port ? `:${window.location.port}` : '';
    window.location.replace(
      `http://localhost${port}${window.location.pathname}${window.location.search}${window.location.hash}`,
    );
  }
}

// Allow CI environments to suppress the 127.0.0.1 → localhost redirect.
declare global {
  interface Window {
    __CI?: boolean;
  }
}

export async function establishGameSession(): Promise<SessionResult> {
  try {
    // autoRefresh=false: session.ts orchestrates refresh manually below to
    // distinguish "needs quickplay" from "needs refresh" paths.
    if (sessionStorage.getItem('uppy-auth-ready') === '1') {
      sessionStorage.removeItem('uppy-auth-ready');
      const quickCheck: Response = await apiFetch('/api/v1/auth/check', { retries: 0, autoRefresh: false });
      if (quickCheck.ok) return { ok: true };
    }

    const checkRes: Response = await apiFetch('/api/v1/auth/check', { retries: 0, autoRefresh: false });
    if (checkRes.ok) return { ok: true };

    const refreshed = await refreshAccessToken();
    if (refreshed) {
      const recheck: Response = await apiFetch('/api/v1/auth/check', { retries: 0, autoRefresh: false });
      if (recheck.ok) return { ok: true };
    }

    const savedNick = localStorage.getItem('uppy-nickname') || '';
    const body = savedNick ? { nickname: savedNick } : {};
    // shared-002: POST is non-idempotent — never retry. A retry could create
    // duplicate sessions if the server received the request but the response
    // was lost. retries=0 ensures the POST is sent exactly once.
    const res = await apiFetch('/api/v1/auth/quickplay', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      retries: 0,
      autoRefresh: false,
    });

    if (!res.ok) {
      if (res.status === 429) return { ok: false, status: 429, reason: 'rate_limit' };
      return { ok: false, status: res.status, reason: 'server' };
    }
    return { ok: true };
  } catch (e) {
    if (isAbortError(e)) {
      return { ok: false, reason: 'network' };
    }
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
