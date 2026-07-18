import js from '@eslint/js';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    ignores: ['dist/**', 'node_modules/**'],
  },
  {
    files: ['src/**/*.{ts,tsx}'],
    rules: {
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
      // shared-012: Upgrade from 'warn' to 'error' to enforce type safety.
      '@typescript-eslint/no-explicit-any': 'error',
    },
  },
  {
    // Allow `any` in test files where mocking and dynamic typing are common.
    files: ['src/**/*.test.ts', 'src/**/*.property.test.ts', 'src/**/*_test_setup.ts'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'warn',
    },
  },
);
