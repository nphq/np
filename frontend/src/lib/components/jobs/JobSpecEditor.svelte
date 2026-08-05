<script lang="ts">
  // Monaco 规格编辑器：JSON 高亮；HCL 用自定义轻量语法。
  import { onDestroy, onMount } from 'svelte'
  import type * as Monaco from 'monaco-editor'
  import { ensureMonacoEnvironment } from '../../jobs/monacoEnv'

  let {
    value = $bindable(''),
    language = 'json',
    placeholder = '',
  }: {
    value: string
    language: 'hcl' | 'json'
    placeholder?: string
  } = $props()

  let host: HTMLDivElement | undefined = $state()
  let editor: Monaco.editor.IStandaloneCodeEditor | undefined
  let monaco: typeof Monaco | undefined
  let applying = false

  onMount(() => {
    void init()
  })

  onDestroy(() => {
    editor?.dispose()
    editor = undefined
  })

  async function init(): Promise<void> {
    ensureMonacoEnvironment()
    const m = await import('monaco-editor')
    monaco = m

    if (!m.languages.getLanguages().some((l) => l.id === 'hcl')) {
      m.languages.register({ id: 'hcl' })
      m.languages.setMonarchTokensProvider('hcl', {
        tokenizer: {
          root: [
            [/#.*$/, 'comment'],
            [/\/\/.*$/, 'comment'],
            [/"(?:[^"\\]|\\.)*"/, 'string'],
            [
              /\b(?:job|group|task|config|resources|network|port|service|check|variable)\b/,
              'keyword',
            ],
            [/\b(?:true|false|null)\b/, 'keyword'],
            [/[0-9]+/, 'number'],
            [/[a-zA-Z_][\w-]*/, 'identifier'],
          ],
        },
      })
    }

    if (!host) return
    editor = m.editor.create(host, {
      value,
      language: language === 'json' ? 'json' : 'hcl',
      theme: 'vs-dark',
      automaticLayout: true,
      minimap: { enabled: false },
      fontSize: 12,
      lineHeight: 18,
      tabSize: 2,
      scrollBeyondLastLine: false,
      wordWrap: 'on',
      padding: { top: 8, bottom: 8 },
      renderLineHighlight: 'line',
      overviewRulerLanes: 0,
      hideCursorInOverviewRuler: true,
      scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8 },
    })

    editor.onDidChangeModelContent(() => {
      if (applying || !editor) return
      value = editor.getValue()
    })
  }

  $effect(() => {
    const lang = language === 'json' ? 'json' : 'hcl'
    const model = editor?.getModel()
    if (model && monaco && model.getLanguageId() !== lang) {
      monaco.editor.setModelLanguage(model, lang)
    }
  })

  $effect(() => {
    if (!editor) return
    if (editor.getValue() === value) return
    applying = true
    editor.setValue(value)
    applying = false
  })
</script>

<div class="relative h-[420px] overflow-hidden rounded border border-zinc-800 bg-[#1e1e1e]">
  <div bind:this={host} class="h-full w-full"></div>
  {#if !value && placeholder}
    <div class="pointer-events-none absolute top-2 left-14 font-mono text-xs text-zinc-600">
      {placeholder}
    </div>
  {/if}
</div>
