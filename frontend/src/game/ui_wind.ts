import { isTutorialDone } from '../shared/data/tutorial_cookie.js';
import { PHYSICS } from './constants.js';
import { state } from './state.js';

const WIND_CLAMP = PHYSICS.WIND_CLAMP;
const STRONG_WIND_THRESHOLD = 0.55;

let $windIndicator: HTMLElement | null = null;
let $windDirection: HTMLElement | null = null;
let $windMeterFill: HTMLElement | null = null;
let $windStrength: HTMLElement | null = null;
let $windFirstHint: HTMLElement | null = null;
let windHintShown = false;

function ensureElements(): void {
  if ($windIndicator && !$windIndicator.isConnected) {
    $windIndicator = null;
    $windDirection = null;
    $windMeterFill = null;
    $windStrength = null;
    $windFirstHint = null;
  }
  if (!$windIndicator) $windIndicator = document.getElementById('wind-indicator');
  if (!$windDirection) $windDirection = document.getElementById('wind-direction');
  if (!$windMeterFill) $windMeterFill = document.getElementById('wind-meter-fill');
  if (!$windStrength) $windStrength = document.getElementById('wind-strength');
  if (!$windFirstHint) $windFirstHint = document.getElementById('wind-first-hint');
}

export function updateWindIndicator(wind: number): void {
  ensureElements();
  if (!$windIndicator || !$windDirection || !$windMeterFill || !$windStrength) return;

  const visible = state.phase === 'playing';
  $windIndicator.classList.toggle('hidden', !visible);
  if (!visible) return;

  const clamped = Math.max(-WIND_CLAMP, Math.min(WIND_CLAMP, wind));
  const magnitude = Math.abs(clamped) / WIND_CLAMP;
  const pct = Math.round(magnitude * 100);
  const isCalm = magnitude < 0.08;

  $windDirection.textContent = isCalm ? '·' : clamped >= 0 ? '→' : '←';
  $windDirection.style.color = magnitude >= STRONG_WIND_THRESHOLD ? '#ffb4c4' : '#a8d4ff';
  $windStrength.textContent = `${pct}%`;

  $windMeterFill.classList.toggle('strong', magnitude >= STRONG_WIND_THRESHOLD);

  if (isCalm) {
    $windMeterFill.style.width = '0%';
    $windMeterFill.style.left = '50%';
    $windMeterFill.style.right = 'auto';
  } else if (clamped >= 0) {
    $windMeterFill.style.left = '50%';
    $windMeterFill.style.right = 'auto';
    $windMeterFill.style.width = `${(magnitude * 50).toFixed(1)}%`;
  } else {
    $windMeterFill.style.right = '50%';
    $windMeterFill.style.left = 'auto';
    $windMeterFill.style.width = `${(magnitude * 50).toFixed(1)}%`;
  }

  const dirLabel = isCalm ? '无' : clamped >= 0 ? '东' : '西';
  $windIndicator.title = `风向 ${dirLabel} · 风力 ${pct}%`;

  if (!windHintShown && !isTutorialDone() && $windFirstHint) {
    windHintShown = true;
    $windFirstHint.classList.remove('hidden');
    setTimeout(() => $windFirstHint?.classList.add('hidden'), 3000);
  }
}

export function hideWindIndicator(): void {
  ensureElements();
  if ($windIndicator) $windIndicator.classList.add('hidden');
}
