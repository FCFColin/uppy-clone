import { describe, it, expect, vi, beforeEach } from 'vitest';
import { getEntryMocks, resetEntryMocks } from './entry_flow_test_setup';
import { createStateJsMockModule } from './ws_handlers_test_setup.js';

vi.hoisted(() => {
  document.body.innerHTML = `
    <div id="loading-overlay">
      <div class="loading-spinner"></div>
      <div class="loading-text"></div>
    </div>
    <div id="loading-error-panel" class="hidden"></div>
    <div id="loading-error-text"></div>
    <div id="loading-error-title"></div>
    <div id="loading-error-actions" class="hidden"></div>
    <div id="waiting-title"></div>
    <div id="nickname-connect-status"></div>
    <div id="waiting-connect-error" class="hidden"></div>
    <div id="nickname-setup-screen"></div>
    <div id="waiting-screen"></div>
    <div id="reconnect-banner" class="hidden"></div>
    <div id="lobby-code"></div>
    <div id="hud-code"></div>
    <div id="loading-back-btn"></div>
    <div id="loading-match-btn"></div>
    <div id="game-canvas" style="pointer-events: none;"></div>
    <div id="game-hud" class="hidden"></div>
  `;
});

const mockState = getEntryMocks().state;

vi.mock('./state.js', async (importActual) => createStateJsMockModule(importActual as any, getEntryMocks().state));

vi.mock('./renderer.js', () => ({
  $canvas: document.getElementById('game-canvas')! as HTMLCanvasElement,
}));

vi.mock('./ui_elements.js', () => ({
  $lobbyCode: document.getElementById('lobby-code')!,
  $hudCode: document.getElementById('hud-code')!,
}));

vi.mock('./lobby_match.js', () => ({
  matchNewRoomCode: vi.fn().mockResolvedValue(null),
}));

import {
  setNicknameStatus,
  nicknameReadyStatus,
  setWaitingInlineError,
  clearWaitingInlineError,
  showLoadingOverlay,
  hideLoadingOverlay,
  updateWaitingStatusLine,
  syncEntryOverlays,
  setLobbyCodeDisplay,
  renderEntryFullScreenError,
  renderStartCountdownTitle,
} from './entry_flow_ui.js';
import { resetEntryFlowForTest } from './entry_flow.js';
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

  it.each([
    ['连接超时', false, '连接超时'],
    ['', true, ''],
  ] as const)('setWaitingInlineError text=%s hides=%s', (text, expectedHidden, expectedText) => {
    const el = document.getElementById('waiting-connect-error')!;
    el.classList.remove('hidden');
    el.textContent = 'old';
    setWaitingInlineError(text);
    expect(el.textContent).toBe(expectedText);
    expect(el.classList.contains('hidden')).toBe(expectedHidden);
  });

  it('clearWaitingInlineError clears the inline error text', () => {
    const el = document.getElementById('waiting-connect-error')!;
    el.textContent = 'some error';
    el.classList.remove('hidden');
    clearWaitingInlineError();
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

  it('hides the loading overlay', () => {
    const overlay = document.getElementById('loading-overlay')!;
    overlay.style.display = 'flex';
    hideLoadingOverlay();
    expect(overlay.classList.contains('hidden')).toBe(true);
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

  it.each([
    [5, '即将开始 · 5…'],
    [0, '正在开始…'],
    [-1, '正在开始…'],
  ] as const)('renderStartCountdownTitle remaining=%s shows %s', (remaining, expected) => {
    renderStartCountdownTitle(remaining);
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
    it('shows error panel with given message and hides spinner/reconnect banner', () => {
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
    });

    it.each([
      ['房间已结束', undefined, '房间已结束'],
      ['连接中断', { midGameDisconnect: true }, '对局连接中断'],
      ['网络连接超时', undefined, '连接失败'],
      ['未知错误', undefined, '无法进入房间'],
    ] as const)('message %s shows title %s', (message, opts, expectedTitle) => {
      renderEntryFullScreenError(message, opts);
      expect(document.getElementById('loading-error-title')!.textContent).toBe(expectedTitle);
    });

    it.each([
      [false, true],
      [true, false],
    ] as const)('showActions=%s hides actions=%s', (showActions, expectedHidden) => {
      renderEntryFullScreenError('错误', { showActions });
      expect(document.getElementById('loading-error-actions')!.classList.contains('hidden')).toBe(expectedHidden);
    });

    it('uses custom title when provided', () => {
      renderEntryFullScreenError('连接超时', { title: '自定义标题' });
      expect(document.getElementById('loading-error-title')!.textContent).toBe('自定义标题');
    });

    it('clicking match button shows failure text when match returns null', async () => {
      const { matchNewRoomCode } = await import('./lobby_match.js');
      vi.mocked(matchNewRoomCode).mockResolvedValue(null);
      renderEntryFullScreenError('匹配失败');
      document.getElementById('loading-match-btn')!.click();
      await new Promise((r) => setTimeout(r, 10));
      expect(document.getElementById('loading-error-text')!.textContent).toBe('匹配失败，请稍后重试或返回大厅');
    });

    it('clicking match button navigates when match returns code', async () => {
      const { matchNewRoomCode } = await import('./lobby_match.js');
      vi.mocked(matchNewRoomCode).mockResolvedValue('NEW12');
      const originalHref = window.location.href;
      Object.defineProperty(window, 'location', { value: { href: '' }, writable: true });
      renderEntryFullScreenError('匹配失败');
      document.getElementById('loading-match-btn')!.click();
      await new Promise((r) => setTimeout(r, 10));
      expect(window.location.href).toContain('NEW12');
      Object.defineProperty(window, 'location', { value: { href: originalHref }, writable: true });
    });
  });

  describe('syncEntryOverlays', () => {
    it.each([
      ['connecting' as const, false, 'loading-overlay'],
      ['nickname' as const, true, 'nickname-setup-screen'],
      ['waiting' as const, true, 'waiting-screen'],
    ])('step=%s shows target element', (step, wsConnected, targetId) => {
      const target = document.getElementById(targetId)!;
      target.classList.add('hidden');
      const ctx: EntryOverlayContext = {
        entryStep: step,
        wsConnected,
        lobbyCode: 'ABC12',
        phase: 'waiting',
        getWaitingTitleText: () => '',
      };
      syncEntryOverlays(ctx);
      expect(target.classList.contains('hidden')).toBe(false);
    });

    it('does not hide loading overlay for error step', () => {
      const overlay = document.getElementById('loading-overlay')!;
      overlay.style.display = 'flex';
      const ctx: EntryOverlayContext = {
        entryStep: 'error',
        wsConnected: false,
        lobbyCode: '',
        phase: 'waiting',
        getWaitingTitleText: () => '',
      };
      syncEntryOverlays(ctx);
      expect(overlay.style.display).toBe('flex');
    });

    it('hides entry overlays on handoff step', () => {
      const nicknameScreen = document.getElementById('nickname-setup-screen')!;
      const waitingScreen = document.getElementById('waiting-screen')!;
      nicknameScreen.classList.remove('hidden');
      waitingScreen.classList.remove('hidden');
      const ctx: EntryOverlayContext = {
        entryStep: 'handoff',
        wsConnected: true,
        lobbyCode: 'ABC12',
        phase: 'playing',
        getWaitingTitleText: () => '',
      };
      syncEntryOverlays(ctx);
      expect(nicknameScreen.classList.contains('hidden')).toBe(true);
      expect(waitingScreen.classList.contains('hidden')).toBe(true);
    });
  });
});
