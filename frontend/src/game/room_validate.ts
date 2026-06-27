import { refreshAccessToken } from '../shared/auth.js';

export type RoomValidateResult =
  | { ok: true }
  | { ok: false; reason: 'not_found' | 'ended' }
  | { ok: true; degraded: true };

interface RoomCheckResponse {
  phase?: string;
  degraded?: boolean;
}

export function getLobbyCodeFromUrl(): string | null {
  const params = new URLSearchParams(window.location.search);
  return params.get('code');
}

export async function validateRoomCode(code: string): Promise<RoomValidateResult> {
  try {
    const res = await fetch(`/api/v1/registry/check/${code}`);
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
    let res = await fetch('/api/v1/registry/match', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
    });
    if (res.status === 401) {
      const refreshed = await refreshAccessToken();
      if (refreshed) {
        res = await fetch('/api/v1/registry/match', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
        });
      }
    }
    if (!res.ok) return null;
    const data = (await res.json()) as { lobbyCode: string };
    return data.lobbyCode ?? null;
  } catch {
    return null;
  }
}
