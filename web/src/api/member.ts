import api from './index'

export const memberApi = {
  list() {
    return api.get('/members')
  },
  transferCaptain(id: number) {
    return api.post(`/members/${id}/transfer-captain`)
  },
  removeMember(id: number) {
    return api.delete(`/members/${id}`)
  }
}
