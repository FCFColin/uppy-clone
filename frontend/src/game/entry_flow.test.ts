import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('./renderer.js', () => ({
  $canvas: document.getElementById('game-canvas') as HTMLCanvasElement,
}));

vi.mock('./ui_common.js', () => ({
  $lobbyCode: document.getElementById('lobby-code')!,
  $hudCode: document.getElementById('hud-code')!,
}));

vi.mock('./lobby_match.js', () => ({
  matchNewRoomCode: vi.fn().mockResolvedValue(null),
}));

const mockSafeSetItem = vi.hoisted(() => vi.fn());
vi.mock('../shared/ui/utils.js', () => ({
  safeSetItem: mockSafeSetItem,
}));

const mockRunTutorialIfNeeded = vi.hoisted(() => vi.fn(() => Promise.resolve()));
vi.mock('./tutorial.js', () => ({
  runTutorialIfNeeded: mockRunTutorialIfNeeded,
}));

import './entry_flow_test_setup';
import { state } from './state.js';
import {
  applyEntryStep,
  getEntryStep,
  isEntryHandoff,
  onLobbyCodeReady,
  onNicknameSubmit,
  onWebSocketOpen,
  onWebSocketClosed,
  tryEntryHandoff,
  resetEntryFlowForTest,
  initEntryFlow,
  bindEntryUI,
  showEntryFullScreenError,
  routeConnectionError,
  clearStartCountdown,
} from './entry_flow.js';
import { clearWaitingInlineError, getWaitingTitleText } from './entry_flow.js';

