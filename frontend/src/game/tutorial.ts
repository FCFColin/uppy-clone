import { markTutorialDone, shouldShowTutorial } from '../shared/data/cookies.js';

const STEPS = [
  '在气球附近点击，把它往上推（点太远无效）',
  '底部冷却条转绿才能再点；人越多冷却越长',
  '躲开鸟和幽灵；靠近幽灵点击可推开；留意风向',
];

let currentStep = 0;
let overlayEl: HTMLElement | null = null;
let showRangeCircle = false;
let rangeCircleUntil = 0;
let resolveWait: (() => void) | null = null;

export function isRangeCircleVisible(): boolean {
  return showRangeCircle && Date.now() < rangeCircleUntil;
}

function getOverlay(): HTMLElement | null {
  if (!overlayEl) overlayEl = document.getElementById('tutorial-overlay');
  return overlayEl;
}

function hideOverlay(): void {
  getOverlay()?.classList.add('hidden');
  showRangeCircle = false;
}

function renderStep(): void {
  const el = getOverlay();
  if (!el) return;
  const textEl = el.querySelector('.tutorial-text');
  const stepEl = el.querySelector('.tutorial-step');
  if (textEl) textEl.textContent = STEPS[currentStep] ?? '';
  if (stepEl) stepEl.textContent = `${currentStep + 1} / ${STEPS.length}`;

  showRangeCircle = currentStep === 0;
  if (showRangeCircle) rangeCircleUntil = Date.now() + 5000;
}

function finishTutorial(): void {
  markTutorialDone();
  hideOverlay();
  resolveWait?.();
  resolveWait = null;
}

function bindButtonsOnce(): void {
  const skipBtn = document.getElementById('tutorial-skip-btn');
  const nextBtn = document.getElementById('tutorial-next-btn');
  if (skipBtn?.dataset.bound) return;
  if (skipBtn) {
    skipBtn.dataset.bound = '1';
    skipBtn.addEventListener('click', finishTutorial);
  }
  if (nextBtn) {
    nextBtn.dataset.bound = '1';
    nextBtn.addEventListener('click', () => {
      currentStep++;
      if (currentStep >= STEPS.length) finishTutorial();
      else renderStep();
    });
  }
}

/** Resolves when tutorial is skipped or completed. No-op if already done. */
export function runTutorialIfNeeded(): Promise<void> {
  bindButtonsOnce();
  return new Promise((resolve) => {
    void (async () => {
      const show = await shouldShowTutorial();
      if (!show) {
        resolve();
        return;
      }
      resolveWait = resolve;
      currentStep = 0;
      getOverlay()?.classList.remove('hidden');
      renderStep();
    })();
  });
}

/** Reset all tutorial state for a new game session. */
export function resetTutorial(): void {
  hideOverlay();
  resolveWait?.();
  resolveWait = null;
  currentStep = 0;
  rangeCircleUntil = 0;
}
