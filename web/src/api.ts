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
  defaultPassword: string
}

export interface UpdateResult {
  release: ReleaseInfo
  updated: boolean
}

export interface Capabilities {
  telegramConfigured: boolean
  relayBotConfigured: boolean
  connectedAccounts: number
}

export interface TelegramAccount {
  id: number
  phone: string
  displayName: string
  status: string
  username: string
  lastError: string
  lastSeenAt?: string
  codeSentAt?: string
  createdAt: string
}

export interface DiscoveryTask {
  id: number
  query: string
  sourceType: string
  status: string
  resultCount: number
  lastError: string
  createdAt: string
}

export interface Campaign {
  id: number
  name: string
  kind: string
  target: string
  message: string
  status: string
  runAt?: string
  sentCount: number
  failedCount: number
  lastError: string
  createdAt: string
}

export interface RelaySettings {
  botUsername: string
  masterUsername: string
  enabled: boolean
  updatedAt?: string
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
  changePassword: (input: { currentPassword: string; newPassword: string; confirmPassword: string }) => request<{ ok: boolean }>('/api/v1/auth/password', { method: 'POST', body: JSON.stringify(input) }),
  logout: () => request<{ ok: boolean }>('/api/v1/auth/logout', { method: 'POST' }),
  systemInfo: () => request<SystemInfo>('/api/v1/system/info'),
  overview: () => request<Overview>('/api/v1/overview'),
  capabilities: () => request<Capabilities>('/api/v1/capabilities'),
  accounts: () => request<TelegramAccount[]>('/api/v1/accounts'),
  createAccount: (input: { phone: string; displayName: string }) => request<{ id: number; status: string }>('/api/v1/accounts', { method: 'POST', body: JSON.stringify(input) }),
  requestAccountCode: (id: number) => request<{ status: string }>(`/api/v1/accounts/${id}/auth/code`, { method: 'POST', body: '{}' }),
  verifyAccountCode: (id: number, code: string) => request<{ status: string }>(`/api/v1/accounts/${id}/auth/verify`, { method: 'POST', body: JSON.stringify({ code }) }),
  verifyAccountPassword: (id: number, password: string) => request<{ status: string }>(`/api/v1/accounts/${id}/auth/password`, { method: 'POST', body: JSON.stringify({ password }) }),
  deleteAccount: (id: number) => request<{ ok: boolean }>(`/api/v1/accounts/${id}`, { method: 'DELETE' }),
  discovery: () => request<DiscoveryTask[]>('/api/v1/discovery'),
  createDiscovery: (input: { query: string; sourceType: string }) => request<{ id: number; status: string }>('/api/v1/discovery', { method: 'POST', body: JSON.stringify(input) }),
  deleteDiscovery: (id: number) => request<{ ok: boolean }>(`/api/v1/discovery/${id}`, { method: 'DELETE' }),
  campaigns: () => request<Campaign[]>('/api/v1/campaigns'),
  createCampaign: (input: { name: string; kind: string; target: string; message: string; runAt: string }) => request<{ id: number; status: string }>('/api/v1/campaigns', { method: 'POST', body: JSON.stringify(input) }),
  updateCampaignStatus: (id: number, status: string) => request<{ ok: boolean; status: string }>(`/api/v1/campaigns/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),
  deleteCampaign: (id: number) => request<{ ok: boolean }>(`/api/v1/campaigns/${id}`, { method: 'DELETE' }),
  relay: () => request<RelaySettings>('/api/v1/relay'),
  updateRelay: (input: RelaySettings) => request<{ ok: boolean }>('/api/v1/relay', { method: 'PUT', body: JSON.stringify(input) }),
  checkUpdate: () => request<ReleaseInfo>('/api/v1/system/update/check'),
  applyUpdate: () => request<UpdateResult>('/api/v1/system/update', { method: 'POST' }),
}
