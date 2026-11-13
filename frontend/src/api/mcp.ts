export interface McpClient {
  id: string
  name: string
  path: string
  status: 'installed' | 'not_installed' | 'conflict' | 'error'
  error?: string
  config: string
}
export interface McpConfiguration { endpoint: string, clients: McpClient[] }
async function request<T>(path: string, method = 'GET'): Promise<T> {
  const response = await fetch(path, { method, signal: AbortSignal.timeout(10000) })
  const data = await response.json()
  if (!response.ok)
    throw new Error(typeof data.error === 'string' ? data.error : `HTTP ${response.status}`)
  return data as T
}
export const fetchMcpConfiguration = () => request<McpConfiguration>('/api/ai-integrations/mcp')
export const setMcpInstallation = (id: string, installed: boolean) => request<McpClient>(`/api/ai-integrations/mcp/${encodeURIComponent(id)}`, installed ? 'POST' : 'DELETE')
