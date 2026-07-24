import api from './index'

export const userApi = {
  getProfile() {
    return api.get('/profile')
  },
  updateProfile(nickname: string) {
    return api.put('/profile', { nickname })
  },
  updateTechDirection(techDirection: string) {
    return api.put('/profile/tech-direction', { tech_direction: techDirection })
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
  },
  uploadAvatar(formData: FormData) {
    return api.post('/profile/avatar', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  }
}