describe('entry_flow', () => {
  beforeEach(() => {
    resetEntryFlowForTest();
    // Replace the form element to drop event listeners bound by previous
    // bindEntryUI() calls — resetEntryFlowForTest only resets the entryUiBound
    // guard, not the DOM listeners already attached to the persistent form.
    const oldForm = document.getElementById('nickname-entry-form');
    if (oldForm) {
      const freshForm = document.createElement('form');
      freshForm.id = 'nickname-entry-form';
      oldForm.replaceWith(freshForm);
    }
    state.lobbyCode = '';
    state.nicknameSubmitted = false;
    state.phase = 'waiting';
    state.players = [];
    mockSafeSetItem.mockClear();
    mockRunTutorialIfNeeded.mockClear();
    mockRunTutorialIfNeeded.mockImplementation(() => Promise.resolve());
    initEntryFlow();
  });

  it('starts at connecting without URL code', () => {
    expect(getEntryStep()).toBe('connecting');
  });

  it('initEntryFlow skips connecting when URL has code', () => {
    vi.stubGlobal('location', { ...window.location, search: '?code=QUICK' });
    resetEntryFlowForTest();
    initEntryFlow();
    expect(getEntryStep()).toBe('nickname');
    expect(state.lobbyCode).toBe('QUICK');
    vi.unstubAllGlobals();
  });

  it('onLobbyCodeReady advances to nickname', () => {
    onLobbyCodeReady('ABC12');
    expect(getEntryStep()).toBe('nickname');
    expect(state.lobbyCode).toBe('ABC12');
  });

  it('onNicknameSubmit advances to waiting, no-op when already past, cannot regress via applyEntryStep', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    expect(getEntryStep()).toBe('waiting');
    expect(document.getElementById('waiting-screen')!.classList.contains('hidden')).toBe(false);
    onNicknameSubmit();
    expect(getEntryStep()).toBe('waiting');
    applyEntryStep('nickname');
    expect(getEntryStep()).toBe('waiting');
  });

  it('tryEntryHandoff moves to handoff on countdown after nickname submit, no-op before', () => {
    // No-op before nickname submit
    onLobbyCodeReady('ABC12');
    tryEntryHandoff('playing');
    expect(getEntryStep()).toBe('nickname');
    expect(isEntryHandoff()).toBe(false);
    // Moves to handoff after nickname submit
    onNicknameSubmit();
    state.nicknameSubmitted = true;
    tryEntryHandoff('countdown');
    expect(getEntryStep()).toBe('handoff');
  });

  it('tryEntryHandoff advances to handoff on ended after nickname submit', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    state.nicknameSubmitted = true;
    state.phase = 'ended';
    tryEntryHandoff('ended');
    expect(getEntryStep()).toBe('handoff');
    expect(isEntryHandoff()).toBe(true);
    // #waiting-screen must be hidden and stripped of entry-overlay-active so it
    // does not block the #ended-screen "查看排行榜" button (z-index 10100 vs 10000).
    const $waiting = document.getElementById('waiting-screen')!;
    expect($waiting.classList.contains('hidden')).toBe(true);
    expect($waiting.classList.contains('entry-overlay-active')).toBe(false);
  });

  it('onWebSocketOpen/Closed updates nickname status when on nickname step', () => {
    onLobbyCodeReady('ABC12');
    onWebSocketOpen();
    expect(document.getElementById('nickname-connect-status')!.textContent).toContain('服务器已连接');
    expect(document.getElementById('nickname-connect-status')!.textContent).toContain('进入游戏');
    onWebSocketClosed();
    expect(document.getElementById('nickname-connect-status')!.textContent).toContain('连接已断开');
  });

  it('bindEntryUI form submit triggers callback on nickname step, ignores when past', async () => {
    onLobbyCodeReady('ABC12');
    const onSubmit = vi.fn();
    bindEntryUI(onSubmit);
    document
      .getElementById('nickname-entry-form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    expect(mockRunTutorialIfNeeded).toHaveBeenCalledOnce();
    await vi.waitFor(() => {
      expect(onSubmit).toHaveBeenCalledOnce();
    });
    // Past nickname step: ignored (tutorial + onSubmit both skipped)
    onNicknameSubmit();
    mockRunTutorialIfNeeded.mockClear();
    document
      .getElementById('nickname-entry-form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await Promise.resolve();
    expect(mockRunTutorialIfNeeded).not.toHaveBeenCalled();
    expect(onSubmit).toHaveBeenCalledOnce();
  });

  it('bindEntryUI runs tutorial before submitting nickname for new players', async () => {
    let resolveTutorial: () => void = () => {};
    mockRunTutorialIfNeeded.mockImplementation(
      () => new Promise<void>((resolve) => {
        resolveTutorial = resolve;
      }),
    );

    onLobbyCodeReady('ABC12');
    const onSubmit = vi.fn();
    bindEntryUI(onSubmit);
    document
      .getElementById('nickname-entry-form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));

    // Tutorial shown, onSubmit not yet called
    expect(mockRunTutorialIfNeeded).toHaveBeenCalledOnce();
    await Promise.resolve();
    expect(onSubmit).not.toHaveBeenCalled();

    // Tutorial completes → onSubmit fires
    resolveTutorial();
    await vi.waitFor(() => {
      expect(onSubmit).toHaveBeenCalledOnce();
    });
  });

  it('bindEntryUI skips tutorial and submits immediately for returning players', async () => {
    // runTutorialIfNeeded resolves immediately (default mock) — simulates cookie set
    onLobbyCodeReady('ABC12');
    const onSubmit = vi.fn();
    bindEntryUI(onSubmit);
    document
      .getElementById('nickname-entry-form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    expect(mockRunTutorialIfNeeded).toHaveBeenCalledOnce();
    await vi.waitFor(() => {
      expect(onSubmit).toHaveBeenCalledOnce();
    });
  });

  it('getWaitingTitleText reflects player count', () => {
    state.players = [{ playerIndex: 1, cooldownEndTime: 0, palette: 0, scoreContribution: 0, nickname: 'A' }];
    expect(getWaitingTitleText()).toBe('即将开始…');
    state.players.push({ playerIndex: 2, cooldownEndTime: 0, palette: 0, scoreContribution: 0, nickname: 'B' });
    expect(getWaitingTitleText()).toBe('等待其他玩家确认昵称…');
  });

  it('onWebSocketOpen preserves countdown, onWebSocketClosed updates waiting title when on waiting step', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    expect(document.getElementById('waiting-title')!.textContent).toMatch(/即将开始 · \d…/);
    onWebSocketOpen();
    expect(document.getElementById('waiting-title')!.textContent).toMatch(/即将开始 · \d…/);
    clearStartCountdown();
    onWebSocketOpen();
    onWebSocketClosed();
    expect(document.getElementById('waiting-title')!.textContent).toContain('正在连接服务器');
  });

  it('countdown timer clears itself after reaching zero', async () => {
    vi.useFakeTimers();
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    vi.advanceTimersByTime(3000);
    expect(document.getElementById('waiting-title')!.textContent).toBe('正在开始…');
    vi.useRealTimers();
  });

  it('routeConnectionError routes to nickname, waiting inline, or full-screen error', () => {
    onLobbyCodeReady('ABC12');
    routeConnectionError('昵称阶段错误');
    expect(document.getElementById('nickname-connect-status')!.textContent).toBe('昵称阶段错误');

    onNicknameSubmit();
    routeConnectionError('等待阶段错误');
    expect(document.getElementById('waiting-connect-error')!.textContent).toBe('等待阶段错误');
    expect(document.getElementById('waiting-connect-error')!.classList.contains('hidden')).toBe(false);

    clearWaitingInlineError();
    expect(document.getElementById('waiting-connect-error')!.classList.contains('hidden')).toBe(true);

    resetEntryFlowForTest();
    initEntryFlow();
    routeConnectionError('连接超时，请重试', { showActions: true, midGameDisconnect: true });
    expect(getEntryStep()).toBe('error');
    expect(document.getElementById('loading-error-title')!.textContent).toBe('对局连接中断');

    resetEntryFlowForTest();
    initEntryFlow();
    routeConnectionError('房间不存在');
    expect(document.getElementById('loading-error-title')!.textContent).toBe('无法进入房间');

    resetEntryFlowForTest();
    initEntryFlow();
    routeConnectionError('网络异常');
    expect(document.getElementById('loading-error-title')!.textContent).toBe('连接失败');

    resetEntryFlowForTest();
    initEntryFlow();
    routeConnectionError('未知错误');
    expect(document.getElementById('loading-error-title')!.textContent).toBe('无法进入房间');
  });

  it('routeConnectionError from connecting step without options shows full-screen error and strips entry-overlay-active', () => {
    // 回归：connectWebSocket 中 establishGameSession 失败时调用 showConnectionError(message)
    // 不带 options。此时 entryStep='connecting'，应显示全屏错误面板，且
    // nickname-setup-screen/waiting-screen 不能带 entry-overlay-active 盖住错误面板。
    resetEntryFlowForTest();
    initEntryFlow();
    expect(getEntryStep()).toBe('connecting');

    // 模拟之前可能残留的 entry-overlay-active（防御性测试）
    document.getElementById('nickname-setup-screen')!.classList.add('entry-overlay-active');
    document.getElementById('waiting-screen')!.classList.add('entry-overlay-active');

    routeConnectionError('连接失败，请检查网络后重试');

    expect(getEntryStep()).toBe('error');
    expect(document.getElementById('loading-error-text')!.textContent).toBe('连接失败，请检查网络后重试');
    expect(document.getElementById('nickname-setup-screen')!.classList.contains('entry-overlay-active')).toBe(false);
    expect(document.getElementById('waiting-screen')!.classList.contains('entry-overlay-active')).toBe(false);
  });

  it('showEntryFullScreenError maps message titles and binds retry actions', async () => {
    const { matchNewRoomCode } = await import('./lobby_match.js');
    vi.mocked(matchNewRoomCode).mockResolvedValue('NEW99');
    vi.stubGlobal('location', { href: '' });

    showEntryFullScreenError('房间不存在', { showActions: true });
    expect(document.getElementById('loading-error-title')!.textContent).toBe('无法进入房间');

    showEntryFullScreenError('房间已结束', { showActions: true });
    expect(document.getElementById('loading-error-title')!.textContent).toBe('房间已结束');

    document.getElementById('loading-back-btn')!.click();
    expect(window.location.href).toBe('/');

    document.getElementById('loading-match-btn')!.click();
    await vi.waitFor(() => {
      expect(window.location.href).toContain('NEW99');
    });

    resetEntryFlowForTest();
    initEntryFlow();
    vi.mocked(matchNewRoomCode).mockResolvedValue(null);
    showEntryFullScreenError('匹配失败', { showActions: true });
    document.getElementById('loading-match-btn')!.click();
    await vi.waitFor(() => {
      expect(document.getElementById('loading-error-text')!.textContent).toContain('匹配失败');
    });
    vi.unstubAllGlobals();
  });

  it('onLobbyCodeReady ignores republication while connecting after publish', () => {
    onLobbyCodeReady('FIRST');
    resetEntryFlowForTest();
    initEntryFlow();
    onLobbyCodeReady('FIRST');
    onLobbyCodeReady('SECOND');
    expect(state.lobbyCode).toBe('FIRST');
  });

  it('applyEntryStep handoff hides entry overlays and enables canvas pointer events', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    state.phase = 'playing';
    applyEntryStep('handoff');
    expect(isEntryHandoff()).toBe(true);
    expect(document.getElementById('nickname-setup-screen')!.classList.contains('hidden')).toBe(true);
    expect(document.getElementById('game-canvas')!.style.pointerEvents).toBe('auto');
  });

  it('applyEntryStep can advance to error from nickname and waiting — error is terminal', () => {
    // 回归：error 是终止态，canAdvanceTo 总允许推进。
    // 否则 applyEntryStep('error') 不 dispatch、不 syncOverlays、不触发 ensureEntryOverlayOnTop，
    // 错误面板会被 nickname-setup-screen/waiting-screen 的 entry-overlay-active 盖住。

    // 从 nickname 推进到 error
    onLobbyCodeReady('ABC12');
    expect(getEntryStep()).toBe('nickname');
    applyEntryStep('error');
    expect(getEntryStep()).toBe('error');
    expect(document.getElementById('nickname-setup-screen')!.classList.contains('entry-overlay-active')).toBe(false);
    expect(document.getElementById('waiting-screen')!.classList.contains('entry-overlay-active')).toBe(false);

    // 从 waiting 推进到 error
    resetEntryFlowForTest();
    initEntryFlow();
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    expect(getEntryStep()).toBe('waiting');
    applyEntryStep('error');
    expect(getEntryStep()).toBe('error');
    expect(document.getElementById('nickname-setup-screen')!.classList.contains('entry-overlay-active')).toBe(false);
    expect(document.getElementById('waiting-screen')!.classList.contains('entry-overlay-active')).toBe(false);
  });

  it('onLobbyCodeReady is no-op in waiting or handoff steps', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    onLobbyCodeReady('OTHER');
    expect(state.lobbyCode).toBe('ABC12');

    applyEntryStep('handoff');
    onLobbyCodeReady('HAND');
    expect(state.lobbyCode).toBe('ABC12');
  });
});
