import { describe, it, expect, beforeEach } from 'vitest';
import {
  isDuplicateSeq,
  clearSeenSeqs,
  getSeenSeqsSize,
} from './seen_seqs.js';
import { MAX_SEEN_SEQS } from './constants.js';

describe('seen_seqs', () => {
  beforeEach(() => {
    clearSeenSeqs();
  });

  it('returns false for new seq, true for duplicate, tracks size', () => {
    expect(isDuplicateSeq(1)).toBe(false);
    expect(isDuplicateSeq(1)).toBe(true);
    isDuplicateSeq(2);
    expect(getSeenSeqsSize()).toBe(2);
  });

  it('clears via clearSeenSeqs', () => {
    isDuplicateSeq(1);
    isDuplicateSeq(2);
    clearSeenSeqs();
    expect(getSeenSeqsSize()).toBe(0);
    // After clear, previously seen seq is no longer duplicate.
    expect(isDuplicateSeq(1)).toBe(false);
  });

  it('detects uint32 wrap-around and clears stale entries', () => {
    // Establish a high maxSeen.
    isDuplicateSeq(1000);
    expect(getSeenSeqsSize()).toBe(1);
    // A seq far below maxSeen - WRAP_THRESHOLD triggers wrap-around clear.
    // WRAP_THRESHOLD = MAX_SEEN_SEQS * 2 = 400. 0 < 1000 - 400 = 600.
    expect(isDuplicateSeq(0)).toBe(false);
    // After wrap, the set is cleared and only the new seq is tracked.
    expect(getSeenSeqsSize()).toBe(1);
    // The old high seq is no longer considered duplicate because the set was cleared.
    expect(isDuplicateSeq(1000)).toBe(false);
  });

  it('does not trigger wrap-around for minor out-of-order delivery', () => {
    isDuplicateSeq(1000);
    // 700 is below 1000 but within WRAP_THRESHOLD (1000 - 400 = 600). 700 >= 600.
    expect(isDuplicateSeq(700)).toBe(false);
    // Set should retain both entries (no wrap-around clear).
    expect(getSeenSeqsSize()).toBe(2);
  });

  it('trims the set when size exceeds MAX_SEEN_SEQS', () => {
    // Add MAX_SEEN_SEQS + 1 unique seqs to trigger trimming.
    for (let i = 0; i <= MAX_SEEN_SEQS; i++) {
      isDuplicateSeq(i);
    }
    // Trim: entries.slice(floor(MAX_SEEN_SEQS / 2)) skips the first 100 entries
    // and keeps the rest: (MAX_SEEN_SEQS + 1) - floor(MAX_SEEN_SEQS / 2) = 201 - 100 = 101.
    const expectedSize = MAX_SEEN_SEQS + 1 - Math.floor(MAX_SEEN_SEQS / 2);
    expect(getSeenSeqsSize()).toBe(expectedSize);
    // Early seqs (before the slice offset) are dropped and no longer count as duplicates.
    expect(isDuplicateSeq(0)).toBe(false);
    // A seq within the retained range is still tracked.
    expect(isDuplicateSeq(MAX_SEEN_SEQS)).toBe(true);
  });
});
