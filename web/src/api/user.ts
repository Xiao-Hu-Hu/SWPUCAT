import api from './index'

export const userApi = {
  getProfile() {
    return api.get('/profile')
  },
  updateProfile(nickname: string) {
    return api.put('/profile', { nickname })
  },
  changePassword(oldPassword: string, newPassword: string, verificationCode: string) {
    return api.put('/profile/password', {
      old_password: oldPassword,
      new_password: newPassword,
      verification_code: verificationCode
    })
  },
  sendVerificationCode(email: string) {
    return api.post('/auth/send-code', { email })
  }
}
