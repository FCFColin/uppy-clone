import { MSG_TYPE } from './constants.js';
import { handlePong } from './ws_connection.js';
import { handleSnapshot } from './ws_handlers_snapshot.js';
import { handleGameStateChange, handleRestartStatus } from './ws_handlers_phase.js';
import {
  handlePlayerJoin, handlePlayerLeave,
  handleTapAccepted, handleTapRejected,
} from './ws_handlers_events.js';

export function handleBinaryMessage(buffer: ArrayBuffer): void {
  const view: DataView = new DataView(buffer);
  const msgType: number = view.getUint8(0);

  switch (msgType) {
    case MSG_TYPE.SNAPSHOT:
      handleSnapshot(view);
      break;
    case MSG_TYPE.PLAYER_JOIN:
      handlePlayerJoin(view);
      break;
    case MSG_TYPE.PLAYER_LEAVE:
      handlePlayerLeave(view);
      break;
    case MSG_TYPE.TAP_ACCEPTED:
      handleTapAccepted(view);
      break;
    case MSG_TYPE.TAP_REJECTED:
      handleTapRejected();
      break;
    case MSG_TYPE.GAME_STATE_CHANGE:
      handleGameStateChange(view);
      break;
    case MSG_TYPE.RESTART_STATUS:
      handleRestartStatus(view);
      break;
    case MSG_TYPE.PONG:
      handlePong();
      break;
    default:
      console.warn('Unknown message type:', msgType);
  }
}
