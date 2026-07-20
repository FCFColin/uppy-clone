export interface EntryMockState {
  phase: string;
  lobbyCode: string;
  players: Array<{
    playerIndex: number;
    nickname: string;
    palette: number;
    cooldownEndTime: number;
    scoreContribution: number;
  }>;
  nicknameSubmitted: boolean;
}

let mocks: { state: EntryMockState } | null = null;

export function getEntryMocks() {
  if (!mocks) {
    mocks = { state: createEntryMockState() };
  }
  return mocks;
}

export function createEntryMockState(): EntryMockState {
  return {
    phase: 'waiting',
    lobbyCode: '',
    players: [],
    nicknameSubmitted: false,
  };
}

export function resetEntryMocks() {
  const s = getEntryMocks().state;
  s.phase = 'waiting';
  s.lobbyCode = '';
  s.players = [];
  s.nicknameSubmitted = false;
}

// F-001: Shared DOM setup for entry_flow tests.
// The HTML template + form creation was duplicated in entry_flow.test.ts and
// entry_flow_dom.test.ts vi.hoisted blocks. Since vi.hoisted callbacks run
// before imports (cannot call imported helpers), we set up the DOM as a
// module side effect. Importing this file ensures the DOM is ready before
// vi.mock factories (which access document.getElementById) run.
const LOADING_OVERLAY_HTML = `
    <div id="loading-overlay">
      <div class="loading-spinner"></div>
      <div class="loading-text"></div>
    </div>
    <div id="loading-error-panel" class="hidden"></div>
    <div id="loading-error-text"></div>
    <div id="loading-error-title"></div>
    <div id="loading-error-actions" class="hidden"></div>
    <div id="waiting-title"></div>
    <div id="nickname-connect-status"></div>
    <div id="waiting-connect-error" class="hidden"></div>
    <div id="nickname-setup-screen"></div>
    <div id="waiting-screen"></div>
    <div id="reconnect-banner" class="hidden"></div>
    <div id="lobby-code"></div>
    <div id="hud-code"></div>
    <div id="loading-back-btn"></div>
    <div id="loading-match-btn"></div>
    <div id="game-canvas" style="pointer-events: none;"></div>
    <div id="game-hud" class="hidden"></div>
  `;

export function setupLoadingOverlayDom() {
  document.body.innerHTML = LOADING_OVERLAY_HTML;
  const form = document.createElement('form');
  form.id = 'nickname-entry-form';
  document.body.appendChild(form);
}

// Side effect: set up the DOM when this module is imported.
// Runs before vi.mock factories that access document.getElementById.
setupLoadingOverlayDom();
