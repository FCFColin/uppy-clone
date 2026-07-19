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


vi.mock('./message_codec.js', () => ({ encodeSetNickname: mockEncodeSetNickname }));
vi.mock('./state.js', () => ({ dispatch: mockDispatch }));
vi.mock('../shared/network/session.js', () => ({ normalizeAuthHost: mockNormalizeAuthHost }));
vi.mock('../shared/ui/utils.js', () => ({
  showToast: mockShowToast,
  safeGetItem: (k: string) => localStorage.getItem(k),
  safeSetItem: (k: string, v: string) => localStorage.setItem(k, v),
}));
vi.mock('./renderer.js', () => ({ resizeCanvas: mockResizeCanvas, gameLoop: mockGameLoop, startGameLoop: mockStartGameLoop, renderOnce: mockRenderOnce }));
vi.mock('./ws_connection.js', () => ({ sendOrQueue: mockSendOrQueue, connectWebSocket: mockConnectWebSocket, showConnectionError: mockShowConnectionError }));
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
}));

import { boot, resetBootBound } from './lifecycle.js';

describe('lifecycle', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    resetBootBound();
  });

  it('boot saves game url to localStorage', () => {
    boot();
    expect(localStorage.getItem('uppy-game-url')).toBe(window.location.href);
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

  it('boot registers game-ws-open listener', () => {
    const addEventListenerSpy = vi.spyOn(window, 'addEventListener');
    boot();
    expect(addEventListenerSpy).toHaveBeenCalledWith('game-ws-open', expect.any(Function));
  });
});
