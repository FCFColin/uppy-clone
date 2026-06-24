/**
 * 首页脚本
 *
 * 处理 Uppy 游戏首页的两种登录方式：
 * - 邮箱 Magic Link 登录（调用 /api/v1/auth/request）
 * - 快速开始（调用 /api/v1/auth/quickplay + /api/v1/registry/match）
 */

export {};

import { storeRefreshToken } from './shared/auth.js';

// === DOM 元素 ===
const emailInput: HTMLInputElement = document.getElementById('email-input') as HTMLInputElement;
const loginBtn: HTMLButtonElement = document.getElementById('login-btn') as HTMLButtonElement;
const successMsg: HTMLElement = document.getElementById('success-msg')!;
const errorMsg: HTMLElement = document.getElementById('error-msg')!;
const quickplayBtn: HTMLButtonElement = document.getElementById('quickplay-btn') as HTMLButtonElement;

/**
 * 显示错误消息
 */
function showError(message: string): void {
  errorMsg.textContent = message;
  errorMsg.style.display = 'block';
}

/**
 * 请求 Magic Link 登录邮件
 *
 * 流程：验证邮箱 → POST /api/v1/auth/request → 显示成功/错误提示
 */
async function requestLoginLink(): Promise<void> {
  const email: string = emailInput.value.trim();
  if (!email || !email.includes('@')) {
    showError('请输入有效的邮箱地址');
    return;
  }

  loginBtn.disabled = true;
  loginBtn.textContent = '发送中...';
  errorMsg.style.display = 'none';

  try {
    const res: Response = await fetch('/api/v1/auth/request', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email }),
    });

    if (res.ok) {
      successMsg.style.display = 'block';
      emailInput.style.display = 'none';
      loginBtn.style.display = 'none';
    } else {
      const data: { error?: string } = await res.json();
      showError(data.error || '发送失败，请重试');
      loginBtn.disabled = false;
      loginBtn.textContent = '发送登录链接';
    }
  } catch {
    showError('网络错误，请重试');
    loginBtn.disabled = false;
    loginBtn.textContent = '发送登录链接';
  }
}

/**
 * 带重试的 fetch 请求
 *
 * 服务端冷启动时首次请求可能失败（ERR_ABORTED），
 * 自动重试一次以缓解此问题。
 */
async function fetchWithRetry(url: string, options: RequestInit, retries: number = 1): Promise<Response> {
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

/**
 * 快速开始游戏
 *
 * 流程：
 * 1. POST /api/v1/auth/quickplay 获取认证 cookie
 * 2. POST /api/v1/registry/match 匹配房间
 * 3. 跳转到 /play.html?code={lobbyCode}
 */
async function quickPlay(): Promise<void> {
  quickplayBtn.disabled = true;
  quickplayBtn.textContent = '加入中...';
  try {
    // Step 1: Quick play auth（带重试，防止冷启动首次请求失败）
    const authRes: Response = await fetchWithRetry('/api/v1/auth/quickplay', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    }, 1);
    if (!authRes.ok) {
      showError('认证失败，请重试');
      quickplayBtn.disabled = false;
      quickplayBtn.textContent = '快速开始';
      return;
    }
    const authData: { userId?: string; refreshToken?: string } = await authRes.json() as { userId?: string; refreshToken?: string };
    if (authData.userId) localStorage.setItem('uppy-player-id', authData.userId);
    if (authData.refreshToken) storeRefreshToken(authData.refreshToken);

    // Step 2: Match or create a room
    const matchRes: Response = await fetch('/api/v1/registry/match', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    if (!matchRes.ok) {
      showError('匹配房间失败，请重试');
      quickplayBtn.disabled = false;
      quickplayBtn.textContent = '快速开始';
      return;
    }
    const matchData: { lobbyCode: string } = await matchRes.json();

    // Step 3: Redirect to play page with room code
    window.location.href = `/play.html?code=${encodeURIComponent(matchData.lobbyCode)}`;
  } catch {
    showError('网络错误，请重试');
    quickplayBtn.disabled = false;
    quickplayBtn.textContent = '快速开始';
  }
}

/**
 * 通过房间号加入游戏
 * 校验格式 → 查询房间状态 → 跳转
 */
async function joinByCode(): Promise<void> {
  const input: HTMLInputElement = document.getElementById('join-code-input') as HTMLInputElement;
  const errorEl: HTMLElement = document.getElementById('join-code-error')!;
  const code: string = input.value.trim().toUpperCase();

  // 格式校验：5 位字母数字
  if (!/^[A-Z0-9]{5}$/.test(code)) {
    errorEl.textContent = '房间号为 5 位字母数字';
    errorEl.classList.remove('hidden');
    return;
  }

  errorEl.classList.add('hidden');

  try {
    // 查询房间状态
    const res: Response = await fetch(`/api/v1/registry/check/${code}`);
    const data: { full?: boolean } = await res.json();

    if (res.status === 404) {
      errorEl.textContent = '房间不存在或已关闭';
      errorEl.classList.remove('hidden');
      return;
    }

    if (data.full) {
      errorEl.textContent = '房间已满';
      errorEl.classList.remove('hidden');
      return;
    }

    // 先确保已认证
    const authCheck: Response = await fetch('/api/v1/auth/check');
    if (!authCheck.ok) {
      // 未认证，先快速认证（带重试）
      const authRes: Response = await fetchWithRetry('/api/v1/auth/quickplay', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      }, 1);
      if (authRes.ok) {
        const authData: { userId?: string; refreshToken?: string } = await authRes.json() as { userId?: string; refreshToken?: string };
        if (authData.userId) localStorage.setItem('uppy-player-id', authData.userId);
        if (authData.refreshToken) storeRefreshToken(authData.refreshToken);
      }
    }

    window.location.href = `/play.html?code=${code}`;
  } catch {
    errorEl.textContent = '网络错误，请重试';
    errorEl.classList.remove('hidden');
  }
}

// === 事件绑定 ===
loginBtn.addEventListener('click', requestLoginLink);
emailInput.addEventListener('keydown', (e: KeyboardEvent) => {
  if (e.key === 'Enter') requestLoginLink();
});
quickplayBtn.addEventListener('click', quickPlay);
document.getElementById('join-code-btn')!.addEventListener('click', joinByCode);
document.getElementById('join-code-input')!.addEventListener('keypress', (e: KeyboardEvent) => {
  if (e.key === 'Enter') joinByCode();
});
