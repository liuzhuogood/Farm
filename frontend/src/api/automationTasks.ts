export interface TaskArtifact {
  id: string
  serial: string
  relative_path: string
  kind: 'image' | 'video' | 'file'
  size?: number
  download_url: string
  preview_url?: string
  removed?: boolean
}

export interface TaskStep {
  index: number
  line?: number
  title: string
  command: string
  status: 'pending' | 'running' | 'done' | 'failed' | 'skipped'
  started_at?: string
  ended_at?: string
  elapsed_ms: number
  result?: string
  error?: string
  artifacts: TaskArtifact[]
}

export interface AutomationTask {
  id: string
  serial: string
  title: string
  source: 'automation' | 'mcp'
  state: 'running' | 'done' | 'failed' | 'stopped'
  summary: string
  started_at: string
  updated_at: string
  ended_at?: string
  active: boolean
  steps: TaskStep[]
  artifacts: TaskArtifact[]
  directory?: string
  steps_truncated?: boolean
}

type ObjectValue = Record<string, unknown>
const object = (value: unknown): value is ObjectValue => value !== null && typeof value === 'object' && !Array.isArray(value)
const optionalString = (value: unknown) => value === undefined || typeof value === 'string'
const optionalNumber = (value: unknown) => value === undefined || (typeof value === 'number' && Number.isFinite(value))
function artifact(value: unknown): value is TaskArtifact {
  return object(value) && typeof value.id === 'string' && typeof value.serial === 'string'
    && typeof value.relative_path === 'string' && ['image', 'video', 'file'].includes(String(value.kind))
    && typeof value.download_url === 'string' && optionalString(value.preview_url) && optionalNumber(value.size)
    && (value.removed === undefined || typeof value.removed === 'boolean')
}
function step(value: unknown): value is TaskStep {
  return object(value) && typeof value.index === 'number' && optionalNumber(value.line)
    && typeof value.title === 'string' && typeof value.command === 'string'
    && ['pending', 'running', 'done', 'failed', 'skipped'].includes(String(value.status))
    && typeof value.elapsed_ms === 'number' && optionalString(value.started_at) && optionalString(value.ended_at)
    && optionalString(value.result) && optionalString(value.error) && Array.isArray(value.artifacts) && value.artifacts.every(artifact)
}
function task(value: unknown): value is AutomationTask {
  return object(value) && typeof value.id === 'string' && typeof value.serial === 'string' && typeof value.title === 'string'
    && ['automation', 'mcp'].includes(String(value.source)) && ['running', 'done', 'failed', 'stopped'].includes(String(value.state))
    && typeof value.summary === 'string' && typeof value.started_at === 'string' && typeof value.updated_at === 'string'
    && optionalString(value.ended_at) && optionalString(value.directory) && typeof value.active === 'boolean'
    && (value.steps_truncated === undefined || typeof value.steps_truncated === 'boolean')
    && Array.isArray(value.steps) && value.steps.every(item => step(item) && item.artifacts.every(media => media.serial === value.serial))
    && Array.isArray(value.artifacts) && value.artifacts.every(item => artifact(item) && item.serial === value.serial)
}

async function request(path: string, signal: AbortSignal, init: RequestInit = {}): Promise<unknown> {
  const response = await fetch(path, { ...init, signal })
  if (!response.headers.get('content-type')?.includes('application/json'))
    throw new Error(`任务服务返回异常（HTTP ${response.status}）`)
  const body: unknown = await response.json()
  if (!response.ok) {
    if (object(body) && typeof body.message === 'string') throw new Error(body.message)
    throw new Error(`任务请求失败（HTTP ${response.status}）`)
  }
  return body
}

export async function listAutomationTasks(serial: string, signal: AbortSignal): Promise<AutomationTask[]> {
  const body = await request(`/api/automation/tasks?${new URLSearchParams({ serial })}`, signal)
  if (!object(body) || !Array.isArray(body.tasks) || !body.tasks.every(task) || body.tasks.some(item => item.serial !== serial))
    throw new Error('任务数据格式不正确')
  return body.tasks
}

export async function fetchAutomationTask(serial: string, id: string, signal: AbortSignal): Promise<AutomationTask> {
  const body = await request(`/api/automation/tasks/${encodeURIComponent(id)}?${new URLSearchParams({ serial })}`, signal)
  if (!object(body) || !task(body.task) || body.task.serial !== serial || body.task.id !== id)
    throw new Error('任务详情格式不正确')
  return body.task
}

export async function stopAutomationTask(serial: string, id: string, signal: AbortSignal): Promise<AutomationTask> {
  const body = await request(`/api/automation/tasks/${encodeURIComponent(id)}/finish?${new URLSearchParams({ serial })}`, signal, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ outcome: 'stopped', summary: '用户停止任务' }),
  })
  if (!object(body) || !task(body.task) || body.task.serial !== serial || body.task.id !== id)
    throw new Error('任务结束结果格式不正确')
  return body.task
}

export function taskDirectoryURL(serial: string, directory: string): string {
  return `/api/automation/artifacts?${new URLSearchParams({ serial, directory })}`
}

// Only automation runtime records prove that a live task is open. MCP audit
// rows can retain historical running states after their session has disappeared.
export function defaultAutomationTask(tasks: AutomationTask[]): AutomationTask | undefined {
  const recent = [...tasks].sort((a, b) => Date.parse(b.started_at) - Date.parse(a.started_at))
  return recent.find(item => item.source === 'automation' && item.state === 'running' && item.active)
    ?? recent.find(item => item.source === 'automation' && item.state === 'running')
    ?? recent[0]
}
