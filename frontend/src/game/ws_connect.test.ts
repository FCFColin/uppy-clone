import { describe, it, expect, vi, beforeEach } from 'vitest';

let lastSocket: MockWebSocket | null = null;

class MockWebSocket {
  static OPEN = 1;
  binaryType = 'arraybuffer';
  readyState = MockWebSocket.OPEN;
  onopen: (() => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(_url: string) {
    lastSocket = this;
  }
  send = vi.fn();
  close = vi.fn();
}

const connectMocks = vi.hoisted(() => ({
  establishGameSession: vi.fn(async () => ({ ok: true as const })),
  validateRoomCode: vi.fn(async () => ({ ok: true as const })),
  resolveLobbyCode: vi.fn(async () => 'ROOM1'),
  hideLoadingOverlay: vi.fn(),
  enqueueBinaryMessage: vi.fn(),
}));

vi.mock('../shared/session.js', () => ({
  establishGameSession: connectMocks.establishGameSession,
  sessionErrorMessage: () => 'auth failed',
}));

vi.mock('./room_validate.js', () => ({
  getLobbyCodeFromUrl: () => 'URL1',
  validateRoomCode: connectMocks.validateRoomCode,
  roomErrorMessage: () => 'bad room',
}));

vi.mock('./ws_connect_lobby.js', () => ({
  resolveLobbyCode: connectMocks.resolveLobbyCode,
}));

vi.mock('./ui.js', () => ({
  hideLoadingOverlay: connectMocks.hideLoadingOverlay,
  $lobbyCode: { textContent: '' },
  $hudCode: { textContent: '' },
}));

vi.mock('./ws_connection.js', () => ({
  setWs: vi.fn(),
  getWsEverOpened: vi.fn(() => false),
  setWsEverOpened: vi.fn(),
  resetReconnectAttempts: vi.fn(),
  setReconnectTimer: vi.fn(),
  startWsHeartbeat: vi.fn(),
  stopHeartbeat: vi.fn(),
  sendOrQueue: vi.fn(),
  flushPendingQueue: vi.fn(),
  hideReconnectBanner: vi.fn(),
  scheduleReconnect: vi.fn(),
  showConnectionError: vi.fn(),
  wasRoomPreChecked: vi.fn(() => false),
  setRoomPreChecked: vi.fn(),
}));

vi.mock('./ws_message_queue.js', () => ({
  enqueueBinaryMessage: connectMocks.enqueueBinaryMessage,
}));

vi.stubGlobal('WebSocket', MockWebSocket);
vi.stubGlobal('localStorage', {
  getItem: vi.fn(() => null),
  setItem: vi.fn(),
});

import { connectWebSocket } from './ws_connect.js';

describe('connectWebSocket', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    lastSocket = null;
    connectMocks.establishGameSession.mockResolvedValue({ ok: true });
    connectMocks.validateRoomCode.mockResolvedValue({ ok: true });
    connectMocks.resolveLobbyCode.mockResolvedValue('ROOM1');
  });

  it('routes inbound frames through the message queue', async () => {
    await connectWebSocket();
    const buf = new ArrayBuffer(4);
    lastSocket?.onmessage?.({ data: buf } as MessageEvent);
    expect(connectMocks.enqueueBinaryMessage).toHaveBeenCalledWith(buf);
  });

  it('shows error when session establishment fails', async () => {
    const { showConnectionError } = await import('./ws_connection.js');
    connectMocks.establishGameSession.mockResolvedValueOnce({ ok: false, reason: 'network' });
    await connectWebSocket();
    expect(showConnectionError).toHaveBeenCalled();
  });
});
