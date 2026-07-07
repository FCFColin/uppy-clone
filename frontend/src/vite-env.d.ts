/// <reference types="vite/client" />
/// <reference lib="dom" />

export {};

declare global {
  var state: unknown;
  var __gamePhase: string;
  var requestRestart: () => void;
  var generateRandomNickname: () => string;
  var __seenSeqs: Set<number>;
  var __interp: unknown;
  var submitSetupNickname: () => Promise<void>;
  var __ws: WebSocket | null;
  var _restartCountdownTimer: ReturnType<typeof setInterval> | null;
}
