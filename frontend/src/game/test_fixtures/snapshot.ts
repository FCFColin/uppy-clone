import { MSG_TYPE } from '../../shared/game/protocol.js';

/**
 * Minimal binary snapshot frame for WS handler integration tests.
 *
 * Layout follows EncodeSnapshot in backend/internal/protocol/encode.go:
 *   msgType(1) + tickCount(4) + score(4) + phaseCode(1)
 *   + balloon: x(4) + y(4) + vx(4) + vy(4)
 *   + bird: active(1) [inactive → no x/y]
 *   + ghost: active(1) [inactive → no x/y/repelTimer]
 *   + playerCount(1) + rippleCount(1) + wind(4)
 *
 * bird/ghost set inactive (active=0) so their conditional x/y/repelTimer
 * fields are omitted. playerCount=0 and rippleCount=0 are packed into the
 * first two bytes of the float32 at offset 28 (0.5 → LE bytes [0x00,0x00,0x00,0x3F]).
 * Remaining bytes are padding to exceed the 37-byte decoder minimum.
 */
export function buildMinimalSnapshot(phaseCode: number, timestamp = 100): ArrayBuffer {
  const buf = new ArrayBuffer(44);
  const dv = new DataView(buf);
  dv.setUint8(0, MSG_TYPE.SNAPSHOT);           // msgType
  let o = 1;
  dv.setUint32(o, timestamp, true); o += 4;     // tickCount
  dv.setUint32(o, 42, true); o += 4;            // score (default 42)
  dv.setUint8(o, phaseCode); o += 1;            // phaseCode
  dv.setFloat32(o, 0.5, true); o += 4;          // balloon.x
  dv.setFloat32(o, 0.6, true); o += 4;          // balloon.y
  dv.setFloat32(o, 0.0, true); o += 4;          // balloon.vx
  dv.setFloat32(o, 0.0, true); o += 4;          // balloon.vy
  dv.setUint8(o, 0); o += 1;                    // bird.active (0 = inactive)
  dv.setUint8(o, 0); o += 1;                    // ghost.active (0 = inactive)
  // float32(0.5) LE = [0x00, 0x00, 0x00, 0x3F]:
  //   byte 0 → playerCount = 0
  //   byte 1 → rippleCount = 0
  //   bytes 2-3 → first half of wind
  dv.setFloat32(o, 0.5, true); o += 4;          // playerCount + rippleCount + wind[0:1]
  // float32(0.5) LE = [0x00, 0x00, 0x00, 0x3F]:
  //   bytes 0-1 → second half of wind
  //   bytes 2-3 → padding
  dv.setFloat32(o, 0.5, true); o += 4;          // wind[2:3] + padding
  dv.setUint16(o, 0, true); o += 2;             // padding
  dv.setUint8(o, 0); o += 1;                    // padding
  dv.setUint8(o, 0); o += 1;                    // padding
  dv.setFloat32(o, 0.5, true);                  // padding
  return buf;
}
