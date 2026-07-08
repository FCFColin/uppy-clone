export interface AdminConfig {
  emailEnabled: boolean;
  resendApiKey: string;
  emailFrom: string;
  adminPassword?: string;
}

export async function loadConfig(
  onUnauthorized: () => void,
  showToast: (msg: string, type: 'success' | 'error') => void,
  applyConfig: (config: AdminConfig) => void,
): Promise<void> {
  try {
    const res: Response = await fetch('/api/v1/admin/config', { credentials: 'include' });
    if (res.status === 401) {
      onUnauthorized();
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
  showToast: (msg: string, type: 'success' | 'error') => void,
  onSaved: () => void,
): Promise<void> {
  try {
    const res: Response = await fetch('/api/v1/admin/config', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(config),
    });
    if (res.ok) {
      showToast('配置已保存', 'success');
      onSaved();
    } else {
      showToast('保存失败', 'error');
    }
  } catch {
    showToast('网络错误', 'error');
  }
}
