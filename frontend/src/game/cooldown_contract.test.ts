import { describe, expect, it } from 'vitest';
import contract from '../../../shared/data/cooldown_contract.json';
import { calculateCooldown } from './message_codec.js';

describe('calculateCooldown contract (shared/data/cooldown_contract.json)', () => {
  for (const tc of contract.cases) {
    it(`playerCount=${tc.playerCount} => ${tc.expectedMs}ms`, () => {
      expect(calculateCooldown(tc.playerCount)).toBe(tc.expectedMs);
    });
  }
});
