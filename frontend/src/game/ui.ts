import { PALETTE_COLORS } from './constants.js';
import { state } from './state.js';
import { calculateCooldown } from './protocol.js';

const NICKNAME_ADJECTIVES: readonly string[] = [
  '快乐的', '勇敢的', '神秘的', '聪明的', '幸运的', '飞翔的', '闪耀的', '敏捷的',
  '温柔的', '狂野的', '优雅的', '调皮的', '冷静的', '热情的', '沉默的', '活泼的',
  '机智的', '憨厚的', '灵巧的', '威武的', '慵懒的', '专注的', '飘逸的', '坚定的',
  '好奇的', '悠闲的', '霸气的', '呆萌的', '睿智的', '潇洒的', '顽强的',
  '璀璨的', '朦胧的', '炽热的', '冰冷的', '迅捷的', '沉稳的', '天真的', '深沉的',
  '从容的', '执着的', '豪迈的',
  '灵动的', '梦幻的', '寂静的', '奔放的', '细腻的', '豪放的', '清新的', '绚烂的',
  '悠然的', '坚韧的', '开朗的', '内敛的', '浪漫的', '朴实的', '华丽的', '素雅的',
] as const;

const NICKNAME_ANIMALS: readonly string[] = [
  '气球', '老鹰', '海豚', '狐狸', '熊猫', '猫咪', '小鹿', '飞鸟',
  '鲸鱼', '蝴蝶', '松鼠', '兔子', '猫头鹰', '企鹅', '海龟', '萤火虫',
  '刺猬', '海鸥', '燕子', '知更鸟', '独角兽', '龙猫',
  '小熊', '浣熊', '灰狼', '雪豹',
] as const;

const NICKNAME_JOBS: readonly string[] = [
  '探险家', '飞行员', '冒险者', '梦想家', '旅行者', '守护者', '追逐者', '航海家',
  '工程师', '艺术家', '音乐家', '诗人', '骑士', '游侠', '法师', '炼金师',
  '天文学家', '园丁', '面包师', '钟表匠', '摄影师', '收藏家',
  '建筑师', '工匠', '猎人', '学者',
] as const;

const NICKNAME_NATURE: readonly string[] = [
  '星辰', '月光', '微风', '晨露', '晚霞', '彩虹', '雪花', '阳光',
  '云朵', '海浪', '山峦', '森林', '溪流', '花朵', '落叶', '流星',
  '极光', '春雨', '夏日', '秋风', '冬雪', '朝雾',
  '星空', '银河', '潮汐', '晨曦',
] as const;

const NICKNAME_SCIFI: readonly string[] = [
  '量子', '光子', '星舰', '虫洞', '星云', '黑洞', '彗星', '卫星',
  '反应堆', '引擎', '芯片', '代码', '像素', '数据', '信号', '频段',
  '轨道', '空间站', '传送门', '力场', '激光', '等离子',
  '中子', '超新星', '陨石', '星尘',
] as const;

const NICKNAME_CATEGORIES: readonly (readonly string[])[] = [
  NICKNAME_ANIMALS, NICKNAME_JOBS, NICKNAME_NATURE, NICKNAME_SCIFI,
] as const;

export const $waitingScreen: HTMLElement = document.getElementById('waiting-screen')!;
export const $endedScreen: HTMLElement = document.getElementById('ended-screen')!;
export const $gameHud: HTMLElement = document.getElementById('game-hud')!;
const $cooldownIndicator: HTMLElement = document.getElementById('cooldown-indicator')!;
const $cooldownBar: HTMLElement = document.getElementById('cooldown-bar')!;
const $cooldownText: HTMLElement = document.getElementById('cooldown-text')!;
export const $rotateHint: HTMLElement = document.getElementById('rotate-hint')!;
export const $lobbyCode: HTMLElement = document.getElementById('lobby-code')!;
export const $hudCode: HTMLElement = document.getElementById('hud-code')!;
const $hudScore: HTMLElement = document.getElementById('hud-score')!;
const $hudPlayers: HTMLElement = document.getElementById('hud-players')!;
const $hudPlayerList: HTMLElement = document.getElementById('hud-player-list')!;
const $finalScore: HTMLElement = document.getElementById('final-score')!;
const $endPlayerList: HTMLElement = document.getElementById('end-player-list')!;
const $playerListWaiting: HTMLElement = document.getElementById('player-list-waiting')!;
export const $copyCodeBtn: HTMLElement | null = document.getElementById('copy-code-btn');
export const $hudCopyBtn: HTMLElement | null = document.getElementById('hud-copy-btn');
export const $nicknameInline: HTMLElement | null = document.getElementById('nickname-inline');
export const $nicknameInput: HTMLInputElement = document.getElementById('nickname-input') as HTMLInputElement;
export const $nicknameBtn: HTMLElement = document.getElementById('nickname-btn')!;
export const $nicknameSetupScreen: HTMLElement | null = document.getElementById('nickname-setup-screen');
export const $setupNicknameInput: HTMLInputElement | null = document.getElementById('setup-nickname-input') as HTMLInputElement | null;

