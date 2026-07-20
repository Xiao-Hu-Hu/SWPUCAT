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
  }
}
