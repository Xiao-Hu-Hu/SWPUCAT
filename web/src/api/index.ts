import axios from 'axios'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'

const api = axios.create({
  baseURL: '/api',
  timeout: 10000
})

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
    if (error.response?.status === 401 && !originalRequest._retry) {
      originalRequest._retry = true
      const authStore = useAuthStore()
      try {
        const { data } = await axios.post('/api/auth/refresh', {
          refresh_token: authStore.refreshToken
        })
        authStore.setTokens(data.data)
        originalRequest.headers.Authorization = `Bearer ${data.data.access_token}`
        return axios(originalRequest)
      } catch {
        authStore.logout()
        router.push({ name: 'home' })
      }
    }
    return Promise.reject(error)
  }
)

export default api
