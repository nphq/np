// Nomad Job JSON 构建 + 精选应用目录（Apps）。
import type { MessageKey } from '../i18n/dictionaries/zh'

export type JobType = 'service' | 'batch' | 'system'

export type AppCategory = 'web' | 'data' | 'observability' | 'messaging' | 'utility'

export interface DockerJobForm {
  jobID: string
  type: JobType
  datacenters: string
  groupName: string
  count: number
  taskName: string
  image: string
  portLabel: string
  portTo: number | null
  cpu: number
  memory: number
  /** 每行 `KEY=VALUE` */
  envText: string
  /** Docker command（可选） */
  command?: string
  /** 空格分隔 args（可选） */
  argsText?: string
}

export interface CatalogApp {
  id: string
  titleKey: MessageKey
  descKey: MessageKey
  category: AppCategory
  /** 列表上展示的镜像标签 */
  imageLabel: string
  kind: 'form' | 'hcl'
  form?: DockerJobForm
  hcl?: string
}

/** @deprecated 用 CatalogApp；保留别名避免旧引用断裂 */
export type JobTemplate = CatalogApp

export function defaultDockerForm(): DockerJobForm {
  return {
    jobID: 'example',
    type: 'service',
    datacenters: 'dc1',
    groupName: 'web',
    count: 1,
    taskName: 'server',
    image: 'nginx:latest',
    portLabel: 'http',
    portTo: 80,
    cpu: 500,
    memory: 256,
    envText: '',
  }
}

function parseDatacenters(raw: string): string[] {
  const list = raw
    .split(/[,\s]+/)
    .map((s) => s.trim())
    .filter(Boolean)
  return list.length > 0 ? list : ['*']
}

function parseEnv(envText: string): Record<string, string> | undefined {
  const env: Record<string, string> = {}
  for (const line of envText.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const eq = trimmed.indexOf('=')
    if (eq <= 0) continue
    env[trimmed.slice(0, eq).trim()] = trimmed.slice(eq + 1).trim()
  }
  return Object.keys(env).length > 0 ? env : undefined
}

/** 把表单变成 Nomad API JSON Job（可直接 format=json 提交）。 */
export function buildDockerJobJSON(form: DockerJobForm): string {
  const id = form.jobID.trim() || 'example'
  const group = form.groupName.trim() || 'web'
  const task = form.taskName.trim() || 'server'
  const image = form.image.trim() || 'nginx:latest'
  const portLabel = form.portLabel.trim()
  const usePort = portLabel !== '' && form.portTo != null && form.portTo > 0

  const config: Record<string, unknown> = { image }
  if (usePort) config.ports = [portLabel]
  const command = form.command?.trim()
  if (command) config.command = command
  const args = (form.argsText ?? '')
    .split(/\s+/)
    .map((s) => s.trim())
    .filter(Boolean)
  if (args.length > 0) config.args = args

  const taskObj: Record<string, unknown> = {
    Name: task,
    Driver: 'docker',
    Config: config,
    Resources: {
      CPU: Math.max(1, Math.floor(form.cpu) || 500),
      MemoryMB: Math.max(1, Math.floor(form.memory) || 256),
    },
  }
  const env = parseEnv(form.envText)
  if (env) taskObj.Env = env

  const groupObj: Record<string, unknown> = {
    Name: group,
    Count: Math.max(1, Math.floor(form.count) || 1),
    Tasks: [taskObj],
  }
  if (usePort) {
    groupObj.Networks = [
      {
        DynamicPorts: [{ Label: portLabel, To: Math.floor(form.portTo!) }],
      },
    ]
  }

  const job = {
    ID: id,
    Name: id,
    Type: form.type,
    Datacenters: parseDatacenters(form.datacenters),
    TaskGroups: [groupObj],
  }
  return JSON.stringify(job, null, 2)
}

export function summarizeDockerForm(form: DockerJobForm): {
  jobID: string
  type: JobType
  image: string
  count: number
} {
  return {
    jobID: form.jobID.trim() || 'example',
    type: form.type,
    image: form.image.trim() || 'nginx:latest',
    count: Math.max(1, Math.floor(form.count) || 1),
  }
}

export function tryFormatJSON(
  spec: string,
): { ok: true; text: string } | { ok: false; error: string } {
  try {
    return { ok: true, text: JSON.stringify(JSON.parse(spec), null, 2) }
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) }
  }
}

export function extractJobIDFromSpec(spec: string, format: 'hcl' | 'json'): string {
  if (format === 'json') {
    try {
      const j = JSON.parse(spec) as { ID?: string; Name?: string }
      return (j.ID || j.Name || '').trim()
    } catch {
      return ''
    }
  }
  const m = spec.match(/job\s+"([^"]+)"/)
  return m?.[1] ?? ''
}

export function findCatalogApp(id: string): CatalogApp | undefined {
  return APP_CATALOG.find((a) => a.id === id)
}

export const STARTER_HCL = `job "example" {
  datacenters = ["dc1"]
  type = "service"

  group "web" {
    count = 1

    network {
      port "http" {
        to = 80
      }
    }

    task "server" {
      driver = "docker"
      config {
        image = "nginx:latest"
        ports = ["http"]
      }
      resources {
        cpu    = 500
        memory = 256
      }
    }
  }
}
`

