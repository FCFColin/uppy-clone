import { describe, it, expect } from 'vitest';
import fc from 'fast-check';
import { encodeSetNickname, truncateNickname, calculateCooldown } from './message_codec.js';
import { CLIENT_MSG } from '../shared/game/constants.js';
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