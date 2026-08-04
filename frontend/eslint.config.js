import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import svelte from 'eslint-plugin-svelte'
import prettier from 'eslint-config-prettier'
import globals from 'globals'

export default tseslint.config(
  // 全局忽略
  {
    ignores: [
      'dist/**',
      'bindings/**', // Wails 自动生成（frontend/bindings/），勿 lint
      '.svelte-kit/**',
      'node_modules/**',
    ],
  },

  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...svelte.configs['flat/recommended'],

  // Svelte 文件：用 svelte-parser 解析 TS
  {
    files: ['**/*.svelte'],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
      },
    },
  },

  // Svelte 5 runes 模块（.svelte.ts/.svelte.js）：typescrit-eslint parser 原生支持
  {
    files: ['**/*.svelte.ts', '**/*.svelte.js'],
    languageOptions: {
      parser: tseslint.parser,
    },
  },

  // 全局：补 wails runtime / browser 全局变量
  {
    languageOptions: {
      globals: {
        ...globals.browser,
        window: 'readonly',
      },
    },
  },

  // 项目约定
  {
    rules: {
      '@typescript-eslint/no-unused-vars': [
        'warn',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
      '@typescript-eslint/no-explicit-any': 'off', // wails 生成的类型含 any
      'no-empty': ['error', { allowEmptyCatch: true }],
    },
  },

  // 关掉所有与 prettier 冲突的格式规则
  prettier,
)
