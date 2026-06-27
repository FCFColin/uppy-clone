import { describe, it, expect } from 'vitest';
import { MSG_TYPE, CLIENT_MSG } from './protocol.js';

/** Must match backend/internal/protocol/constants.go and docs/api/ws-protocol.md */
describe('protocol constants', () => {
  it('server message types match backend', () => {
    expect(MSG_TYPE.SNAPSHOT).toBe(0x01);
    expect(MSG_TYPE.PLAYER_JOIN).toBe(0x02);
    expect(MSG_TYPE.PLAYER_LEAVE).toBe(0x03);
    expect(MSG_TYPE.TAP_ACCEPTED).toBe(0x04);
    expect(MSG_TYPE.TAP_REJECTED).toBe(0x05);
    expect(MSG_TYPE.GAME_STATE_CHANGE).toBe(0x06);
    expect(MSG_TYPE.RESTART_STATUS).toBe(0x07);
    expect(MSG_TYPE.PONG).toBe(0x21);
  });

  it('client message types match backend', () => {
    expect(CLIENT_MSG.TAP).toBe(0x10);
    expect(CLIENT_MSG.SET_NICKNAME).toBe(0x11);
    expect(CLIENT_MSG.RESTART_VOTE).toBe(0x12);
    expect(CLIENT_MSG.PING).toBe(0x20);
  });
});
