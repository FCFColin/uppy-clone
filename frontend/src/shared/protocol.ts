export const MSG_TYPE = {
  SNAPSHOT: 0x01,
  TAP_ACCEPTED: 0x02,
  TAP_REJECTED: 0x03,
  GAME_STATE_CHANGE: 0x04,
  RESTART_STATUS: 0x05,
  PLAYER_JOIN: 0x06,
  PLAYER_LEAVE: 0x07,
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
