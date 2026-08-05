import editorWorker from 'monaco-editor/editor/editor.worker.js?worker'
import jsonWorker from 'monaco-editor/language/json/json.worker.js?worker'

let configured = false

/** Vite worker 入口：按 monaco@0.56 exports 映射到 esm/vs/*。 */
export function ensureMonacoEnvironment(): void {
  if (configured) return
  configured = true
  self.MonacoEnvironment = {
    getWorker(_workerId: string, label: string) {
      if (label === 'json') return new jsonWorker()
      return new editorWorker()
    },
  }
}
