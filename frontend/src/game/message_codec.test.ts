import { describe, it, expect } from 'vitest';
import { codeToPhase } from './message_codec.js';

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
