export type { ClientState } from './state_types.js';
export { state } from './state_types.js';
export {
  updateInterpolation,
  resetInterpolation,
  freezeInterpolation,
  getInterpolatedBalloon,
  getInterpolatedGhost,
  getInterpolatedBird,
  setInterpolationClockOffset,
  commitRenderedState,
  seenSeqs,
  isDuplicateSeq,
  pendingQueue,
  resetClientState,
  getInterpState,
} from './state_interp.js';
