import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockEncodeSetNickname = vi.hoisted(() => vi.fn());
const mockDispatch = vi.hoisted(() => vi.fn());
const mockNormalizeAuthHost = vi.hoisted(() => vi.fn());
const mockShowToast = vi.hoisted(() => vi.fn());
const mockResizeCanvas = vi.hoisted(() => vi.fn());
const mockGameLoop = vi.hoisted(() => vi.fn());
const mockStartGameLoop = vi.hoisted(() => vi.fn());
const mockRenderOnce = vi.hoisted(() => vi.fn());
const mockUpdateUI = vi.hoisted(() => vi.fn());
const mockGenerateRandomNickname = vi.hoisted(() => vi.fn(() => 'RandomNick'));
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
const mockSetupNicknameInput = vi.hoisted(() => ({ value: '' }));

vi.mock('./message_codec.js', () => ({ encodeSetNickname: mockEncodeSetNickname }));
vi.mock('./store.js', () => ({ dispatch: mockDispatch }));
vi.mock('../shared/network/session.js', () => ({ normalizeAuthHost: mockNormalizeAuthHost }));
vi.mock('../shared/ui/toast.js', () => ({ showToast: mockShowToast }));
vi.mock('./renderer.js', () => ({ resizeCanvas: mockResizeCanvas, gameLoop: mockGameLoop, startGameLoop: mockStartGameLoop, renderOnce: mockRenderOnce }));
vi.mock('./ws_connect.js', () => ({ connectWebSocket: mockConnectWebSocket, showConnectionError: mockShowConnectionError }));
vi.mock('./ws_connection.js', () => ({ sendOrQueue: mockSendOrQueue }));
vi.mock('./waiting_tips.js', () => ({ initWaitingTips: mockInitWaitingTips }));
vi.mock('./connection_ui.js', () => ({ bindReconnectRetry: mockBindReconnectRetry }));
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

  it('boot does not throw', () => {
    expect(() => boot()).not.toThrow();
  });

  it('boot saves game url to localStorage', () => {
    boot();
    expect(localStorage.getItem('uppy-game-url')).toBe(window.location.href);
  });

  it('boot calls normalizeAuthHost', () => {
    boot();
    expect(mockNormalizeAuthHost).toHaveBeenCalled();
  });

  it('boot calls connectWebSocket', () => {
    boot();
    expect(mockConnectWebSocket).toHaveBeenCalled();
  });

  it('boot calls initEntryFlow', () => {
    boot();
    expect(mockInitEntryFlow).toHaveBeenCalled();
  });

  it('boot calls bindEntryUI with submit callback', () => {
    boot();
    expect(mockBindEntryUI).toHaveBeenCalledWith(expect.any(Function));
  });

  it('boot calls initWaitingTips', () => {
    boot();
    expect(mockInitWaitingTips).toHaveBeenCalled();
  });

  it('boot calls resizeCanvas and renderOnce', () => {
    boot();
    expect(mockResizeCanvas).toHaveBeenCalled();
    expect(mockRenderOnce).toHaveBeenCalled();
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
