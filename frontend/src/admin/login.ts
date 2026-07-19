export {};

import { apiFetch } from '../shared/network/api_fetch.js';

const loginSection = document.getElementById('login-section')!;
const configSection = document.getElementById('config-section')!;
const adminPasswordInput = document.getElementById('admin-password') as HTMLInputElement;
const loginBtn = document.getElementById('login-btn') as HTMLButtonElement;
const loginError = document.getElementById('login-error')!;

async function doLogin(
  onSuccess: () => void,
  showToast: (msg: string) => void,
): Promise<void> {
  const password: string = adminPasswordInput.value;
  loginBtn.disabled = true;
  loginBtn.textContent = '登录中...';
  try {
    const res: Response = await apiFetch('/api/v1/admin/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password }),
      autoRefresh: false,
    });
    // shared-018: Guard against non-JSON responses (e.g., proxy error pages).
    let data: { error?: string } = {};
    try {
      data = await res.json();
    } catch {
      // Response was not JSON — use generic error message.
    }
    if (res.ok) {
      loginSection.classList.add('hidden');
      configSection.classList.remove('hidden');
      onSuccess();
    } else {
      loginError.textContent = data.error || '密码错误';
      loginError.style.display = 'block';
    }
  } catch {
    showToast('网络错误');
  } finally {
    loginBtn.disabled = false;
    loginBtn.textContent = '登录';
  }
}

export function bindLoginEvents(
  onSuccess: () => void,
  showToast: (msg: string) => void,
): void {
  loginBtn.addEventListener('click', () => doLogin(onSuccess, showToast));
  adminPasswordInput.addEventListener('keydown', (e: KeyboardEvent) => {
    if (e.key === 'Enter') doLogin(onSuccess, showToast);
  });
}
