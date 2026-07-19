/// <reference types="vite/client" />
/// <reference lib="dom" />

export {};

declare global {
  var __gamePhase: string;
  var _restartCountdownTimer: ReturnType<typeof setInterval> | null;
}