export function updateUI(): void {
  if (state.connectionError && $waitingScreen) {
    const errorEl: Element | null = $waitingScreen.querySelector('.error-message');
    if (errorEl) errorEl.textContent = state.connectionError;
  }

  $waitingScreen.classList.toggle('hidden', state.phase !== 'waiting');
  $endedScreen.classList.toggle('hidden', state.phase !== 'ended');
  $gameHud.classList.toggle('hidden', state.phase === 'waiting');
  $cooldownIndicator.classList.toggle('hidden', state.phase !== 'playing');

  $hudScore.textContent = String(state.score);
  $hudPlayers.textContent = String(state.players.length);

  if (state.phase === 'ended') {
    $finalScore.textContent = String(state.score);
    updateEndPlayerList();
  }

  if (state.phase === 'ended' && state.restartVotes) {
    const $restartProgress: HTMLElement | null = document.getElementById('restart-progress');
    const $restartCountdown: HTMLElement | null = document.getElementById('restart-countdown');
    if ($restartProgress) {
      $restartProgress.textContent = `${state.restartVotes.yes}/${state.restartVotes.total} 玩家同意重启`;
    }
    if (state.restartVotes.countdownMs <= 0) {
      if ($restartCountdown) $restartCountdown.textContent = '';
    }
  }

  updatePlayerList();

  if (state.phase === 'waiting') {
    updateWaitingPlayerList();
  }
}

function updatePlayerList(): void {
  $hudPlayerList.textContent = '';
  for (const p of state.players) {
    const color: string = PALETTE_COLORS[p.palette % PALETTE_COLORS.length]!;
    const div: HTMLDivElement = document.createElement('div');
    div.className = 'player-item';
    const dot: HTMLSpanElement = document.createElement('span');
    dot.className = 'player-dot';
    dot.style.background = color;
    const name: HTMLSpanElement = document.createElement('span');
    name.className = 'player-name';
    name.textContent = p.nickname || 'P' + p.playerIndex;
    const score: HTMLSpanElement = document.createElement('span');
    score.className = 'player-score';
    score.textContent = String(p.scoreContribution);
    div.appendChild(dot);
    div.appendChild(name);
    div.appendChild(score);
    $hudPlayerList.appendChild(div);
  }
}

function updateWaitingPlayerList(): void {
  $playerListWaiting.textContent = '';
  for (const p of state.players) {
    const color: string = PALETTE_COLORS[p.palette % PALETTE_COLORS.length]!;
    const div: HTMLDivElement = document.createElement('div');
    div.className = 'player-item';
    const dot: HTMLSpanElement = document.createElement('span');
    dot.className = 'player-dot';
    dot.style.background = color;
    const name: HTMLSpanElement = document.createElement('span');
    name.className = 'player-name';
    name.textContent = p.nickname || 'P' + p.playerIndex;
    div.appendChild(dot);
    div.appendChild(name);
    $playerListWaiting.appendChild(div);
  }
}

function updateEndPlayerList(): void {
  const sorted = [...state.players].sort((a, b) => b.scoreContribution - a.scoreContribution);
  $endPlayerList.innerHTML = '';
  for (const p of sorted) {
    const color: string = PALETTE_COLORS[p.palette % PALETTE_COLORS.length]!;
    const div: HTMLDivElement = document.createElement('div');
    div.className = 'player-item';
    const dot: HTMLSpanElement = document.createElement('span');
    dot.className = 'player-dot';
    dot.style.background = color;
    const name: HTMLSpanElement = document.createElement('span');
    name.className = 'player-name';
    name.textContent = p.nickname || 'P' + p.playerIndex;
    const score: HTMLSpanElement = document.createElement('span');
    score.className = 'player-score';
    score.textContent = String(p.scoreContribution);
    div.appendChild(dot);
    div.appendChild(name);
    div.appendChild(score);
    $endPlayerList.appendChild(div);
  }
}

