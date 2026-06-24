/// <reference types="vite/client" />

interface Window {
  state: unknown;
  __gamePhase: string;
  requestRestart: () => void;
  generateRandomNickname: () => string;
  __seenSeqs: Set<number>;
  __interp: unknown;
  submitSetupNickname: () => Promise<void>;
  __ws: WebSocket | null;
  _restartCountdownTimer: ReturnType<typeof setInterval> | null;
}
