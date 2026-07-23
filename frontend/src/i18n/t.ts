import i18n from './index.js';
/** Shorthand for i18n.t — import this instead of i18n.t directly. */
export const t = i18n.t.bind(i18n) as typeof i18n.t;
