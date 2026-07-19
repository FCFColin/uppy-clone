import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock apiFetch so doLogin's network call is controllable.
const apiFetchMock = vi.hoisted(() => vi.fn());
vi.mock('../shared/network/api_fetch.js', () => ({
  apiFetch: apiFetchMock,
}));

import { bindLoginEvents } from './login.js';

// login.ts reads DOM elements at module load; set them up before import.
vi.hoisted(() => {
  document.body.innerHTML = `
    <div id="login-section"></div>
    <div id="config-section" class="hidden"></div>
    <input id="admin-password" type="password" />
    <button id="login-btn">登录</button>
    <div id="login-error"></div>
  `;
});

describe('admin/login', () => {
  beforeEach(() => {
    apiFetchMock.mockReset();
    // Reset DOM state between tests.
    document.getElementById('login-section')!.classList.remove('hidden');
    document.getElementById('config-section')!.classList.add('hidden');
    document.getElementById('login-error')!.style.display = 'none';
    (document.getElementById('login-error') as HTMLElement).textContent = '';
    (document.getElementById('admin-password') as HTMLInputElement).value = '';
    const btn = document.getElementById('login-btn') as HTMLButtonElement;
    btn.disabled = false;
    btn.textContent = '登录';
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  function clickLogin(): void {
    document.getElementById('login-btn')!.dispatchEvent(new Event('click'));
  }

  function pressEnter(): void {
    const input = document.getElementById('admin-password')!;
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
  }

  it('hides login and shows config on successful login', async () => {
    apiFetchMock.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({}),
    } as Response);
    const onSuccess = vi.fn();
    const showToast = vi.fn();
    bindLoginEvents(onSuccess, showToast);
    (document.getElementById('admin-password') as HTMLInputElement).value = 'secret';
    clickLogin();
    // Allow microtasks to flush.
    await new Promise((r) => setTimeout(r, 10));
    expect(document.getElementById('login-section')!.classList.contains('hidden')).toBe(true);
    expect(document.getElementById('config-section')!.classList.contains('hidden')).toBe(false);
    expect(onSuccess).toHaveBeenCalledOnce();
  });

  it.each([
    ['shows error text on failed login', { ok: false, status: 401, jsonBody: { error: '密码错误' } }, '密码错误', true],
    ['uses generic error when response json fails to parse', { ok: false, status: 502, jsonBody: null }, '密码错误', false],
  ] as const)('%s', async (_label, response, expectedText, checkDisplay) => {
    apiFetchMock.mockResolvedValue({
      ok: response.ok,
      status: response.status,
      json: async () => {
        if (response.jsonBody === null) throw new Error('not json');
        return response.jsonBody;
      },
    } as unknown as Response);
    bindLoginEvents(vi.fn(), vi.fn());
    clickLogin();
    await new Promise((r) => setTimeout(r, 10));
    const errEl = document.getElementById('login-error')!;
    expect(errEl.textContent).toBe(expectedText);
    if (checkDisplay) expect(errEl.style.display).toBe('block');
  });

  it('shows network error toast on fetch throw', async () => {
    apiFetchMock.mockRejectedValue(new Error('offline'));
    const showToast = vi.fn();
    bindLoginEvents(vi.fn(), showToast);
    clickLogin();
    await new Promise((r) => setTimeout(r, 10));
    expect(showToast).toHaveBeenCalledWith('网络错误');
  });

  it('re-enables login button after failure', async () => {
    apiFetchMock.mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({}),
    } as Response);
    bindLoginEvents(vi.fn(), vi.fn());
    clickLogin();
    await new Promise((r) => setTimeout(r, 10));
    const btn = document.getElementById('login-btn') as HTMLButtonElement;
    expect(btn.disabled).toBe(false);
    expect(btn.textContent).toBe('登录');
  });

  it('triggers login on Enter key in password field', async () => {
    apiFetchMock.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({}),
    } as Response);
    const onSuccess = vi.fn();
    bindLoginEvents(onSuccess, vi.fn());
    pressEnter();
    await new Promise((r) => setTimeout(r, 10));
    expect(onSuccess).toHaveBeenCalledOnce();
  });

  it('sends POST with password and autoRefresh:false', async () => {
    apiFetchMock.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({}),
    } as Response);
    bindLoginEvents(vi.fn(), vi.fn());
    (document.getElementById('admin-password') as HTMLInputElement).value = 'pw123';
    clickLogin();
    await new Promise((r) => setTimeout(r, 10));
    const [url, opts] = apiFetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/admin/login');
    expect(opts).toMatchObject({ method: 'POST', autoRefresh: false });
    const body = JSON.parse(opts.body as string);
    expect(body).toEqual({ password: 'pw123' });
  });
});
