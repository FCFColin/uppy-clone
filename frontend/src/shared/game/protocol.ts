export const MSG_TYPE = {
  SNAPSHOT: 0x01,
  PLAYER_JOIN: 0x02,
  PLAYER_LEAVE: 0x03,
  TAP_ACCEPTED: 0x04,
  TAP_REJECTED: 0x05,
  GAME_STATE_CHANGE: 0x06,
  RESTART_STATUS: 0x07,
  PONG: 0x21,
} as const;

export const CLIENT_MSG = {
  TAP: 0x10,
  SET_NICKNAME: 0x11,
  RESTART_VOTE: 0x12,
  PING: 0x20,
} as const;

export const PHASE_CODE = {
  WAITING: 0,
  PLAYING: 1,
  ENDED: 2,
  COUNTDOWN: 3,
} as const;

export type MsgType = typeof MSG_TYPE[keyof typeof MSG_TYPE];
export type ClientMsgType = typeof CLIENT_MSG[keyof typeof CLIENT_MSG];

import { type GamePhase } from './types.js';

export function phaseFromCode(code: number): GamePhase {
    switch (code) {
        case PHASE_CODE.WAITING: return 'waiting';
        case PHASE_CODE.COUNTDOWN: return 'countdown';
        case PHASE_CODE.PLAYING: return 'playing';
        case PHASE_CODE.ENDED: return 'ended';
        default: return 'waiting';
    }
}

export function phaseToCode(phase: GamePhase): number {
    switch (phase) {
        case 'waiting': return PHASE_CODE.WAITING;
        case 'countdown': return PHASE_CODE.COUNTDOWN;
        case 'playing': return PHASE_CODE.PLAYING;
        case 'ended': return PHASE_CODE.ENDED;
    }
}
