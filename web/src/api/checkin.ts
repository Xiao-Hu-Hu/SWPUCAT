import api from './index'

export const checkinApi = {
  getStatus() {
    return api.get('/checkin/status')
  },
  clockIn() {
    return api.post('/checkin/clock-in')
  },
  clockOut() {
    return api.post('/checkin/clock-out')
  },
  getRecords(limit = 10) {
    return api.get('/checkin/records', { params: { limit } })
  },
  getStats(period = 'week') {
    return api.get('/checkin/stats', { params: { period } })
  },
  getOnlineMembers() {
    return api.get('/checkin/online')
  }
}
