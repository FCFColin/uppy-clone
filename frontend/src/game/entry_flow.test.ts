import { describe, it, expect, beforeEach, vi } from 'vitest';

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
  const form = document.createElement('form');
  form.id = 'nickname-entry-form';
  document.body.appendChild(form);
});

vi.mock('./renderer.js', () => ({
  $canvas: document.getElementById('game-canvas') as HTMLCanvasElement,
}));

vi.mock('./ui_common.js', () => ({
  $lobbyCode: document.getElementById('lobby-code')!,
  $hudCode: document.getElementById('hud-code')!,
}));

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
    state.lobbyCode = '';
    state.nicknameSubmitted = false;
    state.phase = 'waiting';
    state.players = [];
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

  it('onWebSocketOpen/Closed updates nickname status when on nickname step', () => {
    onLobbyCodeReady('ABC12');
    onWebSocketOpen();
    expect(document.getElementById('nickname-connect-status')!.textContent).toContain('服务器已连接');
    expect(document.getElementById('nickname-connect-status')!.textContent).toContain('进入游戏');
    onWebSocketClosed();
    expect(document.getElementById('nickname-connect-status')!.textContent).toContain('连接已断开');
  });

  it('bindEntryUI form submit triggers callback on nickname step, ignores when past', () => {
    onLobbyCodeReady('ABC12');
    const onSubmit = vi.fn();
    bindEntryUI(onSubmit);
    document.getElementById('nickname-entry-form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    expect(onSubmit).toHaveBeenCalledOnce();
    // Past nickname step: ignored
    onNicknameSubmit();
    document.getElementById('nickname-entry-form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    expect(onSubmit).toHaveBeenCalledOnce();
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

  it('showEntryFullScreenError maps message titles and binds retry actions', async () => {
    const replace = vi.fn();
    vi.stubGlobal('location', { ...window.location, href: '', replace });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ lobbyCode: 'NEW99' }), { status: 200 })));

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
    vi.mocked(fetch).mockResolvedValueOnce(new Response('', { status: 500 }));
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