import { describe, it, expect } from 'vitest';
import { codeToPhase, calculateCooldown, encodeSetNickname, truncateNickname } from './message_codec.js';
import { CLIENT_MSG } from '../shared/game/constants.js';
import { COOLDOWN } from '../shared/game/constants.js';

describe('Protocol parsing - codeToPhase', () => {
  it('maps known phase codes to the correct GamePhase', () => {
    expect(codeToPhase(0)).toBe('waiting');
    expect(codeToPhase(1)).toBe('playing');
    expect(codeToPhase(2)).toBe('ended');
    expect(codeToPhase(3)).toBe('countdown');
  });

  it('falls back to waiting for unknown codes', () => {
    expect(codeToPhase(99)).toBe('waiting');
    expect(codeToPhase(255)).toBe('waiting');
    expect(codeToPhase(-1)).toBe('waiting');
  });
});

describe('Protocol parsing - calculateCooldown', () => {
  it('returns BASE_MS for a single player (log2(1) = 0)', () => {
    expect(calculateCooldown(1)).toBe(COOLDOWN.BASE_MS);
  });

  it('scales logarithmically with player count', () => {
    // playerCount=2: BASE_MS + LOG_COEFFICIENT * log2(2) = 1000 + 2032 * 1
    expect(calculateCooldown(2)).toBe(COOLDOWN.BASE_MS + COOLDOWN.LOG_COEFFICIENT * 1);
    // playerCount=4: 1000 + 2032 * 2
    expect(calculateCooldown(4)).toBe(COOLDOWN.BASE_MS + COOLDOWN.LOG_COEFFICIENT * 2);
    // playerCount=8: 1000 + 2032 * 3
    expect(calculateCooldown(8)).toBe(COOLDOWN.BASE_MS + COOLDOWN.LOG_COEFFICIENT * 3);
  });

  it('treats zero and negative player counts as a single player', () => {
    expect(calculateCooldown(0)).toBe(COOLDOWN.BASE_MS);
    expect(calculateCooldown(-5)).toBe(COOLDOWN.BASE_MS);
  });

  it('caps the cooldown at MAX_MS for large player counts', () => {
    expect(calculateCooldown(1000)).toBe(COOLDOWN.MAX_MS);
    expect(calculateCooldown(10000)).toBe(COOLDOWN.MAX_MS);
  });

  it('never exceeds MAX_MS across a range of player counts', () => {
    for (const n of [1, 2, 10, 50, 100, 500, 1000]) {
      expect(calculateCooldown(n)).toBeLessThanOrEqual(COOLDOWN.MAX_MS);
    }
  });

  it('is monotonically non-decreasing for increasing player counts', () => {
    let prev: number = calculateCooldown(1);
    for (let n = 2; n <= 200; n += 1) {
      const curr: number = calculateCooldown(n);
      expect(curr).toBeGreaterThanOrEqual(prev);
      prev = curr;
    }
  });
});

describe('Protocol parsing - encodeSetNickname', () => {
  it('encodes an ASCII nickname with the correct header and payload', () => {
    const buf: ArrayBuffer = encodeSetNickname('abc');
    const view: DataView = new DataView(buf);
    expect(view.byteLength).toBe(5); // 1 (type) + 1 (len) + 3 (payload)
    expect(view.getUint8(0)).toBe(CLIENT_MSG.SET_NICKNAME);
    expect(view.getUint8(1)).toBe(3);
    expect(Array.from(new Uint8Array(buf, 2))).toEqual([97, 98, 99]); // 'a','b','c'
  });

  it('round-trips an ASCII nickname through TextDecoder', () => {
    const nick: string = 'player1';
    const buf: ArrayBuffer = encodeSetNickname(nick);
    const view: DataView = new DataView(buf);
    const len: number = view.getUint8(1);
    const decoded: string = new TextDecoder().decode(new Uint8Array(buf, 2, len));
    expect(decoded).toBe(nick);
  });

  it('round-trips a Unicode nickname (UTF-8 multibyte)', () => {
    const nick: string = '玩家';
    const buf: ArrayBuffer = encodeSetNickname(nick);
    const view: DataView = new DataView(buf);
    const len: number = view.getUint8(1);
    expect(len).toBe(new TextEncoder().encode(nick).length);
    const decoded: string = new TextDecoder().decode(new Uint8Array(buf, 2, len));
    expect(decoded).toBe(nick);
  });

  it('encodes an empty nickname with length 0', () => {
    const buf: ArrayBuffer = encodeSetNickname('');
    const view: DataView = new DataView(buf);
    expect(view.byteLength).toBe(2); // type + len only
    expect(view.getUint8(0)).toBe(CLIENT_MSG.SET_NICKNAME);
    expect(view.getUint8(1)).toBe(0);
  });

  it('always sets the SET_NICKNAME message type byte', () => {
    for (const nick of ['', 'a', 'abc', '玩家']) {
      const view: DataView = new DataView(encodeSetNickname(nick));
      expect(view.getUint8(0)).toBe(CLIENT_MSG.SET_NICKNAME);
    }
  });

  it('stores the UTF-8 byte length (not the character count) in the length field', () => {
    // '玩家' = 2 chars but 6 UTF-8 bytes
    const view: DataView = new DataView(encodeSetNickname('玩家'));
    expect(view.getUint8(1)).toBe(6);
  });

  it('truncates nicknames longer than 12 runes before encoding', () => {
    const long = '一二三四五六七八九十十一十二十三';
    expect([...truncateNickname(long)].length).toBe(12);
    const view: DataView = new DataView(encodeSetNickname(long));
    expect(view.getUint8(1)).toBe(new TextEncoder().encode(truncateNickname(long)).length);
  });
});
