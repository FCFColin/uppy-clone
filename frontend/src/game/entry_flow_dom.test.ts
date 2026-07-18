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

  describe('setNicknameStatus', () => {
    it('updates the nickname connect status text', () => {
      const el = document.getElementById('nickname-connect-status')!;
      el.textContent = '';
      setNicknameStatus('test status');
      expect(el.textContent).toBe('test status');
    });
  });

  describe('nicknameReadyStatus', () => {
    it('shows connected message when ws is connected', () => {
      const el = document.getElementById('nickname-connect-status')!;
      nicknameReadyStatus('ABC12', true);
      expect(el.textContent).toContain('服务器已连接');
      expect(el.textContent).toContain('ABC12');
    });

    it('shows ready message when ws is not connected', () => {
      const el = document.getElementById('nickname-connect-status')!;
      nicknameReadyStatus('ABC12', false);
      expect(el.textContent).toContain('已就绪');
      expect(el.textContent).toContain('ABC12');
    });

    it('uses fallback code when lobbyCode is empty', () => {
      const el = document.getElementById('nickname-connect-status')!;
      nicknameReadyStatus('', true);
      expect(el.textContent).toContain('-----');
    });
  });

  describe('setWaitingInlineError', () => {
    it('shows error text and removes hidden class', () => {
      const el = document.getElementById('waiting-connect-error')!;
      el.classList.add('hidden');
      setWaitingInlineError('连接超时');
      expect(el.textContent).toBe('连接超时');
      expect(el.classList.contains('hidden')).toBe(false);
    });

    it('clears error and adds hidden class when text is empty', () => {
      const el = document.getElementById('waiting-connect-error')!;
      el.classList.remove('hidden');
      el.textContent = 'old error';
      setWaitingInlineError('');
      expect(el.textContent).toBe('');
      expect(el.classList.contains('hidden')).toBe(true);
    });
  });

  describe('clearWaitingInlineError', () => {
    it('clears the inline error text', () => {
      const el = document.getElementById('waiting-connect-error')!;
      el.textContent = 'some error';
      el.classList.remove('hidden');
      clearWaitingInlineError();
      expect(el.textContent).toBe('');
      expect(el.classList.contains('hidden')).toBe(true);
    });
  });

  describe('showLoadingOverlay', () => {
    it('shows the loading overlay with default message', () => {
      const overlay = document.getElementById('loading-overlay')!;
      overlay.classList.add('hidden');
      overlay.style.display = 'none';
      showLoadingOverlay();
      expect(overlay.classList.contains('hidden')).toBe(false);
      expect(overlay.querySelector('.loading-text')!.textContent).toBe('正在连接房间…');
    });

    it('shows the loading overlay with custom message', () => {
      showLoadingOverlay('自定义消息');
      expect(document.getElementById('loading-overlay')!.querySelector('.loading-text')!.textContent).toBe('自定义消息');
    });

    it('hides the error panel when showing overlay', () => {
      const panel = document.getElementById('loading-error-panel')!;
      panel.classList.remove('hidden');
      showLoadingOverlay();
      expect(panel.classList.contains('hidden')).toBe(true);
    });
  });

  describe('hideLoadingOverlay', () => {
    it('hides the loading overlay', () => {
      const overlay = document.getElementById('loading-overlay')!;
      overlay.style.display = 'flex';
      hideLoadingOverlay();
      expect(overlay.classList.contains('hidden')).toBe(true);
    });
  });

  describe('updateWaitingStatusLine', () => {
    it('shows connecting message when ws not connected', () => {
      const ctx: EntryOverlayContext = {
        entryStep: 'waiting',
        wsConnected: false,
        lobbyCode: 'ABC12',
        phase: 'waiting',
        getWaitingTitleText: () => '等待中',
      };
      updateWaitingStatusLine(ctx);
      expect(document.getElementById('waiting-title')!.textContent).toBe('已加入等待大厅 · 正在连接服务器…');
    });

    it('shows waiting title text when ws is connected', () => {
      const ctx: EntryOverlayContext = {
        entryStep: 'waiting',
        wsConnected: true,
        lobbyCode: 'ABC12',
        phase: 'waiting',
        getWaitingTitleText: () => '正在等待其他玩家…',
      };
      updateWaitingStatusLine(ctx);
      expect(document.getElementById('waiting-title')!.textContent).toBe('正在等待其他玩家…');
    });
  });

  describe('renderStartCountdownTitle', () => {
    it('shows countdown text when remaining > 0', () => {
      renderStartCountdownTitle(5);
      expect(document.getElementById('waiting-title')!.textContent).toBe('即将开始 · 5…');
    });

    it('shows starting text when remaining <= 0', () => {
      renderStartCountdownTitle(0);
      expect(document.getElementById('waiting-title')!.textContent).toBe('正在开始…');
    });

    it('shows starting text when remaining negative', () => {
      renderStartCountdownTitle(-1);
      expect(document.getElementById('waiting-title')!.textContent).toBe('正在开始…');
    });
  });

  describe('setLobbyCodeDisplay', () => {
    it('updates both lobby code elements', () => {
      document.getElementById('lobby-code')!.textContent = '';
      document.getElementById('hud-code')!.textContent = '';
      setLobbyCodeDisplay('ROOMX');
      expect(document.getElementById('lobby-code')!.textContent).toBe('ROOMX');
      expect(document.getElementById('hud-code')!.textContent).toBe('ROOMX');
    });
  });

  describe('renderEntryFullScreenError', () => {
    it('shows error panel with given message', () => {
      const overlay = document.getElementById('loading-overlay')!;
      overlay.classList.add('hidden');
      renderEntryFullScreenError('连接超时，请重试');
      expect(overlay.classList.contains('hidden')).toBe(false);
      expect(overlay.dataset.error).toBe('true');
      expect(overlay.style.display).toBe('flex');
      expect(document.getElementById('loading-error-text')!.textContent).toBe('连接超时，请重试');
      expect(overlay.querySelector('.loading-spinner')!.classList.contains('hidden')).toBe(true);
    });

    it('shows derived title based on message content', () => {
      renderEntryFullScreenError('房间已结束');
      expect(document.getElementById('loading-error-title')!.textContent).toBe('房间已结束');
    });

    it('shows mid-game disconnect title', () => {
      renderEntryFullScreenError('连接中断', { midGameDisconnect: true });
      expect(document.getElementById('loading-error-title')!.textContent).toBe('对局连接中断');
    });

    it('shows connection failure title for network messages', () => {
      renderEntryFullScreenError('网络连接超时');
      expect(document.getElementById('loading-error-title')!.textContent).toBe('连接失败');
    });

    it('shows default title for unknown messages', () => {
      renderEntryFullScreenError('未知错误');
      expect(document.getElementById('loading-error-title')!.textContent).toBe('无法进入房间');
    });

    it('hides actions when showActions is false', () => {
      renderEntryFullScreenError('错误', { showActions: false });
      expect(document.getElementById('loading-error-actions')!.classList.contains('hidden')).toBe(true);
    });

    it('shows actions when showActions is true', () => {
      renderEntryFullScreenError('错误', { showActions: true });
      expect(document.getElementById('loading-error-actions')!.classList.contains('hidden')).toBe(false);
    });

    it('hides reconnect banner when showing error', () => {
      const banner = document.getElementById('reconnect-banner')!;
      banner.classList.remove('hidden');
      renderEntryFullScreenError('错误');
      expect(banner.classList.contains('hidden')).toBe(true);
    });

    it('uses custom title when provided', () => {
      renderEntryFullScreenError('连接超时', { title: '自定义标题' });
      expect(document.getElementById('loading-error-title')!.textContent).toBe('自定义标题');
    });

    it('clicking match button shows failure text when match returns null', async () => {
      const { matchNewRoomCode } = await import('./lobby_match.js');
      vi.mocked(matchNewRoomCode).mockResolvedValue(null);
      renderEntryFullScreenError('匹配失败');
      const matchBtn = document.getElementById('loading-match-btn')!;
      matchBtn.click();
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
    it('shows loading overlay for connecting step', () => {
      const ctx: EntryOverlayContext = {
        entryStep: 'connecting',
        wsConnected: false,
        lobbyCode: '',
        phase: 'waiting',
        getWaitingTitleText: () => '',
      };
      const overlay = document.getElementById('loading-overlay')!;
      overlay.classList.add('hidden');
      syncEntryOverlays(ctx);
      expect(overlay.classList.contains('hidden')).toBe(false);
    });

    it('does not hide loading overlay for error step', () => {
      const ctx: EntryOverlayContext = {
        entryStep: 'error',
        wsConnected: false,
        lobbyCode: '',
        phase: 'waiting',
        getWaitingTitleText: () => '',
      };
      const overlay = document.getElementById('loading-overlay')!;
      overlay.style.display = 'flex';
      syncEntryOverlays(ctx);
      expect(overlay.style.display).toBe('flex');
    });

    it('shows nickname screen for nickname step', () => {
      const nicknameScreen = document.getElementById('nickname-setup-screen')!;
      nicknameScreen.classList.add('hidden');
      const ctx: EntryOverlayContext = {
        entryStep: 'nickname',
        wsConnected: true,
        lobbyCode: 'ABC12',
        phase: 'waiting',
        getWaitingTitleText: () => '',
      };
      syncEntryOverlays(ctx);
      expect(nicknameScreen.classList.contains('hidden')).toBe(false);
    });

    it('shows waiting screen for waiting step', () => {
      const waitingScreen = document.getElementById('waiting-screen')!;
      waitingScreen.classList.add('hidden');
      const ctx: EntryOverlayContext = {
        entryStep: 'waiting',
        wsConnected: true,
        lobbyCode: 'ABC12',
        phase: 'waiting',
        getWaitingTitleText: () => '等待中',
      };
      syncEntryOverlays(ctx);
      expect(waitingScreen.classList.contains('hidden')).toBe(false);
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