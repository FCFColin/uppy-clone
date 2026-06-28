import { describe, it, expect, beforeEach, vi } from 'vitest';

const dom = vi.hoisted(() => {
  const ids = [
    'loading-overlay', 'nickname-setup-screen', 'waiting-screen',
    'nickname-connect-status', 'waiting-title', 'waiting-connect-error',
    'loading-error-panel', 'loading-error-title', 'loading-error-text',
    'loading-error-actions', 'loading-back-btn', 'loading-match-btn',
    'reconnect-banner', 'lobby-code', 'hud-code', 'game-canvas',
  ];
  const elements = new Map<string, HTMLElement>();
  for (const id of ids) {
    const el = document.createElement('div');
    el.id = id;
    el.className = 'hidden';
    if (id === 'loading-overlay') {
      el.innerHTML = '<div class="loading-spinner"></div><div class="loading-text"></div>';
    }
    elements.set(id, el);
    document.body.appendChild(el);
  }
  const form = document.createElement('form');
  form.id = 'nickname-entry-form';
  document.body.appendChild(form);
  const canvas = document.createElement('canvas');
  canvas.id = 'game-canvas';
  document.body.appendChild(canvas);
  return { elements, form };
});

vi.mock('./renderer_canvas.js', () => ({
  $canvas: document.getElementById('game-canvas') as HTMLCanvasElement,
}));

vi.mock('./ui_elements.js', () => ({
  $lobbyCode: dom.elements.get('lobby-code')!,
  $hudCode: dom.elements.get('hud-code')!,
}));

import { state } from './state.js';
import {
  applyEntryStep,
  getEntryStep,
  getWaitingTitleText,
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
  onConnectError,
  clearWaitingInlineError,
  clearStartCountdownForTest,
} from './entry_flow.js';

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

  it('onLobbyCodeReady is no-op after nickname submit', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    onLobbyCodeReady('OTHER');
    expect(getEntryStep()).toBe('waiting');
    expect(state.lobbyCode).toBe('ABC12');
  });

  it('onNicknameSubmit advances to waiting', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    expect(getEntryStep()).toBe('waiting');
    expect(dom.elements.get('waiting-screen')!.classList.contains('hidden')).toBe(false);
  });

  it('onNicknameSubmit is no-op when not on nickname step', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    onNicknameSubmit();
    expect(getEntryStep()).toBe('waiting');
  });

  it('cannot regress from waiting to nickname via applyEntryStep', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    applyEntryStep('nickname');
    expect(getEntryStep()).toBe('waiting');
  });

  it('tryEntryHandoff moves to handoff on countdown', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    state.nicknameSubmitted = true;
    tryEntryHandoff('countdown');
    expect(getEntryStep()).toBe('handoff');
  });

  it('onWebSocketOpen updates nickname status when on nickname step', () => {
    onLobbyCodeReady('ABC12');
    onWebSocketOpen();
    expect(document.getElementById('nickname-connect-status')!.textContent).toContain('服务器已连接');
    expect(document.getElementById('nickname-connect-status')!.textContent).toContain('进入游戏');
  });

  it('onWebSocketClosed updates nickname status when on nickname step', () => {
    onLobbyCodeReady('ABC12');
    onWebSocketOpen();
    onWebSocketClosed();
    expect(document.getElementById('nickname-connect-status')!.textContent).toContain('连接已断开');
  });

  it('bindEntryUI form submit triggers callback on nickname step', () => {
    onLobbyCodeReady('ABC12');
    const onSubmit = vi.fn();
    bindEntryUI(onSubmit);
    dom.form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    expect(onSubmit).toHaveBeenCalledOnce();
  });

  it('bindEntryUI ignores submit when past nickname step', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    const onSubmit = vi.fn();
    bindEntryUI(onSubmit);
    dom.form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('getWaitingTitleText reflects player count', () => {
    state.players = [{ playerIndex: 1, cooldownEndTime: 0, palette: 0, scoreContribution: 0, nickname: 'A' }];
    expect(getWaitingTitleText()).toBe('即将开始…');
    state.players.push({ playerIndex: 2, cooldownEndTime: 0, palette: 0, scoreContribution: 0, nickname: 'B' });
    expect(getWaitingTitleText()).toBe('等待其他玩家确认昵称…');
  });

  it('onWebSocketOpen preserves countdown when on waiting step', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    expect(document.getElementById('waiting-title')!.textContent).toMatch(/即将开始 · \d…/);
    // WebSocket 连接不应清除正在运行的倒计时
    onWebSocketOpen();
    expect(document.getElementById('waiting-title')!.textContent).toMatch(/即将开始 · \d…/);
  });

  it('onWebSocketClosed updates waiting title when on waiting step', () => {
    onLobbyCodeReady('ABC12');
    onNicknameSubmit();
    // 模拟倒计时已结束
    clearStartCountdownForTest();
    onWebSocketOpen();
    onWebSocketClosed();
    expect(document.getElementById('waiting-title')!.textContent).toContain('正在连接服务器');
  });

  it('onConnectError routes to nickname, waiting inline, or full-screen error', () => {
    onLobbyCodeReady('ABC12');
    onConnectError('昵称阶段错误');
    expect(document.getElementById('nickname-connect-status')!.textContent).toBe('昵称阶段错误');

    onNicknameSubmit();
    onConnectError('等待阶段错误');
    expect(document.getElementById('waiting-connect-error')!.textContent).toBe('等待阶段错误');
    expect(document.getElementById('waiting-connect-error')!.classList.contains('hidden')).toBe(false);

    clearWaitingInlineError();
    expect(document.getElementById('waiting-connect-error')!.classList.contains('hidden')).toBe(true);

    resetEntryFlowForTest();
    initEntryFlow();
    onConnectError('连接超时，请重试', { showActions: true, midGameDisconnect: true });
    expect(getEntryStep()).toBe('error');
    expect(document.getElementById('loading-error-title')!.textContent).toBe('对局连接中断');

    resetEntryFlowForTest();
    initEntryFlow();
    onConnectError('房间不存在');
    expect(document.getElementById('loading-error-title')!.textContent).toBe('无法进入房间');

    resetEntryFlowForTest();
    initEntryFlow();
    onConnectError('网络异常');
    expect(document.getElementById('loading-error-title')!.textContent).toBe('连接失败');

    resetEntryFlowForTest();
    initEntryFlow();
    onConnectError('未知错误');
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

  it('tryEntryHandoff is no-op before nickname submit', () => {
    onLobbyCodeReady('ABC12');
    tryEntryHandoff('playing');
    expect(getEntryStep()).toBe('nickname');
    expect(isEntryHandoff()).toBe(false);
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
