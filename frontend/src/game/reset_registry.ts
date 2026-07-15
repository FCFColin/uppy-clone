/**
 * Reset registry for modules that have circular dependencies with state_interp.ts.
 *
 * Modules like renderer.ts and visual_helpers.ts import from state_interp.ts,
 * so state_interp.ts cannot import their reset functions directly without
 * creating a circular dependency. Instead, these modules register their reset
 * functions here at module-load time, and resetClientState() calls
 * runRegisteredResets() to invoke them all.
 */

type ResetFn = () => void;

const registeredResets: ResetFn[] = [];

export function registerResetFn(fn: ResetFn): void {
  registeredResets.push(fn);
}

export function runRegisteredResets(): void {
  for (const fn of registeredResets) {
    fn();
  }
}
