import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock apiFetch so loadConfig/saveConfig network calls are controllable.
const apiFetchMock = vi.hoisted(() => vi.fn());
vi.mock('../shared/network/api_fetch.js', () => ({
  apiFetch: apiFetchMock,
}));

import { loadConfig, saveConfig, type AdminConfig } from './config.js';

describe('admin/config', () => {
  beforeEach(() => {
    apiFetchMock.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('loadConfig', () => {
    it('calls onUnauthorized when status is 401', async () => {
      apiFetchMock.mockResolvedValue({ ok: false, status: 401 } as Response);
      const onUnauthorized = vi.fn();
      const showToast = vi.fn();
      const applyConfig = vi.fn();
      await loadConfig(onUnauthorized, showToast, applyConfig);
      expect(onUnauthorized).toHaveBeenCalledOnce();
      expect(applyConfig).not.toHaveBeenCalled();
      expect(showToast).not.toHaveBeenCalled();
    });

    it('shows toast on non-401 error', async () => {
      apiFetchMock.mockResolvedValue({ ok: false, status: 500 } as Response);
      const onUnauthorized = vi.fn();
      const showToast = vi.fn();
      const applyConfig = vi.fn();
      await loadConfig(onUnauthorized, showToast, applyConfig);
      expect(showToast).toHaveBeenCalledWith('加载配置失败', 'error');
      expect(applyConfig).not.toHaveBeenCalled();
    });

    it('applies config on success', async () => {
      const config: AdminConfig = {
        emailEnabled: true,
        resendApiKey: 'key',
        emailFrom: 'a@b.com',
      };
      apiFetchMock.mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => config,
      } as Response);
      const onUnauthorized = vi.fn();
      const showToast = vi.fn();
      const applyConfig = vi.fn();
      await loadConfig(onUnauthorized, showToast, applyConfig);
      expect(applyConfig).toHaveBeenCalledWith(config);
    });

    it('shows toast on network error', async () => {
      apiFetchMock.mockRejectedValue(new Error('offline'));
      const onUnauthorized = vi.fn();
      const showToast = vi.fn();
      const applyConfig = vi.fn();
      await loadConfig(onUnauthorized, showToast, applyConfig);
      expect(showToast).toHaveBeenCalledWith('加载配置失败', 'error');
    });

    it('passes autoRefresh:false to apiFetch', async () => {
      apiFetchMock.mockResolvedValue({ ok: false, status: 401 } as Response);
      await loadConfig(vi.fn(), vi.fn(), vi.fn());
      const opts = apiFetchMock.mock.calls[0]![1];
      expect(opts).toMatchObject({ autoRefresh: false });
    });
  });

  describe('saveConfig', () => {
    it('shows success toast and calls onSaved on ok', async () => {
      apiFetchMock.mockResolvedValue({ ok: true, status: 200 } as Response);
      const showToast = vi.fn();
      const onSaved = vi.fn();
      const config: AdminConfig = {
        emailEnabled: false,
        resendApiKey: '',
        emailFrom: '',
      };
      await saveConfig(config, showToast, onSaved);
      expect(showToast).toHaveBeenCalledWith('配置已保存', 'success');
      expect(onSaved).toHaveBeenCalledOnce();
    });

    it('shows error toast on non-ok', async () => {
      apiFetchMock.mockResolvedValue({ ok: false, status: 400 } as Response);
      const showToast = vi.fn();
      const onSaved = vi.fn();
      await saveConfig(
        { emailEnabled: false, resendApiKey: '', emailFrom: '' },
        showToast,
        onSaved,
      );
      expect(showToast).toHaveBeenCalledWith('保存失败', 'error');
      expect(onSaved).not.toHaveBeenCalled();
    });

    it('shows error toast on network error', async () => {
      apiFetchMock.mockRejectedValue(new Error('offline'));
      const showToast = vi.fn();
      const onSaved = vi.fn();
      await saveConfig(
        { emailEnabled: false, resendApiKey: '', emailFrom: '' },
        showToast,
        onSaved,
      );
      expect(showToast).toHaveBeenCalledWith('网络错误', 'error');
    });

    it('sends PATCH with JSON body and autoRefresh:false', async () => {
      apiFetchMock.mockResolvedValue({ ok: true, status: 200 } as Response);
      const config: AdminConfig = {
        emailEnabled: true,
        resendApiKey: 'rk',
        emailFrom: 'x@y.com',
        adminPassword: 'pw',
      };
      await saveConfig(config, vi.fn(), vi.fn());
      const [url, opts] = apiFetchMock.mock.calls[0]!;
      expect(url).toBe('/api/v1/admin/config');
      expect(opts).toMatchObject({
        method: 'PATCH',
        autoRefresh: false,
      });
      const body = JSON.parse(opts.body as string);
      expect(body).toEqual(config);
    });
  });
});
