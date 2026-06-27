/**
 * 带重试的 fetch 请求
 *
 * 服务端冷启动时首次请求可能失败（ERR_ABORTED），
 * 自动重试一次以缓解此问题。
 */
export async function fetchWithRetry(url: string, options: RequestInit, retries: number = 1): Promise<Response> {
  for (let i = 0; i <= retries; i++) {
    try {
      const res: Response = await fetch(url, options);
      return res;
    } catch (e) {
      if (i < retries) {
        console.warn(`Request to ${url} failed, retrying... (${i + 1}/${retries})`);
        await new Promise<void>((r) => setTimeout(r, 500));
        continue;
      }
      throw e;
    }
  }
  throw new Error('fetchWithRetry: unreachable');
}
