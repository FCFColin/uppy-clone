import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockEncodeSetNickname = vi.hoisted(() => vi.fn());
const mockDispatch = vi.hoisted(() => vi.fn());
const mockNormalizeAuthHost = vi.hoisted(() => vi.fn());
const mockShowToast = vi.hoisted(() => vi.fn());
const mockResizeCanvas = vi.hoisted(() => vi.fn());
const mockGameLoop = vi.hoisted(() => vi.fn());
const mockStartGameLoop = vi.hoisted(() => vi.fn());
const mockRenderOnce = vi.hoisted(() => vi.fn());
const mockConnectWebSocket = vi.hoisted(() => vi.fn());
const mockShowConnectionError = vi.hoisted(() => vi.fn());
const mockSendOrQueue = vi.hoisted(() => vi.fn());
const mockInitWaitingTips = vi.hoisted(() => vi.fn());
const mockBindReconnectRetry = vi.hoisted(() => vi.fn());
const mockInitEntryFlow = vi.hoisted(() => vi.fn());
const mockBindEntryUI = vi.hoisted(() => vi.fn());
const mockOnNicknameSubmit = vi.hoisted(() => vi.fn());
const mockOnWebSocketOpen = vi.hoisted(() => vi.fn());
const mockGetEntryStep = vi.hoisted(() => vi.fn(() => 'connecting'));
const mockRouteConnectionError = vi.hoisted(() => vi.fn());
const mockClearStartCountdown = vi.hoisted(() => vi.fn());

vi.mock('./message_codec.js', () => ({ encodeSetNickname: mockEncodeSetNickname }));
vi.mock('./state.js', () => ({ dispatch: mockDispatch }));
vi.mock('../shared/network/session.js', () => ({ normalizeAuthHost: mockNormalizeAuthHost }));
vi.mock('../shared/ui/utils.js', () => ({
  showToast: mockShowToast,
  safeGetItem: (k: string) => localStorage.getItem(k),
  safeSetItem: (k: string, v: string) => localStorage.setItem(k, v),
}));
vi.mock('./renderer.js', () => ({
  resizeCanvas: mockResizeCanvas,
  gameLoop: mockGameLoop,
  startGameLoop: mockStartGameLoop,
  renderOnce: mockRenderOnce,
}));
vi.mock('./ws_connection.js', () => ({
  sendOrQueue: mockSendOrQueue,
  connectWebSocket: mockConnectWebSocket,
  showConnectionError: mockShowConnectionError,
}));
vi.mock('./ui_common.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./ui_common.js')>();
  return {
    ...actual,
    initWaitingTips: mockInitWaitingTips,
    bindReconnectRetry: mockBindReconnectRetry,
  };
});
vi.mock('./entry_flow.js', () => ({
  initEntryFlow: mockInitEntryFlow,
  bindEntryUI: mockBindEntryUI,
  onNicknameSubmit: mockOnNicknameSubmit,
  onWebSocketOpen: mockOnWebSocketOpen,
  getEntryStep: mockGetEntryStep,
  routeConnectionError: mockRouteConnectionError,
  clearStartCountdown: mockClearStartCountdown,
}));

import { boot, resetBootBound } from './lifecycle.js';

describe('lifecycle', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    resetBootBound();
  });

  it('boot does NOT save game url to localStorage (in-game leaderboard is an overlay)', () => {
    boot();
    expect(localStorage.getItem('uppy-game-url')).toBeNull();
  });

  it.each([
    ['normalizeAuthHost', mockNormalizeAuthHost],
    ['connectWebSocket', mockConnectWebSocket],
    ['initEntryFlow', mockInitEntryFlow],
    ['initWaitingTips', mockInitWaitingTips],
    ['resizeCanvas', mockResizeCanvas],
    ['renderOnce', mockRenderOnce],
  ] as const)('boot calls %s', (_name, mock) => {
    boot();
    expect(mock).toHaveBeenCalled();
  });

  it('boot calls bindEntryUI with submit callback', () => {
    boot();
    expect(mockBindEntryUI).toHaveBeenCalledWith(expect.any(Function));
  });

  it('boot sets timeout for connection error', () => {
    vi.useFakeTimers();
    boot();
    vi.advanceTimersByTime(8000);
    expect(mockShowConnectionError).toHaveBeenCalledWith('连接超时，请检查网络或稍后重试', { showActions: true });
    vi.useRealTimers();
  });

  it('waiting timeout fires routeConnectionError when entryStep is waiting', () => {
    mockGetEntryStep.mockReturnValue('waiting');
    vi.useFakeTimers();
    boot();
    vi.advanceTimersByTime(15000);
    expect(mockClearStartCountdown).toHaveBeenCalled();
    expect(mockRouteConnectionError).toHaveBeenCalledWith('未收到服务器响应，请重试', { showActions: true });
    vi.useRealTimers();
    mockGetEntryStep.mockReturnValue('connecting');
  });

  it('waiting timeout is no-op when entryStep is handoff', () => {
    mockGetEntryStep.mockReturnValue('handoff');
    vi.useFakeTimers();
    boot();
    vi.advanceTimersByTime(15000);
    expect(mockClearStartCountdown).not.toHaveBeenCalled();
    expect(mockRouteConnectionError).not.toHaveBeenCalled();
    vi.useRealTimers();
    mockGetEntryStep.mockReturnValue('connecting');
  });

  it('waiting timeout is no-op when entryStep is nickname', () => {
    mockGetEntryStep.mockReturnValue('nickname');
    vi.useFakeTimers();
    boot();
    vi.advanceTimersByTime(15000);
    expect(mockClearStartCountdown).not.toHaveBeenCalled();
    expect(mockRouteConnectionError).not.toHaveBeenCalled();
    vi.useRealTimers();
    mockGetEntryStep.mockReturnValue('connecting');
  });

  it('boot registers game-ws-open listener', () => {
    const addEventListenerSpy = vi.spyOn(window, 'addEventListener');
    boot();
    expect(addEventListenerSpy).toHaveBeenCalledWith('game-ws-open', expect.any(Function));
  });
});
