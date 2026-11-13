export interface SafetyPolicy { preset: string, preset_id: number, rules_count: number, always_allows: Record<string, boolean> }
export interface Approval { id: string, app_package?: string, app_name?: string, app_icon?: string, binding: { device: string, tool: string, args_hash: string }, title: string, detail?: string, intent?: string, rule: string, state: number, deadline: string }
export type ApprovalDecision = 'allow_once' | 'deny' | 'take_over'
export interface AuditTask { id: string, device_id: string, agent: string, title: string, state: string, summary?: string, started_at: string, step_count: number, approval_count: number }
export interface AuditStep { task_id: string, index: number, tool: string, args: string, intent?: string, status: string, elapsed_ms: number, screen_text?: string }
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { ...init, signal: AbortSignal.timeout(10000) })
  if (!response.headers.get('content-type')?.includes('application/json'))
    throw new Error('安全审核服务不可用，请检查服务状态')
  const body = await response.json()
  if (!response.ok)
    throw new Error(body.message)
  return body
}
export const fetchSafetyPolicy = () => request<SafetyPolicy>('/api/safety/policy')
export const saveSafetyPolicy = (preset: number) => request('/api/safety/policy', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ preset }) })
export const fetchApprovals = () => request<{ items: Approval[] | null }>('/api/safety/approvals')
export const decideApproval = (id: string, decision: ApprovalDecision) => request(`/api/safety/approvals/${encodeURIComponent(id)}/decide`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ decision }) })
export const fetchAuditTasks = () => request<{ tasks: AuditTask[] | null }>('/api/safety/history/tasks')
export const fetchAuditTask = (id: string) => request<{ task: AuditTask, steps: AuditStep[] | null }>(`/api/safety/history/tasks/${encodeURIComponent(id)}`)
