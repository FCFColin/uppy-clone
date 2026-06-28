import { handleBinaryMessage } from './ws_handlers.js';

const pending: ArrayBuffer[] = [];
const MAX_PENDING = 32;

export function enqueueBinaryMessage(data: ArrayBuffer): void {
  pending.push(data);
  if (pending.length > MAX_PENDING) {
    pending.shift();
  }
}

export function drainPendingMessages(budget: number): void {
  let processed = 0;
  while (processed < budget && pending.length > 0) {
    const data = pending.shift();
    if (data) {
      handleBinaryMessage(data);
    } /* v8 ignore else -- shift only returns undefined on empty queue */
    processed++;
  }
}
