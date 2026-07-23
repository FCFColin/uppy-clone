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

export function resetRegistryForTests(): void {
  registeredResets.length = 0;
}
