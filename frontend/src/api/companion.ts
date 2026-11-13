export interface CompanionStatus {
  serial: string
  installed: boolean
  enabled: boolean
  ready: boolean
  update_available: boolean
  version?: string
  error?: string
}
export async function getCompanionStatus(serial: string, signal: AbortSignal): Promise<CompanionStatus> {
  return request(serial, { signal })
}
export async function changeCompanion(serial: string, action: 'install' | 'enable' | 'disable', signal: AbortSignal): Promise<CompanionStatus> {
  return request(serial, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ action }), signal })
}
async function request(serial: string, options: RequestInit): Promise<CompanionStatus> {
  const response = await fetch(`/api/devices/${encodeURIComponent(serial)}/companion`, options)
  const body = await response.json()
  if (!response.ok) throw new Error(body.message)
  return body
}
