import axios from 'axios'

export const api = axios.create({ baseURL: '/api' })

// Attach JWT from localStorage to every request
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

// Redirect to /login on 401
api.interceptors.response.use(
  (r) => r,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.href = '/login'
    }
    return Promise.reject(err)
  },
)

// ─── Types ────────────────────────────────────────────────────────────────────

export interface Subscription {
  id: string
  token: string
  name: string | null
  sub_url: string
  uuid: string
  traffic_used_bytes: number
  traffic_limit_bytes: number | null
  expires_at: string | null
  is_enabled: boolean
  is_traffic_exceeded: boolean
  is_expired: boolean
  created_at: string
  last_used_at: string | null
}

export interface Node {
  id: string
  name: string
  public_host: string
  xray_api_addr: string | null
  hy2_api_url: string | null
  agent_url: string | null
  agent_version: string | null
  agent_last_seen: string | null
  is_active: boolean
}

export interface Inbound {
  id: string
  node_id: string
  tag: string
  protocol: string
  port: number
  settings: Record<string, unknown>
  is_active: boolean
}

export interface Plan {
  id: string
  name: string
  description: string | null
  price_usd: number | null
  traffic_limit_bytes: number | null
  duration_days: number | null
  is_active: boolean
}

export interface AdminUser {
  id: string
  username: string
  role: string
  is_active: boolean
  created_at: string
}

// ─── Auth ─────────────────────────────────────────────────────────────────────

export const login = (username: string, password: string) =>
  api.post<{ token: string; role: string }>('/auth/login', { username, password })

export const setup = (username: string, password: string) =>
  api.post('/setup', { username, password })

// ─── Subscriptions ────────────────────────────────────────────────────────────

export const listSubs = (limit = 50, offset = 0) =>
  api.get<{ items: Subscription[] }>(`/subscriptions?limit=${limit}&offset=${offset}`)

export const getSub = (id: string) =>
  api.get<Subscription>(`/subscriptions/${id}`)

export const createSub = (data: {
  name?: string
  inbound_ids: string[]
  traffic_limit_bytes?: number
  expires_at?: string
}) => api.post<Subscription>('/subscriptions', data)

export const deleteSub = (id: string) => api.delete(`/subscriptions/${id}`)
export const enableSub = (id: string) => api.post(`/subscriptions/${id}/enable`)
export const disableSub = (id: string) => api.post(`/subscriptions/${id}/disable`)
export const resetTraffic = (id: string) => api.post(`/subscriptions/${id}/reset-traffic`)

// ─── Nodes ────────────────────────────────────────────────────────────────────

export const listNodes = () => api.get<{ items: Node[] }>('/nodes')

export const createNode = (data: {
  name: string
  public_host: string
  xray_api_addr?: string
  hy2_api_url?: string
  hy2_api_secret?: string
  agent_url?: string
  agent_secret?: string
}) => api.post<Node>('/nodes', data)

export const getNodeInbounds = (nodeId: string) =>
  api.get<{ items: Inbound[] }>(`/nodes/${nodeId}/inbounds`)

export const createInbound = (
  nodeId: string,
  data: { tag: string; protocol: string; port: number; settings: Record<string, unknown> },
) => api.post<Inbound>(`/nodes/${nodeId}/inbounds`, data)

// ─── Users ────────────────────────────────────────────────────────────────────

export const listUsers = () => api.get<{ items: AdminUser[] }>('/users')
export const createUser = (data: { username: string; password: string; role: string }) =>
  api.post<AdminUser>('/users', data)
export const deleteUser = (id: string) => api.delete(`/users/${id}`)

// ─── Plans ────────────────────────────────────────────────────────────────────

export const listPlans = () => api.get<{ items: Plan[] }>('/plans')

// ─── Helpers ─────────────────────────────────────────────────────────────────

export const fmtBytes = (b: number | null): string => {
  if (b === null) return '∞'
  if (b === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(b) / Math.log(1024))
  return `${(b / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

export const fmtDate = (s: string | null): string => {
  if (!s) return '∞'
  return new Date(s).toLocaleDateString('ru-RU')
}
