export {};

const loginSection: HTMLElement = document.getElementById('login-section')!;
const configSection: HTMLElement = document.getElementById('config-section')!;
const adminPasswordInput: HTMLInputElement = document.getElementById('admin-password') as HTMLInputElement;
const loginBtn: HTMLButtonElement = document.getElementById('login-btn') as HTMLButtonElement;
const loginError: HTMLElement = document.getElementById('login-error')!;

async function doLogin(
  onSuccess: () => void,
  showToast: (msg: string, type: 'success' | 'error') => void,
): Promise<void> {
  const password: string = adminPasswordInput.value;
  loginBtn.disabled = true;
  loginBtn.textContent = '登录中...';
  try {
    const res: Response = await fetch('/api/v1/admin/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ password }),
    });
    const data: { error?: string } = await res.json();
    if (res.ok) {
      loginSection.classList.add('hidden');
      configSection.classList.remove('hidden');
      onSuccess();
    } else {
      loginError.textContent = data.error || '密码错误';
      loginError.style.display = 'block';
      loginBtn.disabled = false;
      loginBtn.textContent = '登录';
    }
  } catch {
    showToast('网络错误', 'error');
    loginBtn.disabled = false;
    loginBtn.textContent = '登录';
  }
}

export function bindLoginEvents(
  onSuccess: () => void,
  showToast: (msg: string, type: 'success' | 'error') => void,
): void {
  loginBtn.addEventListener('click', () => doLogin(onSuccess, showToast));
  adminPasswordInput.addEventListener('keydown', (e: KeyboardEvent) => {
    if (e.key === 'Enter') doLogin(onSuccess, showToast);
  });
}
