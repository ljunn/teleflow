export interface SystemInfo {
  name: string
  version: string
  commit: string
  buildDate: string
  publicUrl: string
}

export interface Overview {
  accounts: number
  online: number
  pendingJobs: number
  relayedToday: number
}

export interface ReleaseInfo {
  currentVersion: string
  latestVersion: string
  available: boolean
  configured: boolean
  releaseUrl?: string
  publishedAt?: string
  notes?: string
}

async function get<T>(path: string): Promise<T> {
  const response = await fetch(path, { headers: { Accept: 'application/json' } })
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as { error?: string } | null
    throw new Error(body?.error || `请求失败（${response.status}）`)
  }
  return response.json() as Promise<T>
}

export const api = {
  systemInfo: () => get<SystemInfo>('/api/v1/system/info'),
  overview: () => get<Overview>('/api/v1/overview'),
  checkUpdate: () => get<ReleaseInfo>('/api/v1/system/update/check'),
}
