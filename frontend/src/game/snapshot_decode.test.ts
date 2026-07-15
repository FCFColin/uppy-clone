import { describe, it, expect } from 'vitest';
import { PHASE_CODE } from '../shared/game/protocol.js';
import { textEncoder, applySnapshot, decodeSnapshot } from './message_codec.js';

function buildSnapshotBuffer(extraBytes = 0): ArrayBuffer {
  return new ArrayBuffer(40 + extraBytes);
}

function writeBaseSnapshot(dv: DataView, phaseCode: number, timestamp = 100, score = 42): number {
  let o = 1;
  dv.setUint32(o, timestamp, true); o += 4;
  dv.setUint32(o, score, true); o += 4;
  dv.setUint8(o, phaseCode); o += 1;
  dv.setFloat32(o, 0.5, true); o += 4;
  dv.setFloat32(o, 0.6, true); o += 4;
  dv.setFloat32(o, -0.1, true); o += 4;
  dv.setFloat32(o, 0.2, true); o += 4;
  dv.setUint8(o, 0); o += 1;
  dv.setUint8(o, 0); o += 1;
  dv.setUint8(o, 0); o += 1;
  dv.setUint8(o, 0); o += 1;
  dv.setFloat32(o, 0, true); o += 4;
  return o;
}

describe('decodeSnapshot', () => {
  it('returns null for buffers shorter than 37 bytes', () => {
    expect(decodeSnapshot(new DataView(new ArrayBuffer(10)))).toBeNull();
  });

  it('decodes core fields from a minimal snapshot', () => {
    const buf = buildSnapshotBuffer();
    writeBaseSnapshot(new DataView(buf), PHASE_CODE.PLAYING);
    const decoded = decodeSnapshot(new DataView(buf));
    expect(decoded).not.toBeNull();
    expect(decoded!.phase).toBe('playing');
    expect(decoded!.timestamp).toBe(100);
    expect(decoded!.score).toBe(42);
    expect(decoded!.balloon.y).toBeCloseTo(0.6);
    expect(decoded!.balloon.x).toBeCloseTo(0.5);
    expect(decoded!.balloon.vx).toBeCloseTo(-0.1);
    expect(decoded!.balloon.vy).toBeCloseTo(0.2);
    expect(decoded!.ripples).toEqual([]);
    expect(decoded!.wind).toBe(0);
  });

  it('applySnapshot copies decoded entities into client state', () => {
    const buf = buildSnapshotBuffer();
    writeBaseSnapshot(new DataView(buf), PHASE_CODE.PLAYING);
    const decoded = decodeSnapshot(new DataView(buf))!;
    const target = {
      score: 0,
      balloon: { x: 0, y: 0, vx: 0, vy: 0 },
      bird: { active: false, x: 0, y: 0 },
      ghost: { active: false, x: 0, y: 0, repelTimer: 0 },
      players: [] as Array<{ playerIndex: number; cooldownEndTime: number; palette: number; scoreContribution: number; nickname: string }>,
    };
    applySnapshot(decoded, target);
    expect(target.score).toBe(42);
    expect(target.balloon.y).toBeCloseTo(0.6);
    expect(target.balloon.vx).toBeCloseTo(-0.1);
    expect(target.balloon.vy).toBeCloseTo(0.2);
  });

  it('decodes active bird coordinates', () => {
    const buf = buildSnapshotBuffer(8);
    const dv = new DataView(buf);
    let o = 1;
    dv.setUint32(o, 100, true); o += 4;
    dv.setUint32(o, 10, true); o += 4;
    dv.setUint8(o, PHASE_CODE.PLAYING); o += 1;
    dv.setFloat32(o, 0.5, true); o += 4;
    dv.setFloat32(o, 0.6, true); o += 4;
    dv.setFloat32(o, 0, true); o += 4;
    dv.setFloat32(o, 0, true); o += 4;
    dv.setUint8(o, 1); o += 1;
    dv.setFloat32(o, 0.1, true); o += 4;
    dv.setFloat32(o, 0.9, true); o += 4;
    dv.setUint8(o, 0); o += 1;
    dv.setFloat32(o, 0.5, true); o += 4;
    dv.setFloat32(o, 0.5, true); o += 4;
    dv.setUint16(o, 0, true); o += 2;
    dv.setUint8(o, 0);

    const decoded = decodeSnapshot(new DataView(buf));
    expect(decoded!.bird.active).toBe(true);
    expect(decoded!.bird.x).toBeCloseTo(0.1);
    expect(decoded!.bird.y).toBeCloseTo(0.9);
  });

  it('decodes players, ripples, and wind when present', () => {
    const nick = 'Alice';
    const nickBytes = textEncoder.encode(nick);
    const playerBytes = 2 + 4 + 4 + 4 + 1 + nickBytes.length;
    const tailBytes = 1 + 2 + 4 + 4 + 4;
    const buf = new ArrayBuffer(40 + playerBytes + tailBytes);
    const dv = new DataView(buf);
    const baseEnd = writeBaseSnapshot(dv, PHASE_CODE.PLAYING);
    let o = baseEnd - 6;
    dv.setUint8(o, 1); o += 1;
    dv.setUint16(o, 3, true); o += 2;
    dv.setUint32(o, 500, true); o += 4;
    dv.setUint32(o, 7, true); o += 4;
    dv.setUint32(o, 12, true); o += 4;
    dv.setUint8(o, nickBytes.length); o += 1;
    new Uint8Array(buf, o).set(nickBytes); o += nickBytes.length;
    dv.setUint8(o, 1); o += 1;
    dv.setUint16(o, 3, true); o += 2;
    dv.setFloat32(o, 0.4, true); o += 4;
    dv.setFloat32(o, 0.6, true); o += 4;
    dv.setFloat32(o, 0.75, true); o += 4;

    const decoded = decodeSnapshot(new DataView(buf, 0, o));
    expect(decoded!.playerCount).toBe(1);
    expect(decoded!.ripples).toHaveLength(1);
    expect(decoded!.wind).toBeCloseTo(0.75);
    expect(decoded!.players[0]!.nickname).toBe('Alice');
    expect(decoded!.players[0]!.playerIndex).toBe(3);
  });
});
