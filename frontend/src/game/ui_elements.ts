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
