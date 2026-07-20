import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi } from '@/api/auth'

interface User {
  id: number
  username: string
  nickname: string
  role: string
  joined_at: string
  checkin_count: number
}

interface LoginResponse {
  access_token: string
  refresh_token: string
  expires_in: number
  user: User
}

export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref(localStorage.getItem('access_token') || '')
  const refreshToken = ref(localStorage.getItem('refresh_token') || '')
  const user = ref<User | null>(JSON.parse(localStorage.getItem('user') || 'null'))

  const isAuthenticated = computed(() => !!accessToken.value)
  const isCaptain = computed(() => user.value?.role === 'captain')

  function setTokens(data: LoginResponse) {
    accessToken.value = data.access_token
    refreshToken.value = data.refresh_token
    user.value = data.user

    localStorage.setItem('access_token', data.access_token)
    localStorage.setItem('refresh_token', data.refresh_token)
    localStorage.setItem('user', JSON.stringify(data.user))
  }

  function clearAuth() {
    accessToken.value = ''
    refreshToken.value = ''
    user.value = null

    localStorage.removeItem('access_token')
    localStorage.removeItem('refresh_token')
    localStorage.removeItem('user')
  }

  async function login(username: string, password: string) {
    const res = await authApi.login(username, password)
    const data = res.data || res
    setTokens(data)
    return data
  }

  async function register(username: string, password: string, nickname: string) {
    const res = await authApi.register(username, password, nickname)
    const data = res.data || res
    setTokens(data)
    return data
  }

  async function logout() {
    clearAuth()
  }

  return {
    accessToken,
    refreshToken,
    user,
    isAuthenticated,
    isCaptain,
    login,
    register,
    logout
  }
})
