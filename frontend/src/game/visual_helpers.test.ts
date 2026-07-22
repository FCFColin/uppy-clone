import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { isCollisionDebugEnabled, drawCollisionDebug } from './visual_helpers.js';

// 跟踪 mock ctx 的调用次数，用于断言 drawCollisionDebug 的行为
const ctxCalls = {
  arcCount: 0,
  ellipseCount: 0,
  strokeCount: 0,
  saveCount: 0,
  restoreCount: 0,
};

function makeMockCtx() {
  return {
    save: () => {
      ctxCalls.saveCount++;
    },
    restore: () => {
      ctxCalls.restoreCount++;
    },
    beginPath: () => {},
    arc: () => {
      ctxCalls.arcCount++;
    },
    ellipse: () => {
      ctxCalls.ellipseCount++;
    },
    stroke: () => {
      ctxCalls.strokeCount++;
    },
    set strokeStyle(_v: string) {},
    set lineWidth(_v: number) {},
    set fillStyle(_v: string) {},
  } as unknown as CanvasRenderingContext2D;
}

describe('isCollisionDebugEnabled', () => {
  const originalSearch = window.location.search;

  afterEach(() => {
    // restore location.search by deleting the property we may have redefined
    if (window.location.search !== originalSearch) {
      Object.defineProperty(window, 'location', {
        value: { search: originalSearch },
        writable: true,
        configurable: true,
      });
    }
  });

  function setLocationSearch(search: string): void {
    Object.defineProperty(window, 'location', {
      value: { search },
      writable: true,
      configurable: true,
    });
  }

  beforeEach(() => {
    ctxCalls.arcCount = 0;
    ctxCalls.ellipseCount = 0;
    ctxCalls.strokeCount = 0;
    ctxCalls.saveCount = 0;
    ctxCalls.restoreCount = 0;
  });

  it('returns true when URL has ?debug=collision', () => {
    setLocationSearch('?debug=collision');
    expect(isCollisionDebugEnabled()).toBe(true);
  });

  it('returns false when URL has no parameters', () => {
    setLocationSearch('');
    expect(isCollisionDebugEnabled()).toBe(false);
  });

  it('returns false when URL has other debug value', () => {
    setLocationSearch('?debug=other');
    expect(isCollisionDebugEnabled()).toBe(false);
  });

  it('returns false when URL has unrelated parameters', () => {
    setLocationSearch('?foo=bar&baz=1');
    expect(isCollisionDebugEnabled()).toBe(false);
  });

  it('returns true when ?debug=collision is among multiple params', () => {
    setLocationSearch('?foo=bar&debug=collision&other=2');
    expect(isCollisionDebugEnabled()).toBe(true);
  });
});

describe('drawCollisionDebug', () => {
  beforeEach(() => {
    ctxCalls.arcCount = 0;
    ctxCalls.ellipseCount = 0;
    ctxCalls.strokeCount = 0;
    ctxCalls.saveCount = 0;
    ctxCalls.restoreCount = 0;
  });

  it('draws balloon circle + bird ellipse + ghost ellipse when all active', () => {
    const ctx = makeMockCtx();
    drawCollisionDebug(
      ctx,
      { x: 0.5, y: 0.5, active: true },
      { x: 0.5, y: 0.5, active: true },
      { x: 0.5, y: 0.5 },
    );
    // 1 balloon arc + 1 bird ellipse + 1 ghost ellipse
    expect(ctxCalls.arcCount).toBe(1);
    expect(ctxCalls.ellipseCount).toBe(2);
    // 3 strokes (one per shape)
    expect(ctxCalls.strokeCount).toBe(3);
    // save/restore balanced
    expect(ctxCalls.saveCount).toBe(1);
    expect(ctxCalls.restoreCount).toBe(1);
  });

  it('skips bird ellipse when bird is inactive', () => {
    const ctx = makeMockCtx();
    drawCollisionDebug(
      ctx,
      { x: 0.5, y: 0.5, active: false },
      { x: 0.5, y: 0.5, active: true },
      { x: 0.5, y: 0.5 },
    );
    expect(ctxCalls.arcCount).toBe(1); // balloon only
    expect(ctxCalls.ellipseCount).toBe(1); // ghost only
    expect(ctxCalls.strokeCount).toBe(2);
  });

  it('skips ghost ellipse when ghost is inactive', () => {
    const ctx = makeMockCtx();
    drawCollisionDebug(
      ctx,
      { x: 0.5, y: 0.5, active: true },
      { x: 0.5, y: 0.5, active: false },
      { x: 0.5, y: 0.5 },
    );
    expect(ctxCalls.arcCount).toBe(1); // balloon only
    expect(ctxCalls.ellipseCount).toBe(1); // bird only
    expect(ctxCalls.strokeCount).toBe(2);
  });

  it('skips bird ellipse when bird is null', () => {
    const ctx = makeMockCtx();
    drawCollisionDebug(
      ctx,
      null,
      { x: 0.5, y: 0.5, active: true },
      { x: 0.5, y: 0.5 },
    );
    expect(ctxCalls.arcCount).toBe(1);
    expect(ctxCalls.ellipseCount).toBe(1);
    expect(ctxCalls.strokeCount).toBe(2);
  });

  it('skips ghost ellipse when ghost is null', () => {
    const ctx = makeMockCtx();
    drawCollisionDebug(
      ctx,
      { x: 0.5, y: 0.5, active: true },
      null,
      { x: 0.5, y: 0.5 },
    );
    expect(ctxCalls.arcCount).toBe(1);
    expect(ctxCalls.ellipseCount).toBe(1);
    expect(ctxCalls.strokeCount).toBe(2);
  });

  it('draws only balloon when both bird and ghost are null', () => {
    const ctx = makeMockCtx();
    drawCollisionDebug(ctx, null, null, { x: 0.5, y: 0.5 });
    expect(ctxCalls.arcCount).toBe(1);
    expect(ctxCalls.ellipseCount).toBe(0);
    expect(ctxCalls.strokeCount).toBe(1);
  });
});
