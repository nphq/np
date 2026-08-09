<script lang="ts">
  // CodeMirror 6 规格编辑器：JSON / HCL 高亮，跟随 appearance 主题。
  // CodeMirror 是 bundle 里最大的第三方依赖（~300KB），全部走动态 import：
  // 仅在编辑器真正挂载时加载，构建期被拆进独立 chunk，不拖累首屏。
  import { onDestroy, onMount } from 'svelte'
  import type { EditorView } from 'codemirror'
  import type { Compartment, EditorState, Extension } from '@codemirror/state'
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
  let loading = $state(true)
  // $state.raw：异步挂载后要触发 value/lang/theme 同步 effect，且勿代理 EditorView。
  let view = $state.raw<EditorView | undefined>(undefined)

  // 动态模块句柄（模块级缓存，format/starter 等外部操作不经过这里）。
  type CMDeps = {
    EditorView: typeof import('codemirror').EditorView
    basicSetup: Extension
    Compartment: typeof Compartment
    EditorState: typeof EditorState
    json: () => Extension
    oneDark: Extension
    hcl: () => Extension
  }
  let deps = $state.raw<CMDeps | undefined>(undefined)
  let langComp = $state.raw<Compartment | undefined>(undefined)
  let themeComp = $state.raw<Compartment | undefined>(undefined)
  let applying = false

  function langExtension(deps: CMDeps, lang: 'hcl' | 'json'): Extension {
    return lang === 'json' ? deps.json() : deps.hcl()
  }

  function themeExtension(deps: CMDeps, resolved: 'light' | 'dark'): Extension {
    return resolved === 'dark' ? deps.oneDark : []
  }

  onMount(async () => {
    if (!host) return
    const [cm, cmState, langJson, theme, langHcl] = await Promise.all([
      import('codemirror'),
      import('@codemirror/state'),
      import('@codemirror/lang-json'),
      import('@codemirror/theme-one-dark'),
      import('codemirror-lang-hcl'),
    ])
    if (!host) return // 加载期间组件已销毁
    const d: CMDeps = {
      EditorView: cm.EditorView,
      basicSetup: cm.basicSetup,
      Compartment: cmState.Compartment,
      EditorState: cmState.EditorState,
      json: langJson.json,
      oneDark: theme.oneDark,
      hcl: langHcl.hcl,
    }
    deps = d
    langComp = new d.Compartment()
    themeComp = new d.Compartment()
    view = new d.EditorView({
      parent: host,
      state: d.EditorState.create({
        doc: value,
        extensions: [
          d.basicSetup,
          d.EditorView.lineWrapping,
          langComp.of(langExtension(d, language)),
          themeComp.of(themeExtension(d, appearance.resolved)),
          d.EditorView.theme({
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
          d.EditorView.updateListener.of((u) => {
            if (applying || !u.docChanged) return
            value = u.state.doc.toString()
          }),
        ],
      }),
    })
    loading = false
  })

  onDestroy(() => {
    view?.destroy()
    view = undefined
  })

  $effect(() => {
    const v = view
    const d = deps
    const c = themeComp
    const resolved = appearance.resolved
    if (!v || !d || !c) return
    v.dispatch({ effects: c.reconfigure(themeExtension(d, resolved)) })
  })

  $effect(() => {
    const v = view
    const d = deps
    const c = langComp
    const lang = language
    if (!v || !d || !c) return
    v.dispatch({ effects: c.reconfigure(langExtension(d, lang)) })
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
  {#if loading}
    <div class="absolute inset-0 flex items-center justify-center font-mono text-xs text-zinc-500">
      editor…
    </div>
  {/if}
  {#if !value && placeholder && !loading}
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
