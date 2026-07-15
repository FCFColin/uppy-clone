import { apiFetch } from '../shared/network/api_fetch.js';

export type RoomValidateResult =
  | { ok: true }
  | { ok: false; reason: 'not_found' | 'ended' }
  | { ok: true; degraded: true };

interface RoomCheckResponse {
  phase?: string;
  degraded?: boolean;
}

export const ROOM_CODE_RE = /^[A-Z2-9]{5}$/;

export function getLobbyCodeFromUrl(): string | null {
  const code = new URLSearchParams(window.location.search).get('code');
  return code && ROOM_CODE_RE.test(code) ? code : null;
}

export async function validateRoomCode(code: string): Promise<RoomValidateResult> {
  if (!ROOM_CODE_RE.test(code)) {
    return { ok: false, reason: 'not_found' };
  }
  try {
    const encoded = encodeURIComponent(code);
    const res = await apiFetch(`/api/v1/registry/check/${encoded}`, {
      timeoutMs: 8000,
      retries: 0,
      autoRefresh: false,
    });
    if (res.status === 404) {
      return { ok: false, reason: 'not_found' };
    }
    if (!res.ok) {
      return { ok: true, degraded: true };
    }
    const data = (await res.json()) as RoomCheckResponse;
    if (data.degraded) {
      return { ok: true, degraded: true };
    }
    if (data.phase === 'ended') {
      return { ok: false, reason: 'ended' };
    }
    return { ok: true };
  } catch {
    return { ok: true, degraded: true };
  }
}

export function roomErrorMessage(reason: 'not_found' | 'ended'): string {
  return reason === 'ended' ? '房间已结束' : '房间不存在或已关闭';
}

export async function matchNewRoomCode(): Promise<string | null> {
  try {
    const res = await apiFetch('/api/v1/registry/match', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      timeoutMs: 8000,
      retries: 0,
    });
    if (!res.ok) return null;
    const data = (await res.json()) as { lobbyCode: string };
    return data.lobbyCode ?? null;
  } catch {
    return null;
  }
}
