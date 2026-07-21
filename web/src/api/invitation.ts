import api from './index'

export const invitationApi = {
  generateCode(type: string) {
    return api.post('/invitations/generate', { type })
  },
  getMyCodes() {
    return api.get('/invitations/my')
  }
}
