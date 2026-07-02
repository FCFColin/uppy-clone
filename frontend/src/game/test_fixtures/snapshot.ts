import { MSG_TYPE } from '../constants.js';

/** Minimal binary snapshot frame for WS handler integration tests. */
export function buildMinimalSnapshot(phaseCode: number, timestamp = 100): ArrayBuffer {
  const buf = new ArrayBuffer(40);
  const dv = new DataView(buf);
  dv.setUint8(0, MSG_TYPE.SNAPSHOT);
  let o = 1;
  dv.setUint32(o, timestamp, true); o += 4;
  dv.setUint32(o, 42, true); o += 4;
  dv.setUint8(o, phaseCode); o += 1;
  dv.setFloat32(o, 0.5, true); o += 4;
  dv.setFloat32(o, 0.6, true); o += 4;
  dv.setFloat32(o, 0.0, true); o += 4;
  dv.setFloat32(o, 0.0, true); o += 4;
  dv.setUint8(o, 0); o += 1;
  dv.setUint8(o, 0); o += 1;
  dv.setFloat32(o, 0.5, true); o += 4;
  dv.setFloat32(o, 0.5, true); o += 4;
  dv.setUint16(o, 0, true); o += 2;
  dv.setUint8(o, 0);
  return buf;
}