/** 精选常用应用（本地固化，非远程商店）。 */
export const APP_CATALOG: CatalogApp[] = [
  {
    id: 'nginx',
    titleKey: 'apps.nginx.title',
    descKey: 'apps.nginx.desc',
    category: 'web',
    imageLabel: 'nginx:latest',
    kind: 'form',
    form: {
      ...defaultDockerForm(),
      jobID: 'nginx',
      image: 'nginx:latest',
      portLabel: 'http',
      portTo: 80,
      count: 2,
    },
  },
  {
    id: 'traefik',
    titleKey: 'apps.traefik.title',
    descKey: 'apps.traefik.desc',
    category: 'web',
    imageLabel: 'traefik:v3.3',
    kind: 'form',
    form: {
      ...defaultDockerForm(),
      jobID: 'traefik',
      groupName: 'proxy',
      taskName: 'traefik',
      image: 'traefik:v3.3',
      portLabel: 'web',
      portTo: 80,
      cpu: 200,
      memory: 128,
      count: 1,
    },
  },
  {
    id: 'http-echo',
    titleKey: 'apps.echo.title',
    descKey: 'apps.echo.desc',
    category: 'utility',
    imageLabel: 'hashicorp/http-echo:1.0',
    kind: 'form',
    form: {
      ...defaultDockerForm(),
      jobID: 'http-echo',
      groupName: 'echo',
      taskName: 'server',
      image: 'hashicorp/http-echo:1.0',
      portLabel: 'http',
      portTo: 5678,
      cpu: 100,
      memory: 64,
      envText: 'ECHO_TEXT=hello from nomad',
      count: 1,
    },
  },
  {
    id: 'redis',
    titleKey: 'apps.redis.title',
    descKey: 'apps.redis.desc',
    category: 'data',
    imageLabel: 'redis:7-alpine',
    kind: 'form',
    form: {
      ...defaultDockerForm(),
      jobID: 'redis',
      groupName: 'cache',
      taskName: 'redis',
      image: 'redis:7-alpine',
      portLabel: 'redis',
      portTo: 6379,
      cpu: 200,
      memory: 128,
      count: 1,
    },
  },
  {
    id: 'postgres',
    titleKey: 'apps.postgres.title',
    descKey: 'apps.postgres.desc',
    category: 'data',
    imageLabel: 'postgres:16-alpine',
    kind: 'form',
    form: {
      ...defaultDockerForm(),
      jobID: 'postgres',
      groupName: 'db',
      taskName: 'postgres',
      image: 'postgres:16-alpine',
      portLabel: 'db',
      portTo: 5432,
      cpu: 500,
      memory: 512,
      envText: 'POSTGRES_PASSWORD=changeme\nPOSTGRES_USER=nomad\nPOSTGRES_DB=app',
      count: 1,
    },
  },
  {
    id: 'rabbitmq',
    titleKey: 'apps.rabbitmq.title',
    descKey: 'apps.rabbitmq.desc',
    category: 'messaging',
    imageLabel: 'rabbitmq:3-management-alpine',
    kind: 'form',
    form: {
      ...defaultDockerForm(),
      jobID: 'rabbitmq',
      groupName: 'mq',
      taskName: 'rabbitmq',
      image: 'rabbitmq:3-management-alpine',
      portLabel: 'amqp',
      portTo: 5672,
      cpu: 300,
      memory: 256,
      count: 1,
    },
  },
  {
    id: 'prometheus',
    titleKey: 'apps.prometheus.title',
    descKey: 'apps.prometheus.desc',
    category: 'observability',
    imageLabel: 'prom/prometheus:v2.55.1',
    kind: 'form',
    form: {
      ...defaultDockerForm(),
      jobID: 'prometheus',
      groupName: 'monitor',
      taskName: 'prometheus',
      image: 'prom/prometheus:v2.55.1',
      portLabel: 'http',
      portTo: 9090,
      cpu: 500,
      memory: 512,
      count: 1,
    },
  },
  {
    id: 'grafana',
    titleKey: 'apps.grafana.title',
    descKey: 'apps.grafana.desc',
    category: 'observability',
    imageLabel: 'grafana/grafana:11.3.1',
    kind: 'form',
    form: {
      ...defaultDockerForm(),
      jobID: 'grafana',
      groupName: 'ui',
      taskName: 'grafana',
      image: 'grafana/grafana:11.3.1',
      portLabel: 'http',
      portTo: 3000,
      cpu: 200,
      memory: 256,
      envText: 'GF_SECURITY_ADMIN_PASSWORD=admin',
      count: 1,
    },
  },
  {
    id: 'batch-hello',
    titleKey: 'apps.batchHello.title',
    descKey: 'apps.batchHello.desc',
    category: 'utility',
    imageLabel: 'busybox:1.36',
    kind: 'hcl',
    hcl: `job "batch-hello" {
  datacenters = ["*"]
  type = "batch"

  group "hello" {
    count = 1
    task "hello" {
      driver = "docker"
      config {
        image   = "busybox:1.36"
        command = "echo"
        args    = ["hello from nomad batch"]
      }
      resources {
        cpu    = 50
        memory = 32
      }
    }
  }
}
`,
  },
  {
    id: 'alpine-sleep',
    titleKey: 'apps.alpineSleep.title',
    descKey: 'apps.alpineSleep.desc',
    category: 'utility',
    imageLabel: 'alpine:3.20',
    kind: 'form',
    form: {
      ...defaultDockerForm(),
      jobID: 'alpine-sleep',
      type: 'batch',
      groupName: 'sleep',
      taskName: 'sleep',
      image: 'alpine:3.20',
      portLabel: '',
      portTo: null,
      cpu: 50,
      memory: 32,
      envText: '',
      count: 1,
      command: 'sleep',
      argsText: '3600',
    },
  },
]

/** JobRun 模板页仍复用同一目录。 */
export const JOB_TEMPLATES = APP_CATALOG
