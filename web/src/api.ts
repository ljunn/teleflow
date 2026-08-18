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

export interface AuthStatus {
  configured: boolean
  authenticated: boolean
}

export interface UpdateResult {
  release: ReleaseInfo
  updated: boolean
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { Accept: 'application/json', 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as { error?: string } | null
    throw new Error(body?.error || `请求失败（${response.status}）`)
  }
  return response.json() as Promise<T>
}

export const api = {
  authStatus: () => request<AuthStatus>('/api/v1/auth/status'),
  setup: (password: string) => request<{ ok: boolean }>('/api/v1/auth/setup', { method: 'POST', body: JSON.stringify({ password }) }),
  login: (password: string) => request<{ ok: boolean }>('/api/v1/auth/login', { method: 'POST', body: JSON.stringify({ password }) }),
  logout: () => request<{ ok: boolean }>('/api/v1/auth/logout', { method: 'POST' }),
  systemInfo: () => request<SystemInfo>('/api/v1/system/info'),
  overview: () => request<Overview>('/api/v1/overview'),
  checkUpdate: () => request<ReleaseInfo>('/api/v1/system/update/check'),
  applyUpdate: () => request<UpdateResult>('/api/v1/system/update', { method: 'POST' }),
}
