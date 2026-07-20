import { describe, it, expect, vi, beforeEach } from 'vitest';
import { getEntryMocks, resetEntryMocks } from './entry_flow_test_setup';
import { createStateJsMockModule } from './ws_handlers_test_setup.js';

const mockState = getEntryMocks().state;

vi.mock('./state.js', async (importActual) =>
  createStateJsMockModule(
    importActual as unknown as () => Promise<unknown>,
    getEntryMocks().state as unknown as Record<string, unknown>,
  ),
);

vi.mock('./renderer.js', () => ({
  $canvas: document.getElementById('game-canvas')! as HTMLCanvasElement,
}));

vi.mock('./ui_common.js', () => ({
  $lobbyCode: document.getElementById('lobby-code')!,
  $hudCode: document.getElementById('hud-code')!,
}));

vi.mock('./lobby_match.js', () => ({
  matchNewRoomCode: vi.fn().mockResolvedValue(null),
}));

import {
  resetEntryFlowForTest,
  setNicknameStatus,
  nicknameReadyStatus,
  setWaitingInlineError,
  showLoadingOverlay,
  updateWaitingStatusLine,
  syncEntryOverlays,
  setLobbyCodeDisplay,
  renderEntryFullScreenError,
} from './entry_flow.js';
import type { EntryOverlayContext } from './entry_flow.js';

describe('entry_flow_dom', () => {
  beforeEach(() => {
    resetEntryMocks();
    mockState.lobbyCode = 'ABC12';
    resetEntryFlowForTest();
  });

  it('updates the nickname connect status text', () => {
    const el = document.getElementById('nickname-connect-status')!;
    el.textContent = '';
    setNicknameStatus('test status');
    expect(el.textContent).toBe('test status');
  });

  it.each([
    [true, 'ABC12', '服务器已连接'],
    [false, 'ABC12', '已就绪'],
    [true, '', '-----'],
  ] as const)('nicknameReadyStatus wsConnected=%s lobbyCode=%s shows %s', (wsConnected, lobbyCode, expected) => {
    const el = document.getElementById('nickname-connect-status')!;
    nicknameReadyStatus(lobbyCode, wsConnected);
    expect(el.textContent).toContain(expected);
    if (lobbyCode) expect(el.textContent).toContain(lobbyCode);
  });

  it('setWaitingInlineError sets text and hides/shows the error element', () => {
    const el = document.getElementById('waiting-connect-error')!;
    el.classList.remove('hidden');
    el.textContent = 'old';
    setWaitingInlineError('连接超时');
    expect(el.textContent).toBe('连接超时');
    expect(el.classList.contains('hidden')).toBe(false);

    setWaitingInlineError('');
    expect(el.textContent).toBe('');
    expect(el.classList.contains('hidden')).toBe(true);
  });

  it('shows the loading overlay with default or custom message and hides error panel', () => {
    const overlay = document.getElementById('loading-overlay')!;
    const panel = document.getElementById('loading-error-panel')!;
    overlay.classList.add('hidden');
    panel.classList.remove('hidden');
    showLoadingOverlay();
    expect(overlay.classList.contains('hidden')).toBe(false);
    expect(panel.classList.contains('hidden')).toBe(true);
    expect(overlay.querySelector('.loading-text')!.textContent).toBe('正在连接房间…');

    showLoadingOverlay('自定义消息');
    expect(overlay.querySelector('.loading-text')!.textContent).toBe('自定义消息');
  });

  it.each([
    [false, '已加入等待大厅 · 正在连接服务器…'],
    [true, '正在等待其他玩家…'],
  ] as const)('updateWaitingStatusLine wsConnected=%s shows %s', (wsConnected, expected) => {
    const ctx: EntryOverlayContext = {
      entryStep: 'waiting',
      wsConnected,
      lobbyCode: 'ABC12',
      phase: 'waiting',
      getWaitingTitleText: () => expected,
    };
    updateWaitingStatusLine(ctx);
    expect(document.getElementById('waiting-title')!.textContent).toBe(expected);
  });

  it('setLobbyCodeDisplay updates both lobby code elements', () => {
    document.getElementById('lobby-code')!.textContent = '';
    document.getElementById('hud-code')!.textContent = '';
    setLobbyCodeDisplay('ROOMX');
    expect(document.getElementById('lobby-code')!.textContent).toBe('ROOMX');
    expect(document.getElementById('hud-code')!.textContent).toBe('ROOMX');
  });

  describe('renderEntryFullScreenError', () => {
    it('shows error panel with given message, hides spinner/reconnect banner, and uses custom title when provided', () => {
      const overlay = document.getElementById('loading-overlay')!;
      const banner = document.getElementById('reconnect-banner')!;
      overlay.classList.add('hidden');
      banner.classList.remove('hidden');
      renderEntryFullScreenError('连接超时，请重试');
      expect(overlay.classList.contains('hidden')).toBe(false);
      expect(overlay.dataset.error).toBe('true');
      expect(overlay.style.display).toBe('flex');
      expect(document.getElementById('loading-error-text')!.textContent).toBe('连接超时，请重试');
      expect(overlay.querySelector('.loading-spinner')!.classList.contains('hidden')).toBe(true);
      expect(banner.classList.contains('hidden')).toBe(true);
      renderEntryFullScreenError('连接超时', { title: '自定义标题' });
      expect(document.getElementById('loading-error-title')!.textContent).toBe('自定义标题');
    });

    it('clicking match button shows failure text when match returns null, navigates when match returns code', async () => {
      const { matchNewRoomCode } = await import('./lobby_match.js');
      vi.mocked(matchNewRoomCode).mockResolvedValue(null);
      renderEntryFullScreenError('匹配失败');
      document.getElementById('loading-match-btn')!.click();
      await new Promise((r) => setTimeout(r, 10));
      expect(document.getElementById('loading-error-text')!.textContent).toBe('匹配失败，请稍后重试或返回大厅');

      vi.mocked(matchNewRoomCode).mockResolvedValue('NEW12');
      vi.stubGlobal('location', { href: '' });
      renderEntryFullScreenError('匹配失败');
      document.getElementById('loading-match-btn')!.click();
      await new Promise((r) => setTimeout(r, 10));
      expect(window.location.href).toContain('NEW12');
      vi.unstubAllGlobals();
    });
  });

  describe('syncEntryOverlays', () => {
    it('syncEntryOverlays shows correct element for each step', () => {
      const showTarget = (step: 'connecting' | 'nickname' | 'waiting', targetId: string) => {
        const el = document.getElementById(targetId)!;
        el.classList.add('hidden');
        const ctx: EntryOverlayContext = {
          entryStep: step,
          wsConnected: false,
          lobbyCode: 'ABC12',
          phase: 'waiting',
          getWaitingTitleText: () => '',
        };
        syncEntryOverlays(ctx);
        expect(el.classList.contains('hidden')).toBe(false);
      };
      showTarget('connecting', 'loading-overlay');
      showTarget('nickname', 'nickname-setup-screen');
      showTarget('waiting', 'waiting-screen');
    });
  });
});
