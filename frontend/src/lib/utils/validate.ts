// 与 internal/uiapi/validate.go 对齐的前端校验，尽量在提交前拦住明显错误。

const CLUSTER_ID_RE = /^[a-zA-Z0-9._-]{1,64}$/
const JOB_ID_RE = /^[a-zA-Z0-9._-]{1,256}$/
const NAME_MAX = 64
const REGION_MAX = 64
const NS_MAX = 128

export function isValidClusterID(id: string): boolean {
  return CLUSTER_ID_RE.test(id.trim())
}

export function isValidJobID(id: string): boolean {
  return JOB_ID_RE.test(id.trim())
}

export function normalizeAddress(addr: string): string {
  const a = addr.trim()
  if (!a) return ''
  if (a.startsWith('http://') || a.startsWith('https://')) return a
  return `http://${a}`
}

export function addressLooksHTTPS(addr: string): boolean {
  return addr.trim().toLowerCase().startsWith('https://')
}

/** 返回规范化地址，或错误原因 key（由调用方 t()）。 */
export function validateAddress(
  addr: string,
): { ok: true; value: string } | { ok: false; reason: 'empty' | 'host' | 'port' | 'parse' } {
  const raw = addr.trim()
  if (!raw) return { ok: false, reason: 'empty' }
  let a = raw
  if (!a.startsWith('http://') && !a.startsWith('https://')) a = `http://${a}`
  let u: URL
  try {
    u = new URL(a)
  } catch {
    return { ok: false, reason: 'parse' }
  }
  if (!u.hostname) return { ok: false, reason: 'host' }
  if (u.port) {
    const n = Number(u.port)
    if (!Number.isInteger(n) || n < 1 || n > 65535) return { ok: false, reason: 'port' }
  }
  return { ok: true, value: a }
}

export function validateClusterName(name: string): boolean {
  return name.length <= NAME_MAX
}

export function validateRegion(region: string): boolean {
  if (!region) return true
  if (region.length > REGION_MAX) return false
  return !/[ /\\]/.test(region)
}

export function validateNamespace(ns: string): boolean {
  if (!ns) return true
  if (ns.length > NS_MAX) return false
  return !/[ /\\]/.test(ns)
}

export type DockerFormIssues = {
  jobID?: string
  image?: string
  count?: string
  cpu?: string
  memory?: string
  port?: string
  env?: string
  namespace?: string
}

/** 校验 Docker 快速创建表单；返回字段 → i18n key（非文案）。 */
export function validateDockerForm(
  form: {
    jobID: string
    image: string
    count: number
    cpu: number
    memory: number
    portLabel: string
    portTo: number | null
    envText: string
  },
  namespace = '',
): DockerFormIssues {
  const issues: DockerFormIssues = {}
  const jobID = form.jobID.trim()
  if (!jobID) issues.jobID = 'runJob.err.jobIDRequired'
  else if (!isValidJobID(jobID)) issues.jobID = 'runJob.err.jobIDFormat'

  if (!form.image.trim()) issues.image = 'runJob.err.imageRequired'

  if (!Number.isFinite(form.count) || form.count < 1) issues.count = 'runJob.err.count'
  if (!Number.isFinite(form.cpu) || form.cpu < 1) issues.cpu = 'runJob.err.cpu'
  if (!Number.isFinite(form.memory) || form.memory < 1) issues.memory = 'runJob.err.memory'

  const hasLabel = form.portLabel.trim() !== ''
  const hasPort = form.portTo != null && Number.isFinite(form.portTo)
  if (hasLabel !== hasPort) issues.port = 'runJob.err.portPair'
  if (hasPort) {
    const p = form.portTo as number
    if (p < 1 || p > 65535) issues.port = 'runJob.err.portRange'
  }

  const envBad = parseEnvIssues(form.envText)
  if (envBad > 0) issues.env = 'runJob.err.envLines'

  if (!validateNamespace(namespace.trim())) issues.namespace = 'runJob.err.namespace'

  return issues
}

/** 返回无法解析的环境变量行数（非空且不含 =）。 */
export function parseEnvIssues(envText: string): number {
  let n = 0
  for (const line of envText.split('\n')) {
    const s = line.trim()
    if (!s || s.startsWith('#')) continue
    if (!s.includes('=')) n++
  }
  return n
}

export function issuesList(issues: DockerFormIssues): string[] {
  return Object.values(issues).filter(Boolean) as string[]
}
