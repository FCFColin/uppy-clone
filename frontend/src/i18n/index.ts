import i18n from 'i18next';
import zh from './locales/zh.json';
import en from './locales/en.json';

const LANG_KEY = 'uppy-lang';

function detectLanguage(): string {
  try {
    const saved = localStorage.getItem(LANG_KEY);
    if (saved === 'en' || saved === 'zh') return saved;
  } catch {
    // ignore — storage may be unavailable (private browsing etc.)
  }
  const browserLang = navigator.language || navigator.languages?.[0] || '';
  return browserLang.startsWith('en') ? 'en' : 'zh';
}

void i18n.init({
  resources: {
    zh: { translation: zh },
    en: { translation: en },
  },
  lng: detectLanguage(),
  fallbackLng: 'zh',
  interpolation: { escapeValue: false },
  keySeparator: '.',
});

export function setLanguage(lng: string): void {
  void i18n.changeLanguage(lng);
  try {
    localStorage.setItem(LANG_KEY, lng);
  } catch {
    // ignore — storage may be unavailable (private browsing etc.)
  }
  document.documentElement.lang = lng === 'en' ? 'en' : 'zh-CN';
}

export function getLanguage(): string {
  return i18n.language || 'zh';
}

export function applyTranslations(root: ParentNode = document): void {
  root.querySelectorAll<HTMLElement>('[data-i18n]').forEach((el) => {
    el.textContent = i18n.t(el.dataset['i18n']!);
  });
  root.querySelectorAll<HTMLElement>('[data-i18n-html]').forEach((el) => {
    el.innerHTML = i18n.t(el.dataset['i18nHtml']!);
  });
  root.querySelectorAll<HTMLElement>('[data-i18n-title]').forEach((el) => {
    el.title = i18n.t(el.dataset['i18nTitle']!);
  });
  root.querySelectorAll<HTMLElement>('[data-i18n-aria]').forEach((el) => {
    el.setAttribute('aria-label', i18n.t(el.dataset['i18nAria']!));
  });
  root.querySelectorAll<HTMLInputElement>('[data-i18n-placeholder]').forEach((el) => {
    el.placeholder = i18n.t(el.dataset['i18nPlaceholder']!);
  });
}

export function initLanguageSwitcher(): void {
  const btn = document.querySelector<HTMLElement>('.btn-nav-icon[data-icon="globe"]');
  if (!btn || btn.hasAttribute('data-lang-bound')) return;
  btn.setAttribute('data-lang-bound', 'true');
  btn.addEventListener('click', () => {
    const next = getLanguage() === 'zh' ? 'en' : 'zh';
    setLanguage(next);
    applyTranslations();
  });
}

export default i18n;
