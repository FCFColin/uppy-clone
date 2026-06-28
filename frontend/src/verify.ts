export {};

const statusEl = document.getElementById('status') as HTMLElement | null;

async function verifyMagicLink(): Promise<void> {
  const token = new URLSearchParams(window.location.search).get('token')?.trim();
  if (!token) {
    if (statusEl) statusEl.textContent = '缺少验证令牌，请重新请求登录链接';
    return;
  }

  try {
    const res = await fetch('/api/v1/auth/verify', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ token }),
    });

    if (res.ok) {
      sessionStorage.setItem('uppy-auth-ready', '1');
      window.location.replace('/');
      return;
    }

    const data = (await res.json()) as { error?: string };
    if (statusEl) statusEl.textContent = data.error || '验证失败，请重新请求登录链接';
  } catch {
    if (statusEl) statusEl.textContent = '网络错误，请重试';
  }
}

void verifyMagicLink();
