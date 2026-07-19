import { handleBinaryMessage } from './ws_handlers.js';

// --- Outbound Message Queue (for direct manipulation by ws_connection) ---

const outboundMessageQueue: ArrayBuffer[] = [];

export function clearOutboundQueue(): void {
  outboundMessageQueue.length = 0;
}

export function getOutboundQueueLength(): number {
  return outboundMessageQueue.length;
}

export function shiftOutbound(): ArrayBuffer | undefined {
  return outboundMessageQueue.shift();
}

export function pushToOutbound(msg: ArrayBuffer, maxQueue: number): void {
  outboundMessageQueue.push(msg);
  if (outboundMessageQueue.length > maxQueue) {
    outboundMessageQueue.shift();
  }
}

export function hasOutboundMessages(): boolean {
  return outboundMessageQueue.length > 0;
}

export function requeueOutboundFront(msg: ArrayBuffer): void {
  outboundMessageQueue.unshift(msg);
}

// --- Inbound Binary Message Queue ---

const pendingBinaryMessages: ArrayBuffer[] = [];
const MAX_PENDING_BINARY = 32;

export function enqueueBinaryMessage(data: ArrayBuffer): void {
  pendingBinaryMessages.push(data);
  if (pendingBinaryMessages.length > MAX_PENDING_BINARY) {
    const dropped = pendingBinaryMessages.shift();
    if (dropped) {
      console.warn(`[ws] message queue full (${MAX_PENDING_BINARY}), dropping oldest message`);
    }
  }
}

export function drainPendingMessages(budget: number): void {
  let processed = 0;
  while (pendingBinaryMessages.length > 0 && processed < budget) {
    const data = pendingBinaryMessages.shift();
    if (data) {
      try {
        handleBinaryMessage(data);
      } catch (err: unknown) {
        console.error('[ws] message:', err);
      }
    }
    processed++;
  }
}
