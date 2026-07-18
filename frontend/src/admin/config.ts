import { apiFetch } from '../shared/network/api_fetch.js';

export interface AdminConfig {
  emailEnabled: boolean;
  resendApiKey: string;
  emailFrom: string;
  adminPassword?: string;
}

export async function loadConfig(
  onUnauthorized: () => void,
  showToast: (msg: string) => void,
  applyConfig: (config: AdminConfig) => void,
): Promise<void> {
  try {
    const res: Response = await apiFetch('/api/v1/admin/config', { autoRefresh: false });
    if (res.status === 401) {
      onUnauthorized();
      return;
    }
    // shared-009: Handle non-401 errors (403, 500, etc.) instead of
    // blindly calling res.json() on an error response.
    if (!res.ok) {
      showToast('加载配置失败');
      return;
    }
    const config: AdminConfig = await res.json();
    applyConfig(config);
  } catch {
    showToast('加载配置失败', 'error');
  }
}

export async function saveConfig(
  config: AdminConfig,
  showToast: (msg: string) => void,
  onSaved: () => void,
): Promise<void> {
  try {
    const res: Response = await apiFetch('/api/v1/admin/config', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
      autoRefresh: false,
    });
    if (res.ok) {
      showToast('配置已保存');
      onSaved();
    } else {
      showToast('保存失败', 'error');
    }
  } catch {
    showToast('网络错误');
  }
}
