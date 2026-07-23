import { setLanguage } from './i18n/index.js';

// Force Chinese locale in tests so assertions match the existing Chinese translations.
setLanguage('zh');
