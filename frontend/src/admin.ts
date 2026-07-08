/**
 * 管理后台脚本
 *
 * 处理 Uppy 游戏管理后台的：
 * - 管理员密码登录（admin_token HttpOnly cookie）
 * - 加载/保存应用配置（邮件开关、Resend API Key、发件人地址、管理员密码）
 */

export {};

import { bindLoginEvents } from './admin/login.js';
import { loadConfig, saveConfig, type AdminConfig } from './admin/config.js';

const loginSection = document.getElementById('login-section')!;
const configSection = document.getElementById('config-section')!;
const emailEnabledInput = document.getElementById('email-enabled') as HTMLInputElement;
const emailStatusEl = document.getElementById('email-status')!;
const resendKeyInput = document.getElementById('resend-key') as HTMLInputElement;
const emailFromInput = document.getElementById('email-from') as HTMLInputElement;
const newAdminPasswordInput = document.getElementById('new-admin-password') as HTMLInputElement;
const saveBtn = document.getElementById('save-btn') as HTMLButtonElement;
const toastEl = document.getElementById('toast')!;

function showToast(msg: string, type: 'success' | 'error'): void {
  toastEl.textContent = msg;
  toastEl.className = `toast toast-${type}`;
  toastEl.style.display = 'block';
  setTimeout(() => { toastEl.style.display = 'none'; }, 3000);
}

function updateEmailStatus(enabled: boolean): void {
  if (enabled) {
    emailStatusEl.innerHTML = '<span class="status-badge status-on">已启用</span>';
  } else {
    emailStatusEl.innerHTML = '<span class="status-badge status-off">已禁用</span>';
  }
}

function showLogin(): void {
  configSection.classList.add('hidden');
  loginSection.classList.remove('hidden');
}

function refreshConfig(): void {
  loadConfig(
    showLogin,
    showToast,
    (config: AdminConfig) => {
      emailEnabledInput.checked = config.emailEnabled;
      resendKeyInput.value = config.resendApiKey || '';
      emailFromInput.value = config.emailFrom || '';
      updateEmailStatus(config.emailEnabled);
    },
  );
}

bindLoginEvents(refreshConfig, showToast);

saveBtn.addEventListener('click', async () => {
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

  await saveConfig(config, showToast, () => {
    newAdminPasswordInput.value = '';
    refreshConfig();
  });

  saveBtn.disabled = false;
  saveBtn.textContent = '保存配置';
});

emailEnabledInput.addEventListener('change', function (this: HTMLInputElement) {
  updateEmailStatus(this.checked);
});
