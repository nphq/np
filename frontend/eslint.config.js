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
      'bindings/**', // Wails 自动生成，勿 lint / format
      '.bindings-tmp-*/**', // wails3 generate 临时目录
      '.svelte-kit/**',
      'node_modules/**',
    ],
  },

  js.configs.recommended,
  // strict 比 recommended 更接近开源仓库基线（仍不含 type-checked，避免 Svelte 工程摩擦）
  ...tseslint.configs.strict,
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

  // Svelte 5 runes 模块（.svelte.ts/.svelte.js）
  {
    files: ['**/*.svelte.ts', '**/*.svelte.js'],
    languageOptions: {
      parser: tseslint.parser,
    },
  },

  // 全局：browser + Wails
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
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
      // Wails 绑定与 IPC 边界仍有 any；降为 warn，避免新代码无意识扩散
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-non-null-assertion': 'warn',
      'no-empty': ['error', { allowEmptyCatch: true }],
      eqeqeq: ['error', 'smart'],
      'no-console': ['warn', { allow: ['warn', 'error'] }],
      'prefer-const': 'error',
    },
  },

  // Svelte 5：$props() 必须用 let 解构（可被父组件更新），prefer-const 会误报
  {
    files: ['**/*.svelte'],
    rules: {
      'prefer-const': 'off',
    },
  },

  // 关掉所有与 prettier 冲突的格式规则
  prettier,
)
