import { fetchWithRefresh } from '../shared/network/auth.js';
import { createFetchTimeout } from './fetch_timeout.js';

export async function resolveLobbyCode(): Promise<string | null> {
  const params: URLSearchParams = new URLSearchParams(window.location.search);
  const fromUrl: string | null = params.get('code');
  if (fromUrl) return fromUrl;

  const { signal, cleanup } = createFetchTimeout();
  try {
    const res: Response = await fetchWithRefresh('/api/v1/registry/match', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      signal,
    });
    if (res.ok) {
      const data: { lobbyCode: string } = await res.json() as { lobbyCode: string };
      return data.lobbyCode;
    }
  } catch (e: unknown) {
    console.error('Failed to match room:', e);
  } finally {
    cleanup();
  }
  return null;
}