export function updateCooldownBar(): void {
  const now: number = Date.now();
  if (now < state.myCooldownEnd) {
    const remaining: number = state.myCooldownEnd - now;
    const total: number = calculateCooldown(state.players.length);
    const pct: number = Math.min(100, (remaining / total) * 100);
    $cooldownBar.style.width = pct + '%';
    $cooldownBar.classList.remove('ready');
    $cooldownText.textContent = (remaining / 1000).toFixed(1) + 's';
  } else {
    $cooldownBar.style.width = '0%';
    $cooldownBar.classList.add('ready');
    $cooldownText.textContent = 'Tap!';
  }
}

export function startCountdownTimer(seconds: number): void {
  if (state.countdownTimerInterval !== null) {
    clearInterval(state.countdownTimerInterval);
    state.countdownTimerInterval = null;
  }
  const countdownEl: HTMLElement | null = document.getElementById('countdown-overlay');
  if (!countdownEl) return;
  const numberEl: Element | null = countdownEl.querySelector('.countdown-number');
  let remaining: number = seconds;
  if (numberEl) numberEl.textContent = String(remaining);
  countdownEl.style.display = 'flex';

  state.countdownTimerInterval = setInterval(() => {
    remaining--;
    if (remaining <= 0) {
      clearInterval(state.countdownTimerInterval!);
      state.countdownTimerInterval = null;
      countdownEl.style.display = 'none';
    } else {
      if (numberEl) numberEl.textContent = String(remaining);
    }
  }, 1000);
}

export function hideCountdownOverlay(): void {
  const countdownEl: HTMLElement | null = document.getElementById('countdown-overlay');
  if (countdownEl) countdownEl.style.display = 'none';
}

export function showCountdownOverlay(): void {
  const countdownEl: HTMLElement | null = document.getElementById('countdown-overlay');
  if (countdownEl) countdownEl.style.display = 'flex';
}

export function hideLoadingOverlay(): void {
  const loadingOverlay: HTMLElement | null = document.getElementById('loading-overlay');
  if (loadingOverlay) loadingOverlay.style.display = 'none';
}

export function generateRandomNickname(): string {
  const adj: string = NICKNAME_ADJECTIVES[Math.floor(Math.random() * NICKNAME_ADJECTIVES.length)]!;
  const category: readonly string[] = NICKNAME_CATEGORIES[Math.floor(Math.random() * NICKNAME_CATEGORIES.length)]!;
  const noun: string = category[Math.floor(Math.random() * category.length)]!;
  return adj + noun;
}

export function copyCode(): void {
  const url: string = `${window.location.origin}/play.html?code=${state.lobbyCode}`;
  navigator.clipboard.writeText(url).catch(() => {});
}

export function checkOrientation(): void {
  if (window.innerHeight > window.innerWidth * 1.2 && window.innerWidth < 768) {
    $rotateHint.classList.remove('hidden');
  } else {
    $rotateHint.classList.add('hidden');
  }
}

export function showFallbackErrorScreen(message: string): void {
  if (document.getElementById('game-fallback-error')) return;
  const overlay: HTMLDivElement = document.createElement('div');
  overlay.id = 'game-fallback-error';
  overlay.style.cssText = 'position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.8);z-index:99999;display:flex;align-items:center;justify-content:center;flex-direction:column;color:#fff;font-family:sans-serif;';

  const h2: HTMLHeadingElement = document.createElement('h2');
  h2.style.marginBottom = '1rem';
  h2.textContent = '\u{1F635} 出错了';

  const p: HTMLParagraphElement = document.createElement('p');
  p.style.cssText = 'margin-bottom:1.5rem;color:#ccc;';
  p.textContent = message;

  const btn: HTMLButtonElement = document.createElement('button');
  btn.style.cssText = 'padding:0.8rem 2rem;font-size:1rem;cursor:pointer;border:none;border-radius:8px;background:#0f3460;color:#fff;';
  btn.textContent = '刷新页面';
  btn.onclick = () => location.reload();

  overlay.appendChild(h2);
  overlay.appendChild(p);
  overlay.appendChild(btn);
  document.body.appendChild(overlay);
}
