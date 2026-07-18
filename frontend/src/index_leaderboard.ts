import {
  renderLeaderboardEntries,
  fetchLeaderboard,
  type Scope,
  type LeaderboardEntry,
} from './shared/ui/leaderboard_utils.js';

type Entry = LeaderboardEntry;

let scope: Scope = 'global';
let expanded = false;
let fullLoaded = false;
let collapsedEntries: Entry[] = [];

const PREVIEW_LIMIT = 8;
const COLLAPSED_LIMIT = 3;

function setLoading(loading: boolean): void {
  document.getElementById('leaderboard-preview-loading')?.classList.toggle('hidden', !loading);
}

function setActiveTab(next: Scope): void {
  scope = next;
  document.getElementById('lb-preview-global')?.classList.toggle('active', next === 'global');
  document.getElementById('lb-preview-weekly')?.classList.toggle('active', next === 'weekly');
}

function formatCollapsedSummary(entries: Entry[]): string {
  if (!entries.length) return '暂无记录';
  return entries
    .slice(0, COLLAPSED_LIMIT)
    .map((e) => `#${e.rank} ${e.score}分`)
    .join(' · ');
}

function updateCollapsedSummary(text: string): void {
  const el = document.getElementById('lb-collapsed-summary');
  if (el) el.textContent = text;
}

async function loadCollapsedPreview(): Promise<void> {
  updateCollapsedSummary('加载中…');
  try {
    collapsedEntries = await fetchLeaderboard(scope, COLLAPSED_LIMIT);
    updateCollapsedSummary(formatCollapsedSummary(collapsedEntries));
  } catch {
    updateCollapsedSummary('加载失败');
  }
}

async function loadExpandedPreview(): Promise<void> {
  const listEl = document.getElementById('leaderboard-preview');
  const emptyEl = document.getElementById('leaderboard-preview-empty');
  const errorEl = document.getElementById('leaderboard-preview-error');
  if (!listEl) return;

  setLoading(true);
  emptyEl?.classList.add('hidden');
  errorEl?.classList.add('hidden');
  listEl.textContent = '';

  try {
    const entries = await fetchLeaderboard(scope, PREVIEW_LIMIT);
    fullLoaded = true;
    if (!entries.length) {
      emptyEl?.classList.remove('hidden');
      return;
    }
    renderLeaderboardEntries(listEl, entries);
  } catch {
    if (errorEl) {
      errorEl.textContent = '排行榜加载失败，请确认后端已启动';
      errorEl.classList.remove('hidden');
    }
  } finally {
    setLoading(false);
  }
}

function setExpanded(next: boolean): void {
  expanded = next;
  const body = document.getElementById('lb-collapse-body');
  const toggle = document.getElementById('lb-collapse-toggle');
  document.querySelector('.hero')?.classList.toggle('hero--lb-expanded', next);
  body?.classList.toggle('hidden', !next);
  toggle?.setAttribute('aria-expanded', String(next));
  toggle?.classList.toggle('expanded', next);
  if (next && !fullLoaded) {
    void loadExpandedPreview();
  }
}

export function initCollapsibleLeaderboard(): void {
  document.getElementById('lb-collapse-toggle')?.addEventListener('click', () => {
    setExpanded(!expanded);
  });
  document.getElementById('lb-preview-global')?.addEventListener('click', () => {
    setActiveTab('global');
    fullLoaded = false;
    void loadCollapsedPreview();
    if (expanded) void loadExpandedPreview();
  });
  document.getElementById('lb-preview-weekly')?.addEventListener('click', () => {
    setActiveTab('weekly');
    fullLoaded = false;
    void loadCollapsedPreview();
    if (expanded) void loadExpandedPreview();
  });
  void loadCollapsedPreview();
}
