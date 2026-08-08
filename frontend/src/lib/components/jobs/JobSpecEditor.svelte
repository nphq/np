<script lang="ts">
  // CodeMirror 6 规格编辑器：JSON / HCL 高亮，跟随 appearance 主题。
  import { onDestroy, onMount } from 'svelte'
  import { EditorView, basicSetup } from 'codemirror'
  import { Compartment, EditorState } from '@codemirror/state'
  import { json } from '@codemirror/lang-json'
  import { oneDark } from '@codemirror/theme-one-dark'
  import { hcl } from 'codemirror-lang-hcl'
  import { appearance } from '../../stores/appearance.svelte'

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
  // $state.raw：异步挂载后要触发 value/lang/theme 同步 effect，且勿代理 EditorView。
  let view = $state.raw<EditorView | undefined>(undefined)
  const langComp = new Compartment()
  const themeComp = new Compartment()
  let applying = false

  function langExtension(lang: 'hcl' | 'json') {
    return lang === 'json' ? json() : hcl()
  }

  function themeExtension(resolved: 'light' | 'dark') {
    return resolved === 'dark' ? oneDark : []
  }

  onMount(() => {
    if (!host) return
    view = new EditorView({
      parent: host,
      state: EditorState.create({
        doc: value,
        extensions: [
          basicSetup,
          EditorView.lineWrapping,
          langComp.of(langExtension(language)),
          themeComp.of(themeExtension(appearance.resolved)),
          EditorView.theme({
            '&': { height: '100%', fontSize: '12px' },
            '.cm-scroller': {
              fontFamily:
                'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace',
              lineHeight: '18px',
            },
            '.cm-content': { paddingTop: '8px', paddingBottom: '8px' },
            '.cm-gutters': { border: 'none' },
            '&.cm-focused': { outline: 'none' },
          }),
          EditorView.updateListener.of((u) => {
            if (applying || !u.docChanged) return
            value = u.state.doc.toString()
          }),
        ],
      }),
    })
  })

  onDestroy(() => {
    view?.destroy()
    view = undefined
  })

  $effect(() => {
    const v = view
    const resolved = appearance.resolved
    if (!v) return
    v.dispatch({ effects: themeComp.reconfigure(themeExtension(resolved)) })
  })

  $effect(() => {
    const v = view
    const lang = language
    if (!v) return
    v.dispatch({ effects: langComp.reconfigure(langExtension(lang)) })
  })

  // 父组件改 value（格式化 JSON / starter / HCL↔JSON）时推入编辑器。
  $effect(() => {
    const next = value
    const v = view
    if (!v) return
    if (v.state.doc.toString() === next) return
    applying = true
    v.dispatch({
      changes: { from: 0, to: v.state.doc.length, insert: next },
    })
    applying = false
  })
</script>

<div
  class="relative h-[420px] overflow-hidden rounded border {appearance.resolved === 'light'
    ? 'border-zinc-300 bg-white'
    : 'border-zinc-800 bg-[#1e1e1e]'}"
>
  <div bind:this={host} class="cm-host h-full w-full"></div>
  {#if !value && placeholder}
    <div
      class="pointer-events-none absolute top-2 left-10 font-mono text-xs {appearance.resolved ===
      'light'
        ? 'text-zinc-400'
        : 'text-zinc-500'}"
    >
      {placeholder}
    </div>
  {/if}
</div>

<style>
  :global(.cm-host .cm-editor) {
    height: 100%;
  }
  :global(.cm-host .cm-editor.cm-focused) {
    outline: none;
  }
</style>
