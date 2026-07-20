import api from './index'

export const authApi = {
  login(username: string, password: string) {
    return api.post('/auth/login', { username, password })
  },
  register(username: string, password: string, nickname: string) {
    return api.post('/auth/register', { username, password, nickname })
  },
  refreshToken(refreshToken: string) {
    return api.post('/auth/refresh', { refresh_token: refreshToken })
  }
}
