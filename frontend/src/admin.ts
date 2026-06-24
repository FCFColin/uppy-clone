/**
 * 管理后台脚本
 *
 * 处理 Uppy 游戏管理后台的：
 * - 管理员密码登录（获取 admin token）
 * - 加载/保存应用配置（邮件开关、Resend API Key、发件人地址、管理员密码）
 */

export {};

// === 状态 ===
let adminToken: string = '';

// === DOM 元素 ===
const loginSection: HTMLElement = document.getElementById('login-section')!;
const configSection: HTMLElement = document.getElementById('config-section')!;
const adminPasswordInput: HTMLInputElement = document.getElementById('admin-password') as HTMLInputElement;
const loginBtn: HTMLButtonElement = document.getElementById('login-btn') as HTMLButtonElement;
const loginError: HTMLElement = document.getElementById('login-error')!;
const emailEnabledInput: HTMLInputElement = document.getElementById('email-enabled') as HTMLInputElement;
const emailStatusEl: HTMLElement = document.getElementById('email-status')!;
const resendKeyInput: HTMLInputElement = document.getElementById('resend-key') as HTMLInputElement;
const emailFromInput: HTMLInputElement = document.getElementById('email-from') as HTMLInputElement;
const newAdminPasswordInput: HTMLInputElement = document.getElementById('new-admin-password') as HTMLInputElement;
const saveBtn: HTMLButtonElement = document.getElementById('save-btn') as HTMLButtonElement;
const toastEl: HTMLElement = document.getElementById('toast')!;

/**
 * 显示 Toast 提示
 */
function showToast(msg: string, type: 'success' | 'error'): void {
  toastEl.textContent = msg;
  toastEl.className = `toast toast-${type}`;
  toastEl.style.display = 'block';
  setTimeout(() => { toastEl.style.display = 'none'; }, 3000);
}

/**
 * 更新邮件状态徽章
 */
function updateEmailStatus(enabled: boolean): void {
  if (enabled) {
    emailStatusEl.innerHTML = '<span class="status-badge status-on">已启用</span>';
  } else {
    emailStatusEl.innerHTML = '<span class="status-badge status-off">已禁用</span>';
  }
}

/**
 * 管理员登录
 *
 * 流程：POST /api/v1/admin/login → 获取 token → 显示配置区域
 */
async function doLogin(): Promise<void> {
  const password: string = adminPasswordInput.value;
  loginBtn.disabled = true;
  loginBtn.textContent = '登录中...';

  try {
    const res: Response = await fetch('/api/v1/admin/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password }),
    });
    const data: { token?: string; error?: string } = await res.json();
    if (res.ok) {
      adminToken = data.token ?? '';
      loginSection.classList.add('hidden');
      configSection.classList.remove('hidden');
      loadConfig();
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

interface AdminConfig {
  emailEnabled: boolean;
  resendApiKey: string;
  emailFrom: string;
  adminPassword?: string;
}

/**
 * 加载应用配置
 *
 * GET /api/v1/admin/config → 填充表单字段。
 * 401 表示 token 失效，返回登录界面。
 */
async function loadConfig(): Promise<void> {
  try {
    const res: Response = await fetch('/api/v1/admin/config', {
      headers: { 'Authorization': `Bearer ${adminToken}` },
    });
    if (res.status === 401) {
      adminToken = '';
      configSection.classList.add('hidden');
      loginSection.classList.remove('hidden');
      return;
    }
    const config: AdminConfig = await res.json();
    emailEnabledInput.checked = config.emailEnabled;
    resendKeyInput.value = config.resendApiKey || '';
    emailFromInput.value = config.emailFrom || '';
    updateEmailStatus(config.emailEnabled);
  } catch {
    showToast('加载配置失败', 'error');
  }
}

/**
 * 保存应用配置
 *
 * PATCH /api/v1/admin/config → 更新配置。
 * API Key 为脱敏占位符时视为未修改。
 */
async function saveConfig(): Promise<void> {
  saveBtn.disabled = true;
  saveBtn.textContent = '保存中...';

  const config: AdminConfig = {
    emailEnabled: emailEnabledInput.checked,
    resendApiKey: resendKeyInput.value,
    emailFrom: emailFromInput.value,
  };

  const newPassword: string = newAdminPasswordInput.value;
  if (newPassword) {
    config.adminPassword = newPassword;
  }

  try {
    const res: Response = await fetch('/api/v1/admin/config', {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${adminToken}`,
      },
      body: JSON.stringify(config),
    });
    if (res.ok) {
      showToast('配置已保存', 'success');
      newAdminPasswordInput.value = '';
      loadConfig();
    } else {
      showToast('保存失败', 'error');
    }
  } catch {
    showToast('网络错误', 'error');
  }

  saveBtn.disabled = false;
  saveBtn.textContent = '保存配置';
}

// === 事件绑定 ===
loginBtn.addEventListener('click', doLogin);
saveBtn.addEventListener('click', saveConfig);
adminPasswordInput.addEventListener('keydown', (e: KeyboardEvent) => {
  if (e.key === 'Enter') doLogin();
});
emailEnabledInput.addEventListener('change', function (this: HTMLInputElement) {
  updateEmailStatus(this.checked);
});
