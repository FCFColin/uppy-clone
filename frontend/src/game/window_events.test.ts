import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockCanvas = vi.hoisted(() => {
  const el = document.createElement('canvas');
  el.id = 'game-canvas';
  return el;
});
const mockGetState = vi.hoisted(() => vi.fn(() => ({ phase: 'playing' })));
const mockDispatch = vi.hoisted(() => vi.fn());
const mockResizeCanvas = vi.hoisted(() => vi.fn());
const mockGameLoop = vi.hoisted(() => vi.fn());
const mockSetRenderActive = vi.hoisted(() => vi.fn());
const mockRenderOnce = vi.hoisted(() => vi.fn());
const mockResumeAudioContext = vi.hoisted(() => vi.fn());
const mockHandleTap = vi.hoisted(() => vi.fn());
const mockRequestRestart = vi.hoisted(() => vi.fn());
const mockTapAtBalloonCenter = vi.hoisted(() => vi.fn());
const mockUpdateUI = vi.hoisted(() => vi.fn());
const mockGenerateRandomNickname = vi.hoisted(() => vi.fn(() => 'Random'));
const mockCopyCode = vi.hoisted(() => vi.fn());
const mockRefreshLayout = vi.hoisted(() => vi.fn());
const mockShowFallbackErrorScreen = vi.hoisted(() => vi.fn());
const mockConnectWebSocket = vi.hoisted(() => vi.fn());
const mockStopHeartbeat = vi.hoisted(() => vi.fn());
const mockGetWs = vi.hoisted(() => vi.fn());

vi.mock('../shared/ui/audio.js', () => ({ resumeAudioContext: mockResumeAudioContext }));
vi.mock('./input.js', () => ({
  handleTap: mockHandleTap,
  requestRestart: mockRequestRestart,
  tapAtBalloonCenter: mockTapAtBalloonCenter,
}));
vi.mock('./store.js', () => ({ getState: mockGetState, dispatch: mockDispatch }));
vi.mock('./renderer.js', () => ({
  resizeCanvas: mockResizeCanvas,
  gameLoop: mockGameLoop,
  setRenderActive: mockSetRenderActive,
  renderOnce: mockRenderOnce,
  $canvas: mockCanvas,
}));
vi.mock('./ui.js', () => ({
  updateUI: mockUpdateUI,
  generateRandomNickname: mockGenerateRandomNickname,
  copyCode: mockCopyCode,
  refreshLayout: mockRefreshLayout,
  showFallbackErrorScreen: mockShowFallbackErrorScreen,
  $copyCodeBtn: null,
  $hudCopyBtn: null,
  $setupNicknameInput: null,
}));
vi.mock('./ws_connect.js', () => ({ connectWebSocket: mockConnectWebSocket }));
vi.mock('./ws_connection.js', () => ({ stopHeartbeat: mockStopHeartbeat, getWs: mockGetWs }));

import { bindWindowEvents } from './window_events.js';

describe('windowEvents', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    document.body.innerHTML = '';
  });

  it('bindWindowEvents does not throw', () => {
    expect(() => bindWindowEvents()).not.toThrow();
  });

  it('adds click listener to canvas that resumes audio context', () => {
    bindWindowEvents();
    mockCanvas.dispatchEvent(new MouseEvent('click'));
    expect(mockResumeAudioContext).toHaveBeenCalled();
  });

  it('adds touchstart listener to canvas that resumes audio context', () => {
    bindWindowEvents();
    mockCanvas.dispatchEvent(new TouchEvent('touchstart'));
    expect(mockResumeAudioContext).toHaveBeenCalled();
  });

  it('registers resize handler on window', () => {
    const addEventListenerSpy = vi.spyOn(window, 'addEventListener');
    bindWindowEvents();
    expect(addEventListenerSpy).toHaveBeenCalledWith('resize', expect.any(Function));
    expect(addEventListenerSpy).toHaveBeenCalledWith('orientationchange', expect.any(Function));
  });

  it('registers visibilitychange handler on document', () => {
    const addEventListenerSpy = vi.spyOn(document, 'addEventListener');
    bindWindowEvents();
    expect(addEventListenerSpy).toHaveBeenCalledWith('visibilitychange', expect.any(Function));
  });

  it('registers keydown handler on document', () => {
    const addEventListenerSpy = vi.spyOn(document, 'addEventListener');
    bindWindowEvents();
    expect(addEventListenerSpy).toHaveBeenCalledWith('keydown', expect.any(Function));
  });

  it('registers error handler on window', () => {
    const addEventListenerSpy = vi.spyOn(window, 'addEventListener');
    bindWindowEvents();
    expect(addEventListenerSpy).toHaveBeenCalledWith('error', expect.any(Function));
  });

  it('registers unhandledrejection handler on window', () => {
    const addEventListenerSpy = vi.spyOn(window, 'addEventListener');
    bindWindowEvents();
    expect(addEventListenerSpy).toHaveBeenCalledWith('unhandledrejection', expect.any(Function));
  });

  it('registers online/offline handlers on window', () => {
    const addEventListenerSpy = vi.spyOn(window, 'addEventListener');
    bindWindowEvents();
    expect(addEventListenerSpy).toHaveBeenCalledWith('online', expect.any(Function));
    expect(addEventListenerSpy).toHaveBeenCalledWith('offline', expect.any(Function));
  });

  it('registers beforeunload handler on window', () => {
    const addEventListenerSpy = vi.spyOn(window, 'addEventListener');
    bindWindowEvents();
    expect(addEventListenerSpy).toHaveBeenCalledWith('beforeunload', expect.any(Function));
  });
});
