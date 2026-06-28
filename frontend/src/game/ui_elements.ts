import {
  NICKNAME_ADJECTIVES,
  NICKNAME_CATEGORIES,
} from '../shared/nickname_pools_gen.js';

export const $waitingScreen: HTMLElement = document.getElementById('waiting-screen')!;
export const $endedScreen: HTMLElement = document.getElementById('ended-screen')!;
export const $gameHud: HTMLElement = document.getElementById('game-hud')!;
export const $cooldownIndicator: HTMLElement = document.getElementById('cooldown-indicator')!;
export const $cooldownBar: HTMLElement = document.getElementById('cooldown-bar')!;
export const $cooldownText: HTMLElement = document.getElementById('cooldown-text')!;
export const $lobbyCode: HTMLElement = document.getElementById('lobby-code')!;
export const $hudCode: HTMLElement = document.getElementById('hud-code')!;
export const $hudScore: HTMLElement = document.getElementById('hud-score')!;
export const $hudPlayers: HTMLElement = document.getElementById('hud-players')!;
export const $hudPlayerList: HTMLElement = document.getElementById('hud-player-list')!;
export const $finalScore: HTMLElement = document.getElementById('final-score')!;
export const $endPlayerList: HTMLElement = document.getElementById('end-player-list')!;
export const $playerListWaiting: HTMLElement = document.getElementById('player-list-waiting')!;
export const $copyCodeBtn: HTMLElement | null = document.getElementById('copy-code-btn');
export const $hudCopyBtn: HTMLElement | null = document.getElementById('hud-copy-btn');
export const $nicknameSetupScreen: HTMLElement | null = document.getElementById('nickname-setup-screen');
export const $setupNicknameInput: HTMLInputElement | null = document.getElementById('setup-nickname-input') as HTMLInputElement | null;

export function pickRandomNickname(): string {
  const adj: string = NICKNAME_ADJECTIVES[Math.floor(Math.random() * NICKNAME_ADJECTIVES.length)]!;
  const category: readonly string[] = NICKNAME_CATEGORIES[Math.floor(Math.random() * NICKNAME_CATEGORIES.length)]!;
  const noun: string = category[Math.floor(Math.random() * category.length)]!;
  return adj + noun;
}
