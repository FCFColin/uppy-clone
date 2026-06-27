/**
 * 管理后台脚本
 *
 * 处理 Uppy 游戏管理后台的：
 * - 管理员密码登录（获取 admin token）
 * - 加载/保存应用配置（邮件开关、Resend API Key、发件人地址、管理员密码）
 */

export {};

import { bindLoginEvents } from './admin_login.js';
import { loadConfig, saveConfig, type AdminConfig } from './admin_config.js';

let adminToken: string = '';

const loginSection: HTMLElement = document.getElementById('login-section')!;
const configSection: HTMLElement = document.getElementById('config-section')!;
const emailEnabledInput: HTMLInputElement = document.getElementById('email-enabled') as HTMLInputElement;
const emailStatusEl: HTMLElement = document.getElementById('email-status')!;
const resendKeyInput: HTMLInputElement = document.getElementById('resend-key') as HTMLInputElement;
const emailFromInput: HTMLInputElement = document.getElementById('email-from') as HTMLInputElement;
const newAdminPasswordInput: HTMLInputElement = document.getElementById('new-admin-password') as HTMLInputElement;
const saveBtn: HTMLButtonElement = document.getElementById('save-btn') as HTMLButtonElement;
const toastEl: HTMLElement = document.getElementById('toast')!;

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

function refreshConfig(): void {
  loadConfig(
    adminToken,
    () => {
      adminToken = '';
      configSection.classList.add('hidden');
      loginSection.classList.remove('hidden');
    },
    showToast,
    (config: AdminConfig) => {
      emailEnabledInput.checked = config.emailEnabled;
      resendKeyInput.value = config.resendApiKey || '';
      emailFromInput.value = config.emailFrom || '';
      updateEmailStatus(config.emailEnabled);
    },
  );
}

bindLoginEvents((token: string) => {
  adminToken = token;
  refreshConfig();
}, showToast);

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

  await saveConfig(adminToken, config, showToast, () => {
    newAdminPasswordInput.value = '';
    refreshConfig();
  });

  saveBtn.disabled = false;
  saveBtn.textContent = '保存配置';
});

emailEnabledInput.addEventListener('change', function (this: HTMLInputElement) {
  updateEmailStatus(this.checked);
});
