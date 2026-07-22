import api from './index'

export const memberApi = {
  list() {
    return api.get('/members')
  },
  transferCaptain(id: number, verificationCode: string) {
    return api.post(`/members/${id}/transfer-captain`, { verification_code: verificationCode })
  },
  removeMember(id: number) {
    return api.delete(`/members/${id}`)
  }
}
