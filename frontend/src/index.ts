export {};

import { apiFetch } from './shared/network/api_fetch.js';
import { establishGameSession, normalizeAuthHost, sessionErrorMessage } from './shared/network/session.js';
import { initCollapsibleLeaderboard } from './index_leaderboard.js';
import { ROOM_CODE_RE } from './game/lobby_match.js';

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
  // shared-013: Basic email validation — must have @ and a dot in the domain part.
  // Use a simple regex to catch common malformed inputs without rejecting valid edge cases.
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  if (!email || !emailRegex.test(email)) {
    showError('请输入有效的邮箱地址');
    return;
  }
  loginBtn.disabled = true;
  loginBtn.textContent = '发送中...';
  errorMsg.style.display = 'none';
  try {
    const res: Response = await apiFetch('/api/v1/auth/request', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email }),
      autoRefresh: false,
      retries: 0,
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
    const matchData: { lobbyCode?: string } = await matchRes.json();
    // shared-014: Validate lobbyCode before using it.
    if (!matchData.lobbyCode) {
      showError('匹配房间失败，请重试');
      resetButton(quickplayBtn, '快速开始');
      return;
    }
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
  if (!ROOM_CODE_RE.test(code)) {
    errorEl.textContent = '房间号为 5 位字母数字';
    errorEl.classList.remove('hidden');
    return;
  }
  errorEl.classList.add('hidden');
  joinCodeBtn.disabled = true;
  const prevLabel = joinCodeBtn.textContent;
  joinCodeBtn.textContent = '加入中...';
  try {
    const res: Response = await apiFetch(`/api/v1/registry/check/${code}`);
    if (res.status === 404) {
      errorEl.textContent = '房间不存在或已关闭';
      errorEl.classList.remove('hidden');
      return;
    }
    // shared-010: Check !res.ok before parsing JSON — a 500 response
    // may not contain valid JSON and would throw.
    if (!res.ok) {
      errorEl.textContent = '服务器错误，请重试';
      errorEl.classList.remove('hidden');
      return;
    }
    const data: { full?: boolean } = await res.json();
    if (data.full) {
      errorEl.textContent = '房间已满';
      errorEl.classList.remove('hidden');
      return;
    }
    const session = await establishGameSession();
    if (!session.ok) {
      errorEl.textContent = sessionErrorMessage(session);
      errorEl.classList.remove('hidden');
      return;
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
document.getElementById('join-code-input')!.addEventListener('keydown', (e: KeyboardEvent) => {
  if (e.key === 'Enter') joinByCode();
});

