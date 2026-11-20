import { Channel, invoke, isTauri } from '@tauri-apps/api/core'

export interface CheckResult {
  runtime_mode: string
  update_method: string
  can_upgrade: boolean
  update_reason: string
  current_version: string
  latest_version: string
  has_update: boolean
  release_notes: string
  published_at: string
  asset_name: string
  asset_size: number
  download_url: string
}

export type UpgradeState = 'idle' | 'downloading' | 'verifying' | 'applying' | 'success' | 'error'

export interface UpgradeProgress {
  state: UpgradeState
  percent: number
  downloaded: number
  total: number
  speed: string
  error?: string
}

export async function checkUpdate(): Promise<CheckResult> {
  if (isTauri()) return invoke<CheckResult>('desktop_check_update')
  const res = await fetch('/api/version/check')
  if (!res.ok) {
    throw new Error(`检查更新失败: ${res.statusText}`)
  }
  return res.json()
}

export async function getUpgradeStatus(): Promise<UpgradeProgress> {
  const res = await fetch('/api/version/status')
  if (!res.ok) {
    throw new Error(`获取状态失败: ${res.statusText}`)
  }
  return res.json()
}

export async function startUpgrade(params?: { download_url?: string; asset_size?: number }): Promise<{ status: string; message: string }> {
  const res = await fetch('/api/version/upgrade', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params || {}),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `触发更新失败: ${res.statusText}`)
  }
  return res.json()
}

export function subscribeUpgradeStream(onProgress: (p: UpgradeProgress) => void, onError: (err: any) => void): () => void {
  const es = new EventSource('/api/version/upgrade-stream')
  es.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data) as UpgradeProgress
      onProgress(data)
    } catch (e) {
      onError(e)
    }
  }
  es.onerror = (e) => {
    onError(e)
    es.close()
  }
  return () => {
    es.close()
  }
}

export function isDesktopUpdate(): boolean { return isTauri() }

export type UpdateChannel = 'stable' | 'beta'

export async function getUpdateChannel(): Promise<UpdateChannel> {
  const res = await fetch('/api/version/channel')
  if (!res.ok) throw new Error(`读取更新通道失败: ${res.statusText}`)
  const data = await res.json() as { channel?: string }
  return data.channel === 'beta' ? 'beta' : 'stable'
}

export async function setUpdateChannel(channel: UpdateChannel): Promise<void> {
  const res = await fetch('/api/version/channel', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ channel }),
  })
  if (!res.ok) throw new Error((await res.text()) || `保存更新通道失败: ${res.statusText}`)
}
export async function upgradeDesktop(onProgress: (value: UpgradeProgress) => void): Promise<void> {
 const channel = new Channel<UpgradeProgress>()
 channel.onmessage = onProgress
 await invoke('desktop_upgrade', { onProgress: channel })
}
