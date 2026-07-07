export {};

import { establishGameSession, normalizeAuthHost, sessionErrorMessage } from './shared/network/session.js';
import { initCollapsibleLeaderboard } from './index_leaderboard.js';

normalizeAuthHost();
initCollapsibleLeaderboard();

const emailInput = document.getElementById('email-input') as HTMLInputElement;
const loginBtn = document.getElementById('login-btn') as HTMLButtonElement;
const successMsg = document.getElementById('success-msg')!;
const errorMsg = document.getElementById('error-msg')!;
const quickplayBtn = document.getElementById('quickplay-btn') as HTMLButtonElement;
const joinCodeBtn = document.getElementById('join-code-btn') as HTMLButtonElement;

function showError(message: string): void {
  errorMsg.textContent = message;
  errorMsg.style.display = 'block';
}

function resetButton(btn: HTMLButtonElement, text: string): void {
  btn.disabled = false;
  btn.textContent = text;
}

function resetEmailForm(): void {
  successMsg.style.display = 'none';
  emailInput.style.display = '';
  loginBtn.style.display = '';
  resetButton(loginBtn, '发送登录链接');
  emailInput.value = '';
  emailInput.focus();
}

document.getElementById('email-change-link')?.addEventListener('click', (e) => {
  e.preventDefault();
  resetEmailForm();
});

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
      resetButton(loginBtn, '发送登录链接');
    }
  } catch {
    showError('网络错误，请重试');
    resetButton(loginBtn, '发送登录链接');
  }
}

async function quickPlay(): Promise<void> {
  quickplayBtn.disabled = true;
  quickplayBtn.textContent = '加入中...';
  try {
    const session = await establishGameSession();
    if (!session.ok) {
      showError(sessionErrorMessage(session));
      resetButton(quickplayBtn, '快速开始');
      return;
    }
    const matchRes: Response = await fetch('/api/v1/registry/match', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
    });
    if (!matchRes.ok) {
      showError('匹配房间失败，请重试');
      resetButton(quickplayBtn, '快速开始');
      return;
    }
    const matchData: { lobbyCode: string } = await matchRes.json();
    sessionStorage.setItem('uppy-auth-ready', '1');
    sessionStorage.setItem('uppy-fresh-match', matchData.lobbyCode);
    window.location.href = `/play.html?code=${encodeURIComponent(matchData.lobbyCode)}`;
  } catch {
    showError('网络错误，请重试');
    resetButton(quickplayBtn, '快速开始');
  }
}

async function joinByCode(): Promise<void> {
  const input: HTMLInputElement = document.getElementById('join-code-input') as HTMLInputElement;
  const errorEl: HTMLElement = document.getElementById('join-code-error')!;
  const code: string = input.value.trim().toUpperCase();
  if (!/^[A-Z2-9]{5}$/.test(code)) {
    errorEl.textContent = '房间号为 5 位字母数字';
    errorEl.classList.remove('hidden');
    return;
  }
  errorEl.classList.add('hidden');
  joinCodeBtn.disabled = true;
  const prevLabel = joinCodeBtn.textContent;
  joinCodeBtn.textContent = '加入中...';
  try {
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
    const authCheck: Response = await fetch('/api/v1/auth/check', { credentials: 'include' });
    if (!authCheck.ok) {
      const session = await establishGameSession();
      if (!session.ok) {
        errorEl.textContent = sessionErrorMessage(session);
        errorEl.classList.remove('hidden');
        return;
      }
    }
    sessionStorage.setItem('uppy-auth-ready', '1');
    sessionStorage.setItem('uppy-fresh-match', code);
    window.location.href = `/play.html?code=${code}`;
  } catch {
    errorEl.textContent = '网络错误，请重试';
    errorEl.classList.remove('hidden');
  } finally {
    joinCodeBtn.disabled = false;
    joinCodeBtn.textContent = prevLabel ?? '加入';
  }
}

loginBtn.addEventListener('click', requestLoginLink);
emailInput.addEventListener('keydown', (e: KeyboardEvent) => {
  if (e.key === 'Enter') requestLoginLink();
});
quickplayBtn.addEventListener('click', quickPlay);
joinCodeBtn.addEventListener('click', joinByCode);
document.getElementById('join-code-input')!.addEventListener('keypress', (e: KeyboardEvent) => {
  if (e.key === 'Enter') joinByCode();
});
