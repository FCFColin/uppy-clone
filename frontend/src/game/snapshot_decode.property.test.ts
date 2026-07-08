import { describe, it, expect } from 'vitest';
import fc from 'fast-check';
import { decodeSnapshot } from './message_codec.js';

describe('decodeSnapshot with arbitrary binary input', () => {
  it('returns null for any buffer shorter than 37 bytes', () => {
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

  it('never panics on any binary input (up to 1024 bytes)', () => {
    fc.assert(
      fc.property(
        fc.uint8Array({ minLength: 0, maxLength: 1024 }),
        (buffer) => {
          const dv = new DataView(buffer.buffer, buffer.byteOffset, buffer.length);
          expect(() => decodeSnapshot(dv)).not.toThrow();
        }
      ),
      { numRuns: 200 }
    );
  });

  it('returns DecodedSnapshot with valid structure when decoding succeeds', () => {
    fc.assert(
      fc.property(
        fc.array(fc.integer({ min: 0, max: 255 }), { minLength: 37, maxLength: 300 }),
        (bytes) => {
          const buf = new Uint8Array(bytes).buffer;
          const result = decodeSnapshot(new DataView(buf));
          if (result === null) return;
          expect(typeof result.timestamp).toBe('number');
          expect(typeof result.score).toBe('number');
          expect(typeof result.balloon.x).toBe('number');
          expect(typeof result.balloon.y).toBe('number');
          expect(typeof result.balloon.vx).toBe('number');
          expect(typeof result.balloon.vy).toBe('number');
          expect(typeof result.bird.active).toBe('boolean');
          expect(typeof result.ghost.active).toBe('boolean');
          expect(Array.isArray(result.players)).toBe(true);
          expect(Array.isArray(result.ripples)).toBe(true);
          expect(typeof result.playerCount).toBe('number');
        }
      )
    );
  });

  it('score is non-negative when decoding succeeds', () => {
    fc.assert(
      fc.property(
        fc.array(fc.integer({ min: 0, max: 255 }), { minLength: 37, maxLength: 300 }),
        (bytes) => {
          const buf = new Uint8Array(bytes).buffer;
          const result = decodeSnapshot(new DataView(buf));
          if (result === null) return;
          expect(result.score).toBeGreaterThanOrEqual(0);
        }
      )
    );
  });

  it('playerCount is non-negative when decoding succeeds', () => {
    fc.assert(
      fc.property(
        fc.array(fc.integer({ min: 0, max: 255 }), { minLength: 37, maxLength: 300 }),
        (bytes) => {
          const buf = new Uint8Array(bytes).buffer;
          const result = decodeSnapshot(new DataView(buf));
          if (result === null) return;
          expect(result.playerCount).toBeGreaterThanOrEqual(0);
        }
      )
    );
  });

  it('balloon coordinates are numbers when decoding succeeds', () => {
    fc.assert(
      fc.property(
        fc.array(fc.integer({ min: 0, max: 255 }), { minLength: 37, maxLength: 300 }),
        (bytes) => {
          const buf = new Uint8Array(bytes).buffer;
          const result = decodeSnapshot(new DataView(buf));
          if (result === null) return;
          // Note: Float32 can produce NaN/Infinity from arbitrary bytes;
          // we only assert they are numbers (not undefined/throws).
          expect(typeof result.balloon.x).toBe('number');
          expect(typeof result.balloon.y).toBe('number');
          expect(typeof result.balloon.vx).toBe('number');
          expect(typeof result.balloon.vy).toBe('number');
        }
      )
    );
  });

  it('players array length never exceeds playerCount when decoding succeeds', () => {
    fc.assert(
      fc.property(
        fc.array(fc.integer({ min: 0, max: 255 }), { minLength: 37, maxLength: 300 }),
        (bytes) => {
          const buf = new Uint8Array(bytes).buffer;
          const result = decodeSnapshot(new DataView(buf));
          if (result !== null) {
            // Truncation may cause fewer players than declared; never more.
            expect(result.players.length).toBeLessThanOrEqual(result.playerCount);
          }
        }
      )
    );
  });

  it('phase is always a valid GamePhase string when decoding succeeds', () => {
    fc.assert(
      fc.property(
        fc.array(fc.integer({ min: 0, max: 255 }), { minLength: 37, maxLength: 300 }),
        (bytes) => {
          const buf = new Uint8Array(bytes).buffer;
          const result = decodeSnapshot(new DataView(buf));
          if (result === null) return;
          expect(['waiting', 'playing', 'ended', 'countdown']).toContain(result.phase);
        }
      )
    );
  });
});
