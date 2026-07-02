import { fetchWithRefresh } from '../shared/auth.js';

export async function resolveLobbyCode(): Promise<string | null> {
  const params: URLSearchParams = new URLSearchParams(window.location.search);
  const fromUrl: string | null = params.get('code');
  if (fromUrl) return fromUrl;

  try {
    const res: Response = await fetchWithRefresh('/api/v1/registry/match', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    if (res.ok) {
      const data: { lobbyCode: string } = await res.json() as { lobbyCode: string };
      return data.lobbyCode;
    }
  } catch (e: unknown) {
    console.error('Failed to match room:', e);
  }
  return null;
}
