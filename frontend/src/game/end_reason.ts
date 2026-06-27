export const END_REASON = {
  NONE: 0,
  GROUND: 1,
  BIRD: 2,
  GHOST: 3,
} as const;

const LABELS: Record<number, string> = {
  [END_REASON.GROUND]: '气球落地',
  [END_REASON.BIRD]: '被鸟撞到',
  [END_REASON.GHOST]: '被幽灵碰到',
};

export function endReasonLabel(code: number): string {
  return LABELS[code] ?? '';
}
