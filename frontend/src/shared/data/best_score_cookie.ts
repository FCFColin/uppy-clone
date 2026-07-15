import { apiFetch } from '../network/api_fetch.js';

const BEST_SCORE_COOKIE = 'uppy-best-score';
const BEST_MAX_AGE_SEC = 31536000;

export function getCookieBestScore(): number {
  const match = document.cookie.match(new RegExp(`(?:^|; )${BEST_SCORE_COOKIE}=([^;]*)`));
  if (!match?.[1]) return 0;
  const n = parseInt(match[1], 10);
  return Number.isFinite(n) ? n : 0;
}

export function setCookieBestScore(score: number): void {
  const secure = window.location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${BEST_SCORE_COOKIE}=${score}; path=/; max-age=${BEST_MAX_AGE_SEC}; samesite=lax${secure}`;
}

export async function fetchUserBestScore(): Promise<number> {
  try {
    const res = await apiFetch('/api/v1/user/stats');
    if (!res.ok) return getCookieBestScore();
    const data: { bestScore?: number } = await res.json();
    const apiScore = data.bestScore ?? 0;
    // shared-004: Write back to cookie so subsequent loads don't need a
    // network round-trip and the cookie stays in sync with the server.
    const cookieScore = getCookieBestScore();
    if (apiScore > cookieScore) {
      setCookieBestScore(apiScore);
    }
    return apiScore;
  } catch {
    return getCookieBestScore();
  }
}

export function updateBestScore(currentScore: number): { best: number; isNewRecord: boolean } {
  const prev = getCookieBestScore();
  if (currentScore > prev) {
    setCookieBestScore(currentScore);
    return { best: currentScore, isNewRecord: true };
  }
  return { best: prev, isNewRecord: false };
}
