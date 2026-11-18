import axios from 'axios'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'

const api = axios.create({
  baseURL: '/api',
  timeout: 10000
})

export class StreamHTTPError extends Error {
  status: number
  code?: number

  constructor(status: number, message: string, code?: number) {
    super(message)
    this.name = 'StreamHTTPError'
    this.status = status
    this.code = code
  }
}

async function refreshStreamToken(signal?: AbortSignal) {
  const authStore = useAuthStore()
  const refreshToken = authStore.refreshToken
  const { data } = await axios.post('/api/auth/refresh', {
    refresh_token: refreshToken
  }, { signal })
  if (signal?.aborted) throw signal.reason
  if (authStore.refreshToken !== refreshToken) return
  authStore.setTokens(data.data)
}

/** Authenticated fetch for streaming endpoints that cannot use Axios buffering. */
export async function streamPost(path: string, body: unknown, signal?: AbortSignal, retry = true): Promise<Response> {
  const authStore = useAuthStore()
  const response = await fetch(`/api${path}`, {
    method: 'POST',
    headers: {
      Accept: 'application/x-ndjson',
      'Content-Type': 'application/json',
      ...(authStore.accessToken ? { Authorization: `Bearer ${authStore.accessToken}` } : {})
    },
    body: JSON.stringify(body),
    signal
  })
  if (response.status === 401 && retry) {
    const userId = authStore.user?.id
    try {
      await refreshStreamToken(signal)
      if (signal?.aborted || authStore.user?.id !== userId) throw new DOMException('Session ended', 'AbortError')
      return streamPost(path, body, signal, false)
    } catch (error) {
      if (signal?.aborted || authStore.user?.id !== userId) throw error
      await authStore.logout()
      await router.push({ name: 'login' })
      throw new StreamHTTPError(401, '登录状态已失效，请重新登录', 401)
    }
  }
  if (!response.ok) {
    let payload: any = null
    try {
      payload = await response.json()
    } catch {
      // Keep the stable fallback below for non-JSON proxy errors.
    }
    throw new StreamHTTPError(response.status, payload?.message || '请求失败，请稍后重试', payload?.code)
  }
  return response
}

api.interceptors.request.use((config) => {
  const authStore = useAuthStore()
  if (authStore.accessToken) {
    config.headers.Authorization = `Bearer ${authStore.accessToken}`
  }
  return config
})

api.interceptors.response.use(
  (response) => response.data,
  async (error) => {
    const originalRequest = error.config
    if (error.response?.status === 401 && !originalRequest._retry && !originalRequest.signal?.aborted) {
      originalRequest._retry = true
      const authStore = useAuthStore()
      const userId = authStore.user?.id
      try {
        const { data } = await axios.post('/api/auth/refresh', {
          refresh_token: authStore.refreshToken
        }, { signal: originalRequest.signal })
        if (originalRequest.signal?.aborted || authStore.user?.id !== userId) return Promise.reject(error)
        authStore.setTokens(data.data)
        originalRequest.headers.Authorization = `Bearer ${data.data.access_token}`
        return axios(originalRequest)
      } catch {
        if (!originalRequest.signal?.aborted && authStore.user?.id === userId) {
          authStore.logout()
          router.push({ name: 'login' })
        }
      }
    }
    return Promise.reject(error)
  }
)

export default api
