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