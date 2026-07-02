import { describe, expect, it } from 'vitest';
import { calculateCooldown } from './message_codec.js';

const contractCases = [
  { playerCount: 0, expectedMs: 1000 },
  { playerCount: -5, expectedMs: 1000 },
  { playerCount: 1, expectedMs: 1000 },
  { playerCount: 2, expectedMs: 3032 },
  { playerCount: 4, expectedMs: 5064 },
  { playerCount: 8, expectedMs: 7096 },
  { playerCount: 100, expectedMs: 14500 },
  { playerCount: 1000, expectedMs: 15000 },
  { playerCount: 10000, expectedMs: 15000 },
];

describe('calculateCooldown contract', () => {
  for (const tc of contractCases) {
    it(`playerCount=${tc.playerCount} => ${tc.expectedMs}ms`, () => {
      expect(calculateCooldown(tc.playerCount)).toBe(tc.expectedMs);
    });
  }
});
