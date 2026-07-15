/// <reference types="vite/client" />
/// <reference lib="dom" />

export {};

declare global {
  var __gamePhase: string;
  var __seenSeqs: Set<number>;
  var _restartCountdownTimer: ReturnType<typeof setInterval> | null;
}
