import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import reactHooks from 'eslint-plugin-react-hooks'
import globals from 'globals'

export default tseslint.config(
  {
    ignores: [
      'dist/**',
      'node_modules/**',
      'src/api/types.gen.ts',
      'src/route-tree.ts',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['**/*.{ts,tsx}'],
    plugins: {
      'react-hooks': reactHooks,
    },
    languageOptions: {
      globals: {
        ...globals.browser,
      },
    },
    rules: {
      // Stable, widely-adopted hooks rules — not the experimental
      // React-Compiler ruleset, which this codebase was not written against.
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
      '@typescript-eslint/no-explicit-any': 'warn',
    },
  },
  // Node build scripts and config files run under Node, not the browser.
  {
    files: ['**/*.{mjs,cjs}', 'scripts/**', '*.config.{js,ts}'],
    languageOptions: {
      globals: {
        ...globals.node,
      },
    },
    rules: {
      // Config files (e.g. tailwind.config.ts) legitimately use require().
      '@typescript-eslint/no-require-imports': 'off',
    },
  },
  // Test files: vitest globals; the suite intentionally exercises constant
  // falsy operands (e.g. `false && 'b'`) to assert utility behaviour.
  {
    files: ['**/*.{test,spec}.{ts,tsx}'],
    languageOptions: {
      globals: {
        ...globals.node,
      },
    },
    rules: {
      'no-constant-binary-expression': 'off',
    },
  },
)
