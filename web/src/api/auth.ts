import api from './index'

export const authApi = {
  login(username: string, password: string) {
    return api.post('/auth/login', { username, password })
  },
  register(username: string, password: string, nickname: string, email: string, verificationCode: string) {
    return api.post('/auth/register', { username, password, nickname, email, verification_code: verificationCode })
  },
  refreshToken(refreshToken: string) {
    return api.post('/auth/refresh', { refresh_token: refreshToken })
  },
  sendVerificationCode(email: string) {
    return api.post('/auth/send-code', { email })
  }
}
