import { MSG_TYPE } from '../shared/game/protocol.js';
import { handlePong } from './ws_connection.js';
import { handleSnapshot } from './ws_handlers_snapshot.js';
import { handleGameStateChange, handleRestartStatus } from './ws_handlers_phase.js';
import { handleTapAccepted, handleTapRejected } from './ws_handlers_events.js';

export function handleBinaryMessage(buffer: ArrayBuffer): void {
  const view: DataView = new DataView(buffer);
  const msgType: number = view.getUint8(0);

  switch (msgType) {
    case MSG_TYPE.SNAPSHOT:
      handleSnapshot(view);
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
    case MSG_TYPE.PLAYER_JOIN:
    case MSG_TYPE.PLAYER_LEAVE:
      // Player roster is driven by snapshots; join/leave are server-side events only.
      break;
    default:
      console.warn('Unknown message type:', msgType);
  }
}
