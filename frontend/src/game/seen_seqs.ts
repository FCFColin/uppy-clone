import { MAX_SEEN_SEQS } from './local_constants.js';

let seenSeqs: Set<number> = new Set();
// game-013: Track the highest seq seen to detect uint32 wrap-around.
// After ~50 days at 15 Hz the tick counter wraps from 4294967295 to 0.
// When a new seq is much smaller than maxSeen, we clear the set to avoid
// false-positive duplicate detection after the wrap.
let maxSeen = -1;

// Threshold for detecting a wrap: if the new seq is more than this many
// ticks below maxSeen, assume a wrap occurred. Allows for minor out-of-order
// delivery while catching wraps quickly.
const WRAP_THRESHOLD = MAX_SEEN_SEQS * 2;

export function isDuplicateSeq(seq: number): boolean {
  // Detect uint32 wrap-around: if seq dropped significantly, clear stale entries.
  if (maxSeen >= 0 && seq < maxSeen - WRAP_THRESHOLD) {
    seenSeqs = new Set();
    maxSeen = seq;
  }

  if (seenSeqs.has(seq)) return true;
  seenSeqs.add(seq);
  if (seq > maxSeen) {
    maxSeen = seq;
  }
  if (seenSeqs.size > MAX_SEEN_SEQS) {
    const entries = [...seenSeqs];
    seenSeqs = new Set(entries.slice(Math.floor(MAX_SEEN_SEQS / 2)));
  }
  return false;
}

export function clearSeenSeqs(): void {
  seenSeqs.clear();
  maxSeen = -1;
}

/** Exposed for testing. Use isDuplicateSeq and clearSeenSeqs for all production access. */
export function getSeenSeqsSize(): number {
  return seenSeqs.size;
}
