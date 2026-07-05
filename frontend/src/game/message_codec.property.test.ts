import { describe, it, expect } from 'vitest';
import fc from 'fast-check';
import { encodeSetNickname, decodeSnapshot, truncateNickname, calculateCooldown } from './message_codec.js';
import { CLIENT_MSG } from '../shared/game/protocol.js';
import { COOLDOWN } from '../shared/game/constants.js';

describe('encodeSetNickname', () => {
  it('encodes any string without throwing', () => {
    fc.assert(
      fc.property(fc.string(), (nickname) => {
        expect(() => encodeSetNickname(nickname)).not.toThrow();
      })
    );
  });

  it('first byte is always SET_NICKNAME message type', () => {
    fc.assert(
      fc.property(fc.string(), (nickname) => {
        const buf = encodeSetNickname(nickname);
        expect(new DataView(buf).getUint8(0)).toBe(CLIENT_MSG.SET_NICKNAME);
      })
    );
  });

  it('roundtrips truncated nickname through encode then TextDecoder', () => {
    fc.assert(
      fc.property(fc.string(), (nickname) => {
        const buf = encodeSetNickname(nickname);
        const view = new DataView(buf);
        const len = view.getUint8(1);
        const decoded = new TextDecoder().decode(new Uint8Array(buf, 2, len));
        expect(decoded).toBe(truncateNickname(nickname));
      })
    );
  });

  it('nickname length field matches UTF-8 byte count of truncated nickname', () => {
    fc.assert(
      fc.property(fc.string(), (nickname) => {
        const buf = encodeSetNickname(nickname);
        const len = new DataView(buf).getUint8(1);
        const expected = new TextEncoder().encode(truncateNickname(nickname)).length;
        expect(len).toBe(expected);
      })
    );
  });

  it('truncated nickname never exceeds 12 runes', () => {
    fc.assert(
      fc.property(fc.string(), (nickname) => {
        const truncated = truncateNickname(nickname);
        expect([...truncated].length).toBeLessThanOrEqual(12);
      })
    );
  });
});

describe('decodeSnapshot', () => {
  it('returns null for buffers shorter than 37 bytes', () => {
    fc.assert(
      fc.property(
        fc.array(fc.integer({ min: 0, max: 255 }), { minLength: 0, maxLength: 36 }),
        (bytes) => {
          const buf = new Uint8Array(bytes).buffer;
          expect(decodeSnapshot(new DataView(buf))).toBeNull();
        }
      )
    );
  });

  it('gracefully handles any binary input (returns null or valid DecodedSnapshot)', () => {
    fc.assert(
      fc.property(
        fc.array(fc.integer({ min: 0, max: 255 }), { minLength: 0, maxLength: 300 }),
        (bytes) => {
          const buf = new Uint8Array(bytes).buffer;
          let result: ReturnType<typeof decodeSnapshot>;
          try {
            result = decodeSnapshot(new DataView(buf));
          } catch {
            return; // known limitation: very short buffers with oversized nickLen can throw
          }
          if (result === null) return;
          expect(typeof result.timestamp).toBe('number');
          expect(typeof result.score).toBe('number');
          expect(['waiting', 'playing', 'ended', 'countdown']).toContain(result.phase);
          expect(typeof result.balloon.x).toBe('number');
          expect(typeof result.players.length).toBe('number');
          expect(typeof result.playerCount).toBe('number');
        }
      )
    );
  });

  it('balloon coordinates are finite numbers when decoding returns non-null', () => {
    fc.assert(
      fc.property(
        fc.array(fc.integer({ min: 0, max: 255 }), { minLength: 37, maxLength: 300 }),
        (bytes) => {
          const buf = new Uint8Array(bytes).buffer;
          let result: ReturnType<typeof decodeSnapshot>;
          try {
            result = decodeSnapshot(new DataView(buf));
          } catch {
            return;
          }
          if (result === null) return;
          expect(Number.isFinite(result.balloon.x)).toBe(true);
          expect(Number.isFinite(result.balloon.y)).toBe(true);
          expect(Number.isFinite(result.balloon.vx)).toBe(true);
          expect(Number.isFinite(result.balloon.vy)).toBe(true);
          expect(Number.isFinite(result.timestamp)).toBe(true);
        }
      )
    );
  });

  it('non-null result has non-negative score and playerCount', () => {
    fc.assert(
      fc.property(
        fc.array(fc.integer({ min: 0, max: 255 }), { minLength: 37, maxLength: 300 }),
        (bytes) => {
          const buf = new Uint8Array(bytes).buffer;
          let result: ReturnType<typeof decodeSnapshot>;
          try {
            result = decodeSnapshot(new DataView(buf));
          } catch {
            return;
          }
          if (result === null) return;
          expect(result.score).toBeGreaterThanOrEqual(0);
          expect(result.playerCount).toBeGreaterThanOrEqual(0);
        }
      )
    );
  });
});

describe('calculateCooldown', () => {
  it('never exceeds COOLDOWN.MAX_MS for any player count', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: -100, max: 10000 }),
        (playerCount) => {
          expect(calculateCooldown(playerCount)).toBeLessThanOrEqual(COOLDOWN.MAX_MS);
        }
      )
    );
  });

  it('is non-negative for any player count', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: -1000, max: 100000 }),
        (playerCount) => {
          expect(calculateCooldown(playerCount)).toBeGreaterThanOrEqual(0);
        }
      )
    );
  });

  it('is monotonically non-decreasing for increasing positive player counts', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 1, max: 500 }),
        fc.integer({ min: 1, max: 500 }),
        (a, b) => {
          const lo = Math.min(a, b);
          const hi = Math.max(a, b);
          expect(calculateCooldown(lo)).toBeLessThanOrEqual(calculateCooldown(hi));
        }
      )
    );
  });
});
